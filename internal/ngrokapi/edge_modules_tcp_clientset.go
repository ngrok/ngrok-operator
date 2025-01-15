package ngrokapi

import (
	"github.com/ngrok/ngrok-api-go/v7"
	"github.com/ngrok/ngrok-api-go/v7/edge_modules/tcp_edge_backend"
	"github.com/ngrok/ngrok-api-go/v7/edge_modules/tcp_edge_ip_restriction"
	"github.com/ngrok/ngrok-api-go/v7/edge_modules/tcp_edge_traffic_policy"
)

type TCPEdgeModulesClientset interface {
	Backend() *tcp_edge_backend.Client
	IPRestriction() *tcp_edge_ip_restriction.Client
	TrafficPolicy() *tcp_edge_traffic_policy.Client
}

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

func (c *defaultTCPEdgeModulesClientset) Backend() *tcp_edge_backend.Client {
	return c.backend
}

func (c *defaultTCPEdgeModulesClientset) IPRestriction() *tcp_edge_ip_restriction.Client {
	return c.ipRestriction
}

func (c *defaultTCPEdgeModulesClientset) TrafficPolicy() *tcp_edge_traffic_policy.Client {
	return c.trafficPolicy
}
