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
	"encoding/json"

	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// K8sObjectRef defines a reference to a Kubernetes Object in the same namespace.
type K8sObjectRef struct {
	// The name of the Kubernetes resource being referenced
	// +kubebuilder:validation:Required
	Name string `json:"name"`
}

// K8sObjectRefOptionalNamespace defines a reference to a Kubernetes Object,
// optionally in a different namespace.
type K8sObjectRefOptionalNamespace struct {
	// The name of the Kubernetes resource being referenced
	// +kubebuilder:validation:Required
	Name string `json:"name"`

	// The namespace of the Kubernetes resource being referenced
	Namespace *string `json:"namespace,omitempty"`
}

// ToClientObjectKey converts the K8sObjectRefOptionalNamespace to a client.ObjectKey,
// using the provided defaultNamespace if Namespace is nil.
func (ref *K8sObjectRefOptionalNamespace) ToClientObjectKey(defaultNamespace string) client.ObjectKey {
	namespace := defaultNamespace
	if ref.Namespace != nil {
		namespace = *ref.Namespace
	}
	return client.ObjectKey{
		Name:      ref.Name,
		Namespace: namespace,
	}
}

// Matches returns true if this reference points to the given Kubernetes object.
// A nil or empty namespace in the reference means it matches any namespace.
func (ref *K8sObjectRefOptionalNamespace) Matches(obj client.Object) bool {
	if ref == nil {
		return false
	}

	if ref.Name != obj.GetName() {
		return false
	}

	ns := ptr.Deref(ref.Namespace, "")
	return ns == "" || ns == obj.GetNamespace()
}

// TrafficPolicyCfgType identifies whether a TrafficPolicyCfg is by reference or inline.
type TrafficPolicyCfgType string

const (
	TrafficPolicyCfgType_K8sRef TrafficPolicyCfgType = "targetRef"
	TrafficPolicyCfgType_Inline TrafficPolicyCfgType = "inline"
)

// TrafficPolicyCfg configures a traffic policy via either an inline definition
// or a reference to a TrafficPolicy resource. Exactly one of `inline` or
// `targetRef` must be specified, enforced via XValidation.
//
// +kubebuilder:validation:XValidation:rule="has(self.inline) || has(self.targetRef)", message="targetRef or inline must be provided to trafficPolicy"
// +kubebuilder:validation:XValidation:rule="has(self.inline) != has(self.targetRef)",message="Only one of inline and targetRef can be configured for trafficPolicy"
type TrafficPolicyCfg struct {
	// Inline definition of a TrafficPolicy. The raw JSON-encoded policy is
	// passed through to the ngrok API.
	//
	// +kubebuilder:validation:Schemaless
	// +kubebuilder:pruning:PreserveUnknownFields
	// +kubebuilder:validation:Type=object
	Inline json.RawMessage `json:"inline,omitempty"`

	// Reference to a TrafficPolicy resource to attach.
	TargetRef *K8sObjectRefOptionalNamespace `json:"targetRef,omitempty"`
}

// Type reports whether this TrafficPolicyCfg is configured by reference or inline.
func (t *TrafficPolicyCfg) Type() TrafficPolicyCfgType {
	if t == nil {
		return ""
	}
	if t.TargetRef != nil {
		return TrafficPolicyCfgType_K8sRef
	}
	return TrafficPolicyCfgType_Inline
}

// ApplicationProtocol identifies the application-layer protocol used to
// communicate with an upstream.
//
// +kubebuilder:validation:Enum=http1;http2
type ApplicationProtocol string

const (
	ApplicationProtocol_HTTP1 ApplicationProtocol = "http1"
	ApplicationProtocol_HTTP2 ApplicationProtocol = "http2"
)

// IsKnown reports whether the protocol value is a recognized enum member.
func (t ApplicationProtocol) IsKnown() bool {
	switch t {
	case ApplicationProtocol_HTTP1, ApplicationProtocol_HTTP2:
		return true
	default:
		return false
	}
}

// ProxyProtocolVersion identifies which PROXY-protocol version the agent
// should use when forwarding to the upstream.
//
// +kubebuilder:validation:Enum="1";"2"
type ProxyProtocolVersion string

const (
	ProxyProtocolVersion_1 ProxyProtocolVersion = "1"
	ProxyProtocolVersion_2 ProxyProtocolVersion = "2"
)

// IsKnown reports whether the version value is a recognized enum member.
func (t ProxyProtocolVersion) IsKnown() bool {
	switch t {
	case ProxyProtocolVersion_1, ProxyProtocolVersion_2:
		return true
	default:
		return false
	}
}
