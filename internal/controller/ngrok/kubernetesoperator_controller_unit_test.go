package ngrok

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/go-logr/logr"
	"github.com/ngrok/ngrok-api-go/v9"
	ngrokv1 "github.com/ngrok/ngrok-operator/api/ngrok/v1"
	ngrokv1alpha1 "github.com/ngrok/ngrok-operator/api/ngrok/v1alpha1"
	"github.com/ngrok/ngrok-operator/internal/controller"
	"github.com/ngrok/ngrok-operator/internal/mocks/nmockapi"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	v1 "k8s.io/api/core/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/tools/events"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"
	"sigs.k8s.io/controller-runtime/pkg/event"
)

// TestCalculateFeaturesEnabled is a pure unit test for the calculateFeaturesEnabled function.
func TestCalculateFeaturesEnabled(t *testing.T) {
	tc := []struct {
		name     string
		in       *ngrokv1.KubernetesOperator
		expected []string
	}{
		{
			name: "no features enabled",
			in: &ngrokv1.KubernetesOperator{
				Spec: ngrokv1.KubernetesOperatorSpec{},
			},
			expected: nil,
		},
		{
			name: "all features enabled",
			in: &ngrokv1.KubernetesOperator{
				Spec: ngrokv1.KubernetesOperatorSpec{
					EnabledFeatures: []string{
						ngrokv1.KubernetesOperatorFeatureBindings,
						ngrokv1.KubernetesOperatorFeatureIngress,
						ngrokv1.KubernetesOperatorFeatureGateway,
					},
				},
			},
			expected: []string{"bindings", "ingress", "gateway"},
		},
	}

	for _, tt := range tc {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tt.expected, calculateFeaturesEnabled(tt.in))
		})
	}
}

func TestUpdateStatus_Conditions(t *testing.T) {
	registeredKO := &ngrok.KubernetesOperator{
		ID:              "k8sop_123",
		URI:             "https://api.ngrok.com/kubernetes_operators/k8sop_123",
		EnabledFeatures: []string{"ingress", "bindings"},
	}

	tests := []struct {
		name            string
		ngrokKo         *ngrok.KubernetesOperator
		err             error
		wantRegistered  metav1.ConditionStatus
		wantRegReason   string
		wantReady       metav1.ConditionStatus
		wantReadyReason string
		wantMessage     string
	}{
		{
			name:            "registered",
			ngrokKo:         registeredKO,
			wantRegistered:  metav1.ConditionTrue,
			wantRegReason:   ngrokv1.KubernetesOperatorReasonRegistered,
			wantReady:       metav1.ConditionTrue,
			wantReadyReason: ngrokv1.KubernetesOperatorReasonRegistered,
		},
		{
			name:            "pending",
			wantRegistered:  metav1.ConditionFalse,
			wantRegReason:   ngrokv1.KubernetesOperatorReasonPending,
			wantReady:       metav1.ConditionFalse,
			wantReadyReason: ngrokv1.KubernetesOperatorReasonPending,
		},
		{
			name:            "ngrok api error uses stable failure reason",
			err:             &ngrok.Error{ErrorCode: "ERR_NGROK_123", Msg: "something broke"},
			wantRegistered:  metav1.ConditionFalse,
			wantRegReason:   ngrokv1.KubernetesOperatorReasonRegistrationFailed,
			wantReady:       metav1.ConditionFalse,
			wantReadyReason: ngrokv1.KubernetesOperatorReasonRegistrationFailed,
		},
		{
			name:            "ngrok api error with empty message preserves fallback",
			err:             &ngrok.Error{ErrorCode: "ERR_NGROK_123"},
			wantRegistered:  metav1.ConditionFalse,
			wantRegReason:   ngrokv1.KubernetesOperatorReasonRegistrationFailed,
			wantReady:       metav1.ConditionFalse,
			wantReadyReason: ngrokv1.KubernetesOperatorReasonRegistrationFailed,
			wantMessage:     "HTTP 0",
		},
		{
			name:            "generic error",
			err:             errors.New("boom"),
			wantRegistered:  metav1.ConditionFalse,
			wantRegReason:   ngrokv1.KubernetesOperatorReasonRegistrationFailed,
			wantReady:       metav1.ConditionFalse,
			wantReadyReason: ngrokv1.KubernetesOperatorReasonRegistrationFailed,
		},
		{
			name:            "invalid configuration before registration",
			err:             errBindingsConfiguration,
			wantRegistered:  metav1.ConditionFalse,
			wantRegReason:   ngrokv1.KubernetesOperatorReasonRegistrationFailed,
			wantReady:       metav1.ConditionFalse,
			wantReadyReason: ngrokv1.KubernetesOperatorReasonConfigurationFailed,
		},
		{
			name:            "registered but later step failed",
			ngrokKo:         registeredKO,
			err:             errors.New("failed to update TLS secret"),
			wantRegistered:  metav1.ConditionTrue,
			wantRegReason:   ngrokv1.KubernetesOperatorReasonRegistered,
			wantReady:       metav1.ConditionFalse,
			wantReadyReason: ngrokv1.KubernetesOperatorReasonConfigurationFailed,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			scheme := runtime.NewScheme()
			require.NoError(t, ngrokv1alpha1.AddToScheme(scheme))
			require.NoError(t, ngrokv1.AddToScheme(scheme))

			ko := &ngrokv1.KubernetesOperator{
				Name:       "ko",
				Namespace:  "test-ns",
				Generation: 2,
			}

			fakeClient := fake.NewClientBuilder().
				WithScheme(scheme).
				WithObjects(ko).
				WithStatusSubresource(ko).
				Build()

			reconciler := &KubernetesOperatorReconciler{
				Client: fakeClient,
				controller: &controller.BaseController[*ngrokv1.KubernetesOperator]{
					Kube:     fakeClient,
					Log:      logr.Discard(),
					Recorder: events.NewFakeRecorder(10),
				},
			}

			err := reconciler.updateStatus(context.Background(), ko, tt.ngrokKo, tt.err)
			assert.Equal(t, tt.err, err, "updateStatus passes the original error through")

			persisted := &ngrokv1.KubernetesOperator{}
			require.NoError(t, fakeClient.Get(context.Background(), client.ObjectKeyFromObject(ko), persisted))
			ko = persisted

			registered := meta.FindStatusCondition(ko.Status.Conditions, ngrokv1.KubernetesOperatorConditionRegistered)
			require.NotNil(t, registered)
			assert.Equal(t, tt.wantRegistered, registered.Status)
			assert.Equal(t, tt.wantRegReason, registered.Reason)
			assert.Equal(t, int64(2), registered.ObservedGeneration)

			ready := meta.FindStatusCondition(ko.Status.Conditions, ngrokv1.KubernetesOperatorConditionReady)
			require.NotNil(t, ready)
			assert.Equal(t, tt.wantReady, ready.Status)
			assert.Equal(t, tt.wantReadyReason, ready.Reason)
			assert.Equal(t, int64(2), ko.Status.ObservedGeneration)
			if ngrokErr, ok := tt.err.(*ngrok.Error); ok && ngrokErr.ErrorCode != "" {
				assert.Contains(t, ready.Message, ngrokErr.ErrorCode)
			}
			if tt.wantMessage != "" {
				assert.Contains(t, ready.Message, tt.wantMessage)
			}

			if tt.ngrokKo != nil {
				assert.Equal(t, tt.ngrokKo.ID, ko.Status.ID)
				assert.Equal(t, tt.ngrokKo.EnabledFeatures, ko.Status.EnabledFeatures)
			}
		})
	}
}

func TestUpdate_ExistingOperatorConfigurationFailureRemainsRegistered(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, ngrokv1alpha1.AddToScheme(scheme))
	require.NoError(t, ngrokv1.AddToScheme(scheme))

	ko := &ngrokv1.KubernetesOperator{
		Name:       "ko",
		Namespace:  "test-ns",
		Generation: 2,
		Spec: ngrokv1.KubernetesOperatorSpec{
			EnabledFeatures: []string{ngrokv1.KubernetesOperatorFeatureBindings},
		},
		Status: ngrokv1.KubernetesOperatorStatus{ID: "k8sop_123"},
	}
	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(ko).
		WithStatusSubresource(ko).
		Build()
	reconciler := &KubernetesOperatorReconciler{
		Client: fakeClient,
		controller: &controller.BaseController[*ngrokv1.KubernetesOperator]{
			Kube:     fakeClient,
			Log:      logr.Discard(),
			Recorder: events.NewFakeRecorder(10),
		},
	}

	err := reconciler._update(context.Background(), ko, &ngrok.KubernetesOperator{
		ID: "k8sop_123",
	})
	require.ErrorContains(t, err, "spec.binding is not configured")

	persisted := &ngrokv1.KubernetesOperator{}
	require.NoError(t, fakeClient.Get(context.Background(), client.ObjectKeyFromObject(ko), persisted))
	ko = persisted

	assert.True(t, meta.IsStatusConditionTrue(
		ko.Status.Conditions,
		ngrokv1.KubernetesOperatorConditionRegistered,
	))
	ready := meta.FindStatusCondition(ko.Status.Conditions, ngrokv1.KubernetesOperatorConditionReady)
	require.NotNil(t, ready)
	assert.Equal(t, metav1.ConditionFalse, ready.Status)
	assert.Equal(t, ngrokv1.KubernetesOperatorReasonConfigurationFailed, ready.Reason)
}

func TestKubernetesOperatorReconcilePredicate(t *testing.T) {
	now := metav1.Now()
	base := &ngrokv1.KubernetesOperator{
		Name:       "ko",
		Namespace:  "test-ns",
		Generation: 1,
	}

	tests := []struct {
		name string
		old  *ngrokv1.KubernetesOperator
		new  *ngrokv1.KubernetesOperator
		want bool
	}{
		{
			name: "deletion starts",
			old:  base.DeepCopy(),
			new: func() *ngrokv1.KubernetesOperator {
				obj := base.DeepCopy()
				obj.DeletionTimestamp = &now
				return obj
			}(),
			want: true,
		},
		{
			name: "status-only update",
			old:  base.DeepCopy(),
			new: func() *ngrokv1.KubernetesOperator {
				obj := base.DeepCopy()
				obj.Status.ID = "k8sop_123"
				return obj
			}(),
			want: false,
		},
		{
			name: "status-only update while deleting",
			old: func() *ngrokv1.KubernetesOperator {
				obj := base.DeepCopy()
				obj.DeletionTimestamp = &now
				return obj
			}(),
			new: func() *ngrokv1.KubernetesOperator {
				obj := base.DeepCopy()
				obj.DeletionTimestamp = &now
				obj.Status.ID = "k8sop_123"
				return obj
			}(),
			want: true,
		},
		{
			name: "generation changes",
			old:  base.DeepCopy(),
			new: func() *ngrokv1.KubernetesOperator {
				obj := base.DeepCopy()
				obj.Generation++
				return obj
			}(),
			want: true,
		},
		{
			name: "annotations change",
			old:  base.DeepCopy(),
			new: func() *ngrokv1.KubernetesOperator {
				obj := base.DeepCopy()
				obj.Annotations = map[string]string{"example": "changed"}
				return obj
			}(),
			want: true,
		},
	}

	p := kubernetesOperatorReconcilePredicate()
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, p.Update(event.UpdateEvent{
				ObjectOld: tt.old,
				ObjectNew: tt.new,
			}))
		})
	}
}

func TestBindingCertRenewalState(t *testing.T) {
	now := time.Date(2026, time.January, 10, 12, 0, 0, 0, time.UTC)
	window := 30 * 24 * time.Hour

	tests := []struct {
		name        string
		notAfter    string
		wantRenew   bool
		wantNotZero bool
		wantErr     bool
	}{
		{
			name:        "outside renewal window",
			notAfter:    now.Add(45 * 24 * time.Hour).Format(time.RFC3339),
			wantRenew:   false,
			wantNotZero: true,
		},
		{
			name:        "inside renewal window",
			notAfter:    now.Add(15 * 24 * time.Hour).Format(time.RFC3339),
			wantRenew:   true,
			wantNotZero: true,
		},
		{
			name:        "expired cert",
			notAfter:    now.Add(-time.Hour).Format(time.RFC3339),
			wantRenew:   true,
			wantNotZero: true,
		},
		{
			name:     "invalid not_after",
			notAfter: "not-a-time",
			wantErr:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ko := &ngrok.KubernetesOperator{
				Binding: &ngrok.KubernetesOperatorBinding{
					Cert: ngrok.KubernetesOperatorCert{
						NotAfter: tt.notAfter,
					},
				},
			}

			notAfter, renew, err := bindingCertRenewalState(ko, now, window)
			if tt.wantErr {
				assert.Error(t, err)
				return
			}

			assert.NoError(t, err)
			assert.Equal(t, tt.wantRenew, renew)
			assert.Equal(t, tt.wantNotZero, !notAfter.IsZero())
		})
	}
}

func TestInvalidateTLSSecretCSR(t *testing.T) {
	scheme := runtime.NewScheme()
	assert.NoError(t, v1.AddToScheme(scheme))
	assert.NoError(t, ngrokv1alpha1.AddToScheme(scheme))
	assert.NoError(t, ngrokv1.AddToScheme(scheme))

	secret := &v1.Secret{
		Name:      "tls-secret",
		Namespace: "test-ns",
		Type:      v1.SecretTypeTLS,
		Data: map[string][]byte{
			"tls.key": []byte("key"),
			"tls.crt": []byte("cert"),
			"tls.csr": []byte("csr"),
		},
	}

	reconciler := &KubernetesOperatorReconciler{
		Client: fake.NewClientBuilder().WithScheme(scheme).WithObjects(secret).Build(),
	}

	ko := &ngrokv1.KubernetesOperator{
		Name:      "ko",
		Namespace: "test-ns",
		Spec: ngrokv1.KubernetesOperatorSpec{
			Binding: &ngrokv1.KubernetesOperatorBinding{
				TlsSecretName: "tls-secret",
			},
		},
	}

	assert.NoError(t, reconciler.invalidateTLSSecretCSR(context.Background(), ko))

	updated := &v1.Secret{}
	assert.NoError(t, reconciler.Client.Get(context.Background(), client.ObjectKeyFromObject(secret), updated))
	assert.NotContains(t, updated.Data, "tls.csr")
	assert.Contains(t, updated.Data, "tls.key")
	assert.Contains(t, updated.Data, "tls.crt")
}

func TestReconcileBindingCertRenewalRequeueAfter(t *testing.T) {
	now := time.Date(2026, time.January, 10, 12, 0, 0, 0, time.UTC)
	renewalWindow := 30 * 24 * time.Hour
	notAfter := now.Add(45 * 24 * time.Hour)

	scheme := runtime.NewScheme()
	assert.NoError(t, v1.AddToScheme(scheme))
	assert.NoError(t, ngrokv1alpha1.AddToScheme(scheme))
	assert.NoError(t, ngrokv1.AddToScheme(scheme))

	secret := &v1.Secret{
		Name:      "tls-secret",
		Namespace: "test-ns",
		Type:      v1.SecretTypeTLS,
		Data: map[string][]byte{
			"tls.key": []byte("key"),
			"tls.crt": []byte("cert"),
			"tls.csr": []byte("csr"),
		},
	}

	mockClientset := nmockapi.NewClientset()
	ngrokKO, err := mockClientset.KubernetesOperators().Create(context.Background(), &ngrok.KubernetesOperatorCreate{
		Description: "test",
		Binding: &ngrok.KubernetesOperatorBindingCreate{
			EndpointSelectors: []string{"all()"},
			CSR:               "csr",
		},
	})
	assert.NoError(t, err)
	ngrokKO.Binding.Cert.NotAfter = notAfter.Format(time.RFC3339)

	reconciler := &KubernetesOperatorReconciler{
		Client:                   fake.NewClientBuilder().WithScheme(scheme).WithObjects(secret).Build(),
		NgrokClientset:           mockClientset,
		BindingCertRenewalWindow: renewalWindow,
	}

	ko := &ngrokv1.KubernetesOperator{
		Name:      "ko",
		Namespace: "test-ns",
		Spec: ngrokv1.KubernetesOperatorSpec{
			EnabledFeatures: []string{ngrokv1.KubernetesOperatorFeatureBindings},
			Binding: &ngrokv1.KubernetesOperatorBinding{
				TlsSecretName: "tls-secret",
			},
		},
		Status: ngrokv1.KubernetesOperatorStatus{
			ID: ngrokKO.ID,
		},
	}

	res, err := reconciler.reconcileBindingCertRenewal(context.Background(), ko, now)
	assert.NoError(t, err)
	assert.Equal(t, 15*24*time.Hour, res.RequeueAfter)
}

func TestReconcileBindingCertRenewalInvalidatesCSR(t *testing.T) {
	now := time.Date(2026, time.January, 10, 12, 0, 0, 0, time.UTC)
	renewalWindow := 30 * 24 * time.Hour
	notAfter := now.Add(10 * 24 * time.Hour)

	scheme := runtime.NewScheme()
	assert.NoError(t, v1.AddToScheme(scheme))
	assert.NoError(t, ngrokv1alpha1.AddToScheme(scheme))
	assert.NoError(t, ngrokv1.AddToScheme(scheme))

	secret := &v1.Secret{
		Name:      "tls-secret",
		Namespace: "test-ns",
		Type:      v1.SecretTypeTLS,
		Data: map[string][]byte{
			"tls.key": []byte("key"),
			"tls.crt": []byte("cert"),
			"tls.csr": []byte("csr"),
		},
	}

	mockClientset := nmockapi.NewClientset()
	ngrokKO, err := mockClientset.KubernetesOperators().Create(context.Background(), &ngrok.KubernetesOperatorCreate{
		Description: "test",
		Binding: &ngrok.KubernetesOperatorBindingCreate{
			EndpointSelectors: []string{"all()"},
			CSR:               "csr",
		},
	})
	assert.NoError(t, err)
	ngrokKO.Binding.Cert.NotAfter = notAfter.Format(time.RFC3339)

	fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(secret).Build()
	reconciler := &KubernetesOperatorReconciler{
		Client:                   fakeClient,
		NgrokClientset:           mockClientset,
		BindingCertRenewalWindow: renewalWindow,
	}

	ko := &ngrokv1.KubernetesOperator{
		Name:      "ko",
		Namespace: "test-ns",
		Spec: ngrokv1.KubernetesOperatorSpec{
			EnabledFeatures: []string{ngrokv1.KubernetesOperatorFeatureBindings},
			Binding: &ngrokv1.KubernetesOperatorBinding{
				TlsSecretName: "tls-secret",
			},
		},
		Status: ngrokv1.KubernetesOperatorStatus{
			ID: ngrokKO.ID,
		},
	}

	res, err := reconciler.reconcileBindingCertRenewal(context.Background(), ko, now)
	assert.NoError(t, err)
	assert.Equal(t, time.Second, res.RequeueAfter)

	updated := &v1.Secret{}
	assert.NoError(t, fakeClient.Get(context.Background(), client.ObjectKey{Namespace: "test-ns", Name: "tls-secret"}, updated))
	assert.NotContains(t, updated.Data, "tls.csr")
}

func TestMetadataForAPI(t *testing.T) {
	tests := []struct {
		name     string
		metadata map[string]string
		expected string
	}{
		{name: "nil", metadata: nil, expected: ""},
		{name: "empty", metadata: map[string]string{}, expected: ""},
		{name: "default", metadata: map[string]string{"owned-by": "ngrok-operator"}, expected: `{"owned-by":"ngrok-operator"}`},
		{name: "sorted keys", metadata: map[string]string{"b": "2", "a": "1"}, expected: `{"a":"1","b":"2"}`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, metadataForAPI(tt.metadata))
		})
	}
}

func TestMetadataForAPIMergesNamespaceUID(t *testing.T) {
	merged, err := mergeMetadata(metadataForAPI(map[string]string{"owned-by": "ngrok-operator"}), "ns-uid")
	require.NoError(t, err)
	assert.JSONEq(t, `{"owned-by":"ngrok-operator","namespace.uid":"ns-uid"}`, merged)
}

func kubernetesOperatorCRD(deleting bool) *apiextensionsv1.CustomResourceDefinition {
	crd := &apiextensionsv1.CustomResourceDefinition{
		Name: "kubernetesoperators.ngrok.com",
	}
	if deleting {
		now := metav1.Now()
		crd.DeletionTimestamp = &now
		crd.Finalizers = []string{"customresourcecleanup.apiextensions.k8s.io"}
	}
	return crd
}

func TestShouldSkipDrain(t *testing.T) {
	forbidden := apierrors.NewForbidden(schema.GroupResource{Group: "apiextensions.k8s.io", Resource: "customresourcedefinitions"}, "kubernetesoperators.ngrok.com", errors.New("rbac"))
	draining := []metav1.Condition{{
		Type:   ngrokv1.KubernetesOperatorConditionDraining,
		Status: metav1.ConditionTrue,
		Reason: ngrokv1.KubernetesOperatorReasonDrainInProgress,
	}}

	tests := []struct {
		name        string
		crds        []client.Object
		getErr      error
		annotations map[string]string
		conditions  []metav1.Condition
		wantSkip    bool
		wantErr     bool
	}{
		{name: "crd present", crds: []client.Object{kubernetesOperatorCRD(false)}, wantSkip: false},
		{name: "crd deleting", crds: []client.Object{kubernetesOperatorCRD(true)}, wantSkip: true},
		{name: "crd not found", wantSkip: true},
		{name: "forbidden without uninstall request skips", getErr: forbidden, wantSkip: true},
		{name: "forbidden with uninstall request drains", getErr: forbidden, annotations: map[string]string{AnnotationDrainRequested: "true"}, wantSkip: false},
		{name: "drain already in progress continues even if crd is deleting", crds: []client.Object{kubernetesOperatorCRD(true)}, conditions: draining, wantSkip: false},
		{name: "other error", getErr: errors.New("boom"), wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			scheme := runtime.NewScheme()
			require.NoError(t, apiextensionsv1.AddToScheme(scheme))
			builder := fake.NewClientBuilder().WithScheme(scheme).WithObjects(tt.crds...)
			if tt.getErr != nil {
				builder = builder.WithInterceptorFuncs(interceptor.Funcs{
					Get: func(context.Context, client.WithWatch, client.ObjectKey, client.Object, ...client.GetOption) error {
						return tt.getErr
					},
				})
			}
			reconciler := &KubernetesOperatorReconciler{APIReader: builder.Build(), Log: logr.Discard()}
			ko := &ngrokv1.KubernetesOperator{
				Name:        "ko",
				Namespace:   "test-ns",
				Annotations: tt.annotations,
				Status:      ngrokv1.KubernetesOperatorStatus{Conditions: tt.conditions},
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

func TestDeleteSkipsDrainWhenCRDDeleting(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, ngrokv1.AddToScheme(scheme))
	require.NoError(t, apiextensionsv1.AddToScheme(scheme))

	mockClientset := nmockapi.NewClientset()
	remote, err := mockClientset.KubernetesOperators().Create(context.Background(), &ngrok.KubernetesOperatorCreate{})
	require.NoError(t, err)

	ko := &ngrokv1.KubernetesOperator{
		Name: "ko", Namespace: "test-ns",
		Status: ngrokv1.KubernetesOperatorStatus{ID: remote.ID},
	}
	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(ko, kubernetesOperatorCRD(true)).
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
	assert.Contains(t, <-recorder.Events, ReasonCRDDeletionSkippedDrain)
}
