package ngrok

import (
	"testing"

	"github.com/ngrok/ngrok-api-go/v6"
	ngrokv1alpha1 "github.com/ngrok/ngrok-operator/api/ngrok/v1alpha1"
	"github.com/stretchr/testify/assert"
)

func Test_ngrokK8sopMatchesKubernetesOperator(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		ngrokK8sop *ngrok.KubernetesOperator
		koK8sop    *ngrokv1alpha1.KubernetesOperator
		want       bool
	}{
		{
			name:       "both nil",
			want:       false,
			koK8sop:    nil,
			ngrokK8sop: nil,
		},
		{
			name: "basic deployment",
			want: true,
			ngrokK8sop: &ngrok.KubernetesOperator{
				EnabledFeatures: []string{"Ingress"}, // API returns title cased features
				Deployment: ngrok.KubernetesOperatorDeployment{
					Name:      "example",
					Namespace: "ngrok-operator",
				},
			},
			koK8sop: &ngrokv1alpha1.KubernetesOperator{
				Spec: ngrokv1alpha1.KubernetesOperatorSpec{
					Deployment: &ngrokv1alpha1.KubernetesOperatorDeployment{
						Name:      "example",
						Namespace: "ngrok-operator",
					},
					EnabledFeatures: []string{ngrokv1alpha1.KubernetesOperatorFeatureIngress},
				},
			},
		},
		{
			name: "different namespace",
			want: false,
			ngrokK8sop: &ngrok.KubernetesOperator{
				EnabledFeatures: []string{"Ingress"}, // API returns title cased features
				Deployment: ngrok.KubernetesOperatorDeployment{
					Name:      "example",
					Namespace: "ngrok-operator",
				},
			},
			koK8sop: &ngrokv1alpha1.KubernetesOperator{
				Spec: ngrokv1alpha1.KubernetesOperatorSpec{
					Deployment: &ngrokv1alpha1.KubernetesOperatorDeployment{
						Name:      "example",
						Namespace: "different-namespace",
					},
					EnabledFeatures: []string{ngrokv1alpha1.KubernetesOperatorFeatureIngress},
				},
			},
		},
		{
			name: "different name",
			want: false,
			ngrokK8sop: &ngrok.KubernetesOperator{
				EnabledFeatures: []string{"Ingress"}, // API returns title cased features
				Deployment: ngrok.KubernetesOperatorDeployment{
					Name:      "example",
					Namespace: "ngrok-operator",
				},
			},
			koK8sop: &ngrokv1alpha1.KubernetesOperator{
				Spec: ngrokv1alpha1.KubernetesOperatorSpec{
					Deployment: &ngrokv1alpha1.KubernetesOperatorDeployment{
						Name:      "different-name",
						Namespace: "ngrok-operator",
					},
					EnabledFeatures: []string{ngrokv1alpha1.KubernetesOperatorFeatureIngress},
				},
			},
		},
		{
			name: "bindings: same features, same name",
			want: true,
			ngrokK8sop: &ngrok.KubernetesOperator{
				EnabledFeatures: []string{"Ingress", "Bindings"}, // API returns title cased features
				Deployment: ngrok.KubernetesOperatorDeployment{
					Name:      "example",
					Namespace: "ngrok-operator",
				},
				Binding: &ngrok.KubernetesOperatorBinding{
					Name: "example",
				},
			},
			koK8sop: &ngrokv1alpha1.KubernetesOperator{
				Spec: ngrokv1alpha1.KubernetesOperatorSpec{
					Deployment: &ngrokv1alpha1.KubernetesOperatorDeployment{
						Name:      "example",
						Namespace: "ngrok-operator",
					},
					EnabledFeatures: []string{ngrokv1alpha1.KubernetesOperatorFeatureIngress, ngrokv1alpha1.KubernetesOperatorFeatureBindings},
					Binding: &ngrokv1alpha1.KubernetesOperatorBinding{
						Name: "example",
					},
				},
			},
		},
		{
			name: "bindings: same features, different name",
			want: false,
			ngrokK8sop: &ngrok.KubernetesOperator{
				EnabledFeatures: []string{"Ingress", "Bindings"}, // API returns title cased features
				Deployment: ngrok.KubernetesOperatorDeployment{
					Name:      "example",
					Namespace: "ngrok-operator",
				},
				Binding: &ngrok.KubernetesOperatorBinding{
					Name: "example",
				},
			},
			koK8sop: &ngrokv1alpha1.KubernetesOperator{
				Spec: ngrokv1alpha1.KubernetesOperatorSpec{
					Deployment: &ngrokv1alpha1.KubernetesOperatorDeployment{
						Name:      "example",
						Namespace: "ngrok-operator",
					},
					EnabledFeatures: []string{ngrokv1alpha1.KubernetesOperatorFeatureIngress, ngrokv1alpha1.KubernetesOperatorFeatureBindings},
					Binding: &ngrokv1alpha1.KubernetesOperatorBinding{
						Name: "different-name",
					},
				},
			},
		},
		{
			name: "bindings: different features, enabled -> disabled",
			want: true,
			ngrokK8sop: &ngrok.KubernetesOperator{
				EnabledFeatures: []string{"Ingress", "Bindings"}, // API returns title cased features
				Deployment: ngrok.KubernetesOperatorDeployment{
					Name:      "example",
					Namespace: "ngrok-operator",
				},
				Binding: &ngrok.KubernetesOperatorBinding{
					Name: "example",
				},
			},
			koK8sop: &ngrokv1alpha1.KubernetesOperator{
				Spec: ngrokv1alpha1.KubernetesOperatorSpec{
					Deployment: &ngrokv1alpha1.KubernetesOperatorDeployment{
						Name:      "example",
						Namespace: "ngrok-operator",
					},
					EnabledFeatures: []string{ngrokv1alpha1.KubernetesOperatorFeatureIngress},
				},
			},
		},
		{
			name: "bindings: different features, disabled -> enabled (same name)",
			want: true,
			ngrokK8sop: &ngrok.KubernetesOperator{
				EnabledFeatures: []string{"Ingress"}, // API returns title cased features
				Deployment: ngrok.KubernetesOperatorDeployment{
					Name:      "example",
					Namespace: "ngrok-operator",
				},
				Binding: &ngrok.KubernetesOperatorBinding{
					Name: "example",
				},
			},
			koK8sop: &ngrokv1alpha1.KubernetesOperator{
				Spec: ngrokv1alpha1.KubernetesOperatorSpec{
					Deployment: &ngrokv1alpha1.KubernetesOperatorDeployment{
						Name:      "example",
						Namespace: "ngrok-operator",
					},
					EnabledFeatures: []string{ngrokv1alpha1.KubernetesOperatorFeatureIngress, ngrokv1alpha1.KubernetesOperatorFeatureBindings},
					Binding: &ngrokv1alpha1.KubernetesOperatorBinding{
						Name: "example",
					},
				},
			},
		},
		{
			name: "bindings: different features, disabled -> enabled (different name)",
			// here we've redeployed the ngrok-op with the same name, but we're enabling the bindings feature
			// even after it may have been enabled in the ngrok API previously
			// now the binding name may be different, but we still want to adopt this matching k8sop because
			// we're _enabling_ the feature and declaring the binding name
			want: true,
			ngrokK8sop: &ngrok.KubernetesOperator{
				EnabledFeatures: []string{"Ingress"}, // API returns title cased features
				Deployment: ngrok.KubernetesOperatorDeployment{
					Name:      "example",
					Namespace: "ngrok-operator",
				},
				Binding: &ngrok.KubernetesOperatorBinding{
					Name: "example",
				},
			},
			koK8sop: &ngrokv1alpha1.KubernetesOperator{
				Spec: ngrokv1alpha1.KubernetesOperatorSpec{
					Deployment: &ngrokv1alpha1.KubernetesOperatorDeployment{
						Name:      "example",
						Namespace: "ngrok-operator",
					},
					EnabledFeatures: []string{ngrokv1alpha1.KubernetesOperatorFeatureIngress, ngrokv1alpha1.KubernetesOperatorFeatureBindings},
					Binding: &ngrokv1alpha1.KubernetesOperatorBinding{
						Name: "different-name",
					},
				},
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			assert := assert.New(t)
			assert.Equal(test.want, ngrokK8sopMatchesKubernetesOperator(test.ngrokK8sop, test.koK8sop))
		})
	}
}
