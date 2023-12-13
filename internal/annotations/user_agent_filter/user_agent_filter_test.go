package user_agent_filter

import (
	"testing"

	ingressv1alpha1 "github.com/ngrok/kubernetes-ingress-controller/api/v1alpha1"
	"github.com/ngrok/kubernetes-ingress-controller/internal/annotations/parser"
	"github.com/ngrok/kubernetes-ingress-controller/internal/annotations/testutil"
	"github.com/ngrok/kubernetes-ingress-controller/internal/errors"
	"github.com/stretchr/testify/assert"
)

func TestUserAgentFilterWhenNotSupplied(t *testing.T) {
	ing := testutil.NewIngress()
	ing.SetAnnotations(map[string]string{})
	parsed, err := NewParser().Parse(ing)

	assert.Nil(t, parsed)
	assert.Error(t, err)
	assert.True(t, errors.IsMissingAnnotations(err))
}

func TestUserAgentFilterWhenAnnotationsAreProvided(t *testing.T) {
	ing := testutil.NewIngress()
	annotations := map[string]string{}
	annotations[parser.GetAnnotationWithPrefix("user-agent-filter-allow")] = `(foo)/(\d)+.(\d)+`
	annotations[parser.GetAnnotationWithPrefix("user-agent-filter-allow")] = `(bar)/(\d)+.(\d)+`
	ing.SetAnnotations(annotations)

	parsed, err := NewParser().Parse(ing)
	assert.NoError(t, err)
	assert.NotNil(t, parsed)

	userAgentFilter, ok := parsed.(*ingressv1alpha1.EndpointUserAgentFilter)
	if !ok {
		t.Fatalf("expected *ingressv1alpha1.UserAgentFilter, got %T", parsed)
	}

	assert.Equal(t, []string{`(foo)/(\d)+.(\d)+`}, userAgentFilter.Allow)
	assert.Equal(t, []string{`(bar)/(\d)+.(\d)+`}, userAgentFilter.Deny)

}

func TestUserAgentFilterWhenAnnotationsAreOnlyAllow(t *testing.T) {
	ing := testutil.NewIngress()
	annotations := map[string]string{}
	annotations_allow := parser.GetAnnotationWithPrefix("user-agent-filter-allow")
	annotations[annotations_allow] = `(foo)/(\d)+.(\d)+`
	annotations[annotations_allow] = annotations[annotations_allow] + `(foo)/(\d)+.(\d)+`
	ing.SetAnnotations(annotations)

	parsed, err := NewParser().Parse(ing)
	assert.NoError(t, err)
	assert.NotNil(t, parsed)

	userAgentFilter, ok := parsed.(*ingressv1alpha1.EndpointUserAgentFilter)
	if !ok {
		t.Fatalf("expected *ingressv1alpha1.UserAgentFilter, got %T", parsed)
	}

	assert.Equal(t, []string{`(foo)/(\d)+.(\d)+`, `(foo)/(\d)+.(\d)+`}, userAgentFilter.Allow)
	assert.Equal(t, []string{}, userAgentFilter.Deny)

}

func TestUserAgentFilterWhenAnnotationsAreOnlyDeny(t *testing.T) {
	ing := testutil.NewIngress()
	annotations := map[string]string{}
	annotations_deny := parser.GetAnnotationWithPrefix("user-agent-filter-deny")
	annotations[annotations_deny] = `(foo)/(\d)+.(\d)+`
	annotations[annotations_deny] = annotations[annotations_deny] + `(foo)/(\d)+.(\d)+`
	ing.SetAnnotations(annotations)

	parsed, err := NewParser().Parse(ing)
	assert.NoError(t, err)
	assert.NotNil(t, parsed)

	userAgentFilter, ok := parsed.(*ingressv1alpha1.EndpointUserAgentFilter)
	if !ok {
		t.Fatalf("expected *ingressv1alpha1.UserAgentFilter, got %T", parsed)
	}

	assert.Equal(t, []string{`(foo)/(\d)+.(\d)+`, `(foo)/(\d)+.(\d)+`}, userAgentFilter.Deny)
	assert.Equal(t, []string{}, userAgentFilter.Allow)

}
