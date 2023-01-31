package tls

import (
	"testing"

	"github.com/ngrok/kubernetes-ingress-controller/internal/annotations/parser"
	"github.com/ngrok/kubernetes-ingress-controller/internal/annotations/testutil"
	"github.com/ngrok/kubernetes-ingress-controller/internal/errors"
	"github.com/stretchr/testify/assert"
)

func TestTLSTerminationWhenNotSupplied(t *testing.T) {
	ing := testutil.NewIngress()
	ing.SetAnnotations(map[string]string{})
	parsed, err := NewParser().Parse(ing)

	assert.Nil(t, parsed)
	assert.Error(t, err)
	assert.True(t, errors.IsMissingAnnotations(err))
}

func TestTLSTerminationWhenSupplied(t *testing.T) {
	ing := testutil.NewIngress()
	annotations := map[string]string{}
	annotations[parser.GetAnnotationWithPrefix("tls-min-version")] = "1.3"
	ing.SetAnnotations(annotations)

	parsed, err := NewParser().Parse(ing)
	assert.NoError(t, err)

	tlsTermination, ok := parsed.(*EndpointTLSTerminationAtEdge)
	if !ok {
		t.Fatalf("expected *EndpointTLSTerminationAtEdge, got %T", parsed)
	}
	assert.Equal(t, "1.3", tlsTermination.MinVersion)
}
