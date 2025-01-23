package ngrokapi

import (
	"github.com/ngrok/ngrok-api-go/v7"
	"github.com/ngrok/ngrok-api-go/v7/edge_modules/https_edge_mutual_tls"
	"github.com/ngrok/ngrok-api-go/v7/edge_modules/https_edge_route_backend"
	"github.com/ngrok/ngrok-api-go/v7/edge_modules/https_edge_route_circuit_breaker"
	"github.com/ngrok/ngrok-api-go/v7/edge_modules/https_edge_route_compression"
	"github.com/ngrok/ngrok-api-go/v7/edge_modules/https_edge_route_ip_restriction"
	"github.com/ngrok/ngrok-api-go/v7/edge_modules/https_edge_route_oauth"
	"github.com/ngrok/ngrok-api-go/v7/edge_modules/https_edge_route_oidc"
	"github.com/ngrok/ngrok-api-go/v7/edge_modules/https_edge_route_request_headers"
	"github.com/ngrok/ngrok-api-go/v7/edge_modules/https_edge_route_response_headers"
	"github.com/ngrok/ngrok-api-go/v7/edge_modules/https_edge_route_saml"
	"github.com/ngrok/ngrok-api-go/v7/edge_modules/https_edge_route_traffic_policy"
	"github.com/ngrok/ngrok-api-go/v7/edge_modules/https_edge_route_webhook_verification"
	"github.com/ngrok/ngrok-api-go/v7/edge_modules/https_edge_route_websocket_tcp_converter"
	"github.com/ngrok/ngrok-api-go/v7/edge_modules/https_edge_tls_termination"
)

type HTTPSEdgeModulesClientset interface {
	MutualTLS() HTTPSEdgeModulesMutualTLSClient
	Routes() HTTPSEdgeRouteModulesClientset
	TLSTermination() HTTPSEdgeModulesTLSTerminationClient
}

type (
	HTTPSEdgeModulesMutualTLSClient      = edgeModulesClient[*ngrok.EdgeMutualTLSReplace, *ngrok.EndpointMutualTLS]
	HTTPSEdgeModulesTLSTerminationClient = edgeModulesClient[*ngrok.EdgeTLSTerminationAtEdgeReplace, *ngrok.EndpointTLSTermination]
)

type defaultHTTPSEdgeModulesClientset struct {
	mutualTLS      *https_edge_mutual_tls.Client
	routes         *defaultHTTPSEdgeRouteModulesClientset
	tlsTermination *https_edge_tls_termination.Client
}

func newHTTPSEdgeModulesClientset(config *ngrok.ClientConfig) *defaultHTTPSEdgeModulesClientset {
	return &defaultHTTPSEdgeModulesClientset{
		mutualTLS:      https_edge_mutual_tls.NewClient(config),
		routes:         newHTTPSEdgeRouteModulesClient(config),
		tlsTermination: https_edge_tls_termination.NewClient(config),
	}
}

func (c *defaultHTTPSEdgeModulesClientset) MutualTLS() HTTPSEdgeModulesMutualTLSClient {
	return c.mutualTLS
}

func (c *defaultHTTPSEdgeModulesClientset) Routes() HTTPSEdgeRouteModulesClientset {
	return c.routes
}

func (c *defaultHTTPSEdgeModulesClientset) TLSTermination() HTTPSEdgeModulesTLSTerminationClient {
	return c.tlsTermination
}

type HTTPSEdgeRouteModulesClientset interface {
	Backend() HTTPSEdgeRouteBackendClient
	CircuitBreaker() HTTPSEdgeRouteCircuitBreakerClient
	Compression() HTTPSEdgeRouteCompressionClient
	IPRestriction() HTTPSEdgeRouteIPRestrictionClient
	OAuth() HTTPSEdgeRouteOAuthClient
	OIDC() HTTPSEdgeRouteOIDCClient
	RequestHeaders() HTTPSEdgeRouteRequestHeadersClient
	ResponseHeaders() HTTPSEdgeRouteResponseHeadersClient
	SAML() HTTPSEdgeRouteSAMLClient
	TrafficPolicy() HTTPSEdgeRouteTrafficPolicyClient
	WebhookVerification() HTTPSEdgeRouteWebhookVerificationClient
	WebsocketTCPConverter() HTTPSEdgeRouteWebsocketTCPConverterClient
}

type defaultHTTPSEdgeRouteModulesClientset struct {
	backend               *https_edge_route_backend.Client
	circuitBreaker        *https_edge_route_circuit_breaker.Client
	compression           *https_edge_route_compression.Client
	ipRestriction         *https_edge_route_ip_restriction.Client
	oauth                 *https_edge_route_oauth.Client
	trafficPolicy         *https_edge_route_traffic_policy.Client
	oidc                  *https_edge_route_oidc.Client
	requestHeaders        *https_edge_route_request_headers.Client
	responseHeaders       *https_edge_route_response_headers.Client
	saml                  *https_edge_route_saml.Client
	webhookVerification   *https_edge_route_webhook_verification.Client
	websocketTCPConverter *https_edge_route_websocket_tcp_converter.Client
}

func newHTTPSEdgeRouteModulesClient(config *ngrok.ClientConfig) *defaultHTTPSEdgeRouteModulesClientset {
	return &defaultHTTPSEdgeRouteModulesClientset{
		backend:               https_edge_route_backend.NewClient(config),
		circuitBreaker:        https_edge_route_circuit_breaker.NewClient(config),
		compression:           https_edge_route_compression.NewClient(config),
		ipRestriction:         https_edge_route_ip_restriction.NewClient(config),
		oauth:                 https_edge_route_oauth.NewClient(config),
		trafficPolicy:         https_edge_route_traffic_policy.NewClient(config),
		oidc:                  https_edge_route_oidc.NewClient(config),
		requestHeaders:        https_edge_route_request_headers.NewClient(config),
		responseHeaders:       https_edge_route_response_headers.NewClient(config),
		saml:                  https_edge_route_saml.NewClient(config),
		webhookVerification:   https_edge_route_webhook_verification.NewClient(config),
		websocketTCPConverter: https_edge_route_websocket_tcp_converter.NewClient(config),
	}
}

type httpsEdgeRouteModulesClient[R, T any] interface {
	EdgeRouteModulesDeletor
	EdgeRouteModulesReplacer[R, T]
}

type (
	HTTPSEdgeRouteBackendClient               = httpsEdgeRouteModulesClient[*ngrok.EdgeRouteBackendReplace, *ngrok.EndpointBackend]
	HTTPSEdgeRouteCircuitBreakerClient        = httpsEdgeRouteModulesClient[*ngrok.EdgeRouteCircuitBreakerReplace, *ngrok.EndpointCircuitBreaker]
	HTTPSEdgeRouteCompressionClient           = httpsEdgeRouteModulesClient[*ngrok.EdgeRouteCompressionReplace, *ngrok.EndpointCompression]
	HTTPSEdgeRouteIPRestrictionClient         = httpsEdgeRouteModulesClient[*ngrok.EdgeRouteIPRestrictionReplace, *ngrok.EndpointIPPolicy]
	HTTPSEdgeRouteOAuthClient                 = httpsEdgeRouteModulesClient[*ngrok.EdgeRouteOAuthReplace, *ngrok.EndpointOAuth]
	HTTPSEdgeRouteOIDCClient                  = httpsEdgeRouteModulesClient[*ngrok.EdgeRouteOIDCReplace, *ngrok.EndpointOIDC]
	HTTPSEdgeRouteRequestHeadersClient        = httpsEdgeRouteModulesClient[*ngrok.EdgeRouteRequestHeadersReplace, *ngrok.EndpointRequestHeaders]
	HTTPSEdgeRouteResponseHeadersClient       = httpsEdgeRouteModulesClient[*ngrok.EdgeRouteResponseHeadersReplace, *ngrok.EndpointResponseHeaders]
	HTTPSEdgeRouteSAMLClient                  = httpsEdgeRouteModulesClient[*ngrok.EdgeRouteSAMLReplace, *ngrok.EndpointSAML]
	HTTPSEdgeRouteTrafficPolicyClient         = httpsEdgeRouteModulesClient[*ngrok.EdgeRouteTrafficPolicyReplace, *ngrok.EndpointTrafficPolicy]
	HTTPSEdgeRouteWebhookVerificationClient   = httpsEdgeRouteModulesClient[*ngrok.EdgeRouteWebhookVerificationReplace, *ngrok.EndpointWebhookValidation]
	HTTPSEdgeRouteWebsocketTCPConverterClient = httpsEdgeRouteModulesClient[*ngrok.EdgeRouteWebsocketTCPConverterReplace, *ngrok.EndpointWebsocketTCPConverter]
)

func (c *defaultHTTPSEdgeRouteModulesClientset) Backend() HTTPSEdgeRouteBackendClient {
	return c.backend
}

func (c *defaultHTTPSEdgeRouteModulesClientset) CircuitBreaker() HTTPSEdgeRouteCircuitBreakerClient {
	return c.circuitBreaker
}

func (c *defaultHTTPSEdgeRouteModulesClientset) Compression() HTTPSEdgeRouteCompressionClient {
	return c.compression
}

func (c *defaultHTTPSEdgeRouteModulesClientset) IPRestriction() HTTPSEdgeRouteIPRestrictionClient {
	return c.ipRestriction
}

func (c *defaultHTTPSEdgeRouteModulesClientset) OAuth() HTTPSEdgeRouteOAuthClient {
	return c.oauth
}

func (c *defaultHTTPSEdgeRouteModulesClientset) TrafficPolicy() HTTPSEdgeRouteTrafficPolicyClient {
	return c.trafficPolicy
}

func (c *defaultHTTPSEdgeRouteModulesClientset) OIDC() HTTPSEdgeRouteOIDCClient {
	return c.oidc
}

func (c *defaultHTTPSEdgeRouteModulesClientset) RequestHeaders() HTTPSEdgeRouteRequestHeadersClient {
	return c.requestHeaders
}

func (c *defaultHTTPSEdgeRouteModulesClientset) ResponseHeaders() HTTPSEdgeRouteResponseHeadersClient {
	return c.responseHeaders
}

func (c *defaultHTTPSEdgeRouteModulesClientset) SAML() HTTPSEdgeRouteSAMLClient {
	return c.saml
}

func (c *defaultHTTPSEdgeRouteModulesClientset) WebhookVerification() HTTPSEdgeRouteWebhookVerificationClient {
	return c.webhookVerification
}

func (c *defaultHTTPSEdgeRouteModulesClientset) WebsocketTCPConverter() HTTPSEdgeRouteWebsocketTCPConverterClient {
	return c.websocketTCPConverter
}
