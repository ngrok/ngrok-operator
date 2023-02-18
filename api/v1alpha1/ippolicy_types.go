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

type IPPolicyRule struct {
	NgrokAPICommon `json:",inline"`

	// +kubebuilder:validation:Required
	CIDR string `json:"cidr,omitempty"`
	// +kubebuilder:validation:Required
	Action string `json:"action,omitempty"`
}

type IPPolicyRuleStatus struct {
	ID string `json:"id,omitempty"`

	CIDR   string `json:"cidr,omitempty"`
	Action string `json:"action,omitempty"`
}

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// IPPolicySpec defines the desired state of IPPolicy
type IPPolicySpec struct {
	NgrokAPICommon `json:",inline"`

	// Rules is a list of rules that belong to the policy
	Rules []IPPolicyRule `json:"rules,omitempty"`
}

// IPPolicyStatus defines the observed state of IPPolicy
type IPPolicyStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "make" to regenerate code after modifying this file
	ID string `json:"id,omitempty"`

	Rules []IPPolicyRuleStatus `json:"rules,omitempty"`
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status
//+kubebuilder:printcolumn:name="ID",type=string,JSONPath=`.status.id`,description="IPPolicy ID"
//+kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`,description="Age"

// IPPolicy is the Schema for the ippolicies API
type IPPolicy struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   IPPolicySpec   `json:"spec,omitempty"`
	Status IPPolicyStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// IPPolicyList contains a list of IPPolicy
type IPPolicyList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []IPPolicy `json:"items"`
}

func init() {
	SchemeBuilder.Register(&IPPolicy{}, &IPPolicyList{})
}
