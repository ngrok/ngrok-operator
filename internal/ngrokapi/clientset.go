package ngrokapi

import (
	"github.com/ngrok/ngrok-api-go/v7"
	tunnel_group_backends "github.com/ngrok/ngrok-api-go/v7/backends/tunnel_group"
	https_edges "github.com/ngrok/ngrok-api-go/v7/edges/https"
	https_edge_routes "github.com/ngrok/ngrok-api-go/v7/edges/https_routes"
	tcp_edges "github.com/ngrok/ngrok-api-go/v7/edges/tcp"
	tls_edges "github.com/ngrok/ngrok-api-go/v7/edges/tls"
	"github.com/ngrok/ngrok-api-go/v7/endpoints"
	"github.com/ngrok/ngrok-api-go/v7/ip_policies"
	"github.com/ngrok/ngrok-api-go/v7/ip_policy_rules"
	"github.com/ngrok/ngrok-api-go/v7/kubernetes_operators"
	"github.com/ngrok/ngrok-api-go/v7/reserved_addrs"
	"github.com/ngrok/ngrok-api-go/v7/reserved_domains"
)

type Clientset interface {
	Domains() *reserved_domains.Client
	EdgeModules() EdgeModulesClientset
	Endpoints() *endpoints.Client
	HTTPSEdges() *https_edges.Client
	HTTPSEdgeRoutes() *https_edge_routes.Client
	IPPolicies() *ip_policies.Client
	IPPolicyRules() *ip_policy_rules.Client
	KubernetesOperators() *kubernetes_operators.Client
	TCPAddresses() *reserved_addrs.Client
	TCPEdges() *tcp_edges.Client
	TLSEdges() *tls_edges.Client
	TunnelGroupBackends() *tunnel_group_backends.Client
}

type DefaultClientset struct {
	domainsClient             *reserved_domains.Client
	edgeModulesClientset      *defaultEdgeModulesClientset
	endpointsClient           *endpoints.Client
	httpsEdgesClient          *https_edges.Client
	httpsEdgeRoutesClient     *https_edge_routes.Client
	ipPoliciesClient          *ip_policies.Client
	ipPolicyRulesClient       *ip_policy_rules.Client
	kubernetesOperatorsClient *kubernetes_operators.Client
	tcpAddrsClient            *reserved_addrs.Client
	tcpEdgesClient            *tcp_edges.Client
	tlsEdgesClient            *tls_edges.Client
	tunnelGroupBackendsClient *tunnel_group_backends.Client
}

// NewClientSet creates a new ClientSet from an ngrok client config.
func NewClientSet(config *ngrok.ClientConfig) *DefaultClientset {
	return &DefaultClientset{
		domainsClient:             reserved_domains.NewClient(config),
		edgeModulesClientset:      newEdgeModulesClientset(config),
		endpointsClient:           endpoints.NewClient(config),
		httpsEdgesClient:          https_edges.NewClient(config),
		httpsEdgeRoutesClient:     https_edge_routes.NewClient(config),
		ipPoliciesClient:          ip_policies.NewClient(config),
		ipPolicyRulesClient:       ip_policy_rules.NewClient(config),
		kubernetesOperatorsClient: kubernetes_operators.NewClient(config),
		tcpAddrsClient:            reserved_addrs.NewClient(config),
		tcpEdgesClient:            tcp_edges.NewClient(config),
		tlsEdgesClient:            tls_edges.NewClient(config),
		tunnelGroupBackendsClient: tunnel_group_backends.NewClient(config),
	}
}

func (c *DefaultClientset) Domains() *reserved_domains.Client {
	return c.domainsClient
}

func (c *DefaultClientset) EdgeModules() EdgeModulesClientset {
	return c.edgeModulesClientset
}

func (c *DefaultClientset) Endpoints() *endpoints.Client {
	return c.endpointsClient
}

func (c *DefaultClientset) HTTPSEdges() *https_edges.Client {
	return c.httpsEdgesClient
}

func (c *DefaultClientset) HTTPSEdgeRoutes() *https_edge_routes.Client {
	return c.httpsEdgeRoutesClient
}

func (c *DefaultClientset) IPPolicies() *ip_policies.Client {
	return c.ipPoliciesClient
}

func (c *DefaultClientset) IPPolicyRules() *ip_policy_rules.Client {
	return c.ipPolicyRulesClient
}

func (c *DefaultClientset) KubernetesOperators() *kubernetes_operators.Client {
	return c.kubernetesOperatorsClient
}

func (c *DefaultClientset) TCPAddresses() *reserved_addrs.Client {
	return c.tcpAddrsClient
}

func (c *DefaultClientset) TLSEdges() *tls_edges.Client {
	return c.tlsEdgesClient
}

func (c *DefaultClientset) TCPEdges() *tcp_edges.Client {
	return c.tcpEdgesClient
}

func (c *DefaultClientset) TunnelGroupBackends() *tunnel_group_backends.Client {
	return c.tunnelGroupBackendsClient
}
