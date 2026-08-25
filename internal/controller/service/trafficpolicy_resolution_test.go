package service

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	ngrokv1 "github.com/ngrok/ngrok-operator/api/ngrok/v1"
	ngrokv1alpha1 "github.com/ngrok/ngrok-operator/api/ngrok/v1alpha1"
	"github.com/ngrok/ngrok-operator/internal/annotations"
	"github.com/ngrok/ngrok-operator/internal/testutils"
	"github.com/ngrok/ngrok-operator/internal/trafficpolicy"
)

// TestGetTrafficPolicyForService covers the Service controller's policy
// resolution across both served kinds.
//
// This path previously did a hard client Get against the deprecated
// ngrok.k8s.ngrok.com/v1alpha1 kind only, so a user who followed the
// migration guidance and re-stamped their manifest to ngrok.com/v1
// TrafficPolicy got a NotFound — the annotation names a policy by bare name,
// with no kind or group, so nothing about the Service told them why.
func TestGetTrafficPolicyForService(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, corev1.AddToScheme(scheme))
	require.NoError(t, ngrokv1.AddToScheme(scheme))
	require.NoError(t, ngrokv1alpha1.AddToScheme(scheme))

	svcWithPolicy := func(name string) *corev1.Service {
		return &corev1.Service{
			ObjectMeta: metav1.ObjectMeta{
				Name:        "svc",
				Namespace:   "ns",
				Annotations: map[string]string{annotations.TrafficPolicyAnnotation: name},
			},
		}
	}

	canonical := testutils.NewTestTrafficPolicy("shared", "ns", `{"src":"v1"}`)
	legacy := testutils.NewTestNgrokTrafficPolicy("shared", "ns", `{"src":"legacy"}`)
	legacyOnly := testutils.NewTestNgrokTrafficPolicy("legacy-only", "ns", `{"src":"legacy-only"}`)

	tests := []struct {
		name           string
		objects        []runtime.Object
		policyName     string
		wantPolicy     string
		wantLegacyKind bool
	}{
		{
			name:       "canonical kind resolves",
			objects:    []runtime.Object{&canonical},
			policyName: "shared",
			wantPolicy: `{"src":"v1"}`,
		},
		{
			name:           "legacy kind still resolves",
			objects:        []runtime.Object{&legacyOnly},
			policyName:     "legacy-only",
			wantPolicy:     `{"src":"legacy-only"}`,
			wantLegacyKind: true,
		},
		{
			name:       "canonical wins when both exist",
			objects:    []runtime.Object{&canonical, &legacy},
			policyName: "shared",
			wantPolicy: `{"src":"v1"}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := fake.NewClientBuilder().WithScheme(scheme).WithRuntimeObjects(tt.objects...).Build()

			got, err := getTrafficPolicyForService(context.Background(), c, svcWithPolicy(tt.policyName))
			require.NoError(t, err)
			require.NotNil(t, got)
			assert.Equal(t, json.RawMessage(tt.wantPolicy), got.Policy)
			assert.Equal(t, tt.wantLegacyKind, got.LegacyKind)
		})
	}

	t.Run("no annotation returns no policy and no error", func(t *testing.T) {
		c := fake.NewClientBuilder().WithScheme(scheme).Build()
		svc := &corev1.Service{ObjectMeta: metav1.ObjectMeta{Name: "svc", Namespace: "ns"}}

		got, err := getTrafficPolicyForService(context.Background(), c, svc)
		require.NoError(t, err)
		assert.Nil(t, got, "a service without the annotation must not resolve a policy")
	})

	t.Run("missing in both kinds is a terminal not-found", func(t *testing.T) {
		c := fake.NewClientBuilder().WithScheme(scheme).Build()

		_, err := getTrafficPolicyForService(context.Background(), c, svcWithPolicy("nowhere"))
		require.Error(t, err)
		assert.ErrorIs(t, err, trafficpolicy.ErrTrafficPolicyNotFound)
	})
}
