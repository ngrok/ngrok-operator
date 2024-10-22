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

// common ngrok API/Dashboard fields
type ngrokAPICommon struct {
	// Description is a human-readable description of the object in the ngrok API/Dashboard
	// +kubebuilder:default:=`Created by ngrok-operator`
	Description string `json:"description,omitempty"`
	// Metadata is a string of arbitrary data associated with the object in the ngrok API/Dashboard
	// +kubebuilder:default:=`{"owned-by":"ngrok-operator"}`
	Metadata string `json:"metadata,omitempty"`
}

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

type KubernetesOperatorDeployment struct {
	// Name is the name of the k8s deployment for the operator
	Name string `json:"name,omitempty"`
	// The namespace in which the operator is deployed
	Namespace string `json:"namespace,omitempty"`
	// The version of the operator that is currently running
	Version string `json:"version,omitempty"`
}

type KubernetesOperatorBinding struct {
	// Name is the name of the k8s-binding for the account to bind to this configuration and the ngrok API
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:Pattern=`^k8s[/][a-zA-Z0-9-]{1,63}$`
	Name string `json:"name,omitempty"`

	// AllowedURLs is a list of URI patterns ([scheme://]<service-name>.<namespace-name>) thet determine which EndpointBindings are allowed to be created by the operator
	// TODO(hkatz) We are only implementing `*` for now
	// Support more patterns in the future, see product spec
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:items:Pattern=`^[*]$`
	AllowedURLs []string `json:"allowedURLs,omitempty"`

	// The public ingress endpoint for this Kubernetes Operator
	// +kubebuilder:validation:Optional
	IngressEndpoint *string `json:"ingressEndpoint,omitempty"`

	// TlsSecretName is the name of the k8s secret that contains the TLS private/public keys to use for the ngrok forwarding endpoint
	// +kubebuilder:validation:Required
	// +kubebuilder:default="default-tls"
	TlsSecretName string `json:"tlsSecretName"`
}

// KubernetesOperatorStatus defines the observed state of KubernetesOperator
type KubernetesOperatorStatus struct {
	// ID is the unique identifier for this Kubernetes Operator
	ID string `json:"id,omitempty"`

	// URI is the URI for this Kubernetes Operator
	URI string `json:"uri,omitempty"`
}

const (
	KubernetesOperatorFeatureIngress  = "ingress"
	KubernetesOperatorFeatureGateway  = "gateway"
	KubernetesOperatorFeatureBindings = "bindings"
)

type KubernetesOperatorSpec struct {
	// INSERT ADDITIONAL SPEC FIELDS - desired state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	ngrokAPICommon `json:",inline"`

	// Features enabled for this Kubernetes Operator
	// +kubebuilder:validation:Optional
	// +kubebuilder:validation:items:Enum=ingress,gateway,bindings
	EnabledFeatures []string `json:"enabledFeatures,omitempty"`

	// The ngrok region in which the ingress for this operator is served. Defaults to
	// "global" if not specified.
	// +kubebuilder:validation:Required
	// +kubebuilder:default="global"
	Region string `json:"region,omitempty"`

	// Deployment information of this Kubernetes Operator
	Deployment *KubernetesOperatorDeployment `json:"deployment,omitempty"`

	// Configuration for the binding feature of this Kubernetes Operator
	Binding *KubernetesOperatorBinding `json:"binding,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="ID",type=string,JSONPath=`.status.id`,description="Kubernetes Operator ID"
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`,description="Age"

// KubernetesOperator is the Schema for the ngrok kubernetesoperators API
type KubernetesOperator struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   KubernetesOperatorSpec   `json:"spec,omitempty"`
	Status KubernetesOperatorStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// KubernetesOperatorList contains a list of KubernetesOperator
type KubernetesOperatorList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []KubernetesOperator `json:"items"`
}

func init() {
	SchemeBuilder.Register(&KubernetesOperator{}, &KubernetesOperatorList{})
}