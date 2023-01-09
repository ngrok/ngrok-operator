package annotations

import (
	"testing"

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
	assert.False(t, *modules.Compression.Enabled)

	modules = e.Extract(newIngressWithAnnotations(map[string]string{
		"k8s.ngrok.com/https-compression": "true",
	}))
	assert.True(t, *modules.Compression.Enabled)

	modules = e.Extract(newIngressWithAnnotations(map[string]string{}))
	assert.Nil(t, modules.Compression)
}
