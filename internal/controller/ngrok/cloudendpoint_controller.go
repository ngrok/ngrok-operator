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
	"fmt"
	"slices"
	"time"

	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/events"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	"github.com/go-logr/logr"
	"github.com/ngrok/ngrok-api-go/v7"
	commonv1alpha1 "github.com/ngrok/ngrok-operator/api/common/v1alpha1"
	ingressv1alpha1 "github.com/ngrok/ngrok-operator/api/ingress/v1alpha1"
	ngrokv1alpha1 "github.com/ngrok/ngrok-operator/api/ngrok/v1alpha1"
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

	Log            logr.Logger
	Recorder       events.EventRecorder
	NgrokClientset ngrokapi.Clientset
	DrainState     controller.DrainState

	ControllerLabels           labels.ControllerLabelValues
	DefaultDomainReclaimPolicy *ingressv1alpha1.DomainReclaimPolicy
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

	r.controller = &controller.BaseController[*ngrokv1alpha1.CloudEndpoint]{
		Kube:       r.Client,
		Log:        r.Log,
		Recorder:   r.Recorder,
		DrainState: r.DrainState,

		StatusID: func(clep *ngrokv1alpha1.CloudEndpoint) string { return clep.Status.ID },
		Create:   r.create,
		Update:   r.update,
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
		Watches(
			&ngrokv1alpha1.NgrokTrafficPolicy{},
			r.controller.NewEnqueueRequestForMapFunc(r.findCloudEndpointForTrafficPolicy),
		).
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

// Create will make sure a domain is created before creating the Cloud Endpoint
// It also looks up the Traffic Policy and creates the Cloud Endpoint using this Traffic Policy JSON
func (r *CloudEndpointReconciler) create(ctx context.Context, clep *ngrokv1alpha1.CloudEndpoint) error {
	// EnsureDomainExists handles its own domain-related status
	domainResult, err := r.DomainManager.EnsureDomainExists(ctx, clep)
	if err != nil {
		return r.updateStatus(ctx, clep, nil, domainResult, err)
	}

	tpResult, err := r.TrafficPolicyManager.Resolve(ctx, clep)
	if err != nil {
		return r.updateStatus(ctx, clep, nil, domainResult, err)
	}

	return r.createWithPolicy(ctx, clep, domainResult, tpResult.Policy)
}

// createWithPolicy issues the ngrok API Create call using an already-resolved
// policy. Split out of create() so the update() → 404 → recreate fallback can
// reuse the policy it already resolved instead of calling
// TrafficPolicyManager.Resolve a second time on the same reconcile.
func (r *CloudEndpointReconciler) createWithPolicy(ctx context.Context, clep *ngrokv1alpha1.CloudEndpoint, domainResult *domainpkg.DomainResult, policy string) error {
	metadata := commonv1alpha1.MetadataAPIString(clep.Spec.Metadata)
	createParams := &ngrok.EndpointCreate{
		Type:           "cloud",
		URL:            clep.Spec.URL,
		Description:    &clep.Spec.Description,
		Metadata:       &metadata,
		TrafficPolicy:  policy,
		Bindings:       clep.Spec.Bindings,
		PoolingEnabled: clep.Spec.PoolingEnabled,
	}

	ngrokClep, err := r.NgrokClientset.Endpoints().Create(ctx, createParams)
	if err != nil {
		return r.recordWriteError(ctx, clep, domainResult, policy, err, fmt.Sprintf("Failed to create cloud endpoint: %v", err))
	}

	return r.recordWriteSuccess(ctx, clep, ngrokClep, domainResult, "CloudEndpoint created successfully")
}

// Update is called when we have a status ID and want to update the resource in the ngrok API
// If it fails to find the resource by ID, create a new one instead
func (r *CloudEndpointReconciler) update(ctx context.Context, clep *ngrokv1alpha1.CloudEndpoint) error {
	domainResult, err := r.DomainManager.EnsureDomainExists(ctx, clep)
	if err != nil {
		return r.updateStatus(ctx, clep, nil, domainResult, err)
	}

	tpResult, err := r.TrafficPolicyManager.Resolve(ctx, clep)
	if err != nil {
		return r.updateStatus(ctx, clep, nil, domainResult, err)
	}
	policy := tpResult.Policy

	// Fetch current endpoint state from the ngrok API so we can compare
	// before issuing an update. This avoids redundant API writes on every
	// requeue cycle (e.g. while waiting for a domain to become ready).
	currentEndpoint, err := r.NgrokClientset.Endpoints().Get(ctx, clep.Status.ID)
	if ngrok.IsNotFound(err) {
		r.Recorder.Eventf(clep, nil, v1.EventTypeWarning, "EndpointNotFound", "Reconcile", fmt.Sprintf("Failed to find endpoint %s by ID. Creating a new one", clep.Status.ID))
		clep.Status.ID = ""
		clep.Status.AssignedURL = ""
		if err := r.controller.ReconcileStatus(ctx, clep, nil); err != nil {
			return err
		}
		return r.createWithPolicy(ctx, clep, domainResult, policy)
	}
	if err != nil {
		return r.updateStatus(ctx, clep, nil, domainResult, err)
	}

	// Skip the API update if nothing has changed
	if !endpointNeedsUpdate(currentEndpoint, clep.Spec, policy) {
		return r.recordWriteSuccess(ctx, clep, currentEndpoint, domainResult, "CloudEndpoint updated successfully")
	}

	metadata := commonv1alpha1.MetadataAPIString(clep.Spec.Metadata)
	updateParams := &ngrok.EndpointUpdate{
		ID:             clep.Status.ID,
		Url:            &clep.Spec.URL,
		Description:    &clep.Spec.Description,
		Metadata:       &metadata,
		TrafficPolicy:  &policy,
		Bindings:       clep.Spec.Bindings,
		PoolingEnabled: clep.Spec.PoolingEnabled,
	}

	ngrokClep, err := r.NgrokClientset.Endpoints().Update(ctx, updateParams)
	if err != nil {
		return r.recordWriteError(ctx, clep, domainResult, policy, err, fmt.Sprintf("Failed to update cloud endpoint: %v", err))
	}

	return r.recordWriteSuccess(ctx, clep, ngrokClep, domainResult, "CloudEndpoint updated successfully")
}

// recordWriteSuccess marks the resolved traffic policy as applied and the
// endpoint as created, then writes status. Called after a downstream
// create/update (or a skipped no-op update) succeeds.
func (r *CloudEndpointReconciler) recordWriteSuccess(ctx context.Context, clep *ngrokv1alpha1.CloudEndpoint, ngrokClep *ngrok.Endpoint, domainResult *domainpkg.DomainResult, message string) error {
	r.TrafficPolicyManager.MarkApplied(clep)
	setCloudEndpointCreatedCondition(clep, true, ReasonCloudEndpointCreated, message)
	return r.updateStatus(ctx, clep, ngrokClep, domainResult, nil)
}

// recordWriteError marks the endpoint creation as failed and, when the ngrok
// API rejected the request because of the policy itself, surfaces that on the
// TrafficPolicyApplied condition. Called after a downstream create/update fails.
func (r *CloudEndpointReconciler) recordWriteError(ctx context.Context, clep *ngrokv1alpha1.CloudEndpoint, domainResult *domainpkg.DomainResult, policy string, err error, message string) error {
	setCloudEndpointCreatedCondition(clep, false, ReasonCloudEndpointCreationFailed, message)
	if policy != "" && ngrokapi.IsTrafficPolicyError(err.Error()) {
		r.TrafficPolicyManager.SetError(clep, ngrokapi.SanitizeErrorMessage(err.Error()))
	}
	return r.updateStatus(ctx, clep, nil, domainResult, err)
}

// Simply attempt to delete it. The base controller handles not found errors
func (r *CloudEndpointReconciler) delete(ctx context.Context, clep *ngrokv1alpha1.CloudEndpoint) error {
	return r.NgrokClientset.Endpoints().Delete(ctx, clep.Status.ID)
}

func (r *CloudEndpointReconciler) updateStatus(ctx context.Context, clep *ngrokv1alpha1.CloudEndpoint, ngrokClep *ngrok.Endpoint, domainResult *domainpkg.DomainResult, statusErr error) error {
	// Update status fields if we have an endpoint
	if ngrokClep != nil {
		clep.Status.ID = ngrokClep.ID
		clep.Status.AssignedURL = ngrokClep.URL
	}

	// Calculate overall Ready condition based on other conditions and domain status
	calculateCloudEndpointReadyCondition(clep, domainResult)

	// Write status to k8s API
	if err := r.controller.ReconcileStatus(ctx, clep, statusErr); err != nil {
		return err
	}

	// Requeue if domain is not ready (fallback to watch for convergence)
	if domainResult != nil {
		return domainResult.RequeueError()
	}
	return nil
}

// #region Helper Functions

// findCloudEndpointForTrafficPolicy returns reconcile requests for every
// CloudEndpoint that references the supplied NgrokTrafficPolicy via
// spec.trafficPolicy.targetRef.
func (r *CloudEndpointReconciler) findCloudEndpointForTrafficPolicy(ctx context.Context, o client.Object) []ctrl.Request {
	tp, ok := o.(*ngrokv1alpha1.NgrokTrafficPolicy)
	if !ok {
		return nil
	}

	var list ngrokv1alpha1.CloudEndpointList
	if err := r.Client.List(ctx, &list, client.MatchingFields{trafficpolicypkg.RefIndex: trafficpolicypkg.LookupKey(tp)}); err != nil {
		r.Log.Error(err, "failed to list CloudEndpoints using index")
		return nil
	}

	requests := make([]ctrl.Request, 0, len(list.Items))
	for _, clep := range list.Items {
		requests = append(requests, ctrl.Request{NamespacedName: client.ObjectKey{Name: clep.Name, Namespace: clep.Namespace}})
	}
	return requests
}

// findCloudEndpointsForDomain searches for any CloudEndpoint CRs that reference a particular Domain
func (r *CloudEndpointReconciler) findCloudEndpointsForDomain(ctx context.Context, o client.Object) []ctrl.Request {
	domain, ok := o.(*ingressv1alpha1.Domain)
	if !ok {
		return nil
	}

	var endpoints ngrokv1alpha1.CloudEndpointList
	if err := r.Client.List(ctx, &endpoints, client.InNamespace(domain.Namespace)); err != nil {
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

// endpointNeedsUpdate compares the current endpoint state from the ngrok API
// against the desired state derived from the CloudEndpoint spec and the resolved
// traffic policy. Returns true if an API update call is necessary.
func endpointNeedsUpdate(current *ngrok.Endpoint, spec ngrokv1alpha1.CloudEndpointSpec, policy string) bool {
	if current.URL != spec.URL {
		return true
	}
	if current.Description != spec.Description {
		return true
	}
	if current.Metadata != commonv1alpha1.MetadataAPIString(spec.Metadata) {
		return true
	}
	if current.TrafficPolicy != policy {
		return true
	}
	if !slices.Equal(current.Bindings, spec.Bindings) {
		return true
	}
	if spec.PoolingEnabled != nil && current.PoolingEnabled != *spec.PoolingEnabled {
		return true
	}
	return false
}
