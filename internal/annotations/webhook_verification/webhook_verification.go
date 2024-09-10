package webhook_verification

import (
	ingressv1alpha1 "github.com/ngrok/ngrok-operator/api/ingress/v1alpha1"
	"github.com/ngrok/ngrok-operator/internal/annotations/parser"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type EndpointWebhookVerification = ingressv1alpha1.EndpointWebhookVerification
type SecretKeyRef = ingressv1alpha1.SecretKeyRef

type webhookVerification struct{}

func NewParser() parser.Annotation {
	return webhookVerification{}
}

func (wv webhookVerification) Parse(obj client.Object) (interface{}, error) {
	provider, err := parser.GetStringAnnotation("webhook-verification-provider", obj)
	if err != nil {
		return nil, err
	}

	switch provider {
	case "sns":
		return &EndpointWebhookVerification{Provider: provider}, nil
	}

	secretName, err := parser.GetStringAnnotation("webhook-verification-secret-name", obj)
	if err != nil {
		return nil, err
	}

	secretKey, err := parser.GetStringAnnotation("webhook-verification-secret-key", obj)
	if err != nil {
		return nil, err
	}
	return &EndpointWebhookVerification{
		Provider: provider,
		SecretRef: &SecretKeyRef{
			Name: secretName,
			Key:  secretKey,
		},
	}, nil
}
