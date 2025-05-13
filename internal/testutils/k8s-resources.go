package testutils

import (
	"encoding/json"

	ingressv1alpha1 "github.com/ngrok/ngrok-operator/api/ingress/v1alpha1"
	ngrokv1alpha1 "github.com/ngrok/ngrok-operator/api/ngrok/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	netv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/apimachinery/pkg/util/rand"
	"k8s.io/utils/ptr"

	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"
	gatewayv1alpha2 "sigs.k8s.io/gateway-api/apis/v1alpha2"
	gatewayv1beta1 "sigs.k8s.io/gateway-api/apis/v1beta1"
)

func NewTestIngressClass(name string, isDefault bool, isNgrok bool) netv1.IngressClass {
	i := netv1.IngressClass{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
			Labels: map[string]string{
				"app.kubernetes.io/component": "controller",
			},
		},
	}

	if isNgrok {
		i.Spec.Controller = "k8s.ngrok.com/ingress-controller"
	} else {
		i.Spec.Controller = "kubernetes.io/ingress-other"
	}

	if isDefault {
		i.Annotations = map[string]string{
			"ingressclass.kubernetes.io/is-default-class": "true", // TODO: Move this into a utility for ingress classes
		}
	}

	return i
}

func NewTestIngressV1WithClass(name string, namespace string, ingressClass string) netv1.Ingress {
	i := NewTestIngressV1(name, namespace)
	i.Spec.IngressClassName = &ingressClass
	return i
}

func NewTestIngressV1(name string, namespace string) netv1.Ingress {
	return netv1.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Spec: netv1.IngressSpec{
			Rules: []netv1.IngressRule{
				{
					Host: "example.com",
					IngressRuleValue: netv1.IngressRuleValue{
						HTTP: &netv1.HTTPIngressRuleValue{
							Paths: []netv1.HTTPIngressPath{
								{
									Path: "/",
									Backend: netv1.IngressBackend{
										Service: &netv1.IngressServiceBackend{
											Name: "example",
											Port: netv1.ServiceBackendPort{
												Number: 80,
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
	}
}

func NewTestServiceV1(name string, namespace string) corev1.Service {
	return corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Spec: corev1.ServiceSpec{
			Ports: []corev1.ServicePort{
				{
					Name:       "http",
					Protocol:   "TCP",
					Port:       80,
					TargetPort: intstr.FromString("http"),
				},
			},
		},
	}
}

func NewDomainV1(name string, namespace string) ingressv1alpha1.Domain {
	return ingressv1alpha1.Domain{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Spec: ingressv1alpha1.DomainSpec{
			Domain: name,
		},
	}
}

// RandomName generates a random name with the given prefix. The random part is 5 characters long.
// This is useful for generating unique names for resources in tests.
func RandomName(prefix string) string {
	return prefix + "-" + rand.String(5)
}

// NewGatewayClass creates a new GatewayClass with a random name to be used in tests. If
// isManaged is true, the controller name will be set to the ngrok gateway controller name.
// If isManaged is false, the controller name will be set to a different value.
func NewGatewayClass(isManaged bool) *gatewayv1.GatewayClass {
	var controllerName gatewayv1.GatewayController = "ngrok.com/gateway-controller"
	if !isManaged {
		controllerName = "k8s.io/some-other-controller"
	}
	return &gatewayv1.GatewayClass{
		ObjectMeta: metav1.ObjectMeta{
			Name: RandomName("gateway-class"),
		},
		Spec: gatewayv1.GatewayClassSpec{
			ControllerName: controllerName,
		},
	}
}

func NewGateway(name string, namespace string) gatewayv1.Gateway {
	return gatewayv1.Gateway{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Spec: gatewayv1.GatewaySpec{
			GatewayClassName: "test-gatewayclass",
			Listeners: []gatewayv1.Listener{
				{
					Name:     "http",
					Port:     80,
					Protocol: gatewayv1.HTTPProtocolType,
				},
			},
		},
	}
}

func NewHTTPRoute(name string, namespace string) gatewayv1.HTTPRoute {
	return gatewayv1.HTTPRoute{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Spec: gatewayv1.HTTPRouteSpec{
			Rules: []gatewayv1.HTTPRouteRule{
				{
					Matches: []gatewayv1.HTTPRouteMatch{
						{
							Path: &gatewayv1.HTTPPathMatch{
								Type:  ptr.To(gatewayv1.PathMatchPathPrefix),
								Value: ptr.To("/"),
							},
						},
					},
				},
			},
		},
	}
}

func NewTLSRoute(name string, namespace string) gatewayv1alpha2.TLSRoute {
	return gatewayv1alpha2.TLSRoute{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Spec: gatewayv1alpha2.TLSRouteSpec{
			Rules: []gatewayv1alpha2.TLSRouteRule{
				{
					BackendRefs: []gatewayv1alpha2.BackendRef{
						{
							BackendObjectReference: gatewayv1alpha2.BackendObjectReference{
								Name: "test-service",
								Port: ptr.To(gatewayv1.PortNumber(8000)),
							},
						},
					},
				},
			},
		},
	}
}

func NewTCPRoute(name string, namespace string) gatewayv1alpha2.TCPRoute {
	return gatewayv1alpha2.TCPRoute{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Spec: gatewayv1alpha2.TCPRouteSpec{
			Rules: []gatewayv1alpha2.TCPRouteRule{
				{
					BackendRefs: []gatewayv1alpha2.BackendRef{
						{
							BackendObjectReference: gatewayv1alpha2.BackendObjectReference{
								Name: "tcp-service",
								Port: ptr.To(gatewayv1.PortNumber(8000)),
							},
						},
					},
				},
			},
		},
	}
}

func NewReferenceGrant(name string, namespace string) gatewayv1beta1.ReferenceGrant {
	return gatewayv1beta1.ReferenceGrant{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Spec: gatewayv1beta1.ReferenceGrantSpec{
			From: []gatewayv1beta1.ReferenceGrantFrom{
				{
					Group:     "gateway.networking.k8s.io",
					Kind:      "HTTPRoute",
					Namespace: gatewayv1beta1.Namespace("other-namespace"),
				},
			},
			To: []gatewayv1beta1.ReferenceGrantTo{
				{
					Group: "core",
					Kind:  "Service",
				},
			},
		},
	}
}

func NewHTTPSEdge(name string, namespace string) ingressv1alpha1.HTTPSEdge {
	return ingressv1alpha1.HTTPSEdge{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
	}
}

func NewTestNgrokModuleSet(name string, namespace string, compressionEnabled bool) ingressv1alpha1.NgrokModuleSet {
	return ingressv1alpha1.NgrokModuleSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Modules: ingressv1alpha1.NgrokModuleSetModules{
			Compression: &ingressv1alpha1.EndpointCompression{
				Enabled: compressionEnabled,
			},
		},
	}
}

func NewTestNgrokTrafficPolicy(name string, namespace string, policyStr string) ngrokv1alpha1.NgrokTrafficPolicy {
	return ngrokv1alpha1.NgrokTrafficPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Spec: ngrokv1alpha1.NgrokTrafficPolicySpec{
			Policy: json.RawMessage(policyStr),
		},
	}
}
