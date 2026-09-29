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

package v1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// PrivateEndpointScheme is the URL scheme of a private endpoint.
// +kubebuilder:validation:Enum=http;https;tls;tcp
type PrivateEndpointScheme string

const (
	PrivateEndpointSchemeHTTP  PrivateEndpointScheme = "http"
	PrivateEndpointSchemeHTTPS PrivateEndpointScheme = "https"
	PrivateEndpointSchemeTLS   PrivateEndpointScheme = "tls"
	PrivateEndpointSchemeTCP   PrivateEndpointScheme = "tcp"

	PrivateEndpointConditionReady = "Ready"
)

// PrivateEndpointSpec mirrors one private endpoint URL in the ngrok account.
// It is written by the operator; users do not author PrivateEndpoints.
type PrivateEndpointSpec struct {
	// URL of the private endpoint, e.g. tcp://bar.internal:6379.
	// +kubebuilder:validation:Required
	URL string `json:"url"`

	// +kubebuilder:validation:Required
	Scheme PrivateEndpointScheme `json:"scheme"`

	// +kubebuilder:validation:Required
	Hostname string `json:"hostname"`

	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=65535
	Port int32 `json:"port"`
}

// PrivateEndpointStatus is the in-cluster wiring for a private endpoint.
type PrivateEndpointStatus struct {
	// +optional
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`

	// ClusterIP is the address in-cluster DNS returns for spec.hostname.
	// +optional
	ClusterIP string `json:"clusterIP,omitempty"`

	// ForwarderPort is the forwarder container port serving this endpoint.
	// Unset when the endpoint is served by the shared http/https listener.
	// +optional
	ForwarderPort int32 `json:"forwarderPort,omitempty"`

	// +listType=map
	// +listMapKey=type
	// +kubebuilder:validation:MaxItems=8
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:categories=ngrok
// +kubebuilder:printcolumn:name="URL",type="string",JSONPath=".spec.url"
// +kubebuilder:printcolumn:name="Scheme",type="string",JSONPath=".spec.scheme"
// +kubebuilder:printcolumn:name="ClusterIP",type="string",JSONPath=".status.clusterIP"
// +kubebuilder:printcolumn:name="Ready",type="string",JSONPath=".status.conditions[?(@.type=='Ready')].status"
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// PrivateEndpoint is a read-only mirror of an ngrok private endpoint
// (*.internal or *.ngrok.direct) that pods in this cluster can reach by URL.
type PrivateEndpoint struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   PrivateEndpointSpec   `json:"spec,omitempty"`
	Status PrivateEndpointStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// PrivateEndpointList contains a list of PrivateEndpoint.
type PrivateEndpointList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []PrivateEndpoint `json:"items"`
}
