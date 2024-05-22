package ip_policies

import (
	ingressv1alpha1 "github.com/ngrok/kubernetes-ingress-controller/api/ingress/v1alpha1"
	"github.com/ngrok/kubernetes-ingress-controller/internal/annotations/parser"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type ipPolicy struct{}

func NewParser() parser.Annotation {
	return ipPolicy{}
}

func (p ipPolicy) Parse(obj client.Object) (interface{}, error) {
	v, err := parser.GetStringSliceAnnotation("ip-policies", obj)
	if err != nil {
		return nil, err
	}

	return &ingressv1alpha1.EndpointIPPolicy{IPPolicies: v}, nil
}
