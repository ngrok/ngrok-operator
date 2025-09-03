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
	"strings"
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

var (
	ErrDomainCreating             = errors.New("domain is being created, requeue after delay")
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
			if errors.Is(err, ErrDomainCreating) {
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
		func(o client.Object) []string {
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
		},
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
	// Set initial condition to reconciling
	setReconcilingCondition(endpoint, "Reconciling AgentEndpoint")

	err := r.ensureDomainExists(ctx, endpoint)
	if err != nil {
		return r.controller.ReconcileStatus(ctx, endpoint, err)
	}

	trafficPolicy, err := r.getTrafficPolicy(ctx, endpoint)
	if err != nil {
		return r.controller.ReconcileStatus(ctx, endpoint, err)
	}

	clientCerts, err := r.getClientCerts(ctx, endpoint)
	if err != nil {
		return r.controller.ReconcileStatus(ctx, endpoint, err)
	}

	tunnelName := r.statusID(endpoint)
	result, err := r.AgentDriver.CreateAgentEndpoint(ctx, tunnelName, endpoint.Spec, trafficPolicy, clientCerts)

	r.updateEndpointStatus(endpoint, result, err, trafficPolicy)
	return r.controller.ReconcileStatus(ctx, endpoint, err)
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
			trafficPolicyNameIndex: secretKey,
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
// Updates the passed in AgentEndpoint with the status conditions based on the results.
func (r *AgentEndpointReconciler) getTrafficPolicy(ctx context.Context, aep *ngrokv1alpha1.AgentEndpoint) (string, error) {
	if aep.Spec.TrafficPolicy == nil {
		return "", nil // No traffic policy to fetch, no error
	}

	// Ensure mutually exclusive fields are not both set
	if aep.Spec.TrafficPolicy.Reference != nil && aep.Spec.TrafficPolicy.Inline != nil {
		setTrafficPolicyCondition(aep, false, ReasonTrafficPolicyError, ErrInvalidTrafficPolicyConfig.Error())
		setReadyCondition(aep, false, ReasonTrafficPolicyError, ErrInvalidTrafficPolicyConfig.Error())
		return "", ErrInvalidTrafficPolicyConfig
	}

	var policy string
	var err error

	switch aep.Spec.TrafficPolicy.Type() {
	case ngrokv1alpha1.TrafficPolicyCfgType_Inline:
		policyBytes, err := aep.Spec.TrafficPolicy.Inline.MarshalJSON()
		if err != nil {
			setTrafficPolicyCondition(aep, false, ReasonTrafficPolicyError, err.Error())
			setReadyCondition(aep, false, ReasonTrafficPolicyError, err.Error())
			return "", fmt.Errorf("failed to marshal inline TrafficPolicy: %w", err)
		}
		policy = string(policyBytes)
	case ngrokv1alpha1.TrafficPolicyCfgType_K8sRef:
		// Right now, we only support traffic policies that are in the same namespace as the agent endpoint
		policy, err = r.findTrafficPolicyByName(ctx, aep.Spec.TrafficPolicy.Reference.Name, aep.Namespace)
		if err != nil {
			setTrafficPolicyCondition(aep, false, ReasonTrafficPolicyError, err.Error())
			setReadyCondition(aep, false, ReasonTrafficPolicyError, err.Error())
			return "", err
		}
	}

	return policy, nil
}

// getClientCerts retrieves client certificates for upstream TLS connections.
// Updates the passed in AgentEndpoint with the status conditions based on the results.
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
			setReadyCondition(aep, false, ReasonConfigError, err.Error())
			return nil, err
		}

		certData, exists := certSecret.Data["tls.crt"]
		if !exists {
			err := fmt.Errorf("tls.crt data is missing from AgentEndpoint clientCertRef %q", fmt.Sprintf("%s.%s", key.Name, key.Namespace))
			setReadyCondition(aep, false, ReasonConfigError, err.Error())
			return nil, err
		}
		keyData, exists := certSecret.Data["tls.key"]
		if !exists {
			err := fmt.Errorf("tls.key data is missing from AgentEndpoint clientCertRef %q", fmt.Sprintf("%s.%s", key.Name, key.Namespace))
			setReadyCondition(aep, false, ReasonConfigError, err.Error())
			return nil, err
		}

		cert, err := tls.X509KeyPair(certData, keyData)
		if err != nil {
			err := fmt.Errorf("failed to parse TLS certificate AgentEndpoint clientCertRef %q: %w", fmt.Sprintf("%s.%s", key.Name, key.Namespace), err)
			setReadyCondition(aep, false, ReasonConfigError, err.Error())
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

// ensureDomainExists checks if the Domain CRD exists, and if not, creates it.
// Updates the passed in AgentEndpoint with the status conditions based on the results.
func (r *AgentEndpointReconciler) ensureDomainExists(ctx context.Context, aep *ngrokv1alpha1.AgentEndpoint) error {
	parsedURL, err := util.ParseAndSanitizeEndpointURL(aep.Spec.URL, true)
	if err != nil {
		r.Recorder.Event(aep, v1.EventTypeWarning, "InvalidURL", fmt.Sprintf("Failed to parse URL: %s", aep.Spec.URL))
		setDomainReadyCondition(aep, false, ReasonNgrokAPIError, err.Error())
		setReadyCondition(aep, false, ReasonNgrokAPIError, err.Error())
		return fmt.Errorf("failed to parse URL %q from AgentEndpoint \"%s.%s\"", aep.Spec.URL, aep.Name, aep.Namespace)
	}

	// Ngrok TCP URLs do not have to be reserved
	if parsedURL.Scheme == "tcp" && strings.HasSuffix(parsedURL.Hostname(), "tcp.ngrok.io") {
		setDomainReadyCondition(aep, true, "DomainReady", "Domain is ready")
		return nil
	}

	// TODO: generate a domain for blank strings
	domain := parsedURL.Hostname()
	hyphenatedDomain := ingressv1alpha1.HyphenatedDomainNameFromURL(domain)
	if strings.HasSuffix(domain, ".internal") {
		// Skip creating the Domain CR for ngrok TCP URLs
		setDomainReadyCondition(aep, true, "DomainReady", "Domain is ready")
		return nil
	}

	log := ctrl.LoggerFrom(ctx).WithValues("domain", domain)

	// Check if the Domain CRD already exists
	domainObj := &ingressv1alpha1.Domain{}

	err = r.Get(ctx, client.ObjectKey{Name: hyphenatedDomain, Namespace: aep.Namespace}, domainObj)
	if err == nil {
		// Domain already exists
		if domainObj.Status.ID == "" {
			// Domain is not ready yet
			setDomainReadyCondition(aep, false, ReasonDomainCreating, "Domain is being created")
			setReadyCondition(aep, false, ReasonDomainCreating, "Waiting for domain to be ready")
			return ErrDomainCreating
		}
		setDomainReadyCondition(aep, true, "DomainReady", "Domain is ready")
		return nil
	}
	if client.IgnoreNotFound(err) != nil {
		// Some other error occurred
		log.Error(err, "failed to check Domain CRD existence")
		setDomainReadyCondition(aep, false, ReasonNgrokAPIError, err.Error())
		setReadyCondition(aep, false, ReasonNgrokAPIError, err.Error())
		return err
	}

	// Create the Domain CRD since it doesn't exist
	newDomain := &ingressv1alpha1.Domain{
		ObjectMeta: ctrl.ObjectMeta{
			Name:      hyphenatedDomain,
			Namespace: aep.Namespace,
		},
		Spec: ingressv1alpha1.DomainSpec{
			Domain: domain,
		},
	}

	if r.DefaultDomainReclaimPolicy != nil {
		newDomain.Spec.ReclaimPolicy = *r.DefaultDomainReclaimPolicy
	}

	if err := r.Create(ctx, newDomain); err != nil {
		r.Recorder.Event(aep, v1.EventTypeWarning, "DomainCreationFailed", fmt.Sprintf("Failed to create Domain CRD %s", hyphenatedDomain))
		setDomainReadyCondition(aep, false, ReasonNgrokAPIError, err.Error())
		setReadyCondition(aep, false, ReasonNgrokAPIError, err.Error())
		return err
	}

	r.Recorder.Event(aep, v1.EventTypeNormal, "DomainCreated", fmt.Sprintf("Domain CRD %s created successfully", hyphenatedDomain))
	setDomainReadyCondition(aep, false, ReasonDomainCreating, "Domain is being created")
	setReadyCondition(aep, false, ReasonDomainCreating, "Waiting for domain to be ready")
	return ErrDomainCreating
}

// updateEndpointStatus updates the endpoint status based on creation result from the AgentDriver.
func (r *AgentEndpointReconciler) updateEndpointStatus(endpoint *ngrokv1alpha1.AgentEndpoint, result *agent.EndpointResult, err error, trafficPolicy string) {
	// Set traffic policy status
	if trafficPolicy != "" {
		if endpoint.Spec.TrafficPolicy != nil && endpoint.Spec.TrafficPolicy.Reference != nil {
			endpoint.Status.AttachedTrafficPolicy = endpoint.Spec.TrafficPolicy.Reference.Name
		} else {
			endpoint.Status.AttachedTrafficPolicy = "inline"
		}
	} else {
		endpoint.Status.AttachedTrafficPolicy = "none"
	}

	// Update status based on endpoint creation result
	switch {
	case err != nil || (result != nil && result.Error != nil):
		var errMsg string
		if err != nil {
			errMsg = err.Error()
		} else if result != nil && result.Error != nil {
			errMsg = result.Error.Error()
		}

		errMsg = ngrokapi.SanitizeErrorMessage(errMsg)

		// Check if the error message indicates a traffic policy configuration issue
		reason := ReasonNgrokAPIError
		if trafficPolicy != "" && ngrokapi.IsTrafficPolicyError(errMsg) {
			reason = ReasonTrafficPolicyError
		}

		setEndpointCreatedCondition(endpoint, false, reason, errMsg)
		setReadyCondition(endpoint, false, reason, errMsg)

		if trafficPolicy != "" {
			setTrafficPolicyCondition(endpoint, false, reason, errMsg)
		}
	case result != nil:
		// Success - update status with endpoint information from ngrok
		endpoint.Status.AssignedURL = result.URL

		setEndpointCreatedCondition(endpoint, true, ReasonEndpointCreated, "Endpoint successfully created")
		if trafficPolicy != "" {
			setTrafficPolicyCondition(endpoint, true, "TrafficPolicyApplied", "Traffic policy successfully applied")
		}
		setReadyCondition(endpoint, true, ReasonEndpointActive, "AgentEndpoint is active and ready")
	default:
		// Unexpected case - no result and no error
		setReadyCondition(endpoint, false, ReasonNgrokAPIError, "No endpoint result returned")
	}
}
