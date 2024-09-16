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
	"github.com/ngrok/ngrok-api-go/v5"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// NOTE: Run "make manifests" to regenerate code after modifying this file:w
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

const (
	// DefaultTlsSecretName is the default name of the k8s secret that contains the TLS private/public keys to use for the ngrok forwarding endpoint
	// Note: This Secret is managed by the TlsSecretController
	DefaultTlsSecretName = "global-binding-configuration-certs"
)

// TargetMetadata is a subset of metav1.ObjectMeta that is used to define the target object in the k8s cluster
// +kubebuilder:object:generate=true
type TargetMetadata struct {
	// Map of string keys and values that can be used to organize and categorize
	// (scope and select) objects. May match selectors of replication controllers
	// and services.
	// More info: https://kubernetes.io/docs/concepts/overview/working-with-objects/labels
	// +kubebuilder:validation:Optional
	Labels map[string]string `json:"labels,omitempty"`

	// Annotations is an unstructured key value map stored with a resource that may be
	// set by external tools to store and retrieve arbitrary metadata. They are not
	// queryable and should be preserved when modifying objects.
	// More info: https://kubernetes.io/docs/concepts/overview/working-with-objects/annotations
	// +kubebuilder:validation:Optional
	Annotations map[string]string `json:"annotations,omitempty"`
}

// BindingConfigurationSpec defines the desired state of BindingConfiguration
type BindingConfigurationSpec struct {
	// Name is the name of the k8s-binding for the account to bind to this configuration and the ngrok API
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:Pattern=`^k8s[/][a-zA-Z0-9-]{1,63}$`
	Name string `json:"name"`

	// AllowedURLs is a list of URI patterns ([scheme://]<service-name>.<namespace-name>) thet determine which EndpointBindings are allowed to be created by the operator
	// TODO(hkatz) We are only implementing `*` for now
	// Support more patterns in the future, see product spec
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:items:Pattern=`^[*]$`
	AllowedURLs []string `json:"allowedURLs"`

	// TlsSecretName is the name of the k8s secret that contains the TLS private/public keys to use for the ngrok forwarding endpoint
	// TODO(hkatz) Create controller to manage this Secret lifecycle
	// +kubebuilder:validation:Required
	// +kubebuilder:default="global-binding-configuration-certs"
	TlsSecretName string `json:"tlsSecretName"`

	// Region is the ngrok region to use for the forwarding endpoint connections
	// +kubebuilder:validation:Required
	// +kubebuilder:default=""
	// Note: empty string means global/all regions are allowed
	// TODO(hkatz) implement this
	Region string `json:"region"`

	// ProjectedMetadata is a subset of metav1.ObjectMeta that is used to define the target object in the k8s cluster
	// +kube:validation:Optional
	ProjectedMetadata TargetMetadata `json:"projectedMetadata,omitempty"`
}

// BindingConfigurationStatus defines the observed state of BindingConfiguration
type BindingConfigurationStatus struct {
	// TODO(https://github.com/ngrok-private/ngrok/issues/32666)

	// Endpoints is a list of BindingEndpoint that are attached to the kubernetes operator binding
	Endpoints []BindingEndpoint `json:"endpoints"`
}

// BindingEndpoint is a reference to an Endpoint object in the ngrok API that is attached to the kubernetes operator binding
type BindingEndpoint struct {
	// Ref is the ngrok API reference to the Endpoint object (id, uri)
	ngrok.Ref `json:",inline"`

	// +kubebuilder:validation:Required
	// +kubebuilder:default="unknown"
	Status BindingEndpointStatus `json:"status"`

	// ErrorCode is the ngrok API error code if the status is error
	// +kubebuilder:validation:Optional
	// +kubebuilder:validation:Pattern=`^NGROK_ERR_\d+$`
	// TODO(hkatz) Define error codes and implement in the API
	ErrorCode string `json:"errorCode,omitempty"`

	// ErrorMessage is a free-form error message if the status is error
	// +kubebuilder:validation:Optional
	// +kubebuilder:validation:MaxLength=4096
	ErrorMessage string `json:"errorMessage,omitempty"`
}

// BindingEndpointStatus is an enum that represents the status of a BindingEndpoint
// TODO(https://github.com/ngrok-private/ngrok/issues/32666)
// +kubebuilder:validation:Enum=unknown;provisioning;bound;error
type BindingEndpointStatus string

const (
	StatusUnknown      BindingEndpointStatus = "unknown"
	StatusProvisioning BindingEndpointStatus = "provisioning"
	StatusBound        BindingEndpointStatus = "bound"
	StatusError        BindingEndpointStatus = "error"
)

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status

// BindingConfiguration is the Schema for the bindingconfigurations API
// +kubebuilder:printcolumn:name="Name",type="string",JSONPath=".spec.name"
// +kubebuilder:printcolumn:name="ID",type="string",JSONPath=".spec.id"
// +kubebuilder:printcolumn:name="Status",type="string",JSONPath=".spec.Status"
type BindingConfiguration struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   BindingConfigurationSpec   `json:"spec,omitempty"`
	Status BindingConfigurationStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// BindingConfigurationList contains a list of BindingConfiguration
type BindingConfigurationList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []BindingConfiguration `json:"items"`
}

func init() {
	SchemeBuilder.Register(&BindingConfiguration{}, &BindingConfigurationList{})
}
