package compression

import (
	ingressv1alpha1 "github.com/ngrok/ngrok-operator/api/ingress/v1alpha1"
	"github.com/ngrok/ngrok-operator/internal/annotations/parser"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type compression struct{}

func NewParser() parser.Annotation {
	return compression{}
}

// Parse parses the annotations contained in the ingress and returns a
// compression configuration or an error. If no compression annotations are
// found, the returned error an errors.ErrMissingAnnotations.
func (c compression) Parse(obj client.Object) (interface{}, error) {
	v, err := parser.GetBoolAnnotation("https-compression", obj)
	if err != nil {
		return nil, err
	}

	return &ingressv1alpha1.EndpointCompression{
		Enabled: v,
	}, nil
}
