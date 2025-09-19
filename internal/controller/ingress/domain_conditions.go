package ingress

import (
	"fmt"

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
	if domain.Status.ID == "" {
		message := "Domain could not be reserved"
		setDomainCreatedCondition(domain, false, ReasonDomainInvalid, message)
		setCertificateReadyCondition(domain, false, ReasonDomainInvalid, message)
		setDNSConfiguredCondition(domain, false, ReasonDomainInvalid, message)
		setDomainReadyCondition(domain, false, ReasonDomainInvalid, message)
		return
	}

	setDomainCreatedCondition(domain, true, ReasonDomainCreated, "Domain successfully reserved")

	if isNgrokManagedDomain(ngrokDomain) {
		setCertificateReadyCondition(domain, true, ReasonNgrokManaged, "Certificate managed by ngrok")
		setDNSConfiguredCondition(domain, true, ReasonNgrokManaged, "DNS managed by ngrok")
		setDomainReadyCondition(domain, true, ReasonDomainActive, "Domain ready for use")
		return
	}

	job := currentProvisioningJob(domain.Status.CertificateManagementStatus)
	if hasProvisioningError(job) {
		message := job.Message
		if job.ErrorCode != "" {
			message = fmt.Sprintf("%s (code=%s)", job.Message, job.ErrorCode)
		}
		setCertificateReadyCondition(domain, false, ReasonProvisioningError, message)
		setDNSConfiguredCondition(domain, false, ReasonProvisioningError, message)
		setDomainReadyCondition(domain, false, ReasonProvisioningError, message)
		return
	}

	if domain.Status.Certificate != nil {
		setCertificateReadyCondition(domain, true, ReasonCertificateReady, "Certificate provisioned successfully")
		setDNSConfiguredCondition(domain, true, ReasonDomainCreated, "DNS records configured")
		setDomainReadyCondition(domain, true, ReasonDomainActive, "Domain ready for use")
		return
	}

	message := "Certificate provisioning in progress"
	if job != nil && job.Message != "" {
		message = job.Message
	}
	setCertificateReadyCondition(domain, false, ReasonCertificateProvisioning, message)
	setDNSConfiguredCondition(domain, true, ReasonDomainCreated, "Waiting for certificate provisioning to complete")
	setDomainReadyCondition(domain, false, ReasonWaitingForCertificate, message)
}

// Helper functions to determine domain and certificate status

func isNgrokManagedDomain(ngrokDomain *ngrok.ReservedDomain) bool {
	return ngrokDomain.CertificateManagementPolicy == nil
}

func currentProvisioningJob(status *ingressv1alpha1.DomainStatusCertificateManagementStatus) *ingressv1alpha1.DomainStatusProvisioningJob {
	if status == nil {
		return nil
	}
	return status.ProvisioningJob
}

func hasProvisioningError(job *ingressv1alpha1.DomainStatusProvisioningJob) bool {
	return job != nil && job.ErrorCode != ""
}

// setDomainCreationFailedConditions sets conditions for domains that failed to be created
func setDomainCreationFailedConditions(domain *ingressv1alpha1.Domain, err error) {
	message := err.Error()
	setDomainCreatedCondition(domain, false, ReasonDomainCreationFailed, message)
	setCertificateReadyCondition(domain, false, ReasonDomainCreationFailed, "Domain creation failed")
	setDNSConfiguredCondition(domain, false, ReasonDomainCreationFailed, "Domain creation failed")
	setDomainReadyCondition(domain, false, ReasonDomainCreationFailed, message)
}
