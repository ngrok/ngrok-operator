package ingress

import (
	"strings"

	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/ngrok/ngrok-api-go/v7"
	ingressv1alpha1 "github.com/ngrok/ngrok-operator/api/ingress/v1alpha1"
)

// Standard condition types for Domain
const (
	ConditionDomainReady      = "Ready"
	ConditionDomainCreated    = "DomainCreated"
	ConditionCertificateReady = "CertificateReady"
	ConditionDNSConfigured    = "DNSConfigured"
)

// Standard condition reasons for Domain
const (
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
)

// setReadyCondition sets the Ready condition based on the overall domain state
func setDomainReadyCondition(domain *ingressv1alpha1.Domain, ready bool, reason, message string) {
	status := metav1.ConditionTrue
	if !ready {
		status = metav1.ConditionFalse
	}

	condition := metav1.Condition{
		Type:               ConditionDomainReady,
		Status:             status,
		Reason:             reason,
		Message:            message,
		ObservedGeneration: domain.Generation,
	}

	meta.SetStatusCondition(&domain.Status.Conditions, condition)
}

// setDomainCreatedCondition sets the DomainCreated condition
func setDomainCreatedCondition(domain *ingressv1alpha1.Domain, created bool, reason, message string) {
	status := metav1.ConditionTrue
	if !created {
		status = metav1.ConditionFalse
	}

	condition := metav1.Condition{
		Type:               ConditionDomainCreated,
		Status:             status,
		Reason:             reason,
		Message:            message,
		ObservedGeneration: domain.Generation,
	}

	meta.SetStatusCondition(&domain.Status.Conditions, condition)
}

// setCertificateReadyCondition sets the CertificateReady condition
func setCertificateReadyCondition(domain *ingressv1alpha1.Domain, ready bool, reason, message string) {
	status := metav1.ConditionTrue
	if !ready {
		status = metav1.ConditionFalse
	}

	condition := metav1.Condition{
		Type:               ConditionCertificateReady,
		Status:             status,
		Reason:             reason,
		Message:            message,
		ObservedGeneration: domain.Generation,
	}

	meta.SetStatusCondition(&domain.Status.Conditions, condition)
}

// setDNSConfiguredCondition sets the DNSConfigured condition
func setDNSConfiguredCondition(domain *ingressv1alpha1.Domain, configured bool, reason, message string) {
	status := metav1.ConditionTrue
	if !configured {
		status = metav1.ConditionFalse
	}

	condition := metav1.Condition{
		Type:               ConditionDNSConfigured,
		Status:             status,
		Reason:             reason,
		Message:            message,
		ObservedGeneration: domain.Generation,
	}

	meta.SetStatusCondition(&domain.Status.Conditions, condition)
}

// updateDomainConditions updates all domain conditions based on the ngrok domain state
func updateDomainConditions(domain *ingressv1alpha1.Domain, ngrokDomain *ngrok.ReservedDomain) {
	// Always set DomainCreated=True if we have an ID
	if domain.Status.ID != "" {
		setDomainCreatedCondition(domain, true, ReasonDomainCreated, "Domain successfully reserved")
	} else {
		setDomainCreatedCondition(domain, false, ReasonDomainInvalid, "Domain could not be reserved")
		setDomainReadyCondition(domain, false, ReasonDomainInvalid, "Domain could not be reserved")
		return
	}

	// Determine domain readiness based on type and certificate status
	if isNgrokSubdomain(domain.Spec.Domain) {
		// ngrok subdomains: no cert management needed, ready immediately
		setCertificateReadyCondition(domain, true, ReasonNgrokManaged, "Certificate managed by ngrok")
		setDNSConfiguredCondition(domain, true, ReasonNgrokManaged, "DNS managed by ngrok")
		setDomainReadyCondition(domain, true, ReasonDomainActive, "Domain ready for use")
	} else {
		// Custom domains: check certificate provisioning status
		if hasProvisioningError(ngrokDomain) {
			error := getProvisioningError(ngrokDomain)
			setCertificateReadyCondition(domain, false, ReasonProvisioningError, error.Message)

			if isDNSError(error) {
				setDNSConfiguredCondition(domain, false, ReasonDNSError, error.Message)
				setDomainReadyCondition(domain, false, ReasonDNSError, "DNS configuration required: "+error.Message)
			} else {
				setDNSConfiguredCondition(domain, true, ReasonDomainCreated, "DNS records configured")
				setDomainReadyCondition(domain, false, ReasonProvisioningError, "Certificate provisioning error: "+error.Message)
			}
		} else if isCertificateProvisioned(ngrokDomain) {
			setCertificateReadyCondition(domain, true, ReasonCertificateReady, "Certificate provisioned successfully")
			setDNSConfiguredCondition(domain, true, ReasonDomainCreated, "DNS records configured")
			setDomainReadyCondition(domain, true, ReasonDomainActive, "Domain ready for use")
		} else {
			// Still provisioning
			setCertificateReadyCondition(domain, false, ReasonCertificateProvisioning, "Certificate being provisioned")
			setDNSConfiguredCondition(domain, true, ReasonDomainCreated, "DNS records configured")
			setDomainReadyCondition(domain, false, ReasonWaitingForCertificate, "Waiting for certificate provisioning to complete")
		}
	}
}

// Helper functions to determine domain and certificate status

func isNgrokSubdomain(domain string) bool {
	ngrokDomains := []string{".ngrok.app", ".ngrok.dev", ".ngrok.io", ".ngrok.pizza"}
	for _, suffix := range ngrokDomains {
		if strings.HasSuffix(domain, suffix) {
			return true
		}
	}
	return false
}

func hasProvisioningError(ngrokDomain *ngrok.ReservedDomain) bool {
	return ngrokDomain.CertificateManagementStatus != nil &&
		ngrokDomain.CertificateManagementStatus.ProvisioningJob != nil &&
		ngrokDomain.CertificateManagementStatus.ProvisioningJob.ErrorCode != nil &&
		*ngrokDomain.CertificateManagementStatus.ProvisioningJob.ErrorCode != ""
}

func getProvisioningError(ngrokDomain *ngrok.ReservedDomain) *ingressv1alpha1.DomainStatusProvisioningJob {
	if !hasProvisioningError(ngrokDomain) {
		return nil
	}

	job := ngrokDomain.CertificateManagementStatus.ProvisioningJob
	return &ingressv1alpha1.DomainStatusProvisioningJob{
		ErrorCode: *job.ErrorCode,
		Message:   job.Msg,
	}
}

func isDNSError(job *ingressv1alpha1.DomainStatusProvisioningJob) bool {
	return job != nil && job.ErrorCode == "DNS_ERROR"
}

func isCertificateProvisioned(ngrokDomain *ngrok.ReservedDomain) bool {
	// Certificate is provisioned if:
	// 1. We have a certificate reference, AND
	// 2. There's no active provisioning job with errors
	return ngrokDomain.Certificate != nil && !hasProvisioningError(ngrokDomain)
}

func isDomainReady(domain *ingressv1alpha1.Domain) bool {
	readyCondition := meta.FindStatusCondition(domain.Status.Conditions, ConditionDomainReady)
	return readyCondition != nil && readyCondition.Status == metav1.ConditionTrue
}

// setDomainCreationFailedConditions sets conditions for domains that failed to be created
func setDomainCreationFailedConditions(domain *ingressv1alpha1.Domain, err error) {
	errorMsg := err.Error()

	// Determine specific failure reason based on error
	// TODO: These should use the consts from above
	var reason string
	if strings.Contains(errorMsg, "dangling") {
		reason = "DanglingDNSRecord"
	} else if strings.Contains(errorMsg, "already reserved") || strings.Contains(errorMsg, "protected") {
		reason = "ProtectedDomain"
	} else {
		reason = "DomainCreationFailed"
	}

	// Set all conditions to indicate creation failure
	setDomainCreatedCondition(domain, false, reason, errorMsg)
	setCertificateReadyCondition(domain, false, reason, "Domain creation failed")
	setDNSConfiguredCondition(domain, false, reason, "Domain creation failed")
	setDomainReadyCondition(domain, false, reason, errorMsg)
}
