package ngrokapi

import (
	"github.com/ngrok/ngrok-api-go/v7"
	"github.com/ngrok/ngrok-api-go/v7/edge_modules/tcp_edge_backend"
	"github.com/ngrok/ngrok-api-go/v7/edge_modules/tcp_edge_ip_restriction"
	"github.com/ngrok/ngrok-api-go/v7/edge_modules/tcp_edge_traffic_policy"
)

type TCPEdgeModulesClientset interface {
	Backend() TCPEdgeModulesBackendClient
	IPRestriction() TCPEdgeModulesIPRestrictionClient
	TrafficPolicy() TCPEdgeModulesTrafficPolicyClient
}

type (
	TCPEdgeModulesBackendClient       = edgeModulesClient[*ngrok.EdgeBackendReplace, *ngrok.EndpointBackend]
	TCPEdgeModulesIPRestrictionClient = edgeModulesClient[*ngrok.EdgeIPRestrictionReplace, *ngrok.EndpointIPPolicy]
	TCPEdgeModulesTrafficPolicyClient = edgeModulesClient[*ngrok.EdgeTrafficPolicyReplace, *ngrok.EndpointTrafficPolicy]
)

type defaultTCPEdgeModulesClientset struct {
	backend       *tcp_edge_backend.Client
	ipRestriction *tcp_edge_ip_restriction.Client
	trafficPolicy *tcp_edge_traffic_policy.Client
}

func newTCPEdgeModulesClientset(config *ngrok.ClientConfig) *defaultTCPEdgeModulesClientset {
	return &defaultTCPEdgeModulesClientset{
		backend:       tcp_edge_backend.NewClient(config),
		ipRestriction: tcp_edge_ip_restriction.NewClient(config),
		trafficPolicy: tcp_edge_traffic_policy.NewClient(config),
	}
}

func (c *defaultTCPEdgeModulesClientset) Backend() TCPEdgeModulesBackendClient {
	return c.backend
}

func (c *defaultTCPEdgeModulesClientset) IPRestriction() TCPEdgeModulesIPRestrictionClient {
	return c.ipRestriction
}

func (c *defaultTCPEdgeModulesClientset) TrafficPolicy() TCPEdgeModulesTrafficPolicyClient {
	return c.trafficPolicy
}
