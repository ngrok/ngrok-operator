package ngrok

import (
	"context"
	"testing"

	"github.com/go-logr/logr"
	"github.com/ngrok/ngrok-api-go/v7"
	ngrokv1alpha1 "github.com/ngrok/ngrok-operator/api/ngrok/v1alpha1"
	"github.com/ngrok/ngrok-operator/internal/controller"
	"github.com/ngrok/ngrok-operator/internal/mocks/nmockapi"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/events"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func newTestReconciler(t *testing.T) *KubernetesOperatorReconciler {
	t.Helper()

	s := runtime.NewScheme()
	require.NoError(t, ngrokv1alpha1.AddToScheme(s))
	require.NoError(t, v1.AddToScheme(s))

	ns := &v1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-ns",
			UID:  "test-uid",
		},
	}

	fakeClient := fake.NewClientBuilder().
		WithScheme(s).
		WithStatusSubresource(&ngrokv1alpha1.KubernetesOperator{}).
		WithObjects(ns).
		Build()

	clientset := nmockapi.NewClientset()

	r := &KubernetesOperatorReconciler{
		Client:         fakeClient,
		Scheme:         s,
		Log:            logr.Discard(),
		Recorder:       events.NewFakeRecorder(10),
		NgrokClientset: clientset,
		K8sOpNamespace: "test-ns",
		K8sOpName:      "test-ko",
	}

	r.controller = &controller.BaseController[*ngrokv1alpha1.KubernetesOperator]{
		Kube:      fakeClient,
		Log:       logr.Discard(),
		Recorder:  events.NewFakeRecorder(10),
		Namespace: &r.K8sOpNamespace,
		StatusID:  func(obj *ngrokv1alpha1.KubernetesOperator) string { return obj.Status.ID },
		Create:    r.create,
		Update:    r.update,
		Delete:    r.delete,
	}

	return r
}

func TestNilBindingWithBindingsFeatureEnabled(t *testing.T) {
	t.Parallel()

	tc := []struct {
		name string
		ko   *ngrokv1alpha1.KubernetesOperator
	}{
		{
			name: "create with bindings enabled but nil Binding",
			ko: &ngrokv1alpha1.KubernetesOperator{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-ko",
					Namespace: "test-ns",
				},
				Spec: ngrokv1alpha1.KubernetesOperatorSpec{
					EnabledFeatures: []string{ngrokv1alpha1.KubernetesOperatorFeatureBindings},
					Binding:         nil,
				},
			},
		},
	}

	for _, tt := range tc {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			r := newTestReconciler(t)
			ctx := context.Background()

			// create the object in k8s so status updates work
			require.NoError(t, r.Client.Create(ctx, tt.ko))

			// Should not panic — returns error instead of nil-deref
			err := r.create(ctx, tt.ko)
			assert.Error(t, err)
			assert.Contains(t, err.Error(), "binding")
		})
	}
}

func TestNilBindingWithBindingsFeatureEnabled_Update(t *testing.T) {
	t.Parallel()

	r := newTestReconciler(t)
	ctx := context.Background()

	ko := &ngrokv1alpha1.KubernetesOperator{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-ko",
			Namespace: "test-ns",
		},
		Spec: ngrokv1alpha1.KubernetesOperatorSpec{
			EnabledFeatures: []string{ngrokv1alpha1.KubernetesOperatorFeatureBindings},
			Binding:         nil,
		},
	}

	require.NoError(t, r.Client.Create(ctx, ko))

	ngrokKo := &ngrok.KubernetesOperator{
		ID:  "ko_existing",
		URI: "https://example.com",
	}

	// Should not panic — returns error instead of nil-deref
	err := r._update(ctx, ko, ngrokKo)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "binding")
}

func TestNilDeployment_FindExisting(t *testing.T) {
	t.Parallel()

	r := newTestReconciler(t)
	ctx := context.Background()

	ko := &ngrokv1alpha1.KubernetesOperator{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-ko",
			Namespace: "test-ns",
		},
		Spec: ngrokv1alpha1.KubernetesOperatorSpec{
			Deployment: nil,
		},
	}

	// Should not panic even with nil Deployment
	result, err := r.findExisting(ctx, ko)
	assert.NoError(t, err)
	assert.Nil(t, result)
}

func TestNilDeployment_Create(t *testing.T) {
	t.Parallel()

	r := newTestReconciler(t)
	ctx := context.Background()

	ko := &ngrokv1alpha1.KubernetesOperator{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-ko",
			Namespace: "test-ns",
		},
		Spec: ngrokv1alpha1.KubernetesOperatorSpec{
			Deployment: nil,
		},
	}

	require.NoError(t, r.Client.Create(ctx, ko))

	// Should not panic with nil Deployment
	err := r.create(ctx, ko)
	assert.NoError(t, err)
}
