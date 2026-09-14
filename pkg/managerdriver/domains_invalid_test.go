package managerdriver

import (
	"context"
	"errors"
	"testing"

	"github.com/go-logr/logr"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/validation/field"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"

	ingressv1alpha1 "github.com/ngrok/ngrok-operator/api/ingress/v1alpha1"
)

// invalidDomainErr mimics what the API server returns when the CEL immutability
// rule on Domain.spec.domain rejects a rename. The fake client does not evaluate
// CEL, so the rejection is injected instead. internal/controller/ingress pins
// that a real CEL rejection lands in this error class.
func invalidDomainErr(name string) error {
	return apierrors.NewInvalid(
		schema.GroupKind{Group: "ingress.k8s.ngrok.com", Kind: "Domain"},
		name,
		field.ErrorList{
			field.Invalid(
				field.NewPath("spec", "domain"),
				"renamed.example.com",
				"spec.domain is immutable. Reserved domains cannot be renamed via the ngrok API; delete this Domain and create a new one instead",
			),
		},
	)
}

func domainsTestScheme(t *testing.T) *runtime.Scheme {
	t.Helper()

	scheme := runtime.NewScheme()
	require.NoError(t, clientgoscheme.AddToScheme(scheme))
	require.NoError(t, ingressv1alpha1.AddToScheme(scheme))
	return scheme
}

func newDomainsTestDriver(scheme *runtime.Scheme) *Driver {
	const (
		controllerNamespace = "ngrok-system"
		controllerName      = "ngrok-operator"
	)

	return NewDriver(
		logr.Discard(),
		scheme,
		controllerName,
		types.NamespacedName{Namespace: controllerNamespace, Name: controllerName},
	)
}

func desiredDomain(name, domain string) ingressv1alpha1.Domain {
	return ingressv1alpha1.Domain{
		Name:      name,
		Namespace: "default",
		Spec:      ingressv1alpha1.DomainSpec{Domain: domain},
	}
}

// A Domain whose spec.domain drifted from its desired hostname before the CEL
// immutability rule shipped makes applyDomains' rewrite a rename, which
// admission rejects and no retry can fix. That must not fail the errgroup:
// Sync returns early on an applyDomains error, skipping applyAgentEndpoints,
// applyCloudEndpoints and updateStatuses, so one unfixable Domain would wedge
// reconciliation for every Ingress and Gateway in the cluster.
func TestApplyDomains_InvalidDomainIsNonFatal(t *testing.T) {
	scheme := domainsTestScheme(t)

	drifted := &ingressv1alpha1.Domain{
		Name:      "drifted-example-com",
		Namespace: "default",
		Spec:      ingressv1alpha1.DomainSpec{Domain: "renamed.example.com"},
	}

	c := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(drifted).
		WithInterceptorFuncs(interceptor.Funcs{
			Patch: func(ctx context.Context, cl client.WithWatch, obj client.Object, patch client.Patch, opts ...client.PatchOption) error {
				if obj.GetName() == drifted.Name {
					return invalidDomainErr(obj.GetName())
				}
				return cl.Patch(ctx, obj, patch, opts...)
			},
		}).
		Build()

	desired := map[string]ingressv1alpha1.Domain{
		"drifted.example.com": desiredDomain(drifted.Name, "drifted.example.com"),
		"healthy.example.com": desiredDomain("healthy-example-com", "healthy.example.com"),
	}

	require.NoError(t,
		newDomainsTestDriver(scheme).applyDomains(context.Background(), c, desired),
		"an Invalid rejection on one Domain must not fail the sync",
	)

	// The healthy domain in the same batch was still applied.
	healthy := &ingressv1alpha1.Domain{}
	require.NoError(t, c.Get(context.Background(), client.ObjectKey{Namespace: "default", Name: "healthy-example-com"}, healthy))
	assert.Equal(t, "healthy.example.com", healthy.Spec.Domain)

	// The rejected Domain keeps its drifted spec; nothing was silently forced.
	got := &ingressv1alpha1.Domain{}
	require.NoError(t, c.Get(context.Background(), client.ObjectKeyFromObject(drifted), got))
	assert.Equal(t, "renamed.example.com", got.Spec.Domain)
}

// Every other error class stays fatal, so genuine failures still surface and are
// retried by the caller rather than being swallowed alongside Invalid.
func TestApplyDomains_OtherErrorsAreFatal(t *testing.T) {
	scheme := domainsTestScheme(t)

	existing := &ingressv1alpha1.Domain{
		Name:      "example-com",
		Namespace: "default",
		Spec:      ingressv1alpha1.DomainSpec{Domain: "example.com"},
	}

	boom := errors.New("boom")

	c := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(existing).
		WithInterceptorFuncs(interceptor.Funcs{
			Patch: func(_ context.Context, _ client.WithWatch, _ client.Object, _ client.Patch, _ ...client.PatchOption) error {
				return boom
			},
		}).
		Build()

	desired := map[string]ingressv1alpha1.Domain{
		// A label backfill is enough to make CreateOrPatch issue a Patch.
		"example.com": desiredDomain(existing.Name, "example.com"),
	}

	err := newDomainsTestDriver(scheme).applyDomains(context.Background(), c, desired)
	require.Error(t, err)
	assert.ErrorIs(t, err, boom)
}
