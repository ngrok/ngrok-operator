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
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/events"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	ngrokv1alpha1 "github.com/ngrok/ngrok-operator/api/ngrok/v1alpha1"
)

func newTestManager(t *testing.T, objs ...client.Object) (*Manager, *events.FakeRecorder) {
	t.Helper()
	scheme := runtime.NewScheme()
	require.NoError(t, ngrokv1alpha1.AddToScheme(scheme))
	c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(objs...).Build()
	rec := events.NewFakeRecorder(10)
	return NewManager(c, rec), rec
}

// errClient wraps a client.Client and forces Get to fail with a fixed error,
// used to exercise the retryable (non-NotFound) resolution path.
type errClient struct {
	client.Client
	err error
}

func (c errClient) Get(_ context.Context, _ client.ObjectKey, _ client.Object, _ ...client.GetOption) error {
	return c.err
}

func newAgentEndpoint(namespace string, cfg *ngrokv1alpha1.TrafficPolicyCfg) *ngrokv1alpha1.AgentEndpoint {
	return &ngrokv1alpha1.AgentEndpoint{
		ObjectMeta: metav1.ObjectMeta{
			Name:       "test-aep",
			Namespace:  namespace,
			Generation: 7,
		},
		Spec: ngrokv1alpha1.AgentEndpointSpec{
			URL:           "tcp://1.tcp.ngrok.io:1234",
			TrafficPolicy: cfg,
		},
	}
}

func newPolicy(name, namespace, body string) *ngrokv1alpha1.NgrokTrafficPolicy {
	return &ngrokv1alpha1.NgrokTrafficPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Spec: ngrokv1alpha1.NgrokTrafficPolicySpec{
			Policy: json.RawMessage(body),
		},
	}
}

func findCondition(conds []metav1.Condition, t string) *metav1.Condition {
	return meta.FindStatusCondition(conds, t)
}

func TestResolve_NoPolicy_ClearsCondition(t *testing.T) {
	m, _ := newTestManager(t)
	ep := newAgentEndpoint("ns", nil)
	// Seed a stale True condition; Resolve should remove it.
	meta.SetStatusCondition(&ep.Status.Conditions, metav1.Condition{
		Type:   ConditionTrafficPolicy,
		Status: metav1.ConditionTrue,
		Reason: "Stale",
	})

	res, err := m.Resolve(context.Background(), ep)

	require.NoError(t, err)
	require.NotNil(t, res)
	assert.Equal(t, SourceNone, res.Source)
	assert.Empty(t, res.Policy)
	assert.Nil(t, findCondition(ep.Status.Conditions, ConditionTrafficPolicy))
}

func TestResolve_Inline(t *testing.T) {
	m, _ := newTestManager(t)
	body := `{"on_http_request":[{"name":"rate-limit"}]}`
	ep := newAgentEndpoint("ns", &ngrokv1alpha1.TrafficPolicyCfg{
		Inline: json.RawMessage(body),
	})

	res, err := m.Resolve(context.Background(), ep)

	require.NoError(t, err)
	assert.Equal(t, body, res.Policy)
	assert.Equal(t, SourceInline, res.Source)

	// Resolve no longer sets the condition to True on its own; callers
	// invoke MarkApplied after the downstream apply succeeds.
	assert.Nil(t, findCondition(ep.Status.Conditions, ConditionTrafficPolicy))

	m.MarkApplied(ep)
	cond := findCondition(ep.Status.Conditions, ConditionTrafficPolicy)
	require.NotNil(t, cond)
	assert.Equal(t, metav1.ConditionTrue, cond.Status)
	assert.Equal(t, ReasonTrafficPolicyApplied, cond.Reason)
	assert.Equal(t, int64(7), cond.ObservedGeneration)
}

func TestResolve_TargetRef_SameNamespace(t *testing.T) {
	policy := newPolicy("my-policy", "ns", `{"on_http_request":[{"name":"log"}]}`)
	m, _ := newTestManager(t, policy)

	ep := newAgentEndpoint("ns", &ngrokv1alpha1.TrafficPolicyCfg{
		Reference: &ngrokv1alpha1.K8sObjectRef{Name: "my-policy"},
	})

	res, err := m.Resolve(context.Background(), ep)

	require.NoError(t, err)
	assert.Contains(t, res.Policy, `"log"`)
	assert.Equal(t, "my-policy", res.Source)

	// Resolve only sets the negative side; condition stays unset until MarkApplied.
	assert.Nil(t, findCondition(ep.Status.Conditions, ConditionTrafficPolicy))
}

func TestResolve_TargetRef_ResolvesInEndpointNamespace(t *testing.T) {
	// A TrafficPolicy with the same name in a different namespace must NOT be
	// resolved — the ref always resolves in the endpoint's own namespace.
	otherNs := newPolicy("shared", "policies", `{"on_http_request":[{"name":"other-ns"}]}`)
	m, _ := newTestManager(t, otherNs)

	ep := newAgentEndpoint("apps", &ngrokv1alpha1.TrafficPolicyCfg{
		Reference: &ngrokv1alpha1.K8sObjectRef{Name: "shared"},
	})

	res, err := m.Resolve(context.Background(), ep)

	// The policy only exists in "policies"; resolving in "apps" must miss.
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrTrafficPolicyNotFound)
	assert.Nil(t, res)
}

func TestResolve_TargetRef_Missing_SetsErrorCondition(t *testing.T) {
	m, rec := newTestManager(t)
	ep := newAgentEndpoint("ns", &ngrokv1alpha1.TrafficPolicyCfg{
		Reference: &ngrokv1alpha1.K8sObjectRef{Name: "missing"},
	})

	res, err := m.Resolve(context.Background(), ep)

	require.Error(t, err)
	// A missing reference is a terminal (non-retryable) error: callers treat
	// ErrTrafficPolicyNotFound as no-requeue and rely on the TrafficPolicy
	// watch to re-enqueue when the policy is (re)created.
	assert.ErrorIs(t, err, ErrTrafficPolicyNotFound)
	assert.Nil(t, res)

	cond := findCondition(ep.Status.Conditions, ConditionTrafficPolicy)
	require.NotNil(t, cond)
	assert.Equal(t, metav1.ConditionFalse, cond.Status)
	assert.Equal(t, ReasonTrafficPolicyError, cond.Reason)

	// Warning event recorded so users see the missing reference.
	select {
	case ev := <-rec.Events:
		assert.Contains(t, ev, "TrafficPolicyNotFound")
	default:
		t.Fatal("expected a TrafficPolicyNotFound event")
	}
}

func TestResolve_TargetRef_GetError_IsRetryable(t *testing.T) {
	// A transient client error (not NotFound) must stay retryable so the
	// caller requeues with backoff rather than treating it as terminal.
	m, _ := newTestManager(t)
	m.Client = errClient{Client: m.Client, err: errors.NewServiceUnavailable("apiserver down")}

	ep := newAgentEndpoint("ns", &ngrokv1alpha1.TrafficPolicyCfg{
		Reference: &ngrokv1alpha1.K8sObjectRef{Name: "whatever"},
	})

	res, err := m.Resolve(context.Background(), ep)

	require.Error(t, err)
	assert.NotErrorIs(t, err, ErrTrafficPolicyNotFound)
	assert.NotErrorIs(t, err, ErrInvalidConfig)
	assert.Nil(t, res)
}

func TestResolve_Inline_MalformedJSON_TerminalError(t *testing.T) {
	// Inline policy that bypassed admission with malformed JSON is a terminal
	// configuration error, not an infinite requeue.
	m, _ := newTestManager(t)
	ep := newAgentEndpoint("ns", &ngrokv1alpha1.TrafficPolicyCfg{
		Inline: json.RawMessage(`{not valid json`),
	})

	res, err := m.Resolve(context.Background(), ep)

	require.Error(t, err)
	assert.ErrorIs(t, err, ErrInvalidPolicyJSON)
	assert.Nil(t, res)

	cond := findCondition(ep.Status.Conditions, ConditionTrafficPolicy)
	require.NotNil(t, cond)
	assert.Equal(t, metav1.ConditionFalse, cond.Status)
	assert.Equal(t, ReasonTrafficPolicyError, cond.Reason)
}

func TestResolve_InvalidUnion_SetsErrorCondition(t *testing.T) {
	m, _ := newTestManager(t)
	ep := newAgentEndpoint("ns", &ngrokv1alpha1.TrafficPolicyCfg{
		Inline:    json.RawMessage(`{}`),
		Reference: &ngrokv1alpha1.K8sObjectRef{Name: "x"},
	})

	res, err := m.Resolve(context.Background(), ep)

	require.ErrorIs(t, err, ErrInvalidConfig)
	assert.Nil(t, res)

	cond := findCondition(ep.Status.Conditions, ConditionTrafficPolicy)
	require.NotNil(t, cond)
	assert.Equal(t, metav1.ConditionFalse, cond.Status)
}

func TestMarkApplied_FlipsConditionTrueAfterResolve(t *testing.T) {
	m, _ := newTestManager(t)
	ep := newAgentEndpoint("ns", &ngrokv1alpha1.TrafficPolicyCfg{
		Inline: json.RawMessage(`{}`),
	})

	_, err := m.Resolve(context.Background(), ep)
	require.NoError(t, err)
	// Condition still unset right after Resolve.
	assert.Nil(t, findCondition(ep.Status.Conditions, ConditionTrafficPolicy))

	m.MarkApplied(ep)

	cond := findCondition(ep.Status.Conditions, ConditionTrafficPolicy)
	require.NotNil(t, cond)
	assert.Equal(t, metav1.ConditionTrue, cond.Status)
	assert.Equal(t, ReasonTrafficPolicyApplied, cond.Reason)
}

func TestMarkApplied_Noop_WhenNoPolicyConfigured(t *testing.T) {
	m, _ := newTestManager(t)
	ep := newAgentEndpoint("ns", nil)

	m.MarkApplied(ep)

	// No condition should be set because the endpoint has no policy.
	assert.Nil(t, findCondition(ep.Status.Conditions, ConditionTrafficPolicy))
}

func TestSetError_FlipsConditionToFalse(t *testing.T) {
	m, _ := newTestManager(t)
	ep := newAgentEndpoint("ns", &ngrokv1alpha1.TrafficPolicyCfg{
		Inline: json.RawMessage(`{}`),
	})

	_, err := m.Resolve(context.Background(), ep)
	require.NoError(t, err)
	m.MarkApplied(ep)

	// SetError records a False condition with the downstream error message.
	m.SetError(ep, "ngrok api rejected policy")

	cond := findCondition(ep.Status.Conditions, ConditionTrafficPolicy)
	require.NotNil(t, cond)
	assert.Equal(t, metav1.ConditionFalse, cond.Status)
	assert.Equal(t, ReasonTrafficPolicyError, cond.Reason)
	assert.Equal(t, "ngrok api rejected policy", cond.Message)
}

func TestIntendedSource(t *testing.T) {
	cases := []struct {
		name       string
		cfg        *ngrokv1alpha1.TrafficPolicyCfg
		wantSource string
	}{
		{name: "nil cfg", cfg: nil, wantSource: SourceNone},
		{name: "inline", cfg: &ngrokv1alpha1.TrafficPolicyCfg{Inline: json.RawMessage(`{}`)}, wantSource: SourceInline},
		{
			name:       "ref",
			cfg:        &ngrokv1alpha1.TrafficPolicyCfg{Reference: &ngrokv1alpha1.K8sObjectRef{Name: "p"}},
			wantSource: "p",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.wantSource, IntendedSource(tc.cfg))
		})
	}
}
