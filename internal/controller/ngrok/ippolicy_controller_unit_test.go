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

package ngrok

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/ngrok/ngrok-api-go/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/events"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	ingressv1alpha1 "github.com/ngrok/ngrok-operator/api/ingress/v1alpha1"
	ngrokv1 "github.com/ngrok/ngrok-operator/api/ngrok/v1"
	"github.com/ngrok/ngrok-operator/internal/drain"
	"github.com/ngrok/ngrok-operator/internal/ippolicy"
	"github.com/ngrok/ngrok-operator/internal/mocks/nmockapi"
	"github.com/ngrok/ngrok-operator/internal/util"
)

// These tests drive one Reconcile pass at a time over a twin pair, using a
// fake Kubernetes client and the mock ngrok API, and check the invariants
// documented on IPPolicyReconciler.

var pairKey = types.NamespacedName{Namespace: "default", Name: "pair"}

type pairEnv struct {
	r        *IPPolicyReconciler
	kube     client.Client
	policies *nmockapi.IPPolicyClient
	rules    *nmockapi.IPPolicyRuleClient
	recorder *events.FakeRecorder
}

type pairOpt func(*fake.ClientBuilder, *IPPolicyReconciler)

func withInterceptor(f interceptor.Funcs) pairOpt {
	return func(b *fake.ClientBuilder, _ *IPPolicyReconciler) { b.WithInterceptorFuncs(f) }
}

func withDrainState(s drain.State) pairOpt {
	return func(_ *fake.ClientBuilder, r *IPPolicyReconciler) { r.DrainState = s }
}

func withAPIReader(c client.Reader) pairOpt {
	return func(_ *fake.ClientBuilder, r *IPPolicyReconciler) { r.APIReader = c }
}

func pairScheme(t *testing.T) *runtime.Scheme {
	t.Helper()
	s := runtime.NewScheme()
	require.NoError(t, ngrokv1.AddToScheme(s))
	require.NoError(t, ingressv1alpha1.AddToScheme(s))
	return s
}

func newPairEnv(t *testing.T, policies *nmockapi.IPPolicyClient, objs []client.Object, opts ...pairOpt) *pairEnv {
	t.Helper()
	b := fake.NewClientBuilder().
		WithScheme(pairScheme(t)).
		WithObjects(objs...).
		WithStatusSubresource(&ngrokv1.IPPolicy{}, &ingressv1alpha1.IPPolicy{})
	r := &IPPolicyReconciler{
		Log:              logf.Log.WithName("ippolicy-test"),
		IPPoliciesClient: policies,
	}
	for _, o := range opts {
		o(b, r)
	}
	kube := b.Build()
	rules := nmockapi.NewIPPolicyRuleClient(policies)
	recorder := events.NewFakeRecorder(100)
	r.Client = kube
	if r.APIReader == nil {
		r.APIReader = kube
	}
	r.Recorder = recorder
	r.IPPolicyRulesClient = rules
	r.init()
	return &pairEnv{r: r, kube: kube, policies: policies, rules: rules, recorder: recorder}
}

func (e *pairEnv) reconcile(t *testing.T) (ctrl.Result, error) {
	t.Helper()
	return e.r.Reconcile(t.Context(), ctrl.Request{NamespacedName: pairKey})
}

// remote creates a remote policy in the mock API and returns its ID.
func remote(t *testing.T, policies *nmockapi.IPPolicyClient) string {
	t.Helper()
	p, err := policies.Create(t.Context(), &ngrok.IPPolicyCreate{Description: "pair"})
	require.NoError(t, err)
	return p.ID
}

func remoteExists(t *testing.T, policies *nmockapi.IPPolicyClient, id string) bool {
	t.Helper()
	_, err := policies.Get(t.Context(), id)
	if err != nil {
		require.True(t, ngrok.IsNotFound(err), "unexpected error: %v", err)
		return false
	}
	return true
}

func remoteCount(t *testing.T, policies *nmockapi.IPPolicyClient) int {
	t.Helper()
	n := 0
	iter := policies.List(&ngrok.FilteredPaging{})
	for iter.Next(t.Context()) {
		n++
	}
	require.NoError(t, iter.Err())
	return n
}

type objOpt func(client.Object)

func withID(id string) objOpt {
	return func(o client.Object) { o.(ippolicy.IPPolicyResource).SetID(id) }
}

// deleting marks the object as being deleted. It carries the operator's
// finalizer, as it would on a real cluster once reconciled.
func deleting() objOpt {
	return func(o client.Object) {
		now := metav1.Now()
		o.SetDeletionTimestamp(&now)
		o.SetFinalizers([]string{util.FinalizerName})
	}
}

func finalized() objOpt {
	return func(o client.Object) { o.SetFinalizers([]string{util.FinalizerName}) }
}

func v1Obj(opts ...objOpt) *ngrokv1.IPPolicy {
	p := &ngrokv1.IPPolicy{
		Name: pairKey.Name, Namespace: pairKey.Namespace, Generation: 1,
		Spec: ngrokv1.IPPolicySpec{Description: "v1", Rules: []ngrokv1.IPPolicyRule{{CIDR: "10.0.0.0/8", Action: "deny"}}},
	}
	for _, o := range opts {
		o(p)
	}
	return p
}

func legacyObj(opts ...objOpt) *ingressv1alpha1.IPPolicy {
	p := &ingressv1alpha1.IPPolicy{
		Name: pairKey.Name, Namespace: pairKey.Namespace, Generation: 1,
		Spec: ingressv1alpha1.IPPolicySpec{Description: "legacy", Rules: []ingressv1alpha1.IPPolicyRule{{CIDR: "10.0.0.0/8", Action: "allow"}}},
	}
	for _, o := range opts {
		o(p)
	}
	return p
}

// get returns the current state of the pair object of obj's kind, or nil if
// it no longer exists.
func get[T client.Object](t *testing.T, c client.Client, obj T) T {
	t.Helper()
	fresh := obj.DeepCopyObject().(T)
	err := c.Get(t.Context(), pairKey, fresh)
	if apierrors.IsNotFound(err) {
		var zero T
		return zero
	}
	require.NoError(t, err)
	return fresh
}

func readyReason(t *testing.T, p ippolicy.IPPolicyResource) string {
	t.Helper()
	c := meta.FindStatusCondition(*p.GetConditions(), ippolicy.ConditionIPPolicyReady)
	require.NotNil(t, c, "Ready condition not set")
	return c.Reason
}

func emitted(rec *events.FakeRecorder, reason string) bool {
	for {
		select {
		case e := <-rec.Events:
			if strings.Contains(e, " "+reason+" ") {
				return true
			}
		default:
			return false
		}
	}
}

func TestIPPolicyPair_OnlyLegacyIsTheWriter(t *testing.T) {
	policies := nmockapi.NewIPPolicyClient()
	env := newPairEnv(t, policies, []client.Object{legacyObj()})

	_, err := env.reconcile(t)
	require.NoError(t, err)

	legacy := get(t, env.kube, &ingressv1alpha1.IPPolicy{})
	require.NotEmpty(t, legacy.GetID())
	assert.True(t, util.HasFinalizer(legacy))
	assert.Equal(t, ippolicy.ReasonIPPolicyActive, readyReason(t, legacy))
	assert.Equal(t, 1, remoteCount(t, policies))
}

func TestIPPolicyPair_OnlyV1IsTheWriter(t *testing.T) {
	policies := nmockapi.NewIPPolicyClient()
	env := newPairEnv(t, policies, []client.Object{v1Obj()})

	_, err := env.reconcile(t)
	require.NoError(t, err)

	v1 := get(t, env.kube, &ngrokv1.IPPolicy{})
	require.NotEmpty(t, v1.GetID())
	assert.Equal(t, ippolicy.ReasonIPPolicyActive, readyReason(t, v1))
	assert.Equal(t, 1, remoteCount(t, policies))
}

func TestIPPolicyPair_V1AdoptsLegacyID(t *testing.T) {
	policies := nmockapi.NewIPPolicyClient()
	id := remote(t, policies)
	env := newPairEnv(t, policies, []client.Object{v1Obj(), legacyObj(withID(id), finalized())})

	_, err := env.reconcile(t)
	require.NoError(t, err)

	v1 := get(t, env.kube, &ngrokv1.IPPolicy{})
	legacy := get(t, env.kube, &ingressv1alpha1.IPPolicy{})
	assert.Equal(t, id, v1.GetID())
	assert.Equal(t, ippolicy.ReasonIPPolicyActive, readyReason(t, v1))
	assert.Equal(t, ippolicy.ReasonSupersededByV1, readyReason(t, legacy))
	assert.Equal(t, 1, remoteCount(t, policies), "adoption must not create a second remote policy")
	assert.True(t, emitted(env.recorder, EventIPPolicyAdopted))

	remotePolicy, err := policies.Get(t.Context(), id)
	require.NoError(t, err)
	assert.Equal(t, "v1", remotePolicy.Description, "v1 is the writer after adoption")
}

func TestIPPolicyPair_FreshTwinsShareOneRemote(t *testing.T) {
	policies := nmockapi.NewIPPolicyClient()
	env := newPairEnv(t, policies, []client.Object{v1Obj(), legacyObj()})

	_, err := env.reconcile(t)
	require.NoError(t, err)

	legacy := get(t, env.kube, &ingressv1alpha1.IPPolicy{})
	assert.Equal(t, 1, remoteCount(t, policies), "a legacy twin without an ID must not create its own remote policy")
	assert.Empty(t, legacy.GetID())
	assert.Equal(t, ippolicy.ReasonSupersededByV1, readyReason(t, legacy))
}

func TestIPPolicyPair_PassiveLegacyMakesNoAPICalls(t *testing.T) {
	policies := nmockapi.NewIPPolicyClient()
	id := remote(t, policies)
	env := newPairEnv(t, policies, []client.Object{v1Obj(withID(id), finalized()), legacyObj(withID(id), finalized())})
	_, err := env.reconcile(t)
	require.NoError(t, err)

	policies.ResetUpdateCallCount()
	legacy := get(t, env.kube, &ingressv1alpha1.IPPolicy{})
	legacy.Spec.Description = "edited after migrating"
	legacy.Generation++
	require.NoError(t, env.kube.Update(t.Context(), legacy))

	_, err = env.reconcile(t)
	require.NoError(t, err)
	assert.Zero(t, policies.UpdateCallCount())
	remotePolicy, err := policies.Get(t.Context(), id)
	require.NoError(t, err)
	assert.Equal(t, "v1", remotePolicy.Description)
}

func TestIPPolicyPair_DifferentIDsAreLeftAlone(t *testing.T) {
	policies := nmockapi.NewIPPolicyClient()
	v1ID, legacyID := remote(t, policies), remote(t, policies)
	env := newPairEnv(t, policies, []client.Object{v1Obj(withID(v1ID), finalized()), legacyObj(withID(legacyID), finalized())})

	_, err := env.reconcile(t)
	require.NoError(t, err)

	assert.Equal(t, v1ID, get(t, env.kube, &ngrokv1.IPPolicy{}).GetID())
	assert.Equal(t, legacyID, get(t, env.kube, &ingressv1alpha1.IPPolicy{}).GetID())
	assert.True(t, remoteExists(t, policies, v1ID))
	assert.True(t, remoteExists(t, policies, legacyID))
	assert.True(t, emitted(env.recorder, EventIPPolicyIDConflict))
}

func TestIPPolicyPair_DeletingLegacyKeepsAdoptedRemote(t *testing.T) {
	policies := nmockapi.NewIPPolicyClient()
	id := remote(t, policies)
	// The re-stamp-then-delete path: v1 hasn't adopted yet when legacy is deleted.
	env := newPairEnv(t, policies, []client.Object{v1Obj(), legacyObj(withID(id), deleting())})

	_, err := env.reconcile(t)
	require.NoError(t, err)

	assert.Nil(t, get(t, env.kube, &ingressv1alpha1.IPPolicy{}), "legacy finalizer should be released")
	assert.Equal(t, id, get(t, env.kube, &ngrokv1.IPPolicy{}).GetID())
	assert.True(t, remoteExists(t, policies, id))
}

func TestIPPolicyPair_DeletingV1HandsBackToLegacy(t *testing.T) {
	policies := nmockapi.NewIPPolicyClient()
	id := remote(t, policies)
	env := newPairEnv(t, policies, []client.Object{v1Obj(withID(id), deleting()), legacyObj(withID(id), finalized())})

	_, err := env.reconcile(t)
	require.NoError(t, err)

	assert.Nil(t, get(t, env.kube, &ngrokv1.IPPolicy{}))
	assert.True(t, remoteExists(t, policies, id))
	legacy := get(t, env.kube, &ingressv1alpha1.IPPolicy{})
	assert.Equal(t, ippolicy.ReasonIPPolicyActive, readyReason(t, legacy), "legacy is the writer again in the same pass")
	remotePolicy, err := policies.Get(t.Context(), id)
	require.NoError(t, err)
	assert.Equal(t, "legacy", remotePolicy.Description)
}

func TestIPPolicyPair_DeletingBothDeletesRemoteOnce(t *testing.T) {
	policies := nmockapi.NewIPPolicyClient()
	id := remote(t, policies)
	env := newPairEnv(t, policies, []client.Object{v1Obj(withID(id), deleting()), legacyObj(withID(id), deleting())})

	_, err := env.reconcile(t)
	require.NoError(t, err)

	assert.Nil(t, get(t, env.kube, &ngrokv1.IPPolicy{}))
	assert.Nil(t, get(t, env.kube, &ingressv1alpha1.IPPolicy{}))
	assert.False(t, remoteExists(t, policies, id))
}

func TestIPPolicyPair_DrainDeletesWithoutAdopting(t *testing.T) {
	policies := nmockapi.NewIPPolicyClient()
	id := remote(t, policies)
	env := newPairEnv(t, policies, []client.Object{v1Obj(), legacyObj(withID(id), deleting())}, withDrainState(drain.AlwaysDraining{}))

	_, err := env.reconcile(t)
	require.NoError(t, err)

	assert.Nil(t, get(t, env.kube, &ingressv1alpha1.IPPolicy{}), "drain must not wait on adoption")
	assert.False(t, remoteExists(t, policies, id), "legacy was the last holder of the ID")
	v1 := get(t, env.kube, &ngrokv1.IPPolicy{})
	assert.Empty(t, v1.GetID())
	assert.False(t, util.HasFinalizer(v1), "no finalizers are added while draining")
}

func TestIPPolicyPair_DrainSkipsNonDeletes(t *testing.T) {
	policies := nmockapi.NewIPPolicyClient()
	env := newPairEnv(t, policies, []client.Object{v1Obj(), legacyObj()}, withDrainState(drain.AlwaysDraining{}))

	_, err := env.reconcile(t)
	require.NoError(t, err)

	assert.Zero(t, remoteCount(t, policies))
	assert.False(t, util.HasFinalizer(get(t, env.kube, &ngrokv1.IPPolicy{})))
}

func TestIPPolicyPair_FailedAdoptionWriteKeepsLegacyFinalizer(t *testing.T) {
	policies := nmockapi.NewIPPolicyClient()
	id := remote(t, policies)
	failV1Status := interceptor.Funcs{
		SubResourceUpdate: func(ctx context.Context, c client.Client, sub string, obj client.Object, opts ...client.SubResourceUpdateOption) error {
			if _, ok := obj.(*ngrokv1.IPPolicy); ok {
				return errors.New("injected status write failure")
			}
			return c.SubResource(sub).Update(ctx, obj, opts...)
		},
	}
	env := newPairEnv(t, policies, []client.Object{v1Obj(), legacyObj(withID(id), deleting())}, withInterceptor(failV1Status))

	_, err := env.reconcile(t)
	require.Error(t, err)

	legacy := get(t, env.kube, &ingressv1alpha1.IPPolicy{})
	require.NotNil(t, legacy, "the legacy finalizer must not be released before v1 has persisted the ID")
	assert.True(t, util.HasFinalizer(legacy))
	assert.True(t, remoteExists(t, policies, id))
}

func TestIPPolicyPair_LiveTwinBlocksRemoteDelete(t *testing.T) {
	policies := nmockapi.NewIPPolicyClient()
	id := remote(t, policies)
	// The cache hasn't seen the v1 twin yet, but the API server has it.
	live := fake.NewClientBuilder().WithScheme(pairScheme(t)).WithObjects(v1Obj()).Build()
	env := newPairEnv(t, policies, []client.Object{legacyObj(withID(id), deleting())}, withAPIReader(live))

	res, err := env.reconcile(t)
	require.NoError(t, err)

	assert.NotZero(t, res.RequeueAfter)
	assert.True(t, remoteExists(t, policies, id))
	assert.NotNil(t, get(t, env.kube, &ingressv1alpha1.IPPolicy{}), "legacy finalizer kept until v1 can adopt")
}

func TestIPPolicyPair_RemoteDeletedOutOfBandIsRecreatedOnce(t *testing.T) {
	policies := nmockapi.NewIPPolicyClient()
	id := remote(t, policies)
	env := newPairEnv(t, policies, []client.Object{v1Obj(withID(id), finalized()), legacyObj(withID(id), finalized())})
	require.NoError(t, policies.Delete(t.Context(), id))

	_, err := env.reconcile(t)
	require.Error(t, err, "the first pass reports the missing remote policy")
	assert.Empty(t, get(t, env.kube, &ingressv1alpha1.IPPolicy{}).GetID(), "the stale ID must be cleared on the legacy twin too")

	_, err = env.reconcile(t)
	require.NoError(t, err)
	v1 := get(t, env.kube, &ngrokv1.IPPolicy{})
	require.NotEmpty(t, v1.GetID())
	assert.NotEqual(t, id, v1.GetID())
	assert.Equal(t, 1, remoteCount(t, policies))
	assert.Equal(t, ippolicy.ReasonSupersededByV1, readyReason(t, get(t, env.kube, &ingressv1alpha1.IPPolicy{})))
}
