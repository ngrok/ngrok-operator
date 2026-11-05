/*
MIT License

Copyright (c) 2024 ngrok, Inc.

Permission is hereby granted, free of charge, to any person obtaining a copy
of this software and associated documentation files (the "Software"), to deal
in the Software without restriction, including without limitation the rights
to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
copies of the Software, and to permit persons to whom the Software is
furnished to do so, subject to the following conditions:

The above copyright notice and this permission notice shall be included in all
copies or substantial portions of the Software.

THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
SOFTWARE.
*/

package ngrok

import (
	"context"
	"errors"
	"time"

	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/events"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	"github.com/go-logr/logr"
	"github.com/ngrok/ngrok-api-go/v9"
	ingressv1alpha1 "github.com/ngrok/ngrok-operator/api/ingress/v1alpha1"
	ngrokv1 "github.com/ngrok/ngrok-operator/api/ngrok/v1"
	ngrokv1alpha1 "github.com/ngrok/ngrok-operator/api/ngrok/v1alpha1"
	"github.com/ngrok/ngrok-operator/internal/cloudendpoint"
	"github.com/ngrok/ngrok-operator/internal/controller"
	"github.com/ngrok/ngrok-operator/internal/controller/labels"
	domainpkg "github.com/ngrok/ngrok-operator/internal/domain"
	"github.com/ngrok/ngrok-operator/internal/ngrokapi"
	trafficpolicypkg "github.com/ngrok/ngrok-operator/internal/trafficpolicy"
)

// CloudEndpointReconciler reconciles a CloudEndpoint object
type CloudEndpointReconciler struct {
	client.Client
	Scheme     *runtime.Scheme
	controller *controller.BaseController[*ngrokv1alpha1.CloudEndpoint]
	syncer     *cloudendpoint.Syncer

	Log            logr.Logger
	Recorder       events.EventRecorder
	NgrokClientset ngrokapi.Clientset
	DrainState     controller.DrainState

	ControllerLabels           labels.ControllerLabelValues
	DefaultDomainReclaimPolicy *ngrokv1.DomainReclaimPolicy
	DomainManager              *domainpkg.Manager
	TrafficPolicyManager       *trafficpolicypkg.Manager
}

// SetupWithManager sets up the controller with the Manager.
// It also sets up a Field Indexer to index Cloud Endpoints by their Traffic Policy name.
func (r *CloudEndpointReconciler) SetupWithManager(mgr ctrl.Manager) error {
	if r.NgrokClientset == nil {
		return errors.New("NgrokClientset is required")
	}

	// Initialize domain manager if not already set
	if r.DomainManager == nil {
		if err := labels.ValidateControllerLabelValues(r.ControllerLabels); err != nil {
			return err
		}

		opts := []domainpkg.ManagerOption{
			domainpkg.WithControllerLabels(r.ControllerLabels),
			domainpkg.WithAPIReader(mgr.GetAPIReader()),
		}

		if r.DefaultDomainReclaimPolicy != nil {
			opts = append(opts, domainpkg.WithDefaultDomainReclaimPolicy(*r.DefaultDomainReclaimPolicy))
		}

		dm, err := domainpkg.NewManager(r.Client, r.Recorder, opts...)
		if err != nil {
			return err
		}
		r.DomainManager = dm
	}

	if r.TrafficPolicyManager == nil {
		r.TrafficPolicyManager = trafficpolicypkg.NewManager(r.Client, r.Recorder)
	}

	// TODO(#923): Domains becomes the ngrok.com/v1 Domain manager once Domain moves.
	r.syncer = &cloudendpoint.Syncer{
		Endpoints:       r.NgrokClientset.Endpoints(),
		Domains:         r.DomainManager,
		TrafficPolicies: r.TrafficPolicyManager,
		Recorder:        r.Recorder,
	}

	r.controller = &controller.BaseController[*ngrokv1alpha1.CloudEndpoint]{
		Kube:       r.Client,
		Log:        r.Log,
		Recorder:   r.Recorder,
		DrainState: r.DrainState,

		StatusID: func(clep *ngrokv1alpha1.CloudEndpoint) string { return clep.GetID() },
		Create:   r.sync,
		Update:   r.sync,
		Delete:   r.delete,
		ErrResult: func(_ controller.BaseControllerOp, cr *ngrokv1alpha1.CloudEndpoint, err error) (ctrl.Result, error) {
			retryableErrors := []int{
				// 18016 and 18017 are state based errors that can happen when endpoint pooling for a given URL
				// disagrees with an already active endpoint with the same URL. Since this state can change in ngrok when moving
				// between agent and cloud endpoints, we need to retry on this 400, instead of assuming its terminal like we
				// do for other 400s.
				//
				// Ref:
				//  * https://ngrok.com/docs/errors/err_ngrok_18016/
				//  * https://ngrok.com/docs/errors/err_ngrok_18017/
				18016,
				18017,
			}
			if ngrok.IsErrorCode(err, retryableErrors...) {
				return ctrl.Result{}, err
			}
			if errors.Is(err, domainpkg.ErrDomainNotReady) {
				return ctrl.Result{RequeueAfter: 10 * time.Second}, nil
			}
			if errors.Is(err, trafficpolicypkg.ErrInvalidConfig) || errors.Is(err, trafficpolicypkg.ErrInvalidPolicyJSON) {
				r.Recorder.Eventf(cr, nil, v1.EventTypeWarning, "ConfigError", "Reconcile", err.Error())
				r.Log.Error(err, "invalid TrafficPolicy configuration", "name", cr.Name, "namespace", cr.Namespace)
				return ctrl.Result{}, nil // Do not requeue
			}
			if errors.Is(err, trafficpolicypkg.ErrTrafficPolicyNotFound) {
				// Terminal: the condition is already False and an event was
				// emitted during Resolve. Don't requeue — the TrafficPolicy
				// watch re-enqueues this endpoint when the policy is (re)created.
				r.Log.Info("referenced TrafficPolicy not found; awaiting (re)creation", "name", cr.Name, "namespace", cr.Namespace)
				return ctrl.Result{}, nil
			}
			return controller.CtrlResultForErr(err)
		},
	}

	if err := mgr.GetFieldIndexer().IndexField(context.Background(), &ngrokv1alpha1.CloudEndpoint{}, trafficpolicypkg.RefIndex, indexCloudEndpointTrafficPolicyRefs); err != nil {
		return err
	}

	return ctrl.NewControllerManagedBy(mgr).
		For(&ngrokv1alpha1.CloudEndpoint{}, builder.WithPredicates(
			predicate.Or(
				predicate.AnnotationChangedPredicate{},
				predicate.GenerationChangedPredicate{},
			),
		)).
		// LEGACY-trafficpolicy-kind: BEGIN
		// Watch the deprecated ngrok.k8s.ngrok.com/v1alpha1 NgrokTrafficPolicy
		// so endpoints resolving through the fallback path re-enqueue on
		// legacy-kind updates. Delete this Watches call at cleanup.
		Watches(
			&ngrokv1alpha1.NgrokTrafficPolicy{},
			r.controller.NewEnqueueRequestForMapFunc(r.findCloudEndpointForTrafficPolicy),
		).
		// LEGACY-trafficpolicy-kind: END
		Watches(
			// Canonical ngrok.com/v1 TrafficPolicy. Same mapper — the index
			// key is namespace/name only, so both kinds enqueue identically.
			&ngrokv1.TrafficPolicy{},
			r.controller.NewEnqueueRequestForMapFunc(r.findCloudEndpointForTrafficPolicy),
		).
		Watches(
			&ngrokv1.Domain{},
			r.controller.NewEnqueueRequestForMapFunc(r.findCloudEndpointsForDomain),
		).
		// LEGACY-domain-kind: delete this Watches call at cleanup.
		Watches(
			&ingressv1alpha1.Domain{},
			r.controller.NewEnqueueRequestForMapFunc(r.findCloudEndpointsForDomain),
		).
		Complete(r)
}

// indexCloudEndpointTrafficPolicyRefs returns the composite key for the
// TrafficPolicy this CloudEndpoint depends on so updates to that policy
// requeue the endpoint. Inline-only CloudEndpoints produce no index entry
// because they have no external ref to watch.
func indexCloudEndpointTrafficPolicyRefs(o client.Object) []string {
	clep, ok := o.(*ngrokv1alpha1.CloudEndpoint)
	if !ok {
		return nil
	}
	if k := trafficpolicypkg.IndexKey(clep); k != "" {
		return []string{k}
	}
	return nil
}

// #region Reconcile CRUD

func (r *CloudEndpointReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	return r.controller.Reconcile(ctx, req, new(ngrokv1alpha1.CloudEndpoint))
}

func (r *CloudEndpointReconciler) sync(ctx context.Context, clep *ngrokv1alpha1.CloudEndpoint) error {
	return r.controller.ReconcileStatus(ctx, clep, r.syncer.Sync(ctx, clep))
}

func (r *CloudEndpointReconciler) delete(ctx context.Context, clep *ngrokv1alpha1.CloudEndpoint) error {
	return r.syncer.Delete(ctx, clep)
}

// #region Helper Functions

// findCloudEndpointForTrafficPolicy returns reconcile requests for every
// CloudEndpoint that references the supplied TrafficPolicy via
// spec.trafficPolicy.targetRef.
//
// The mapper is kind-agnostic by construction: it accepts anything satisfying
// trafficpolicypkg.TrafficPolicyResource, which today means both the canonical
// ngrok.com/v1 TrafficPolicy and the deprecated
// ngrok.k8s.ngrok.com/v1alpha1 NgrokTrafficPolicy. That works because the
// endpoint index key is namespace/name only, with no kind component. Testing
// against the interface rather than a list of concrete types means this
// function needs no edit when the legacy kind is dropped.
func (r *CloudEndpointReconciler) findCloudEndpointForTrafficPolicy(ctx context.Context, o client.Object) []ctrl.Request {
	if _, ok := o.(trafficpolicypkg.TrafficPolicyResource); !ok {
		return nil
	}

	var list ngrokv1alpha1.CloudEndpointList
	if err := r.Client.List(ctx, &list, client.MatchingFields{trafficpolicypkg.RefIndex: trafficpolicypkg.LookupKey(o)}); err != nil {
		r.Log.Error(err, "failed to list CloudEndpoints using index")
		return nil
	}

	requests := make([]ctrl.Request, 0, len(list.Items))
	for _, clep := range list.Items {
		requests = append(requests, ctrl.Request{Name: clep.Name, Namespace: clep.Namespace})
	}
	return requests
}

// findCloudEndpointsForDomain searches for any CloudEndpoint CRs that reference a particular Domain
func (r *CloudEndpointReconciler) findCloudEndpointsForDomain(ctx context.Context, domain client.Object) []ctrl.Request {
	if _, ok := domainpkg.HostnameOf(domain); !ok {
		return nil
	}

	var endpoints ngrokv1alpha1.CloudEndpointList
	if err := r.Client.List(ctx, &endpoints, client.InNamespace(domain.GetNamespace())); err != nil {
		return nil
	}

	var requests []ctrl.Request
	for _, ep := range endpoints.Items {
		if ep.GetDomainRef().Matches(domain) {
			requests = append(requests, ctrl.Request{
				NamespacedName: client.ObjectKeyFromObject(&ep),
			})
		}
	}
	return requests
}
