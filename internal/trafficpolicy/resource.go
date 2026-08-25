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
	"context"
	"encoding/json"
	"fmt"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	ngrokv1 "github.com/ngrok/ngrok-operator/api/ngrok/v1"
	ngrokv1alpha1 "github.com/ngrok/ngrok-operator/api/ngrok/v1alpha1"
)

// TrafficPolicyResource is the kind-agnostic view of a traffic-policy custom
// resource. Both the canonical ngrok.com/v1 TrafficPolicy and the deprecated
// ngrok.k8s.ngrok.com/v1alpha1 NgrokTrafficPolicy satisfy it: the two kinds
// are structurally identical (a schemaless spec.policy plus an
// observedGeneration/conditions status), so everything that reconciles,
// validates, or watches a traffic policy can be written once against this
// interface instead of once per kind.
//
// It deliberately covers only traffic-policy resources — the accessors below
// are not a general CRD contract, and no other kind in this operator
// implements them.
//
// LEGACY-trafficpolicy-kind: this interface outlives the migration. At cleanup
// only the NgrokTrafficPolicy assertion below and the accessor methods on that
// type go away; the interface itself and every generic consumer of it stay as
// they are.
type TrafficPolicyResource interface {
	client.Object

	// GetPolicy returns the raw JSON policy body from the spec.
	GetPolicy() json.RawMessage

	// GetConditions returns a pointer to the status condition slice so
	// shared helpers (conditions.Set and friends) can mutate it in place.
	GetConditions() *[]metav1.Condition

	// GetObservedGeneration returns the generation last reconciled.
	GetObservedGeneration() int64

	// SetObservedGeneration records the generation just reconciled.
	SetObservedGeneration(int64)
}

// TrafficPolicyResourcePtr constrains PT to *T where *T is a
// TrafficPolicyResource. Generic code needs both halves: T to allocate a fresh
// zero value with new(T), and the interface to do anything useful with it.
// Together they let a single implementation serve every traffic-policy kind:
//
//	func reconcile[T any, PT TrafficPolicyResourcePtr[T]](...) {
//		policy := PT(new(T)) // typed, allocated, and usable as a client.Object
//	}
//
// The Kubernetes API machinery only ever deals in pointer receivers, which is
// why the constraint is expressed over *T rather than T directly.
type TrafficPolicyResourcePtr[T any] interface {
	*T
	TrafficPolicyResource
}

// Compile-time proof that both served kinds satisfy the interface. If a future
// change to either type breaks the contract, it fails here with a clear
// message rather than at a distant generic instantiation.
var (
	_ TrafficPolicyResource = (*ngrokv1.TrafficPolicy)(nil)
	// LEGACY-trafficpolicy-kind: drop this assertion at cleanup.
	_ TrafficPolicyResource = (*ngrokv1alpha1.NgrokTrafficPolicy)(nil)
)

// PolicyLookup is the result of resolving a traffic-policy reference straight
// against the API server. It is the client-side twin of
// store.TrafficPolicyLookup, which answers the same question from the
// managerdriver's cache; the field names match deliberately so the two read
// the same at call sites.
type PolicyLookup struct {
	// Policy is the raw JSON policy body.
	Policy json.RawMessage
	// Object is the CR that supplied the policy — either a
	// *ngrokv1.TrafficPolicy or a *ngrokv1alpha1.NgrokTrafficPolicy. Callers
	// pass it to a recorder when emitting events about the policy itself.
	Object client.Object
	// LegacyKind is true when the canonical kind was absent and the
	// deprecated ngrok.k8s.ngrok.com/v1alpha1 NgrokTrafficPolicy answered the
	// lookup. Callers use it to emit a DeprecatedAPIGroup warning.
	//
	// LEGACY-trafficpolicy-kind: delete this field at cleanup.
	LegacyKind bool
}

// LookupPolicy resolves a traffic-policy reference by key, canonical-first:
// the ngrok.com/v1 TrafficPolicy is consulted first, and only when it is
// absent does the lookup fall back to the deprecated
// ngrok.k8s.ngrok.com/v1alpha1 NgrokTrafficPolicy. Both kinds present under
// the same name is a valid transitional state — canonical wins silently.
//
// It returns an error wrapping ErrTrafficPolicyNotFound only when neither kind
// holds an object under key; that error is terminal, so callers should surface
// it rather than requeue. Any other error is transient and worth a retry.
//
// This is the single client-side resolver: every code path that reads a policy
// through a client.Client goes through it, so a caller cannot accidentally
// support only one of the two kinds. The store-backed equivalent is
// store.Store.ResolveTrafficPolicy.
//
// LEGACY-trafficpolicy-kind: at cleanup the body collapses to a single Get
// against ngrokv1.TrafficPolicy and the fallback block below goes away.
func LookupPolicy(ctx context.Context, c client.Client, key client.ObjectKey) (PolicyLookup, error) {
	canonical := &ngrokv1.TrafficPolicy{}
	err := c.Get(ctx, key, canonical)
	switch {
	case err == nil:
		return PolicyLookup{Policy: canonical.Spec.Policy, Object: canonical}, nil
	case !apierrors.IsNotFound(err):
		// Transient/unexpected error — let the caller requeue with backoff.
		return PolicyLookup{}, err
	}

	// LEGACY-trafficpolicy-kind: BEGIN — delete this fallback at cleanup; a
	// canonical miss then becomes ErrTrafficPolicyNotFound directly.
	legacy := &ngrokv1alpha1.NgrokTrafficPolicy{}
	if err := c.Get(ctx, key, legacy); err != nil {
		if apierrors.IsNotFound(err) {
			return PolicyLookup{}, fmt.Errorf("%w: %s", ErrTrafficPolicyNotFound, key)
		}
		return PolicyLookup{}, err
	}
	return PolicyLookup{Policy: legacy.Spec.Policy, Object: legacy, LegacyKind: true}, nil
	// LEGACY-trafficpolicy-kind: END
}
