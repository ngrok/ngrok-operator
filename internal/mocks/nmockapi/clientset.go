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

// DomainsMock returns the concrete mock domain client, for tests that need to
// seed reservations or inspect the filter that was sent. Domains() returns the
// narrower ngrokapi.DomainClient interface, which would otherwise force a type
// assertion at every call site.
func (m *Clientset) DomainsMock() *DomainClient {
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
