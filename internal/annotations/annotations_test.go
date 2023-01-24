package annotations

import (
	"testing"

	ingressv1alpha1 "github.com/ngrok/kubernetes-ingress-controller/api/v1alpha1"
	"github.com/ngrok/kubernetes-ingress-controller/internal/annotations/parser"
	"github.com/ngrok/kubernetes-ingress-controller/internal/annotations/testutil"
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

func TestParsesIPPolicies(t *testing.T) {
	e := NewAnnotationsExtractor()
	ing := testutil.NewIngress()
	ing.SetAnnotations(map[string]string{
		parser.GetAnnotationWithPrefix("ip-policy-ids"): "abc123,def456",
	})

	modules := e.Extract(ing)

	assert.Equal(t, &ingressv1alpha1.EndpointIPPolicy{
		IPPolicyIDs: []string{
			"abc123",
			"def456",
		},
	}, modules.IPRestriction)

	modules = e.Extract(newIngressWithAnnotations(map[string]string{}))
	assert.Nil(t, modules.IPRestriction)
}
