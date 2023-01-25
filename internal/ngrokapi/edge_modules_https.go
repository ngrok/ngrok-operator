package ngrokapi

import (
	"github.com/ngrok/ngrok-api-go/v5"
	"github.com/ngrok/ngrok-api-go/v5/edge_modules/https_edge_mutual_tls"
	"github.com/ngrok/ngrok-api-go/v5/edge_modules/https_edge_route_backend"
	"github.com/ngrok/ngrok-api-go/v5/edge_modules/https_edge_route_circuit_breaker"
	"github.com/ngrok/ngrok-api-go/v5/edge_modules/https_edge_route_compression"
	"github.com/ngrok/ngrok-api-go/v5/edge_modules/https_edge_route_ip_restriction"
	"github.com/ngrok/ngrok-api-go/v5/edge_modules/https_edge_route_oauth"
	"github.com/ngrok/ngrok-api-go/v5/edge_modules/https_edge_route_oidc"
	"github.com/ngrok/ngrok-api-go/v5/edge_modules/https_edge_route_request_headers"
	"github.com/ngrok/ngrok-api-go/v5/edge_modules/https_edge_route_response_headers"
	"github.com/ngrok/ngrok-api-go/v5/edge_modules/https_edge_route_saml"
	"github.com/ngrok/ngrok-api-go/v5/edge_modules/https_edge_route_webhook_verification"
	"github.com/ngrok/ngrok-api-go/v5/edge_modules/https_edge_route_websocket_tcp_converter"
	"github.com/ngrok/ngrok-api-go/v5/edge_modules/https_edge_tls_termination"
)

type HTTPSEdgeModulesClientset interface {
	MutualTLS() *https_edge_mutual_tls.Client
	Routes() HTTPSEdgeRouteModulesClientset
	TLSTermination() *https_edge_tls_termination.Client
}

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

func (c *defaultHTTPSEdgeModulesClientset) MutualTLS() *https_edge_mutual_tls.Client {
	return c.mutualTLS
}

func (c *defaultHTTPSEdgeModulesClientset) Routes() HTTPSEdgeRouteModulesClientset {
	return c.routes
}

func (c *defaultHTTPSEdgeModulesClientset) TLSTermination() *https_edge_tls_termination.Client {
	return c.tlsTermination
}

type HTTPSEdgeRouteModulesClientset interface {
	Backend() *https_edge_route_backend.Client
	CircuitBreaker() *https_edge_route_circuit_breaker.Client
	Compression() *https_edge_route_compression.Client
	IPRestriction() *https_edge_route_ip_restriction.Client
	OAuth() *https_edge_route_oauth.Client
	OIDC() *https_edge_route_oidc.Client
	RequestHeaders() *https_edge_route_request_headers.Client
	ResponseHeaders() *https_edge_route_response_headers.Client
	SAML() *https_edge_route_saml.Client
	WebhookVerification() *https_edge_route_webhook_verification.Client
	WebsocketTCPConverter() *https_edge_route_websocket_tcp_converter.Client
}

type defaultHTTPSEdgeRouteModulesClientset struct {
	backend               *https_edge_route_backend.Client
	circuitBreaker        *https_edge_route_circuit_breaker.Client
	compression           *https_edge_route_compression.Client
	ipRestriction         *https_edge_route_ip_restriction.Client
	oauth                 *https_edge_route_oauth.Client
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
		oidc:                  https_edge_route_oidc.NewClient(config),
		requestHeaders:        https_edge_route_request_headers.NewClient(config),
		responseHeaders:       https_edge_route_response_headers.NewClient(config),
		saml:                  https_edge_route_saml.NewClient(config),
		webhookVerification:   https_edge_route_webhook_verification.NewClient(config),
		websocketTCPConverter: https_edge_route_websocket_tcp_converter.NewClient(config),
	}
}

func (c *defaultHTTPSEdgeRouteModulesClientset) Backend() *https_edge_route_backend.Client {
	return c.backend
}

func (c *defaultHTTPSEdgeRouteModulesClientset) CircuitBreaker() *https_edge_route_circuit_breaker.Client {
	return c.circuitBreaker
}

func (c *defaultHTTPSEdgeRouteModulesClientset) Compression() *https_edge_route_compression.Client {
	return c.compression
}

func (c *defaultHTTPSEdgeRouteModulesClientset) IPRestriction() *https_edge_route_ip_restriction.Client {
	return c.ipRestriction
}

func (c *defaultHTTPSEdgeRouteModulesClientset) OAuth() *https_edge_route_oauth.Client {
	return c.oauth
}

func (c *defaultHTTPSEdgeRouteModulesClientset) OIDC() *https_edge_route_oidc.Client {
	return c.oidc
}

func (c *defaultHTTPSEdgeRouteModulesClientset) RequestHeaders() *https_edge_route_request_headers.Client {
	return c.requestHeaders
}

func (c *defaultHTTPSEdgeRouteModulesClientset) ResponseHeaders() *https_edge_route_response_headers.Client {
	return c.responseHeaders
}

func (c *defaultHTTPSEdgeRouteModulesClientset) SAML() *https_edge_route_saml.Client {
	return c.saml
}

func (c *defaultHTTPSEdgeRouteModulesClientset) WebhookVerification() *https_edge_route_webhook_verification.Client {
	return c.webhookVerification
}

func (c *defaultHTTPSEdgeRouteModulesClientset) WebsocketTCPConverter() *https_edge_route_websocket_tcp_converter.Client {
	return c.websocketTCPConverter
}
