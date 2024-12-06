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
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/go-logr/logr"
	ingressv1alpha1 "github.com/ngrok/ngrok-operator/api/ingress/v1alpha1"
	ngrokv1alpha1 "github.com/ngrok/ngrok-operator/api/ngrok/v1alpha1"
	"github.com/ngrok/ngrok-operator/internal/controller"
	"github.com/ngrok/ngrok-operator/pkg/tunneldriver"
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
	trafficPolicyNameIndex = "spec.trafficPolicy.targetRef.name"
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

// AgentEndpointReconciler reconciles an AgentEndpoint object
type AgentEndpointReconciler struct {
	client.Client

	Log          logr.Logger
	Scheme       *runtime.Scheme
	Recorder     record.EventRecorder
	TunnelDriver *tunneldriver.TunnelDriver

	controller *controller.BaseController[*ngrokv1alpha1.AgentEndpoint]
}

// SetupWithManager sets up the controller with the Manager
func (r *AgentEndpointReconciler) SetupWithManager(mgr ctrl.Manager) error {
	if r.TunnelDriver == nil {
		return fmt.Errorf("TunnelDriver is nil")
	}

	r.controller = &controller.BaseController[*ngrokv1alpha1.AgentEndpoint]{
		Kube:     r.Client,
		Log:      r.Log,
		Recorder: r.Recorder,
		Update:   r.update,
		Delete:   r.delete,
		StatusID: r.statusID,
		ErrResult: func(op controller.BaseControllerOp, cr *ngrokv1alpha1.AgentEndpoint, err error) (ctrl.Result, error) {
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

	return ctrl.NewControllerManagedBy(mgr).
		For(&ngrokv1alpha1.AgentEndpoint{}).
		Watches(
			&ngrokv1alpha1.NgrokTrafficPolicy{},
			r.controller.NewEnqueueRequestForMapFunc(r.findAgentEndpointForTrafficPolicy),
			// Don't process delete events as it will just fail to look it up.
			// Instead rely on the user to either delete the AgentEndpoint CR or update it with a new TrafficPolicy name
			builder.WithPredicates(&predicate.Funcs{
				DeleteFunc: func(e event.DeleteEvent) bool {
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
	err := r.ensureDomainExists(ctx, endpoint)
	if err != nil {
		return err
	}

	trafficPolicy, err := r.getTrafficPolicy(ctx, endpoint)
	if err != nil {
		return err
	}
	tunnelName := r.statusID(endpoint)
	return r.TunnelDriver.CreateAgentEndpoint(ctx, tunnelName, endpoint.Spec, trafficPolicy)
}

func (r *AgentEndpointReconciler) delete(ctx context.Context, endpoint *ngrokv1alpha1.AgentEndpoint) error {
	tunnelName := r.statusID(endpoint)
	return r.TunnelDriver.DeleteAgentEndpoint(ctx, tunnelName)
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

// getTrafficPolicy returns the TrafficPolicy JSON string from either the name reference or inline policy
func (r *AgentEndpointReconciler) getTrafficPolicy(ctx context.Context, aep *ngrokv1alpha1.AgentEndpoint) (string, error) {
	if aep.Spec.TrafficPolicy == nil {
		return "", nil // No traffic policy to fetch, no error
	}

	// Ensure mutually exclusive fields are not both set
	if aep.Spec.TrafficPolicy.Reference != nil && aep.Spec.TrafficPolicy.Inline != nil {
		return "", ErrInvalidTrafficPolicyConfig
	}

	var policy string
	var err error

	switch aep.Spec.TrafficPolicy.Type() {
	case ngrokv1alpha1.TrafficPolicyCfgType_Inline:
		policyBytes, err := aep.Spec.TrafficPolicy.Inline.MarshalJSON()
		if err != nil {
			return "", fmt.Errorf("failed to marshal inline TrafficPolicy: %w", err)
		}
		policy = string(policyBytes)
	case ngrokv1alpha1.TrafficPolicyCfgType_K8sRef:
		// Right now, we only support traffic policies that are in the same namespace as the agent endpoint
		policy, err = r.findTrafficPolicyByName(ctx, aep.Spec.TrafficPolicy.Reference.Name, aep.Namespace)
		if err != nil {
			return "", err
		}
	}

	return policy, nil
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
func (r *AgentEndpointReconciler) ensureDomainExists(ctx context.Context, aep *ngrokv1alpha1.AgentEndpoint) error {
	parsedURL, err := tunneldriver.ParseAndSanitizeEndpointURL(aep.Spec.URL, true)
	if err != nil {
		r.Recorder.Event(aep, v1.EventTypeWarning, "InvalidURL", fmt.Sprintf("Failed to parse URL: %s", aep.Spec.URL))
		return fmt.Errorf("failed to parse URL %q from AgentEndpoint \"%s.%s\"", aep.Spec.URL, aep.Name, aep.Namespace)
	}

	// TODO: generate a domain for blank strings
	domain := parsedURL.Hostname()
	hyphenatedDomain := ingressv1alpha1.HyphenatedDomainNameFromURL(domain)
	if strings.HasSuffix(domain, ".internal") {
		// Skip creating the Domain CRD for reserved TLDs
		return nil
	}

	log := ctrl.LoggerFrom(ctx).WithValues("domain", domain)

	// Check if the Domain CRD already exists
	domainObj := &ingressv1alpha1.Domain{}
	if err := r.Get(ctx, client.ObjectKey{Name: hyphenatedDomain, Namespace: aep.Namespace}, domainObj); err != nil {
		// Domain already exists
		// TODO: might be out of date and need to be updated
		return nil
	}
	if client.IgnoreNotFound(err) != nil {
		// Some other error occurred
		log.Error(err, "failed to check Domain CRD existence")
		return err
	}

	// Create the Domain CRD
	newDomain := &ingressv1alpha1.Domain{
		ObjectMeta: ctrl.ObjectMeta{
			Name:      hyphenatedDomain,
			Namespace: aep.Namespace,
		},
		Spec: ingressv1alpha1.DomainSpec{
			Domain: domain,
		},
	}
	if err := r.Create(ctx, newDomain); err != nil {
		r.Recorder.Event(aep, v1.EventTypeWarning, "DomainCreationFailed", fmt.Sprintf("Failed to create Domain CRD %s", hyphenatedDomain))
		return err
	}

	r.Recorder.Event(aep, v1.EventTypeNormal, "DomainCreated", fmt.Sprintf("Domain CRD %s created successfully", hyphenatedDomain))
	return ErrDomainCreating
}
