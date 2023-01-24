package headers

import (
	ingressv1alpha1 "github.com/ngrok/kubernetes-ingress-controller/api/v1alpha1"
	"github.com/ngrok/kubernetes-ingress-controller/internal/annotations/parser"
	"github.com/ngrok/kubernetes-ingress-controller/internal/errors"
	networking "k8s.io/api/networking/v1"
	"k8s.io/utils/pointer"
)

type headers struct{}

func NewParser() parser.IngressAnnotation {
	return headers{}
}

func (h headers) Parse(ing *networking.Ingress) (interface{}, error) {
	parsed := &ingressv1alpha1.EndpointHeaders{
		Request: &ingressv1alpha1.EndpointRequestHeaders{
			Enabled: pointer.Bool(false),
		},
		Response: &ingressv1alpha1.EndpointResponseHeaders{
			Enabled: pointer.Bool(false),
		},
	}

	v, err := parser.GetStringSliceAnnotation("request-headers-remove", ing)
	if err != nil {
		if !errors.IsMissingAnnotations(err) {
			return parsed, err
		}
	}

	if len(v) > 0 {
		parsed.Request.Enabled = pointer.Bool(true)
		parsed.Request.Remove = v
	}

	v, err = parser.GetStringSliceAnnotation("response-headers-remove", ing)
	if err != nil {
		if !errors.IsMissingAnnotations(err) {
			return parsed, err
		}
	}

	if len(v) > 0 {
		parsed.Response.Enabled = pointer.Bool(true)
		parsed.Response.Remove = v
	}

	return parsed, nil
}
