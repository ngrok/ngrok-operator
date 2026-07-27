package ngrok

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/go-logr/logr"
	"github.com/ngrok/ngrok-api-go/v7"
	ngrokv1alpha1 "github.com/ngrok/ngrok-operator/api/ngrok/v1alpha1"
	"github.com/ngrok/ngrok-operator/internal/controller"
	"github.com/ngrok/ngrok-operator/internal/mocks/nmockapi"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/events"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/event"
)

// TestCalculateFeaturesEnabled is a pure unit test for the calculateFeaturesEnabled function.
func TestCalculateFeaturesEnabled(t *testing.T) {
	tc := []struct {
		name     string
		in       *ngrokv1alpha1.KubernetesOperator
		expected []string
	}{
		{
			name: "no features enabled",
			in: &ngrokv1alpha1.KubernetesOperator{
				Spec: ngrokv1alpha1.KubernetesOperatorSpec{},
			},
			expected: []string{},
		},
		{
			name: "all features enabled",
			in: &ngrokv1alpha1.KubernetesOperator{
				Spec: ngrokv1alpha1.KubernetesOperatorSpec{
					EnabledFeatures: []string{
						ngrokv1alpha1.KubernetesOperatorFeatureBindings,
						ngrokv1alpha1.KubernetesOperatorFeatureIngress,
						ngrokv1alpha1.KubernetesOperatorFeatureGateway,
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
			wantRegReason:   ngrokv1alpha1.KubernetesOperatorReasonRegistered,
			wantReady:       metav1.ConditionTrue,
			wantReadyReason: ngrokv1alpha1.KubernetesOperatorReasonRegistered,
		},
		{
			name:            "pending",
			wantRegistered:  metav1.ConditionFalse,
			wantRegReason:   ngrokv1alpha1.KubernetesOperatorReasonPending,
			wantReady:       metav1.ConditionFalse,
			wantReadyReason: ngrokv1alpha1.KubernetesOperatorReasonPending,
		},
		{
			name:            "ngrok api error uses stable failure reason",
			err:             &ngrok.Error{ErrorCode: "ERR_NGROK_123", Msg: "something broke"},
			wantRegistered:  metav1.ConditionFalse,
			wantRegReason:   ngrokv1alpha1.KubernetesOperatorReasonRegistrationFailed,
			wantReady:       metav1.ConditionFalse,
			wantReadyReason: ngrokv1alpha1.KubernetesOperatorReasonRegistrationFailed,
		},
		{
			name:            "ngrok api error with empty message preserves fallback",
			err:             &ngrok.Error{ErrorCode: "ERR_NGROK_123"},
			wantRegistered:  metav1.ConditionFalse,
			wantRegReason:   ngrokv1alpha1.KubernetesOperatorReasonRegistrationFailed,
			wantReady:       metav1.ConditionFalse,
			wantReadyReason: ngrokv1alpha1.KubernetesOperatorReasonRegistrationFailed,
			wantMessage:     "HTTP 0",
		},
		{
			name:            "generic error",
			err:             errors.New("boom"),
			wantRegistered:  metav1.ConditionFalse,
			wantRegReason:   ngrokv1alpha1.KubernetesOperatorReasonRegistrationFailed,
			wantReady:       metav1.ConditionFalse,
			wantReadyReason: ngrokv1alpha1.KubernetesOperatorReasonRegistrationFailed,
		},
		{
			name:            "invalid configuration before registration",
			err:             errBindingsConfiguration,
			wantRegistered:  metav1.ConditionFalse,
			wantRegReason:   ngrokv1alpha1.KubernetesOperatorReasonRegistrationFailed,
			wantReady:       metav1.ConditionFalse,
			wantReadyReason: ngrokv1alpha1.KubernetesOperatorReasonConfigurationFailed,
		},
		{
			name:            "registered but later step failed",
			ngrokKo:         registeredKO,
			err:             errors.New("failed to update TLS secret"),
			wantRegistered:  metav1.ConditionTrue,
			wantRegReason:   ngrokv1alpha1.KubernetesOperatorReasonRegistered,
			wantReady:       metav1.ConditionFalse,
			wantReadyReason: ngrokv1alpha1.KubernetesOperatorReasonConfigurationFailed,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			scheme := runtime.NewScheme()
			require.NoError(t, ngrokv1alpha1.AddToScheme(scheme))

			ko := &ngrokv1alpha1.KubernetesOperator{
				ObjectMeta: metav1.ObjectMeta{
					Name:       "ko",
					Namespace:  "test-ns",
					Generation: 2,
				},
			}

			fakeClient := fake.NewClientBuilder().
				WithScheme(scheme).
				WithObjects(ko).
				WithStatusSubresource(ko).
				Build()

			reconciler := &KubernetesOperatorReconciler{
				Client: fakeClient,
				controller: &controller.BaseController[*ngrokv1alpha1.KubernetesOperator]{
					Kube:     fakeClient,
					Log:      logr.Discard(),
					Recorder: events.NewFakeRecorder(10),
				},
			}

			err := reconciler.updateStatus(context.Background(), ko, tt.ngrokKo, tt.err)
			assert.Equal(t, tt.err, err, "updateStatus passes the original error through")

			persisted := &ngrokv1alpha1.KubernetesOperator{}
			require.NoError(t, fakeClient.Get(context.Background(), client.ObjectKeyFromObject(ko), persisted))
			ko = persisted

			registered := meta.FindStatusCondition(ko.Status.Conditions, ngrokv1alpha1.KubernetesOperatorConditionRegistered)
			require.NotNil(t, registered)
			assert.Equal(t, tt.wantRegistered, registered.Status)
			assert.Equal(t, tt.wantRegReason, registered.Reason)
			assert.Equal(t, int64(2), registered.ObservedGeneration)

			ready := meta.FindStatusCondition(ko.Status.Conditions, ngrokv1alpha1.KubernetesOperatorConditionReady)
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
				assert.Equal(t, tt.ngrokKo.EnabledFeatures, []string(ko.Status.EnabledFeatures))
			}
		})
	}
}

func TestUpdate_ExistingOperatorConfigurationFailureRemainsRegistered(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, ngrokv1alpha1.AddToScheme(scheme))

	ko := &ngrokv1alpha1.KubernetesOperator{
		ObjectMeta: metav1.ObjectMeta{
			Name:       "ko",
			Namespace:  "test-ns",
			Generation: 2,
		},
		Spec: ngrokv1alpha1.KubernetesOperatorSpec{
			EnabledFeatures: []string{ngrokv1alpha1.KubernetesOperatorFeatureBindings},
		},
		Status: ngrokv1alpha1.KubernetesOperatorStatus{ID: "k8sop_123"},
	}
	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(ko).
		WithStatusSubresource(ko).
		Build()
	reconciler := &KubernetesOperatorReconciler{
		Client: fakeClient,
		controller: &controller.BaseController[*ngrokv1alpha1.KubernetesOperator]{
			Kube:     fakeClient,
			Log:      logr.Discard(),
			Recorder: events.NewFakeRecorder(10),
		},
	}

	err := reconciler._update(context.Background(), ko, &ngrok.KubernetesOperator{
		ID: "k8sop_123",
	})
	require.ErrorContains(t, err, "spec.binding is not configured")

	persisted := &ngrokv1alpha1.KubernetesOperator{}
	require.NoError(t, fakeClient.Get(context.Background(), client.ObjectKeyFromObject(ko), persisted))
	ko = persisted

	assert.True(t, meta.IsStatusConditionTrue(
		ko.Status.Conditions,
		ngrokv1alpha1.KubernetesOperatorConditionRegistered,
	))
	ready := meta.FindStatusCondition(ko.Status.Conditions, ngrokv1alpha1.KubernetesOperatorConditionReady)
	require.NotNil(t, ready)
	assert.Equal(t, metav1.ConditionFalse, ready.Status)
	assert.Equal(t, ngrokv1alpha1.KubernetesOperatorReasonConfigurationFailed, ready.Reason)
}

func TestKubernetesOperatorReconcilePredicate(t *testing.T) {
	now := metav1.Now()
	base := &ngrokv1alpha1.KubernetesOperator{
		ObjectMeta: metav1.ObjectMeta{
			Name:       "ko",
			Namespace:  "test-ns",
			Generation: 1,
		},
	}

	tests := []struct {
		name string
		old  *ngrokv1alpha1.KubernetesOperator
		new  *ngrokv1alpha1.KubernetesOperator
		want bool
	}{
		{
			name: "deletion starts",
			old:  base.DeepCopy(),
			new: func() *ngrokv1alpha1.KubernetesOperator {
				obj := base.DeepCopy()
				obj.DeletionTimestamp = &now
				return obj
			}(),
			want: true,
		},
		{
			name: "status-only update",
			old:  base.DeepCopy(),
			new: func() *ngrokv1alpha1.KubernetesOperator {
				obj := base.DeepCopy()
				obj.Status.ID = "k8sop_123"
				return obj
			}(),
			want: false,
		},
		{
			name: "status-only update while deleting",
			old: func() *ngrokv1alpha1.KubernetesOperator {
				obj := base.DeepCopy()
				obj.DeletionTimestamp = &now
				return obj
			}(),
			new: func() *ngrokv1alpha1.KubernetesOperator {
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
			new: func() *ngrokv1alpha1.KubernetesOperator {
				obj := base.DeepCopy()
				obj.Generation++
				return obj
			}(),
			want: true,
		},
		{
			name: "annotations change",
			old:  base.DeepCopy(),
			new: func() *ngrokv1alpha1.KubernetesOperator {
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

	secret := &v1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "tls-secret",
			Namespace: "test-ns",
		},
		Type: v1.SecretTypeTLS,
		Data: map[string][]byte{
			"tls.key": []byte("key"),
			"tls.crt": []byte("cert"),
			"tls.csr": []byte("csr"),
		},
	}

	reconciler := &KubernetesOperatorReconciler{
		Client: fake.NewClientBuilder().WithScheme(scheme).WithObjects(secret).Build(),
	}

	ko := &ngrokv1alpha1.KubernetesOperator{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "ko",
			Namespace: "test-ns",
		},
		Spec: ngrokv1alpha1.KubernetesOperatorSpec{
			Binding: &ngrokv1alpha1.KubernetesOperatorBinding{
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

	secret := &v1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "tls-secret",
			Namespace: "test-ns",
		},
		Type: v1.SecretTypeTLS,
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

	ko := &ngrokv1alpha1.KubernetesOperator{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "ko",
			Namespace: "test-ns",
		},
		Spec: ngrokv1alpha1.KubernetesOperatorSpec{
			EnabledFeatures: []string{ngrokv1alpha1.KubernetesOperatorFeatureBindings},
			Binding: &ngrokv1alpha1.KubernetesOperatorBinding{
				TlsSecretName: "tls-secret",
			},
		},
		Status: ngrokv1alpha1.KubernetesOperatorStatus{
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

	secret := &v1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "tls-secret",
			Namespace: "test-ns",
		},
		Type: v1.SecretTypeTLS,
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

	ko := &ngrokv1alpha1.KubernetesOperator{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "ko",
			Namespace: "test-ns",
		},
		Spec: ngrokv1alpha1.KubernetesOperatorSpec{
			EnabledFeatures: []string{ngrokv1alpha1.KubernetesOperatorFeatureBindings},
			Binding: &ngrokv1alpha1.KubernetesOperatorBinding{
				TlsSecretName: "tls-secret",
			},
		},
		Status: ngrokv1alpha1.KubernetesOperatorStatus{
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
