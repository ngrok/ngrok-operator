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

package v1

import (
	"encoding/json"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// TrafficPolicySpec defines the desired state of TrafficPolicy.
//
// The spec is byte-compatible with the deprecated
// ngrok.k8s.ngrok.com/v1alpha1 NgrokTrafficPolicySpec so a manual migration is
// a manifest re-stamp (change kind + apiVersion, re-apply, delete the legacy
// object).
type TrafficPolicySpec struct {
	// The raw json encoded policy that was applied to the ngrok API.
	// Intentionally schemaless: the traffic policy language is defined and
	// versioned by the ngrok API, so validating its shape here would break
	// whenever new phases, actions, or fields ship server-side. The policy is
	// parsed at reconcile time and the result is reported via the Ready/Valid
	// conditions.
	// +kubebuilder:validation:Schemaless
	// +kubebuilder:pruning:PreserveUnknownFields
	// +kubebuilder:validation:Type=object
	Policy json.RawMessage `json:"policy,omitempty"`
}

// TrafficPolicyStatus defines the observed state of TrafficPolicy.
type TrafficPolicyStatus struct {
	// ObservedGeneration is the most recent metadata.generation observed by the
	// controller. When it matches metadata.generation, the status reflects the
	// latest spec.
	// +optional
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`

	// Conditions describe the current conditions of the TrafficPolicy.
	//
	// +listType=map
	// +listMapKey=type
	// +kubebuilder:validation:MaxItems=8
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:categories=ngrok
// +kubebuilder:printcolumn:name="Ready",type="string",JSONPath=".status.conditions[?(@.type=='Ready')].status"
// +kubebuilder:printcolumn:name="Reason",type="string",JSONPath=".status.conditions[?(@.type=='Ready')].reason",priority=1
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// TrafficPolicy is the Schema for the trafficpolicies API in the canonical
// ngrok.com/v1 group. It replaces the deprecated
// ngrok.k8s.ngrok.com/v1alpha1 NgrokTrafficPolicy.
type TrafficPolicy struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   TrafficPolicySpec   `json:"spec,omitempty"`
	Status TrafficPolicyStatus `json:"status,omitempty"`
}

// SetObservedGeneration records the generation the controller reconciled.
func (tp *TrafficPolicy) SetObservedGeneration(generation int64) {
	tp.Status.ObservedGeneration = generation
}

// +kubebuilder:object:root=true

// TrafficPolicyList contains a list of TrafficPolicy.
type TrafficPolicyList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []TrafficPolicy `json:"items"`
}
