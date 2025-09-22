package ingress

import (
	"time"

	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/ngrok/ngrok-api-go/v7"
	ingressv1alpha1 "github.com/ngrok/ngrok-operator/api/ingress/v1alpha1"
	"github.com/ngrok/ngrok-operator/internal/ngrokapi"
)

const (
	// condition types for Domain
	ConditionDomainReady      = "Ready"
	ConditionDomainCreated    = "DomainCreated"
	ConditionCertificateReady = "CertificateReady"
	ConditionDNSConfigured    = "DNSConfigured"
	ConditionProgressing      = "Progressing"

	// condition reasons for Domain
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

// setProgressingCondition sets the Progressing condition
func setProgressingCondition(domain *ingressv1alpha1.Domain, progressing bool, reason, message string) {
	status := metav1.ConditionTrue
	if !progressing {
		status = metav1.ConditionFalse
	}

	condition := metav1.Condition{
		Type:               ConditionProgressing,
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
	if domain.Status.ID == "" {
		message := "Domain could not be reserved"
		setDomainCreatedCondition(domain, false, ReasonDomainInvalid, message)
		setCertificateReadyCondition(domain, false, ReasonDomainInvalid, message)
		setDNSConfiguredCondition(domain, false, ReasonDomainInvalid, message)
		setDomainReadyCondition(domain, false, ReasonDomainInvalid, message)
		return
	}

	setDomainCreatedCondition(domain, true, ReasonDomainCreated, "Domain successfully reserved")

	// Check if its an ngrok domain. If so the DNS and certs are managed by ngrok
	// and already setup so the domain is ready.
	if isNgrokManagedDomain(ngrokDomain) {
		setCertificateReadyCondition(domain, true, ReasonNgrokManaged, "Certificate managed by ngrok")
		setDNSConfiguredCondition(domain, true, ReasonNgrokManaged, "DNS managed by ngrok")
		setDomainReadyCondition(domain, true, ReasonDomainActive, "Domain ready for use")
		return
	}

	// If the certificate is not null, then the certificate is provisioned and the domain is ready.
	if domain.Status.Certificate != nil {
		setCertificateReadyCondition(domain, true, ReasonCertificateReady, "Certificate provisioned successfully")
		setDNSConfiguredCondition(domain, true, ReasonDomainCreated, "DNS records configured")
		setDomainReadyCondition(domain, true, ReasonDomainActive, "Domain ready for use")
		return
	}

	// Otherwise for custom domains, check the certificate management status
	message := "Certificate provisioning in progress"
	job := currentProvisioningJob(domain.Status.CertificateManagementStatus)
	if job != nil {
		// Check for errors
		if job.ErrorCode != "" {
			message = job.ErrorCode + " " + job.Message
		} else {
			// Otherwise just use the message
			message = job.Message
		}

		if job.StartedAt != nil {
			// Example: "started_at": "2025-09-19T01:19:46Z"
			message = message + " Started at " + job.StartedAt.Format(time.RFC3339)
		}

		if job.RetriesAt != nil {
			// Example: "retries_at": "2025-09-19T22:43:23Z"
			message = message + " Retries at " + job.RetriesAt.Format(time.RFC3339)
		}
	}

	setCertificateReadyCondition(domain, false, ReasonProvisioningError, message)
	setDNSConfiguredCondition(domain, false, ReasonProvisioningError, message)
	setDomainReadyCondition(domain, false, ReasonProvisioningError, message)
	setProgressingCondition(domain, true, ReasonProvisioning, message)
}

// Helper functions to determine domain and certificate status

// This uses the fact that the API returns back a null value for the certificate management policy for ngrok managed domains
// VS a custom domain has a policy for provisioning the custom cert.
func isNgrokManagedDomain(ngrokDomain *ngrok.ReservedDomain) bool {
	return ngrokDomain.CertificateManagementPolicy == nil
}

// currentProvisioningJob returns the current provisioning job from the domain status with nil checks
func currentProvisioningJob(status *ingressv1alpha1.DomainStatusCertificateManagementStatus) *ingressv1alpha1.DomainStatusProvisioningJob {
	if status == nil {
		return nil
	}
	return status.ProvisioningJob
}

// setDomainCreationFailedConditions sets conditions for domains that failed to be created
func setDomainCreationFailedConditions(domain *ingressv1alpha1.Domain, err error) {
	message := ngrokapi.SanitizeErrorMessage(err.Error())
	setDomainCreatedCondition(domain, false, ReasonDomainCreationFailed, message)
	setCertificateReadyCondition(domain, false, ReasonDomainCreationFailed, "Domain creation failed")
	setDNSConfiguredCondition(domain, false, ReasonDomainCreationFailed, "Domain creation failed")
	setDomainReadyCondition(domain, false, ReasonDomainCreationFailed, message)
}

// needsStatusFollowUp determines if a domain needs a requeue to observe
// certificate provisioning progress based on status conditions.
func needsStatusFollowUp(domain *ingressv1alpha1.Domain) bool {
	// Check if domain creation failed - don't follow up on terminally failed domains
	domainCreatedCondition := meta.FindStatusCondition(domain.Status.Conditions, ConditionDomainCreated)
	if domainCreatedCondition != nil && domainCreatedCondition.Status == metav1.ConditionFalse {
		return false
	}

	// Check if domain is ready - no need to follow up on ready domains
	readyCondition := meta.FindStatusCondition(domain.Status.Conditions, ConditionDomainReady)
	if readyCondition != nil && readyCondition.Status == metav1.ConditionTrue {
		return false
	}

	// Check certificate condition - follow up if certificate is not ready
	certCondition := meta.FindStatusCondition(domain.Status.Conditions, ConditionCertificateReady)
	if certCondition != nil && certCondition.Status == metav1.ConditionFalse {
		// Only follow up if it's not a terminal error (like domain creation failed)
		if certCondition.Reason != ReasonDomainCreationFailed &&
			certCondition.Reason != ReasonDomainInvalid {
			return true
		}
	}

	// Check DNS condition - follow up if DNS is not configured
	dnsCondition := meta.FindStatusCondition(domain.Status.Conditions, ConditionDNSConfigured)
	if dnsCondition != nil && dnsCondition.Status == metav1.ConditionFalse {
		// Only follow up if it's not a terminal error (like domain creation failed)
		if dnsCondition.Reason != ReasonDomainCreationFailed &&
			dnsCondition.Reason != ReasonDomainInvalid {
			return true
		}
	}

	return false
}
