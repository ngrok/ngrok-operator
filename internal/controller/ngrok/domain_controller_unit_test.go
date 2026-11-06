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

// LEGACY-domain-kind: delete this file at cleanup. It covers the twin
// decisions the envtest specs in domain_migration_test.go can't reach: read
// errors the API server won't produce on demand, and twin states that only
// arise when the two informers disagree.

package ngrok

import (
	"errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"

	ingressv1alpha1 "github.com/ngrok/ngrok-operator/api/ingress/v1alpha1"
	ngrokv1 "github.com/ngrok/ngrok-operator/api/ngrok/v1"
	domainpkg "github.com/ngrok/ngrok-operator/internal/domain"
)

// Rule 5: the live twin read fails closed.
func TestClassifyTwinRead(t *testing.T) {
	gr := schema.GroupResource{Group: "ngrok.com", Resource: "domains"}
	cases := []struct {
		name string
		err  error
		want twinRead
	}{
		{"read succeeded", nil, twinReadFound},
		{"not found counts as no twin", apierrors.NewNotFound(gr, "d"), twinReadAbsent},
		{"not served counts as no twin", &meta.NoKindMatchError{GroupKind: schema.GroupKind{Group: "ngrok.com", Kind: "Domain"}}, twinReadAbsent},
		{"forbidden counts as a twin", apierrors.NewForbidden(gr, "d", errors.New("rbac")), twinReadAssumed},
		{"wrapped forbidden counts as a twin", fmt.Errorf("get: %w", apierrors.NewForbidden(gr, "d", errors.New("rbac"))), twinReadAssumed},
		{"other errors retry", errors.New("connection refused"), twinReadRetry},
		{"server errors retry", apierrors.NewInternalError(errors.New("etcd")), twinReadRetry},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, classifyTwinRead(tc.err))
		})
	}
}

type domainOpt func(metav1.Object, *ngrokv1.DomainStatus)

func withStatusID(id string) domainOpt {
	return func(_ metav1.Object, s *ngrokv1.DomainStatus) { s.ID = id }
}

func coveredByWildcard() domainOpt {
	return func(_ metav1.Object, s *ngrokv1.DomainStatus) {
		s.CoveredByWildcardDomain = &ngrokv1.DomainStatusWildcardDomain{ID: "rd_wildcard", Domain: "*.ngrok.app"}
	}
}

func beingDeleted() domainOpt {
	return func(o metav1.Object, _ *ngrokv1.DomainStatus) {
		now := metav1.Now()
		o.SetDeletionTimestamp(&now)
	}
}

func v1Domain(hostname string, opts ...domainOpt) domainpkg.Resource {
	d := &ngrokv1.Domain{
		Spec: ngrokv1.DomainSpec{Domain: hostname, ReclaimPolicy: ngrokv1.DomainReclaimPolicyDelete},
	}
	for _, o := range opts {
		o(d, &d.Status)
	}
	return domainpkg.FromV1(d)
}

func legacyDomain(t *testing.T, hostname string, opts ...domainOpt) domainpkg.Resource {
	t.Helper()
	d := &ingressv1alpha1.Domain{
		Spec: ingressv1alpha1.DomainSpec{Domain: hostname, ReclaimPolicy: ingressv1alpha1.DomainReclaimPolicyDelete},
	}
	var status ngrokv1.DomainStatus
	for _, o := range opts {
		o(d, &status)
	}
	d.Status.ID = status.ID
	if status.CoveredByWildcardDomain != nil {
		d.Status.CoveredByWildcardDomain = &ingressv1alpha1.DomainStatusWildcardDomain{
			ID:     status.CoveredByWildcardDomain.ID,
			Domain: status.CoveredByWildcardDomain.Domain,
		}
	}
	r, err := domainpkg.FromLegacy(d)
	require.NoError(t, err)
	return r
}

// Rules 5 and 9: a twin keeps the reservation alive while it is alive and
// either holds the same ID or names the same hostname. The hostname case is a
// v1 Domain that hasn't adopted yet, or a legacy Domain with a stale ID.
func TestHoldsReservation(t *testing.T) {
	const host = "a.ngrok.app"
	cases := []struct {
		name         string
		twin, domain domainpkg.Resource
		want         bool
	}{
		{"same ID", v1Domain(host, withStatusID("rd_1")), legacyDomain(t, host, withStatusID("rd_1")), true},
		{"v1 twin not adopted yet", v1Domain(host), legacyDomain(t, host, withStatusID("rd_1")), true},
		{"stale legacy ID, same hostname", v1Domain(host, withStatusID("rd_1")), legacyDomain(t, host, withStatusID("rd_stale")), true},
		{"hostnames differ only in case", v1Domain("A.ngrok.app"), legacyDomain(t, host, withStatusID("rd_1")), true},
		{"different hostname and ID", v1Domain("b.ngrok.app", withStatusID("rd_2")), legacyDomain(t, host, withStatusID("rd_1")), false},
		{"different hostname, same ID", v1Domain("b.ngrok.app", withStatusID("rd_1")), legacyDomain(t, host, withStatusID("rd_1")), true},
		{"twin being deleted", v1Domain(host, withStatusID("rd_1"), beingDeleted()), legacyDomain(t, host, withStatusID("rd_1")), false},
		{"legacy twin of a v1 Domain", legacyDomain(t, host, withStatusID("rd_1")), v1Domain(host, withStatusID("rd_1")), true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, holdsReservation(tc.twin, tc.domain))
		})
	}
}

// Rule 3: only an alive, unbacked v1 Domain adopts, and only its same-hostname
// legacy twin's ID.
func TestCanAdopt(t *testing.T) {
	const host = "a.ngrok.app"
	cases := []struct {
		name              string
		canonical, legacy domainpkg.Resource
		want              bool
	}{
		{"unbacked v1, backed legacy", v1Domain(host), legacyDomain(t, host, withStatusID("rd_1")), true},
		{"no legacy twin", v1Domain(host), nil, false},
		{"no v1 twin", nil, legacyDomain(t, host, withStatusID("rd_1")), false},
		{"v1 already has an ID", v1Domain(host, withStatusID("rd_2")), legacyDomain(t, host, withStatusID("rd_1")), false},
		{"v1 covered by a wildcard", v1Domain(host, coveredByWildcard()), legacyDomain(t, host, withStatusID("rd_1")), false},
		{"legacy has no ID", v1Domain(host), legacyDomain(t, host), false},
		{"legacy covered by a wildcard", v1Domain(host), legacyDomain(t, host, coveredByWildcard()), false},
		{"different hostnames", v1Domain("b.ngrok.app"), legacyDomain(t, host, withStatusID("rd_1")), false},
		{"v1 being deleted", v1Domain(host, beingDeleted()), legacyDomain(t, host, withStatusID("rd_1")), false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, canAdopt(tc.canonical, tc.legacy))
		})
	}
}
