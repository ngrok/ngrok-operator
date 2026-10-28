package ngrok

// LEGACY-kubernetesoperator-kind: delete this whole file at cleanup, together
// with kubernetesoperator_legacy.go.

import (
	"context"
	"errors"
	"testing"

	"github.com/ngrok/ngrok-api-go/v9"
	ngrokv1 "github.com/ngrok/ngrok-operator/api/ngrok/v1"
	ngrokv1alpha1 "github.com/ngrok/ngrok-operator/api/ngrok/v1alpha1"
	"github.com/ngrok/ngrok-operator/internal/mocks/nmockapi"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/tools/events"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"
)

func legacyTwin() *ngrokv1alpha1.KubernetesOperator {
	return &ngrokv1alpha1.KubernetesOperator{Name: "ko", Namespace: "test-ns"}
}

func TestShouldSkipDrain(t *testing.T) {
	legacyGVR := schema.GroupResource{Group: "ngrok.k8s.ngrok.com", Resource: "kubernetesoperators"}
	draining := []metav1.Condition{{
		Type:   ngrokv1.KubernetesOperatorConditionDraining,
		Status: metav1.ConditionTrue,
		Reason: ngrokv1.KubernetesOperatorReasonDrainInProgress,
	}}

	tests := []struct {
		name       string
		objs       []client.Object
		getErr     error
		conditions []metav1.Condition
		wantSkip   bool
		wantErr    bool
	}{
		{name: "legacy twin present skips", objs: []client.Object{legacyTwin()}, wantSkip: true},
		{name: "no legacy twin drains", wantSkip: false},
		{name: "legacy kind not installed drains", getErr: &meta.NoKindMatchError{GroupKind: schema.GroupKind{Group: "ngrok.k8s.ngrok.com", Kind: "KubernetesOperator"}}, wantSkip: false},
		{name: "legacy read forbidden skips", getErr: apierrors.NewForbidden(legacyGVR, "ko", errors.New("rbac")), wantSkip: true},
		{name: "drain already in progress continues", objs: []client.Object{legacyTwin()}, conditions: draining, wantSkip: false},
		{name: "other error", getErr: errors.New("boom"), wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			scheme := runtime.NewScheme()
			require.NoError(t, ngrokv1alpha1.AddToScheme(scheme))
			builder := fake.NewClientBuilder().WithScheme(scheme).WithObjects(tt.objs...)
			if tt.getErr != nil {
				builder = builder.WithInterceptorFuncs(interceptor.Funcs{
					Get: func(context.Context, client.WithWatch, client.ObjectKey, client.Object, ...client.GetOption) error {
						return tt.getErr
					},
				})
			}
			reconciler := &KubernetesOperatorReconciler{APIReader: builder.Build()}
			ko := &ngrokv1.KubernetesOperator{
				Name: "ko", Namespace: "test-ns",
				Status: ngrokv1.KubernetesOperatorStatus{Conditions: tt.conditions},
			}

			skip, err := reconciler.shouldSkipDrain(context.Background(), ko)
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.wantSkip, skip)
		})
	}
}

func TestDeleteSkipsDrainWhenLegacyTwinExists(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, ngrokv1.AddToScheme(scheme))
	require.NoError(t, ngrokv1alpha1.AddToScheme(scheme))

	mockClientset := nmockapi.NewClientset()
	remote, err := mockClientset.KubernetesOperators().Create(context.Background(), &ngrok.KubernetesOperatorCreate{})
	require.NoError(t, err)

	ko := &ngrokv1.KubernetesOperator{
		Name: "ko", Namespace: "test-ns",
		Status: ngrokv1.KubernetesOperatorStatus{ID: remote.ID},
	}
	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(ko, legacyTwin()).
		WithStatusSubresource(ko).
		Build()
	recorder := events.NewFakeRecorder(10)
	reconciler := &KubernetesOperatorReconciler{
		Client:         fakeClient,
		APIReader:      fakeClient,
		Recorder:       recorder,
		NgrokClientset: mockClientset,
		// DrainOrchestrator is nil: reaching the drain would panic.
	}

	require.NoError(t, reconciler.delete(context.Background(), ko))

	_, err = mockClientset.KubernetesOperators().Get(context.Background(), remote.ID)
	require.NoError(t, err, "the ngrok KubernetesOperator must not be deleted")
	assert.Nil(t, meta.FindStatusCondition(ko.Status.Conditions, ngrokv1.KubernetesOperatorConditionDraining))
	require.Len(t, recorder.Events, 1)
	assert.Contains(t, <-recorder.Events, ReasonLegacyTwinSkippedDrain)
}
