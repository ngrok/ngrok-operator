package ngrok

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/go-logr/logr"
	"github.com/ngrok/ngrok-api-go/v7"
	bindingsv1alpha1 "github.com/ngrok/ngrok-operator/api/bindings/v1alpha1"
	ingressv1alpha1 "github.com/ngrok/ngrok-operator/api/ingress/v1alpha1"
	ngrokv1alpha1 "github.com/ngrok/ngrok-operator/api/ngrok/v1alpha1"
	"github.com/ngrok/ngrok-operator/internal/controller"
	"github.com/ngrok/ngrok-operator/internal/drain"
	"github.com/ngrok/ngrok-operator/internal/mocks/nmockapi"
	"github.com/ngrok/ngrok-operator/internal/testutils"
	"github.com/ngrok/ngrok-operator/internal/util"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	v1 "k8s.io/api/core/v1"
	netv1 "k8s.io/api/networking/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/events"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"
	gatewayv1alpha2 "sigs.k8s.io/gateway-api/apis/v1alpha2"
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
			name:            "ngrok api error uses error code as reason",
			err:             &ngrok.Error{ErrorCode: "ERR_NGROK_123", Msg: "something broke"},
			wantRegistered:  metav1.ConditionFalse,
			wantRegReason:   "ERR_NGROK_123",
			wantReady:       metav1.ConditionFalse,
			wantReadyReason: "ERR_NGROK_123",
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
			name:            "registered but later step failed",
			ngrokKo:         registeredKO,
			err:             errors.New("failed to update TLS secret"),
			wantRegistered:  metav1.ConditionTrue,
			wantRegReason:   ngrokv1alpha1.KubernetesOperatorReasonRegistered,
			wantReady:       metav1.ConditionFalse,
			wantReadyReason: ngrokv1alpha1.KubernetesOperatorReasonRegistrationFailed,
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

			registered := meta.FindStatusCondition(ko.Status.Conditions, ngrokv1alpha1.KubernetesOperatorConditionRegistered)
			require.NotNil(t, registered)
			assert.Equal(t, tt.wantRegistered, registered.Status)
			assert.Equal(t, tt.wantRegReason, registered.Reason)
			assert.Equal(t, int64(2), registered.ObservedGeneration)

			ready := meta.FindStatusCondition(ko.Status.Conditions, ngrokv1alpha1.KubernetesOperatorConditionReady)
			require.NotNil(t, ready)
			assert.Equal(t, tt.wantReady, ready.Status)
			assert.Equal(t, tt.wantReadyReason, ready.Reason)

			if tt.ngrokKo != nil {
				assert.Equal(t, tt.ngrokKo.ID, ko.Status.ID)
				assert.Equal(t, tt.ngrokKo.EnabledFeatures, ko.Status.EnabledFeatures)
			}
		})
	}
}

// TestDelete_DrainLifecycle runs the delete flow end-to-end on an empty cluster:
// drain is initiated (Draining=True), completes (Draining=False/DrainCompleted),
// and the KubernetesOperator is removed from the ngrok API.
func TestDelete_DrainLifecycle(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, ngrokv1alpha1.AddToScheme(scheme))
	require.NoError(t, ingressv1alpha1.AddToScheme(scheme))
	require.NoError(t, bindingsv1alpha1.AddToScheme(scheme))
	require.NoError(t, v1.AddToScheme(scheme))
	require.NoError(t, netv1.AddToScheme(scheme))
	require.NoError(t, gatewayv1.Install(scheme))
	require.NoError(t, gatewayv1alpha2.Install(scheme))

	mockClientset := nmockapi.NewClientset()
	ngrokKO, err := mockClientset.KubernetesOperators().Create(context.Background(), &ngrok.KubernetesOperatorCreate{
		Description: "test",
	})
	require.NoError(t, err)

	ko := &ngrokv1alpha1.KubernetesOperator{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "my-release",
			Namespace: "test-ns",
		},
		Status: ngrokv1alpha1.KubernetesOperatorStatus{
			ID: ngrokKO.ID,
		},
	}

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(ko).
		WithStatusSubresource(ko).
		Build()

	reconciler := &KubernetesOperatorReconciler{
		Client:         fakeClient,
		NgrokClientset: mockClientset,
		DrainOrchestrator: drain.NewOrchestrator(drain.OrchestratorConfig{
			Client:         fakeClient,
			Recorder:       events.NewFakeRecorder(10),
			Log:            logr.Discard(),
			K8sOpNamespace: "test-ns",
			K8sOpName:      "my-release",
		}),
	}

	require.NoError(t, reconciler.delete(context.Background(), ko))

	draining := meta.FindStatusCondition(ko.Status.Conditions, ngrokv1alpha1.KubernetesOperatorConditionDraining)
	require.NotNil(t, draining)
	assert.Equal(t, metav1.ConditionFalse, draining.Status)
	assert.Equal(t, ngrokv1alpha1.KubernetesOperatorReasonDrainCompleted, draining.Reason)
	assert.True(t, ko.IsDrainComplete())

	ready := meta.FindStatusCondition(ko.Status.Conditions, ngrokv1alpha1.KubernetesOperatorConditionReady)
	require.NotNil(t, ready)
	assert.Equal(t, metav1.ConditionFalse, ready.Status)

	assert.Empty(t, ko.Status.ID, "ID is cleared after deletion from the ngrok API")
	_, err = mockClientset.KubernetesOperators().Get(context.Background(), ngrokKO.ID)
	assert.True(t, ngrok.IsNotFound(err), "KubernetesOperator is deleted from the ngrok API")
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

var _ = Describe("KubernetesOperator Controller", Ordered, func() {
	const (
		timeout  = 15 * time.Second
		interval = 500 * time.Millisecond
	)

	var kginkgo *testutils.KGinkgo

	BeforeAll(func() {
		kginkgo = testutils.NewKGinkgo(k8sClient)

		kginkgo.ExpectCreateNamespace(context.Background(), controllerNamespace)
	})

	// forceDeleteKO removes the finalizer and deletes the KubernetesOperator to
	// avoid triggering the drain workflow during test cleanup.
	forceDeleteKO := func(ctx context.Context) {
		ko := &ngrokv1alpha1.KubernetesOperator{}
		err := k8sClient.Get(ctx, client.ObjectKey{
			Namespace: controllerNamespace,
			Name:      k8sOpName,
		}, ko)
		if apierrors.IsNotFound(err) {
			return
		}
		Expect(err).NotTo(HaveOccurred())

		if util.RemoveFinalizer(ko) {
			Expect(k8sClient.Update(ctx, ko)).To(Succeed())
		}
		Expect(client.IgnoreNotFound(k8sClient.Delete(ctx, ko))).To(Succeed())

		// Wait for it to actually be gone
		Eventually(func() bool {
			err := k8sClient.Get(ctx, client.ObjectKey{
				Namespace: controllerNamespace,
				Name:      k8sOpName,
			}, &ngrokv1alpha1.KubernetesOperator{})
			return apierrors.IsNotFound(err)
		}).WithTimeout(timeout).WithPolling(interval).Should(BeTrue())
	}

	AfterEach(func() {
		mockClientset.KubernetesOperators().(*nmockapi.KubernetesOperatorsClient).Reset()
		forceDeleteKO(context.Background())
	})

	It("should register successfully with ingress feature enabled", func(ctx SpecContext) {
		ko := &ngrokv1alpha1.KubernetesOperator{
			ObjectMeta: metav1.ObjectMeta{
				Name:      k8sOpName,
				Namespace: controllerNamespace,
			},
			Spec: ngrokv1alpha1.KubernetesOperatorSpec{
				Description:     "test operator",
				Metadata:        `{"owned-by":"test"}`,
				EnabledFeatures: []string{ngrokv1alpha1.KubernetesOperatorFeatureIngress},
				Region:          "global",
			},
		}

		By("Creating the KubernetesOperator")
		Expect(k8sClient.Create(ctx, ko)).To(Succeed())

		By("Expecting the finalizer to be added")
		// R1: AddFinalizer writes the legacy key. Flip to util.FinalizerName in R2.
		kginkgo.ExpectFinalizerToBeAdded(ctx, ko, util.LegacyFinalizerName, testutils.WithTimeout(timeout))

		By("Expecting registration to succeed")
		kginkgo.EventuallyWithObject(ctx, ko.DeepCopy(), func(g Gomega, fetched client.Object) {
			koFetched := fetched.(*ngrokv1alpha1.KubernetesOperator)
			g.Expect(koFetched.Status.ID).NotTo(BeEmpty())
			g.Expect(meta.IsStatusConditionTrue(koFetched.Status.Conditions, ngrokv1alpha1.KubernetesOperatorConditionRegistered)).To(BeTrue())
			g.Expect(meta.IsStatusConditionTrue(koFetched.Status.Conditions, ngrokv1alpha1.KubernetesOperatorConditionReady)).To(BeTrue())
		}, testutils.WithTimeout(timeout))
	})

	It("should not panic with bindings enabled but nil Binding spec", func(ctx SpecContext) {
		ko := &ngrokv1alpha1.KubernetesOperator{
			ObjectMeta: metav1.ObjectMeta{
				Name:      k8sOpName,
				Namespace: controllerNamespace,
			},
			Spec: ngrokv1alpha1.KubernetesOperatorSpec{
				Description:     "test operator with nil binding",
				Metadata:        `{"owned-by":"test"}`,
				EnabledFeatures: []string{ngrokv1alpha1.KubernetesOperatorFeatureBindings},
				Binding:         nil,
				Region:          "global",
			},
		}

		By("Creating the KubernetesOperator")
		Expect(k8sClient.Create(ctx, ko)).To(Succeed())

		By("Expecting the finalizer to be added (controller ran without panic)")
		kginkgo.ExpectFinalizerToBeAdded(ctx, ko, util.LegacyFinalizerName, testutils.WithTimeout(timeout))

		By("Expecting the status to reflect a pending state (not registered)")
		kginkgo.EventuallyWithObject(ctx, ko.DeepCopy(), func(g Gomega, fetched client.Object) {
			koFetched := fetched.(*ngrokv1alpha1.KubernetesOperator)
			// The nil binding guard prevents a panic and returns an error,
			// which updateStatus records. The ID stays empty because no
			// ngrok API object was created.
			g.Expect(koFetched.Status.ID).To(BeEmpty())
		}, testutils.WithTimeout(timeout))
	})

	It("should not panic with nil Deployment and should register successfully", func(ctx SpecContext) {
		ko := &ngrokv1alpha1.KubernetesOperator{
			ObjectMeta: metav1.ObjectMeta{
				Name:      k8sOpName,
				Namespace: controllerNamespace,
			},
			Spec: ngrokv1alpha1.KubernetesOperatorSpec{
				Description:     "test operator with nil deployment",
				Metadata:        `{"owned-by":"test"}`,
				EnabledFeatures: []string{ngrokv1alpha1.KubernetesOperatorFeatureIngress},
				Deployment:      nil,
				Region:          "global",
			},
		}

		By("Creating the KubernetesOperator")
		Expect(k8sClient.Create(ctx, ko)).To(Succeed())

		By("Expecting the finalizer to be added")
		kginkgo.ExpectFinalizerToBeAdded(ctx, ko, util.LegacyFinalizerName, testutils.WithTimeout(timeout))

		By("Expecting registration to succeed even with nil Deployment")
		kginkgo.EventuallyWithObject(ctx, ko.DeepCopy(), func(g Gomega, fetched client.Object) {
			koFetched := fetched.(*ngrokv1alpha1.KubernetesOperator)
			g.Expect(koFetched.Status.ID).NotTo(BeEmpty())
			g.Expect(meta.IsStatusConditionTrue(koFetched.Status.Conditions, ngrokv1alpha1.KubernetesOperatorConditionRegistered)).To(BeTrue())
		}, testutils.WithTimeout(timeout))
	})
})
