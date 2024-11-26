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

package v1alpha1

import (
	"encoding/json"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// AgentEndpoint is the Schema for the agentendpoints API
//
// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:categories="networking";"ngrok"
// +kubebuilder:resource:shortName=aep
// +kubebuilder:printcolumn:name="URL",type="string",JSONPath=".spec.url"
// +kubebuilder:printcolumn:name="Upstream URL",type="string",JSONPath=".spec.upstream.url"
// +kubebuilder:printcolumn:name="Bindings",type="string",JSONPath=".spec.bindings"
// +kubebuilder:printcolumn:name="Status",type="string",JSONPath=".status.conditions[?(@.type=='Status')].status"
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`
type AgentEndpoint struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec AgentEndpointSpec `json:"spec,omitempty"`
	// Status AgentEndpointStatus `json:"status,omitempty"` Status subresource not yet supported
}

type UpstreamProtocol string

const (
	UpstreamProtocol_HTTP1 UpstreamProtocol = "http1"
	UpstreamProtocol_HTTP2 UpstreamProtocol = "http2"
)

func (t UpstreamProtocol) IsKnown() bool {
	switch t {
	case UpstreamProtocol_HTTP1, UpstreamProtocol_HTTP2:
		return true
	default:
		return false
	}
}

type ProxyProtocolVersion string

const (
	ProxyProtocolVersion_1 ProxyProtocolVersion = "1"
	ProxyProtocolVersion_2 ProxyProtocolVersion = "2"
)

func (t ProxyProtocolVersion) IsKnown() bool {
	switch t {
	case ProxyProtocolVersion_1, ProxyProtocolVersion_2:
		return true
	default:
		return false
	}
}

type EndpointUpstream struct {
	// The local or remote address you would like to incoming traffic to be forwarded to. Accepted formats are:
	// Origin - https://example.org or http://example.org:80 or tcp://127.0.0.1:80
	//     When using the origin format you are defining the protocol, domain and port.
	//         When no port is present and scheme is https or http the port will be inferred.
	//             For https port will be443.
	//             For http port will be 80.
	// Domain - example.org
	//     This is only allowed for https and http endpoints.
	//         For tcp and tls endpoints host and port is required.
	//     When using the domain format you are only defining the host.
	//         Scheme will default to http.
	//         Port will default to 80.
	// Scheme (shorthand) - https://
	//     This only works for https and http.
	//         For tcp and tls host and port is required.
	//     When using scheme you are defining the protocol and the port will be inferred on the local host.
	//         For https port will be443.
	//         For http port will be 80.
	//         Host will be localhost.
	// Port (shorthand) - 8080
	//     When using port you are defining the port on the local host that will receive traffic.
	//         Scheme will default to http.
	//         Host will default to localhost.
	//
	// +kubebuilder:validation:Required
	URL string `json:"url"`

	// Specifies the protocol to use when connecting to the upstream. Currently only http1 and http2 are supported
	// with prior knowledge (defaulting to http1). alpn negotiation is not currently supported.
	//
	// +kubebuilder:validation:Enum=http1;http2
	// +kubebuilder:validation:Required
	// +kubebuilder:default=http1
	Protocol UpstreamProtocol `json:"protocol"`

	// Optionally specify the version of proxy protocol to use if the upstream requires it
	//
	// +kubebuilder:validation:Optional
	// +kubebuilder:validation:Enum=1;2
	ProxyProtocolVersion *ProxyProtocolVersion `json:"proxyProtocolVersion"`
}

// AgentEndpointSpec defines the desired state of an AgentEndpoint
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
	//
	// +kubebuilder:validation:Optional
	TrafficPolicy *TrafficPolicyCfg `json:"trafficPolicy,omitempty"`

	// Human-readable description of this agent endpoint
	//
	// +kubebuilder:default=`Created by the ngrok-operator`
	Description string `json:"description,omitempty"`

	// String of arbitrary data associated with the object in the ngrok API/Dashboard
	//
	// +kubebuilder:default=`{"owned-by":"ngrok-operator"}`
	Metadata string `json:"metadata,omitempty"`

	// List of Binding IDs to associate with the endpoint
	// Accepted values are "public", "internal", or strings matching the pattern "k8s/*"
	//
	// +kubebuilder:validation:Optional
	// +kubebuilder:validation:Items=pattern=`^(public|internal|k8s/.*)$`
	Bindings []string `json:"bindings,omitempty"`
}

type TrafficPolicyCfgType string

const (
	TrafficPolicyCfgType_K8sRef TrafficPolicyCfgType = "targetRef"
	TrafficPolicyCfgType_Inline TrafficPolicyCfgType = "inline"
)

func (t TrafficPolicyCfgType) IsKnown() bool {
	switch t {
	case TrafficPolicyCfgType_K8sRef, TrafficPolicyCfgType_Inline:
		return true
	default:
		return false
	}
}

// +kubebuilder:validation:XValidation:rule="self.type == 'inline' ? has(self.inline) && !has(self.targetRef) : true",message="When type is inline, inline must be set, and targetRef must not be set."
// +kubebuilder:validation:XValidation:rule="self.type == 'targetRef' ? has(self.targetRef) && !has(self.inline) : true",message="When type is targetRef, targetRef must be set, and inline must not be set."
type TrafficPolicyCfg struct {
	// Controls the way that the TrafficPolicy configuration will be provided to the Agent Endpoint
	//
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:Enum=targetRef;inline
	Type TrafficPolicyCfgType `json:"type"`

	// Inline definition of a TrafficPolicy to attach to the agent Endpoint
	// The raw json encoded policy that was applied to the ngrok API
	//
	// +kubebuilder:validation:Optional
	// +kubebuilder:validation:Schemaless
	// +kubebuilder:pruning:PreserveUnknownFields
	// +kubebuilder:validation:Type=object
	Inline json.RawMessage `json:"inline,omitempty"`

	// Reference to a TrafficPolicy resource to attach to the Agent Endpoint
	//
	// +kubebuilder:validation:Optional
	Reference *K8sObjectRef `json:"targetRef,omitempty"`
}

// AgentEndpointStatus defines the observed state of an AgentEndpoint
type AgentEndpointStatus struct {
	// The unique identifier for this endpoint
	ID string `json:"id,omitempty"`

	// The assigned URL. This will either be the user-supplied url, or the generated assigned url
	// depending on the configuration of spec.url
	//
	// +kubebuilder:validation:Optional
	AssignedURL string `json:"assignedURL,omitempty"`

	// Identifies any traffic policies attached to the AgentEndpoint ("inline", "none", or reference name).
	//
	// +kubebuilder:validation:Optional
	AttachedTrafficPolicy string `json:"trafficPolicy,omitempty"`

	// Conditions describe the current conditions of the AgentEndpoint.
	//
	// +kubebuilder:validation:Optional
	// +listType=map
	// +listMapKey=type
	// +kubebuilder:validation:MaxItems=8
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// AgentEndpointList contains a list of AgentEndpoints
//
// +kubebuilder:object:root=true
type AgentEndpointList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []AgentEndpoint `json:"items"`
}

func init() {
	SchemeBuilder.Register(&AgentEndpoint{}, &AgentEndpointList{})
}
