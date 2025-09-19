package ingress

import (
	"fmt"
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
				ID:     "rd_456",
				Domain: "test.example.com",
			},
			expectedReady:  false,
			expectedReason: ReasonWaitingForCertificate,
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
			updateDomainConditions(tt.domain, tt.ngrokDomain)

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
		name           string
		errorMsg       string
		expectedReason string
	}{
		{
			name:           "dangling DNS record error",
			errorMsg:       "The domain 'ngrok.com' has a dangling A, AAAA, ALIAS or other record pointing to ngrok",
			expectedReason: "DanglingDNSRecord",
		},
		{
			name:           "protected domain error",
			errorMsg:       "This domain is already reserved for another account",
			expectedReason: "ProtectedDomain",
		},
		{
			name:           "generic creation error",
			errorMsg:       "Some other API error occurred",
			expectedReason: "DomainCreationFailed",
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

			err := fmt.Errorf("%s", tt.errorMsg)
			setDomainCreationFailedConditions(domain, err)

			// Check that all conditions are set to False
			readyCondition := meta.FindStatusCondition(domain.Status.Conditions, ConditionDomainReady)
			assert.NotNil(t, readyCondition)
			assert.Equal(t, metav1.ConditionFalse, readyCondition.Status)
			assert.Equal(t, tt.expectedReason, readyCondition.Reason)
			assert.Contains(t, readyCondition.Message, tt.errorMsg)

			createdCondition := meta.FindStatusCondition(domain.Status.Conditions, ConditionDomainCreated)
			assert.NotNil(t, createdCondition)
			assert.Equal(t, metav1.ConditionFalse, createdCondition.Status)
			assert.Equal(t, tt.expectedReason, createdCondition.Reason)
		})
	}
}
