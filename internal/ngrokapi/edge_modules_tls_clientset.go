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
	Backend() TLSEdgeModulesBackendClient
	IPRestriction() TLSEdgeModulesIPRestrictionClient
	MutualTLS() TLSEdgeModulesMutualTLSClient
	TLSTermination() TLSEdgeModulesTLSTerminationClient
	TrafficPolicy() TLSEdgeModulesTrafficPolicyClient
}

type (
	TLSEdgeModulesBackendClient        = edgeModulesClient[*ngrok.EdgeBackendReplace, *ngrok.EndpointBackend]
	TLSEdgeModulesIPRestrictionClient  = edgeModulesClient[*ngrok.EdgeIPRestrictionReplace, *ngrok.EndpointIPPolicy]
	TLSEdgeModulesMutualTLSClient      = edgeModulesClient[*ngrok.EdgeMutualTLSReplace, *ngrok.EndpointMutualTLS]
	TLSEdgeModulesTLSTerminationClient = edgeModulesClient[*ngrok.EdgeTLSTerminationReplace, *ngrok.EndpointTLSTermination]
	TLSEdgeModulesTrafficPolicyClient  = edgeModulesClient[*ngrok.EdgeTrafficPolicyReplace, *ngrok.EndpointTrafficPolicy]
)

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

func (c *defaultTLSEdgeModulesClientset) Backend() TLSEdgeModulesBackendClient {
	return c.backend
}

func (c *defaultTLSEdgeModulesClientset) IPRestriction() TLSEdgeModulesIPRestrictionClient {
	return c.ipRestriction
}

func (c *defaultTLSEdgeModulesClientset) MutualTLS() TLSEdgeModulesMutualTLSClient {
	return c.mutualTLS
}

func (c *defaultTLSEdgeModulesClientset) TLSTermination() TLSEdgeModulesTLSTerminationClient {
	return c.tlsTermination
}

func (c *defaultTLSEdgeModulesClientset) TrafficPolicy() TLSEdgeModulesTrafficPolicyClient {
	return c.trafficPolicy
}
