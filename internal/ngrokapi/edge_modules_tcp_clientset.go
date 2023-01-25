package ngrokapi

import (
	"github.com/ngrok/ngrok-api-go/v5"
	"github.com/ngrok/ngrok-api-go/v5/edge_modules/tcp_edge_backend"
	"github.com/ngrok/ngrok-api-go/v5/edge_modules/tcp_edge_ip_restriction"
)

type TCPEdgeModulesClientset interface {
	Backend() *tcp_edge_backend.Client
	IPRestriction() *tcp_edge_ip_restriction.Client
}

type defaultTCPEdgeModulesClientset struct {
	backend       *tcp_edge_backend.Client
	ipRestriction *tcp_edge_ip_restriction.Client
}

func newTCPEdgeModulesClientset(config *ngrok.ClientConfig) *defaultTCPEdgeModulesClientset {
	return &defaultTCPEdgeModulesClientset{
		backend:       tcp_edge_backend.NewClient(config),
		ipRestriction: tcp_edge_ip_restriction.NewClient(config),
	}
}

func (c *defaultTCPEdgeModulesClientset) Backend() *tcp_edge_backend.Client {
	return c.backend
}

func (c *defaultTCPEdgeModulesClientset) IPRestriction() *tcp_edge_ip_restriction.Client {
	return c.ipRestriction
}
