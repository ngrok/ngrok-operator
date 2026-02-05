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
	domain := createTestDomain("test-domain", "test.example.com", "rd_123")
	createErr := errors.New("domain creation failed")

	updateDomainConditions(domain, nil, createErr)

	// All conditions should be false with creation failed reason
	readyCondition := meta.FindStatusCondition(domain.Status.Conditions, ConditionDomainReady)
	assert.NotNil(t, readyCondition)
	assert.Equal(t, metav1.ConditionFalse, readyCondition.Status)
	assert.Equal(t, ReasonDomainCreationFailed, readyCondition.Reason)

	createdCondition := meta.FindStatusCondition(domain.Status.Conditions, ConditionDomainCreated)
	assert.NotNil(t, createdCondition)
	assert.Equal(t, metav1.ConditionFalse, createdCondition.Status)
	assert.Equal(t, ReasonDomainCreationFailed, createdCondition.Reason)

	certCondition := meta.FindStatusCondition(domain.Status.Conditions, ConditionCertificateReady)
	assert.NotNil(t, certCondition)
	assert.Equal(t, metav1.ConditionFalse, certCondition.Status)
	assert.Equal(t, ReasonDomainCreationFailed, certCondition.Reason)

	dnsCondition := meta.FindStatusCondition(domain.Status.Conditions, ConditionDNSConfigured)
	assert.NotNil(t, dnsCondition)
	assert.Equal(t, metav1.ConditionFalse, dnsCondition.Status)
	assert.Equal(t, ReasonDomainCreationFailed, dnsCondition.Reason)
}

func TestUpdateDomainConditions_NoID(t *testing.T) {
	domain := createTestDomain("test-domain", "test.example.com", "")

	updateDomainConditions(domain, nil, nil)

	// All conditions should be false with invalid reason
	readyCondition := meta.FindStatusCondition(domain.Status.Conditions, ConditionDomainReady)
	assert.NotNil(t, readyCondition)
	assert.Equal(t, metav1.ConditionFalse, readyCondition.Status)
	assert.Equal(t, ReasonDomainInvalid, readyCondition.Reason)
}

func TestUpdateDomainConditions_NgrokManagedDomain(t *testing.T) {
	domain := createTestDomain("test-domain", "test.ngrok.app", "rd_123")
	ngrokDomain := &ngrok.ReservedDomain{
		ID:     "rd_123",
		Domain: "test.ngrok.app",
		// No CertificateManagementPolicy = ngrok managed
	}

	updateDomainConditions(domain, ngrokDomain, nil)

	// All conditions should be true for ngrok managed domains
	readyCondition := meta.FindStatusCondition(domain.Status.Conditions, ConditionDomainReady)
	assert.NotNil(t, readyCondition)
	assert.Equal(t, metav1.ConditionTrue, readyCondition.Status)
	assert.Equal(t, ReasonDomainActive, readyCondition.Reason)

	createdCondition := meta.FindStatusCondition(domain.Status.Conditions, ConditionDomainCreated)
	assert.NotNil(t, createdCondition)
	assert.Equal(t, metav1.ConditionTrue, createdCondition.Status)
	assert.Equal(t, ReasonDomainCreated, createdCondition.Reason)

	certCondition := meta.FindStatusCondition(domain.Status.Conditions, ConditionCertificateReady)
	assert.NotNil(t, certCondition)
	assert.Equal(t, metav1.ConditionTrue, certCondition.Status)
	assert.Equal(t, ReasonNgrokManaged, certCondition.Reason)

	dnsCondition := meta.FindStatusCondition(domain.Status.Conditions, ConditionDNSConfigured)
	assert.NotNil(t, dnsCondition)
	assert.Equal(t, metav1.ConditionTrue, dnsCondition.Status)
	assert.Equal(t, ReasonNgrokManaged, dnsCondition.Reason)
}

func TestUpdateDomainConditions_CustomDomainWithCertificate(t *testing.T) {
	domain := createTestDomainWithCertificate("test-domain", "test.example.com", "rd_123")
	ngrokDomain := &ngrok.ReservedDomain{
		ID:                          "rd_123",
		Domain:                      "test.example.com",
		CertificateManagementPolicy: &ngrok.ReservedDomainCertPolicy{Authority: "letsencrypt"},
	}

	updateDomainConditions(domain, ngrokDomain, nil)

	// All conditions should be true when certificate is provisioned
	readyCondition := meta.FindStatusCondition(domain.Status.Conditions, ConditionDomainReady)
	assert.NotNil(t, readyCondition)
	assert.Equal(t, metav1.ConditionTrue, readyCondition.Status)
	assert.Equal(t, ReasonDomainActive, readyCondition.Reason)

	certCondition := meta.FindStatusCondition(domain.Status.Conditions, ConditionCertificateReady)
	assert.NotNil(t, certCondition)
	assert.Equal(t, metav1.ConditionTrue, certCondition.Status)
	assert.Equal(t, ReasonCertificateReady, certCondition.Reason)

	dnsCondition := meta.FindStatusCondition(domain.Status.Conditions, ConditionDNSConfigured)
	assert.NotNil(t, dnsCondition)
	assert.Equal(t, metav1.ConditionTrue, dnsCondition.Status)
	assert.Equal(t, ReasonDomainCreated, dnsCondition.Reason)
}

func TestUpdateDomainConditions_CustomDomainProvisioning(t *testing.T) {
	domain := createTestDomain("test-domain", "test.example.com", "rd_123")
	ngrokDomain := &ngrok.ReservedDomain{
		ID:                          "rd_123",
		Domain:                      "test.example.com",
		CertificateManagementPolicy: &ngrok.ReservedDomainCertPolicy{Authority: "letsencrypt"},
	}

	updateDomainConditions(domain, ngrokDomain, nil)

	// Domain should be created but not ready
	createdCondition := meta.FindStatusCondition(domain.Status.Conditions, ConditionDomainCreated)
	assert.NotNil(t, createdCondition)
	assert.Equal(t, metav1.ConditionTrue, createdCondition.Status)
	assert.Equal(t, ReasonDomainCreated, createdCondition.Reason)

	// But not ready due to provisioning
	readyCondition := meta.FindStatusCondition(domain.Status.Conditions, ConditionDomainReady)
	assert.NotNil(t, readyCondition)
	assert.Equal(t, metav1.ConditionFalse, readyCondition.Status)
	assert.Equal(t, ReasonProvisioningError, readyCondition.Reason)

	// Progressing should be true
	progressingCondition := meta.FindStatusCondition(domain.Status.Conditions, ConditionProgressing)
	assert.NotNil(t, progressingCondition)
	assert.Equal(t, metav1.ConditionTrue, progressingCondition.Status)
	assert.Equal(t, ReasonProvisioning, progressingCondition.Reason)
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
	domain := createTestDomainWithCertManagement("test-domain", "test.example.com", "rd_123", job)
	ngrokDomain := &ngrok.ReservedDomain{
		ID:                          "rd_123",
		Domain:                      "test.example.com",
		CertificateManagementPolicy: &ngrok.ReservedDomainCertPolicy{Authority: "letsencrypt"},
	}

	updateDomainConditions(domain, ngrokDomain, nil)

	// Should include job details in message
	readyCondition := meta.FindStatusCondition(domain.Status.Conditions, ConditionDomainReady)
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

func TestIsDomainReady(t *testing.T) {
	tests := []struct {
		name     string
		domain   *ingressv1alpha1.Domain
		expected bool
	}{
		{
			name: "domain with no ID",
			domain: &ingressv1alpha1.Domain{
				Status: ingressv1alpha1.DomainStatus{
					ID: "",
				},
			},
			expected: false,
		},
		{
			name: "domain with ID but no Ready condition",
			domain: &ingressv1alpha1.Domain{
				Status: ingressv1alpha1.DomainStatus{
					ID:         "rd_123",
					Conditions: []metav1.Condition{},
				},
			},
			expected: false,
		},
		{
			name: "domain with ID and Ready condition false",
			domain: &ingressv1alpha1.Domain{
				Status: ingressv1alpha1.DomainStatus{
					ID: "rd_123",
					Conditions: []metav1.Condition{
						{
							Type:   ConditionDomainReady,
							Status: metav1.ConditionFalse,
							Reason: ReasonProvisioningError,
						},
					},
				},
			},
			expected: false,
		},
		{
			name: "domain with ID and Ready condition true",
			domain: &ingressv1alpha1.Domain{
				Status: ingressv1alpha1.DomainStatus{
					ID: "rd_123",
					Conditions: []metav1.Condition{
						{
							Type:   ConditionDomainReady,
							Status: metav1.ConditionTrue,
							Reason: ReasonDomainActive,
						},
					},
				},
			},
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := IsDomainReady(tt.domain)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestBuildResolvesToRequest(t *testing.T) {
	tests := []struct {
		name     string
		input    *[]ingressv1alpha1.DomainResolvesToEntry
		expected []ngrok.ReservedDomainResolvesToEntry
	}{
		{
			name:     "nil input",
			input:    nil,
			expected: nil,
		},
		{
			name:     "empty slice",
			input:    &[]ingressv1alpha1.DomainResolvesToEntry{},
			expected: nil,
		},
		{
			name:     "single entry",
			input:    &[]ingressv1alpha1.DomainResolvesToEntry{{Value: "us"}},
			expected: []ngrok.ReservedDomainResolvesToEntry{{Value: "us"}},
		},
		{
			name:     "multiple entries",
			input:    &[]ingressv1alpha1.DomainResolvesToEntry{{Value: "us"}, {Value: "eu"}},
			expected: []ngrok.ReservedDomainResolvesToEntry{{Value: "us"}, {Value: "eu"}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := buildResolvesToRequest(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestBuildResolvesToStatus(t *testing.T) {
	tests := []struct {
		name     string
		input    []ngrok.ReservedDomainResolvesToEntry
		expected *[]ingressv1alpha1.DomainResolvesToEntry
	}{
		{
			name:     "nil input",
			input:    nil,
			expected: nil,
		},
		{
			name:     "empty slice",
			input:    []ngrok.ReservedDomainResolvesToEntry{},
			expected: nil,
		},
		{
			name:     "single entry",
			input:    []ngrok.ReservedDomainResolvesToEntry{{Value: "us"}},
			expected: &[]ingressv1alpha1.DomainResolvesToEntry{{Value: "us"}},
		},
		{
			name:     "multiple entries",
			input:    []ngrok.ReservedDomainResolvesToEntry{{Value: "us"}, {Value: "eu"}},
			expected: &[]ingressv1alpha1.DomainResolvesToEntry{{Value: "us"}, {Value: "eu"}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := buildResolvesToStatus(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}
