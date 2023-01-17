package ngrokapi

import (
	"github.com/ngrok/ngrok-api-go/v5"
	tunnel_group_backends "github.com/ngrok/ngrok-api-go/v5/backends/tunnel_group"
	https_edges "github.com/ngrok/ngrok-api-go/v5/edges/https"
	https_edge_routes "github.com/ngrok/ngrok-api-go/v5/edges/https_routes"
	tcp_edges "github.com/ngrok/ngrok-api-go/v5/edges/tcp"
	"github.com/ngrok/ngrok-api-go/v5/ip_policies"
	"github.com/ngrok/ngrok-api-go/v5/ip_policy_rules"
	"github.com/ngrok/ngrok-api-go/v5/reserved_addrs"
	"github.com/ngrok/ngrok-api-go/v5/reserved_domains"
)

type Clientset interface {
	Domains() *reserved_domains.Client
	HTTPSEdges() *https_edges.Client
	HTTPSEdgeRoutes() *https_edge_routes.Client
	IPPolicies() *ip_policies.Client
	IPPolicyRules() *ip_policy_rules.Client
	TCPAddresses() *reserved_addrs.Client
	TCPEdges() *tcp_edges.Client
	TunnelGroupBackends() *tunnel_group_backends.Client
}

type DefaultClientset struct {
	domainsClient             *reserved_domains.Client
	httpsEdgesClient          *https_edges.Client
	httpsEdgeRoutesClient     *https_edge_routes.Client
	tcpAddrsClient            *reserved_addrs.Client
	ipPoliciesClient          *ip_policies.Client
	ipPolicyRulesClient       *ip_policy_rules.Client
	tcpEdgesClient            *tcp_edges.Client
	tunnelGroupBackendsClient *tunnel_group_backends.Client
}

// NewClientSet creates a new ClientSet from an ngrok client config.
func NewClientSet(config *ngrok.ClientConfig) *DefaultClientset {
	return &DefaultClientset{
		domainsClient:             reserved_domains.NewClient(config),
		httpsEdgesClient:          https_edges.NewClient(config),
		httpsEdgeRoutesClient:     https_edge_routes.NewClient(config),
		ipPoliciesClient:          ip_policies.NewClient(config),
		ipPolicyRulesClient:       ip_policy_rules.NewClient(config),
		tcpAddrsClient:            reserved_addrs.NewClient(config),
		tcpEdgesClient:            tcp_edges.NewClient(config),
		tunnelGroupBackendsClient: tunnel_group_backends.NewClient(config),
	}
}

func (c *DefaultClientset) Domains() *reserved_domains.Client {
	return c.domainsClient
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

func (c *DefaultClientset) TCPAddresses() *reserved_addrs.Client {
	return c.tcpAddrsClient
}

func (c *DefaultClientset) TCPEdges() *tcp_edges.Client {
	return c.tcpEdgesClient
}

func (c *DefaultClientset) TunnelGroupBackends() *tunnel_group_backends.Client {
	return c.tunnelGroupBackendsClient
}
