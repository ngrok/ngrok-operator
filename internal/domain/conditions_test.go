package domain

import (
	"testing"

	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	ingressv1alpha1 "github.com/ngrok/ngrok-operator/api/ingress/v1alpha1"
)

func TestIsInternalDomain(t *testing.T) {
	tests := []struct {
		name     string
		host     string
		expected bool
	}{
		{"simple internal domain", "foo.internal", true},
		{"subdomain internal", "bar.foo.internal", true},
		{"uppercase internal", "FOO.INTERNAL", true},
		{"mixed case internal", "Foo.Internal", true},
		{"trailing dot internal", "foo.internal.", true},
		{"with spaces", "  foo.internal  ", true},
		{"internal as subdomain - not internal TLD", "foo.internal.example.com", false},
		{"regular domain", "example.com", false},
		{"ngrok domain", "app.ngrok.io", false},
		{"empty string", "", false},
		{"just internal", "internal", false},
		{"dot internal only", ".internal", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := IsInternalDomain(tt.host)
			assert.Equal(t, tt.expected, result, "IsInternalDomain(%q)", tt.host)
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
			name: "domain with ID and Ready condition true",
			domain: &ingressv1alpha1.Domain{
				Status: ingressv1alpha1.DomainStatus{
					ID: "rd_123",
					Conditions: []metav1.Condition{
						{
							Type:   ConditionDomainReady,
							Status: metav1.ConditionTrue,
						},
					},
				},
			},
			expected: true,
		},
		{
			name: "domain with ID but Ready condition false",
			domain: &ingressv1alpha1.Domain{
				Status: ingressv1alpha1.DomainStatus{
					ID: "rd_123",
					Conditions: []metav1.Condition{
						{
							Type:   ConditionDomainReady,
							Status: metav1.ConditionFalse,
						},
					},
				},
			},
			expected: false,
		},
		{
			name: "domain with ID but no Ready condition",
			domain: &ingressv1alpha1.Domain{
				Status: ingressv1alpha1.DomainStatus{
					ID: "rd_123",
				},
			},
			expected: false,
		},
		{
			name: "domain without ID",
			domain: &ingressv1alpha1.Domain{
				Status: ingressv1alpha1.DomainStatus{},
			},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := IsDomainReady(tt.domain)
			assert.Equal(t, tt.expected, result)
		})
	}
}
