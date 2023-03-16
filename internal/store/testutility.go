package store

import (
	ingressv1alpha1 "github.com/ngrok/kubernetes-ingress-controller/api/v1alpha1"
	netv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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

func NewHTTPSEdge(name string, namespace string, domain string) ingressv1alpha1.HTTPSEdge {
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
