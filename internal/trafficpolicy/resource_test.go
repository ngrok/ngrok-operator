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
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	ngrokv1 "github.com/ngrok/ngrok-operator/api/ngrok/v1"
	ngrokv1alpha1 "github.com/ngrok/ngrok-operator/api/ngrok/v1alpha1"
)

// canonicalGetErrClient wraps a client.Client and forces Get to fail with a
// fixed error only when the target is the canonical ngrokv1.TrafficPolicy;
// every other Get (namely the legacy fallback) passes through to the
// underlying client untouched. This lets tests simulate the canonical Get
// failing (e.g. with a meta.NoMatchError) while still exercising the real
// legacy-lookup behavior.
type canonicalGetErrClient struct {
	client.Client
	err error
}

func (c canonicalGetErrClient) Get(ctx context.Context, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
	if _, ok := obj.(*ngrokv1.TrafficPolicy); ok {
		return c.err
	}
	return c.Client.Get(ctx, key, obj, opts...)
}

func newLookupTestClient(t *testing.T, objs ...client.Object) client.Client {
	t.Helper()
	scheme := runtime.NewScheme()
	require.NoError(t, ngrokv1.AddToScheme(scheme))
	require.NoError(t, ngrokv1alpha1.AddToScheme(scheme))
	return fake.NewClientBuilder().WithScheme(scheme).WithObjects(objs...).Build()
}

func newCanonicalPolicy(name, namespace, body string) *ngrokv1.TrafficPolicy {
	return &ngrokv1.TrafficPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Spec: ngrokv1.TrafficPolicySpec{
			Policy: json.RawMessage(body),
		},
	}
}

func TestLookupPolicy_Canonical_Found(t *testing.T) {
	policy := newCanonicalPolicy("my-policy", "ns", `{"on_http_request":[{"name":"log"}]}`)
	c := newLookupTestClient(t, policy)

	res, err := LookupPolicy(context.Background(), c, client.ObjectKey{Name: "my-policy", Namespace: "ns"})

	require.NoError(t, err)
	assert.Contains(t, string(res.Policy), `"log"`)
	returned, ok := res.Object.(*ngrokv1.TrafficPolicy)
	require.True(t, ok, "expected Object to be a *ngrokv1.TrafficPolicy, got %T", res.Object)
	assert.Equal(t, policy.Name, returned.Name)
	assert.False(t, res.LegacyKind)
}

func TestLookupPolicy_FallsBackToLegacy_WhenCanonicalNotFound(t *testing.T) {
	legacy := newLegacyPolicy("my-policy", "ns", `{"on_http_request":[{"name":"legacy"}]}`)
	c := newLookupTestClient(t, legacy)

	res, err := LookupPolicy(context.Background(), c, client.ObjectKey{Name: "my-policy", Namespace: "ns"})

	require.NoError(t, err)
	assert.Contains(t, string(res.Policy), `"legacy"`)
	assert.True(t, res.LegacyKind)
}

func TestLookupPolicy_NeitherFound_ReturnsNotFoundError(t *testing.T) {
	c := newLookupTestClient(t)

	res, err := LookupPolicy(context.Background(), c, client.ObjectKey{Name: "missing", Namespace: "ns"})

	require.Error(t, err)
	assert.ErrorIs(t, err, ErrTrafficPolicyNotFound)
	assert.Equal(t, PolicyLookup{}, res)
}

func TestLookupPolicy_CanonicalGetError_IsRetryable(t *testing.T) {
	// A transient client error (not NotFound, not a NoMatchError) must be
	// returned as-is rather than falling back to the legacy kind, so the
	// caller can requeue with backoff.
	c := canonicalGetErrClient{
		Client: newLookupTestClient(t),
		err:    errors.NewServiceUnavailable("apiserver down"),
	}

	res, err := LookupPolicy(context.Background(), c, client.ObjectKey{Name: "whatever", Namespace: "ns"})

	require.Error(t, err)
	assert.NotErrorIs(t, err, ErrTrafficPolicyNotFound)
	assert.Equal(t, PolicyLookup{}, res)
}

func TestLookupPolicy_CanonicalNoMatchError_FallsBackToLegacy(t *testing.T) {
	// A NoMatchError from the canonical Get means the ngrok.com/v1
	// TrafficPolicy CRD/RESTMapping isn't present in the cluster (e.g. an
	// older CRD install that predates the canonical kind) rather than a
	// transient failure. That must be treated the same as a NotFound and
	// fall through to the legacy lookup instead of being surfaced as a
	// retryable error.
	legacy := newLegacyPolicy("my-policy", "ns", `{"on_http_request":[{"name":"legacy"}]}`)
	c := canonicalGetErrClient{
		Client: newLookupTestClient(t, legacy),
		err: &meta.NoKindMatchError{
			GroupKind:        schema.GroupKind{Group: "ngrok.com", Kind: "TrafficPolicy"},
			SearchedVersions: []string{"v1"},
		},
	}

	res, err := LookupPolicy(context.Background(), c, client.ObjectKey{Name: "my-policy", Namespace: "ns"})

	require.NoError(t, err)
	assert.Contains(t, string(res.Policy), `"legacy"`)
	assert.True(t, res.LegacyKind)
}

func TestLookupPolicy_CanonicalNoMatchError_LegacyAlsoMissing_ReturnsNotFoundError(t *testing.T) {
	c := canonicalGetErrClient{
		Client: newLookupTestClient(t),
		err: &meta.NoResourceMatchError{
			PartialResource: schema.GroupVersionResource{Group: "ngrok.com", Version: "v1", Resource: "trafficpolicies"},
		},
	}

	res, err := LookupPolicy(context.Background(), c, client.ObjectKey{Name: "missing", Namespace: "ns"})

	require.Error(t, err)
	assert.ErrorIs(t, err, ErrTrafficPolicyNotFound)
	assert.Equal(t, PolicyLookup{}, res)
}
