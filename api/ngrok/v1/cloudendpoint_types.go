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

// CloudEndpointSpec defines the desired state of CloudEndpoint.
type CloudEndpointSpec struct {
	// The unique URL for this cloud endpoint. The following formats are accepted:
	// Domain - example.org
	//     Only defines the domain; scheme and port inferred.
	// Origin - https://example.ngrok.app or https://example.ngrok.app:443 or tcp://1.tcp.ngrok.io:12345 or tls://example.ngrok.app
	// Scheme (shorthand) - https:// or tcp:// or tls:// or http://
	// Empty - defaults to https with a randomly assigned address.
	// Internal - some.domain.internal — only accessible via the forward-internal traffic policy action.
	//
	// +kubebuilder:validation:Required
	URL string `json:"url"`

	// Configures a TrafficPolicy to attach to the CloudEndpoint, either inline
	// or by reference to a TrafficPolicy resource. Exactly one of `inline` or
	// `targetRef` must be specified.
	TrafficPolicy *TrafficPolicyCfg `json:"trafficPolicy,omitempty"`

	// Controls whether the CloudEndpoint may share its URL with other
	// CloudEndpoints (pool). When pooling is enabled across multiple
	// CloudEndpoints sharing a URL, traffic is distributed among them.
	//
	// +kubebuilder:validation:Optional
	PoolingEnabled *bool `json:"poolingEnabled,omitempty"`

	// Human-readable description of this cloud endpoint.
	//
	// +kubebuilder:default:=`Created by the ngrok-operator`
	Description string `json:"description,omitempty"`

	// Arbitrary key/value metadata associated with the object in the ngrok API/Dashboard.
	//
	// +kubebuilder:default:={owned-by: ngrok-operator}
	Metadata map[string]string `json:"metadata,omitempty"`

	// Bindings is the list of Binding IDs to associate with the endpoint.
	// Accepted values are "public", "internal", or "kubernetes".
	//
	// +kubebuilder:validation:MaxItems=1
	// +kubebuilder:validation:items:Pattern=`^(public|internal|kubernetes)$`
	Bindings []string `json:"bindings,omitempty"`
}

// CloudEndpointStatus defines the observed state of CloudEndpoint.
type CloudEndpointStatus struct {
	// ID is the ngrok API resource identifier for this endpoint.
	ID string `json:"id,omitempty"`

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

	// Conditions describe the current state of the CloudEndpoint.
	//
	// +listType=map
	// +listMapKey=type
	// +kubebuilder:validation:MaxItems=8
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// CloudEndpoint is the Schema for the cloudendpoints API.
//
// +kubebuilder:object:root=true
// +kubebuilder:storageversion
// +kubebuilder:subresource:status
// +kubebuilder:resource:categories="networking";"ngrok"
// +kubebuilder:resource:shortName=clep
// +kubebuilder:printcolumn:name="ID",type="string",JSONPath=".status.id"
// +kubebuilder:printcolumn:name="URL",type="string",JSONPath=".spec.url"
// +kubebuilder:printcolumn:name="Traffic Policy",type="string",JSONPath=".spec.trafficPolicy.targetRef.name"
// +kubebuilder:printcolumn:name="Bindings",type="string",JSONPath=".spec.bindings"
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`
// +kubebuilder:printcolumn:name="Ready",type="string",JSONPath=".status.conditions[?(@.type=='Ready')].status"
// +kubebuilder:printcolumn:name="Reason",type="string",JSONPath=".status.conditions[?(@.type=='Ready')].reason",priority=1
// +kubebuilder:printcolumn:name="Message",type="string",JSONPath=".status.conditions[?(@.type=='Ready')].message",priority=1
type CloudEndpoint struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   CloudEndpointSpec   `json:"spec,omitempty"`
	Status CloudEndpointStatus `json:"status,omitempty"`
}

// CloudEndpointList contains a list of CloudEndpoint.
//
// +kubebuilder:object:root=true
type CloudEndpointList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []CloudEndpoint `json:"items"`
}

// GetConditions returns a pointer to the conditions slice for CloudEndpoint.
func (c *CloudEndpoint) GetConditions() *[]metav1.Condition {
	return &c.Status.Conditions
}

// GetGeneration returns the generation for CloudEndpoint.
func (c *CloudEndpoint) GetGeneration() int64 {
	return c.Generation
}

// GetDomainRef returns the domain reference for CloudEndpoint.
func (c *CloudEndpoint) GetDomainRef() *K8sObjectRefOptionalNamespace {
	return c.Status.DomainRef
}

// SetDomainRef sets the domain reference for CloudEndpoint.
func (c *CloudEndpoint) SetDomainRef(ref *K8sObjectRefOptionalNamespace) {
	c.Status.DomainRef = ref
}

// GetURL returns the URL for the CloudEndpoint.
func (c *CloudEndpoint) GetURL() string {
	return c.Spec.URL
}

// GetBindings returns the bindings for the CloudEndpoint.
func (c *CloudEndpoint) GetBindings() []string {
	return c.Spec.Bindings
}

func init() {
	SchemeBuilder.Register(&CloudEndpoint{}, &CloudEndpointList{})
}
