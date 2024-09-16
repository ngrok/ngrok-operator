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

package v1beta1

import (
	"github.com/ngrok/ngrok-api-go/v5"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// NOTE: Run "make" to regenerate code after modifying this file
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// OperatorConfigurationSpec defines the configured installation state of OperatorConfiguration
type OperatorConfigurationSpec struct {
	ngrok.Ref `json:",inline"`
	// Description is a human-readable description of the object in the ngrok API/Dashboard
	// +kubebuilder:validation:Required
	// +kubebuilder:default:=`Created by ngrok-operator`
	// +kubebuilder:validation:MaxLength=4096
	Description string `json:"description,omitempty"`

	// Metadata is a JSON encoded tring of arbitrary data associated with the object in the ngrok API/Dashboard
	// +kubebuilder:validation:Required
	// +kubebuilder:default:=`{"owned-by":"ngrok-operator"}`
	// +kubebuilder:validation:MaxLength=4096
	Metadata string `json:"metadata,omitempty"`

	// ApiUrl is the base URL of the ngrok API that the operator is currently connected to
	// +kubebuilder:validation:Required
	ApiURL string `json:"apiURL,omitempty"`

	// Region is the region that the operator uses for request traffic
	// +kubebuilder:validation:Optional
	Region string `json:"region,omitempty"`

	// AppVersion is the version of the operator that is currently running
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:Pattern=`^\d+[.]\d+[.]\d+$`
	AppVersion string `json:"appVersion,omitempty"`

	// +kubebuilder:validation:Required
	// +kubeduilder:validation:items:Enum=ingress,gateway,bindings
	EnabledFeatures []string `json:"enabledFeatures,omitempty"`

	// ClusterDomain is the base domain for DNS resolution used in the cluster
	// +kubebuilder:validation:Required
	ClusterDomain string `json:"clusterDomain,omitempty"`
}

// OperatorConfigurationStatus defines the observed state of OperatorConfiguration
type OperatorConfigurationStatus struct {
	// TODO(hkatz) How should we connect feature statuses such as binding_endpoints or ingress_endpoints
	// TODO(hkatz) Where should we present free-form status information about the operator? kind: ConfigMap?
}

// OperatorFeature is an enum of the features that the operator can enable
// TODO(hkatz) potentioally move this to a shared features package
type OperatorFeature string

const (
	OperatorFeatureIngress  OperatorFeature = "ingress"
	OperatorFeatureGateway  OperatorFeature = "gateway"
	OperatorFeatureBindings OperatorFeature = "bindings"
)

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status

// OperatorConfiguration is the Schema for the operatorconfigurations API
// Note: This CRD is read-only and provides status information about the current state of the ngrok-operator
type OperatorConfiguration struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   OperatorConfigurationSpec   `json:"spec,omitempty"`
	Status OperatorConfigurationStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// OperatorConfigurationList contains a list of OperatorConfiguration
type OperatorConfigurationList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []OperatorConfiguration `json:"items"`
}

func init() {
	SchemeBuilder.Register(&OperatorConfiguration{}, &OperatorConfigurationList{})
}
