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
	bindingsv1alpha1 "github.com/ngrok/ngrok-operator/api/bindings/v1alpha1"
	ingressv1alpha1 "github.com/ngrok/ngrok-operator/api/ingress/v1alpha1"
	ngrokv1alpha1 "github.com/ngrok/ngrok-operator/api/ngrok/v1alpha1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	netv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"
	gatewayv1alpha2 "sigs.k8s.io/gateway-api/apis/v1alpha2"
)

func setupTestScheme(t *testing.T) *runtime.Scheme {
	scheme := runtime.NewScheme()
	require.NoError(t, ngrokv1alpha1.AddToScheme(scheme))
	require.NoError(t, ingressv1alpha1.AddToScheme(scheme))
	require.NoError(t, bindingsv1alpha1.AddToScheme(scheme))
	require.NoError(t, corev1.AddToScheme(scheme))
	require.NoError(t, netv1.AddToScheme(scheme))
	require.NoError(t, gatewayv1.Install(scheme))
	require.NoError(t, gatewayv1alpha2.Install(scheme))
	return scheme
}

func TestOrchestrator_State(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, ngrokv1alpha1.AddToScheme(scheme))

	client := fake.NewClientBuilder().
		WithScheme(scheme).
		Build()

	orchestrator := NewOrchestrator(OrchestratorConfig{
		Client:         client,
		Recorder:       record.NewFakeRecorder(10),
		Log:            logr.Discard(),
		K8sOpNamespace: "ngrok-operator",
		K8sOpName:      "my-release",
	})

	// State() should return a non-nil drainstate.State
	state := orchestrator.State()
	require.NotNil(t, state)

	// Initially not draining
	assert.False(t, state.IsDraining(context.Background()))
}

func TestOrchestrator_HandleDrain_CompletesSuccessfully(t *testing.T) {
	scheme := setupTestScheme(t)

	ko := &ngrokv1alpha1.KubernetesOperator{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "my-release",
			Namespace: "ngrok-operator",
		},
		Spec: ngrokv1alpha1.KubernetesOperatorSpec{
			Drain: &ngrokv1alpha1.DrainConfig{
				Policy: ngrokv1alpha1.DrainPolicyRetain,
			},
		},
	}

	client := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(ko).
		WithStatusSubresource(ko).
		Build()

	recorder := record.NewFakeRecorder(10)
	orchestrator := NewOrchestrator(OrchestratorConfig{
		Client:         client,
		Recorder:       recorder,
		Log:            logr.Discard(),
		K8sOpNamespace: "ngrok-operator",
		K8sOpName:      "my-release",
	})

	ctx := context.Background()

	// HandleDrain should complete successfully with empty cluster
	outcome, err := orchestrator.HandleDrain(ctx, ko)
	require.NoError(t, err)
	assert.Equal(t, OutcomeComplete, outcome)

	// Status should be updated to completed
	assert.Equal(t, ngrokv1alpha1.DrainStatusCompleted, ko.Status.DrainStatus)
	assert.Equal(t, "Drain completed successfully", ko.Status.DrainMessage)

	// State should now report draining
	assert.True(t, orchestrator.State().IsDraining(ctx))
}

func TestOrchestrator_HandleDrain_SetsStatusToDraining(t *testing.T) {
	scheme := setupTestScheme(t)

	ko := &ngrokv1alpha1.KubernetesOperator{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "my-release",
			Namespace: "ngrok-operator",
		},
	}

	client := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(ko).
		WithStatusSubresource(ko).
		Build()

	recorder := record.NewFakeRecorder(10)
	orchestrator := NewOrchestrator(OrchestratorConfig{
		Client:         client,
		Recorder:       recorder,
		Log:            logr.Discard(),
		K8sOpNamespace: "ngrok-operator",
		K8sOpName:      "my-release",
	})

	ctx := context.Background()

	// Before drain, status is empty
	assert.Empty(t, ko.Status.DrainStatus)

	// HandleDrain should update status
	_, err := orchestrator.HandleDrain(ctx, ko)
	require.NoError(t, err)

	// Status was set during drain
	assert.Equal(t, ngrokv1alpha1.DrainStatusCompleted, ko.Status.DrainStatus)
}
