package tls

import (
	ingressv1alpha1 "github.com/ngrok/kubernetes-ingress-controller/api/ingress/v1alpha1"
	"github.com/ngrok/kubernetes-ingress-controller/internal/annotations/parser"
	networking "k8s.io/api/networking/v1"
)

type EndpointTLSTerminationAtEdge = ingressv1alpha1.EndpointTLSTerminationAtEdge

type tls struct{}

func NewParser() parser.IngressAnnotation {
	return tls{}
}

// Parse parses the annotations contained in the ingress and returns a
// tls configuration or an error. If no tls annotations are
// found, the returned error an errors.ErrMissingAnnotations.
func (t tls) Parse(ing *networking.Ingress) (interface{}, error) {
	v, err := parser.GetStringAnnotation("tls-min-version", ing)
	if err != nil {
		return nil, err
	}

	return &EndpointTLSTerminationAtEdge{MinVersion: v}, nil
}
