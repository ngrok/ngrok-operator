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
	v6 "github.com/ngrok/ngrok-api-go/v6"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// NOTE: Run "make" to regenerate code after modifying this file
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// EndpointBindingSpec defines the desired state of EndpointBinding
type EndpointBindingSpec struct {
	// EndpointURI is the unique identifier
	// representing the EndpointBinding + its Endpoints
	// Format: <scheme>://<service>.<namespace>:<port>
	//
	// +kubebuilder:validation:Required
	// See: https://regex101.com/r/9QkXWl/1
	// +kubebuilder:validation:Pattern=`^((?P<scheme>(tcp|http|https|tls)?)://)?(?P<service>[a-z][a-zA-Z0-9-]{0,62})\.(?P<namespace>[a-z][a-zA-Z0-9-]{0,62})(:(?P<port>\d+))?$`
	EndpointURI string `json:"endpointURI"`

	// Scheme is a user-defined field for endpoints that describe how the data packets
	// are framed by the pod forwarders mTLS connection to the ngrok edge
	// +kubebuilder:validation:Required
	// +kubebuilder:default=`https`
	// +kubebuilder:validation:Enum=tcp;http;https;tls
	Scheme string `json:"scheme"`

	// Port is the Service port this Endpoint uses internally to communicate with its Upstream Service
	// +kubebuilder:validation:Required
	Port uint16 `json:"port"`

	// EndpointTarget is the target Service that this Endpoint projects
	// +kubebuilder:validation:Required
	Target EndpointTarget `json:"target"`
}

// EndpointBindingStatus defines the observed state of EndpointBinding
type EndpointBindingStatus struct {
	// Endpoints is the list of BindingEndpoints that are created for this EndpointBinding
	//
	// Note: The collection of Endpoints per Binding are Many-to-One
	//       The uniqueness of each Endpoint is not ID, but rather the 4-tuple <scheme,service-name,namespace,port>
	//       All Endpoints bound to a EndpointBinding will share the same 4-tuple, statuses, errors, etc...
	//       this is because EndpointBinding represents 1 Service, yet many Endpoints
	//
	// +kubebuilder:validation:Required
	Endpoints []BindingEndpoint `json:"endpoints"`

	// HashName is the hashed output of the TargetService and TargetNamespace for unique identification
	// +kubebuilder:validation:Required
	HashedName string `json:"hashedName"`
}

// EndpointTarget hold the data for the projected Service that binds the endpoint to the k8s cluster resource
type EndpointTarget struct {
	// Service is the name of the Service that this Endpoint projects
	// +kubebuilder:validation:Required
	Service string `json:"service"`

	// Namespace is the destination Namespace for the Service this Endpoint projects
	// +kubebuilder:validation:Required
	Namespace string `json:"namespace"`

	// Protocol is the Service protocol this Endpoint uses
	// +kubebuilder:validation:Required
	// +kubebuilder:default=`TCP`
	// +kubebuilder:validation:Enum=TCP
	Protocol string `json:"protocol"`

	// Port is the Service targetPort this Endpoint's Target Service uses for requests
	// +kubebuilder:validation:Required
	Port int32 `json:"port"`

	// Metadata is a subset of metav1.ObjectMeta that is added to the Service
	// +kube:validation:Optional
	Metadata TargetMetadata `json:"metadata,omitempty"`
}

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

// BindingEndpoint is a reference to an Endpoint object in the ngrok API that is attached to the kubernetes operator binding
type BindingEndpoint struct {
	// Ref is the ngrok API reference to the Endpoint object (id, uri)
	v6.Ref `json:",inline"`

	// +kubebuilder:validation:Required
	// +kube:validation:Enum=provisioning;bound;error;unknown
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

// EndpointBinding is the Schema for the endpointbindings API
// +kubebuilder:printcolumn:name="URI",type="string",JSONPath=".spec.endpointURI"
// +kubebuilder:printcolumn:name="Port",type="string",JSONPath=".spec.port"
// +kubebuilder:printcolumn:name="Status",type="string",JSONPath=".status.endpoints[0].status"
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`,description="Age"
type EndpointBinding struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   EndpointBindingSpec   `json:"spec,omitempty"`
	Status EndpointBindingStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:rbac:groups=core,resources=services,verbs=get;create;update;delete;list;watch

// EndpointBindingList contains a list of EndpointBinding
type EndpointBindingList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []EndpointBinding `json:"items"`
}

func init() {
	SchemeBuilder.Register(&EndpointBinding{}, &EndpointBindingList{})
}
