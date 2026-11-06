package managerdriver

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"

	ngrokv1 "github.com/ngrok/ngrok-operator/api/ngrok/v1"
	"github.com/ngrok/ngrok-operator/internal/controller/labels"
)

// The driver's client reads through the manager's cache, which can lag behind
// a Domain applyDomains has just created. Applying it must not read it back
// through that cache: a second create would fail with AlreadyExists, and Sync
// would return before applying endpoints or updating statuses.
func TestApplyDomains_CreatedDomainNotYetInCache(t *testing.T) {
	scheme := domainsTestScheme(t)
	stale := fake.NewClientBuilder().
		WithScheme(scheme).
		WithInterceptorFuncs(interceptor.Funcs{
			// The cache never sees any v1 Domain.
			Get: func(ctx context.Context, cl client.WithWatch, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
				if _, ok := obj.(*ngrokv1.Domain); ok {
					return apierrors.NewNotFound(ngrokv1.GroupVersion.WithResource("domains").GroupResource(), key.Name)
				}
				return cl.Get(ctx, key, obj, opts...)
			},
		}).
		Build()

	desired := map[string]ngrokv1.Domain{"example.com": desiredDomain("example-com", "example.com")}
	require.NoError(t, newDomainsTestDriver(scheme).applyDomains(context.Background(), stale, desired))

	list := &ngrokv1.DomainList{}
	require.NoError(t, stale.List(context.Background(), list))
	require.Len(t, list.Items, 1)
	got := list.Items[0]
	assert.Equal(t, "example.com", got.Spec.Domain)
	assert.Equal(t, "ngrok-operator", got.Labels[labels.ControllerName])
}
