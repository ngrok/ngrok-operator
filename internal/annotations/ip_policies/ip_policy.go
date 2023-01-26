package ip_policies

import (
	ingressv1alpha1 "github.com/ngrok/kubernetes-ingress-controller/api/v1alpha1"
	"github.com/ngrok/kubernetes-ingress-controller/internal/annotations/parser"
	networking "k8s.io/api/networking/v1"
)

type ipPolicy struct{}

func NewParser() parser.IngressAnnotation {
	return ipPolicy{}
}

func (p ipPolicy) Parse(ing *networking.Ingress) (interface{}, error) {
	v, err := parser.GetStringSliceAnnotation("ip-policy-ids", ing)
	if err != nil {
		return nil, err
	}

	return &ingressv1alpha1.EndpointIPPolicy{
		IPPolicyIDs: v,
	}, nil
}
