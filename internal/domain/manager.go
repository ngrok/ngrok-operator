package domain

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strings"

	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	ingressv1alpha1 "github.com/ngrok/ngrok-operator/api/ingress/v1alpha1"
	ngrokv1alpha1 "github.com/ngrok/ngrok-operator/api/ngrok/v1alpha1"
	"github.com/ngrok/ngrok-operator/internal/controller/ingress"
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

// DomainCheckParams contains parameters for domain checks
type DomainCheckParams struct {
	URL      string
	Bindings []string
}

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

// Manager handles domain creation and condition management
type Manager struct {
	Client                     client.Client
	Recorder                   record.EventRecorder
	DefaultDomainReclaimPolicy *ingressv1alpha1.DomainReclaimPolicy
}

// EnsureDomainExists checks if the Domain CRD exists, creates it if needed, and sets conditions/domainRef
func (m *Manager) EnsureDomainExists(ctx context.Context, endpoint ngrokv1alpha1.EndpointWithDomain, params DomainCheckParams) (*DomainResult, error) {
	parsedURL, err := m.parseAndValidateURL(endpoint, params.URL)
	if err != nil {
		return nil, err
	}

	if result := m.checkSkippedDomains(ctx, endpoint, parsedURL, params.Bindings); result != nil {
		return result, nil
	}

	domain := parsedURL.Hostname()
	return m.getOrCreateDomain(ctx, endpoint, domain)
}

// parseAndValidateURL parses and validates the endpoint URL
func (m *Manager) parseAndValidateURL(endpoint ngrokv1alpha1.EndpointWithDomain, urlStr string) (*url.URL, error) {
	parsedURL, err := util.ParseAndSanitizeEndpointURL(urlStr, true)
	if err != nil {
		m.setDomainCondition(endpoint, false, ReasonNgrokAPIError, err.Error())
		return nil, fmt.Errorf("failed to parse URL %q: %w", urlStr, err)
	}
	return parsedURL, nil
}

// checkSkippedDomains checks if the domain should be skipped (TCP, internal, or Kubernetes bindings)
func (m *Manager) checkSkippedDomains(ctx context.Context, endpoint ngrokv1alpha1.EndpointWithDomain, parsedURL *url.URL, bindings []string) *DomainResult {
	// Skip Kubernetes-bound endpoints (no domain reservation needed)
	if hasKubernetesBinding(bindings) {
		msg := "Domain ready (Kubernetes binding - no domain reservation needed)"
		m.deleteStaleBindingDomain(ctx, endpoint)
		m.setDomainCondition(endpoint, true, ReasonDomainReady, msg)
		endpoint.SetDomainRef(nil)
		return &DomainResult{
			IsReady:      true,
			ReadyReason:  ReasonDomainReady,
			ReadyMessage: msg,
		}
	}

	// Skip internal-bound endpoints (no domain reservation needed)
	if hasInternalBinding(bindings) {
		msg := "Domain ready (internal binding - no domain reservation needed)"
		m.setDomainCondition(endpoint, true, ReasonDomainReady, msg)
		endpoint.SetDomainRef(nil)
		return &DomainResult{
			IsReady:      true,
			ReadyReason:  ReasonDomainReady,
			ReadyMessage: msg,
		}
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
		}
	}

	// Skip internal domains
	if strings.HasSuffix(parsedURL.Hostname(), ".internal") {
		msg := "Domain ready (internal domain - no domain reservation needed)"
		m.setDomainCondition(endpoint, true, ReasonDomainReady, msg)
		endpoint.SetDomainRef(nil)
		return &DomainResult{
			IsReady:      true,
			ReadyReason:  ReasonDomainReady,
			ReadyMessage: msg,
		}
	}

	return nil
}

// getOrCreateDomain gets an existing domain or creates a new one
func (m *Manager) getOrCreateDomain(ctx context.Context, endpoint ngrokv1alpha1.EndpointWithDomain, domain string) (*DomainResult, error) {
	log := ctrl.LoggerFrom(ctx).WithValues("domain", domain)
	hyphenatedDomain := ingressv1alpha1.HyphenatedDomainNameFromURL(domain)

	domainObj := &ingressv1alpha1.Domain{}
	err := m.Client.Get(ctx, client.ObjectKey{Name: hyphenatedDomain, Namespace: endpoint.GetNamespace()}, domainObj)
	if err == nil {
		return m.checkExistingDomain(endpoint, domainObj)
	}

	if client.IgnoreNotFound(err) != nil {
		log.Error(err, "failed to check Domain CRD existence")
		m.setDomainCondition(endpoint, false, ReasonNgrokAPIError, err.Error())
		return nil, err
	}

	return m.createNewDomain(ctx, endpoint, domain, hyphenatedDomain)
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

	if m.DefaultDomainReclaimPolicy != nil {
		newDomain.Spec.ReclaimPolicy = *m.DefaultDomainReclaimPolicy
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

// EndpointReferencesDomain checks if an endpoint has a domain reference that matches the given domain.
// This checks both the domain name and namespace to determine if they match.
func EndpointReferencesDomain(endpoint ngrokv1alpha1.EndpointWithDomain, domain *ingressv1alpha1.Domain) bool {
	domainRef := endpoint.GetDomainRef()
	if domainRef == nil {
		return false
	}

	if domainRef.Name != domain.Name {
		return false
	}

	// Check namespace match (nil or empty means same namespace)
	if domainRef.Namespace != nil && *domainRef.Namespace != "" && *domainRef.Namespace != domain.Namespace {
		return false
	}

	return true
}

// hasKubernetesBinding checks if the bindings list contains a Kubernetes binding
func hasKubernetesBinding(bindings []string) bool {
	for _, binding := range bindings {
		if binding == "kubernetes" {
			return true
		}
	}
	return false
}

// hasInternalBinding checks if the bindings list contains an internal binding
func hasInternalBinding(bindings []string) bool {
	for _, binding := range bindings {
		if binding == "internal" {
			return true
		}
	}
	return false
}

// deleteStaleBindingDomain deletes a domain if it exists for an endpoint that now has kubernetes or internal bindings.
// This cleans up domains that were created before bindings were added to the endpoint.
func (m *Manager) deleteStaleBindingDomain(ctx context.Context, endpoint ngrokv1alpha1.EndpointWithDomain) {
	log := ctrl.LoggerFrom(ctx)

	domainRef := endpoint.GetDomainRef()
	if domainRef == nil {
		// No domain ref, nothing to clean up
		return
	}

	// Get the domain to delete
	domain := &ingressv1alpha1.Domain{}
	domainKey := domainRef.ToClientObjectKey(endpoint.GetNamespace())

	if err := m.Client.Get(ctx, domainKey, domain); err != nil {
		if client.IgnoreNotFound(err) != nil {
			log.Error(err, "Failed to get domain for cleanup", "domain", domainKey)
		}
		// Domain doesn't exist or error getting it, nothing to clean up
		return
	}

	// Delete the domain
	log.Info("Deleting stale domain for binding-based endpoint", "domain", domainKey, "endpoint", client.ObjectKeyFromObject(endpoint))
	if err := m.Client.Delete(ctx, domain); err != nil {
		if client.IgnoreNotFound(err) != nil {
			log.Error(err, "Failed to delete stale domain", "domain", domainKey)
		}
	}
}
