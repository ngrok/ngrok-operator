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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// CloudEndpointSpec defines the desired state of CloudEndpoint
type CloudEndpointSpec struct {
	// The unique URL for this cloud endpoint. This URL is the public address. The following formats are accepted
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

	// Reference to the TrafficPolicy resource to attach to the Cloud Endpoint
	TrafficPolicyName string `json:"trafficPolicyName,omitempty"`

	// Controls whether or not the Cloud Endpoint should allow pooling with other
	// Cloud Endpoints sharing the same URL. When Cloud Endpoints are pooled, any requests
	// going to the URL for the pooled endpoint will be distributed among all Cloud Endpoints
	// in the pool. A URL can only be shared across multiple Cloud Endpoints if they all have pooling enabled.
	//
	// +kubebuilder:validation:Optional
	PoolingEnabled *bool `json:"poolingEnabled,omitempty"`

	// Allows inline definition of a TrafficPolicy object
	TrafficPolicy *NgrokTrafficPolicySpec `json:"trafficPolicy,omitempty"`

	// Human-readable description of this cloud endpoint
	//
	// +kubebuilder:default:=`Created by the ngrok-operator`
	Description string `json:"description,omitempty"`

	// String of arbitrary data associated with the object in the ngrok API/Dashboard
	//
	// +kubebuilder:default:=`{"owned-by":"ngrok-operator"}`
	Metadata string `json:"metadata,omitempty"`

	// Bindings is the list of Binding IDs to associate with the endpoint
	// Accepted values are "public", "internal", or "kubernetes"
	//
	// +kubebuilder:validation:MaxItems=1
	// +kubebuilder:validation:items:Pattern=`^(public|internal|kubernetes)$`
	Bindings []string `json:"bindings,omitempty"`
}

// CloudEndpointStatus defines the observed state of CloudEndpoint
type CloudEndpointStatus struct {
	// ID is the unique identifier for this endpoint
	ID string `json:"id,omitempty"`

	// DomainRef is a reference to the Domain resource associated with this endpoint.
	// For internal endpoints, this will be nil.
	// +kubebuilder:validation:Optional
	// +nullable
	DomainRef *K8sObjectRefOptionalNamespace `json:"domainRef"`

	// Conditions describe the current conditions of the AgentEndpoint.
	//
	// +listType=map
	// +listMapKey=type
	// +kubebuilder:validation:MaxItems=8
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// CloudEndpoint is the Schema for the cloudendpoints API
//
// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:categories="networking";"ngrok"
// +kubebuilder:resource:shortName=clep
// +kubebuilder:printcolumn:name="ID",type="string",JSONPath=".status.id"
// +kubebuilder:printcolumn:name="URL",type="string",JSONPath=".spec.url"
// +kubebuilder:printcolumn:name="Traffic Policy",type="string",JSONPath=".spec.trafficPolicyName"
// +kubebuilder:printcolumn:name="Bindings",type="string",JSONPath=".spec.bindings"
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`
// +kubebuilder:printcolumn:name="Ready",type="string",JSONPath=".status.conditions[?(@.type=='Ready')].status"
type CloudEndpoint struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   CloudEndpointSpec   `json:"spec,omitempty"`
	Status CloudEndpointStatus `json:"status,omitempty"`
}

// CloudEndpointList contains a list of CloudEndpoint
//
// +kubebuilder:object:root=true
type CloudEndpointList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []CloudEndpoint `json:"items"`
}

// EndpointWithDomain implementation for CloudEndpoint
var _ EndpointWithDomain = &CloudEndpoint{}

// GetConditions returns a pointer to the conditions slice for CloudEndpoint
func (c *CloudEndpoint) GetConditions() *[]metav1.Condition {
	return &c.Status.Conditions
}

// GetGeneration returns the generation for CloudEndpoint
func (c *CloudEndpoint) GetGeneration() int64 {
	return c.Generation
}

// GetDomainRef returns the domain reference for CloudEndpoint
func (c *CloudEndpoint) GetDomainRef() *K8sObjectRefOptionalNamespace {
	return c.Status.DomainRef
}

// SetDomainRef sets the domain reference for CloudEndpoint
func (c *CloudEndpoint) SetDomainRef(ref *K8sObjectRefOptionalNamespace) {
	c.Status.DomainRef = ref
}

// GetURL returns the URL for the CloudEndpoint
func (c *CloudEndpoint) GetURL() string {
	return c.Spec.URL
}

// GetBindings returns the bindings for the CloudEndpoint
func (c *CloudEndpoint) GetBindings() []string {
	return c.Spec.Bindings
}

func init() {
	SchemeBuilder.Register(&CloudEndpoint{}, &CloudEndpointList{})
}
