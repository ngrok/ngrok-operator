package nmockapi

import (
	"github.com/ngrok/ngrok-operator/internal/ngrokapi"
)

// Clientset implements ngrokapi.Clientset for testing
type Clientset struct {
	domainsClient             *DomainClient
	endpointsClient           *EndpointsClient
	kubernetesOperatorsClient *KubernetesOperatorsClient
}

func NewClientset() *Clientset {
	return &Clientset{
		domainsClient:             NewDomainClient(),
		endpointsClient:           NewEndpointsClient(),
		kubernetesOperatorsClient: NewKubernetesOperatorsClient(),
	}
}

func (m *Clientset) Domains() ngrokapi.DomainClient {
	return m.domainsClient
}

func (m *Clientset) Endpoints() ngrokapi.EndpointsClient {
	return m.endpointsClient
}

func (m *Clientset) IPPolicies() ngrokapi.IPPoliciesClient {
	return nil
}

func (m *Clientset) IPPolicyRules() ngrokapi.IPPolicyRulesClient {
	return nil
}

func (m *Clientset) KubernetesOperators() ngrokapi.KubernetesOperatorsClient {
	return m.kubernetesOperatorsClient
}

func (m *Clientset) TCPAddresses() ngrokapi.TCPAddressesClient {
	return nil
}
