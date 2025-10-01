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
	ErrDomainCreating = errors.New("waiting while domain is being created")
)

// EndpointWithDomain represents an endpoint resource that has domain conditions and references
type EndpointWithDomain interface {
	client.Object
	GetConditions() *[]metav1.Condition
	GetGeneration() int64
	GetDomainRef() *ngrokv1alpha1.K8sObjectRefOptionalNamespace
	SetDomainRef(*ngrokv1alpha1.K8sObjectRefOptionalNamespace)
}

// DomainResult contains the result of domain operations
type DomainResult struct {
	Domain       *ingressv1alpha1.Domain
	IsReady      bool
	ReadyReason  string // Reason from domain's Ready condition
	ReadyMessage string // Message from domain's Ready condition
}

// Manager handles domain creation and condition management
type Manager struct {
	Client                     client.Client
	Recorder                   record.EventRecorder
	DefaultDomainReclaimPolicy *ingressv1alpha1.DomainReclaimPolicy
}

// EnsureDomainExists checks if the Domain CRD exists, creates it if needed, and sets conditions/domainRef
func (m *Manager) EnsureDomainExists(ctx context.Context, endpoint EndpointWithDomain, url string) (*DomainResult, error) {
	parsedURL, err := m.parseAndValidateURL(endpoint, url)
	if err != nil {
		return nil, err
	}

	if result := m.checkSkippedDomains(endpoint, parsedURL); result != nil {
		return result, nil
	}

	domain := parsedURL.Hostname()
	return m.getOrCreateDomain(ctx, endpoint, domain)
}

// parseAndValidateURL parses and validates the endpoint URL
func (m *Manager) parseAndValidateURL(endpoint EndpointWithDomain, urlStr string) (*url.URL, error) {
	parsedURL, err := util.ParseAndSanitizeEndpointURL(urlStr, true)
	if err != nil {
		m.setDomainCondition(endpoint, false, ReasonNgrokAPIError, err.Error())
		return nil, fmt.Errorf("failed to parse URL %q: %w", urlStr, err)
	}
	return parsedURL, nil
}

// checkSkippedDomains checks if the domain should be skipped (TCP or internal)
func (m *Manager) checkSkippedDomains(endpoint EndpointWithDomain, parsedURL *url.URL) *DomainResult {
	// Skip TCP ngrok URLs
	if parsedURL.Scheme == "tcp" && strings.HasSuffix(parsedURL.Hostname(), "tcp.ngrok.io") {
		m.setDomainCondition(endpoint, true, "DomainReady", "Domain is ready")
		endpoint.SetDomainRef(nil)
		return &DomainResult{
			IsReady: true,
		}
	}

	// Skip internal domains
	if strings.HasSuffix(parsedURL.Hostname(), ".internal") {
		m.setDomainCondition(endpoint, true, "DomainReady", "Domain is ready")
		endpoint.SetDomainRef(nil)
		return &DomainResult{
			IsReady: true,
		}
	}

	return nil
}

// getOrCreateDomain gets an existing domain or creates a new one
func (m *Manager) getOrCreateDomain(ctx context.Context, endpoint EndpointWithDomain, domain string) (*DomainResult, error) {
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
func (m *Manager) checkExistingDomain(endpoint EndpointWithDomain, domainObj *ingressv1alpha1.Domain) (*DomainResult, error) {
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
	if isReady {
		m.setDomainCondition(endpoint, true, readyReason, readyMessage)
		return &DomainResult{
			Domain:       domainObj,
			IsReady:      true,
			ReadyReason:  readyReason,
			ReadyMessage: readyMessage,
		}, nil
	}

	m.setDomainCondition(endpoint, false, readyReason, readyMessage)
	return &DomainResult{
		Domain:       domainObj,
		IsReady:      false,
		ReadyReason:  readyReason,
		ReadyMessage: readyMessage,
	}, ErrDomainCreating

}

// createNewDomain creates a new Domain CRD
func (m *Manager) createNewDomain(ctx context.Context, endpoint EndpointWithDomain, domain, hyphenatedDomain string) (*DomainResult, error) {
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
	}, ErrDomainCreating
}

// setDomainCondition sets the DomainReady condition on the endpoint
func (m *Manager) setDomainCondition(endpoint EndpointWithDomain, ready bool, reason, message string) {
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
