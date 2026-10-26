package kubernetesoperator

import (
	"context"
	"testing"

	ngrokv1 "github.com/ngrok/ngrok-operator/api/ngrok/v1"
	ngrokv1alpha1 "github.com/ngrok/ngrok-operator/api/ngrok/v1alpha1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

const (
	testNamespace = "ngrok-operator"
	testName      = "ngrok-operator"
)

var testKey = client.ObjectKey{Namespace: testNamespace, Name: testName}

func canonicalKO(id, ingressEndpoint string) *ngrokv1.KubernetesOperator {
	return &ngrokv1.KubernetesOperator{
		Namespace: testNamespace, Name: testName,
		Status: ngrokv1.KubernetesOperatorStatus{ID: id, BindingsIngressEndpoint: ingressEndpoint},
	}
}

func legacyKO(id, ingressEndpoint string) *ngrokv1alpha1.KubernetesOperator {
	return &ngrokv1alpha1.KubernetesOperator{
		Namespace: testNamespace, Name: testName,
		Spec: ngrokv1alpha1.KubernetesOperatorSpec{
			Binding: &ngrokv1alpha1.KubernetesOperatorBinding{
				EndpointSelectors: []string{"true"},
				TlsSecretName:     "default-tls",
			},
		},
		Status: ngrokv1alpha1.KubernetesOperatorStatus{
			ID:                      id,
			BindingsIngressEndpoint: ingressEndpoint,
			Conditions: []metav1.Condition{{
				Type:   ngrokv1alpha1.KubernetesOperatorConditionDraining,
				Status: metav1.ConditionTrue,
				Reason: ngrokv1alpha1.KubernetesOperatorReasonDrainInProgress,
			}},
		},
	}
}

func newClient(t *testing.T, objs ...client.Object) client.Client {
	t.Helper()
	scheme := runtime.NewScheme()
	require.NoError(t, ngrokv1.AddToScheme(scheme))
	require.NoError(t, ngrokv1alpha1.AddToScheme(scheme))
	return fake.NewClientBuilder().WithScheme(scheme).WithObjects(objs...).WithStatusSubresource(objs...).Build()
}

func TestLookup(t *testing.T) {
	tests := []struct {
		name         string
		objs         []client.Object
		wantNotFound bool
		wantID       string
		wantEndpoint string
		wantLegacy   bool
	}{
		{
			name:         "canonical registered only",
			objs:         []client.Object{canonicalKO("k8sop_canonical", "canonical.example:443")},
			wantID:       "k8sop_canonical",
			wantEndpoint: "canonical.example:443",
		},
		{
			name:         "canonical registered wins over legacy",
			objs:         []client.Object{canonicalKO("k8sop_canonical", "canonical.example:443"), legacyKO("k8sop_legacy", "legacy.example:443")},
			wantID:       "k8sop_canonical",
			wantEndpoint: "canonical.example:443",
		},
		{
			name:         "canonical unregistered falls back to legacy",
			objs:         []client.Object{canonicalKO("", ""), legacyKO("k8sop_legacy", "legacy.example:443")},
			wantID:       "k8sop_legacy",
			wantEndpoint: "legacy.example:443",
			wantLegacy:   true,
		},
		{
			name:         "legacy only",
			objs:         []client.Object{legacyKO("k8sop_legacy", "legacy.example:443")},
			wantID:       "k8sop_legacy",
			wantEndpoint: "legacy.example:443",
			wantLegacy:   true,
		},
		{
			name: "canonical unregistered but draining wins over legacy",
			objs: []client.Object{
				func() client.Object {
					ko := canonicalKO("", "")
					ko.Status.Conditions = []metav1.Condition{{
						Type:   ngrokv1.KubernetesOperatorConditionDraining,
						Status: metav1.ConditionTrue,
						Reason: ngrokv1.KubernetesOperatorReasonDrainInProgress,
					}}
					return ko
				}(),
				legacyKO("k8sop_legacy", "legacy.example:443"),
			},
			wantID: "",
		},
		{
			name:   "canonical unregistered and no legacy",
			objs:   []client.Object{canonicalKO("", "")},
			wantID: "",
		},
		{
			name:         "neither",
			wantNotFound: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ko, err := Lookup(context.Background(), newClient(t, tt.objs...), testKey)
			if tt.wantNotFound {
				require.Error(t, err)
				assert.True(t, apierrors.IsNotFound(err), "expected NotFound, got %v", err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.wantID, ko.Status.ID)
			assert.Equal(t, tt.wantEndpoint, ko.Status.BindingsIngressEndpoint)
			if tt.wantLegacy {
				require.NotNil(t, ko.Spec.Binding)
				assert.Equal(t, []string{"true"}, ko.Spec.Binding.EndpointSelectors)
				assert.Equal(t, "default-tls", ko.Spec.Binding.TlsSecretName)
				require.Len(t, ko.Status.Conditions, 1)
				assert.Equal(t, ngrokv1.KubernetesOperatorConditionDraining, ko.Status.Conditions[0].Type)
			}
		})
	}
}
