package nmockapi

import (
	"github.com/ngrok/ngrok-operator/internal/ngrokapi"
)

// Clientset implements ngrokapi.Clientset for testing
type Clientset struct {
	domainsClient             *DomainClient
	endpointsClient           *EndpointsClient
	ipPoliciesClient          *IPPolicyClient
	ipPolicyRulesClient       *IPPolicyRuleClient
	kubernetesOperatorsClient *KubernetesOperatorsClient
}

func NewClientset() *Clientset {
	return &Clientset{
		domainsClient:             NewDomainClient(),
		endpointsClient:           NewEndpointsClient(),
		ipPoliciesClient:          NewIPPolicyClient(),
		ipPolicyRulesClient:       NewIPPolicyRuleClient(NewIPPolicyClient()),
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
	return m.ipPoliciesClient
}

func (m *Clientset) IPPolicyRules() ngrokapi.IPPolicyRulesClient {
	return m.ipPolicyRulesClient
}

func (m *Clientset) KubernetesOperators() ngrokapi.KubernetesOperatorsClient {
	return m.kubernetesOperatorsClient
}

func (m *Clientset) TCPAddresses() ngrokapi.TCPAddressesClient {
	return nil
}
