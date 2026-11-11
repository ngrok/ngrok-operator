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

// LEGACY-agentendpoint-kind: delete this whole file at cleanup, then re-run
// `make manifests` and delete the CRD manifest it stops emitting,
// helm/ngrok-crds/templates/ngrok.k8s.ngrok.com_agentendpoints.yaml. The
// canonical replacement is api/ngrok/v1.AgentEndpoint. See
// docs/developer-guide/passivity-shims.md.

package v1alpha1

import (
	"encoding/json"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	ngrokv1 "github.com/ngrok/ngrok-operator/api/ngrok/v1"
)

// AgentEndpoint is the Schema for the agentendpoints API
//
// Deprecated: use the canonical ngrok.com/v1 AgentEndpoint. This kind is still
// served and reconciled for one migration release; see
// docs/developer-guide/passivity-shims.md.
//
// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:deprecatedversion:warning="ngrok.k8s.ngrok.com/v1alpha1 AgentEndpoint is deprecated; use ngrok.com/v1 AgentEndpoint. Re-stamp the apiVersion (spec.metadata must be a map), apply it, wait until Ready, then delete this object."
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

// The spec's sub-structs are shared with ngrok.com/v1, so a legacy spec
// converts to the driver's input without a field-by-field copy.
type (
	EndpointUpstream       = ngrokv1.EndpointUpstream
	EndpointTLSTermination = ngrokv1.EndpointTLSTermination
	EndpointMutualTLS      = ngrokv1.EndpointMutualTLS
	EndpointMutualTLSMode  = ngrokv1.EndpointMutualTLSMode
)

const (
	EndpointMutualTLSModeRequire = ngrokv1.EndpointMutualTLSModeRequire
	EndpointMutualTLSModeRequest = ngrokv1.EndpointMutualTLSModeRequest
)

// AgentEndpointSpec defines the desired state of an AgentEndpoint
//
// +kubebuilder:validation:XValidation:rule="!has(self.tlsTermination) || self.url.startsWith('tls://')",message="spec.url must be a tls:// URL when tlsTermination is set"
type AgentEndpointSpec struct {
	// The unique URL for this agent endpoint. This URL is the public address. The following formats are accepted
	// Domain - example.org
	//     When using the domain format you are only defining the domain. The scheme and port will be inferred.
	// Origin - https://example.ngrok.app or https://example.ngrok.app:443 or tcp://1.tcp.ngrok.io:12345 or tls://example.ngrok.app
	//     When using the origin format you are defining the protocol, domain and port. HTTP endpoints accept ports 80 or 443 with respective protocol.
	// Scheme (shorthand) - https:// or tcp:// or tls:// or http://
	//     When using scheme you are defining the protocol and will receive back a randomly assigned ngrok address.
	// Empty - ``
	//     When empty your endpoint will default to be https and receive back a randomly assigned ngrok address.
	// Internal - some.domain.internal
	//     When ending your url with .internal, an internal endpoint will be created. nternal Endpoints cannot be accessed directly, but rather
	//     can only be accessed using the forward-internal traffic policy action.
	//
	// +kubebuilder:validation:Required
	URL string `json:"url"`

	// Defines the destination for traffic to this AgentEndpoint
	//
	// +kubebuilder:validation:Required
	Upstream EndpointUpstream `json:"upstream"`

	// Allows configuring a TrafficPolicy to be used with this AgentEndpoint
	// When configured, the traffic policy is provided inline or as a reference to an NgrokTrafficPolicy resource
	TrafficPolicy *TrafficPolicyCfg `json:"trafficPolicy,omitempty"`

	// Human-readable description of this agent endpoint
	//
	// +kubebuilder:default=`Created by the ngrok-operator`
	Description string `json:"description,omitempty"`

	// Metadata is arbitrary key/value data associated with the object in the
	// ngrok API/Dashboard. A raw JSON string is also accepted for backward
	// compatibility and is deprecated; use a map of string values instead.
	// The ngrok.metadata Helm value is not merged into this field.
	//
	// +kubebuilder:validation:Schemaless
	// +kubebuilder:pruning:PreserveUnknownFields
	// +kubebuilder:default=`{"owned-by":"ngrok-operator"}`
	Metadata json.RawMessage `json:"metadata,omitempty"`

	// List of Binding IDs to associate with the endpoint
	// Accepted values are "public", "internal", or "kubernetes"
	//
	// The ngrok API currently supports a single binding per endpoint, so this
	// list is capped at one item. It is a list (rather than a scalar) so that
	// multiple bindings can be supported in the future without a breaking
	// schema change.
	//
	// +kubebuilder:validation:MaxItems=1
	// +kubebuilder:validation:items:Pattern=`^(public|internal|kubernetes)$`
	Bindings []string `json:"bindings,omitempty"`

	// List of references to kubernetes.io/tls Secrets containing the client
	// certificates to present to the upstream when performing a TLS handshake.
	// Each Secret must contain the certificate under the `tls.crt` key and its
	// private key under the `tls.key` key, and must live in the same namespace
	// as the AgentEndpoint.
	ClientCertificateRefs []K8sObjectRef `json:"clientCertificateRefs,omitempty"`

	// TLSTermination configures the agent to terminate TLS in-cluster for incoming
	// traffic ("zero-knowledge TLS"). When set, ngrok's edge routes the encrypted
	// stream to the agent based on SNI; the TLS handshake completes between the
	// client and the agent, using the server certificate referenced here. The agent
	// then forwards plaintext (or re-establishes TLS, depending on
	// spec.upstream.url) to the upstream.
	//
	// Requires spec.url to be a tls:// URL. Cannot be combined with the edge-side
	// `terminate-tls` traffic-policy action, which terminates at the edge before
	// traffic reaches the agent.
	//
	// +kubebuilder:validation:Optional
	TLSTermination *EndpointTLSTermination `json:"tlsTermination,omitempty"`
}

// AgentEndpointStatus is shared with ngrok.com/v1; the status shape didn't
// change.
type AgentEndpointStatus = ngrokv1.AgentEndpointStatus

// AgentEndpointList contains a list of AgentEndpoints
//
// +kubebuilder:object:root=true
type AgentEndpointList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []AgentEndpoint `json:"items"`
}

// EndpointWithDomain implementation for AgentEndpoint
var _ EndpointWithDomain = &AgentEndpoint{}

// EndpointWithTrafficPolicy implementation for AgentEndpoint
var _ EndpointWithTrafficPolicy = &AgentEndpoint{}

// GetTrafficPolicyCfg returns the traffic policy configuration for AgentEndpoint
func (a *AgentEndpoint) GetTrafficPolicyCfg() *TrafficPolicyCfg {
	return a.Spec.TrafficPolicy
}

// GetConditions returns a pointer to the conditions slice for AgentEndpoint
func (a *AgentEndpoint) GetConditions() *[]metav1.Condition {
	return &a.Status.Conditions
}

// GetGeneration returns the generation for AgentEndpoint
func (a *AgentEndpoint) GetGeneration() int64 {
	return a.Generation
}

// SetObservedGeneration records the generation the controller reconciled.
func (a *AgentEndpoint) SetObservedGeneration(generation int64) {
	a.Status.ObservedGeneration = generation
}

// GetDomainRef returns the domain reference for AgentEndpoint
func (a *AgentEndpoint) GetDomainRef() *K8sObjectRefOptionalNamespace {
	return a.Status.DomainRef
}

// SetDomainRef sets the domain reference for AgentEndpoint
func (a *AgentEndpoint) SetDomainRef(ref *K8sObjectRefOptionalNamespace) {
	a.Status.DomainRef = ref
}

// GetURL returns the URL for the AgentEndpoint
func (a *AgentEndpoint) GetURL() string {
	return a.Spec.URL
}

// GetBindings returns the bindings for the AgentEndpoint
func (a *AgentEndpoint) GetBindings() []string {
	return a.Spec.Bindings
}
