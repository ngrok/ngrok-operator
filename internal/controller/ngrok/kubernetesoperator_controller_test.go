package ngrok

import (
	"context"
	"fmt"
	"testing"

	ngrokv1alpha1 "github.com/ngrok/ngrok-operator/api/ngrok/v1alpha1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
)

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

func TestFindExisting_GetNamespaceUIDError(t *testing.T) {
	t.Parallel()

	scheme := runtime.NewScheme()
	require.NoError(t, ngrokv1alpha1.AddToScheme(scheme))

	injectedErr := fmt.Errorf("simulated RBAC error")

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithInterceptorFuncs(interceptor.Funcs{
			Get: func(ctx context.Context, c client.WithWatch, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
				return injectedErr
			},
		}).
		Build()

	r := &KubernetesOperatorReconciler{
		Client: fakeClient,
	}

	ko := &ngrokv1alpha1.KubernetesOperator{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-operator",
			Namespace: "test-ns",
		},
	}

	ctx := ctrl.LoggerInto(context.Background(), zap.New(zap.UseDevMode(true)))
	result, err := r.findExisting(ctx, ko)

	assert.Nil(t, result)
	assert.Error(t, err)
	assert.ErrorIs(t, err, injectedErr)
}

func TestFindOrCreateTLSSecret(t *testing.T) {
	const (
		namespace  = "test-ns"
		secretName = "test-tls-secret"
	)

	ko := &ngrokv1alpha1.KubernetesOperator{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-ko",
			Namespace: namespace,
		},
		Spec: ngrokv1alpha1.KubernetesOperatorSpec{
			Binding: &ngrokv1alpha1.KubernetesOperatorBinding{
				TlsSecretName: secretName,
			},
		},
	}

	scheme := runtime.NewScheme()
	require.NoError(t, v1.AddToScheme(scheme))

	tc := []struct {
		name              string
		existingSecret    *v1.Secret
		expectNewSecret   bool
		skipTypeAssertion bool
	}{
		{
			name:            "no existing secret creates a new one",
			existingSecret:  nil,
			expectNewSecret: true,
		},
		{
			name: "valid existing secret is returned as-is",
			existingSecret: &v1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      secretName,
					Namespace: namespace,
				},
				Type: v1.SecretTypeTLS,
				Data: map[string][]byte{
					"tls.key": []byte("existing-key"),
					"tls.csr": []byte("existing-csr"),
					"tls.crt": []byte("existing-crt"),
				},
			},
			expectNewSecret: false,
		},
		{
			name: "existing secret with missing tls.key is regenerated",
			existingSecret: &v1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      secretName,
					Namespace: namespace,
				},
				Type: v1.SecretTypeTLS,
				Data: map[string][]byte{
					"tls.csr": []byte("existing-csr"),
				},
			},
			expectNewSecret: true,
		},
		{
			name: "existing secret with missing tls.csr is regenerated",
			existingSecret: &v1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      secretName,
					Namespace: namespace,
				},
				Type: v1.SecretTypeTLS,
				Data: map[string][]byte{
					"tls.key": []byte("existing-key"),
				},
			},
			expectNewSecret: true,
		},
		{
			name: "existing secret with wrong type is regenerated",
			existingSecret: &v1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      secretName,
					Namespace: namespace,
				},
				Type: v1.SecretTypeOpaque,
				Data: map[string][]byte{
					"tls.key": []byte("existing-key"),
					"tls.csr": []byte("existing-csr"),
				},
			},
			expectNewSecret:   true,
			skipTypeAssertion: true, // Secret type is immutable; CreateOrUpdate can't change it
		},
	}

	for _, tt := range tc {
		t.Run(tt.name, func(t *testing.T) {
			builder := fake.NewClientBuilder().WithScheme(scheme)
			if tt.existingSecret != nil {
				builder = builder.WithObjects(tt.existingSecret)
			}
			fakeClient := builder.Build()

			r := &KubernetesOperatorReconciler{
				Client:         fakeClient,
				K8sOpNamespace: namespace,
			}

			secret, err := r.findOrCreateTLSSecret(context.Background(), ko)
			require.NoError(t, err)
			require.NotNil(t, secret)

			if !tt.skipTypeAssertion {
				assert.Equal(t, v1.SecretTypeTLS, secret.Type)
			}
			assert.NotEmpty(t, secret.Data["tls.key"])
			assert.NotEmpty(t, secret.Data["tls.csr"])

			if !tt.expectNewSecret {
				// Should be the original secret data
				assert.Equal(t, []byte("existing-key"), secret.Data["tls.key"])
				assert.Equal(t, []byte("existing-csr"), secret.Data["tls.csr"])
			} else {
				// Should have generated new crypto material
				assert.NotEqual(t, []byte("existing-key"), secret.Data["tls.key"])
			}
		})
	}
}
