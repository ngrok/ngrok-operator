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

// LEGACY-domain-kind: these tests cover the migration window; at cleanup keep
// only the single-kind cases (v1 deleted with Delete / Retain).

package ngrok

import (
	"context"
	"strings"
	"testing"

	"github.com/go-logr/logr"
	"github.com/ngrok/ngrok-api-go/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/events"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	ingressv1alpha1 "github.com/ngrok/ngrok-operator/api/ingress/v1alpha1"
	ngrokv1 "github.com/ngrok/ngrok-operator/api/ngrok/v1"
	domainpkg "github.com/ngrok/ngrok-operator/internal/domain"
	"github.com/ngrok/ngrok-operator/internal/mocks/nmockapi"
	trafficpolicypkg "github.com/ngrok/ngrok-operator/internal/trafficpolicy"
	"github.com/ngrok/ngrok-operator/internal/util"
)

const (
	pairNamespace = "default"
	pairName      = "a-ngrok-app"
	pairHostname  = "a.ngrok.app"
)

var pairKey = client.ObjectKey{Namespace: pairNamespace, Name: pairName}

// spyDomains counts the ngrok API writes the reconciler makes.
type spyDomains struct {
	*nmockapi.DomainClient
	creates, updates, deletes int
}

func (s *spyDomains) Create(ctx context.Context, item *ngrok.ReservedDomainCreate) (*ngrok.ReservedDomain, error) {
	s.creates++
	return s.DomainClient.Create(ctx, item)
}

func (s *spyDomains) Update(ctx context.Context, item *ngrok.ReservedDomainUpdate) (*ngrok.ReservedDomain, error) {
	s.updates++
	return s.DomainClient.Update(ctx, item)
}

func (s *spyDomains) Delete(ctx context.Context, id string) error {
	s.deletes++
	return s.DomainClient.Delete(ctx, id)
}

func (s *spyDomains) writes() int { return s.creates + s.updates + s.deletes }

func (s *spyDomains) reservations(t *testing.T) []*ngrok.ReservedDomain {
	t.Helper()
	var out []*ngrok.ReservedDomain
	iter := s.List(&ngrok.FilteredPaging{})
	for iter.Next(t.Context()) {
		out = append(out, iter.Item())
	}
	require.NoError(t, iter.Err())
	return out
}

// unusedReader fails the test if the reconciler reads from it.
type unusedReader struct{ t *testing.T }

func (u unusedReader) Get(context.Context, client.ObjectKey, client.Object, ...client.GetOption) error {
	u.t.Error("unexpected live read: a twin visible in the cache needs no re-check")
	return nil
}

func (u unusedReader) List(context.Context, client.ObjectList, ...client.ListOption) error {
	u.t.Error("unexpected live list")
	return nil
}

type alwaysDraining struct{}

func (alwaysDraining) IsDraining(context.Context) bool { return true }

type pairTest struct {
	r        *DomainReconciler
	c        client.Client
	api      *spyDomains
	recorder *events.FakeRecorder
	// rd is a reservation that exists before the test starts.
	rd *ngrok.ReservedDomain
}

func pairScheme(t *testing.T) *runtime.Scheme {
	t.Helper()
	s := runtime.NewScheme()
	require.NoError(t, ngrokv1.AddToScheme(s))
	require.NoError(t, ingressv1alpha1.AddToScheme(s))
	return s
}

func newPairTest(t *testing.T, objs ...client.Object) *pairTest {
	t.Helper()
	api := &spyDomains{DomainClient: nmockapi.NewDomainClient()}
	rd, err := api.DomainClient.Create(t.Context(), &ngrok.ReservedDomainCreate{Domain: pairHostname, Description: "original"})
	require.NoError(t, err)

	// Fixtures reference the pre-existing reservation as "rd_existing".
	for _, obj := range objs {
		switch d := obj.(type) {
		case *ngrokv1.Domain:
			if d.Status.ID == "rd_existing" {
				d.Status.ID = rd.ID
			}
		case *ingressv1alpha1.Domain:
			if d.Status.ID == "rd_existing" {
				d.Status.ID = rd.ID
			}
		}
	}

	c := fake.NewClientBuilder().
		WithScheme(pairScheme(t)).
		WithObjects(objs...).
		WithStatusSubresource(&ngrokv1.Domain{}, &ingressv1alpha1.Domain{}).
		Build()
	recorder := events.NewFakeRecorder(100)
	return &pairTest{
		r: &DomainReconciler{
			Client:        c,
			APIReader:     c,
			Log:           logr.Discard(),
			Recorder:      recorder,
			DomainsClient: api,
		},
		c:        c,
		api:      api,
		recorder: recorder,
		rd:       rd,
	}
}

func (p *pairTest) reconcile(t *testing.T) ctrl.Result {
	t.Helper()
	res, err := p.r.Reconcile(t.Context(), ctrl.Request{NamespacedName: pairKey})
	require.NoError(t, err)
	return res
}

func (p *pairTest) v1(t *testing.T) *ngrokv1.Domain {
	t.Helper()
	d := &ngrokv1.Domain{}
	err := p.c.Get(t.Context(), pairKey, d)
	if apierrors.IsNotFound(err) {
		return nil
	}
	require.NoError(t, err)
	return d
}

func (p *pairTest) legacy(t *testing.T) *ingressv1alpha1.Domain {
	t.Helper()
	d := &ingressv1alpha1.Domain{}
	err := p.c.Get(t.Context(), pairKey, d)
	if apierrors.IsNotFound(err) {
		return nil
	}
	require.NoError(t, err)
	return d
}

func (p *pairTest) events() string {
	var out []string
	for {
		select {
		case e := <-p.recorder.Events:
			out = append(out, e)
		default:
			return strings.Join(out, "\n")
		}
	}
}

type fixtureOpt func(metav1.Object)

func deleting() fixtureOpt {
	return func(o metav1.Object) {
		now := metav1.Now()
		o.SetDeletionTimestamp(&now)
	}
}

func generation(gen int64) fixtureOpt {
	return func(o metav1.Object) { o.SetGeneration(gen) }
}

func v1Fixture(hostname, id string, opts ...fixtureOpt) *ngrokv1.Domain {
	d := &ngrokv1.Domain{
		ObjectMeta: metav1.ObjectMeta{
			Name: pairName, Namespace: pairNamespace,
			Finalizers: []string{util.FinalizerName},
			Generation: 1,
		},
		Spec: ngrokv1.DomainSpec{
			Domain:        hostname,
			Description:   "from v1",
			ReclaimPolicy: ngrokv1.DomainReclaimPolicyDelete,
		},
		Status: ngrokv1.DomainStatus{ID: id, ObservedGeneration: 1},
	}
	for _, opt := range opts {
		opt(d)
	}
	return d
}

func legacyFixture(hostname, id string, opts ...fixtureOpt) *ingressv1alpha1.Domain {
	d := &ingressv1alpha1.Domain{
		ObjectMeta: metav1.ObjectMeta{
			Name: pairName, Namespace: pairNamespace,
			Finalizers: []string{util.FinalizerName},
			Generation: 1,
		},
		Spec: ingressv1alpha1.DomainSpec{
			Domain:        hostname,
			Description:   "from legacy",
			ReclaimPolicy: ingressv1alpha1.DomainReclaimPolicyDelete,
		},
		Status: ingressv1alpha1.DomainStatus{
			ID: id, ObservedGeneration: 1,
			// A sentinel: a superseded legacy Domain's status must stay frozen.
			CNAMETarget: new("frozen.example.com"),
		},
	}
	for _, opt := range opts {
		opt(d)
	}
	return d
}

func readyReason(t *testing.T, conditions []metav1.Condition) string {
	t.Helper()
	c := meta.FindStatusCondition(conditions, domainpkg.ConditionReady)
	require.NotNil(t, c, "Ready condition")
	return c.Reason
}

// Legacy-only Domains behave exactly as before: legacy is the writer.
func TestDomainPair_LegacyOnly(t *testing.T) {
	p := newPairTest(t, legacyFixture(pairHostname, "rd_existing", generation(2)))

	p.reconcile(t)

	legacy := p.legacy(t)
	assert.Equal(t, p.rd.ID, legacy.Status.ID)
	assert.Equal(t, domainpkg.ReasonDomainActive, readyReason(t, legacy.Status.Conditions))
	assert.Equal(t, int64(2), legacy.Status.ObservedGeneration)
	assert.Equal(t, "from legacy", p.api.reservations(t)[0].Description, "legacy pushes its own spec")
	assert.Contains(t, p.events(), trafficpolicypkg.EventDeprecatedAPIGroup)
}

// The main migration path: a v1 Domain appears next to a backed legacy one.
// It must adopt the same reservation (never create a second one), take over
// writing, and supersede the legacy object without touching its status
// fields or finalizer.
func TestDomainPair_V1AdoptsAndSupersedesLegacy(t *testing.T) {
	p := newPairTest(t,
		v1Fixture(pairHostname, ""),
		legacyFixture(pairHostname, "rd_existing"),
	)

	p.reconcile(t)

	assert.Zero(t, p.api.creates, "the v1 Domain adopts the existing reservation")
	require.Len(t, p.api.reservations(t), 1)

	v1 := p.v1(t)
	assert.Equal(t, p.rd.ID, v1.Status.ID)
	assert.Equal(t, domainpkg.ReasonDomainActive, readyReason(t, v1.Status.Conditions))
	assert.Equal(t, "from v1", p.api.reservations(t)[0].Description, "v1 is now the writer")

	legacy := p.legacy(t)
	assert.Equal(t, domainpkg.ReasonSupersededByV1, readyReason(t, legacy.Status.Conditions))
	assert.True(t, meta.IsStatusConditionTrue(legacy.Status.Conditions, domainpkg.ConditionReady))
	assert.Equal(t, "frozen.example.com", *legacy.Status.CNAMETarget, "other status fields stay frozen")
	assert.Contains(t, legacy.Finalizers, util.FinalizerName, "the legacy finalizer stays until deletion")
	assert.Contains(t, p.events(), EventDomainSupersededByV1)

	// A second pass changes nothing on the superseded object.
	before := legacy.ResourceVersion
	p.reconcile(t)
	assert.Equal(t, before, p.legacy(t).ResourceVersion)
}

// Editing a superseded legacy Domain makes no ngrok API call and only warns.
func TestDomainPair_EditingSupersededLegacyIsANoOp(t *testing.T) {
	p := newPairTest(t,
		v1Fixture(pairHostname, "rd_existing"),
		legacyFixture(pairHostname, "rd_existing"),
	)
	p.reconcile(t) // v1 pushes its spec; legacy is superseded
	p.api.creates, p.api.updates, p.api.deletes = 0, 0, 0
	p.events()

	legacy := p.legacy(t)
	legacy.Spec.Description = "edited on the legacy object"
	legacy.Generation = 2 // the fake client does not bump generation
	require.NoError(t, p.c.Update(t.Context(), legacy))

	p.reconcile(t)

	assert.Zero(t, p.api.writes(), "a superseded legacy Domain never writes to the reservation")
	assert.Equal(t, "from v1", p.api.reservations(t)[0].Description)
	got := p.legacy(t)
	assert.Equal(t, domainpkg.ReasonSupersededByV1, readyReason(t, got.Status.Conditions))
	assert.Equal(t, int64(2), got.Status.ObservedGeneration)
	ev := p.events()
	assert.Contains(t, ev, "Warning "+trafficpolicypkg.EventDeprecatedAPIGroup)
	assert.Contains(t, ev, "this change has no effect")
}

// Deleting the legacy Domain after a re-stamp must keep the reservation.
func TestDomainPair_DeletingLegacyKeepsReservationForV1(t *testing.T) {
	for _, v1ID := range []string{"rd_existing", ""} {
		t.Run("v1 id="+v1ID, func(t *testing.T) {
			p := newPairTest(t,
				v1Fixture(pairHostname, v1ID),
				legacyFixture(pairHostname, "rd_existing", deleting()),
			)
			p.r.APIReader = unusedReader{t}

			p.reconcile(t)

			assert.Zero(t, p.api.deletes)
			require.Len(t, p.api.reservations(t), 1)
			assert.Nil(t, p.legacy(t), "the legacy finalizer is released")
			assert.Equal(t, p.rd.ID, p.v1(t).Status.ID)
			assert.Contains(t, p.events(), EventDomainReservationKept)
		})
	}
}

// Deleting the v1 Domain while a legacy twin exists hands the reservation back
// to the legacy object in the same pass.
func TestDomainPair_DeletingV1HandsBackToLegacy(t *testing.T) {
	legacy := legacyFixture(pairHostname, "rd_existing")
	legacy.Status.Conditions = []metav1.Condition{{
		Type: domainpkg.ConditionReady, Status: metav1.ConditionTrue, Reason: domainpkg.ReasonSupersededByV1,
		LastTransitionTime: metav1.Now(),
	}}
	p := newPairTest(t, v1Fixture(pairHostname, "rd_existing", deleting()), legacy)
	p.r.APIReader = unusedReader{t}

	p.reconcile(t)

	assert.Zero(t, p.api.deletes)
	require.Len(t, p.api.reservations(t), 1)
	assert.Nil(t, p.v1(t))

	got := p.legacy(t)
	assert.Equal(t, domainpkg.ReasonDomainActive, readyReason(t, got.Status.Conditions), "legacy is the writer again")
	assert.Equal(t, "from legacy", p.api.reservations(t)[0].Description)
	assert.Contains(t, got.Finalizers, util.FinalizerName)
}

func TestDomainPair_LastHolderFollowsReclaimPolicy(t *testing.T) {
	t.Run("Delete", func(t *testing.T) {
		p := newPairTest(t, v1Fixture(pairHostname, "rd_existing", deleting()))
		p.reconcile(t)
		assert.Empty(t, p.api.reservations(t))
		assert.Nil(t, p.v1(t))
	})
	t.Run("Retain", func(t *testing.T) {
		v1 := v1Fixture(pairHostname, "rd_existing", deleting())
		v1.Spec.ReclaimPolicy = ngrokv1.DomainReclaimPolicyRetain
		p := newPairTest(t, v1)
		p.reconcile(t)
		assert.Len(t, p.api.reservations(t), 1)
		assert.Nil(t, p.v1(t))
	})
}

func TestDomainPair_BothDeletingReleasesOnce(t *testing.T) {
	p := newPairTest(t,
		v1Fixture(pairHostname, "rd_existing", deleting()),
		legacyFixture(pairHostname, "rd_existing", deleting()),
	)

	p.reconcile(t)

	assert.Empty(t, p.api.reservations(t))
	assert.Nil(t, p.v1(t))
	assert.Nil(t, p.legacy(t))
}

// The two kinds come from separate informers. If the cache hasn't seen a v1
// Domain applied just before the legacy one was deleted, the live re-read
// must still find it and keep the reservation.
func TestDomainPair_LiveRecheckBeforeRelease(t *testing.T) {
	p := newPairTest(t, legacyFixture(pairHostname, "rd_existing", deleting()))
	p.r.APIReader = fake.NewClientBuilder().
		WithScheme(pairScheme(t)).
		WithObjects(v1Fixture(pairHostname, "")).
		Build()

	p.reconcile(t)

	assert.Zero(t, p.api.deletes)
	assert.Len(t, p.api.reservations(t), 1)
	assert.Nil(t, p.legacy(t))
}

// Twins that name different hostnames hold unrelated reservations: each is
// reconciled on its own, and neither guards the other's reservation.
func TestDomainPair_MismatchedTwins(t *testing.T) {
	p := newPairTest(t,
		v1Fixture("b.ngrok.app", "", generation(2)),
		legacyFixture(pairHostname, "rd_existing"),
	)

	p.reconcile(t)

	assert.Equal(t, 1, p.api.creates, "v1 reserves its own hostname")
	assert.Len(t, p.api.reservations(t), 2)
	assert.Equal(t, domainpkg.ReasonDomainActive, readyReason(t, p.legacy(t).Status.Conditions), "legacy is not superseded")
	assert.Contains(t, p.events(), EventDomainTwinMismatch)
}

func TestDomainPair_InternalHostnameIsReleasedWithoutAPICalls(t *testing.T) {
	p := newPairTest(t, legacyFixture("svc.ns.internal", ""))

	p.reconcile(t)

	assert.Zero(t, p.api.writes())
	assert.NotContains(t, p.legacy(t).Finalizers, util.FinalizerName)
}

func TestDomainPair_DrainSkipsNonDeleteWork(t *testing.T) {
	p := newPairTest(t,
		v1Fixture(pairHostname, ""),
		legacyFixture(pairHostname, "rd_existing"),
	)
	p.r.DrainState = alwaysDraining{}

	p.reconcile(t)

	assert.Zero(t, p.api.writes())
	assert.Empty(t, p.v1(t).Status.ID, "no adoption while draining")
}

func TestDomainPair_DrainStillProcessesDeletes(t *testing.T) {
	p := newPairTest(t,
		v1Fixture(pairHostname, "rd_existing"),
		legacyFixture(pairHostname, "rd_existing", deleting()),
	)
	p.r.DrainState = alwaysDraining{}

	p.reconcile(t)

	assert.Nil(t, p.legacy(t))
	assert.Len(t, p.api.reservations(t), 1, "the v1 twin still holds the reservation")
}

func TestEarliest(t *testing.T) {
	none := ctrl.Result{}
	soon := ctrl.Result{RequeueAfter: 1}
	later := ctrl.Result{RequeueAfter: 2}
	now := ctrl.Result{Requeue: true} //nolint:staticcheck

	assert.Equal(t, soon, earliest(soon, later))
	assert.Equal(t, soon, earliest(later, soon))
	assert.Equal(t, soon, earliest(none, soon))
	assert.Equal(t, soon, earliest(soon, none))
	assert.Equal(t, now, earliest(later, now))
	assert.Equal(t, none, earliest(none, none))
}
