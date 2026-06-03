/*
MIT License

Copyright (c) 2024 ngrok, Inc.

Permission is hereby granted, free of charge, to any person obtaining a copy
of this software and associated documentation files (the "Software"), to deal
in the Software without restriction, including without limitation the rights
to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
copies of the Software, and to permit persons to whom the Software is
furnished to do so, subject to the following conditions:

The above copyright notice and this permission notice shall be included in all
copies or substantial portions of the Software.

THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
SOFTWARE.
*/

package v1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// AgentEndpoint condition types.
const (
	AgentEndpointConditionEndpointCreated = "EndpointCreated"
	AgentEndpointConditionTrafficPolicy   = "TrafficPolicy"
	AgentEndpointConditionDomainReady     = "DomainReady"
	AgentEndpointConditionReady           = "Ready"
)

// AgentEndpoint is the Schema for the agentendpoints API.
//
// +kubebuilder:object:root=true
// +kubebuilder:storageversion
// +kubebuilder:subresource:status
// +kubebuilder:resource:categories="networking";"ngrok"
// +kubebuilder:resource:shortName=aep
// +kubebuilder:printcolumn:name="URL",type="string",JSONPath=".spec.url"
// +kubebuilder:printcolumn:name="Upstream URL",type="string",JSONPath=".spec.upstream.url"
// +kubebuilder:printcolumn:name="Bindings",type="string",JSONPath=".spec.bindings"
// +kubebuilder:printcolumn:name="Ready",type="string",JSONPath=".status.conditions[?(@.type=='Ready')].status"
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`
// +kubebuilder:printcolumn:name="Reason",type="string",JSONPath=".status.conditions[?(@.type=='Ready')].reason",priority=1
// +kubebuilder:printcolumn:name="Message",type="string",JSONPath=".status.conditions[?(@.type=='Ready')].message",priority=1
type AgentEndpoint struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   AgentEndpointSpec   `json:"spec,omitempty"`
	Status AgentEndpointStatus `json:"status,omitempty"`
}

// EndpointUpstream defines where traffic delivered to an AgentEndpoint is forwarded.
type EndpointUpstream struct {
	// The local or remote address to forward incoming traffic to. Accepted formats:
	// Origin - https://example.org or http://example.org:80 or tcp://127.0.0.1:80
	// Domain - example.org (https/http only; port defaults to 80; scheme defaults to http)
	// Scheme (shorthand) - https:// (https/http only; host defaults to localhost)
	// Port (shorthand) - 8080 (scheme defaults to http; host defaults to localhost)
	//
	// +kubebuilder:validation:Required
	URL string `json:"url"`

	// Specifies the protocol to use when connecting to the upstream. Currently
	// only http1 and http2 are supported with prior knowledge (defaulting to
	// http1). ALPN negotiation is not currently supported.
	//
	// +kubebuilder:validation:Optional
	Protocol *ApplicationProtocol `json:"protocol,omitempty"`

	// Optionally specify the version of PROXY protocol to use if the upstream requires it.
	//
	// +kubebuilder:validation:Optional
	ProxyProtocolVersion *ProxyProtocolVersion `json:"proxyProtocolVersion,omitempty"`
}

// AgentEndpointSpec defines the desired state of an AgentEndpoint.
//
// +kubebuilder:validation:XValidation:rule="!has(self.tlsTermination) || self.url.startsWith('tls://')",message="spec.url must be a tls:// URL when tlsTermination is set"
type AgentEndpointSpec struct {
	// The unique URL for this agent endpoint. See CloudEndpoint.spec.url for accepted formats.
	//
	// +kubebuilder:validation:Required
	URL string `json:"url"`

	// Defines the destination for traffic to this AgentEndpoint.
	//
	// +kubebuilder:validation:Required
	Upstream EndpointUpstream `json:"upstream"`

	// Configures a TrafficPolicy to attach to this AgentEndpoint, either inline
	// or by reference. Exactly one of `inline` or `targetRef` must be specified.
	TrafficPolicy *TrafficPolicyCfg `json:"trafficPolicy,omitempty"`

	// Human-readable description of this agent endpoint.
	//
	// +kubebuilder:default:=`Created by the ngrok-operator`
	Description string `json:"description,omitempty"`

	// Arbitrary key/value metadata associated with the object in the ngrok API/Dashboard.
	//
	// +kubebuilder:default:={owned-by: ngrok-operator}
	Metadata map[string]string `json:"metadata,omitempty"`

	// List of Binding IDs to associate with the endpoint.
	// Accepted values are "public", "internal", or "kubernetes".
	//
	// +kubebuilder:validation:MaxItems=1
	// +kubebuilder:validation:items:Pattern=`^(public|internal|kubernetes)$`
	Bindings []string `json:"bindings,omitempty"`

	// List of client certificates to present to the upstream when performing a TLS handshake.
	ClientCertificateRefs []K8sObjectRefOptionalNamespace `json:"clientCertificateRefs,omitempty"`

	// TLSTermination configures the agent to terminate TLS in-cluster for
	// incoming traffic ("zero-knowledge TLS"). Requires spec.url to be a tls://
	// URL. Cannot be combined with the edge-side `terminate-tls` traffic-policy
	// action, which terminates at the edge before traffic reaches the agent.
	//
	// +kubebuilder:validation:Optional
	TLSTermination *EndpointTLSTermination `json:"tlsTermination,omitempty"`
}

// EndpointTLSTermination configures agent-side ("zero-knowledge") TLS termination.
type EndpointTLSTermination struct {
	// Reference to a kubernetes.io/tls Secret containing the server certificate
	// (tls.crt) and private key (tls.key) the agent will present to clients when
	// terminating TLS for incoming traffic. The Secret must live in the same
	// namespace as the AgentEndpoint.
	//
	// +kubebuilder:validation:Required
	ServerCertificateRef K8sObjectRef `json:"serverCertificateRef"`

	// Optional mutual-TLS configuration. When set, the agent will require or
	// request client certificates during the TLS handshake and validate them
	// against the supplied CA bundle.
	//
	// +kubebuilder:validation:Optional
	MutualTLS *EndpointMutualTLS `json:"mutualTLS,omitempty"`
}

// EndpointMutualTLS configures client-certificate verification at the agent.
type EndpointMutualTLS struct {
	// Reference to a Secret whose `ca.crt` key contains a PEM-encoded bundle of
	// certificate authorities trusted to sign client certificates. The Secret
	// must live in the same namespace as the AgentEndpoint.
	//
	// +kubebuilder:validation:Required
	ClientCAsRef K8sObjectRef `json:"clientCAsRef"`

	// Mode selects the TLS client-auth policy:
	//   require - reject the handshake if no valid client cert is presented
	//   request - request a client cert but do not require one
	//
	// +kubebuilder:validation:Enum=require;request
	// +kubebuilder:default=require
	Mode EndpointMutualTLSMode `json:"mode,omitempty"`
}

// EndpointMutualTLSMode selects the agent's client-certificate verification mode.
type EndpointMutualTLSMode string

const (
	EndpointMutualTLSModeRequire EndpointMutualTLSMode = "require"
	EndpointMutualTLSModeRequest EndpointMutualTLSMode = "request"
)

// AgentEndpointStatus defines the observed state of an AgentEndpoint.
type AgentEndpointStatus struct {
	// AssignedURL is the URL assigned by ngrok (may differ from spec.url when
	// the user supplied a scheme-only or empty URL).
	AssignedURL string `json:"assignedURL,omitempty"`

	// AttachedTrafficPolicy reports which TrafficPolicy is currently attached:
	// "none", "inline", or the name of the referenced policy.
	AttachedTrafficPolicy string `json:"attachedTrafficPolicy,omitempty"`

	// DomainRef references the Domain resource associated with this endpoint.
	// For internal endpoints this is nil.
	//
	// +kubebuilder:validation:Optional
	// +nullable
	DomainRef *K8sObjectRefOptionalNamespace `json:"domainRef"`

	// Conditions describe the current state of the AgentEndpoint.
	//
	// +listType=map
	// +listMapKey=type
	// +kubebuilder:validation:MaxItems=8
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// AgentEndpointList contains a list of AgentEndpoints.
//
// +kubebuilder:object:root=true
type AgentEndpointList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []AgentEndpoint `json:"items"`
}

// GetConditions returns a pointer to the conditions slice for AgentEndpoint.
func (a *AgentEndpoint) GetConditions() *[]metav1.Condition {
	return &a.Status.Conditions
}

// GetGeneration returns the generation for AgentEndpoint.
func (a *AgentEndpoint) GetGeneration() int64 {
	return a.Generation
}

// GetDomainRef returns the domain reference for AgentEndpoint.
func (a *AgentEndpoint) GetDomainRef() *K8sObjectRefOptionalNamespace {
	return a.Status.DomainRef
}

// SetDomainRef sets the domain reference for AgentEndpoint.
func (a *AgentEndpoint) SetDomainRef(ref *K8sObjectRefOptionalNamespace) {
	a.Status.DomainRef = ref
}

// GetURL returns the URL for the AgentEndpoint.
func (a *AgentEndpoint) GetURL() string {
	return a.Spec.URL
}

// GetBindings returns the bindings for the AgentEndpoint.
func (a *AgentEndpoint) GetBindings() []string {
	return a.Spec.Bindings
}

func init() {
	SchemeBuilder.Register(&AgentEndpoint{}, &AgentEndpointList{})
}
