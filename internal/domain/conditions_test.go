package domain

import (
	"errors"
	"testing"
	"time"

	"github.com/ngrok/ngrok-api-go/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	ngrokv1 "github.com/ngrok/ngrok-operator/api/ngrok/v1"
)

// Helper function to create a test domain
func createTestDomain(name, domainName, id string) *ngrokv1.Domain {
	return &ngrokv1.Domain{
		Name:       name,
		Generation: 1,
		Spec: ngrokv1.DomainSpec{
			Domain: domainName,
		},
		Status: ngrokv1.DomainStatus{
			ID: id,
		},
	}
}

// Helper function to create a test domain with certificate
func createTestDomainWithCertificate(name, domainName, id string) *ngrokv1.Domain {
	domain := createTestDomain(name, domainName, id)
	domain.Status.Certificate = &ngrokv1.DomainStatusCertificateInfo{
		ID: "cert_123",
	}
	return domain
}

// Helper function to create a test domain with certificate management status
func createTestDomainWithCertManagement(name, domainName, id string, job *ngrokv1.DomainStatusProvisioningJob) *ngrokv1.Domain {
	domain := createTestDomain(name, domainName, id)
	domain.Status.CertificateManagementStatus = &ngrokv1.DomainStatusCertificateManagementStatus{
		ProvisioningJob: job,
	}
	return domain
}

// Helper function to create a test domain whose reservation was skipped because
// a wildcard parent already covers it. status.ID stays empty on purpose.
func createTestDomainCoveredByWildcard(name, domainName, wildcard string) *ngrokv1.Domain {
	domain := createTestDomain(name, domainName, "")
	domain.Status.Domain = domainName
	domain.Status.CoveredByWildcardDomain = &ngrokv1.DomainStatusWildcardDomain{ID: "rd_wildcard", Domain: wildcard}
	return domain
}

func TestUpdateDomainConditions_CreationError(t *testing.T) {
	domain := createTestDomain("test-domain", "test.example.com", "rd_123")
	createErr := errors.New("domain creation failed")

	UpdateConditions(FromV1(domain), nil, createErr)

	// All conditions should be false with creation failed reason
	readyCondition := meta.FindStatusCondition(domain.Status.Conditions, ConditionReady)
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

	UpdateConditions(FromV1(domain), nil, nil)

	// All conditions should be false with invalid reason
	readyCondition := meta.FindStatusCondition(domain.Status.Conditions, ConditionReady)
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

	UpdateConditions(FromV1(domain), ngrokDomain, nil)

	// All conditions should be true for ngrok managed domains
	readyCondition := meta.FindStatusCondition(domain.Status.Conditions, ConditionReady)
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

	UpdateConditions(FromV1(domain), ngrokDomain, nil)

	// All conditions should be true when certificate is provisioned
	readyCondition := meta.FindStatusCondition(domain.Status.Conditions, ConditionReady)
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

	UpdateConditions(FromV1(domain), ngrokDomain, nil)

	// Domain should be created but not ready
	createdCondition := meta.FindStatusCondition(domain.Status.Conditions, ConditionDomainCreated)
	assert.NotNil(t, createdCondition)
	assert.Equal(t, metav1.ConditionTrue, createdCondition.Status)
	assert.Equal(t, ReasonDomainCreated, createdCondition.Reason)

	// But not ready due to provisioning
	readyCondition := meta.FindStatusCondition(domain.Status.Conditions, ConditionReady)
	assert.NotNil(t, readyCondition)
	assert.Equal(t, metav1.ConditionFalse, readyCondition.Status)
	assert.Equal(t, ReasonProvisioningError, readyCondition.Reason)
}

func TestUpdateDomainConditions_CustomDomainWithProvisioningJob(t *testing.T) {
	startTime := metav1.NewTime(time.Now())
	retryTime := metav1.NewTime(time.Now().Add(time.Hour))
	job := &ngrokv1.DomainStatusProvisioningJob{
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

	UpdateConditions(FromV1(domain), ngrokDomain, nil)

	// Should include job details in message
	readyCondition := meta.FindStatusCondition(domain.Status.Conditions, ConditionReady)
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
		status   *ngrokv1.DomainStatusCertificateManagementStatus
		expected *ngrokv1.DomainStatusProvisioningJob
	}{
		{
			name:     "nil status",
			status:   nil,
			expected: nil,
		},
		{
			name:     "status with nil job",
			status:   &ngrokv1.DomainStatusCertificateManagementStatus{},
			expected: nil,
		},
		{
			name: "status with job",
			status: &ngrokv1.DomainStatusCertificateManagementStatus{
				ProvisioningJob: &ngrokv1.DomainStatusProvisioningJob{
					ErrorCode: "DNS_ERROR",
					Message:   "DNS not configured",
				},
			},
			expected: &ngrokv1.DomainStatusProvisioningJob{
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

func TestIsReady(t *testing.T) {
	tests := []struct {
		name     string
		domain   *ngrokv1.Domain
		expected bool
	}{
		{
			name: "domain with no ID",
			domain: &ngrokv1.Domain{
				Status: ngrokv1.DomainStatus{
					ID: "",
				},
			},
			expected: false,
		},
		{
			name: "domain with ID but no Ready condition",
			domain: &ngrokv1.Domain{
				Status: ngrokv1.DomainStatus{
					ID:         "rd_123",
					Conditions: []metav1.Condition{},
				},
			},
			expected: false,
		},
		{
			// A wildcard-covered domain holds no reservation of its own, so an
			// empty ID is its steady state rather than a failure.
			name: "wildcard-covered domain with no ID but Ready=True",
			domain: &ngrokv1.Domain{
				Status: ngrokv1.DomainStatus{
					ID:                      "",
					CoveredByWildcardDomain: &ngrokv1.DomainStatusWildcardDomain{ID: "rd_wildcard", Domain: "*.example.com"},
					Conditions: []metav1.Condition{{
						Type:   ConditionReady,
						Status: metav1.ConditionTrue,
					}},
				},
			},
			expected: true,
		},
		{
			name: "wildcard-covered domain with Ready=False",
			domain: &ngrokv1.Domain{
				Status: ngrokv1.DomainStatus{
					ID:                      "",
					CoveredByWildcardDomain: &ngrokv1.DomainStatusWildcardDomain{ID: "rd_wildcard", Domain: "*.example.com"},
					Conditions: []metav1.Condition{{
						Type:   ConditionReady,
						Status: metav1.ConditionFalse,
					}},
				},
			},
			expected: false,
		},
		{
			name: "wildcard-covered domain with no Ready condition",
			domain: &ngrokv1.Domain{
				Status: ngrokv1.DomainStatus{
					ID:                      "",
					CoveredByWildcardDomain: &ngrokv1.DomainStatusWildcardDomain{ID: "rd_wildcard", Domain: "*.example.com"},
				},
			},
			expected: false,
		},
		{
			name: "domain with ID and Ready condition false",
			domain: &ngrokv1.Domain{
				Status: ngrokv1.DomainStatus{
					ID: "rd_123",
					Conditions: []metav1.Condition{
						{
							Type:   ConditionReady,
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
			domain: &ngrokv1.Domain{
				Status: ngrokv1.DomainStatus{
					ID: "rd_123",
					Conditions: []metav1.Condition{
						{
							Type:   ConditionReady,
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
			result := IsReady(FromV1(tt.domain))
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestUpdateDomainConditions_CoveredByWildcard_NgrokManaged(t *testing.T) {
	domain := createTestDomainCoveredByWildcard("test-domain", "a.example.ngrok.app", "*.example.ngrok.app")
	// An ngrok-managed wildcard: nil CertificateManagementPolicy means ngrok
	// handles DNS and certificates.
	wildcard := &ngrok.ReservedDomain{ID: "rd_wildcard", Domain: "*.example.ngrok.app"}

	UpdateConditions(FromV1(domain), wildcard, nil)

	// The empty status.ID must not be mistaken for "could not be reserved".
	created := meta.FindStatusCondition(domain.Status.Conditions, ConditionDomainCreated)
	require.NotNil(t, created)
	assert.Equal(t, metav1.ConditionTrue, created.Status)
	assert.Equal(t, ReasonCoveredByWildcardDomain, created.Reason)
	assert.Contains(t, created.Message, "*.example.ngrok.app")

	ready := meta.FindStatusCondition(domain.Status.Conditions, ConditionReady)
	require.NotNil(t, ready)
	assert.Equal(t, metav1.ConditionTrue, ready.Status)
	assert.Equal(t, ReasonCoveredByWildcardDomain, ready.Reason)
	assert.Contains(t, ready.Message, "*.example.ngrok.app")

	assert.True(t, IsReady(FromV1(domain)))
}

func TestUpdateDomainConditions_CoveredByWildcard_CustomWithCertificate(t *testing.T) {
	domain := createTestDomainCoveredByWildcard("test-domain", "a.custom.xyz", "*.custom.xyz")
	domain.Status.Certificate = &ngrokv1.DomainStatusCertificateInfo{ID: "cert_123"}
	wildcard := &ngrok.ReservedDomain{
		ID:                          "rd_wildcard",
		Domain:                      "*.custom.xyz",
		CertificateManagementPolicy: &ngrok.ReservedDomainCertPolicy{Authority: "letsencrypt"},
	}

	UpdateConditions(FromV1(domain), wildcard, nil)

	ready := meta.FindStatusCondition(domain.Status.Conditions, ConditionReady)
	require.NotNil(t, ready)
	assert.Equal(t, metav1.ConditionTrue, ready.Status)
	assert.True(t, IsReady(FromV1(domain)))
}

// A custom wildcard whose certificate has not provisioned cannot actually serve
// its children, so a covered child must not report Ready. This is what stops
// endpoints from coming up and failing TLS at request time.
func TestUpdateDomainConditions_CoveredByWildcard_CustomStillProvisioning(t *testing.T) {
	domain := createTestDomainCoveredByWildcard("test-domain", "a.custom.xyz", "*.custom.xyz")
	domain.Status.CertificateManagementStatus = &ngrokv1.DomainStatusCertificateManagementStatus{
		ProvisioningJob: &ngrokv1.DomainStatusProvisioningJob{
			Message: "provisioning certificate",
		},
	}
	wildcard := &ngrok.ReservedDomain{
		ID:                          "rd_wildcard",
		Domain:                      "*.custom.xyz",
		CertificateManagementPolicy: &ngrok.ReservedDomainCertPolicy{Authority: "letsencrypt"},
	}

	UpdateConditions(FromV1(domain), wildcard, nil)

	ready := meta.FindStatusCondition(domain.Status.Conditions, ConditionReady)
	require.NotNil(t, ready)
	assert.Equal(t, metav1.ConditionFalse, ready.Status)
	assert.Equal(t, ReasonProvisioningError, ready.Reason)
	assert.Contains(t, ready.Message, "provisioning certificate")

	assert.False(t, IsReady(FromV1(domain)))
}

// Guards against relaxing the empty-ID check too far: without wildcard
// coverage, an empty status.ID still means the domain could not be reserved.
func TestUpdateDomainConditions_EmptyIDWithoutWildcardStillInvalid(t *testing.T) {
	domain := createTestDomain("test-domain", "a.example.com", "")

	UpdateConditions(FromV1(domain), nil, nil)

	ready := meta.FindStatusCondition(domain.Status.Conditions, ConditionReady)
	require.NotNil(t, ready)
	assert.Equal(t, metav1.ConditionFalse, ready.Status)
	assert.Equal(t, ReasonDomainInvalid, ready.Reason)
	assert.False(t, IsReady(FromV1(domain)))
}

// Creation errors keep precedence over wildcard coverage.
func TestUpdateDomainConditions_CreateErrorWinsOverWildcard(t *testing.T) {
	domain := createTestDomainCoveredByWildcard("test-domain", "a.custom.xyz", "*.custom.xyz")

	UpdateConditions(FromV1(domain), nil, errors.New("boom"))

	ready := meta.FindStatusCondition(domain.Status.Conditions, ConditionReady)
	require.NotNil(t, ready)
	assert.Equal(t, metav1.ConditionFalse, ready.Status)
	assert.Equal(t, ReasonDomainCreationFailed, ready.Reason)
}
