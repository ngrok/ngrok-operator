package ingress

import (
	"time"

	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/ngrok/ngrok-api-go/v7"
	ingressv1alpha1 "github.com/ngrok/ngrok-operator/api/ingress/v1alpha1"
	"github.com/ngrok/ngrok-operator/internal/ngrokapi"
)

// setReadyCondition sets the Ready condition based on the overall domain state
func setDomainReadyCondition(domain *ingressv1alpha1.Domain, ready bool, reason ingressv1alpha1.DomainConditionReadyReason, message string) {
	status := metav1.ConditionTrue
	if !ready {
		status = metav1.ConditionFalse
	}

	condition := metav1.Condition{
		Type:               string(ingressv1alpha1.DomainConditionReady),
		Status:             status,
		Reason:             string(reason),
		Message:            message,
		ObservedGeneration: domain.Generation,
	}

	meta.SetStatusCondition(&domain.Status.Conditions, condition)
}

// setProgressingCondition sets the Progressing condition
func setProgressingCondition(domain *ingressv1alpha1.Domain, progressing bool, reason ingressv1alpha1.DomainConditionProgressingReason, message string) {
	status := metav1.ConditionTrue
	if !progressing {
		status = metav1.ConditionFalse
	}

	condition := metav1.Condition{
		Type:               string(ingressv1alpha1.DomainConditionProgressing),
		Status:             status,
		Reason:             string(reason),
		Message:            message,
		ObservedGeneration: domain.Generation,
	}
	meta.SetStatusCondition(&domain.Status.Conditions, condition)
}

// setDomainCreatedCondition sets the DomainCreated condition
func setDomainCreatedCondition(domain *ingressv1alpha1.Domain, created bool, reason ingressv1alpha1.DomainConditionCreatedReason, message string) {
	status := metav1.ConditionTrue
	if !created {
		status = metav1.ConditionFalse
	}

	condition := metav1.Condition{
		Type:               string(ingressv1alpha1.DomainConditionCreated),
		Status:             status,
		Reason:             string(reason),
		Message:            message,
		ObservedGeneration: domain.Generation,
	}

	meta.SetStatusCondition(&domain.Status.Conditions, condition)
}

// setCertificateReadyCondition sets the CertificateReady condition
func setCertificateReadyCondition(domain *ingressv1alpha1.Domain, ready bool, reason ingressv1alpha1.DomainConditionCertificateReadyReason, message string) {
	status := metav1.ConditionTrue
	if !ready {
		status = metav1.ConditionFalse
	}

	condition := metav1.Condition{
		Type:               string(ingressv1alpha1.DomainConditionCertificateReady),
		Status:             status,
		Reason:             string(reason),
		Message:            message,
		ObservedGeneration: domain.Generation,
	}

	meta.SetStatusCondition(&domain.Status.Conditions, condition)
}

// setDNSConfiguredCondition sets the DNSConfigured condition
func setDNSConfiguredCondition(domain *ingressv1alpha1.Domain, configured bool, reason ingressv1alpha1.DomainConditionDNSConfiguredReason, message string) {
	status := metav1.ConditionTrue
	if !configured {
		status = metav1.ConditionFalse
	}

	condition := metav1.Condition{
		Type:               string(ingressv1alpha1.DomainConditionDNSConfigured),
		Status:             status,
		Reason:             string(reason),
		Message:            message,
		ObservedGeneration: domain.Generation,
	}

	meta.SetStatusCondition(&domain.Status.Conditions, condition)
}

// updateDomainConditions updates all domain conditions based on the ngrok domain state and any creation errors
func updateDomainConditions(domain *ingressv1alpha1.Domain, ngrokDomain *ngrok.ReservedDomain, createErr error) {
	// Handle creation errors first
	if createErr != nil {
		message := ngrokapi.SanitizeErrorMessage(createErr.Error())
		setDomainCreatedCondition(domain, false, ingressv1alpha1.DomainCreatedReasonCreationFailed, message)
		setCertificateReadyCondition(domain, false, ingressv1alpha1.DomainCertificateReadyReasonCreationFailed, "Domain creation failed")
		setDNSConfiguredCondition(domain, false, ingressv1alpha1.DomainDNSConfiguredReasonCreationFailed, "Domain creation failed")
		setDomainReadyCondition(domain, false, ingressv1alpha1.DomainReasonCreationFailed, message)
		return
	}

	if domain.Status.ID == "" {
		message := "Domain could not be reserved"
		setDomainCreatedCondition(domain, false, ingressv1alpha1.DomainCreatedReasonInvalid, message)
		setCertificateReadyCondition(domain, false, ingressv1alpha1.DomainCertificateReadyReasonInvalid, message)
		setDNSConfiguredCondition(domain, false, ingressv1alpha1.DomainDNSConfiguredReasonInvalid, message)
		setDomainReadyCondition(domain, false, ingressv1alpha1.DomainReasonInvalid, message)
		return
	}

	setDomainCreatedCondition(domain, true, ingressv1alpha1.DomainCreatedReasonCreated, "Domain successfully reserved")

	// Check if its an ngrok domain. If so the DNS and certs are managed by ngrok
	// and already setup so the domain is ready.
	if isNgrokManagedDomain(ngrokDomain) {
		setCertificateReadyCondition(domain, true, ingressv1alpha1.DomainCertificateReadyReasonNgrokManaged, "Certificate managed by ngrok")
		setDNSConfiguredCondition(domain, true, ingressv1alpha1.DomainDNSConfiguredReasonNgrokManaged, "DNS managed by ngrok")
		setDomainReadyCondition(domain, true, ingressv1alpha1.DomainReasonActive, "Domain ready for use")
		return
	}

	// If the certificate is not null, then the certificate is provisioned and the domain is ready.
	if domain.Status.Certificate != nil {
		setCertificateReadyCondition(domain, true, ingressv1alpha1.DomainCertificateReadyReasonReady, "Certificate provisioned successfully")
		setDNSConfiguredCondition(domain, true, ingressv1alpha1.DomainDNSConfiguredReasonConfigured, "DNS records configured")
		setDomainReadyCondition(domain, true, ingressv1alpha1.DomainReasonActive, "Domain ready for use")
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

	setCertificateReadyCondition(domain, false, ingressv1alpha1.DomainCertificateReadyReasonProvisioningError, message)
	setDNSConfiguredCondition(domain, false, ingressv1alpha1.DomainDNSConfiguredReasonProvisioningError, message)
	setDomainReadyCondition(domain, false, ingressv1alpha1.DomainReasonProvisioningError, message)
	setProgressingCondition(domain, true, ingressv1alpha1.DomainReasonProvisioning, message)
}

// Helper functions to determine domain and certificate status

// This uses the fact that the API returns back a null value for the certificate management policy for ngrok managed domains
// VS a custom domain has a policy for provisioning the custom cert.
func isNgrokManagedDomain(ngrokDomain *ngrok.ReservedDomain) bool {
	if ngrokDomain == nil {
		return false
	}
	return ngrokDomain.CertificateManagementPolicy == nil
}

// currentProvisioningJob returns the current provisioning job from the domain status with nil checks
func currentProvisioningJob(status *ingressv1alpha1.DomainStatusCertificateManagementStatus) *ingressv1alpha1.DomainStatusProvisioningJob {
	if status == nil {
		return nil
	}
	return status.ProvisioningJob
}

// IsDomainReady checks if a domain is ready by examining both Status.ID and Ready condition
func IsDomainReady(domain *ingressv1alpha1.Domain) bool {
	// First check if domain has an ID (basic requirement)
	if domain.Status.ID == "" {
		return false
	}

	// Then check the Ready condition for more detailed status
	readyCondition := meta.FindStatusCondition(domain.Status.Conditions, string(ingressv1alpha1.DomainConditionReady))
	if readyCondition == nil {
		// No ready condition set yet, so it's not ready
		return false
	}

	return readyCondition.Status == metav1.ConditionTrue
}
