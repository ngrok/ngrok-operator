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
	"encoding/json"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	common "github.com/ngrok/ngrok-operator/api/common/v1alpha1"
)

type IPPolicyRule struct {
	// Description is a human-readable description of the object in the ngrok API/Dashboard
	// +kubebuilder:default:=`Created by ngrok-operator`
	// +kubebuilder:validation:MaxLength=255
	Description string `json:"description,omitempty"`
	// Metadata is arbitrary key/value data associated with the object in the
	// ngrok API/Dashboard. A raw JSON string is also accepted for backward
	// compatibility and is deprecated; use a map of string values instead.
	// The ngrok.metadata Helm value is not merged into this field.
	//
	// +kubebuilder:validation:Schemaless
	// +kubebuilder:pruning:PreserveUnknownFields
	// +kubebuilder:default:=`{"owned-by":"ngrok-operator"}`
	Metadata json.RawMessage `json:"metadata,omitempty"`
	// CIDR is an IPv4 or IPv6 address range in CIDR notation (e.g. 10.0.0.0/8 or 2001:db8::/32)
	// Pattern adapted from the standard IPv4/IPv6 validation regex documented at
	// https://www.ditig.com/validating-ipv4-and-ipv6-addresses-with-regexp
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:Pattern=`^(((25[0-5]|2[0-4][0-9]|1[0-9][0-9]|[1-9]?[0-9])\.){3}(25[0-5]|2[0-4][0-9]|1[0-9][0-9]|[1-9]?[0-9])/(3[0-2]|[12]?[0-9])|(([0-9a-fA-F]{1,4}:){7}[0-9a-fA-F]{1,4}|([0-9a-fA-F]{1,4}:){1,7}:|([0-9a-fA-F]{1,4}:){1,6}:[0-9a-fA-F]{1,4}|([0-9a-fA-F]{1,4}:){1,5}(:[0-9a-fA-F]{1,4}){1,2}|([0-9a-fA-F]{1,4}:){1,4}(:[0-9a-fA-F]{1,4}){1,3}|([0-9a-fA-F]{1,4}:){1,3}(:[0-9a-fA-F]{1,4}){1,4}|([0-9a-fA-F]{1,4}:){1,2}(:[0-9a-fA-F]{1,4}){1,5}|[0-9a-fA-F]{1,4}:((:[0-9a-fA-F]{1,4}){1,6})|:((:[0-9a-fA-F]{1,4}){1,7}|:))/(12[0-8]|1[01][0-9]|[1-9]?[0-9]))$`
	CIDR string `json:"cidr,omitempty"`
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:Enum=allow;deny
	Action string `json:"action,omitempty"`
}

// IPPolicySpec defines the desired state of IPPolicy
type IPPolicySpec struct {
	// Description is a human-readable description of the object in the ngrok API/Dashboard
	// +kubebuilder:default:=`Created by ngrok-operator`
	Description string `json:"description,omitempty"`
	// Metadata is arbitrary key/value data associated with the object in the
	// ngrok API/Dashboard. A raw JSON string is also accepted for backward
	// compatibility and is deprecated; use a map of string values instead.
	// The ngrok.metadata Helm value is not merged into this field.
	//
	// +kubebuilder:validation:Schemaless
	// +kubebuilder:pruning:PreserveUnknownFields
	// +kubebuilder:default:=`{"owned-by":"ngrok-operator"}`
	Metadata json.RawMessage `json:"metadata,omitempty"`
	// Rules is a list of rules that belong to the policy
	Rules []IPPolicyRule `json:"rules,omitempty"`
}

// IPPolicyStatus defines the observed state of IPPolicy
type IPPolicyStatus struct {
	// ObservedGeneration is the most recent metadata.generation observed by the
	// controller. When it matches metadata.generation, the status reflects the
	// latest spec.
	// +optional
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`

	ID string `json:"id,omitempty"`

	// Conditions represent the latest available observations of the IP policy's state
	// +listType=map
	// +listMapKey=type
	// +kubebuilder:validation:MaxItems=8
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="ID",type=string,JSONPath=`.status.id`,description="IPPolicy ID"
// +kubebuilder:printcolumn:name="Ready",type=string,JSONPath=`.status.conditions[?(@.type=='Ready')].status`,description="IPPolicy Ready"
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`,description="Age"
// +kubebuilder:printcolumn:name="Reason",type=string,JSONPath=`.status.conditions[?(@.type=='Ready')].reason`,description="Ready Reason",priority=1
// +kubebuilder:printcolumn:name="Message",type=string,JSONPath=`.status.conditions[?(@.type=='Ready')].message`,description="Ready Message",priority=1

// IPPolicy is the Schema for the ippolicies API.
//
// Deprecated: superseded by the canonical ngrok.com/v1 IPPolicy. This kind
// stays fully operable and readable for the whole migration window — see
// docs/developer-guide/passivity-shims.md §"Per-shim catalog: TrafficPolicy
// CRD kind + group rename" for the pattern this follows
// (LEGACY-ippolicy-kind). Everything that reconciles, resolves, or watches an
// IPPolicy should be written against internal/ippolicy.IPPolicyResource so it
// serves both kinds from one implementation, rather than duplicating logic
// per kind.
type IPPolicy struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   IPPolicySpec   `json:"spec,omitempty"`
	Status IPPolicyStatus `json:"status,omitempty"`
}

// SetObservedGeneration records the generation the controller reconciled.
func (p *IPPolicy) SetObservedGeneration(generation int64) {
	p.Status.ObservedGeneration = generation
}

// The accessors below satisfy internal/ippolicy.IPPolicyResource so this
// legacy kind can be driven by the same generic code as the canonical
// ngrok.com/v1 IPPolicy.
//
// LEGACY-ippolicy-kind: these accessors go away when this type is deleted at
// cleanup; the interface and its generic consumers stay.

// GetDescription returns the human-readable description from the spec.
func (p *IPPolicy) GetDescription() string {
	return p.Spec.Description
}

// GetMetadata returns the spec metadata parsed into a map[string]string,
// handling both the canonical object form and the deprecated legacy string
// form.
func (p *IPPolicy) GetMetadata() map[string]string {
	return common.MetadataToMap(p.Spec.Metadata)
}

// GetRulesJSON returns the spec rules re-encoded as JSON so kind-agnostic
// callers can consume them without depending on this package's IPPolicyRule
// type.
func (p *IPPolicy) GetRulesJSON() (json.RawMessage, error) {
	return json.Marshal(p.Spec.Rules)
}

// GetID returns the ngrok API ID assigned to this policy, if any.
func (p *IPPolicy) GetID() string {
	return p.Status.ID
}

// SetID records the ngrok API ID assigned to this policy.
func (p *IPPolicy) SetID(id string) {
	p.Status.ID = id
}

// GetConditions returns a pointer to the status condition slice so shared
// helpers can mutate it in place.
func (p *IPPolicy) GetConditions() *[]metav1.Condition {
	return &p.Status.Conditions
}

// GetObservedGeneration returns the generation the controller last reconciled.
func (p *IPPolicy) GetObservedGeneration() int64 {
	return p.Status.ObservedGeneration
}

// +kubebuilder:object:root=true

// IPPolicyList contains a list of IPPolicy
type IPPolicyList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []IPPolicy `json:"items"`
}
