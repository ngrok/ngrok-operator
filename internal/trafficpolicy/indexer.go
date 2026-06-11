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

package trafficpolicy

import (
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	ngrokv1alpha1 "github.com/ngrok/ngrok-operator/api/ngrok/v1alpha1"
)

// RefIndex is the canonical field-indexer name used by endpoint controllers
// that watch NgrokTrafficPolicy. Each controller registers an index with this
// name on its endpoint type so the watch mapper can list endpoints by the
// composite namespace/name of their referenced TrafficPolicy.
const RefIndex = ".spec.trafficPolicy.targetRef"

// IndexKey returns the composite "<namespace>/<name>" key for the endpoint's
// canonical TrafficPolicy targetRef, or empty string when no ref is set
// (inline policies and missing configs alike). The endpoint's own namespace
// is used when the ref does not specify one.
func IndexKey(ep ngrokv1alpha1.EndpointWithTrafficPolicy) string {
	cfg := ep.GetTrafficPolicyCfg()
	if cfg == nil || cfg.Reference == nil {
		return ""
	}
	return cfg.Reference.ToClientObjectKey(ep.GetNamespace()).String()
}

// IndexKeyForObject is an IndexField extractor suitable for direct use with
// mgr.GetFieldIndexer().IndexField for any endpoint type that satisfies
// EndpointWithTrafficPolicy. Returns the composite ref key, or nil if the
// object does not implement the interface or has no ref configured.
func IndexKeyForObject(o client.Object) []string {
	ep, ok := o.(ngrokv1alpha1.EndpointWithTrafficPolicy)
	if !ok {
		return nil
	}
	if k := IndexKey(ep); k != "" {
		return []string{k}
	}
	return nil
}

// LookupKey returns the composite "<namespace>/<name>" key for a TrafficPolicy
// object — the value the watch mapper passes to client.MatchingFields when
// listing endpoints whose RefIndex matches a changed TrafficPolicy.
func LookupKey(tp client.Object) string {
	return types.NamespacedName{Namespace: tp.GetNamespace(), Name: tp.GetName()}.String()
}
