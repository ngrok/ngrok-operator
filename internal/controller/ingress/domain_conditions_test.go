package ingress

import (
	"errors"
	"testing"
	"time"

	"github.com/ngrok/ngrok-api-go/v7"
	"github.com/stretchr/testify/assert"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	ingressv1alpha1 "github.com/ngrok/ngrok-operator/api/ingress/v1alpha1"
	"github.com/ngrok/ngrok-operator/internal/domain"
)

// Helper function to create a test domain
func createTestDomain(name, domainName, id string) *ingressv1alpha1.Domain {
	return &ingressv1alpha1.Domain{
		ObjectMeta: metav1.ObjectMeta{
			Name:       name,
			Generation: 1,
		},
		Spec: ingressv1alpha1.DomainSpec{
			Domain: domainName,
		},
		Status: ingressv1alpha1.DomainStatus{
			ID: id,
		},
	}
}

// Helper function to create a test domain with certificate
func createTestDomainWithCertificate(name, domainName, id string) *ingressv1alpha1.Domain {
	domain := createTestDomain(name, domainName, id)
	domain.Status.Certificate = &ingressv1alpha1.DomainStatusCertificateInfo{
		ID: "cert_123",
	}
	return domain
}

// Helper function to create a test domain with certificate management status
func createTestDomainWithCertManagement(name, domainName, id string, job *ingressv1alpha1.DomainStatusProvisioningJob) *ingressv1alpha1.Domain {
	domain := createTestDomain(name, domainName, id)
	domain.Status.CertificateManagementStatus = &ingressv1alpha1.DomainStatusCertificateManagementStatus{
		ProvisioningJob: job,
	}
	return domain
}

func TestUpdateDomainConditions_CreationError(t *testing.T) {
	d := createTestDomain("test-domain", "test.example.com", "rd_123")
	createErr := errors.New("domain creation failed")

	updateDomainConditions(d, nil, createErr)

	// All conditions should be false with creation failed reason
	readyCondition := meta.FindStatusCondition(d.Status.Conditions, domain.ConditionDomainReady)
	assert.NotNil(t, readyCondition)
	assert.Equal(t, metav1.ConditionFalse, readyCondition.Status)
	assert.Equal(t, domain.ReasonDomainCreationFailed, readyCondition.Reason)

	createdCondition := meta.FindStatusCondition(d.Status.Conditions, domain.ConditionDomainCreated)
	assert.NotNil(t, createdCondition)
	assert.Equal(t, metav1.ConditionFalse, createdCondition.Status)
	assert.Equal(t, domain.ReasonDomainCreationFailed, createdCondition.Reason)

	certCondition := meta.FindStatusCondition(d.Status.Conditions, domain.ConditionCertificateReady)
	assert.NotNil(t, certCondition)
	assert.Equal(t, metav1.ConditionFalse, certCondition.Status)
	assert.Equal(t, domain.ReasonDomainCreationFailed, certCondition.Reason)

	dnsCondition := meta.FindStatusCondition(d.Status.Conditions, domain.ConditionDNSConfigured)
	assert.NotNil(t, dnsCondition)
	assert.Equal(t, metav1.ConditionFalse, dnsCondition.Status)
	assert.Equal(t, domain.ReasonDomainCreationFailed, dnsCondition.Reason)
}

func TestUpdateDomainConditions_NoID(t *testing.T) {
	d := createTestDomain("test-domain", "test.example.com", "")

	updateDomainConditions(d, nil, nil)

	// All conditions should be false with invalid reason
	readyCondition := meta.FindStatusCondition(d.Status.Conditions, domain.ConditionDomainReady)
	assert.NotNil(t, readyCondition)
	assert.Equal(t, metav1.ConditionFalse, readyCondition.Status)
	assert.Equal(t, domain.ReasonDomainInvalid, readyCondition.Reason)
}

func TestUpdateDomainConditions_NgrokManagedDomain(t *testing.T) {
	d := createTestDomain("test-domain", "test.ngrok.app", "rd_123")
	ngrokDomain := &ngrok.ReservedDomain{
		ID:     "rd_123",
		Domain: "test.ngrok.app",
		// No CertificateManagementPolicy = ngrok managed
	}

	updateDomainConditions(d, ngrokDomain, nil)

	// All conditions should be true for ngrok managed domains
	readyCondition := meta.FindStatusCondition(d.Status.Conditions, domain.ConditionDomainReady)
	assert.NotNil(t, readyCondition)
	assert.Equal(t, metav1.ConditionTrue, readyCondition.Status)
	assert.Equal(t, domain.ReasonDomainActive, readyCondition.Reason)

	createdCondition := meta.FindStatusCondition(d.Status.Conditions, domain.ConditionDomainCreated)
	assert.NotNil(t, createdCondition)
	assert.Equal(t, metav1.ConditionTrue, createdCondition.Status)
	assert.Equal(t, domain.ReasonDomainCreated, createdCondition.Reason)

	certCondition := meta.FindStatusCondition(d.Status.Conditions, domain.ConditionCertificateReady)
	assert.NotNil(t, certCondition)
	assert.Equal(t, metav1.ConditionTrue, certCondition.Status)
	assert.Equal(t, domain.ReasonNgrokManaged, certCondition.Reason)

	dnsCondition := meta.FindStatusCondition(d.Status.Conditions, domain.ConditionDNSConfigured)
	assert.NotNil(t, dnsCondition)
	assert.Equal(t, metav1.ConditionTrue, dnsCondition.Status)
	assert.Equal(t, domain.ReasonNgrokManaged, dnsCondition.Reason)
}

func TestUpdateDomainConditions_CustomDomainWithCertificate(t *testing.T) {
	d := createTestDomainWithCertificate("test-domain", "test.example.com", "rd_123")
	ngrokDomain := &ngrok.ReservedDomain{
		ID:                          "rd_123",
		Domain:                      "test.example.com",
		CertificateManagementPolicy: &ngrok.ReservedDomainCertPolicy{Authority: "letsencrypt"},
	}

	updateDomainConditions(d, ngrokDomain, nil)

	// All conditions should be true when certificate is provisioned
	readyCondition := meta.FindStatusCondition(d.Status.Conditions, domain.ConditionDomainReady)
	assert.NotNil(t, readyCondition)
	assert.Equal(t, metav1.ConditionTrue, readyCondition.Status)
	assert.Equal(t, domain.ReasonDomainActive, readyCondition.Reason)

	certCondition := meta.FindStatusCondition(d.Status.Conditions, domain.ConditionCertificateReady)
	assert.NotNil(t, certCondition)
	assert.Equal(t, metav1.ConditionTrue, certCondition.Status)
	assert.Equal(t, domain.ReasonCertificateReady, certCondition.Reason)

	dnsCondition := meta.FindStatusCondition(d.Status.Conditions, domain.ConditionDNSConfigured)
	assert.NotNil(t, dnsCondition)
	assert.Equal(t, metav1.ConditionTrue, dnsCondition.Status)
	assert.Equal(t, domain.ReasonDomainCreated, dnsCondition.Reason)
}

func TestUpdateDomainConditions_CustomDomainProvisioning(t *testing.T) {
	d := createTestDomain("test-domain", "test.example.com", "rd_123")
	ngrokDomain := &ngrok.ReservedDomain{
		ID:                          "rd_123",
		Domain:                      "test.example.com",
		CertificateManagementPolicy: &ngrok.ReservedDomainCertPolicy{Authority: "letsencrypt"},
	}

	updateDomainConditions(d, ngrokDomain, nil)

	// Domain should be created but not ready
	createdCondition := meta.FindStatusCondition(d.Status.Conditions, domain.ConditionDomainCreated)
	assert.NotNil(t, createdCondition)
	assert.Equal(t, metav1.ConditionTrue, createdCondition.Status)
	assert.Equal(t, domain.ReasonDomainCreated, createdCondition.Reason)

	// But not ready due to provisioning
	readyCondition := meta.FindStatusCondition(d.Status.Conditions, domain.ConditionDomainReady)
	assert.NotNil(t, readyCondition)
	assert.Equal(t, metav1.ConditionFalse, readyCondition.Status)
	assert.Equal(t, domain.ReasonProvisioningError, readyCondition.Reason)

	// Progressing should be true
	progressingCondition := meta.FindStatusCondition(d.Status.Conditions, domain.ConditionProgressing)
	assert.NotNil(t, progressingCondition)
	assert.Equal(t, metav1.ConditionTrue, progressingCondition.Status)
	assert.Equal(t, domain.ReasonProvisioning, progressingCondition.Reason)
}

func TestUpdateDomainConditions_CustomDomainWithProvisioningJob(t *testing.T) {
	startTime := metav1.NewTime(time.Now())
	retryTime := metav1.NewTime(time.Now().Add(time.Hour))
	job := &ingressv1alpha1.DomainStatusProvisioningJob{
		ErrorCode: "DNS_ERROR",
		Message:   "DNS records not configured",
		StartedAt: &startTime,
		RetriesAt: &retryTime,
	}
	d := createTestDomainWithCertManagement("test-domain", "test.example.com", "rd_123", job)
	ngrokDomain := &ngrok.ReservedDomain{
		ID:                          "rd_123",
		Domain:                      "test.example.com",
		CertificateManagementPolicy: &ngrok.ReservedDomainCertPolicy{Authority: "letsencrypt"},
	}

	updateDomainConditions(d, ngrokDomain, nil)

	// Should include job details in message
	readyCondition := meta.FindStatusCondition(d.Status.Conditions, domain.ConditionDomainReady)
	assert.NotNil(t, readyCondition)
	assert.Contains(t, readyCondition.Message, "DNS_ERROR")
	assert.Contains(t, readyCondition.Message, "DNS records not configured")
	assert.Contains(t, readyCondition.Message, "Started at")
	assert.Contains(t, readyCondition.Message, "Retries at")
}

func TestIsNgrokManagedDomain(t *testing.T) {
	tests := []struct {
		name        string
		ngrokDomain *ngrok.ReservedDomain
		expected    bool
	}{
		{
			name: "ngrok managed domain (no certificate management policy)",
			ngrokDomain: &ngrok.ReservedDomain{
				ID:     "rd_123",
				Domain: "test.ngrok.app",
				// No CertificateManagementPolicy
			},
			expected: true,
		},
		{
			name: "custom domain (has certificate management policy)",
			ngrokDomain: &ngrok.ReservedDomain{
				ID:                          "rd_456",
				Domain:                      "test.example.com",
				CertificateManagementPolicy: &ngrok.ReservedDomainCertPolicy{Authority: "letsencrypt"},
			},
			expected: false,
		},
		{
			name:        "nil domain",
			ngrokDomain: nil,
			expected:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isNgrokManagedDomain(tt.ngrokDomain)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestCurrentProvisioningJob(t *testing.T) {
	tests := []struct {
		name     string
		status   *ingressv1alpha1.DomainStatusCertificateManagementStatus
		expected *ingressv1alpha1.DomainStatusProvisioningJob
	}{
		{
			name:     "nil status",
			status:   nil,
			expected: nil,
		},
		{
			name:     "status with nil job",
			status:   &ingressv1alpha1.DomainStatusCertificateManagementStatus{},
			expected: nil,
		},
		{
			name: "status with job",
			status: &ingressv1alpha1.DomainStatusCertificateManagementStatus{
				ProvisioningJob: &ingressv1alpha1.DomainStatusProvisioningJob{
					ErrorCode: "DNS_ERROR",
					Message:   "DNS not configured",
				},
			},
			expected: &ingressv1alpha1.DomainStatusProvisioningJob{
				ErrorCode: "DNS_ERROR",
				Message:   "DNS not configured",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := currentProvisioningJob(tt.status)
			assert.Equal(t, tt.expected, result)
		})
	}
}
