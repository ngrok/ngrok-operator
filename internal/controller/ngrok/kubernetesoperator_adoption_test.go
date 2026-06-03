/*
MIT License

Copyright (c) 2024 ngrok, Inc.

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

package ngrok

import (
	"context"
	"testing"

	ngrokv1alpha1 "github.com/ngrok/ngrok-operator/api/ngrok/v1alpha1"
	"github.com/ngrok/ngrok-operator/internal/controller"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/events"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

// Test_KubernetesOperator_delete_adoptionSkipsDrain verifies that when the
// migration-adopted annotation is present, delete() skips the drain workflow
// and the ngrok API delete, marks the drain completed, clears status.ID, and
// returns nil so the base controller removes the finalizer. The DrainOrchestrator
// is intentionally nil here: if the skip path ever fell through to the drain it
// would panic, which is exactly the regression this guards against.
func Test_KubernetesOperator_delete_adoptionSkipsDrain(t *testing.T) {
	s := runtime.NewScheme()
	require.NoError(t, ngrokv1alpha1.AddToScheme(s))

	ko := &ngrokv1alpha1.KubernetesOperator{
		ObjectMeta: metav1.ObjectMeta{
			Name:        "ngrok-operator",
			Namespace:   "ngrok-op",
			Annotations: map[string]string{controller.AnnotationMigrationAdopted: "true"},
		},
		Status: ngrokv1alpha1.KubernetesOperatorStatus{ID: "k8sop_123"},
	}

	c := fake.NewClientBuilder().
		WithScheme(s).
		WithObjects(ko).
		WithStatusSubresource(&ngrokv1alpha1.KubernetesOperator{}).
		Build()

	r := &KubernetesOperatorReconciler{
		Client:   c,
		Recorder: events.NewFakeRecorder(10),
		// DrainOrchestrator deliberately left nil.
	}

	require.NoError(t, r.delete(context.Background(), ko))

	got := &ngrokv1alpha1.KubernetesOperator{}
	require.NoError(t, c.Get(context.Background(), types.NamespacedName{Name: ko.Name, Namespace: ko.Namespace}, got))
	assert.Equal(t, ngrokv1alpha1.DrainStatusCompleted, got.Status.DrainStatus)
	assert.Empty(t, got.Status.ID)
}
