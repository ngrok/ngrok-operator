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
	"crypto/x509"
	"errors"
	"fmt"
	"time"

	"github.com/go-logr/logr"
	ingressv1alpha1 "github.com/ngrok/ngrok-operator/api/ingress/v1alpha1"
	ngrokv1alpha1 "github.com/ngrok/ngrok-operator/api/ngrok/v1alpha1"
	"github.com/ngrok/ngrok-operator/internal/controller"
	"github.com/ngrok/ngrok-operator/internal/controller/labels"
	domainpkg "github.com/ngrok/ngrok-operator/internal/domain"
	"github.com/ngrok/ngrok-operator/internal/ngrokapi"
	trafficpolicypkg "github.com/ngrok/ngrok-operator/internal/trafficpolicy"
	"github.com/ngrok/ngrok-operator/internal/util"
	"github.com/ngrok/ngrok-operator/pkg/agent"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/events"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
)

const (
	clientCertificateRefsIndex = "spec.clientCertificateRefs"
	tlsTerminationSecretsIndex = "spec.tlsTermination.secrets"
)

// indexClientCertificateRefs extracts client certificate reference keys for indexing
func indexClientCertificateRefs(o client.Object) []string {
	aep, ok := o.(*ngrokv1alpha1.AgentEndpoint)
	if !ok {
		return nil
	}
	var keys []string
	for _, ref := range aep.Spec.ClientCertificateRefs {
		keys = append(keys, secretIndexKey(aep.Namespace, ref))
	}
	return keys
}

// indexTLSTerminationSecrets extracts Secret reference keys used by spec.tlsTermination
// (server certificate and optional mTLS client-CA bundle) for indexing. These
// refs are same-namespace only, so they always resolve against aep.Namespace.
func indexTLSTerminationSecrets(o client.Object) []string {
	aep, ok := o.(*ngrokv1alpha1.AgentEndpoint)
	if !ok || aep.Spec.TLSTermination == nil {
		return nil
	}
	keys := []string{aep.Namespace + "/" + aep.Spec.TLSTermination.ServerCertificateRef.Name}
	if aep.Spec.TLSTermination.MutualTLS != nil {
		keys = append(keys, aep.Namespace+"/"+aep.Spec.TLSTermination.MutualTLS.ClientCAsRef.Name)
	}
	return keys
}

// secretIndexKey returns the "namespace/name" key used by Secret-watch indexes,
// resolving the ref's namespace (defaulting to the owning AgentEndpoint's namespace).
func secretIndexKey(defaultNamespace string, ref ngrokv1alpha1.K8sObjectRefOptionalNamespace) string {
	ns := defaultNamespace
	if ref.Namespace != nil && *ref.Namespace != "" {
		ns = *ref.Namespace
	}
	return ns + "/" + ref.Name
}

// AgentEndpointReconciler reconciles an AgentEndpoint object
type AgentEndpointReconciler struct {
	client.Client

	Log         logr.Logger
	Scheme      *runtime.Scheme
	Recorder    events.EventRecorder
	AgentDriver agent.Driver

	controller *controller.BaseController[*ngrokv1alpha1.AgentEndpoint]

	ControllerLabels           labels.ControllerLabelValues
	DefaultDomainReclaimPolicy *ingressv1alpha1.DomainReclaimPolicy
	DomainManager              *domainpkg.Manager
	TrafficPolicyManager       *trafficpolicypkg.Manager

	// DrainState is used to check if the operator is draining.
	// If draining, non-delete reconciles are skipped to prevent new finalizers.
	DrainState controller.DrainState
}

// SetupWithManager sets up the controller with the Manager

func (r *AgentEndpointReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return r.SetupWithManagerNamed(mgr, "agentendpoint")
}

// SetupWithManagerNamed sets up the controller with the Manager using a custom controller name.
// This is useful for tests that need to run multiple controllers.
func (r *AgentEndpointReconciler) SetupWithManagerNamed(mgr ctrl.Manager, controllerName string) error {
	if r.AgentDriver == nil {
		return errors.New("AgentDriver is nil")
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

	r.controller = &controller.BaseController[*ngrokv1alpha1.AgentEndpoint]{
		Kube:       r.Client,
		Log:        r.Log,
		Recorder:   r.Recorder,
		DrainState: r.DrainState,
		Update:     r.update,
		Delete:     r.delete,
		StatusID:   r.statusID,
		ErrResult: func(_ controller.BaseControllerOp, cr *ngrokv1alpha1.AgentEndpoint, err error) (ctrl.Result, error) {
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

	if err := mgr.GetFieldIndexer().IndexField(context.Background(), &ngrokv1alpha1.AgentEndpoint{}, trafficpolicypkg.RefIndex, trafficpolicypkg.IndexKeyForObject); err != nil {
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

	if err := mgr.GetFieldIndexer().IndexField(
		context.Background(),
		&ngrokv1alpha1.AgentEndpoint{},
		tlsTerminationSecretsIndex,
		indexTLSTerminationSecrets,
	); err != nil {
		return err
	}

	return ctrl.NewControllerManagedBy(mgr).
		Named(controllerName).
		For(&ngrokv1alpha1.AgentEndpoint{}, builder.WithPredicates(
			predicate.Or(
				predicate.AnnotationChangedPredicate{},
				predicate.GenerationChangedPredicate{},
			),
		)).
		Watches(
			&ngrokv1alpha1.NgrokTrafficPolicy{},
			r.controller.NewEnqueueRequestForMapFunc(r.findAgentEndpointForTrafficPolicy),
		).
		Watches(
			&v1.Secret{},
			r.controller.NewEnqueueRequestForMapFunc(r.findAgentEndpointForSecret),
		).
		Watches(
			&ingressv1alpha1.Domain{},
			r.controller.NewEnqueueRequestForMapFunc(r.findAgentEndpointsForDomain),
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

	// EnsureDomainExists checks if the domain exists, creates it if needed, and sets conditions/domainRef
	domainResult, err := r.DomainManager.EnsureDomainExists(ctx, endpoint)
	if err != nil {
		return r.updateStatus(ctx, endpoint, nil, domainResult, err)
	}

	// Populate status.trafficPolicy from the spec up front so a failure path
	// reflects what the user is trying to attach rather than a stale success
	// value from a prior generation. Resolve manages the condition's False
	// path; we MarkApplied below only after the downstream create succeeds.
	endpoint.Status.AttachedTrafficPolicy = trafficpolicypkg.IntendedSource(endpoint.Namespace, endpoint.Spec.TrafficPolicy)
	tpResult, err := r.TrafficPolicyManager.Resolve(ctx, endpoint)
	if err != nil {
		return r.updateStatus(ctx, endpoint, nil, domainResult, err)
	}
	endpoint.Status.AttachedTrafficPolicy = tpResult.Source

	clientCerts, err := r.getClientCerts(ctx, endpoint)
	if err != nil {
		setEndpointCreatedCondition(endpoint, false, ReasonConfigError, fmt.Sprintf("Failed to get client certificates: %v", err))
		return r.updateStatus(ctx, endpoint, nil, domainResult, err)
	}

	agentTLS, err := r.getAgentTLSTermination(ctx, endpoint)
	if err != nil {
		setEndpointCreatedCondition(endpoint, false, ReasonConfigError, fmt.Sprintf("Failed to get TLS termination config: %v", err))
		return r.updateStatus(ctx, endpoint, nil, domainResult, err)
	}

	// Create the endpoint
	tunnelName := r.statusID(endpoint)
	result, err := r.AgentDriver.CreateAgentEndpoint(ctx, tunnelName, endpoint.Spec, tpResult.Policy, clientCerts, agentTLS)
	if err != nil {
		// Mark the endpoint as failed creation
		setEndpointCreatedCondition(endpoint, false, ReasonNgrokAPIError, fmt.Sprintf("Failed to create endpoint: %v", err))
		// If error indicates traffic policy issue, surface it via the shared condition too.
		if tpResult.Policy != "" && ngrokapi.IsTrafficPolicyError(err.Error()) {
			r.TrafficPolicyManager.SetError(endpoint, ngrokapi.SanitizeErrorMessage(err.Error()))
		}
		return r.updateStatus(ctx, endpoint, nil, domainResult, err)
	}

	// Downstream create succeeded — record the policy as applied (true)
	// and mark the endpoint as created.
	r.TrafficPolicyManager.MarkApplied(endpoint)
	setEndpointCreatedCondition(endpoint, true, ReasonEndpointCreated, "Endpoint successfully created")

	return r.updateStatus(ctx, endpoint, result, domainResult, nil)
}

func (r *AgentEndpointReconciler) delete(ctx context.Context, endpoint *ngrokv1alpha1.AgentEndpoint) error {
	tunnelName := r.statusID(endpoint)
	return r.AgentDriver.DeleteAgentEndpoint(ctx, tunnelName)
	// TODO: Delete any associated domain
}

func (r *AgentEndpointReconciler) statusID(endpoint *ngrokv1alpha1.AgentEndpoint) string {
	return fmt.Sprintf("%s/%s", endpoint.Namespace, endpoint.Name)
}

// findAgentEndpointForTrafficPolicy searches for any AgentEndpoint CRs that
// reference a particular TrafficPolicy, including cross-namespace references.
func (r *AgentEndpointReconciler) findAgentEndpointForTrafficPolicy(ctx context.Context, o client.Object) []ctrl.Request {
	tp, ok := o.(*ngrokv1alpha1.NgrokTrafficPolicy)
	if !ok {
		return nil
	}

	// Use the shared composite-key index to find AgentEndpoints that reference
	// this TrafficPolicy across namespaces.
	var agentEndpointList ngrokv1alpha1.AgentEndpointList
	if err := r.Client.List(ctx, &agentEndpointList,
		client.MatchingFields{trafficpolicypkg.RefIndex: trafficpolicypkg.LookupKey(tp)}); err != nil {
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

	// An AgentEndpoint can reference a Secret via either ClientCertificateRefs
	// (upstream client certs) or TLSTermination (agent-side server cert / mTLS CAs),
	// each backed by a separate field index. Query both, then dedupe by NamespacedName.
	seen := map[client.ObjectKey]struct{}{}
	var requests []ctrl.Request

	for _, indexName := range []string{clientCertificateRefsIndex, tlsTerminationSecretsIndex} {
		var agentEndpointList ngrokv1alpha1.AgentEndpointList
		if err := r.Client.List(ctx, &agentEndpointList,
			client.MatchingFields{indexName: secretKey},
		); err != nil {
			r.Log.Error(err, "failed to list AgentEndpoints using index", "index", indexName)
			continue
		}
		for _, aep := range agentEndpointList.Items {
			key := client.ObjectKey{Name: aep.Name, Namespace: aep.Namespace}
			if _, dup := seen[key]; dup {
				continue
			}
			seen[key] = struct{}{}
			requests = append(requests, ctrl.Request{NamespacedName: key})
		}
	}

	return requests
}

// findAgentEndpointsForDomain searches for any AgentEndpoint CRs that reference a particular Domain
func (r *AgentEndpointReconciler) findAgentEndpointsForDomain(ctx context.Context, o client.Object) []ctrl.Request {
	domain, ok := o.(*ingressv1alpha1.Domain)
	if !ok {
		return nil
	}

	var endpoints ngrokv1alpha1.AgentEndpointList
	if err := r.Client.List(ctx, &endpoints, client.InNamespace(domain.Namespace)); err != nil {
		return nil
	}

	// Get the domain name from the Domain CR
	domainName := domain.Spec.Domain
	hyphenatedDomain := ingressv1alpha1.HyphenatedDomainNameFromURL(domainName)

	var requests []ctrl.Request
	// First match by domainRef
	for _, ep := range endpoints.Items {
		if ep.GetDomainRef().Matches(domain) {
			requests = append(requests, ctrl.Request{
				NamespacedName: client.ObjectKeyFromObject(&ep),
			})
			continue
		}

		// ALSO match by URL - critical for catching domains created by old pods during rolling updates
		// When old pod creates domain, domainRef might not be set yet in the cached view
		if ep.Spec.URL != "" {
			parsedURL, err := util.ParseAndSanitizeEndpointURL(ep.Spec.URL, true)
			if err == nil && ingressv1alpha1.HyphenatedDomainNameFromURL(parsedURL.Hostname()) == hyphenatedDomain {
				requests = append(requests, ctrl.Request{
					NamespacedName: client.ObjectKeyFromObject(&ep),
				})
			}
		}
	}
	return requests
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
			r.Recorder.Eventf(aep, nil, v1.EventTypeWarning, "SecretNotFound", "Reconcile", fmt.Sprintf("Failed to find Secret %s/%s for clientCertificateRef: %v", key.Namespace, key.Name, err))
			return nil, err
		}

		certData, exists := certSecret.Data["tls.crt"]
		if !exists {
			return nil, fmt.Errorf("tls.crt data is missing from AgentEndpoint clientCertRef %q", fmt.Sprintf("%s.%s", key.Name, key.Namespace))
		}
		keyData, exists := certSecret.Data["tls.key"]
		if !exists {
			return nil, fmt.Errorf("tls.key data is missing from AgentEndpoint clientCertRef %q", fmt.Sprintf("%s.%s", key.Name, key.Namespace))
		}

		cert, err := tls.X509KeyPair(certData, keyData)
		if err != nil {
			return nil, fmt.Errorf("failed to parse TLS certificate AgentEndpoint clientCertRef %q: %w", fmt.Sprintf("%s.%s", key.Name, key.Namespace), err)
		}

		ret = append(ret, cert)
	}
	return ret, nil
}

// getAgentTLSTermination resolves spec.tlsTermination into the driver-facing
// AgentTLSTermination struct. Returns nil when tlsTermination is not configured.
// Caller is responsible for surfacing errors via ReasonConfigError.
func (r *AgentEndpointReconciler) getAgentTLSTermination(ctx context.Context, aep *ngrokv1alpha1.AgentEndpoint) (*agent.AgentTLSTermination, error) {
	if aep.Spec.TLSTermination == nil {
		return nil, nil
	}

	serverCert, err := r.getServerCert(ctx, aep)
	if err != nil {
		return nil, err
	}

	out := &agent.AgentTLSTermination{ServerCert: serverCert}

	if aep.Spec.TLSTermination.MutualTLS != nil {
		clientCAs, err := r.getClientCAs(ctx, aep)
		if err != nil {
			return nil, err
		}
		out.ClientCAs = clientCAs
		out.ClientAuth = clientAuthForMode(ctx, aep.Spec.TLSTermination.MutualTLS.Mode)
	}

	return out, nil
}

// getServerCert fetches the agent's server certificate (tls.crt + tls.key) for
// TLS termination from the referenced kubernetes.io/tls Secret.
func (r *AgentEndpointReconciler) getServerCert(ctx context.Context, aep *ngrokv1alpha1.AgentEndpoint) (*tls.Certificate, error) {
	ref := aep.Spec.TLSTermination.ServerCertificateRef
	key := client.ObjectKey{Name: ref.Name, Namespace: aep.Namespace}

	secret := &v1.Secret{}
	if err := r.Client.Get(ctx, key, secret); err != nil {
		r.Recorder.Eventf(aep, nil, v1.EventTypeWarning, "SecretNotFound", "Reconcile", fmt.Sprintf("Failed to find Secret %s/%s for tlsTermination.serverCertificateRef: %v", key.Namespace, key.Name, err))
		return nil, err
	}

	certData, ok := secret.Data["tls.crt"]
	if !ok {
		return nil, fmt.Errorf("tls.crt data is missing from AgentEndpoint tlsTermination.serverCertificateRef %q", fmt.Sprintf("%s.%s", key.Name, key.Namespace))
	}
	keyData, ok := secret.Data["tls.key"]
	if !ok {
		return nil, fmt.Errorf("tls.key data is missing from AgentEndpoint tlsTermination.serverCertificateRef %q", fmt.Sprintf("%s.%s", key.Name, key.Namespace))
	}

	cert, err := tls.X509KeyPair(certData, keyData)
	if err != nil {
		return nil, fmt.Errorf("failed to parse TLS certificate AgentEndpoint tlsTermination.serverCertificateRef %q: %w", fmt.Sprintf("%s.%s", key.Name, key.Namespace), err)
	}
	return &cert, nil
}

// getClientCAs builds the mTLS client-CA pool from the referenced Secret's ca.crt key.
func (r *AgentEndpointReconciler) getClientCAs(ctx context.Context, aep *ngrokv1alpha1.AgentEndpoint) (*x509.CertPool, error) {
	ref := aep.Spec.TLSTermination.MutualTLS.ClientCAsRef
	key := client.ObjectKey{Name: ref.Name, Namespace: aep.Namespace}

	secret := &v1.Secret{}
	if err := r.Client.Get(ctx, key, secret); err != nil {
		r.Recorder.Eventf(aep, nil, v1.EventTypeWarning, "SecretNotFound", "Reconcile", fmt.Sprintf("Failed to find Secret %s/%s for tlsTermination.mutualTLS.clientCAsRef: %v", key.Namespace, key.Name, err))
		return nil, err
	}

	caData, ok := secret.Data["ca.crt"]
	if !ok {
		return nil, fmt.Errorf("ca.crt data is missing from AgentEndpoint tlsTermination.mutualTLS.clientCAsRef %q", fmt.Sprintf("%s.%s", key.Name, key.Namespace))
	}

	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(caData) {
		return nil, fmt.Errorf("no PEM-encoded certificates found in AgentEndpoint tlsTermination.mutualTLS.clientCAsRef %q ca.crt", fmt.Sprintf("%s.%s", key.Name, key.Namespace))
	}
	return pool, nil
}

// clientAuthForMode maps the CRD mTLS mode to tls.ClientAuthType. Defaults to
// RequireAndVerifyClientCert when the mode is empty (matches the CRD default).
func clientAuthForMode(ctx context.Context, mode ngrokv1alpha1.EndpointMutualTLSMode) tls.ClientAuthType {
	switch mode {
	case ngrokv1alpha1.EndpointMutualTLSModeRequest:
		return tls.VerifyClientCertIfGiven
	case ngrokv1alpha1.EndpointMutualTLSModeRequire, "":
		return tls.RequireAndVerifyClientCert
	default:
		ctrl.LoggerFrom(ctx).Info("unknown mutualTLS mode, defaulting to RequireAndVerifyClientCert", "mode", mode)
		return tls.RequireAndVerifyClientCert
	}
}

// updateStatus updates the endpoint status fields, calculates Ready condition, and writes to k8s API.
// The TrafficPolicyApplied condition is maintained by the trafficpolicy.Manager
// during Resolve; the caller writes status.trafficPolicy from Result.Source.
func (r *AgentEndpointReconciler) updateStatus(ctx context.Context, endpoint *ngrokv1alpha1.AgentEndpoint, result *agent.EndpointResult, domainResult *domainpkg.DomainResult, statusErr error) error {
	// Update status fields if we have a result
	if result != nil {
		endpoint.Status.AssignedURL = result.URL
	}

	// Calculate overall Ready condition based on other conditions and domain status
	calculateAgentEndpointReadyCondition(endpoint, domainResult)

	// Write status to k8s API
	if err := r.controller.ReconcileStatus(ctx, endpoint, statusErr); err != nil {
		return err
	}

	// Requeue if domain is not ready (fallback to watch for convergence)
	if domainResult != nil {
		return domainResult.RequeueError()
	}
	return nil
}
