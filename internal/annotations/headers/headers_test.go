package headers

import (
	"testing"

	"github.com/ngrok/kubernetes-ingress-controller/internal/annotations/parser"
	"github.com/ngrok/kubernetes-ingress-controller/internal/annotations/testutil"
	"github.com/ngrok/kubernetes-ingress-controller/internal/errors"
	"github.com/stretchr/testify/assert"
)

func TestHeadersWhenNotSupplied(t *testing.T) {
	ing := testutil.NewIngress()
	ing.SetAnnotations(map[string]string{})
	parsed, err := NewParser().Parse(ing)

	assert.Nil(t, parsed)
	assert.Error(t, err)
	assert.True(t, errors.IsMissingAnnotations(err))
}

func TestHeadersWhenRequestHeadersSupplied(t *testing.T) {
	ing := testutil.NewIngress()
	annotations := map[string]string{}
	annotations[parser.GetAnnotationWithPrefix("request-headers-remove")] = "Server"
	annotations[parser.GetAnnotationWithPrefix("request-headers-add")] = `{"X-Request-Header": "value"}`
	ing.SetAnnotations(annotations)

	parsed, err := NewParser().Parse(ing)
	assert.NoError(t, err)
	assert.NotNil(t, parsed)

	endpointHeaders, ok := parsed.(*EndpointHeaders)
	if !ok {
		t.Fatalf("expected *EndpointHeaders, got %T", parsed)
	}

	assert.Nil(t, endpointHeaders.Response)
	assert.Equal(t, []string{"Server"}, endpointHeaders.Request.Remove)
	assert.Equal(t, map[string]string{"X-Request-Header": "value"}, endpointHeaders.Request.Add)
}

func TestHeadersWhenResponseHeadersSupplied(t *testing.T) {
	ing := testutil.NewIngress()
	annotations := map[string]string{}
	annotations[parser.GetAnnotationWithPrefix("response-headers-remove")] = "Server"
	annotations[parser.GetAnnotationWithPrefix("response-headers-add")] = `{"X-Response-Header": "value"}`
	ing.SetAnnotations(annotations)

	parsed, err := NewParser().Parse(ing)
	assert.NoError(t, err)
	assert.NotNil(t, parsed)

	endpointHeaders, ok := parsed.(*EndpointHeaders)
	if !ok {
		t.Fatalf("expected *EndpointHeaders, got %T", parsed)
	}
	assert.Nil(t, endpointHeaders.Request)
	assert.Equal(t, []string{"Server"}, endpointHeaders.Response.Remove)
	assert.Equal(t, map[string]string{"X-Response-Header": "value"}, endpointHeaders.Response.Add)
}

func TestInvalidRequestHeadersAdd(t *testing.T) {
	ing := testutil.NewIngress()
	annotations := map[string]string{}
	// Not valid JSON
	annotations[parser.GetAnnotationWithPrefix("request-headers-add")] = `{X-Request-Header: value}`
	ing.SetAnnotations(annotations)

	_, err := NewParser().Parse(ing)
	assert.Error(t, err)
	assert.True(t, errors.IsInvalidContent(err))
}

func TestInvalidResponseHeadersAdd(t *testing.T) {
	ing := testutil.NewIngress()
	annotations := map[string]string{}
	// Not valid JSON
	annotations[parser.GetAnnotationWithPrefix("response-headers-add")] = `{X-Response-Header: value}`
	ing.SetAnnotations(annotations)

	_, err := NewParser().Parse(ing)
	assert.Error(t, err)
	assert.True(t, errors.IsInvalidContent(err))
}
