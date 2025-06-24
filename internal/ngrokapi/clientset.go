package ngrokapi

import (
	"context"

	"github.com/ngrok/ngrok-api-go/v7"
	tunnel_group_backends "github.com/ngrok/ngrok-api-go/v7/backends/tunnel_group"
	https_edges "github.com/ngrok/ngrok-api-go/v7/edges/https"
	https_edge_routes "github.com/ngrok/ngrok-api-go/v7/edges/https_routes"
	"github.com/ngrok/ngrok-api-go/v7/endpoints"
	"github.com/ngrok/ngrok-api-go/v7/ip_policies"
	"github.com/ngrok/ngrok-api-go/v7/ip_policy_rules"
	"github.com/ngrok/ngrok-api-go/v7/kubernetes_operators"
	"github.com/ngrok/ngrok-api-go/v7/reserved_addrs"
	"github.com/ngrok/ngrok-api-go/v7/reserved_domains"
)

type Clientset interface {
	Domains() DomainClient
	EdgeModules() EdgeModulesClientset
	Endpoints() EndpointsClient
	HTTPSEdges() HTTPSEdgeClient
	HTTPSEdgeRoutes() HTTPSEdgeRoutesClient
	IPPolicies() IPPoliciesClient
	IPPolicyRules() IPPolicyRulesClient
	KubernetesOperators() KubernetesOperatorsClient
	TCPAddresses() TCPAddressesClient
	TunnelGroupBackends() TunnelGroupBackendsClient
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
		tunnelGroupBackendsClient: tunnel_group_backends.NewClient(config),
	}
}

type Creator[R, T any] interface {
	Create(context.Context, R) (T, error)
}

type Reader[T any] interface {
	Get(context.Context, string) (T, error)
}

type Updater[R, T any] interface {
	Update(context.Context, R) (T, error)
}

type Deletor interface {
	Delete(context.Context, string) error
}

type Lister[T any] interface {
	List(*ngrok.Paging) ngrok.Iter[T]
}

type DomainClient interface {
	Creator[*ngrok.ReservedDomainCreate, *ngrok.ReservedDomain]
	Reader[*ngrok.ReservedDomain]
	Updater[*ngrok.ReservedDomainUpdate, *ngrok.ReservedDomain]
	Deletor
	Lister[*ngrok.ReservedDomain]
}

func (c *DefaultClientset) Domains() DomainClient {
	return c.domainsClient
}

func (c *DefaultClientset) EdgeModules() EdgeModulesClientset {
	return c.edgeModulesClientset
}

type EndpointsClient interface {
	Creator[*ngrok.EndpointCreate, *ngrok.Endpoint]
	Reader[*ngrok.Endpoint]
	Updater[*ngrok.EndpointUpdate, *ngrok.Endpoint]
	Deletor
	Lister[*ngrok.Endpoint]
}

func (c *DefaultClientset) Endpoints() EndpointsClient {
	return c.endpointsClient
}

type HTTPSEdgeClient interface {
	Creator[*ngrok.HTTPSEdgeCreate, *ngrok.HTTPSEdge]
	Reader[*ngrok.HTTPSEdge]
	Updater[*ngrok.HTTPSEdgeUpdate, *ngrok.HTTPSEdge]
	Deletor
	Lister[*ngrok.HTTPSEdge]
}

func (c *DefaultClientset) HTTPSEdges() HTTPSEdgeClient {
	return c.httpsEdgesClient
}

type HTTPSEdgeRoutesClient interface {
	Creator[*ngrok.HTTPSEdgeRouteCreate, *ngrok.HTTPSEdgeRoute]
	Get(context.Context, *ngrok.EdgeRouteItem) (*ngrok.HTTPSEdgeRoute, error)
	Updater[*ngrok.HTTPSEdgeRouteUpdate, *ngrok.HTTPSEdgeRoute]
	Delete(context.Context, *ngrok.EdgeRouteItem) error
}

func (c *DefaultClientset) HTTPSEdgeRoutes() HTTPSEdgeRoutesClient {
	return c.httpsEdgeRoutesClient
}

type IPPoliciesClient interface {
	Creator[*ngrok.IPPolicyCreate, *ngrok.IPPolicy]
	Reader[*ngrok.IPPolicy]
	Updater[*ngrok.IPPolicyUpdate, *ngrok.IPPolicy]
	Deletor
}

func (c *DefaultClientset) IPPolicies() IPPoliciesClient {
	return c.ipPoliciesClient
}

type IPPolicyRulesClient interface {
	Creator[*ngrok.IPPolicyRuleCreate, *ngrok.IPPolicyRule]
	Deletor
	Updater[*ngrok.IPPolicyRuleUpdate, *ngrok.IPPolicyRule]
	Lister[*ngrok.IPPolicyRule]
}

func (c *DefaultClientset) IPPolicyRules() IPPolicyRulesClient {
	return c.ipPolicyRulesClient
}

type KubernetesOperatorsClient interface {
	Creator[*ngrok.KubernetesOperatorCreate, *ngrok.KubernetesOperator]
	Reader[*ngrok.KubernetesOperator]
	Updater[*ngrok.KubernetesOperatorUpdate, *ngrok.KubernetesOperator]
	Deletor
	Lister[*ngrok.KubernetesOperator]
	GetBoundEndpoints(string, *ngrok.Paging) ngrok.Iter[*ngrok.Endpoint]
}

func (c *DefaultClientset) KubernetesOperators() KubernetesOperatorsClient {
	return c.kubernetesOperatorsClient
}

type TCPAddressesClient interface {
	Creator[*ngrok.ReservedAddrCreate, *ngrok.ReservedAddr]
	Updater[*ngrok.ReservedAddrUpdate, *ngrok.ReservedAddr]
	Lister[*ngrok.ReservedAddr]
}

func (c *DefaultClientset) TCPAddresses() TCPAddressesClient {
	return c.tcpAddrsClient
}

type TunnelGroupBackendsClient interface {
	Creator[*ngrok.TunnelGroupBackendCreate, *ngrok.TunnelGroupBackend]
	Reader[*ngrok.TunnelGroupBackend]
	Updater[*ngrok.TunnelGroupBackendUpdate, *ngrok.TunnelGroupBackend]
	Lister[*ngrok.TunnelGroupBackend]
}

func (c *DefaultClientset) TunnelGroupBackends() TunnelGroupBackendsClient {
	return c.tunnelGroupBackendsClient
}
