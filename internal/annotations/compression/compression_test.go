package compression

import (
	"testing"

	ingressv1alpha1 "github.com/ngrok/kubernetes-ingress-controller/api/v1alpha1"
	"github.com/ngrok/kubernetes-ingress-controller/internal/annotations/parser"
	"github.com/stretchr/testify/assert"
	v1 "k8s.io/api/core/v1"
	networking "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func createIngressWithAnnotations(annotations map[string]string) *networking.Ingress {
	return &networking.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Name:        "test-ingress",
			Namespace:   v1.NamespaceDefault,
			Annotations: annotations,
		},
	}
}

func TestCompressionWhenNotSupplied(t *testing.T) {
	ing := createIngressWithAnnotations(map[string]string{})
	parsed, err := NewParser().Parse(ing)

	assert.Nil(t, parsed)
	assert.Error(t, err)
}

func TestCompressionWhenSuppliedAndTrue(t *testing.T) {
	ing := createIngressWithAnnotations(map[string]string{
		parser.GetAnnotationWithPrefix("https-compression"): "true",
	})
	parsed, err := NewParser().Parse(ing)
	assert.NoError(t, err)

	compression, ok := parsed.(*ingressv1alpha1.EndpointCompression)
	if !ok {
		t.Fatalf("expected *ingressv1alpha1.EndpointCompression, got %T", parsed)
	}
	assert.Equal(t, true, compression.Enabled)
}

func TestCompressionWhenSuppliedAndFalse(t *testing.T) {
	ing := createIngressWithAnnotations(map[string]string{
		parser.GetAnnotationWithPrefix("https-compression"): "false",
	})
	parsed, err := NewParser().Parse(ing)
	assert.NoError(t, err)

	compression, ok := parsed.(*ingressv1alpha1.EndpointCompression)
	if !ok {
		t.Fatalf("expected *ingressv1alpha1.EndpointCompression, got %T", parsed)
	}
	assert.Equal(t, false, compression.Enabled)
}
