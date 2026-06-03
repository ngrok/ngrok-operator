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

package bindings

import (
	"context"
	"testing"

	bindingsv1alpha1 "github.com/ngrok/ngrok-operator/api/bindings/v1alpha1"
	"github.com/ngrok/ngrok-operator/internal/controller"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	v1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/events"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

// adoptionTestScheme builds a scheme with the core and bindings types needed to
// exercise BoundEndpoint deletion against a fake client.
func adoptionTestScheme(t *testing.T) *runtime.Scheme {
	t.Helper()
	s := runtime.NewScheme()
	require.NoError(t, v1.AddToScheme(s))
	require.NoError(t, bindingsv1alpha1.AddToScheme(s))
	return s
}

func newAdoptionBoundEndpoint(adopted bool) *bindingsv1alpha1.BoundEndpoint {
	be := &bindingsv1alpha1.BoundEndpoint{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "abc123",
			Namespace: "ngrok-op",
		},
		Spec: bindingsv1alpha1.BoundEndpointSpec{
			Scheme: "https",
			Port:   8080,
			Target: bindingsv1alpha1.EndpointTarget{
				Service:   "client-service",
				Namespace: "client-namespace",
				Protocol:  "TCP",
				Port:      8080,
			},
		},
		Status: bindingsv1alpha1.BoundEndpointStatus{HashedName: "abc123"},
	}
	if adopted {
		be.Annotations = map[string]string{controller.AnnotationMigrationAdopted: "true"}
	}
	return be
}

// Test_BoundEndpoint_delete_adoptionSkipsServiceCleanup verifies that when the
// migration-adopted annotation is present, delete() leaves the target and
// upstream Services in place so the new ngrok.com/v1 controller can re-parent
// them.
func Test_BoundEndpoint_delete_adoptionSkipsServiceCleanup(t *testing.T) {
	s := adoptionTestScheme(t)

	be := newAdoptionBoundEndpoint(true)
	r := &BoundEndpointReconciler{ClusterDomain: "svc.cluster.local"}
	target, upstream := r.convertBoundEndpointToServices(be)
	targetNS := &v1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: target.Namespace}}

	c := fake.NewClientBuilder().
		WithScheme(s).
		WithObjects(targetNS, target, upstream).
		Build()

	r.Client = c
	r.Recorder = events.NewFakeRecorder(10)

	require.NoError(t, r.delete(context.Background(), be))

	// Both Services must still exist — adoption skips cleanup.
	gotTarget := &v1.Service{}
	assert.NoError(t, c.Get(context.Background(), types.NamespacedName{Name: target.Name, Namespace: target.Namespace}, gotTarget))
	gotUpstream := &v1.Service{}
	assert.NoError(t, c.Get(context.Background(), types.NamespacedName{Name: upstream.Name, Namespace: upstream.Namespace}, gotUpstream))
}

// Test_BoundEndpoint_delete_withoutAnnotationDeletesServices verifies the
// unchanged behavior: without the annotation, delete() removes both Services.
func Test_BoundEndpoint_delete_withoutAnnotationDeletesServices(t *testing.T) {
	s := adoptionTestScheme(t)

	be := newAdoptionBoundEndpoint(false)
	r := &BoundEndpointReconciler{ClusterDomain: "svc.cluster.local"}
	target, upstream := r.convertBoundEndpointToServices(be)
	targetNS := &v1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: target.Namespace}}

	c := fake.NewClientBuilder().
		WithScheme(s).
		WithObjects(targetNS, target, upstream).
		Build()

	r.Client = c
	r.Recorder = events.NewFakeRecorder(10)

	require.NoError(t, r.delete(context.Background(), be))

	// Both Services must be gone.
	err := c.Get(context.Background(), types.NamespacedName{Name: target.Name, Namespace: target.Namespace}, &v1.Service{})
	assert.True(t, apierrors.IsNotFound(err), "target service should be deleted, got %v", err)
	err = c.Get(context.Background(), types.NamespacedName{Name: upstream.Name, Namespace: upstream.Namespace}, &v1.Service{})
	assert.True(t, apierrors.IsNotFound(err), "upstream service should be deleted, got %v", err)
}
