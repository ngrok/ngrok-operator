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
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// K8sObjectRef defines a reference to a Kubernetes Object
type K8sObjectRef struct {
	// The name of the Kubernetes resource being referenced
	// +kubebuilder:validation:Required
	Name string `json:"name"`
}

type K8sObjectRefOptionalNamespace struct {
	// The name of the Kubernetes resource being referenced
	// +kubebuilder:validation:Required
	Name string `json:"name"`

	// The namespace of the Kubernetes resource being referenced
	// +kubebuilder:validation:Optional
	Namespace *string `json:"namespace,omitempty"`
}

// +kubebuilder:object:generate=false
// EndpointWithDomain represents an endpoint resource that has domain conditions and references
type EndpointWithDomain interface {
	client.Object
	GetURL() string
	GetBindings() []string
	GetConditions() *[]metav1.Condition
	GetGeneration() int64
	GetDomainRef() *K8sObjectRefOptionalNamespace
	SetDomainRef(*K8sObjectRefOptionalNamespace)
}

// ToClientObjectKey converts the K8sObjectRefOptionalNamespace to a client.ObjectKey,
// using the provided defaultNamespace if Namespace is nil
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

	// return true
	ns := ptr.Deref(ref.Namespace, "")
	return ns == "" || ns == obj.GetNamespace()
}
