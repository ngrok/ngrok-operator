package tls

import (
	ingressv1alpha1 "github.com/ngrok/kubernetes-ingress-controller/api/ingress/v1alpha1"
	"github.com/ngrok/kubernetes-ingress-controller/internal/annotations/parser"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type EndpointTLSTerminationAtEdge = ingressv1alpha1.EndpointTLSTerminationAtEdge

type tls struct{}

func NewParser() parser.Annotation {
	return tls{}
}

// Parse parses the annotations contained in the ingress and returns a
// tls configuration or an error. If no tls annotations are
// found, the returned error an errors.ErrMissingAnnotations.
func (t tls) Parse(obj client.Object) (interface{}, error) {
	v, err := parser.GetStringAnnotation("tls-min-version", obj)
	if err != nil {
		return nil, err
	}

	return &EndpointTLSTerminationAtEdge{MinVersion: v}, nil
}
