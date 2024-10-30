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
	// The unique URL for this cloud endpoint. This URL is the public address
	// +kubebuilder:validation:Required
	URL string `json:"url"`

	// TrafficPolicyRef is a reference to the TrafficPolicy resource to attach to the Cloud Endpoint
	// +kubebuilder:validation:Optional
	TrafficPolicyName string `json:"trafficPolicyName,omitempty"`

	// TrafficPolicy allows inline definition of a TrafficPolicy object
	// +kubebuilder:validation:Optional
	TrafficPolicy *NgrokTrafficPolicySpec `json:"trafficPolicy,omitempty"`

	// Description is a human-readable description of this cloud endpoint
	// +kubebuilder:default:=`Created by the ngrok-operator`
	Description string `json:"description,omitempty"`

	// Metadata is a string of arbitrary data associated with the object in the ngrok API/Dashboard
	// +kubebuilder:default:=`{"owned-by":"ngrok-operator"}`
	Metadata string `json:"metadata,omitempty"`

	// Bindings is the list of Binding IDs to associate with the endpoint
	// Accepted values are "public", "internal", or strings matching the pattern "k8s/*"
	// +kubebuilder:validation:Optional
	// +kubebuilder:validation:Items=pattern=`^(public|internal|k8s/.*)$`
	Bindings []string `json:"bindings,omitempty"`
}

// CloudEndpointStatus defines the observed state of CloudEndpoint
type CloudEndpointStatus struct {
	// ID is the unique identifier for this endpoint
	ID string `json:"id,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="URL",type="string",JSONPath=".spec.url"
// +kubebuilder:printcolumn:name="Traffic Policy",type="string",JSONPath=".spec.trafficPolicyName"
// +kubebuilder:printcolumn:name="Bindings",type="string",JSONPath=".spec.bindings"

// CloudEndpoint is the Schema for the cloudendpoints API
type CloudEndpoint struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   CloudEndpointSpec   `json:"spec,omitempty"`
	Status CloudEndpointStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// CloudEndpointList contains a list of CloudEndpoint
type CloudEndpointList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []CloudEndpoint `json:"items"`
}

func init() {
	SchemeBuilder.Register(&CloudEndpoint{}, &CloudEndpointList{})
}
