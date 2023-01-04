package controllers

import (
	"context"
	"testing"

	ingressv1alpha1 "github.com/ngrok/ngrok-ingress-controller/api/v1alpha1"
	"github.com/ngrok/ngrok-ingress-controller/pkg/ngrokapidriver"
	"github.com/stretchr/testify/assert"
	netv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestIngressReconcilerIngressToEdge(t *testing.T) {
	prefix := netv1.PathTypePrefix
	testCases := []struct {
		testName string
		ingress  *netv1.Ingress
		edge     *ngrokapidriver.Edge
		err      error
	}{
		{
			testName: "Returns a nil edge when ingress is nil",
			ingress:  nil,
			edge:     nil,
		},
		{
			testName: "Returns a nil edge when ingress has no rules",
			ingress: &netv1.Ingress{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-ingress",
				},
				Spec: netv1.IngressSpec{
					Rules: []netv1.IngressRule{},
				},
			},
			edge: nil,
		},
		{
			testName: "",
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
											Path:     "/",
											PathType: &prefix,
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
			edge: &ngrokapidriver.Edge{
				Hostport: "my-test-tunnel.ngrok.io:443",
				Id:       "",
				Routes: []ngrokapidriver.Route{
					{
						Id:        "",
						Match:     "/",
						MatchType: "path_prefix",
						Labels: map[string]string{
							"k8s.ngrok.com/namespace": "test-namespace",
							"k8s.ngrok.com/port":      "8080",
							"k8s.ngrok.com/service":   "test-service",
						},
					},
				},
			},
		},
	}

	for _, testCase := range testCases {
		irec := IngressReconciler{}
		edge, err := irec.ingressToEdge(context.Background(), testCase.ingress)

		if testCase.err != nil {
			assert.ErrorIs(t, err, testCase.err)
			continue
		}
		assert.NoError(t, err)

		if testCase.edge == nil {
			assert.Nil(t, edge)
			continue
		}

		assert.Equal(t, testCase.edge.Hostport, edge.Hostport, "Hostport does not match expected value")
		assert.Equal(t, testCase.edge.Id, edge.Id, "ID does not match expected value")
		assert.ElementsMatch(t, testCase.edge.Routes, edge.Routes)
	}
}

func TestIngressToTunnels(t *testing.T) {

	testCases := []struct {
		testName string
		ingress  *netv1.Ingress
		tunnels  []ingressv1alpha1.Tunnel
	}{
		{
			testName: "Returns empty list when ingress is nil",
			ingress:  nil,
			tunnels:  []ingressv1alpha1.Tunnel{},
		},
		{
			testName: "Returns empty list when ingress has no rules",
			ingress: &netv1.Ingress{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-ingress",
					Namespace: "test-namespace",
				},
				Spec: netv1.IngressSpec{
					Rules: []netv1.IngressRule{},
				},
			},
			tunnels: []ingressv1alpha1.Tunnel{},
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
			tunnels: []ingressv1alpha1.Tunnel{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-service-8080",
						Namespace: "test-namespace",
					},
					Spec: ingressv1alpha1.TunnelSpec{
						ForwardsTo: "test-service.test-namespace.svc.cluster.local:8080",
						Labels: map[string]string{
							"k8s.ngrok.com/namespace": "test-namespace",
							"k8s.ngrok.com/service":   "test-service",
							"k8s.ngrok.com/port":      "8080",
						},
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
			assert.Equal(t, tunnels[i].Spec.ForwardsTo, test.tunnels[i].Spec.ForwardsTo)
			assert.Equal(t, tunnels[i].Spec.Labels, test.tunnels[i].Spec.Labels)
		}
	}
}
