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

package domain

import (
	"context"

	"k8s.io/apimachinery/pkg/api/meta"

	"sigs.k8s.io/controller-runtime/pkg/client"

	ingressv1alpha1 "github.com/ngrok/ngrok-operator/api/ingress/v1alpha1"
	ngrokv1 "github.com/ngrok/ngrok-operator/api/ngrok/v1"
	"github.com/ngrok/ngrok-operator/internal/util"
)

// Pair holds the Domains of both kinds that share one namespace/name. Either
// may be nil.
//
// During the migration window a hostname can be served by a legacy Domain, a
// v1 Domain, or both. Readers must not pick a kind directly: a v1 Domain the
// operator has only just created isn't Ready yet, and preferring it over a
// ready legacy twin would blank load balancer statuses and flap endpoint
// readiness. Use Authoritative (one key) or ByHostname (lists).
type Pair struct {
	V1 *ngrokv1.Domain
	// LEGACY-domain-kind: delete at cleanup.
	Legacy *ingressv1alpha1.Domain
}

// GetPair reads both kinds at key. A kind that doesn't exist is left nil.
func GetPair(ctx context.Context, c client.Reader, key client.ObjectKey) (Pair, error) {
	var p Pair

	v1 := &ngrokv1.Domain{}
	if err := c.Get(ctx, key, v1); err == nil {
		p.V1 = v1
	} else if client.IgnoreNotFound(err) != nil {
		return p, err
	}

	// LEGACY-domain-kind: BEGIN
	legacy := &ingressv1alpha1.Domain{}
	if err := c.Get(ctx, key, legacy); err == nil {
		p.Legacy = legacy
	} else if client.IgnoreNotFound(err) != nil {
		return p, err
	}
	// LEGACY-domain-kind: END

	return p, nil
}

// Empty reports whether neither kind exists.
func (p Pair) Empty() bool {
	return p.V1 == nil && p.Legacy == nil
}

// Matched reports whether both kinds exist and name the same hostname, so they
// share one reservation and must coordinate. Twins with different hostnames
// are unrelated reservations that happen to share a name.
//
// LEGACY-domain-kind: delete at cleanup.
func (p Pair) Matched() bool {
	return p.V1 != nil && p.Legacy != nil &&
		util.NormalizeHostname(p.V1.Spec.Domain) == util.NormalizeHostname(p.Legacy.Spec.Domain)
}

// Authoritative returns the Domain readers should use for this key, or nil if
// neither kind exists:
//
//  1. the v1 Domain, once it is alive and has taken over from its legacy twin
//     (see v1TakesOver);
//  2. otherwise the legacy Domain, if there is one;
//  3. otherwise the v1 Domain.
func (p Pair) Authoritative() (Resource, error) {
	if p.V1 != nil && (p.Legacy == nil || v1TakesOver(p.V1, p.Legacy)) {
		return FromV1(p.V1), nil
	}
	// LEGACY-domain-kind: BEGIN
	if p.Legacy != nil {
		return FromLegacy(p.Legacy)
	}
	// LEGACY-domain-kind: END
	return nil, nil
}

// ByHostname resolves lists of both kinds into the Domain readers should use
// for each spec.domain, applying the same precedence as Authoritative. Within
// one kind, duplicates of a hostname (Domains in different namespaces) keep
// the existing last-one-wins behavior.
//
// LEGACY-domain-kind: at cleanup drop the legacy parameter and its loop.
func ByHostname(v1 []ngrokv1.Domain, legacy []ingressv1alpha1.Domain) (map[string]Resource, error) {
	out := make(map[string]Resource, len(v1)+len(legacy))

	// LEGACY-domain-kind: BEGIN
	for i := range legacy {
		r, err := FromLegacy(&legacy[i])
		if err != nil {
			return nil, err
		}
		out[legacy[i].Spec.Domain] = r
	}
	// LEGACY-domain-kind: END

	for i := range v1 {
		host := v1[i].Spec.Domain
		cur, found := out[host]
		// The v1 Domain wins when nothing else claims the hostname, when the
		// current entry is another v1 Domain (last one wins, as before), or when
		// the current entry is a legacy Domain it has taken over from. The type
		// assertion is safe: IsLegacy is true only for the legacy adapter.
		if !found || !cur.IsLegacy() || v1TakesOver(&v1[i], cur.Object().(*ingressv1alpha1.Domain)) {
			out[host] = FromV1(&v1[i])
		}
	}
	return out, nil
}

// v1TakesOver reports whether readers should use a v1 Domain over its legacy
// twin: the v1 Domain must be alive, and either Ready or already the one the
// reconciler superseded the legacy Domain for. Until then the legacy Domain
// is the one serving the hostname. Requiring Ready (not just a reservation ID)
// keeps a v1 Domain whose first status write recorded an error from shadowing
// a working legacy one; once the legacy Domain is superseded its status is
// frozen, so from then on the v1 Domain is the truth, ready or not.
//
// LEGACY-domain-kind: delete at cleanup.
func v1TakesOver(v1 *ngrokv1.Domain, legacy *ingressv1alpha1.Domain) bool {
	if !IsAlive(v1) {
		return false
	}
	if IsReady(FromV1(v1)) {
		return true
	}
	ready := meta.FindStatusCondition(legacy.Status.Conditions, ConditionReady)
	return ready != nil && ready.Reason == ReasonSupersededByV1
}

// IsAlive reports whether obj is not being deleted. obj must be non-nil: a
// typed nil pointer (e.g. a nil Pair.V1) is not detected here.
func IsAlive(obj client.Object) bool {
	return obj.GetDeletionTimestamp().IsZero()
}

// HostnameOf returns spec.domain of a Domain of either kind. ok is false for
// any other object.
func HostnameOf(obj client.Object) (hostname string, ok bool) {
	switch d := obj.(type) {
	case *ngrokv1.Domain:
		return d.Spec.Domain, true
	case *ingressv1alpha1.Domain: // LEGACY-domain-kind: delete this case at cleanup.
		return d.Spec.Domain, true
	default:
		return "", false
	}
}
