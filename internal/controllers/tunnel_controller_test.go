package controllers

import (
	"testing"

	"github.com/ngrok/ngrok-ingress-controller/pkg/agentapiclient"
	"github.com/stretchr/testify/assert"
	netv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestIngressToTunnels(t *testing.T) {

	testCases := []struct {
		testName string
		ingress  *netv1.Ingress
		tunnels  []agentapiclient.TunnelsAPIBody
	}{
		{
			testName: "Returns empty list when ingress is nil",
			ingress:  nil,
			tunnels:  []agentapiclient.TunnelsAPIBody{},
		},
		{
			testName: "Returns empty list when ingress has no rules",
			ingress: &netv1.Ingress{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-ingress",
				},
				Spec: netv1.IngressSpec{
					Rules: []netv1.IngressRule{},
				},
			},
			tunnels: []agentapiclient.TunnelsAPIBody{},
		},
		{
			ingress: &netv1.Ingress{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-ingress",
					Namespace: "test-namespace",
				},
				Spec: netv1.IngressSpec{
					Rules: []netv1.IngressRule{
						{
							Host: "my-test-tunnel.ngrok.io",
							IngressRuleValue: netv1.IngressRuleValue{
								HTTP: &netv1.HTTPIngressRuleValue{
									Paths: []netv1.HTTPIngressPath{
										{
											Path: "/",
											Backend: netv1.IngressBackend{
												Service: &netv1.IngressServiceBackend{
													Name: "test-service",
													Port: netv1.ServiceBackendPort{
														Number: 8080,
													},
												},
											},
										},
									},
								},
							},
						},
					},
				},
			},
			tunnels: []agentapiclient.TunnelsAPIBody{
				{
					Addr:      "test-service.test-namespace.svc.cluster.local:8080",
					Name:      "test-ingress-test-namespace-test-service-8080--",
					SubDomain: "",
					Labels: []string{
						"k8s.ngrok.com/ingress-name=test-ingress",
						"k8s.ngrok.com/ingress-namespace=test-namespace",
						"k8s.ngrok.com/service=test-service",
						"k8s.ngrok.com/port=8080",
					},
				},
			},
		},
	}

	for _, test := range testCases {
		tunnels := ingressToTunnels(test.ingress)

		assert.Equal(t, len(tunnels), len(test.tunnels))
		for i := 0; i < len(tunnels); i++ {
			assert.Equal(t, tunnels[i].Name, test.tunnels[i].Name)
			assert.Equal(t, tunnels[i].Addr, test.tunnels[i].Addr)
			assert.Equal(t, tunnels[i].SubDomain, test.tunnels[i].SubDomain)
			assert.ElementsMatch(t, tunnels[i].Labels, test.tunnels[i].Labels)
		}
	}
}
