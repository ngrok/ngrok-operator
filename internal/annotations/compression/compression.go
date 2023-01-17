package compression

import (
	ingressv1alpha1 "github.com/ngrok/kubernetes-ingress-controller/api/v1alpha1"
	"github.com/ngrok/kubernetes-ingress-controller/internal/annotations/parser"
	networking "k8s.io/api/networking/v1"
	"k8s.io/utils/pointer"
)

type compression struct{}

func NewParser() parser.IngressAnnotation {
	return compression{}
}

func (c compression) Parse(ing *networking.Ingress) (interface{}, error) {
	v, err := parser.GetBoolAnnotation("https-compression", ing)
	if err != nil {
		return nil, err
	}

	return &ingressv1alpha1.EndpointCompression{
		Enabled: pointer.Bool(v),
	}, nil
}
