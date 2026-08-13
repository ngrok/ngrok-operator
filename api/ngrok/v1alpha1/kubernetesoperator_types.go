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
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

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
	// EndpointSelectors is a list of cel expression that determine which kubernetes-bound Endpoints will be created by the operator
	// +kubebuilder:validation:Required
	EndpointSelectors []string `json:"endpointSelectors,omitempty"`

	// The public ingress endpoint for this Kubernetes Operator
	IngressEndpoint *string `json:"ingressEndpoint,omitempty"`

	// TlsSecretName is the name of the k8s secret that contains the TLS private/public keys to use for the ngrok forwarding endpoint
	// +kubebuilder:validation:Required
	// +kubebuilder:default="default-tls"
	TlsSecretName string `json:"tlsSecretName"`
}

// KubernetesOperatorStatus defines the observed state of KubernetesOperator
type KubernetesOperatorStatus struct {
	// ObservedGeneration is the most recent metadata.generation observed by the
	// controller. When it matches metadata.generation, the status reflects the
	// latest spec.
	// +optional
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`

	// ID is the unique identifier for this Kubernetes Operator
	ID string `json:"id,omitempty"`

	// URI is the URI for this Kubernetes Operator
	URI string `json:"uri,omitempty"`

	// Conditions describe the current state of the KubernetesOperator.
	// Known condition types are "Ready", "Registered" and "Draining".
	// +optional
	// +listType=map
	// +listMapKey=type
	// +kubebuilder:validation:MaxItems=8
	Conditions []metav1.Condition `json:"conditions,omitempty"`

	// LEGACY-enabledfeatures-format: forces the CRD schema to accept the
	// comma-separated string wire format this field writes during the
	// migration window (MarshalJSON in kubernetesoperator_status_compat.go).
	// Delete this marker in the cleanup release; controller-gen will
	// regenerate the natural array schema once MarshalJSON is also removed.

	// EnabledFeatures are the features enabled for this Kubernetes Operator, as
	// reported by the ngrok API
	// +kubebuilder:validation:Schemaless
	// +kubebuilder:pruning:PreserveUnknownFields
	EnabledFeatures KubernetesOperatorEnabledFeatures `json:"enabledFeatures,omitempty"`

	// BindingsIngressEndpoint is the URL that the operator will use to talk
	// to the ngrok edge when forwarding traffic for k8s-bound endpoints
	BindingsIngressEndpoint string `json:"bindingsIngressEndpoint,omitempty"`

	// Drain reports the progress of the drain triggered by deleting this resource
	// +optional
	Drain *KubernetesOperatorDrainStatus `json:"drain,omitempty"`
}

// KubernetesOperatorDrainStatus reports drain progress while the operator is
// cleaning up the resources it manages during deletion
type KubernetesOperatorDrainStatus struct {
	// DrainedResources is the number of resources successfully drained in the latest attempt
	DrainedResources int `json:"drainedResources"`

	// FailedResources is the number of resources that could not be drained in the latest attempt
	FailedResources int `json:"failedResources"`

	// TotalResources is the total number of resources the drain will process
	TotalResources int `json:"totalResources"`

	// Errors contains the most recent errors encountered during drain
	// +optional
	// +kubebuilder:validation:MaxItems=20
	// +kubebuilder:validation:items:MaxLength=1024
	Errors []string `json:"errors,omitempty"`
}

// Condition types for KubernetesOperator. The condition type and reason string
// values are part of the public API contract — tooling like kubectl wait,
// Argo CD and Flux health checks depend on them.
const (
	// KubernetesOperatorConditionReady summarizes the overall health of the
	// KubernetesOperator
	KubernetesOperatorConditionReady = "Ready"
	// KubernetesOperatorConditionRegistered reports whether this operator is
	// registered with the ngrok API
	KubernetesOperatorConditionRegistered = "Registered"
	// KubernetesOperatorConditionDraining is present only while this resource is
	// being deleted: True while the drain of managed resources is in progress or
	// retrying after errors, False with reason DrainCompleted once it finishes
	KubernetesOperatorConditionDraining = "Draining"
)

// Condition reasons for KubernetesOperator. Provider-specific error codes are
// included in condition messages rather than expanding this stable reason set.
const (
	KubernetesOperatorReasonRegistered          = "Registered"
	KubernetesOperatorReasonDeregistered        = "Deregistered"
	KubernetesOperatorReasonPending             = "Pending"
	KubernetesOperatorReasonRegistrationFailed  = "RegistrationFailed"
	KubernetesOperatorReasonConfigurationFailed = "ConfigurationFailed"
	KubernetesOperatorReasonDraining            = "Draining"
	KubernetesOperatorReasonDrainInProgress     = "DrainInProgress"
	KubernetesOperatorReasonDrainFailed         = "DrainFailed"
	KubernetesOperatorReasonDrainCompleted      = "DrainCompleted"
)

const (
	KubernetesOperatorFeatureIngress  = "ingress"
	KubernetesOperatorFeatureGateway  = "gateway"
	KubernetesOperatorFeatureBindings = "bindings"
)

// DrainPolicy determines how ngrok API resources are handled during drain
// +kubebuilder:validation:Enum=Delete;Retain
type DrainPolicy string

const (
	// DrainPolicyDelete deletes the CR, triggering controllers to clean up ngrok API resources
	DrainPolicyDelete DrainPolicy = "Delete"
	// DrainPolicyRetain removes finalizers but preserves ngrok API resources
	DrainPolicyRetain DrainPolicy = "Retain"
)

type KubernetesOperatorSpec struct {
	// INSERT ADDITIONAL SPEC FIELDS - desired state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	// Description is a human-readable description of the object in the ngrok API/Dashboard
	// +kubebuilder:default:=`Created by ngrok-operator`
	Description string `json:"description,omitempty"`

	// Metadata is arbitrary key/value data associated with the object in the
	// ngrok API/Dashboard. The ngrokMetadata Helm value is not merged into
	// this field.
	//
	// +kubebuilder:default:={"owned-by":"ngrok-operator"}
	Metadata map[string]string `json:"metadata,omitempty"`

	// Features enabled for this Kubernetes Operator
	// +kubebuilder:validation:items:Enum=ingress;gateway;bindings
	EnabledFeatures []string `json:"enabledFeatures,omitempty"`

	// The ngrok region in which the ingress for this operator is served. Defaults to
	// "global" if not specified.
	// +kubebuilder:default="global"
	Region string `json:"region,omitempty"`

	// Deployment information of this Kubernetes Operator
	Deployment *KubernetesOperatorDeployment `json:"deployment,omitempty"`

	// Configuration for the binding feature of this Kubernetes Operator
	Binding *KubernetesOperatorBinding `json:"binding,omitempty"`

	// Drain configures the drain behavior for uninstall
	Drain *DrainConfig `json:"drain,omitempty"`
}

// DrainConfig configures the drain behavior during operator uninstall
type DrainConfig struct {
	// Policy determines whether to delete ngrok API resources or just remove finalizers
	// +kubebuilder:default=Retain
	Policy DrainPolicy `json:"policy,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="ID",type=string,JSONPath=`.status.id`,description="Kubernetes Operator ID"
// +kubebuilder:printcolumn:name="Ready",type=string,JSONPath=`.status.conditions[?(@.type=='Ready')].status`
// +kubebuilder:printcolumn:name="Enabled Features",type="string",JSONPath=".status.enabledFeatures"
// +kubebuilder:printcolumn:name="Endpoint Selectors",type="string",JSONPath=".spec.binding.endpointSelectors"
// +kubebuilder:printcolumn:name="Binding Ingress Endpoint", type="string", JSONPath=".spec.binding.ingressEndpoint",priority=2
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`,description="Age"
// +kubebuilder:printcolumn:name="Reason",type=string,JSONPath=`.status.conditions[?(@.type=='Ready')].reason`,priority=1
// +kubebuilder:printcolumn:name="Drained",type=integer,JSONPath=`.status.drain.drainedResources`,priority=1
// +kubebuilder:printcolumn:name="Drain Total",type=integer,JSONPath=`.status.drain.totalResources`,priority=1

// KubernetesOperator is the Schema for the ngrok kubernetesoperators API
type KubernetesOperator struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   KubernetesOperatorSpec   `json:"spec,omitempty"`
	Status KubernetesOperatorStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// KubernetesOperatorList contains a list of KubernetesOperator
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

// IsDrainComplete reports whether the drain triggered by deleting this resource
// has finished (the Draining condition is False with reason DrainCompleted).
// This only checks condition status, not reason: it relies on the invariant
// that DrainCompleted is the only reason ever set alongside status False.
func (ko *KubernetesOperator) IsDrainComplete() bool {
	condition := meta.FindStatusCondition(ko.Status.Conditions, KubernetesOperatorConditionDraining)
	return condition != nil &&
		condition.Status == metav1.ConditionFalse &&
		condition.Reason == KubernetesOperatorReasonDrainCompleted
}

// SetObservedGeneration records the generation the controller reconciled.
func (ko *KubernetesOperator) SetObservedGeneration(generation int64) {
	ko.Status.ObservedGeneration = generation
}
