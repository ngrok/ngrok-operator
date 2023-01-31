package webhook_verification

import (
	"testing"

	ingressv1alpha1 "github.com/ngrok/kubernetes-ingress-controller/api/v1alpha1"
	"github.com/ngrok/kubernetes-ingress-controller/internal/annotations/parser"
	"github.com/ngrok/kubernetes-ingress-controller/internal/annotations/testutil"
	"github.com/ngrok/kubernetes-ingress-controller/internal/errors"
	"github.com/stretchr/testify/assert"
)

func TestWebhookVerificationWhenNotSupplied(t *testing.T) {
	ing := testutil.NewIngress()
	ing.SetAnnotations(map[string]string{})
	parsed, err := NewParser().Parse(ing)

	assert.Nil(t, parsed)
	assert.Error(t, err)
	assert.True(t, errors.IsMissingAnnotations(err))
}

func TestWebhookVerificationWhenSecretRefDataNotSupplied(t *testing.T) {
	ing := testutil.NewIngress()
	annotations := map[string]string{}
	annotations[parser.GetAnnotationWithPrefix("webhook-verification-provider")] = "github"
	ing.SetAnnotations(annotations)

	parsed, err := NewParser().Parse(ing)
	assert.Nil(t, parsed)
	assert.Error(t, err)
	assert.True(t, errors.IsMissingAnnotations(err))
}

func TestWebhookVerificationWhenSecretRefNameNotSupplied(t *testing.T) {
	ing := testutil.NewIngress()
	annotations := map[string]string{}
	annotations[parser.GetAnnotationWithPrefix("webhook-verification-provider")] = "github"
	annotations[parser.GetAnnotationWithPrefix("webhook-verification-secret-name")] = "my-webhook-secret"
	ing.SetAnnotations(annotations)

	parsed, err := NewParser().Parse(ing)
	assert.Nil(t, parsed)
	assert.Error(t, err)
	assert.True(t, errors.IsMissingAnnotations(err))
}

func TestWebhookVerificationWhenSecretRefKeyNotSupplied(t *testing.T) {
	ing := testutil.NewIngress()
	annotations := map[string]string{}
	annotations[parser.GetAnnotationWithPrefix("webhook-verification-provider")] = "github"
	annotations[parser.GetAnnotationWithPrefix("webhook-verification-secret-key")] = "SECRET_TOKEN"
	ing.SetAnnotations(annotations)

	parsed, err := NewParser().Parse(ing)
	assert.Nil(t, parsed)
	assert.Error(t, err)
	assert.True(t, errors.IsMissingAnnotations(err))
}

func TestWebhookVerificationWhenAnnotationsAreProvided(t *testing.T) {
	ing := testutil.NewIngress()
	annotations := map[string]string{}
	annotations[parser.GetAnnotationWithPrefix("webhook-verification-provider")] = "github"
	annotations[parser.GetAnnotationWithPrefix("webhook-verification-secret-name")] = "my-webhook-secret"
	annotations[parser.GetAnnotationWithPrefix("webhook-verification-secret-key")] = "SECRET_TOKEN"
	ing.SetAnnotations(annotations)

	parsed, err := NewParser().Parse(ing)
	assert.NoError(t, err)
	assert.NotNil(t, parsed)

	webhookVerification, ok := parsed.(*ingressv1alpha1.EndpointWebhookVerification)
	if !ok {
		t.Fatalf("expected *ingressv1alpha1.WebhookVerification, got %T", parsed)
	}

	assert.Equal(t, "github", webhookVerification.Provider)
	assert.Equal(t, "my-webhook-secret", webhookVerification.SecretRef.Name)
	assert.Equal(t, "SECRET_TOKEN", webhookVerification.SecretRef.Key)

}
