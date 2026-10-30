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
	"github.com/ngrok/ngrok-operator/internal/util"
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

func TestReconcileLegacy(t *testing.T) {
	legacyGK := schema.GroupKind{Group: "ngrok.k8s.ngrok.com", Kind: "KubernetesOperator"}
	superseded := []metav1.Condition{{
		Type:   ngrokv1alpha1.KubernetesOperatorConditionReady,
		Status: metav1.ConditionFalse,
		Reason: ReasonSupersededByV1,
	}}
	now := metav1.Now()

	tests := []struct {
		name          string
		legacy        *ngrokv1alpha1.KubernetesOperator
		canonicalID   string
		getErr        error
		wantGone      bool
		wantFinalizer bool
		wantReason    string
		wantRequeue   bool
	}{
		{
			name:          "live legacy keeps its finalizer and is marked superseded",
			legacy:        &ngrokv1alpha1.KubernetesOperator{Name: "ko", Namespace: "test-ns", Finalizers: []string{util.LegacyFinalizerName}},
			canonicalID:   "k8sop_123",
			wantFinalizer: true,
			wantReason:    ReasonSupersededByV1,
		},
		{
			name: "already superseded legacy is left alone",
			legacy: &ngrokv1alpha1.KubernetesOperator{
				Name: "ko", Namespace: "test-ns", Finalizers: []string{util.LegacyFinalizerName},
				Status: ngrokv1alpha1.KubernetesOperatorStatus{Conditions: superseded},
			},
			canonicalID:   "k8sop_123",
			wantFinalizer: true,
			wantReason:    ReasonSupersededByV1,
		},
		{
			name: "deleted legacy is released while the v1 twin holds the ID",
			legacy: &ngrokv1alpha1.KubernetesOperator{
				Name: "ko", Namespace: "test-ns", DeletionTimestamp: &now,
				Finalizers: []string{util.LegacyFinalizerName, util.FinalizerName},
			},
			canonicalID: "k8sop_123",
			wantGone:    true,
		},
		{
			name: "deleted legacy keeps its finalizer until the v1 twin holds the ID",
			legacy: &ngrokv1alpha1.KubernetesOperator{
				Name: "ko", Namespace: "test-ns", DeletionTimestamp: &now,
				Finalizers: []string{util.LegacyFinalizerName},
			},
			wantFinalizer: true,
			wantRequeue:   true,
		},
		{name: "no legacy object", canonicalID: "k8sop_123"},
		{name: "legacy kind not installed", canonicalID: "k8sop_123", getErr: &meta.NoKindMatchError{GroupKind: legacyGK}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			scheme := runtime.NewScheme()
			require.NoError(t, ngrokv1.AddToScheme(scheme))
			require.NoError(t, ngrokv1alpha1.AddToScheme(scheme))

			canonical := &ngrokv1.KubernetesOperator{
				Name: "ko", Namespace: "test-ns",
				Status: ngrokv1.KubernetesOperatorStatus{ID: tt.canonicalID},
			}
			builder := fake.NewClientBuilder().WithScheme(scheme).
				WithObjects(canonical).
				WithStatusSubresource(canonical, &ngrokv1alpha1.KubernetesOperator{})
			if tt.legacy != nil {
				builder = builder.WithObjects(tt.legacy)
			}
			if tt.getErr != nil {
				builder = builder.WithInterceptorFuncs(interceptor.Funcs{
					Get: func(ctx context.Context, c client.WithWatch, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
						if _, ok := obj.(*ngrokv1alpha1.KubernetesOperator); ok {
							return tt.getErr
						}
						return c.Get(ctx, key, obj, opts...)
					},
				})
			}
			fakeClient := builder.Build()
			recorder := events.NewFakeRecorder(10)
			// NgrokClientset is nil: any ngrok API call would panic.
			reconciler := &KubernetesOperatorReconciler{Client: fakeClient, Recorder: recorder}

			res, err := reconciler.reconcileLegacy(context.Background(), canonical)
			require.NoError(t, err)
			assert.Equal(t, tt.wantRequeue, res.RequeueAfter > 0)
			assert.Empty(t, recorder.Events, "the legacy object is passive; no events")
			if tt.legacy == nil {
				return
			}

			got := &ngrokv1alpha1.KubernetesOperator{}
			err = fakeClient.Get(context.Background(), client.ObjectKeyFromObject(tt.legacy), got)
			if tt.wantGone {
				assert.True(t, apierrors.IsNotFound(err), "legacy object should be released and gone, got %v", err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.wantFinalizer, util.HasFinalizer(got))
			if tt.wantReason != "" {
				ready := meta.FindStatusCondition(got.Status.Conditions, ngrokv1alpha1.KubernetesOperatorConditionReady)
				require.NotNil(t, ready)
				assert.Equal(t, tt.wantReason, ready.Reason)
				assert.Equal(t, metav1.ConditionFalse, ready.Status)
			}
		})
	}
}
