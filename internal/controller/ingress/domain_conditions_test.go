package ingress

import (
	"testing"

	"github.com/ngrok/ngrok-api-go/v7"
	"github.com/stretchr/testify/assert"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	ingressv1alpha1 "github.com/ngrok/ngrok-operator/api/ingress/v1alpha1"
)

func TestUpdateDomainConditions(t *testing.T) {
	tests := []struct {
		name           string
		domain         *ingressv1alpha1.Domain
		ngrokDomain    *ngrok.ReservedDomain
		expectedReady  bool
		expectedReason string
	}{
		{
			name: "ngrok subdomain should be ready immediately",
			domain: &ingressv1alpha1.Domain{
				ObjectMeta: metav1.ObjectMeta{
					Name:       "test-domain",
					Generation: 1,
				},
				Spec: ingressv1alpha1.DomainSpec{
					Domain: "test.ngrok.app",
				},
				Status: ingressv1alpha1.DomainStatus{
					ID: "rd_123",
				},
			},
			ngrokDomain: &ngrok.ReservedDomain{
				ID:     "rd_123",
				Domain: "test.ngrok.app",
			},
			expectedReady:  true,
			expectedReason: ReasonDomainActive,
		},
		{
			name: "custom domain should be provisioning initially",
			domain: &ingressv1alpha1.Domain{
				ObjectMeta: metav1.ObjectMeta{
					Name:       "test-custom-domain",
					Generation: 1,
				},
				Spec: ingressv1alpha1.DomainSpec{
					Domain: "test.example.com",
				},
				Status: ingressv1alpha1.DomainStatus{
					ID: "rd_456",
				},
			},
			ngrokDomain: &ngrok.ReservedDomain{
				ID:                          "rd_456",
				Domain:                      "test.example.com",
				CertificateManagementPolicy: &ngrok.ReservedDomainCertPolicy{Authority: "letsencrypt"},
			},
			expectedReady:  false,
			expectedReason: ReasonProvisioningError,
		},
		{
			name: "custom domain with provisioned certificate should be ready",
			domain: &ingressv1alpha1.Domain{
				ObjectMeta: metav1.ObjectMeta{
					Name:       "test-custom-ready",
					Generation: 1,
				},
				Spec: ingressv1alpha1.DomainSpec{
					Domain: "ready.example.com",
				},
				Status: ingressv1alpha1.DomainStatus{
					ID: "rd_789",
				},
			},
			ngrokDomain: &ngrok.ReservedDomain{
				ID:     "rd_789",
				Domain: "ready.example.com",
				Certificate: &ngrok.Ref{
					ID:  "cert_123",
					URI: "https://api.ngrok.com/tls_certificates/cert_123",
				},
			},
			expectedReady:  true,
			expectedReason: ReasonDomainActive,
		},
		{
			name: "domain without ID should not be ready",
			domain: &ingressv1alpha1.Domain{
				ObjectMeta: metav1.ObjectMeta{
					Name:       "test-failed-domain",
					Generation: 1,
				},
				Spec: ingressv1alpha1.DomainSpec{
					Domain: "invalid.domain",
				},
				Status: ingressv1alpha1.DomainStatus{},
			},
			ngrokDomain:    nil,
			expectedReady:  false,
			expectedReason: ReasonDomainInvalid,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			updateDomainConditions(tt.domain, tt.ngrokDomain, nil)

			// Check Ready condition
			readyCondition := meta.FindStatusCondition(tt.domain.Status.Conditions, ConditionDomainReady)
			assert.NotNil(t, readyCondition, "Ready condition should be set")

			if tt.expectedReady {
				assert.Equal(t, metav1.ConditionTrue, readyCondition.Status, "Domain should be ready")
			} else {
				assert.Equal(t, metav1.ConditionFalse, readyCondition.Status, "Domain should not be ready")
			}

			assert.Equal(t, tt.expectedReason, readyCondition.Reason, "Ready condition reason should match")

			// Check DomainCreated condition for domains with ID
			if tt.domain.Status.ID != "" {
				createdCondition := meta.FindStatusCondition(tt.domain.Status.Conditions, ConditionDomainCreated)
				assert.NotNil(t, createdCondition, "DomainCreated condition should be set")
				assert.Equal(t, metav1.ConditionTrue, createdCondition.Status, "DomainCreated should be true")
			}
		})
	}
}

func TestSetDomainCreationFailedConditions(t *testing.T) {
	tests := []struct {
		name     string
		errorMsg string
	}{
		{
			name:     "dangling DNS record error",
			errorMsg: "The domain 'ngrok.com' has a dangling A, AAAA, ALIAS or other record pointing to ngrok",
		},
		{
			name:     "protected domain error",
			errorMsg: "This domain is already reserved for another account",
		},
		{
			name:     "generic creation error",
			errorMsg: "Some other API error occurred",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			domain := &ingressv1alpha1.Domain{
				ObjectMeta: metav1.ObjectMeta{
					Name:       "test-failed-domain",
					Generation: 1,
				},
				Spec: ingressv1alpha1.DomainSpec{
					Domain: "test.example.com",
				},
			}

			// err := fmt.Errorf("%s", tt.errorMsg)
			// setDomainCreationFailedConditions(domain, domain.Status.ID, err)

			// Check that all conditions are set to False with ReasonDomainCreationFailed
			readyCondition := meta.FindStatusCondition(domain.Status.Conditions, ConditionDomainReady)
			assert.NotNil(t, readyCondition)
			assert.Equal(t, metav1.ConditionFalse, readyCondition.Status)
			assert.Equal(t, ReasonDomainCreationFailed, readyCondition.Reason)
			assert.Contains(t, readyCondition.Message, tt.errorMsg)

			createdCondition := meta.FindStatusCondition(domain.Status.Conditions, ConditionDomainCreated)
			assert.NotNil(t, createdCondition)
			assert.Equal(t, metav1.ConditionFalse, createdCondition.Status)
			assert.Equal(t, ReasonDomainCreationFailed, createdCondition.Reason)

			certificateCondition := meta.FindStatusCondition(domain.Status.Conditions, ConditionCertificateReady)
			assert.NotNil(t, certificateCondition)
			assert.Equal(t, metav1.ConditionFalse, certificateCondition.Status)
			assert.Equal(t, ReasonDomainCreationFailed, certificateCondition.Reason)
			assert.Equal(t, "Domain creation failed", certificateCondition.Message)

			dnsCondition := meta.FindStatusCondition(domain.Status.Conditions, ConditionDNSConfigured)
			assert.NotNil(t, dnsCondition)
			assert.Equal(t, metav1.ConditionFalse, dnsCondition.Status)
			assert.Equal(t, ReasonDomainCreationFailed, dnsCondition.Reason)
			assert.Equal(t, "Domain creation failed", dnsCondition.Message)
		})
	}
}

func TestNeedsStatusFollowUp(t *testing.T) {
	tests := []struct {
		name     string
		domain   *ingressv1alpha1.Domain
		expected bool
		reason   string
	}{
		{
			name: "domain with no conditions - should not follow up",
			domain: &ingressv1alpha1.Domain{
				Status: ingressv1alpha1.DomainStatus{
					Conditions: []metav1.Condition{},
				},
			},
			expected: false,
			reason:   "no conditions to evaluate",
		},
		{
			name: "domain creation failed - no follow up",
			domain: &ingressv1alpha1.Domain{
				Status: ingressv1alpha1.DomainStatus{
					Conditions: []metav1.Condition{
						{
							Type:   ConditionDomainCreated,
							Status: metav1.ConditionFalse,
							Reason: ReasonDomainCreationFailed,
						},
					},
				},
			},
			expected: false,
			reason:   "domain creation failed - terminal state",
		},
		{
			name: "domain ready - no follow up",
			domain: &ingressv1alpha1.Domain{
				Status: ingressv1alpha1.DomainStatus{
					Conditions: []metav1.Condition{
						{
							Type:   ConditionDomainReady,
							Status: metav1.ConditionTrue,
							Reason: ReasonDomainActive,
						},
					},
				},
			},
			expected: false,
			reason:   "domain is ready",
		},
		{
			name: "certificate not ready due to DNS error - should follow up",
			domain: &ingressv1alpha1.Domain{
				Status: ingressv1alpha1.DomainStatus{
					Conditions: []metav1.Condition{
						{
							Type:   ConditionCertificateReady,
							Status: metav1.ConditionFalse,
							Reason: ReasonDNSError,
						},
					},
				},
			},
			expected: true,
			reason:   "certificate not ready due to DNS error",
		},
		{
			name: "certificate not ready due to domain creation failure - no follow up",
			domain: &ingressv1alpha1.Domain{
				Status: ingressv1alpha1.DomainStatus{
					Conditions: []metav1.Condition{
						{
							Type:   ConditionCertificateReady,
							Status: metav1.ConditionFalse,
							Reason: ReasonDomainCreationFailed,
						},
					},
				},
			},
			expected: false,
			reason:   "certificate not ready due to terminal domain creation failure",
		},
		{
			name: "DNS not configured due to provisioning error - should follow up",
			domain: &ingressv1alpha1.Domain{
				Status: ingressv1alpha1.DomainStatus{
					Conditions: []metav1.Condition{
						{
							Type:   ConditionDNSConfigured,
							Status: metav1.ConditionFalse,
							Reason: ReasonProvisioningError,
						},
					},
				},
			},
			expected: true,
			reason:   "DNS not configured due to provisioning error",
		},
		{
			name: "DNS not configured due to invalid domain - no follow up",
			domain: &ingressv1alpha1.Domain{
				Status: ingressv1alpha1.DomainStatus{
					Conditions: []metav1.Condition{
						{
							Type:   ConditionDNSConfigured,
							Status: metav1.ConditionFalse,
							Reason: ReasonDomainInvalid,
						},
					},
				},
			},
			expected: false,
			reason:   "DNS not configured due to invalid domain - terminal state",
		},
		{
			name: "certificate provisioning in progress - should follow up",
			domain: &ingressv1alpha1.Domain{
				Status: ingressv1alpha1.DomainStatus{
					Conditions: []metav1.Condition{
						{
							Type:   ConditionDomainCreated,
							Status: metav1.ConditionTrue,
							Reason: ReasonDomainCreated,
						},
						{
							Type:   ConditionCertificateReady,
							Status: metav1.ConditionFalse,
							Reason: ReasonCertificateProvisioning,
						},
						{
							Type:   ConditionDomainReady,
							Status: metav1.ConditionFalse,
							Reason: ReasonWaitingForCertificate,
						},
					},
				},
			},
			expected: true,
			reason:   "certificate still provisioning",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// result := needsStatusFollowUp(tt.domain)
			// assert.Equal(t, tt.expected, result, "needsStatusFollowUp should return %v for %s", tt.expected, tt.reason)
		})
	}
}
