package managerdriver

import (
	"testing"

	"github.com/stretchr/testify/assert"
	netv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"

	ingressv1alpha1 "github.com/ngrok/ngrok-operator/api/ingress/v1alpha1"
	"github.com/ngrok/ngrok-operator/internal/testutils"
)

func TestIngressToDomains_SkipsInternalDomains(t *testing.T) {
	tests := []struct {
		name            string
		hosts           []string
		expectedDomains []string
	}{
		{
			name:            "regular domain is included",
			hosts:           []string{"example.com"},
			expectedDomains: []string{"example.com"},
		},
		{
			name:            "internal domain is skipped",
			hosts:           []string{"foo.internal"},
			expectedDomains: []string{},
		},
		{
			name:            "mixed domains - only non-internal included",
			hosts:           []string{"example.com", "foo.internal", "bar.ngrok.io"},
			expectedDomains: []string{"example.com", "bar.ngrok.io"},
		},
		{
			name:            "subdomain internal is skipped",
			hosts:           []string{"service.namespace.internal"},
			expectedDomains: []string{},
		},
		{
			name:            "internal as subdomain is NOT skipped",
			hosts:           []string{"internal.example.com"},
			expectedDomains: []string{"internal.example.com"},
		},
		{
			name:            "uppercase internal is skipped",
			hosts:           []string{"FOO.INTERNAL"},
			expectedDomains: []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ingress := testutils.NewTestIngressV1WithHosts("test-ingress", "test-namespace", tt.hosts...)
			result := ingressToDomains(ingress, "", nil)

			assert.Len(t, result, len(tt.expectedDomains))
			for _, expectedDomain := range tt.expectedDomains {
				_, found := result[expectedDomain]
				assert.True(t, found, "expected domain %s to be in result", expectedDomain)
			}
		})
	}
}

func TestGatewayToDomains_SkipsInternalDomains(t *testing.T) {
	tests := []struct {
		name            string
		hostnames       []string
		expectedDomains []string
	}{
		{
			name:            "regular domain is included",
			hostnames:       []string{"example.com"},
			expectedDomains: []string{"example.com"},
		},
		{
			name:            "internal domain is skipped",
			hostnames:       []string{"foo.internal"},
			expectedDomains: []string{},
		},
		{
			name:            "mixed domains - only non-internal included",
			hostnames:       []string{"example.com", "foo.internal", "bar.ngrok.io"},
			expectedDomains: []string{"example.com", "bar.ngrok.io"},
		},
		{
			name:            "subdomain internal is skipped",
			hostnames:       []string{"service.namespace.internal"},
			expectedDomains: []string{},
		},
		{
			name:            "internal as subdomain is NOT skipped",
			hostnames:       []string{"internal.example.com"},
			expectedDomains: []string{"internal.example.com"},
		},
		{
			name:            "uppercase internal is skipped",
			hostnames:       []string{"FOO.INTERNAL"},
			expectedDomains: []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gateway := testutils.NewGatewayWithHostnames("test-gateway", "test-namespace", tt.hostnames...)
			result := gatewayToDomains(gateway, "", nil)

			assert.Len(t, result, len(tt.expectedDomains))
			for _, expectedDomain := range tt.expectedDomains {
				_, found := result[expectedDomain]
				assert.True(t, found, "expected domain %s to be in result", expectedDomain)
			}
		})
	}
}

func TestIngressToDomains_SkipsExistingDomains(t *testing.T) {
	ingress := testutils.NewTestIngressV1WithHosts("test-ingress", "test-namespace", "example.com", "new.example.com")
	existingDomains := map[string]ingressv1alpha1.Domain{
		"example.com": {
			ObjectMeta: metav1.ObjectMeta{Name: "example-com"},
			Spec:       ingressv1alpha1.DomainSpec{Domain: "example.com"},
		},
	}

	result := ingressToDomains(ingress, "", existingDomains)

	assert.Len(t, result, 1)
	_, found := result["new.example.com"]
	assert.True(t, found, "expected new.example.com to be in result")
	_, found = result["example.com"]
	assert.False(t, found, "expected example.com to be skipped as it exists")
}

func TestGatewayToDomains_SkipsExistingDomains(t *testing.T) {
	gateway := testutils.NewGatewayWithHostnames("test-gateway", "test-namespace", "example.com", "new.example.com")
	existingDomains := map[string]ingressv1alpha1.Domain{
		"example.com": {
			ObjectMeta: metav1.ObjectMeta{Name: "example-com"},
			Spec:       ingressv1alpha1.DomainSpec{Domain: "example.com"},
		},
	}

	result := gatewayToDomains(gateway, "", existingDomains)

	assert.Len(t, result, 1)
	_, found := result["new.example.com"]
	assert.True(t, found, "expected new.example.com to be in result")
	_, found = result["example.com"]
	assert.False(t, found, "expected example.com to be skipped as it exists")
}

func TestIngressToDomains_SkipsEmptyHost(t *testing.T) {
	ingress := &netv1.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-ingress",
			Namespace: "test-namespace",
		},
		Spec: netv1.IngressSpec{
			Rules: []netv1.IngressRule{
				{Host: ""},
				{Host: "example.com"},
			},
		},
	}

	result := ingressToDomains(ingress, "", nil)

	assert.Len(t, result, 1)
	_, found := result["example.com"]
	assert.True(t, found)
}

func TestGatewayToDomains_SkipsNilHostname(t *testing.T) {
	gateway := &gatewayv1.Gateway{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-gateway",
			Namespace: "test-namespace",
		},
		Spec: gatewayv1.GatewaySpec{
			Listeners: []gatewayv1.Listener{
				{Name: "no-hostname", Hostname: nil},
				{Name: "with-hostname", Hostname: ptr.To(gatewayv1.Hostname("example.com"))},
			},
		},
	}

	result := gatewayToDomains(gateway, "", nil)

	assert.Len(t, result, 1)
	_, found := result["example.com"]
	assert.True(t, found)
}
