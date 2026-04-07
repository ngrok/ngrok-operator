package ngrok

import (
	"context"
	"fmt"
	"testing"

	ngrokv1alpha1 "github.com/ngrok/ngrok-operator/api/ngrok/v1alpha1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
