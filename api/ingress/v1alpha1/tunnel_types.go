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

	commonv1alpha1 "github.com/ngrok/ngrok-operator/api/common/v1alpha1"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// TunnelSpec defines the desired state of Tunnel
type TunnelSpec struct {
	// INSERT ADDITIONAL SPEC FIELDS - desired state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	// ForwardsTo is the name and port of the service to forward traffic to
	// +kubebuilder:validation:Required
	ForwardsTo string `json:"forwardsTo,omitempty"`

	// Labels are key/value pairs that are attached to the tunnel
	Labels map[string]string `json:"labels,omitempty"`

	// The configuration for backend connections to services
	BackendConfig *BackendConfig `json:"backend,omitempty"`

	// Specifies the protocol to use when connecting to the backend. Currently only http1 and http2 are supported
	// with prior knowledge (defaulting to http1).
	// +kubebuilder:validation:Optional
	// +kubebuilder:validation:Enum=http1;http2
	AppProtocol *commonv1alpha1.ApplicationProtocol `json:"appProtocol,omitempty"`
}

// BackendConfig defines the configuration for backend connections to services.
// This can be extended to include ServerName, InsecureSkipVerify, etc. down the road.
type BackendConfig struct {
	Protocol string `json:"protocol,omitempty"`
}

// TunnelStatus defines the observed state of Tunnel
type TunnelStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "make" to regenerate code after modifying this file
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="ForwardsTo",type=string,JSONPath=`.spec.forwardsTo`,description="Service/port to forward to"
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`,description="Age"

// Tunnel is the Schema for the tunnels API
type Tunnel struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   TunnelSpec   `json:"spec,omitempty"`
	Status TunnelStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// TunnelList contains a list of Tunnel
type TunnelList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Tunnel `json:"items"`
}

type TunnelGroupBackend struct {
	ngrokAPICommon `json:",inline"`

	// Labels to watch for tunnels on this backend
	Labels map[string]string `json:"labels,omitempty"`
}

type TunnelGroupBackendStatus struct {
	// ID is the unique identifier for this backend
	ID string `json:"id,omitempty"`
}

func init() {
	SchemeBuilder.Register(&Tunnel{}, &TunnelList{})
}
