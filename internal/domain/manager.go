package domain

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"slices"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/events"
	"k8s.io/client-go/util/retry"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/go-logr/logr"
	ingressv1alpha1 "github.com/ngrok/ngrok-operator/api/ingress/v1alpha1"
	ngrokv1 "github.com/ngrok/ngrok-operator/api/ngrok/v1"
	ngrokv1alpha1 "github.com/ngrok/ngrok-operator/api/ngrok/v1alpha1"
	"github.com/ngrok/ngrok-operator/internal/controller/labels"
	"github.com/ngrok/ngrok-operator/internal/util"
)

const (
	ConditionDomainReady = "DomainReady"
)

const (
	ReasonDomainReady    = "DomainReady"
	ReasonDomainCreating = "DomainCreating"
	ReasonNgrokAPIError  = "NgrokAPIError"
)

// EventDomainMigrationBlocked is the Warning event reason recorded on an
// endpoint whose legacy Domain can't be copied to ngrok.com/v1.
//
// LEGACY-domain-kind: delete at cleanup.
const EventDomainMigrationBlocked = "DomainMigrationBlocked"

var (
	ErrDomainNotReady = errors.New("domain is not ready yet")
)

// DomainResult contains the result of domain operations
type DomainResult struct {
	// Domain is the Domain the endpoint is served by (see Pair.Authoritative).
	Domain       Resource
	IsReady      bool
	ReadyReason  string // Reason from domain's Ready condition
	ReadyMessage string // Message from domain's Ready condition
}

// RequeueError returns ErrDomainNotReady if the domain is not ready, otherwise nil
func (r *DomainResult) RequeueError() error {
	if !r.IsReady {
		return ErrDomainNotReady
	}
	return nil
}

// ManagerOption is a functional option for configuring the Domain Manager
type ManagerOption func(*Manager)

// WithDefaultDomainReclaimPolicy sets the default domain reclaim policy for the Domain Manager
func WithDefaultDomainReclaimPolicy(policy ngrokv1.DomainReclaimPolicy) ManagerOption {
	return func(m *Manager) {
		m.defaultDomainReclaimPolicy = &policy
	}
}

// WithControllerLabels sets the controller labels for the Domain Manager
func WithControllerLabels(clv labels.ControllerLabelValues) ManagerOption {
	return func(m *Manager) {
		m.controllerLabels = &clv
	}
}

// Manager handles domain creation and condition management
type Manager struct {
	Client                     client.Client
	Recorder                   events.EventRecorder
	defaultDomainReclaimPolicy *ngrokv1.DomainReclaimPolicy
	controllerLabels           *labels.ControllerLabelValues
}

func NewManager(client client.Client, recorder events.EventRecorder, opts ...ManagerOption) (*Manager, error) {
	m := &Manager{
		Client:   client,
		Recorder: recorder,
	}

	for _, opt := range opts {
		opt(m)
	}

	if m.controllerLabels != nil {
		if err := labels.ValidateControllerLabelValues(*m.controllerLabels); err != nil {
			return nil, err
		}
	}
	return m, nil
}

// EnsureDomainExists checks if the Domain CRD exists, creates it if needed, and sets conditions/domainRef
func (m *Manager) EnsureDomainExists(ctx context.Context, endpoint ngrokv1alpha1.EndpointWithDomain) (*DomainResult, error) {
	parsedURL, err := m.parseAndValidateURL(endpoint)
	if err != nil {
		return nil, err
	}

	if result, err := m.checkSkippedDomains(ctx, endpoint, parsedURL); result != nil || err != nil {
		return result, err
	}

	domain := parsedURL.Hostname()
	return m.getOrCreateDomain(ctx, endpoint, domain)
}

// parseAndValidateURL parses and validates the endpoint URL
func (m *Manager) parseAndValidateURL(endpoint ngrokv1alpha1.EndpointWithDomain) (*url.URL, error) {
	urlStr := endpoint.GetURL()
	parsedURL, err := util.ParseAndSanitizeEndpointURL(urlStr, true)
	if err != nil {
		m.setDomainCondition(endpoint, false, ReasonNgrokAPIError, err.Error())
		return nil, fmt.Errorf("failed to parse URL %q: %w", urlStr, err)
	}
	return parsedURL, nil
}

// checkSkippedDomains checks if the domain should be skipped (TCP, internal, or Kubernetes bindings)
func (m *Manager) checkSkippedDomains(ctx context.Context, endpoint ngrokv1alpha1.EndpointWithDomain, parsedURL *url.URL) (*DomainResult, error) {
	bindings := endpoint.GetBindings()
	// Skip Kubernetes-bound endpoints (no domain reservation needed)
	if slices.Contains(bindings, "kubernetes") {
		msg := "Domain ready (Kubernetes binding - no domain reservation needed)"
		if err := m.deleteStaleBindingDomain(ctx, endpoint); err != nil {
			return nil, err
		}
		m.setDomainCondition(endpoint, true, ReasonDomainReady, msg)
		endpoint.SetDomainRef(nil)
		return &DomainResult{
			IsReady:      true,
			ReadyReason:  ReasonDomainReady,
			ReadyMessage: msg,
		}, nil
	}

	// Skip internal-bound endpoints (no domain reservation needed)
	if slices.Contains(bindings, "internal") {
		msg := "Domain ready (internal binding - no domain reservation needed)"
		m.setDomainCondition(endpoint, true, ReasonDomainReady, msg)
		endpoint.SetDomainRef(nil)
		return &DomainResult{
			IsReady:      true,
			ReadyReason:  ReasonDomainReady,
			ReadyMessage: msg,
		}, nil
	}

	// Skip TCP ngrok URLs
	if parsedURL.Scheme == "tcp" {
		msg := "Domain ready (TCP ngrok URL - no domain reservation needed)"
		m.setDomainCondition(endpoint, true, ReasonDomainReady, msg)
		endpoint.SetDomainRef(nil)
		return &DomainResult{
			IsReady:      true,
			ReadyReason:  ReasonDomainReady,
			ReadyMessage: msg,
		}, nil
	}

	// Skip internal domains
	if util.IsInternalDomain(parsedURL.Hostname()) {
		msg := "Domain ready (internal domain - no domain reservation needed)"
		m.setDomainCondition(endpoint, true, ReasonDomainReady, msg)
		endpoint.SetDomainRef(nil)
		return &DomainResult{
			IsReady:      true,
			ReadyReason:  ReasonDomainReady,
			ReadyMessage: msg,
		}, nil
	}

	return nil, nil
}

// getOrCreateDomain ensures the ngrok.com/v1 Domain for domain exists and
// reports the state of the Domain the endpoint is served by.
func (m *Manager) getOrCreateDomain(ctx context.Context, endpoint ngrokv1alpha1.EndpointWithDomain, domain string) (*DomainResult, error) {
	log := ctrl.LoggerFrom(ctx).WithValues("domain", domain)
	domainKey := client.ObjectKey{Name: ngrokv1.HyphenatedDomainNameFromURL(domain), Namespace: endpoint.GetNamespace()}

	v1, _, err := EnsureV1(ctx, m.Client, domainKey, func(d *ngrokv1.Domain) {
		d.Spec.Domain = domain
		if m.controllerLabels != nil {
			m.controllerLabels.EnsureLabels(d)
		}
		if m.defaultDomainReclaimPolicy != nil {
			d.Spec.ReclaimPolicy = *m.defaultDomainReclaimPolicy
		}
	})
	switch {
	// LEGACY-domain-kind: BEGIN
	case errors.Is(err, ErrMigrationBlocked):
		// Keep serving the endpoint from the legacy Domain; someone has to
		// re-create it as ngrok.com/v1 by hand.
		log.Info("Not migrating legacy Domain to ngrok.com/v1", "reason", err.Error())
		m.Recorder.Eventf(endpoint, nil, corev1.EventTypeWarning, EventDomainMigrationBlocked, "Reconcile", err.Error())
	// LEGACY-domain-kind: END
	case err != nil:
		log.Error(err, "failed to ensure Domain CRD exists")
		m.setDomainCondition(endpoint, false, ReasonNgrokAPIError, err.Error())
		return nil, err
	default:
		// Also covers a v1 just copied from a legacy Domain, which carries the
		// legacy object's labels rather than the defaults.
		if err := m.ensureControllerLabels(ctx, log, v1); err != nil {
			return nil, err
		}
	}

	pair, err := GetPair(ctx, m.Client, domainKey)
	if err != nil {
		m.setDomainCondition(endpoint, false, ReasonNgrokAPIError, err.Error())
		return nil, err
	}
	if v1 != nil {
		// The cache may not have caught up with a Create made a moment ago.
		pair.V1 = v1
	}
	res, err := pair.Authoritative()
	if err != nil {
		m.setDomainCondition(endpoint, false, ReasonNgrokAPIError, err.Error())
		return nil, err
	}
	return m.checkExistingDomain(endpoint, res)
}

func (m *Manager) ensureControllerLabels(ctx context.Context, log logr.Logger, domainObj *ngrokv1.Domain) error {
	if m.controllerLabels == nil {
		return nil
	}

	// Clone-and-probe: EnsureLabels reports whether it would change anything.
	// Running it against a copy tells us whether domainObj already carries every
	// label the operator wants — and, during the migration window, that no
	// legacy-prefix pair is left to strip — without mutating it. The probe
	// tracks whatever EnsureLabels does, so there is no migration-specific code
	// to clean up here.
	if !m.controllerLabels.EnsureLabels(domainObj.DeepCopy()) {
		return nil
	}

	return retry.RetryOnConflict(retry.DefaultBackoff, func() error {
		// Re-fetch the domain to get the latest version
		if err := m.Client.Get(ctx, client.ObjectKeyFromObject(domainObj), domainObj); err != nil {
			return err
		}

		if !m.controllerLabels.EnsureLabels(domainObj) {
			return nil
		}

		log.Info("Adding controller labels to existing Domain CRD", "domain", client.ObjectKeyFromObject(domainObj))
		return m.Client.Update(ctx, domainObj)
	})
}

// checkExistingDomain reports the state of the Domain the endpoint is served by
func (m *Manager) checkExistingDomain(endpoint ngrokv1alpha1.EndpointWithDomain, domainRes Resource) (*DomainResult, error) {
	domainObj := domainRes.Object()
	domainRef := &ngrokv1alpha1.K8sObjectRefOptionalNamespace{
		Name:      domainObj.GetName(),
		Namespace: new(domainObj.GetNamespace()),
	}
	endpoint.SetDomainRef(domainRef)

	// Get domain's Ready condition to propagate reason/message to endpoint
	readyCondition := meta.FindStatusCondition(domainRes.Status().Conditions, ConditionReady)
	readyReason := ReasonDomainCreating
	readyMessage := "Domain is being created"
	if readyCondition != nil {
		readyReason = readyCondition.Reason
		readyMessage = readyCondition.Message
	}

	isReady := IsReady(domainRes)
	m.setDomainCondition(endpoint, isReady, readyReason, readyMessage)

	return &DomainResult{
		Domain:       domainRes,
		IsReady:      isReady,
		ReadyReason:  readyReason,
		ReadyMessage: readyMessage,
	}, nil
}

// setDomainCondition sets the DomainReady condition on the endpoint
func (m *Manager) setDomainCondition(endpoint ngrokv1alpha1.EndpointWithDomain, ready bool, reason, message string) {
	status := metav1.ConditionTrue
	if !ready {
		status = metav1.ConditionFalse
	}

	condition := metav1.Condition{
		Type:               ConditionDomainReady,
		Status:             status,
		Reason:             reason,
		Message:            message,
		ObservedGeneration: endpoint.GetGeneration(),
	}

	meta.SetStatusCondition(endpoint.GetConditions(), condition)
}

// deleteStaleBindingDomain deletes a domain if it exists for an endpoint that now has kubernetes or internal bindings.
// This cleans up domains that were created before bindings were added to the endpoint.
func (m *Manager) deleteStaleBindingDomain(ctx context.Context, endpoint ngrokv1alpha1.EndpointWithDomain) error {
	log := ctrl.LoggerFrom(ctx)

	domainRef := endpoint.GetDomainRef()
	if domainRef == nil {
		return nil
	}

	domains := []client.Object{
		&ngrokv1.Domain{},
		&ingressv1alpha1.Domain{}, // LEGACY-domain-kind: delete at cleanup.
	}
	for _, domain := range domains {
		domain.SetNamespace(endpoint.GetNamespace())
		domain.SetName(domainRef.Name)

		log.Info("Deleting stale domain for binding-based endpoint", "domain", client.ObjectKeyFromObject(domain), "endpoint", client.ObjectKeyFromObject(endpoint))
		if err := client.IgnoreNotFound(m.Client.Delete(ctx, domain)); err != nil {
			return err
		}
	}
	return nil
}
