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
	"errors"
	"testing"

	"github.com/go-logr/logr"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	netv1 "k8s.io/api/networking/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"
	gatewayv1alpha2 "sigs.k8s.io/gateway-api/apis/v1alpha2"

	bindingsv1alpha1 "github.com/ngrok/ngrok-operator/api/bindings/v1alpha1"
	ingressv1alpha1 "github.com/ngrok/ngrok-operator/api/ingress/v1alpha1"
	ngrokv1alpha1 "github.com/ngrok/ngrok-operator/api/ngrok/v1alpha1"
	"github.com/ngrok/ngrok-operator/internal/util"
)

func TestDrainResult(t *testing.T) {
	t.Run("Progress", func(t *testing.T) {
		r := &DrainResult{Completed: 5, Total: 10}
		assert.Equal(t, "5/10", r.Progress())

		r = &DrainResult{Completed: 5, Total: 10, Failed: 3}
		assert.Equal(t, "8/10", r.Progress())
	})

	t.Run("ErrorStrings", func(t *testing.T) {
		r := &DrainResult{
			Errors: []error{
				errors.New("error 1"),
				errors.New("error 2"),
			},
		}
		strs := r.ErrorStrings()
		assert.Equal(t, []string{"error 1", "error 2"}, strs)

		r = &DrainResult{}
		assert.Equal(t, []string{}, r.ErrorStrings())
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
			Finalizers: []string{util.FinalizerName},
		},
	}

	c := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(ingress).
		Build()

	drainer := &Drainer{
		Client: c,
		Log:    logr.Discard(),
	}

	err := drainer.drainUserResource(context.Background(), ingress)
	require.NoError(t, err)

	assert.False(t, util.HasFinalizer(ingress), "finalizer should be removed")
}

func TestDrainer_DrainOperatorResource(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, ingressv1alpha1.AddToScheme(scheme))
	require.NoError(t, ngrokv1alpha1.AddToScheme(scheme))

	// Note: no finalizer here - the fake client will immediately delete the resource
	// when Delete is called. In a real cluster, the controller would handle
	// removing the finalizer after cleanup, but we can't simulate that here.
	domain := &ingressv1alpha1.Domain{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-domain",
			Namespace: "ngrok-operator",
		},
		Spec: ingressv1alpha1.DomainSpec{
			Domain: "test.ngrok.io",
		},
	}

	c := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(domain).
		Build()

	drainer := &Drainer{
		Client: c,
		Log:    logr.Discard(),
		Policy: ngrokv1alpha1.DrainPolicyDelete,
	}

	err := drainer.drainOperatorResource(context.Background(), domain)
	require.NoError(t, err)

	// Verify resource was deleted
	var fetched ingressv1alpha1.Domain
	err = c.Get(context.Background(), client.ObjectKey{Name: "test-domain", Namespace: "ngrok-operator"}, &fetched)
	assert.True(t, errors.Is(err, errors.New("")) || err != nil, "resource should be deleted")
}

func TestDrainer_DrainOperatorResource_RetainMode(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, ingressv1alpha1.AddToScheme(scheme))
	require.NoError(t, ngrokv1alpha1.AddToScheme(scheme))

	domain := &ingressv1alpha1.Domain{
		ObjectMeta: metav1.ObjectMeta{
			Name:       "test-domain",
			Namespace:  "ngrok-operator",
			Finalizers: []string{util.FinalizerName},
		},
		Spec: ingressv1alpha1.DomainSpec{
			Domain: "test.ngrok.io",
		},
	}

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(domain).
		Build()

	drainer := &Drainer{
		Client: fakeClient,
		Log:    logr.Discard(),
		Policy: ngrokv1alpha1.DrainPolicyRetain,
	}

	err := drainer.drainOperatorResource(context.Background(), domain)
	require.NoError(t, err)

	// In Retain mode, finalizer should be removed but resource should still exist
	var fetched ingressv1alpha1.Domain
	err = fakeClient.Get(context.Background(), client.ObjectKey{Name: "test-domain", Namespace: "ngrok-operator"}, &fetched)
	require.NoError(t, err, "resource should still exist in Retain mode")
	assert.False(t, util.HasFinalizer(&fetched), "finalizer should be removed")
}

func TestDrainer_DrainAll_EmptyCluster(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, netv1.AddToScheme(scheme))
	require.NoError(t, ingressv1alpha1.AddToScheme(scheme))
	require.NoError(t, ngrokv1alpha1.AddToScheme(scheme))

	c := fake.NewClientBuilder().
		WithScheme(scheme).
		Build()

	drainer := &Drainer{
		Client: c,
		Log:    logr.Discard(),
	}

	result, err := drainer.DrainAll(context.Background())
	require.NoError(t, err)
	assert.Equal(t, 0, result.Completed)
	assert.True(t, result.IsComplete())
}

func TestDrainer_DrainUserCreatedResources_NoControllerLabels(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, netv1.AddToScheme(scheme))
	require.NoError(t, corev1.AddToScheme(scheme))
	require.NoError(t, ingressv1alpha1.AddToScheme(scheme))
	require.NoError(t, ngrokv1alpha1.AddToScheme(scheme))
	require.NoError(t, bindingsv1alpha1.AddToScheme(scheme))
	require.NoError(t, gatewayv1.Install(scheme))
	require.NoError(t, gatewayv1alpha2.Install(scheme))

	ipPolicy := &ingressv1alpha1.IPPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:       "user-created-ippolicy",
			Namespace:  "default",
			Finalizers: []string{util.FinalizerName},
		},
		Spec: ingressv1alpha1.IPPolicySpec{
			Description: "User-created IP Policy without controller labels",
		},
	}

	cloudEndpoint := &ngrokv1alpha1.CloudEndpoint{
		ObjectMeta: metav1.ObjectMeta{
			Name:       "user-created-cloudendpoint",
			Namespace:  "default",
			Finalizers: []string{util.FinalizerName},
		},
		Spec: ngrokv1alpha1.CloudEndpointSpec{
			URL: "https://example.ngrok.io",
		},
	}

	agentEndpoint := &ngrokv1alpha1.AgentEndpoint{
		ObjectMeta: metav1.ObjectMeta{
			Name:       "user-created-agentendpoint",
			Namespace:  "default",
			Finalizers: []string{util.FinalizerName},
		},
		Spec: ngrokv1alpha1.AgentEndpointSpec{
			URL: "https://example.ngrok.io",
		},
	}

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(ipPolicy, cloudEndpoint, agentEndpoint).
		Build()

	drainer := &Drainer{
		Client: fakeClient,
		Log:    logr.Discard(),
		Policy: ngrokv1alpha1.DrainPolicyRetain,
	}

	result, err := drainer.DrainAll(context.Background())
	require.NoError(t, err)
	assert.Equal(t, 3, result.Total, "should find 3 resources to drain")
	assert.Equal(t, 3, result.Completed, "should drain all 3 resources")
	assert.False(t, result.HasErrors())

	var fetchedIPPolicy ingressv1alpha1.IPPolicy
	err = fakeClient.Get(context.Background(), client.ObjectKey{Name: "user-created-ippolicy", Namespace: "default"}, &fetchedIPPolicy)
	require.NoError(t, err)
	assert.False(t, util.HasFinalizer(&fetchedIPPolicy), "IPPolicy finalizer should be removed")

	var fetchedCloudEndpoint ngrokv1alpha1.CloudEndpoint
	err = fakeClient.Get(context.Background(), client.ObjectKey{Name: "user-created-cloudendpoint", Namespace: "default"}, &fetchedCloudEndpoint)
	require.NoError(t, err)
	assert.False(t, util.HasFinalizer(&fetchedCloudEndpoint), "CloudEndpoint finalizer should be removed")

	var fetchedAgentEndpoint ngrokv1alpha1.AgentEndpoint
	err = fakeClient.Get(context.Background(), client.ObjectKey{Name: "user-created-agentendpoint", Namespace: "default"}, &fetchedAgentEndpoint)
	require.NoError(t, err)
	assert.False(t, util.HasFinalizer(&fetchedAgentEndpoint), "AgentEndpoint finalizer should be removed")
}

func TestDrainer_SkipsResourcesWithoutFinalizer(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, netv1.AddToScheme(scheme))
	require.NoError(t, corev1.AddToScheme(scheme))
	require.NoError(t, ingressv1alpha1.AddToScheme(scheme))
	require.NoError(t, ngrokv1alpha1.AddToScheme(scheme))
	require.NoError(t, bindingsv1alpha1.AddToScheme(scheme))
	require.NoError(t, gatewayv1.Install(scheme))
	require.NoError(t, gatewayv1alpha2.Install(scheme))

	ipPolicyWithFinalizer := &ingressv1alpha1.IPPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:       "with-finalizer",
			Namespace:  "default",
			Finalizers: []string{util.FinalizerName},
		},
	}

	ipPolicyWithoutFinalizer := &ingressv1alpha1.IPPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "without-finalizer",
			Namespace: "default",
		},
	}

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(ipPolicyWithFinalizer, ipPolicyWithoutFinalizer).
		Build()

	drainer := &Drainer{
		Client: fakeClient,
		Log:    logr.Discard(),
		Policy: ngrokv1alpha1.DrainPolicyRetain,
	}

	result, err := drainer.DrainAll(context.Background())
	require.NoError(t, err)
	assert.Equal(t, 1, result.Total, "should only count resource with finalizer")
	assert.Equal(t, 1, result.Completed, "should only drain resource with finalizer")
}

type errorClient struct {
	client.Client
	listErr   error
	deleteErr error
}

func (c *errorClient) List(ctx context.Context, list client.ObjectList, opts ...client.ListOption) error {
	if c.listErr != nil {
		return c.listErr
	}
	return c.Client.List(ctx, list, opts...)
}

func (c *errorClient) Delete(ctx context.Context, obj client.Object, opts ...client.DeleteOption) error {
	if c.deleteErr != nil {
		return c.deleteErr
	}
	return c.Client.Delete(ctx, obj, opts...)
}

func TestDrainer_drainList_ListError(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, ingressv1alpha1.AddToScheme(scheme))

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		Build()

	listErr := errors.New("list failed: connection refused")
	errClient := &errorClient{Client: fakeClient, listErr: listErr}

	drainer := &Drainer{
		Client: errClient,
		Log:    logr.Discard(),
		Policy: ngrokv1alpha1.DrainPolicyRetain,
	}

	completed, total, errs := drainer.drainList(
		context.Background(),
		"Domain",
		&ingressv1alpha1.DomainList{},
		false,
		drainer.drainUserResource,
	)

	assert.Equal(t, 0, completed)
	assert.Equal(t, 0, total)
	require.Len(t, errs, 1)
	assert.Contains(t, errs[0].Error(), "failed to list Domain")
	assert.Contains(t, errs[0].Error(), "connection refused")
}

type noMatchErrorClient struct {
	client.Client
}

func (c *noMatchErrorClient) List(_ context.Context, _ client.ObjectList, _ ...client.ListOption) error {
	return &meta.NoKindMatchError{
		GroupKind:        schema.GroupKind{Group: "gateway.networking.k8s.io", Kind: "HTTPRoute"},
		SearchedVersions: []string{"v1"},
	}
}

func TestDrainer_drainList_NoMatchError_SkipsOptionalCRD(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, gatewayv1.Install(scheme))

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		Build()

	errClient := &noMatchErrorClient{Client: fakeClient}

	drainer := &Drainer{
		Client: errClient,
		Log:    logr.Discard(),
		Policy: ngrokv1alpha1.DrainPolicyRetain,
	}

	completed, total, errs := drainer.drainList(
		context.Background(),
		"HTTPRoute",
		&gatewayv1.HTTPRouteList{},
		true,
		drainer.drainUserResource,
	)

	assert.Equal(t, 0, completed)
	assert.Equal(t, 0, total)
	assert.Len(t, errs, 0, "should skip NoMatch error when skipNoMatch is true")
}

func TestDrainer_DrainOperatorResource_AlreadyDeleted(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, ingressv1alpha1.AddToScheme(scheme))
	require.NoError(t, ngrokv1alpha1.AddToScheme(scheme))

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		Build()

	domain := &ingressv1alpha1.Domain{
		ObjectMeta: metav1.ObjectMeta{
			Name:       "already-deleted",
			Namespace:  "ngrok-operator",
			Finalizers: []string{util.FinalizerName},
		},
		Spec: ingressv1alpha1.DomainSpec{
			Domain: "test.ngrok.io",
		},
	}

	drainer := &Drainer{
		Client: fakeClient,
		Log:    logr.Discard(),
		Policy: ngrokv1alpha1.DrainPolicyDelete,
	}

	err := drainer.drainOperatorResource(context.Background(), domain)
	require.NoError(t, err, "should succeed when resource is already deleted (NotFound)")
}

func TestDrainer_DrainOperatorResource_DeleteError(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, ingressv1alpha1.AddToScheme(scheme))
	require.NoError(t, ngrokv1alpha1.AddToScheme(scheme))

	domain := &ingressv1alpha1.Domain{
		ObjectMeta: metav1.ObjectMeta{
			Name:       "test-domain",
			Namespace:  "ngrok-operator",
			Finalizers: []string{util.FinalizerName},
		},
		Spec: ingressv1alpha1.DomainSpec{
			Domain: "test.ngrok.io",
		},
	}

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(domain).
		Build()

	deleteErr := errors.New("forbidden: insufficient permissions")
	errClient := &errorClient{Client: fakeClient, deleteErr: deleteErr}

	drainer := &Drainer{
		Client: errClient,
		Log:    logr.Discard(),
		Policy: ngrokv1alpha1.DrainPolicyDelete,
	}

	err := drainer.drainOperatorResource(context.Background(), domain)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to delete")
	assert.Contains(t, err.Error(), "insufficient permissions")
}

type updateErrorClientForDrain struct {
	client.Client
	updateErr error
}

func (c *updateErrorClientForDrain) Update(ctx context.Context, obj client.Object, opts ...client.UpdateOption) error {
	if c.updateErr != nil {
		return c.updateErr
	}
	return c.Client.Update(ctx, obj, opts...)
}

func TestDrainer_DrainOperatorResource_RetainUpdateError(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, ingressv1alpha1.AddToScheme(scheme))
	require.NoError(t, ngrokv1alpha1.AddToScheme(scheme))

	domain := &ingressv1alpha1.Domain{
		ObjectMeta: metav1.ObjectMeta{
			Name:       "test-domain",
			Namespace:  "ngrok-operator",
			Finalizers: []string{util.FinalizerName},
		},
		Spec: ingressv1alpha1.DomainSpec{
			Domain: "test.ngrok.io",
		},
	}

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(domain).
		Build()

	updateErr := errors.New("conflict: object has been modified")
	errClient := &updateErrorClientForDrain{Client: fakeClient, updateErr: updateErr}

	drainer := &Drainer{
		Client: errClient,
		Log:    logr.Discard(),
		Policy: ngrokv1alpha1.DrainPolicyRetain,
	}

	err := drainer.drainOperatorResource(context.Background(), domain)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to remove finalizer")
}
