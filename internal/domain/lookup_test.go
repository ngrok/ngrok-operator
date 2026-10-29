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
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	ingressv1alpha1 "github.com/ngrok/ngrok-operator/api/ingress/v1alpha1"
	ngrokv1 "github.com/ngrok/ngrok-operator/api/ngrok/v1"
)

type v1State int

const (
	v1None v1State = iota
	v1Unbacked
	v1Backed // backed and Ready
	v1BackedNotReady
	v1Covered
	v1Deleting
)

type legacyState int

const (
	legacyNone legacyState = iota
	legacyAlive
	legacySuperseded
	legacyDeleting
)

var readyConditions = []metav1.Condition{{Type: ConditionReady, Status: metav1.ConditionTrue, Reason: ReasonDomainActive}}

func pairFixture(v1 v1State, legacy legacyState) Pair {
	var p Pair
	deleting := metav1.Now()
	switch v1 {
	case v1None:
	case v1Unbacked:
		p.V1 = &ngrokv1.Domain{Spec: ngrokv1.DomainSpec{Domain: "a.example.com"}}
	case v1Backed:
		p.V1 = &ngrokv1.Domain{Spec: ngrokv1.DomainSpec{Domain: "a.example.com"}, Status: ngrokv1.DomainStatus{ID: "rd_1", Conditions: readyConditions}}
	case v1BackedNotReady:
		p.V1 = &ngrokv1.Domain{Spec: ngrokv1.DomainSpec{Domain: "a.example.com"}, Status: ngrokv1.DomainStatus{ID: "rd_1"}}
	case v1Covered:
		p.V1 = &ngrokv1.Domain{
			Spec: ngrokv1.DomainSpec{Domain: "a.example.com"},
			Status: ngrokv1.DomainStatus{
				CoveredByWildcardDomain: &ngrokv1.DomainStatusWildcardDomain{ID: "rd_w", Domain: "*.example.com"},
				Conditions:              readyConditions,
			},
		}
	case v1Deleting:
		p.V1 = &ngrokv1.Domain{
			ObjectMeta: metav1.ObjectMeta{DeletionTimestamp: &deleting, Finalizers: []string{"x"}},
			Spec:       ngrokv1.DomainSpec{Domain: "a.example.com"},
			Status:     ngrokv1.DomainStatus{ID: "rd_1"},
		}
	}
	switch legacy {
	case legacyNone:
	case legacyAlive:
		p.Legacy = &ingressv1alpha1.Domain{Spec: ingressv1alpha1.DomainSpec{Domain: "a.example.com"}, Status: ingressv1alpha1.DomainStatus{ID: "rd_1"}}
	case legacySuperseded:
		p.Legacy = &ingressv1alpha1.Domain{
			Spec: ingressv1alpha1.DomainSpec{Domain: "a.example.com"},
			Status: ingressv1alpha1.DomainStatus{ID: "rd_1", Conditions: []metav1.Condition{{
				Type: ConditionReady, Status: metav1.ConditionTrue, Reason: ReasonSupersededByV1,
			}}},
		}
	case legacyDeleting:
		p.Legacy = &ingressv1alpha1.Domain{
			ObjectMeta: metav1.ObjectMeta{DeletionTimestamp: &deleting, Finalizers: []string{"x"}},
			Spec:       ingressv1alpha1.DomainSpec{Domain: "a.example.com"},
			Status:     ingressv1alpha1.DomainStatus{ID: "rd_1"},
		}
	}
	return p
}

func TestPairAuthoritative(t *testing.T) {
	const (
		none   = ""
		v1     = "v1"
		legacy = "legacy"
	)
	cases := []struct {
		v1     v1State
		legacy legacyState
		want   string
	}{
		{v1None, legacyNone, none},
		{v1None, legacyAlive, legacy},
		{v1None, legacyDeleting, legacy},
		{v1Unbacked, legacyNone, v1},
		// The case this function exists for: a v1 the operator just created
		// must not shadow a ready legacy twin.
		{v1Unbacked, legacyAlive, legacy},
		{v1Unbacked, legacyDeleting, legacy},
		{v1Backed, legacyNone, v1},
		{v1Backed, legacyAlive, v1},
		{v1Backed, legacyDeleting, v1},
		{v1Covered, legacyAlive, v1},
		// A v1 Domain with a reservation ID but not Ready (say its first spec
		// push failed) must not shadow a working legacy twin...
		{v1BackedNotReady, legacyAlive, legacy},
		{v1BackedNotReady, legacyNone, v1},
		// ...but once the legacy twin is superseded its status is frozen, and
		// the v1 Domain is the truth, ready or not.
		{v1BackedNotReady, legacySuperseded, v1},
		{v1Unbacked, legacySuperseded, v1},
		{v1Deleting, legacyNone, v1},
		{v1Deleting, legacyAlive, legacy},
		{v1Deleting, legacyDeleting, legacy},
	}
	for _, tc := range cases {
		p := pairFixture(tc.v1, tc.legacy)
		got, err := p.Authoritative()
		require.NoError(t, err)

		gotKind := none
		if got != nil {
			gotKind = v1
			if got.IsLegacy() {
				gotKind = legacy
			}
		}
		assert.Equal(t, tc.want, gotKind, "v1=%d legacy=%d", tc.v1, tc.legacy)
	}
}

func TestPairMatched(t *testing.T) {
	p := pairFixture(v1Backed, legacyAlive)
	assert.True(t, p.Matched())

	p.Legacy.Spec.Domain = "A.Example.com "
	assert.True(t, p.Matched(), "hostnames compare normalized")

	p.Legacy.Spec.Domain = "b.example.com"
	assert.False(t, p.Matched(), "twins with different hostnames hold unrelated reservations")

	assert.False(t, pairFixture(v1Backed, legacyNone).Matched())
	assert.False(t, pairFixture(v1None, legacyAlive).Matched())
}

func TestByHostname(t *testing.T) {
	v1s := []ngrokv1.Domain{
		{ObjectMeta: metav1.ObjectMeta{Name: "backed"}, Spec: ngrokv1.DomainSpec{Domain: "backed.example.com"}, Status: ngrokv1.DomainStatus{ID: "rd_v1", Conditions: readyConditions}},
		{ObjectMeta: metav1.ObjectMeta{Name: "unbacked"}, Spec: ngrokv1.DomainSpec{Domain: "unbacked.example.com"}},
		{ObjectMeta: metav1.ObjectMeta{Name: "v1-only"}, Spec: ngrokv1.DomainSpec{Domain: "v1-only.example.com"}},
	}
	legacies := []ingressv1alpha1.Domain{
		{ObjectMeta: metav1.ObjectMeta{Name: "backed"}, Spec: ingressv1alpha1.DomainSpec{Domain: "backed.example.com"}, Status: ingressv1alpha1.DomainStatus{ID: "rd_legacy"}},
		{ObjectMeta: metav1.ObjectMeta{Name: "unbacked"}, Spec: ingressv1alpha1.DomainSpec{Domain: "unbacked.example.com"}, Status: ingressv1alpha1.DomainStatus{ID: "rd_legacy"}},
		{ObjectMeta: metav1.ObjectMeta{Name: "legacy-only"}, Spec: ingressv1alpha1.DomainSpec{Domain: "legacy-only.example.com"}},
	}

	got, err := ByHostname(v1s, legacies)
	require.NoError(t, err)
	require.Len(t, got, 4)

	assert.False(t, got["backed.example.com"].IsLegacy(), "a backed v1 wins")
	assert.True(t, got["unbacked.example.com"].IsLegacy(), "an unbacked v1 doesn't shadow legacy")
	assert.False(t, got["v1-only.example.com"].IsLegacy())
	assert.True(t, got["legacy-only.example.com"].IsLegacy())
}

func TestGetPair(t *testing.T) {
	v1 := &ngrokv1.Domain{ObjectMeta: metav1.ObjectMeta{Name: "a-example-com", Namespace: "default"}, Spec: ngrokv1.DomainSpec{Domain: "a.example.com"}}
	legacy := &ingressv1alpha1.Domain{ObjectMeta: metav1.ObjectMeta{Name: "a-example-com", Namespace: "default"}, Spec: ingressv1alpha1.DomainSpec{Domain: "a.example.com"}}
	_, c := newTestManager(t, v1, legacy)

	p, err := GetPair(t.Context(), c, types.NamespacedName{Name: "a-example-com", Namespace: "default"})
	require.NoError(t, err)
	assert.NotNil(t, p.V1)
	assert.NotNil(t, p.Legacy)

	p, err = GetPair(t.Context(), c, types.NamespacedName{Name: "missing", Namespace: "default"})
	require.NoError(t, err)
	assert.True(t, p.Empty())
}
