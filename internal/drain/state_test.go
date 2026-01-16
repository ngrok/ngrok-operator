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
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	ngrokv1alpha1 "github.com/ngrok/ngrok-operator/api/ngrok/v1alpha1"
	"github.com/ngrok/ngrok-operator/internal/drainstate"
)

func TestNeverDraining(t *testing.T) {
	nd := drainstate.NeverDraining{}
	assert.False(t, nd.IsDraining(context.Background()))
}

func TestAlwaysDraining(t *testing.T) {
	ad := drainstate.AlwaysDraining{}
	assert.True(t, ad.IsDraining(context.Background()))
}

func TestStateChecker_NoDraining(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, ngrokv1alpha1.AddToScheme(scheme))

	ko := &ngrokv1alpha1.KubernetesOperator{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "my-release",
			Namespace: "ngrok-operator",
		},
		Spec: ngrokv1alpha1.KubernetesOperatorSpec{},
	}

	client := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(ko).
		Build()

	// StateChecker looks up KubernetesOperator by name (release name)
	checker := NewStateChecker(client, "ngrok-operator", "my-release")
	assert.False(t, checker.IsDraining(context.Background()))
}

func TestStateChecker_DrainModeEnabled(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, ngrokv1alpha1.AddToScheme(scheme))

	ko := &ngrokv1alpha1.KubernetesOperator{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "my-release",
			Namespace: "ngrok-operator",
		},
		Spec: ngrokv1alpha1.KubernetesOperatorSpec{
			Drain: &ngrokv1alpha1.DrainConfig{
				Enabled: true,
			},
		},
	}

	client := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(ko).
		Build()

	checker := NewStateChecker(client, "ngrok-operator", "my-release")
	assert.True(t, checker.IsDraining(context.Background()))
}

func TestStateChecker_DeletionTimestampSet(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, ngrokv1alpha1.AddToScheme(scheme))

	now := metav1.NewTime(time.Now())
	ko := &ngrokv1alpha1.KubernetesOperator{
		ObjectMeta: metav1.ObjectMeta{
			Name:              "my-release",
			Namespace:         "ngrok-operator",
			DeletionTimestamp: &now,
			Finalizers:        []string{"test-finalizer"},
		},
		Spec: ngrokv1alpha1.KubernetesOperatorSpec{},
	}

	client := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(ko).
		Build()

	checker := NewStateChecker(client, "ngrok-operator", "my-release")
	assert.True(t, checker.IsDraining(context.Background()))
}

func TestStateChecker_SetDraining(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, ngrokv1alpha1.AddToScheme(scheme))

	client := fake.NewClientBuilder().
		WithScheme(scheme).
		Build()

	// When CR doesn't exist, IsDraining returns false
	checker := NewStateChecker(client, "ngrok-operator", "my-release")
	assert.False(t, checker.IsDraining(context.Background()))

	// But SetDraining can override this
	checker.SetDraining(true)
	assert.True(t, checker.IsDraining(context.Background()))
}

func TestStateChecker_CachesDrainingState(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, ngrokv1alpha1.AddToScheme(scheme))

	ko := &ngrokv1alpha1.KubernetesOperator{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "my-release",
			Namespace: "ngrok-operator",
		},
		Spec: ngrokv1alpha1.KubernetesOperatorSpec{
			Drain: &ngrokv1alpha1.DrainConfig{
				Enabled: true,
			},
		},
	}

	client := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(ko).
		Build()

	checker := NewStateChecker(client, "ngrok-operator", "my-release")

	// Once detected as draining, should stay cached
	assert.True(t, checker.IsDraining(context.Background()))
	assert.True(t, checker.IsDraining(context.Background()))
}
