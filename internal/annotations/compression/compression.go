package compression

import (
	ingressv1alpha1 "github.com/ngrok/kubernetes-ingress-controller/api/v1alpha1"
	"github.com/ngrok/kubernetes-ingress-controller/internal/annotations/parser"
	networking "k8s.io/api/networking/v1"
)

type compression struct{}

func NewParser() parser.IngressAnnotation {
	return compression{}
}

// Parse parses the annotations contained in the ingress and returns a
// compression configuration or an error. If no compression annotations are
// found, the returned error an errors.ErrMissingAnnotations.
func (c compression) Parse(ing *networking.Ingress) (interface{}, error) {
	v, err := parser.GetBoolAnnotation("https-compression", ing)
	if err != nil {
		return nil, err
	}

	return &ingressv1alpha1.EndpointCompression{
		Enabled: v,
	}, nil
}
