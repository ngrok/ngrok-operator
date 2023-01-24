package headers

import (
	ingressv1alpha1 "github.com/ngrok/kubernetes-ingress-controller/api/v1alpha1"
	"github.com/ngrok/kubernetes-ingress-controller/internal/annotations/parser"
	"github.com/ngrok/kubernetes-ingress-controller/internal/errors"
	networking "k8s.io/api/networking/v1"
)

type headers struct{}

func NewParser() parser.IngressAnnotation {
	return headers{}
}

func (h headers) Parse(ing *networking.Ingress) (interface{}, error) {
	parsed := &ingressv1alpha1.EndpointHeaders{}

	v, err := parser.GetStringSliceAnnotation("request-headers-remove", ing)
	if err != nil {
		if !errors.IsMissingAnnotations(err) {
			return parsed, err
		}
	}

	if len(v) > 0 {
		if parsed.Request == nil {
			parsed.Request = &ingressv1alpha1.EndpointRequestHeaders{}
		}
		parsed.Request.Remove = v
	}

	v, err = parser.GetStringSliceAnnotation("response-headers-remove", ing)
	if err != nil {
		if !errors.IsMissingAnnotations(err) {
			return parsed, err
		}
	}

	if len(v) > 0 {
		if parsed.Response == nil {
			parsed.Response = &ingressv1alpha1.EndpointResponseHeaders{}
		}
		parsed.Response.Remove = v
	}

	return parsed, nil
}
