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
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"

	commonv1alpha1 "github.com/ngrok/ngrok-operator/api/common/v1alpha1"
	ingressv1alpha1 "github.com/ngrok/ngrok-operator/api/ingress/v1alpha1"
	ngrokv1 "github.com/ngrok/ngrok-operator/api/ngrok/v1"
)

var ensureKey = types.NamespacedName{Name: "a-example-com", Namespace: "default"}

func ensureDefaults(d *ngrokv1.Domain) {
	d.Spec.Domain = "a.example.com"
	d.Spec.ReclaimPolicy = ngrokv1.DomainReclaimPolicyDelete
}

func TestEnsureV1_CreatesFromDefaultsWithoutLegacy(t *testing.T) {
	_, c := newTestManager(t)

	got, created, err := EnsureV1(t.Context(), c, ensureKey, ensureDefaults)
	require.NoError(t, err)
	assert.True(t, created)
	assert.Equal(t, "a.example.com", got.Spec.Domain)

	stored := &ngrokv1.Domain{}
	require.NoError(t, c.Get(t.Context(), ensureKey, stored))
	assert.Equal(t, ngrokv1.DomainReclaimPolicyDelete, stored.Spec.ReclaimPolicy)
}

func TestEnsureV1_ReturnsExisting(t *testing.T) {
	existing := &ngrokv1.Domain{
		ObjectMeta: metav1.ObjectMeta{Name: ensureKey.Name, Namespace: ensureKey.Namespace},
		Spec:       ngrokv1.DomainSpec{Domain: "a.example.com", Description: "already here"},
	}
	_, c := newTestManager(t, existing)

	got, created, err := EnsureV1(t.Context(), c, ensureKey, func(*ngrokv1.Domain) {
		t.Fatal("defaults must not run for an existing object")
	})
	require.NoError(t, err)
	assert.False(t, created)
	assert.Equal(t, "already here", got.Spec.Description)
}

// The v1 copy becomes the object that pushes its spec to the shared
// reservation, so it must carry whatever the legacy object set, never
// defaults.
func TestEnsureV1_CopiesLegacySpec(t *testing.T) {
	legacy := &ingressv1alpha1.Domain{
		ObjectMeta: metav1.ObjectMeta{
			Name: ensureKey.Name, Namespace: ensureKey.Namespace,
			Labels:      map[string]string{"team": "edge"},
			Annotations: map[string]string{"kubectl.kubernetes.io/last-applied-configuration": "{}"},
		},
		Spec: ingressv1alpha1.DomainSpec{
			Domain:        "a.example.com",
			Description:   "hand-written",
			Metadata:      commonv1alpha1.MetadataFromLegacyString(`{"owner":"me"}`),
			ResolvesTo:    []ingressv1alpha1.DomainResolvesToEntry{{Value: "eu"}},
			ReclaimPolicy: ingressv1alpha1.DomainReclaimPolicyRetain,
		},
	}
	_, c := newTestManager(t, legacy)

	got, created, err := EnsureV1(t.Context(), c, ensureKey, func(*ngrokv1.Domain) {
		t.Fatal("defaults must not run when a legacy object exists")
	})
	require.NoError(t, err)
	assert.True(t, created)
	assert.Equal(t, ngrokv1.DomainSpec{
		Domain:        "a.example.com",
		Description:   "hand-written",
		Metadata:      map[string]string{"owner": "me"},
		ResolvesTo:    []ngrokv1.DomainResolvesToEntry{{Value: "eu"}},
		ReclaimPolicy: ngrokv1.DomainReclaimPolicyRetain,
	}, got.Spec)
	assert.Equal(t, map[string]string{"team": "edge"}, got.Labels)
	assert.Empty(t, got.Annotations, "annotations belong to whatever applied the legacy object")
}

func TestEnsureV1_BlockedByUnmappableMetadata(t *testing.T) {
	for _, raw := range []string{`"free text"`, `"{\"a\":1}"`, `{"a":{"b":"c"}}`} {
		t.Run(raw, func(t *testing.T) {
			legacy := &ingressv1alpha1.Domain{
				ObjectMeta: metav1.ObjectMeta{Name: ensureKey.Name, Namespace: ensureKey.Namespace},
				Spec:       ingressv1alpha1.DomainSpec{Domain: "a.example.com", Metadata: json.RawMessage(raw)},
			}
			_, c := newTestManager(t, legacy)

			_, _, err := EnsureV1(t.Context(), c, ensureKey, ensureDefaults)
			require.ErrorIs(t, err, ErrMigrationBlocked)

			err = c.Get(t.Context(), ensureKey, &ngrokv1.Domain{})
			assert.True(t, apierrors.IsNotFound(err), "no v1 copy may be created")
		})
	}
}

func TestEnsureV1_EmptyLegacyMetadataIsNotBlocked(t *testing.T) {
	legacy := &ingressv1alpha1.Domain{
		ObjectMeta: metav1.ObjectMeta{Name: ensureKey.Name, Namespace: ensureKey.Namespace},
		Spec:       ingressv1alpha1.DomainSpec{Domain: "a.example.com"},
	}
	_, c := newTestManager(t, legacy)

	got, _, err := EnsureV1(t.Context(), c, ensureKey, ensureDefaults)
	require.NoError(t, err)
	assert.Nil(t, got.Spec.Metadata)
}

// The driver and an endpoint reconcile can both decide to create the same v1
// Domain; the loser must return the winner's object, not an error.
func TestEnsureV1_ConcurrentCreate(t *testing.T) {
	winner := &ngrokv1.Domain{
		ObjectMeta: metav1.ObjectMeta{Name: ensureKey.Name, Namespace: ensureKey.Namespace},
		Spec:       ngrokv1.DomainSpec{Domain: "a.example.com", Description: "winner"},
	}
	c := fake.NewClientBuilder().WithScheme(setupScheme()).WithInterceptorFuncs(interceptor.Funcs{
		Create: func(ctx context.Context, cl client.WithWatch, obj client.Object, _ ...client.CreateOption) error {
			// Someone else creates it between our Get and Create.
			require.NoError(t, cl.Create(ctx, winner.DeepCopy()))
			return apierrors.NewAlreadyExists(schema.GroupResource{Group: "ngrok.com", Resource: "domains"}, obj.GetName())
		},
	}).Build()

	got, created, err := EnsureV1(t.Context(), c, ensureKey, ensureDefaults)
	require.NoError(t, err)
	assert.False(t, created)
	assert.Equal(t, "winner", got.Spec.Description)
}
