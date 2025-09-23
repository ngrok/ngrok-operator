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

package agent

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"time"

	"github.com/go-logr/logr"
	ingressv1alpha1 "github.com/ngrok/ngrok-operator/api/ingress/v1alpha1"
	ngrokv1alpha1 "github.com/ngrok/ngrok-operator/api/ngrok/v1alpha1"
	"github.com/ngrok/ngrok-operator/internal/controller"
	"github.com/ngrok/ngrok-operator/internal/ngrokapi"
	"github.com/ngrok/ngrok-operator/internal/util"
	"github.com/ngrok/ngrok-operator/pkg/agent"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
)

const (
	trafficPolicyNameIndex     = "spec.trafficPolicy.targetRef.name"
	clientCertificateRefsIndex = "spec.clientCertificateRefs"
)

// indexClientCertificateRefs extracts client certificate reference keys for indexing
func indexClientCertificateRefs(o client.Object) []string {
	aep, ok := o.(*ngrokv1alpha1.AgentEndpoint)
	if !ok {
		return nil
	}
	var keys []string
	for _, ref := range aep.Spec.ClientCertificateRefs {
		effectiveNamespace := aep.Namespace
		if ref.Namespace != nil && *ref.Namespace != "" {
			effectiveNamespace = *ref.Namespace
		}
		key := effectiveNamespace + "/" + ref.Name
		keys = append(keys, key)
	}
	return keys
}

var (
	ErrInvalidTrafficPolicyConfig = errors.New("invalid TrafficPolicy configuration: both targetRef and inline are set")
)

// +kubebuilder:rbac:groups=ngrok.k8s.ngrok.com,resources=agentendpoints,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=ngrok.k8s.ngrok.com,resources=agentendpoints/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=ngrok.k8s.ngrok.com,resources=agentendpoints/finalizers,verbs=update
// +kubebuilder:rbac:groups=ngrok.k8s.ngrok.com,resources=ngroktrafficpolicies,verbs=get;list;watch
// +kubebuilder:rbac:groups=ingress.k8s.ngrok.com,resources=domains,verbs=get;list;watch;patch;create;delete
// +kubebuilder:rbac:groups="",resources=secrets,verbs=get;list;watch

// AgentEndpointReconciler reconciles an AgentEndpoint object
type AgentEndpointReconciler struct {
	client.Client

	Log         logr.Logger
	Scheme      *runtime.Scheme
	Recorder    record.EventRecorder
	AgentDriver agent.Driver

	controller *controller.BaseController[*ngrokv1alpha1.AgentEndpoint]

	DefaultDomainReclaimPolicy *ingressv1alpha1.DomainReclaimPolicy
}

// SetupWithManager sets up the controller with the Manager
// +kubebuilder:rbac:groups="",resources=secrets,verbs=get;list;watch

func (r *AgentEndpointReconciler) SetupWithManager(mgr ctrl.Manager) error {
	if r.AgentDriver == nil {
		return errors.New("AgentDriver is nil")
	}

	r.controller = &controller.BaseController[*ngrokv1alpha1.AgentEndpoint]{
		Kube:     r.Client,
		Log:      r.Log,
		Recorder: r.Recorder,
		Update:   r.update,
		Delete:   r.delete,
		StatusID: r.statusID,
		ErrResult: func(_ controller.BaseControllerOp, cr *ngrokv1alpha1.AgentEndpoint, err error) (ctrl.Result, error) {
			if errors.Is(err, util.ErrDomainCreating) {
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

	if err := mgr.GetFieldIndexer().IndexField(context.Background(), &ngrokv1alpha1.AgentEndpoint{}, trafficPolicyNameIndex, func(o client.Object) []string {
		aep, ok := o.(*ngrokv1alpha1.AgentEndpoint)
		if !ok || aep.Spec.TrafficPolicy == nil || aep.Spec.TrafficPolicy.Reference == nil {
			return nil
		}
		return []string{aep.Spec.TrafficPolicy.Reference.Name}
	}); err != nil {
		return err
	}

	if err := mgr.GetFieldIndexer().IndexField(
		context.Background(),
		&ngrokv1alpha1.AgentEndpoint{},
		clientCertificateRefsIndex,
		indexClientCertificateRefs,
	); err != nil {
		return err
	}

	return ctrl.NewControllerManagedBy(mgr).
		For(&ngrokv1alpha1.AgentEndpoint{}).
		Watches(
			&ngrokv1alpha1.NgrokTrafficPolicy{},
			r.controller.NewEnqueueRequestForMapFunc(r.findAgentEndpointForTrafficPolicy),
			// Don't process delete events as it will just fail to look it up.
			// Instead rely on the user to either delete the AgentEndpoint CR or update it with a new TrafficPolicy name
			builder.WithPredicates(&predicate.Funcs{
				DeleteFunc: func(_ event.DeleteEvent) bool {
					return false
				},
			}),
		).
		Watches(
			&v1.Secret{},
			r.controller.NewEnqueueRequestForMapFunc(r.findAgentEndpointForSecret),
			builder.WithPredicates(&predicate.Funcs{
				DeleteFunc: func(_ event.DeleteEvent) bool {
					return false
				},
			}),
		).
		WithEventFilter(
			predicate.Or(
				predicate.AnnotationChangedPredicate{},
				predicate.GenerationChangedPredicate{},
			),
		).
		Complete(r)
}

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.13.1/pkg/reconcile
func (r *AgentEndpointReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	return r.controller.Reconcile(ctx, req, new(ngrokv1alpha1.AgentEndpoint))
}

func (r *AgentEndpointReconciler) update(ctx context.Context, endpoint *ngrokv1alpha1.AgentEndpoint) error {
	setReconcilingCondition(endpoint, "Reconciling AgentEndpoint")

	var finalErr error
	defer func() {
		// Single status update per reconcile
		_ = r.controller.ReconcileStatus(ctx, endpoint, finalErr)
	}()

	err := r.ensureDomainExists(ctx, endpoint)
	if err != nil {
		finalErr = err
		return err
	}

	trafficPolicy, err := r.getTrafficPolicy(ctx, endpoint)
	if err != nil {
		finalErr = err
		return err
	}

	clientCerts, err := r.getClientCerts(ctx, endpoint)
	if err != nil {
		finalErr = err
		return err
	}

	tunnelName := r.statusID(endpoint)
	result, err := r.AgentDriver.CreateAgentEndpoint(ctx, tunnelName, endpoint.Spec, trafficPolicy, clientCerts)

	r.updateEndpointStatus(endpoint, result, err, trafficPolicy != "")
	endpoint.Status.AttachedTrafficPolicy = util.GetAgentEndpointTrafficPolicyName(&endpoint.Spec)
	if result != nil {
		endpoint.Status.AssignedURL = result.URL
	}

	finalErr = err
	return err
}

func (r *AgentEndpointReconciler) delete(ctx context.Context, endpoint *ngrokv1alpha1.AgentEndpoint) error {
	tunnelName := r.statusID(endpoint)
	return r.AgentDriver.DeleteAgentEndpoint(ctx, tunnelName)
	// TODO: Delete any associated domain
}

func (r *AgentEndpointReconciler) statusID(endpoint *ngrokv1alpha1.AgentEndpoint) string {
	return fmt.Sprintf("%s/%s", endpoint.Namespace, endpoint.Name)
}

// findAgentEndpointForTrafficPolicy searches for any Agent Endpoints CRs that have a reference to a particular Traffic Policy
func (r *AgentEndpointReconciler) findAgentEndpointForTrafficPolicy(ctx context.Context, o client.Object) []ctrl.Request {
	tp, ok := o.(*ngrokv1alpha1.NgrokTrafficPolicy)
	if !ok {
		return nil
	}

	// Use the index to find AgentEndpoints that reference this TrafficPolicy
	var agentEndpointList ngrokv1alpha1.AgentEndpointList
	if err := r.Client.List(ctx, &agentEndpointList,
		client.InNamespace(tp.Namespace),
		client.MatchingFields{trafficPolicyNameIndex: tp.Name}); err != nil {
		r.Log.Error(err, "failed to list AgentEndpoints using index")
		return nil
	}

	// Collect the requests for matching AgentEndpoints
	var requests []ctrl.Request
	for _, aep := range agentEndpointList.Items {
		requests = append(requests, ctrl.Request{
			NamespacedName: client.ObjectKey{
				Name:      aep.Name,
				Namespace: aep.Namespace,
			},
		})
	}

	return requests
}

func (r *AgentEndpointReconciler) findAgentEndpointForSecret(ctx context.Context, o client.Object) []ctrl.Request {
	secret, ok := o.(*v1.Secret)
	if !ok {
		return nil
	}

	secretKey := fmt.Sprintf("%s/%s", secret.Namespace, secret.Name)

	// Use the index to find AgentEndpoints that reference this Secret
	var agentEndpointList ngrokv1alpha1.AgentEndpointList
	if err := r.Client.List(ctx, &agentEndpointList,
		client.MatchingFields{
			clientCertificateRefsIndex: secretKey,
		},
	); err != nil {
		r.Log.Error(err, "failed to list AgentEndpoints using index")
		return nil
	}

	// Collect the requests for matching AgentEndpoints
	var requests []ctrl.Request
	for _, aep := range agentEndpointList.Items {
		requests = append(requests, ctrl.Request{
			NamespacedName: client.ObjectKey{
				Name:      aep.Name,
				Namespace: aep.Namespace,
			},
		})
	}

	return requests
}

// getTrafficPolicy returns the TrafficPolicy JSON string from either the name reference or inline policy.
func (r *AgentEndpointReconciler) getTrafficPolicy(ctx context.Context, aep *ngrokv1alpha1.AgentEndpoint) (string, error) {
	// Use shared utility for traffic policy resolution
	var spec interface{} = aep.Spec.TrafficPolicy
	policy, err := util.ResolveTrafficPolicy(ctx, r.Client, r.Recorder, aep.Namespace, spec)
	if err != nil {
		if errors.Is(err, util.ErrInvalidTrafficPolicyConfig) {
			setTrafficPolicyError(aep, err.Error())
		} else {
			setTrafficPolicyError(aep, err.Error())
		}
		return "", err
	}
	if policy != "" {
		setTrafficPolicyCondition(aep, true, "TrafficPolicyApplied", "Traffic policy successfully applied")
	}
	return policy, nil
}

// getClientCerts retrieves client certificates for upstream TLS connections.
func (r *AgentEndpointReconciler) getClientCerts(ctx context.Context, aep *ngrokv1alpha1.AgentEndpoint) ([]tls.Certificate, error) {
	if aep.Spec.ClientCertificateRefs == nil {
		return nil, nil // Nothing to fetch
	}

	ret := []tls.Certificate{}
	for _, clientCertRef := range aep.Spec.ClientCertificateRefs {
		key := client.ObjectKey{Name: clientCertRef.Name, Namespace: aep.Namespace}
		if clientCertRef.Namespace != nil {
			key.Namespace = *clientCertRef.Namespace
		}

		// Attempt to get the Secret from the API server
		certSecret := &v1.Secret{}
		if err := r.Client.Get(ctx, key, certSecret); err != nil {
			r.Recorder.Event(certSecret, v1.EventTypeWarning, "SecretNotFound", fmt.Sprintf("Failed to find Secret %s", clientCertRef.Name))
			setConfigError(aep, err.Error())
			return nil, err
		}

		certData, exists := certSecret.Data["tls.crt"]
		if !exists {
			err := fmt.Errorf("tls.crt data is missing from AgentEndpoint clientCertRef %q", fmt.Sprintf("%s.%s", key.Name, key.Namespace))
			setConfigError(aep, err.Error())
			return nil, err
		}
		keyData, exists := certSecret.Data["tls.key"]
		if !exists {
			err := fmt.Errorf("tls.key data is missing from AgentEndpoint clientCertRef %q", fmt.Sprintf("%s.%s", key.Name, key.Namespace))
			setConfigError(aep, err.Error())
			return nil, err
		}

		cert, err := tls.X509KeyPair(certData, keyData)
		if err != nil {
			err := fmt.Errorf("failed to parse TLS certificate AgentEndpoint clientCertRef %q: %w", fmt.Sprintf("%s.%s", key.Name, key.Namespace), err)
			setConfigError(aep, err.Error())
			return nil, err
		}

		ret = append(ret, cert)
	}
	return ret, nil
}

// findTrafficPolicyByName fetches the TrafficPolicy CRD from the API server and returns the JSON policy as a string
func (r *AgentEndpointReconciler) findTrafficPolicyByName(ctx context.Context, tpName, tpNamespace string) (string, error) {
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



// ensureDomainExists checks if the Domain CRD exists for the given URL, and if not, creates it.
func (r *AgentEndpointReconciler) ensureDomainExists(ctx context.Context, aep *ngrokv1alpha1.AgentEndpoint) error {
	_, err := util.EnsureDomainExists(ctx, r.Client, r.Recorder, aep, aep.Spec.URL, r.DefaultDomainReclaimPolicy)
	
	if err != nil {
		if errors.Is(err, util.ErrDomainCreating) {
			setDomainWaiting(aep, "Domain is being created")
		} else {
			setDomainError(aep, err.Error())
		}
		return err
	}

	// Domain exists and is ready, or no domain needed
	setDomainReady(aep, "Domain is ready")
	return nil
}

// updateEndpointStatus updates the endpoint status based on creation result from the AgentDriver.
func (r *AgentEndpointReconciler) updateEndpointStatus(endpoint *ngrokv1alpha1.AgentEndpoint, result *agent.EndpointResult, err error, hasTrafficPolicy bool) {
	// Update status based on endpoint creation result
	switch {
	case err != nil:
		errMsg := ngrokapi.SanitizeErrorMessage(err.Error())

		// Check if the error message indicates a traffic policy configuration issue
		if hasTrafficPolicy && ngrokapi.IsTrafficPolicyError(errMsg) {
			setTrafficPolicyError(endpoint, errMsg)
		} else {
			setEndpointCreateFailed(endpoint, ReasonNgrokAPIError, errMsg)
		}
	case result != nil:
		// Success - update status with endpoint information from ngrok
		setEndpointSuccess(endpoint, "AgentEndpoint is active and ready", hasTrafficPolicy)
	default:
		// Unexpected case - no result and no error
		setEndpointCreateFailed(endpoint, ReasonNgrokAPIError, "No endpoint result returned")
	}
}
