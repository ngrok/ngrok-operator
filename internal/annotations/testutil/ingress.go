// Test Utilities for Ingress Annotations
package testutil

import (
	v1 "k8s.io/api/core/v1"
	networking "k8s.io/api/networking/v1"
)

func NewIngress() *networking.Ingress {
	return &networking.Ingress{
		Name:      "test-ingress",
		Namespace: v1.NamespaceDefault,
		Spec: networking.IngressSpec{
			Rules: []networking.IngressRule{
				{
					Host: "test.ngrok.app",
					HTTP: &networking.HTTPIngressRuleValue{
						Paths: []networking.HTTPIngressPath{
							{
								Path: "/foo",
								Backend: networking.IngressBackend{
									Service: &networking.IngressServiceBackend{
										Name: "foo",
										Port: networking.ServiceBackendPort{
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
	}
}
