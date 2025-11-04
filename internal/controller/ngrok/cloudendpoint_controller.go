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
	"time"

	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	"github.com/go-logr/logr"
	"github.com/ngrok/ngrok-api-go/v7"
	ingressv1alpha1 "github.com/ngrok/ngrok-operator/api/ingress/v1alpha1"
	ngrokv1alpha1 "github.com/ngrok/ngrok-operator/api/ngrok/v1alpha1"
	"github.com/ngrok/ngrok-operator/internal/controller"
	domainpkg "github.com/ngrok/ngrok-operator/internal/domain"
	"github.com/ngrok/ngrok-operator/internal/ngrokapi"
)

const (
	trafficPolicyNameIndex = "spec.trafficPolicyName"
)

// CloudEndpointReconciler reconciles a CloudEndpoint object
type CloudEndpointReconciler struct {
	client.Client
	Scheme     *runtime.Scheme
	controller *controller.BaseController[*ngrokv1alpha1.CloudEndpoint]

	Log            logr.Logger
	Recorder       record.EventRecorder
	NgrokClientset ngrokapi.Clientset

	DefaultDomainReclaimPolicy *ingressv1alpha1.DomainReclaimPolicy
	DomainManager              *domainpkg.Manager
}

// Define a custom error types to catch and handle requeuing logic for
var (
	ErrInvalidTrafficPolicyConfig = errors.New("invalid TrafficPolicy configuration: both TrafficPolicyName and TrafficPolicy are set")
)

// SetupWithManager sets up the controller with the Manager.
// It also sets up a Field Indexer to index Cloud Endpoints by their Traffic Policy name
// Additionally, this triggers updates when a trafficPolicy is created or updated but not when deleted
func (r *CloudEndpointReconciler) SetupWithManager(mgr ctrl.Manager) error {
	if r.NgrokClientset == nil {
		return errors.New("NgrokClientset is required")
	}

	// Initialize domain manager if not already set
	if r.DomainManager == nil {
		r.DomainManager = &domainpkg.Manager{
			Client:                     r.Client,
			Recorder:                   r.Recorder,
			DefaultDomainReclaimPolicy: r.DefaultDomainReclaimPolicy,
		}
	}

	r.controller = &controller.BaseController[*ngrokv1alpha1.CloudEndpoint]{
		Kube:     r.Client,
		Log:      r.Log,
		Recorder: r.Recorder,

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
			if errors.Is(err, ErrInvalidTrafficPolicyConfig) {
				r.Recorder.Event(cr, v1.EventTypeWarning, "ConfigError", err.Error())
				r.Log.Error(err, "invalid TrafficPolicy configuration", "name", cr.Name, "namespace", cr.Namespace)
				return ctrl.Result{}, nil // Do not requeue
			}
			return controller.CtrlResultForErr(err)
		},
	}

	if err := mgr.GetFieldIndexer().IndexField(context.Background(), &ngrokv1alpha1.CloudEndpoint{}, trafficPolicyNameIndex, func(o client.Object) []string {
		clep, ok := o.(*ngrokv1alpha1.CloudEndpoint)
		if !ok {
			return nil
		}
		return []string{clep.Spec.TrafficPolicyName}
	}); err != nil {
		return err
	}

	return ctrl.NewControllerManagedBy(mgr).
		For(&ngrokv1alpha1.CloudEndpoint{}).
		Watches(
			&ngrokv1alpha1.NgrokTrafficPolicy{},
			r.controller.NewEnqueueRequestForMapFunc(r.findCloudEndpointForTrafficPolicy),
			// Don't process delete events as it will just fail to look it up.
			// Instead rely on the user to either delete the CloudEndpoint CR or update it with a new TrafficPolicy name
			builder.WithPredicates(&predicate.Funcs{
				DeleteFunc: func(_ event.DeleteEvent) bool {
					return false
				},
			}),
		).
		Watches(
			&ingressv1alpha1.Domain{},
			r.controller.NewEnqueueRequestForMapFunc(r.findCloudEndpointsForDomain),
		).
		WithEventFilter(
			predicate.Or(
				predicate.GenerationChangedPredicate{},
			),
		).
		Complete(r)
}

// #region Reconcile CRUD

// +kubebuilder:rbac:groups=ngrok.k8s.ngrok.com,resources=cloudendpoints,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=ngrok.k8s.ngrok.com,resources=cloudendpoints/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=ngrok.k8s.ngrok.com,resources=cloudendpoints/finalizers,verbs=update

func (r *CloudEndpointReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	return r.controller.Reconcile(ctx, req, new(ngrokv1alpha1.CloudEndpoint))
}

// Create will make sure a domain is created before creating the Cloud Endpoint
// It also looks up the Traffic Policy and creates the Cloud Endpoint using this Traffic Policy JSON
func (r *CloudEndpointReconciler) create(ctx context.Context, clep *ngrokv1alpha1.CloudEndpoint) error {
	// EnsureDomainExists handles its own domain-related status
	domainResult, err := r.DomainManager.EnsureDomainExists(ctx, clep, domainpkg.DomainCheckParams{
		URL:      clep.Spec.URL,
		Bindings: clep.Spec.Bindings,
	})
	if err != nil {
		return r.updateStatus(ctx, clep, nil, domainResult, err)
	}

	policy, err := r.getTrafficPolicy(ctx, clep)
	if err != nil {
		return r.updateStatus(ctx, clep, nil, domainResult, err)
	}

	createParams := &ngrok.EndpointCreate{
		Type:           "cloud",
		URL:            clep.Spec.URL,
		Description:    &clep.Spec.Description,
		Metadata:       &clep.Spec.Metadata,
		TrafficPolicy:  policy,
		Bindings:       clep.Spec.Bindings,
		PoolingEnabled: clep.Spec.PoolingEnabled,
	}

	ngrokClep, err := r.NgrokClientset.Endpoints().Create(ctx, createParams)
	if err != nil {
		setCloudEndpointCreatedCondition(clep, false, ReasonCloudEndpointCreationFailed, fmt.Sprintf("Failed to create cloud endpoint: %v", err))
		return r.updateStatus(ctx, clep, nil, domainResult, err)
	}

	// Set success condition
	setCloudEndpointCreatedCondition(clep, true, ReasonCloudEndpointCreated, "CloudEndpoint created successfully")

	// Update status (includes requeue check for domain readiness)
	return r.updateStatus(ctx, clep, ngrokClep, domainResult, nil)
}

// Update is called when we have a status ID and want to update the resource in the ngrok API
// If it fails to find the resource by ID, create a new one instead
func (r *CloudEndpointReconciler) update(ctx context.Context, clep *ngrokv1alpha1.CloudEndpoint) error {
	domainResult, err := r.DomainManager.EnsureDomainExists(ctx, clep, domainpkg.DomainCheckParams{
		URL:      clep.Spec.URL,
		Bindings: clep.Spec.Bindings,
	})
	if err != nil {
		return r.updateStatus(ctx, clep, nil, domainResult, err)
	}

	policy, err := r.getTrafficPolicy(ctx, clep)
	if err != nil {
		return r.updateStatus(ctx, clep, nil, domainResult, err)
	}

	updateParams := &ngrok.EndpointUpdate{
		ID:             clep.Status.ID,
		Url:            &clep.Spec.URL,
		Description:    &clep.Spec.Description,
		Metadata:       &clep.Spec.Metadata,
		TrafficPolicy:  &policy,
		Bindings:       clep.Spec.Bindings,
		PoolingEnabled: clep.Spec.PoolingEnabled,
	}

	ngrokClep, err := r.NgrokClientset.Endpoints().Update(ctx, updateParams)
	if ngrok.IsNotFound(err) {
		// Couldn't find endpoint by ID to update, so blank it out and create a new one
		r.Recorder.Event(clep, v1.EventTypeWarning, "EndpointNotFound", fmt.Sprintf("Failed to update endpoint %s by ID because it was not found. Creating a new one", clep.Status.ID))
		clep.Status.ID = ""
		_ = r.Client.Status().Update(ctx, clep)
		return r.create(ctx, clep)
	}
	if err != nil {
		setCloudEndpointCreatedCondition(clep, false, ReasonCloudEndpointCreationFailed, fmt.Sprintf("Failed to update cloud endpoint: %v", err))
		return r.updateStatus(ctx, clep, nil, domainResult, err)
	}

	// Set success condition
	setCloudEndpointCreatedCondition(clep, true, ReasonCloudEndpointCreated, "CloudEndpoint updated successfully")

	// Update status (includes requeue check for domain readiness)
	return r.updateStatus(ctx, clep, ngrokClep, domainResult, nil)
}

// Simply attempt to delete it. The base controller handles not found errors
func (r *CloudEndpointReconciler) delete(ctx context.Context, clep *ngrokv1alpha1.CloudEndpoint) error {
	return r.NgrokClientset.Endpoints().Delete(ctx, clep.Status.ID)
}

func (r *CloudEndpointReconciler) updateStatus(ctx context.Context, clep *ngrokv1alpha1.CloudEndpoint, ngrokClep *ngrok.Endpoint, domainResult *domainpkg.DomainResult, statusErr error) error {
	// Update status fields if we have an endpoint
	if ngrokClep != nil {
		clep.Status.ID = ngrokClep.ID
	}

	// Update domain status fields
	if domainResult != nil && domainResult.Domain != nil {
		// Set the deprecated domain status for backwards compatibility
		//nolint:staticcheck
		clep.Status.Domain = ngrokv1alpha1.ConvertDomainStatusToDeprecatedDomainStatus(&domainResult.Domain.Status)
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

// findCloudEndpointForTrafficPolicy searches for any Cloud Endpoints CRs that have a reference to a particular Traffic Policy
func (r *CloudEndpointReconciler) findCloudEndpointForTrafficPolicy(ctx context.Context, o client.Object) []ctrl.Request {
	tp, ok := o.(*ngrokv1alpha1.NgrokTrafficPolicy)
	if !ok {
		return nil
	}

	// Use the index to find CloudEndpoints that reference this TrafficPolicy
	var cloudEndpointList ngrokv1alpha1.CloudEndpointList
	if err := r.Client.List(ctx, &cloudEndpointList,
		client.InNamespace(tp.Namespace),
		client.MatchingFields{trafficPolicyNameIndex: tp.Name}); err != nil {
		r.Log.Error(err, "failed to list CloudEndpoints using index")
		return nil
	}

	// Collect the requests for matching CloudEndpoints
	var requests []ctrl.Request
	for _, clep := range cloudEndpointList.Items {
		requests = append(requests, ctrl.Request{
			NamespacedName: client.ObjectKey{
				Name:      clep.Name,
				Namespace: clep.Namespace,
			},
		})
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
		if domainpkg.EndpointReferencesDomain(&ep, domain) {
			requests = append(requests, ctrl.Request{
				NamespacedName: client.ObjectKeyFromObject(&ep),
			})
		}
	}
	return requests
}

// getTrafficPolicy returns the TrafficPolicy JSON string from either the name reference or inline policy
func (r *CloudEndpointReconciler) getTrafficPolicy(ctx context.Context, clep *ngrokv1alpha1.CloudEndpoint) (string, error) {
	// Ensure mutually exclusive fields are not both set
	if clep.Spec.TrafficPolicyName != "" && clep.Spec.TrafficPolicy != nil {
		return "", ErrInvalidTrafficPolicyConfig
	}

	var policy string
	var err error

	// Handle either finding the TrafficPolicy by name or using the inline policy
	if clep.Spec.TrafficPolicyName != "" {
		policy, err = r.findTrafficPolicyByName(ctx, clep.Spec.TrafficPolicyName, clep.Namespace)
		if err != nil {
			return "", err
		}
	} else if clep.Spec.TrafficPolicy != nil {
		// Marshal the inline TrafficPolicy to JSON
		policyBytes, err := clep.Spec.TrafficPolicy.Policy.MarshalJSON()
		if err != nil {
			return "", fmt.Errorf("failed to marshal inline TrafficPolicy: %w", err)
		}
		policy = string(policyBytes)
	}

	return policy, nil
}

// findTrafficPolicyByName fetches the TrafficPolicy CRD from the API server and returns the JSON policy as a string
func (r *CloudEndpointReconciler) findTrafficPolicyByName(ctx context.Context, tpName, tpNamespace string) (string, error) {
	log := ctrl.LoggerFrom(ctx).WithValues("name", tpName, "namespace", tpNamespace)

	// Create a TrafficPolicy object to store the fetched result
	tp := &ngrokv1alpha1.NgrokTrafficPolicy{}
	key := client.ObjectKey{Name: tpName, Namespace: tpNamespace}

	// Attempt to get the TrafficPolicy from the API server
	if err := r.Client.Get(ctx, key, tp); err != nil {
		r.Recorder.Event(tp, v1.EventTypeWarning, "TrafficPolicyNotFound", fmt.Sprintf("Failed to find TrafficPolicy %s", tpName))
		return "", err
	}

	// Convert the JSON policy to a string
	policyBytes, err := tp.Spec.Policy.MarshalJSON()
	if err != nil {
		log.Error(err, "failed to marshal TrafficPolicy JSON")
		return "", err
	}

	return string(policyBytes), nil
}
