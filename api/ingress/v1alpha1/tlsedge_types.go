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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// TLSEdgeSpec defines the desired state of TLSEdge
type TLSEdgeSpec struct {
	ngrokAPICommon `json:",inline"`

	// Backend is the definition for the tunnel group backend
	// that serves traffic for this edge
	// +kubebuilder:validation:Required
	Backend TunnelGroupBackend `json:"backend,omitempty"`

	// Hostports is a list of hostports served by this edge
	// +kubebuilder:validation:Required
	Hostports []string `json:"hostports,omitempty"`

	// IPRestriction is an IPRestriction to apply to this edge
	IPRestriction *EndpointIPPolicy `json:"ipRestriction,omitempty"`

	TLSTermination *EndpointTLSTermination `json:"tlsTermination,omitempty"`

	MutualTLS *EndpointMutualTLS `json:"mutualTls,omitempty"`

	// raw json policy string that was applied to the ngrok API
	// +kubebuilder:validation:Schemaless
	// +kubebuilder:pruning:PreserveUnknownFields
	// +kubebuilder:validation:Type=object
	Policy EndpointTrafficPolicy `json:"policy,omitempty"`
}

// TLSEdgeStatus defines the observed state of TLSEdge
type TLSEdgeStatus struct {
	// ID is the unique identifier for this edge
	ID string `json:"id,omitempty"`

	// URI is the URI of the edge
	URI string `json:"uri,omitempty"`

	// Hostports served by this edge
	Hostports []string `json:"hostports,omitempty"`

	// Backend stores the status of the tunnel group backend,
	// mainly the ID of the backend
	Backend TunnelGroupBackendStatus `json:"backend,omitempty"`
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status
//+kubebuilder:printcolumn:name="ID",type=string,JSONPath=`.status.id`,description="Domain ID"
//+kubebuilder:printcolumn:name="Hostports",type=string,JSONPath=`.status.hostports`,description="Hostports"
//+kubebuilder:printcolumn:name="Backend ID",type=string,JSONPath=`.status.backend.id`,description="Tunnel Group Backend ID"
//+kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`,description="Age"

// TLSEdge is the Schema for the tlsedges API
type TLSEdge struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   TLSEdgeSpec   `json:"spec,omitempty"`
	Status TLSEdgeStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// TLSEdgeList contains a list of TLSEdge
type TLSEdgeList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []TLSEdge `json:"items"`
}

func init() {
	SchemeBuilder.Register(&TLSEdge{}, &TLSEdgeList{})
}
