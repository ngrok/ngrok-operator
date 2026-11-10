package kubernetesoperator

import (
	"context"
	"testing"

	ngrokv1 "github.com/ngrok/ngrok-operator/api/ngrok/v1"
	ngrokv1alpha1 "github.com/ngrok/ngrok-operator/api/ngrok/v1alpha1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
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

var drainingConditions = []metav1.Condition{{
	Type:   ngrokv1.KubernetesOperatorConditionDraining,
	Status: metav1.ConditionTrue,
	Reason: ngrokv1.KubernetesOperatorReasonDrainInProgress,
}}

func canonicalKO(id, ingressEndpoint string) *ngrokv1.KubernetesOperator {
	return &ngrokv1.KubernetesOperator{
		Namespace: testNamespace, Name: testName,
		Status: ngrokv1.KubernetesOperatorStatus{ID: id, BindingsIngressEndpoint: ingressEndpoint},
	}
}

func legacyKO(id, ingressEndpoint string) *ngrokv1alpha1.KubernetesOperator {
	return &ngrokv1alpha1.KubernetesOperator{
		Namespace: testNamespace, Name: testName,
		Status: ngrokv1alpha1.KubernetesOperatorStatus{ID: id, BindingsIngressEndpoint: ingressEndpoint},
	}
}

func drainingCanonicalKO(id string) *ngrokv1.KubernetesOperator {
	ko := canonicalKO(id, "")
	ko.Status.Conditions = drainingConditions
	return ko
}

func drainingLegacyKO(id string) *ngrokv1alpha1.KubernetesOperator {
	ko := legacyKO(id, "")
	ko.Status.Conditions = drainingConditions
	return ko
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
		wantDraining bool
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
			name:         "canonical registered wins over draining legacy",
			objs:         []client.Object{canonicalKO("k8sop_canonical", "canonical.example:443"), drainingLegacyKO("k8sop_legacy")},
			wantID:       "k8sop_canonical",
			wantEndpoint: "canonical.example:443",
		},
		{
			name:         "canonical unregistered falls back to legacy",
			objs:         []client.Object{canonicalKO("", ""), legacyKO("k8sop_legacy", "legacy.example:443")},
			wantID:       "k8sop_legacy",
			wantEndpoint: "legacy.example:443",
		},
		{
			name:         "canonical unregistered falls back to draining legacy",
			objs:         []client.Object{canonicalKO("", ""), drainingLegacyKO("k8sop_legacy")},
			wantID:       "k8sop_legacy",
			wantDraining: true,
		},
		{
			name:         "legacy only",
			objs:         []client.Object{legacyKO("k8sop_legacy", "legacy.example:443")},
			wantID:       "k8sop_legacy",
			wantEndpoint: "legacy.example:443",
		},
		{
			name:         "canonical unregistered but draining wins over legacy",
			objs:         []client.Object{drainingCanonicalKO(""), legacyKO("k8sop_legacy", "legacy.example:443")},
			wantID:       "",
			wantDraining: true,
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
			assert.Equal(t, tt.wantDraining, meta.IsStatusConditionTrue(ko.Status.Conditions, ngrokv1.KubernetesOperatorConditionDraining))
		})
	}
}

func TestFromLegacy(t *testing.T) {
	legacy := drainingLegacyKO("k8sop_legacy")
	legacy.Status.BindingsIngressEndpoint = "legacy.example:443"
	legacy.Spec.Binding = &ngrokv1alpha1.KubernetesOperatorBinding{
		EndpointSelectors: []string{"true"},
		IngressEndpoint:   new("legacy.example:443"),
		TlsSecretName:     "default-tls",
	}

	ko := fromLegacy(legacy)

	assert.Equal(t, legacy.ObjectMeta, ko.ObjectMeta)
	assert.Equal(t, "k8sop_legacy", ko.Status.ID)
	assert.Equal(t, "legacy.example:443", ko.Status.BindingsIngressEndpoint)
	assert.Equal(t, drainingConditions, ko.Status.Conditions)
	require.NotNil(t, ko.Spec.Binding)
	assert.Equal(t, []string{"true"}, ko.Spec.Binding.EndpointSelectors)
	assert.Equal(t, legacy.Spec.Binding.IngressEndpoint, ko.Spec.Binding.IngressEndpoint)
	assert.Equal(t, "default-tls", ko.Spec.Binding.TlsSecretName)
}
