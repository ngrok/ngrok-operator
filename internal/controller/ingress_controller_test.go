package controllers

import (
	netv1 "k8s.io/api/networking/v1"
)

//nolint:unused
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
