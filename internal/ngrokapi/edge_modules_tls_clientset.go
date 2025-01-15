package ngrokapi

import (
	"github.com/ngrok/ngrok-api-go/v7"
	"github.com/ngrok/ngrok-api-go/v7/edge_modules/tls_edge_backend"
	"github.com/ngrok/ngrok-api-go/v7/edge_modules/tls_edge_ip_restriction"
	"github.com/ngrok/ngrok-api-go/v7/edge_modules/tls_edge_mutual_tls"
	"github.com/ngrok/ngrok-api-go/v7/edge_modules/tls_edge_tls_termination"
	"github.com/ngrok/ngrok-api-go/v7/edge_modules/tls_edge_traffic_policy"
)

type TLSEdgeModulesClientset interface {
	Backend() *tls_edge_backend.Client
	IPRestriction() *tls_edge_ip_restriction.Client
	MutualTLS() *tls_edge_mutual_tls.Client
	TLSTermination() *tls_edge_tls_termination.Client
	TrafficPolicy() *tls_edge_traffic_policy.Client
}

type defaultTLSEdgeModulesClientset struct {
	backend        *tls_edge_backend.Client
	ipRestriction  *tls_edge_ip_restriction.Client
	mutualTLS      *tls_edge_mutual_tls.Client
	tlsTermination *tls_edge_tls_termination.Client
	trafficPolicy  *tls_edge_traffic_policy.Client
}

func newTLSEdgeModulesClientset(config *ngrok.ClientConfig) *defaultTLSEdgeModulesClientset {
	return &defaultTLSEdgeModulesClientset{
		backend:        tls_edge_backend.NewClient(config),
		ipRestriction:  tls_edge_ip_restriction.NewClient(config),
		mutualTLS:      tls_edge_mutual_tls.NewClient(config),
		tlsTermination: tls_edge_tls_termination.NewClient(config),
		trafficPolicy:  tls_edge_traffic_policy.NewClient(config),
	}
}

func (c *defaultTLSEdgeModulesClientset) Backend() *tls_edge_backend.Client {
	return c.backend
}

func (c *defaultTLSEdgeModulesClientset) IPRestriction() *tls_edge_ip_restriction.Client {
	return c.ipRestriction
}

func (c *defaultTLSEdgeModulesClientset) MutualTLS() *tls_edge_mutual_tls.Client {
	return c.mutualTLS
}

func (c *defaultTLSEdgeModulesClientset) TLSTermination() *tls_edge_tls_termination.Client {
	return c.tlsTermination
}

func (c *defaultTLSEdgeModulesClientset) TrafficPolicy() *tls_edge_traffic_policy.Client {
	return c.trafficPolicy
}
