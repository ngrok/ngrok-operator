package annotations

import (
	"testing"

	ingressv1alpha1 "github.com/ngrok/kubernetes-ingress-controller/api/v1alpha1"
	"github.com/stretchr/testify/assert"
	networking "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func newIngressWithAnnotations(annotations map[string]string) *networking.Ingress {
	return &networking.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Annotations: annotations,
		},
	}
}

func TestCompression(t *testing.T) {
	e := NewAnnotationsExtractor()
	modules := e.Extract(newIngressWithAnnotations(map[string]string{
		"k8s.ngrok.com/https-compression": "false",
	}))
	assert.False(t, modules.Compression.Enabled)

	modules = e.Extract(newIngressWithAnnotations(map[string]string{
		"k8s.ngrok.com/https-compression": "true",
	}))
	assert.True(t, modules.Compression.Enabled)

	modules = e.Extract(newIngressWithAnnotations(map[string]string{}))
	assert.Nil(t, modules.Compression)
}

func TestIPPolicies(t *testing.T) {
	e := NewAnnotationsExtractor()
	modules := e.Extract(newIngressWithAnnotations(map[string]string{
		"k8s.ngrok.com/ip-policy-ids": "abc123,def456",
	}))
	assert.Equal(t, &ingressv1alpha1.EndpointIPPolicy{
		IPPolicyIDs: []string{
			"abc123",
			"def456",
		},
	}, modules.IPRestriction)

	modules = e.Extract(newIngressWithAnnotations(map[string]string{}))
	assert.Nil(t, modules.IPRestriction)
}
