package user_agent_filter

import (
	ingressv1alpha1 "github.com/ngrok/kubernetes-ingress-controller/api/v1alpha1"
	"github.com/ngrok/kubernetes-ingress-controller/internal/annotations/parser"
	networking "k8s.io/api/networking/v1"
)

type EndpointUserAgentFilter = ingressv1alpha1.EndpointUserAgentFilter

type userAgentFilter struct{}

func NewParser() parser.IngressAnnotation {
	return userAgentFilter{}
}

func (uaf userAgentFilter) Parse(ing *networking.Ingress) (interface{}, error) {
	allow, err := parser.GetStringSliceAnnotation("user-agent-filter-allow", ing)
	if err != nil {
		return nil, err
	}

	deny, err := parser.GetStringSliceAnnotation("user-agent-filter-deny", ing)
	if err != nil {
		return nil, err
	}

	return &EndpointUserAgentFilter{
		Allow: allow,
		Deny:  deny,
	}, nil
}
