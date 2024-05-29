package headers

import (
	ingressv1alpha1 "github.com/ngrok/kubernetes-ingress-controller/api/ingress/v1alpha1"
	"github.com/ngrok/kubernetes-ingress-controller/internal/annotations/parser"
	"github.com/ngrok/kubernetes-ingress-controller/internal/errors"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type EndpointHeaders = ingressv1alpha1.EndpointHeaders
type EndpointRequestHeaders = ingressv1alpha1.EndpointRequestHeaders
type EndpointResponseHeaders = ingressv1alpha1.EndpointResponseHeaders

type headers struct{}

func NewParser() parser.Annotation {
	return headers{}
}

func (h headers) Parse(obj client.Object) (interface{}, error) {
	parsed := &EndpointHeaders{}

	v, err := parser.GetStringSliceAnnotation("request-headers-remove", obj)
	if err != nil {
		if !errors.IsMissingAnnotations(err) {
			return parsed, err
		}
	}

	if len(v) > 0 {
		if parsed.Request == nil {
			parsed.Request = &EndpointRequestHeaders{}
		}
		parsed.Request.Remove = v
	}

	m, err := parser.GetStringMapAnnotation("request-headers-add", obj)
	if err != nil {
		if !errors.IsMissingAnnotations(err) {
			return parsed, err
		}
	}

	if len(m) > 0 {
		if parsed.Request == nil {
			parsed.Request = &EndpointRequestHeaders{}
		}
		parsed.Request.Add = m
	}

	if err != nil {
		if !errors.IsMissingAnnotations(err) {
			return parsed, err
		}
	}

	v, err = parser.GetStringSliceAnnotation("response-headers-remove", obj)
	if err != nil {
		if !errors.IsMissingAnnotations(err) {
			return parsed, err
		}
	}

	if len(v) > 0 {
		if parsed.Response == nil {
			parsed.Response = &EndpointResponseHeaders{}
		}
		parsed.Response.Remove = v
	}

	m, err = parser.GetStringMapAnnotation("response-headers-add", obj)
	if err != nil {
		if !errors.IsMissingAnnotations(err) {
			return parsed, err
		}
	}

	if len(m) > 0 {
		if parsed.Response == nil {
			parsed.Response = &EndpointResponseHeaders{}
		}
		parsed.Response.Add = m
	}

	// If none of the annotations are present, return the missing annotations error
	if parsed.Request == nil && parsed.Response == nil {
		return nil, errors.ErrMissingAnnotations
	}

	return parsed, nil
}
