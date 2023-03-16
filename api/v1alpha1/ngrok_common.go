package v1alpha1

import (
	"k8s.io/apimachinery/pkg/api/resource"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// common ngrok API/Dashboard fields
type ngrokAPICommon struct {
	// Description is a human-readable description of the object in the ngrok API/Dashboard
	// +kubebuilder:default:=`Created by kubernetes-ingress-controller`
	Description string `json:"description,omitempty"`
	// Metadata is a string of arbitrary data associated with the object in the ngrok API/Dashboard
	// +kubebuilder:default:=`{"owned-by":"kubernetes-ingress-controller"}`
	Metadata string `json:"metadata,omitempty"`
}

// Route Module Types

type EndpointCompression struct {
	// Enabled is whether or not to enable compression for this endpoint
	Enabled bool `json:"enabled,omitempty"`
}

type EndpointIPPolicy struct {
	IPPolicies []string `json:"policies,omitempty"`
}

// EndpointRequestHeaders is the configuration for a HTTPSEdgeRoute's request headers
// to be added or removed from the request before it is sent to the backend service.
type EndpointRequestHeaders struct {
	// a map of header key to header value that will be injected into the HTTP Request
	// before being sent to the upstream application server
	Add map[string]string `json:"add,omitempty"`
	// a list of header names that will be removed from the HTTP Request before being
	// sent to the upstream application server
	Remove []string `json:"remove,omitempty"`
}

// EndpointResponseHeaders is the configuration for a HTTPSEdgeRoute's response headers
// to be added or removed from the response before it is sent to the client.
type EndpointResponseHeaders struct {
	// a map of header key to header value that will be injected into the HTTP Response
	// returned to the HTTP client
	Add map[string]string `json:"add,omitempty"`
	// a list of header names that will be removed from the HTTP Response returned to
	// the HTTP client
	Remove []string `json:"remove,omitempty"`
}

type EndpointHeaders struct {
	// Request headers are the request headers module configuration or null
	Request *EndpointRequestHeaders `json:"request,omitempty"`
	// Response headers are the response headers module configuration or null
	Response *EndpointResponseHeaders `json:"response,omitempty"`
}

type EndpointTLSTerminationAtEdge struct {
	// MinVersion is the minimum TLS version to allow for connections to the edge
	MinVersion string `json:"minVersion,omitempty"`
}

type SecretKeyRef struct {
	// Name of the Kubernetes secret
	Name string `json:"name,omitempty"`
	// Key in the secret to use
	Key string `json:"key,omitempty"`
}

type EndpointWebhookVerification struct {
	// a string indicating which webhook provider will be sending webhooks to this
	// endpoint. Value must be one of the supported providers defined at
	// https://ngrok.com/docs/cloud-edge#webhook-verification
	Provider string `json:"provider,omitempty"`
	// SecretRef is a reference to a secret containing the secret used to validate
	// requests from the given provider. All providers except AWS SNS require a secret
	SecretRef *SecretKeyRef `json:"secret,omitempty"`
}

type EndpointCircuitBreaker struct {
	// Integer number of seconds after which the circuit is tripped to wait before
	// re-evaluating upstream health
	TrippedDuration uint32 `json:"trippedDuration,omitempty"`

	// Integer number of seconds in the statistical rolling window that metrics are
	// retained for.
	RollingWindow uint32 `json:"rollingWindow,omitempty"`

	// Integer number of buckets into which metrics are retained. Max 128.
	//+kubebuilder:validation:Minimum=1
	//+kubebuilder:validation:Maximum=128
	NumBuckets uint32 `json:"numBuckets,omitempty"`

	// Integer number of requests in a rolling window that will trip the circuit.
	// Helpful if traffic volume is low.
	VolumeThreshold uint32 `json:"volumeThreshold,omitempty"`

	// Error threshold percentage should be between 0 - 1.0, not 0-100.0
	ErrorThresholdPercentage resource.Quantity `json:"errorThresholdPercentage,omitempty"`
}

type EndpointOIDC struct {
	// Do not enforce authentication on HTTP OPTIONS requests. necessary if you are
	// supporting CORS.
	OptionsPassthrough bool `json:"optionsPassthrough,omitempty"`
	// the prefix of the session cookie that ngrok sets on the http client to cache
	// authentication. default is 'ngrok.'
	CookiePrefix string `json:"cookiePrefix,omitempty"`
	// Duration of inactivity after which if the user has not accessed
	// the endpoint, their session will time out and they will be forced to
	// reauthenticate.
	//+kubebuilder:validation:Format=duration
	InactivityTimeout v1.Duration `json:"inactivityTimeout,omitempty"`
	// The maximum duration of an authenticated session.
	// After this period is exceeded, a user must reauthenticate.
	//+kubebuilder:validation:Format=duration
	MaximumDuration v1.Duration `json:"maximumDuration,omitempty"`
	// URL of the OIDC "OpenID provider". This is the base URL used for discovery.
	Issuer string `json:"issuer,omitempty"`
	// The OIDC app's client ID and OIDC audience.
	ClientID string `json:"clientId,omitempty"`
	// The OIDC app's client secret.
	ClientSecret SecretKeyRef `json:"clientSecret,omitempty"`
	// The set of scopes to request from the OIDC identity provider.
	Scopes []string `json:"scopes,omitempty"`
}
