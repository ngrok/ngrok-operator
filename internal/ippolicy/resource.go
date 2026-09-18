/*
MIT License

Copyright (c) 2026 ngrok, Inc.

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

// Package ippolicy holds the kind-agnostic contract shared by the canonical
// ngrok.com/v1 IPPolicy and the deprecated ingress.k8s.ngrok.com/v1alpha1
// IPPolicy, following the same shape as internal/trafficpolicy for the
// TrafficPolicy kind + group migration (see
// docs/superpowers/plans/2026-08-12-trafficpolicy-kind-migration-analysis.md
// and docs/developer-guide/passivity-shims.md §"Per-shim catalog: TrafficPolicy
// CRD kind + group rename" — IPPolicy follows the same LEGACY-KIND-MIGRATION
// pattern documented there, tagged LEGACY-ippolicy-kind).
package ippolicy

import (
	"encoding/json"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	ingressv1alpha1 "github.com/ngrok/ngrok-operator/api/ingress/v1alpha1"
	ngrokv1 "github.com/ngrok/ngrok-operator/api/ngrok/v1"
)

// IPPolicyResource is the kind-agnostic view of an IP-policy custom resource.
// Both the canonical ngrok.com/v1 IPPolicy and the deprecated
// ingress.k8s.ngrok.com/v1alpha1 IPPolicy satisfy it: the two kinds are
// structurally identical (description + metadata + rules spec, an
// observedGeneration/id/conditions status), so everything that reconciles,
// resolves, or watches an IP policy can be written once against this
// interface instead of once per kind.
//
// It deliberately covers only IP-policy resources — the accessors below are
// not a general CRD contract, and no other kind in this operator implements
// them.
//
// LEGACY-ippolicy-kind: this interface outlives the migration. At cleanup
// only the legacy IPPolicy assertion below and the accessor methods on that
// type go away; the interface itself and every generic consumer of it stay as
// they are.
type IPPolicyResource interface {
	client.Object

	// GetDescription returns the human-readable description from the spec.
	GetDescription() string

	// GetMetadata returns the raw JSON metadata blob from the spec.
	GetMetadata() map[string]string

	// GetRulesJSON returns the spec rules re-encoded as JSON. Both kinds
	// declare byte-compatible IPPolicyRule schemas (identical field names and
	// validation), but as distinct Go types, so JSON is the kind-agnostic
	// wire format generic callers decode into whatever shape they need,
	// mirroring how TrafficPolicyResource.GetPolicy exposes its schemaless
	// spec.
	GetRulesJSON() (json.RawMessage, error)

	// GetID returns the ngrok API ID assigned to this policy, if any.
	GetID() string

	// SetID records the ngrok API ID assigned to this policy.
	SetID(string)

	// GetConditions returns a pointer to the status condition slice so shared
	// helpers (conditions.Set and friends) can mutate it in place.
	GetConditions() *[]metav1.Condition

	// GetObservedGeneration returns the generation last reconciled.
	GetObservedGeneration() int64

	// SetObservedGeneration records the generation just reconciled.
	SetObservedGeneration(int64)
}

// IPPolicyResourcePtr constrains PT to *T where *T is an IPPolicyResource.
// Generic code needs both halves: T to allocate a fresh zero value with
// new(T), and the interface to do anything useful with it. Together they let
// a single implementation serve every IP-policy kind:
//
//	func reconcile[T any, PT IPPolicyResourcePtr[T]](...) {
//		policy := PT(new(T)) // typed, allocated, and usable as a client.Object
//	}
//
// The Kubernetes API machinery only ever deals in pointer receivers, which is
// why the constraint is expressed over *T rather than T directly.
type IPPolicyResourcePtr[T any] interface {
	*T
	IPPolicyResource
}

// Compile-time proof that both served kinds satisfy the interface. If a
// future change to either type breaks the contract, it fails here with a
// clear message rather than at a distant generic instantiation.
var (
	_ IPPolicyResource = (*ngrokv1.IPPolicy)(nil)
	// LEGACY-ippolicy-kind: drop this assertion at cleanup.
	_ IPPolicyResource = (*ingressv1alpha1.IPPolicy)(nil)
)
