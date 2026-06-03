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

// KubernetesOperatorDeployment describes the operator deployment this CR represents.
type KubernetesOperatorDeployment struct {
	// Name is the name of the k8s deployment for the operator.
	Name string `json:"name,omitempty"`
	// Namespace in which the operator is deployed.
	Namespace string `json:"namespace,omitempty"`
	// Version of the operator that is currently running.
	Version string `json:"version,omitempty"`
}

// KubernetesOperatorBinding configures the bindings feature of a KubernetesOperator.
type KubernetesOperatorBinding struct {
	// EndpointSelectors is a list of CEL expressions that determine which
	// kubernetes-bound Endpoints will be created by the operator.
	// +kubebuilder:validation:Required
	EndpointSelectors []string `json:"endpointSelectors,omitempty"`

	// The public ingress endpoint for this Kubernetes Operator.
	IngressEndpoint *string `json:"ingressEndpoint,omitempty"`

	// TlsSecretName is the name of the k8s secret that contains the TLS
	// private/public keys to use for the ngrok forwarding endpoint.
	// +kubebuilder:validation:Required
	// +kubebuilder:default="default-tls"
	TlsSecretName string `json:"tlsSecretName"`
}

// KubernetesOperatorStatus defines the observed state of KubernetesOperator.
type KubernetesOperatorStatus struct {
	// ID is the unique identifier for this Kubernetes Operator.
	ID string `json:"id,omitempty"`

	// URI is the URI for this Kubernetes Operator.
	URI string `json:"uri,omitempty"`

	// RegistrationStatus is the status of the registration of this Kubernetes Operator with the ngrok API.
	// +kubebuilder:validation:Enum=registered;error;pending
	// +kubebuilder:default="pending"
	RegistrationStatus string `json:"registrationStatus,omitempty"`

	// RegistrationErrorCode is the returned ngrok error code.
	// +kubebuilder:validation:Pattern=`^ERR_NGROK_\d+$`
	RegistrationErrorCode string `json:"registrationErrorCode,omitempty"`

	// RegistrationErrorMessage is a free-form error message if the status is error.
	// +kubebuilder:validation:MaxLength=4096
	RegistrationErrorMessage string `json:"registrationErrorMessage,omitempty"`

	// EnabledFeatures is the string representation of the features enabled for this Kubernetes Operator.
	EnabledFeatures string `json:"enabledFeatures,omitempty"`

	// BindingsIngressEndpoint is the URL that the operator will use to talk to
	// the ngrok edge when forwarding traffic for k8s-bound endpoints.
	BindingsIngressEndpoint string `json:"bindingsIngressEndpoint,omitempty"`

	// DrainStatus indicates the current state of the drain process.
	DrainStatus DrainStatus `json:"drainStatus,omitempty"`

	// DrainMessage provides additional information about the drain status.
	DrainMessage string `json:"drainMessage,omitempty"`

	// DrainProgress indicates how many resources have been drained vs total.
	// Format: "X/Y" where X is processed (completed + failed) and Y is total.
	DrainProgress string `json:"drainProgress,omitempty"`

	// DrainErrors contains the most recent errors encountered during drain.
	// +optional
	DrainErrors []string `json:"drainErrors,omitempty"`
}

const (
	KubernetesOperatorRegistrationStatusSuccess = "registered"
	KubernetesOperatorRegistrationStatusError   = "error"
	KubernetesOperatorRegistrationStatusPending = "pending"
)

const (
	KubernetesOperatorFeatureIngress  = "ingress"
	KubernetesOperatorFeatureGateway  = "gateway"
	KubernetesOperatorFeatureBindings = "bindings"
)

// DrainStatus indicates the current state of the drain process.
// +kubebuilder:validation:Enum=pending;draining;completed;failed
type DrainStatus string

const (
	DrainStatusPending   DrainStatus = "pending"
	DrainStatusDraining  DrainStatus = "draining"
	DrainStatusCompleted DrainStatus = "completed"
	DrainStatusFailed    DrainStatus = "failed"
)

// DrainPolicy determines how ngrok API resources are handled during drain.
// +kubebuilder:validation:Enum=Delete;Retain
type DrainPolicy string

const (
	// DrainPolicyDelete deletes the CR, triggering controllers to clean up ngrok API resources.
	DrainPolicyDelete DrainPolicy = "Delete"
	// DrainPolicyRetain removes finalizers but preserves ngrok API resources.
	DrainPolicyRetain DrainPolicy = "Retain"
)

// KubernetesOperatorSpec defines the desired state of KubernetesOperator.
type KubernetesOperatorSpec struct {
	// Description is a human-readable description of the object in the ngrok API/Dashboard.
	// +kubebuilder:default:=`Created by the ngrok-operator`
	Description string `json:"description,omitempty"`

	// Metadata is arbitrary key/value data associated with the object in the ngrok API/Dashboard.
	// +kubebuilder:default:={owned-by: ngrok-operator}
	Metadata map[string]string `json:"metadata,omitempty"`

	// Features enabled for this Kubernetes Operator.
	// +kubebuilder:validation:items:Enum=ingress;gateway;bindings
	EnabledFeatures []string `json:"enabledFeatures,omitempty"`

	// The ngrok region in which the ingress for this operator is served.
	// Defaults to "global" if not specified.
	// +kubebuilder:default="global"
	Region string `json:"region,omitempty"`

	// Deployment information of this Kubernetes Operator.
	Deployment *KubernetesOperatorDeployment `json:"deployment,omitempty"`

	// Configuration for the binding feature of this Kubernetes Operator.
	Binding *KubernetesOperatorBinding `json:"binding,omitempty"`

	// Drain configures the drain behavior for uninstall.
	Drain *DrainConfig `json:"drain,omitempty"`
}

// DrainConfig configures the drain behavior during operator uninstall.
type DrainConfig struct {
	// Policy determines whether to delete ngrok API resources or just remove finalizers.
	// +kubebuilder:default=Retain
	Policy DrainPolicy `json:"policy,omitempty"`
}

// KubernetesOperator is the Schema for the ngrok kubernetesoperators API.
//
// +kubebuilder:object:root=true
// +kubebuilder:storageversion
// +kubebuilder:subresource:status
// +kubebuilder:resource:categories="networking";"ngrok"
// +kubebuilder:printcolumn:name="ID",type=string,JSONPath=`.status.id`,description="Kubernetes Operator ID"
// +kubebuilder:printcolumn:name="Status",type="string",JSONPath=".status.registrationStatus"
// +kubebuilder:printcolumn:name="Enabled Features",type="string",JSONPath=".status.enabledFeatures"
// +kubebuilder:printcolumn:name="Endpoint Selectors",type="string",JSONPath=".spec.binding.endpointSelectors"
// +kubebuilder:printcolumn:name="Binding Ingress Endpoint", type="string", JSONPath=".spec.binding.ingressEndpoint",priority=2
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`,description="Age"
type KubernetesOperator struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   KubernetesOperatorSpec   `json:"spec,omitempty"`
	Status KubernetesOperatorStatus `json:"status,omitempty"`
}

// KubernetesOperatorList contains a list of KubernetesOperator.
//
// +kubebuilder:object:root=true
type KubernetesOperatorList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []KubernetesOperator `json:"items"`
}

// GetDrainPolicy returns the configured drain policy, defaulting to Retain if not set.
func (ko *KubernetesOperator) GetDrainPolicy() DrainPolicy {
	if ko.Spec.Drain != nil && ko.Spec.Drain.Policy != "" {
		return ko.Spec.Drain.Policy
	}
	return DrainPolicyRetain
}

func init() {
	SchemeBuilder.Register(&KubernetesOperator{}, &KubernetesOperatorList{})
}
