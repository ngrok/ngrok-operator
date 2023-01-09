package ngrokapi

import (
	"github.com/ngrok/ngrok-api-go/v5"
	tunnel_group_backends "github.com/ngrok/ngrok-api-go/v5/backends/tunnel_group"
	https_edges "github.com/ngrok/ngrok-api-go/v5/edges/https"
	https_edge_routes "github.com/ngrok/ngrok-api-go/v5/edges/https_routes"
	tcp_edges "github.com/ngrok/ngrok-api-go/v5/edges/tcp"
	"github.com/ngrok/ngrok-api-go/v5/reserved_domains"
)

type CLientset interface {
	Domains() *reserved_domains.Client
	HTTPSEdges() *https_edges.Client
	HTTPSEdgeRoutes() *https_edge_routes.Client
	TCPEdges() *tcp_edges.Client
	TunnelGroupBackends() *tunnel_group_backends.Client
}

type DefaultClientset struct {
	domainsClient             *reserved_domains.Client
	httpsEdgesClient          *https_edges.Client
	httpsEdgeRoutesClient     *https_edge_routes.Client
	tcpEdgesClient            *tcp_edges.Client
	tunnelGroupBackendsClient *tunnel_group_backends.Client
}

func NewClientSet(config *ngrok.ClientConfig) *DefaultClientset {
	return &DefaultClientset{
		domainsClient:             reserved_domains.NewClient(config),
		httpsEdgesClient:          https_edges.NewClient(config),
		httpsEdgeRoutesClient:     https_edge_routes.NewClient(config),
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

func (c *DefaultClientset) TCPEdges() *tcp_edges.Client {
	return c.tcpEdgesClient
}

func (c *DefaultClientset) TunnelGroupBackends() *tunnel_group_backends.Client {
	return c.tunnelGroupBackendsClient
}
