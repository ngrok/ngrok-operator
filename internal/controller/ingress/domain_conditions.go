package ingress

import (
	"time"

	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/ngrok/ngrok-api-go/v7"
	ingressv1alpha1 "github.com/ngrok/ngrok-operator/api/ingress/v1alpha1"
	"github.com/ngrok/ngrok-operator/internal/domain"
	"github.com/ngrok/ngrok-operator/internal/ngrokapi"
)

// setReadyCondition sets the Ready condition based on the overall domain state
func setDomainReadyCondition(d *ingressv1alpha1.Domain, ready bool, reason, message string) {
	status := metav1.ConditionTrue
	if !ready {
		status = metav1.ConditionFalse
	}

	condition := metav1.Condition{
		Type:               domain.ConditionDomainReady,
		Status:             status,
		Reason:             reason,
		Message:            message,
		ObservedGeneration: d.Generation,
	}

	meta.SetStatusCondition(&d.Status.Conditions, condition)
}

// setProgressingCondition sets the Progressing condition
func setProgressingCondition(d *ingressv1alpha1.Domain, progressing bool, reason, message string) {
	status := metav1.ConditionTrue
	if !progressing {
		status = metav1.ConditionFalse
	}

	condition := metav1.Condition{
		Type:               domain.ConditionProgressing,
		Status:             status,
		Reason:             reason,
		Message:            message,
		ObservedGeneration: d.Generation,
	}
	meta.SetStatusCondition(&d.Status.Conditions, condition)
}

// setDomainCreatedCondition sets the DomainCreated condition
func setDomainCreatedCondition(d *ingressv1alpha1.Domain, created bool, reason, message string) {
	status := metav1.ConditionTrue
	if !created {
		status = metav1.ConditionFalse
	}

	condition := metav1.Condition{
		Type:               domain.ConditionDomainCreated,
		Status:             status,
		Reason:             reason,
		Message:            message,
		ObservedGeneration: d.Generation,
	}

	meta.SetStatusCondition(&d.Status.Conditions, condition)
}

// setCertificateReadyCondition sets the CertificateReady condition
func setCertificateReadyCondition(d *ingressv1alpha1.Domain, ready bool, reason, message string) {
	status := metav1.ConditionTrue
	if !ready {
		status = metav1.ConditionFalse
	}

	condition := metav1.Condition{
		Type:               domain.ConditionCertificateReady,
		Status:             status,
		Reason:             reason,
		Message:            message,
		ObservedGeneration: d.Generation,
	}

	meta.SetStatusCondition(&d.Status.Conditions, condition)
}

// setDNSConfiguredCondition sets the DNSConfigured condition
func setDNSConfiguredCondition(d *ingressv1alpha1.Domain, configured bool, reason, message string) {
	status := metav1.ConditionTrue
	if !configured {
		status = metav1.ConditionFalse
	}

	condition := metav1.Condition{
		Type:               domain.ConditionDNSConfigured,
		Status:             status,
		Reason:             reason,
		Message:            message,
		ObservedGeneration: d.Generation,
	}

	meta.SetStatusCondition(&d.Status.Conditions, condition)
}

// updateDomainConditions updates all domain conditions based on the ngrok domain state and any creation errors
func updateDomainConditions(d *ingressv1alpha1.Domain, ngrokDomain *ngrok.ReservedDomain, createErr error) {
	// Handle creation errors first
	if createErr != nil {
		message := ngrokapi.SanitizeErrorMessage(createErr.Error())
		setDomainCreatedCondition(d, false, domain.ReasonDomainCreationFailed, message)
		setCertificateReadyCondition(d, false, domain.ReasonDomainCreationFailed, "Domain creation failed")
		setDNSConfiguredCondition(d, false, domain.ReasonDomainCreationFailed, "Domain creation failed")
		setDomainReadyCondition(d, false, domain.ReasonDomainCreationFailed, message)
		return
	}

	if d.Status.ID == "" {
		message := "Domain could not be reserved"
		setDomainCreatedCondition(d, false, domain.ReasonDomainInvalid, message)
		setCertificateReadyCondition(d, false, domain.ReasonDomainInvalid, message)
		setDNSConfiguredCondition(d, false, domain.ReasonDomainInvalid, message)
		setDomainReadyCondition(d, false, domain.ReasonDomainInvalid, message)
		return
	}

	setDomainCreatedCondition(d, true, domain.ReasonDomainCreated, "Domain successfully reserved")

	// Check if its an ngrok domain. If so the DNS and certs are managed by ngrok
	// and already setup so the domain is ready.
	if isNgrokManagedDomain(ngrokDomain) {
		setCertificateReadyCondition(d, true, domain.ReasonNgrokManaged, "Certificate managed by ngrok")
		setDNSConfiguredCondition(d, true, domain.ReasonNgrokManaged, "DNS managed by ngrok")
		setDomainReadyCondition(d, true, domain.ReasonDomainActive, "Domain ready for use")
		return
	}

	// If the certificate is not null, then the certificate is provisioned and the domain is ready.
	if d.Status.Certificate != nil {
		setCertificateReadyCondition(d, true, domain.ReasonCertificateReady, "Certificate provisioned successfully")
		setDNSConfiguredCondition(d, true, domain.ReasonDomainCreated, "DNS records configured")
		setDomainReadyCondition(d, true, domain.ReasonDomainActive, "Domain ready for use")
		return
	}

	// Otherwise for custom domains, check the certificate management status
	message := "Certificate provisioning in progress"
	job := currentProvisioningJob(d.Status.CertificateManagementStatus)
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

	setCertificateReadyCondition(d, false, domain.ReasonProvisioningError, message)
	setDNSConfiguredCondition(d, false, domain.ReasonProvisioningError, message)
	setDomainReadyCondition(d, false, domain.ReasonProvisioningError, message)
	setProgressingCondition(d, true, domain.ReasonProvisioning, message)
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
