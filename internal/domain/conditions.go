package domain

import (
	"strings"

	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	ingressv1alpha1 "github.com/ngrok/ngrok-operator/api/ingress/v1alpha1"
)

const (
	// ConditionDomainReady is the condition type for Domain CRDs indicating the domain is fully ready.
	// Used on: Domain CRD status.conditions
	// Value: "Ready" (standard Kubernetes convention for primary readiness)
	ConditionDomainReady = "Ready"

	// ConditionEndpointDomainReady is the condition type for endpoint resources (AgentEndpoint, CloudEndpoint)
	// indicating their associated domain is ready. This is set by the domain Manager.
	// Used on: AgentEndpoint/CloudEndpoint status.conditions
	// Value: "DomainReady" (prefixed to distinguish from endpoint's own Ready condition)
	ConditionEndpointDomainReady = "DomainReady"

	ConditionDomainCreated    = "DomainCreated"
	ConditionCertificateReady = "CertificateReady"
	ConditionDNSConfigured    = "DNSConfigured"
	ConditionProgressing      = "Progressing"

	// condition reasons for Domain CRDs (used by ingress controller)
	ReasonDomainActive            = "DomainActive"
	ReasonDomainCreated           = "DomainCreated"
	ReasonDomainInvalid           = "DomainInvalid"
	ReasonCertificateProvisioning = "CertificateProvisioning"
	ReasonCertificateReady        = "CertificateReady"
	ReasonDNSError                = "DNSError"
	ReasonACMEChallengeRequired   = "ACMEChallengeRequired"
	ReasonNgrokManaged            = "NgrokManaged"
	ReasonProvisioningError       = "ProvisioningError"
	ReasonWaitingForCertificate   = "WaitingForCertificate"
	ReasonDanglingDNSRecord       = "DanglingDNSRecord"
	ReasonProtectedDomain         = "ProtectedDomain"
	ReasonDomainCreationFailed    = "DomainCreationFailed"
	ReasonProvisioning            = "Provisioning"

	// condition reasons for endpoints (AgentEndpoint, CloudEndpoint) - used by domain Manager
	ReasonDomainReady    = "DomainReady"
	ReasonDomainCreating = "DomainCreating"
	ReasonNgrokAPIError  = "NgrokAPIError"
)

// IsDomainReady checks if a domain is ready by examining both Status.ID and Ready condition
func IsDomainReady(domain *ingressv1alpha1.Domain) bool {
	// First check if domain has an ID (basic requirement)
	if domain.Status.ID == "" {
		return false
	}

	// Then check the Ready condition for more detailed status
	readyCondition := meta.FindStatusCondition(domain.Status.Conditions, ConditionDomainReady)
	if readyCondition == nil {
		// No ready condition set yet, so it's not ready
		return false
	}

	return readyCondition.Status == metav1.ConditionTrue
}

// IsInternalDomain returns true if the given hostname ends with ".internal" TLD.
// Internal domains cannot be reserved via the ngrok API (returns HTTP 400).
// This function handles case-insensitivity and trailing dots.
func IsInternalDomain(host string) bool {
	h := strings.ToLower(strings.TrimSuffix(strings.TrimSpace(host), "."))
	return strings.HasSuffix(h, ".internal")
}
