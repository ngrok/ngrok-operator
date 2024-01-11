package webhook_verification

import (
	ingressv1alpha1 "github.com/ngrok/kubernetes-ingress-controller/api/ingress/v1alpha1"
	"github.com/ngrok/kubernetes-ingress-controller/internal/annotations/parser"
	networking "k8s.io/api/networking/v1"
)

type EndpointWebhookVerification = ingressv1alpha1.EndpointWebhookVerification
type SecretKeyRef = ingressv1alpha1.SecretKeyRef

type webhookVerification struct{}

func NewParser() parser.IngressAnnotation {
	return webhookVerification{}
}

func (wv webhookVerification) Parse(ing *networking.Ingress) (interface{}, error) {
	provider, err := parser.GetStringAnnotation("webhook-verification-provider", ing)
	if err != nil {
		return nil, err
	}

	switch provider {
	case "sns":
		return &EndpointWebhookVerification{Provider: provider}, nil
	}

	secretName, err := parser.GetStringAnnotation("webhook-verification-secret-name", ing)
	if err != nil {
		return nil, err
	}

	secretKey, err := parser.GetStringAnnotation("webhook-verification-secret-key", ing)
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
