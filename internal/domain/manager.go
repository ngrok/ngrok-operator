package domain

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"slices"
	"strings"

	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/record"
	"k8s.io/client-go/util/retry"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/go-logr/logr"
	ingressv1alpha1 "github.com/ngrok/ngrok-operator/api/ingress/v1alpha1"
	ngrokv1alpha1 "github.com/ngrok/ngrok-operator/api/ngrok/v1alpha1"
	"github.com/ngrok/ngrok-operator/internal/controller/ingress"
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

var (
	ErrDomainNotReady = errors.New("domain is not ready yet")
)

// DomainResult contains the result of domain operations
type DomainResult struct {
	Domain       *ingressv1alpha1.Domain
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
func WithDefaultDomainReclaimPolicy(policy ingressv1alpha1.DomainReclaimPolicy) ManagerOption {
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
	Recorder                   record.EventRecorder
	defaultDomainReclaimPolicy *ingressv1alpha1.DomainReclaimPolicy
	controllerLabels           *labels.ControllerLabelValues
}

func NewManager(client client.Client, recorder record.EventRecorder, opts ...ManagerOption) (*Manager, error) {
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
	if parsedURL.Scheme == "tcp" && strings.HasSuffix(parsedURL.Hostname(), "tcp.ngrok.io") {
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

// getOrCreateDomain gets an existing domain or creates a new one
func (m *Manager) getOrCreateDomain(ctx context.Context, endpoint ngrokv1alpha1.EndpointWithDomain, domain string) (*DomainResult, error) {
	log := ctrl.LoggerFrom(ctx).WithValues("domain", domain)
	hyphenatedDomain := ingressv1alpha1.HyphenatedDomainNameFromURL(domain)
	domainKey := client.ObjectKey{Name: hyphenatedDomain, Namespace: endpoint.GetNamespace()}

	domainObj := &ingressv1alpha1.Domain{}
	err := m.Client.Get(ctx, domainKey, domainObj)
	if err == nil {
		if err := m.ensureControllerLabels(ctx, log, domainObj); err != nil {
			return nil, err
		}
		return m.checkExistingDomain(endpoint, domainObj)
	}

	if client.IgnoreNotFound(err) != nil {
		log.Error(err, "failed to check Domain CRD existence")
		m.setDomainCondition(endpoint, false, ReasonNgrokAPIError, err.Error())
		return nil, err
	}

	return m.createNewDomain(ctx, endpoint, domain, hyphenatedDomain)
}

func (m *Manager) ensureControllerLabels(ctx context.Context, log logr.Logger, domainObj *ingressv1alpha1.Domain) error {
	if m.controllerLabels == nil {
		return nil
	}

	l := domainObj.GetLabels()
	_, hasControllerNameLabel := l[labels.ControllerName]
	_, hasControllerNamespaceLabel := l[labels.ControllerNamespace]

	if hasControllerNameLabel && hasControllerNamespaceLabel {
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

// checkExistingDomain checks the status of an existing domain
func (m *Manager) checkExistingDomain(endpoint ngrokv1alpha1.EndpointWithDomain, domainObj *ingressv1alpha1.Domain) (*DomainResult, error) {
	domainRef := &ngrokv1alpha1.K8sObjectRefOptionalNamespace{
		Name:      domainObj.Name,
		Namespace: &domainObj.Namespace,
	}
	endpoint.SetDomainRef(domainRef)

	// Get domain's Ready condition to propagate reason/message to endpoint
	readyCondition := meta.FindStatusCondition(domainObj.Status.Conditions, ingress.ConditionDomainReady)
	readyReason := ReasonDomainCreating
	readyMessage := "Domain is being created"
	if readyCondition != nil {
		readyReason = readyCondition.Reason
		readyMessage = readyCondition.Message
	}

	isReady := ingress.IsDomainReady(domainObj)
	m.setDomainCondition(endpoint, isReady, readyReason, readyMessage)

	return &DomainResult{
		Domain:       domainObj,
		IsReady:      isReady,
		ReadyReason:  readyReason,
		ReadyMessage: readyMessage,
	}, nil
}

// createNewDomain creates a new Domain CRD
func (m *Manager) createNewDomain(ctx context.Context, endpoint ngrokv1alpha1.EndpointWithDomain, domain, hyphenatedDomain string) (*DomainResult, error) {
	newDomain := &ingressv1alpha1.Domain{
		ObjectMeta: ctrl.ObjectMeta{
			Name:      hyphenatedDomain,
			Namespace: endpoint.GetNamespace(),
		},
		Spec: ingressv1alpha1.DomainSpec{
			Domain: domain,
		},
	}

	if m.controllerLabels != nil {
		m.controllerLabels.EnsureLabels(newDomain)
	}

	if m.defaultDomainReclaimPolicy != nil {
		newDomain.Spec.ReclaimPolicy = *m.defaultDomainReclaimPolicy
	}

	if err := m.Client.Create(ctx, newDomain); err != nil {
		m.setDomainCondition(endpoint, false, ReasonNgrokAPIError, err.Error())
		return nil, err
	}

	domainRef := &ngrokv1alpha1.K8sObjectRefOptionalNamespace{
		Name:      newDomain.Name,
		Namespace: &newDomain.Namespace,
	}
	endpoint.SetDomainRef(domainRef)
	m.setDomainCondition(endpoint, false, ReasonDomainCreating, "Domain is being created")

	return &DomainResult{
		Domain:       newDomain,
		IsReady:      false,
		ReadyReason:  ReasonDomainCreating,
		ReadyMessage: "Domain is being created",
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

	domain := &ingressv1alpha1.Domain{}
	domain.SetNamespace(endpoint.GetNamespace())
	domain.SetName(domainRef.Name)

	log.Info("Deleting stale domain for binding-based endpoint", "domain", client.ObjectKeyFromObject(domain), "endpoint", client.ObjectKeyFromObject(endpoint))
	return client.IgnoreNotFound(m.Client.Delete(ctx, domain))
}
