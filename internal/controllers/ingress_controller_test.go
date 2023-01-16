package controllers

import (
	"context"
	"fmt"
	"testing"

	ingressv1alpha1 "github.com/ngrok/ngrok-ingress-controller/api/v1alpha1"
	"github.com/stretchr/testify/assert"
	netv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func makeTestBackend(serviceName string, servicePort int32) netv1.IngressBackend {
	return netv1.IngressBackend{
		Service: &netv1.IngressServiceBackend{
			Name: serviceName,
			Port: netv1.ServiceBackendPort{
				Number: servicePort,
			},
		},
	}
}

func makeTestTunnel(namespace, serviceName string, servicePort int) ingressv1alpha1.Tunnel {
	return ingressv1alpha1.Tunnel{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("%s-%d", serviceName, servicePort),
			Namespace: namespace,
		},
		Spec: ingressv1alpha1.TunnelSpec{
			ForwardsTo: fmt.Sprintf("%s.%s.%s:%d", serviceName, namespace, clusterDomain, servicePort),
			Labels: map[string]string{
				"k8s.ngrok.com/namespace": namespace,
				"k8s.ngrok.com/service":   serviceName,
				"k8s.ngrok.com/port":      fmt.Sprintf("%d", servicePort),
			},
		},
	}
}

func TestIngressReconcilerIngressToEdge(t *testing.T) {
	prefix := netv1.PathTypePrefix
	testCases := []struct {
		testName string
		ingress  *netv1.Ingress
		edge     *ingressv1alpha1.HTTPSEdge
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
											Backend:  makeTestBackend("test-service", 8080),
										},
									},
								},
							},
						},
					},
				},
			},
			edge: &ingressv1alpha1.HTTPSEdge{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-ingress",
					Namespace: "test-namespace",
				},
				Spec: ingressv1alpha1.HTTPSEdgeSpec{
					Hostports: []string{"my-test-tunnel.ngrok.io:443"},
					Routes: []ingressv1alpha1.HTTPSEdgeRouteSpec{
						{
							Match:     "/",
							MatchType: "path_prefix",
							Backend: ingressv1alpha1.TunnelGroupBackend{
								Labels: map[string]string{
									"k8s.ngrok.com/namespace": "test-namespace",
									"k8s.ngrok.com/service":   "test-service",
									"k8s.ngrok.com/port":      "8080",
								},
							},
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

		assert.Equal(t, testCase.edge, edge, "Edge does not match expected value")
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
			testName: "Converts an ingress to a tunnel",
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
											Path:    "/",
											Backend: makeTestBackend("test-service", 8080),
										},
									},
								},
							},
						},
					},
				},
			},
			tunnels: []ingressv1alpha1.Tunnel{
				makeTestTunnel("test-namespace", "test-service", 8080),
			},
		},
		{
			testName: "Correctly converts an ingress with multiple paths that point to the same service",
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
											Path:    "/",
											Backend: makeTestBackend("test-service", 8080),
										},
										{
											Path:    "/api",
											Backend: makeTestBackend("test-api", 80),
										},
									},
								},
							},
						},
						{
							Host: "my-other-test-tunnel.ngrok.io",
							IngressRuleValue: netv1.IngressRuleValue{
								HTTP: &netv1.HTTPIngressRuleValue{
									Paths: []netv1.HTTPIngressPath{
										{
											Path:    "/",
											Backend: makeTestBackend("test-service", 8080),
										},
										{
											Path:    "/api",
											Backend: makeTestBackend("test-api", 80),
										},
									},
								},
							},
						},
					},
				},
			},
			tunnels: []ingressv1alpha1.Tunnel{
				makeTestTunnel("test-namespace", "test-service", 8080),
				makeTestTunnel("test-namespace", "test-api", 80),
			},
		},
	}

	for _, test := range testCases {
		tunnels := ingressToTunnels(test.ingress)
		assert.ElementsMatch(t, tunnels, test.tunnels)
	}
}
