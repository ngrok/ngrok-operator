/*
MIT License

Copyright (c) 2025 ngrok, Inc.

Permission is hereby granted, free of charge, to any person obtaining a copy
of this software and associated documentation files (the "Software"), to deal
in the Software without restriction, including without limitation the rights
to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
copies of the Software, and to permit persons to whom the Software is
furnished to do so, subject to the following conditions:

The above copyright notice and this permission notice shall be included in all
copies or substantial portions of the Software.

THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
SOFTWARE.
*/
package drain

import (
	"context"
	"testing"

	"github.com/go-logr/logr"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	netv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	ingressv1alpha1 "github.com/ngrok/ngrok-operator/api/ingress/v1alpha1"
	ngrokv1alpha1 "github.com/ngrok/ngrok-operator/api/ngrok/v1alpha1"
	"github.com/ngrok/ngrok-operator/internal/controller"
	"github.com/ngrok/ngrok-operator/internal/controller/labels"
)

func TestDrainResult(t *testing.T) {
	t.Run("Progress", func(t *testing.T) {
		r := &DrainResult{Completed: 5, Total: 10}
		assert.Equal(t, "5/10", r.Progress())
	})

	t.Run("IsComplete", func(t *testing.T) {
		tests := []struct {
			name       string
			result     DrainResult
			isComplete bool
		}{
			{"all completed", DrainResult{Completed: 10, Total: 10, Failed: 0}, true},
			{"some completed", DrainResult{Completed: 5, Total: 10, Failed: 0}, false},
			{"all failed", DrainResult{Completed: 0, Total: 10, Failed: 10}, true},
			{"mixed completed and failed", DrainResult{Completed: 5, Total: 10, Failed: 5}, true},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				assert.Equal(t, tt.isComplete, tt.result.IsComplete())
			})
		}
	})

	t.Run("HasErrors", func(t *testing.T) {
		tests := []struct {
			name      string
			result    DrainResult
			hasErrors bool
		}{
			{"no errors", DrainResult{Errors: nil}, false},
			{"empty errors", DrainResult{Errors: []error{}}, false},
			{"has errors", DrainResult{Errors: []error{assert.AnError}}, true},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				assert.Equal(t, tt.hasErrors, tt.result.HasErrors())
			})
		}
	})
}

func TestDrainer_DrainUserResource(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, netv1.AddToScheme(scheme))

	ingress := &netv1.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Name:       "test-ingress",
			Namespace:  "default",
			Finalizers: []string{controller.FinalizerName},
			Labels: map[string]string{
				labels.ControllerNamespace: "ngrok-operator",
				labels.ControllerName:      "ngrok-operator",
			},
		},
	}

	client := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(ingress).
		Build()

	drainer := &Drainer{
		Client:              client,
		Log:                 logr.Discard(),
		ControllerNamespace: "ngrok-operator",
		ControllerName:      "ngrok-operator",
	}

	err := drainer.drainUserResource(context.Background(), ingress)
	require.NoError(t, err)

	assert.False(t, controller.HasFinalizer(ingress), "finalizer should be removed")
}

func TestDrainer_DrainOperatorResource(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, ingressv1alpha1.AddToScheme(scheme))
	require.NoError(t, ngrokv1alpha1.AddToScheme(scheme))

	domain := &ingressv1alpha1.Domain{
		ObjectMeta: metav1.ObjectMeta{
			Name:       "test-domain",
			Namespace:  "ngrok-operator",
			Finalizers: []string{controller.FinalizerName},
			Labels: map[string]string{
				labels.ControllerNamespace: "ngrok-operator",
				labels.ControllerName:      "ngrok-operator",
			},
		},
		Spec: ingressv1alpha1.DomainSpec{
			Domain: "test.ngrok.io",
		},
	}

	client := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(domain).
		Build()

	drainer := &Drainer{
		Client:              client,
		Log:                 logr.Discard(),
		ControllerNamespace: "ngrok-operator",
		ControllerName:      "ngrok-operator",
	}

	err := drainer.drainOperatorResource(context.Background(), domain)
	require.NoError(t, err)
}

func TestDrainer_DrainAll_EmptyCluster(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, netv1.AddToScheme(scheme))
	require.NoError(t, ingressv1alpha1.AddToScheme(scheme))
	require.NoError(t, ngrokv1alpha1.AddToScheme(scheme))

	client := fake.NewClientBuilder().
		WithScheme(scheme).
		Build()

	drainer := &Drainer{
		Client:              client,
		Log:                 logr.Discard(),
		ControllerNamespace: "ngrok-operator",
		ControllerName:      "ngrok-operator",
	}

	result, err := drainer.DrainAll(context.Background())
	require.NoError(t, err)
	assert.Equal(t, 0, result.Completed)
	assert.True(t, result.IsComplete())
}
