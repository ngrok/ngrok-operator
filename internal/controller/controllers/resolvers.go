package controllers

import (
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type IpPolicyResolver struct {
	Client client.Reader
}

type SecretResolver struct {
	Client client.Reader
}
