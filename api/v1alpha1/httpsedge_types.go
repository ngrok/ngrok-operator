/*
MIT License

Copyright (c) 2022 ngrok, Inc.

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
	"reflect"

	"github.com/ngrok/ngrok-api-go/v5"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

type HTTPSEdgeRouteSpec struct {
	ngrokAPICommon `json:",inline"`

	// MatchType is the type of match to use for this route. Valid values are:
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:Enum=exact_path;path_prefix
	MatchType string `json:"matchType"`

	// Match is the value to match against the request path
	// +kubebuilder:validation:Required
	Match string `json:"match"`

	// Backend is the definition for the tunnel group backend
	// that serves traffic for this edge
	// +kubebuilder:validation:Required
	Backend TunnelGroupBackend `json:"backend,omitempty"`

	// CircuitBreaker is a circuit breaker configuration to apply to this route
	CircuitBreaker *EndpointCircuitBreaker `json:"circuitBreaker,omitempty"`

	// Compression is whether or not to enable compression for this route
	Compression *EndpointCompression `json:"compression,omitempty"`

	// IPRestriction is an IPRestriction to apply to this route
	IPRestriction *EndpointIPPolicy `json:"ipRestriction,omitempty"`

	// Headers are request/response headers to apply to this route
	Headers *EndpointHeaders `json:"headers,omitempty"`

	// OAuth configuration to apply to this route
	OAuth *EndpointOAuth `json:"oauth,omitempty"`

	// OIDC is the OpenID Connect configuration to apply to this route
	OIDC *EndpointOIDC `json:"oidc,omitempty"`

	// SAML is the SAML configuration to apply to this route
	SAML *EndpointSAML `json:"saml,omitempty"`

	// WebhookVerification is webhook verification configuration to apply to this route
	WebhookVerification *EndpointWebhookVerification `json:"webhookVerification,omitempty"`
}

// HTTPSEdgeSpec defines the desired state of HTTPSEdge
type HTTPSEdgeSpec struct {
	ngrokAPICommon `json:",inline"`

	// Hostports is a list of hostports served by this edge
	// +kubebuilder:validation:Required
	Hostports []string `json:"hostports,omitempty"`

	// Routes is a list of routes served by this edge
	Routes []HTTPSEdgeRouteSpec `json:"routes,omitempty"`

	// TLSTermination is the TLS termination configuration for this edge
	TLSTermination *EndpointTLSTerminationAtEdge `json:"tlsTermination,omitempty"`
}

type HTTPSEdgeRouteStatus struct {
	// ID is the unique identifier for this route
	ID string `json:"id,omitempty"`

	// URI is the URI for this route
	URI string `json:"uri,omitempty"`

	Match string `json:"match,omitempty"`

	MatchType string `json:"matchType,omitempty"`

	// Backend stores the status of the tunnel group backend,
	// mainly the ID of the backend
	Backend TunnelGroupBackendStatus `json:"backend,omitempty"`
}

// HTTPSEdgeStatus defines the observed state of HTTPSEdge
type HTTPSEdgeStatus struct {
	// ID is the unique identifier for this edge
	ID string `json:"id,omitempty"`

	// URI is the URI for this edge
	URI string `json:"uri,omitempty"`

	Routes []HTTPSEdgeRouteStatus `json:"routes,omitempty"`
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status

// HTTPSEdge is the Schema for the httpsedges API
type HTTPSEdge struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   HTTPSEdgeSpec   `json:"spec,omitempty"`
	Status HTTPSEdgeStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// HTTPSEdgeList contains a list of HTTPSEdge
type HTTPSEdgeList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []HTTPSEdge `json:"items"`
}

func init() {
	SchemeBuilder.Register(&HTTPSEdge{}, &HTTPSEdgeList{})
}

// Equal returns true if the two HTTPSEdge objects are equal
// It only checks the top level attributes like hostports and metadata
// It does not check the routes or the tunnel group backend
func (e *HTTPSEdge) Equal(edge *ngrok.HTTPSEdge) bool {
	if e == nil && edge == nil {
		return true
	}

	if e == nil || edge == nil {
		return false
	}

	// check if the metadata matches
	if e.Spec.Metadata != edge.Metadata {
		return false
	}

	// check if the hostports match
	if !reflect.DeepEqual(e.Spec.Hostports, edge.Hostports) {
		return false
	}

	// check if TLSTermination matches
	if e.Spec.TLSTermination == nil && edge.TlsTermination == nil {
		return true
	}
	if (e.Spec.TLSTermination == nil && edge.TlsTermination != nil) || (e.Spec.TLSTermination != nil && edge.TlsTermination == nil) {
		// one is nil and the other is not so they don't match
		return false
	}
	if e.Spec.TLSTermination.MinVersion != *edge.TlsTermination.MinVersion {
		return false
	}
	return true
}
