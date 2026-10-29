// LEGACY-domain-kind: delete this file at cleanup.

package managerdriver

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	netv1 "k8s.io/api/networking/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	commonv1alpha1 "github.com/ngrok/ngrok-operator/api/common/v1alpha1"
	ingressv1alpha1 "github.com/ngrok/ngrok-operator/api/ingress/v1alpha1"
	ngrokv1 "github.com/ngrok/ngrok-operator/api/ngrok/v1"
	"github.com/ngrok/ngrok-operator/internal/controller/labels"
)

// The driver creates the v1 Domain for every Ingress/Gateway hostname. When a
// legacy Domain already exists at that name, the v1 object must be a copy of
// it: it takes over the shared reservation and pushes its spec to it.
func TestApplyDomains_CopiesLegacyDomainToV1(t *testing.T) {
	scheme := domainsTestScheme(t)
	legacy := &ingressv1alpha1.Domain{
		Name:      "example-com",
		Namespace: "default",
		Spec: ingressv1alpha1.DomainSpec{
			Domain:        "example.com",
			Description:   "hand-written",
			ResolvesTo:    []ingressv1alpha1.DomainResolvesToEntry{{Value: "eu"}},
			ReclaimPolicy: ingressv1alpha1.DomainReclaimPolicyRetain,
		},
	}
	c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(legacy).Build()

	desired := map[string]ngrokv1.Domain{"example.com": desiredDomain("example-com", "example.com")}
	require.NoError(t, newDomainsTestDriver(scheme).applyDomains(context.Background(), c, desired))

	got := &ngrokv1.Domain{}
	require.NoError(t, c.Get(context.Background(), client.ObjectKeyFromObject(legacy), got))
	assert.Equal(t, "hand-written", got.Spec.Description)
	assert.Equal(t, []ngrokv1.DomainResolvesToEntry{{Value: "eu"}}, got.Spec.ResolvesTo)
	assert.Equal(t, ngrokv1.DomainReclaimPolicyRetain, got.Spec.ReclaimPolicy)
	assert.Equal(t, "ngrok-operator", got.Labels[labels.ControllerName])

	// The legacy object is left exactly as it was.
	stillThere := &ingressv1alpha1.Domain{}
	require.NoError(t, c.Get(context.Background(), client.ObjectKeyFromObject(legacy), stillThere))
	assert.Equal(t, legacy.Spec, stillThere.Spec)
	assert.Empty(t, stillThere.Labels)
}

// A legacy Domain whose metadata can't be represented in v1 is left to serve
// the hostname, and doesn't fail the sync for every other Domain.
func TestApplyDomains_MigrationBlockedIsNonFatal(t *testing.T) {
	scheme := domainsTestScheme(t)
	legacy := &ingressv1alpha1.Domain{
		Name:      "example-com",
		Namespace: "default",
		Spec: ingressv1alpha1.DomainSpec{
			Domain:   "example.com",
			Metadata: commonv1alpha1.MetadataFromLegacyString("free text"),
		},
	}
	c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(legacy).Build()

	desired := map[string]ngrokv1.Domain{
		"example.com":         desiredDomain("example-com", "example.com"),
		"healthy.example.com": desiredDomain("healthy-example-com", "healthy.example.com"),
	}
	require.NoError(t, newDomainsTestDriver(scheme).applyDomains(context.Background(), c, desired))

	err := c.Get(context.Background(), client.ObjectKeyFromObject(legacy), &ngrokv1.Domain{})
	assert.True(t, apierrors.IsNotFound(err), "no v1 copy for a blocked legacy Domain")
	require.NoError(t, c.Get(context.Background(), client.ObjectKey{Namespace: "default", Name: "healthy-example-com"}, &ngrokv1.Domain{}))
}

// Ingress load balancer status must not flip to a v1 Domain that has no
// reservation yet: the empty status could make tools that watch Ingress
// status, such as external-dns, remove records mid-migration.
func TestGetDomainsByDomain_UnbackedV1DoesNotShadowLegacy(t *testing.T) {
	scheme := domainsTestScheme(t)
	legacy := &ingressv1alpha1.Domain{
		Name: "example-com", Namespace: "default",
		Spec:   ingressv1alpha1.DomainSpec{Domain: "example.com"},
		Status: ingressv1alpha1.DomainStatus{ID: "rd_1", CNAMETarget: new("legacy.ngrok-cname.com")},
	}
	v1 := &ngrokv1.Domain{
		Name: "example-com", Namespace: "default",
		Spec: ngrokv1.DomainSpec{Domain: "example.com"},
	}
	ingress := &netv1.Ingress{
		ObjectMeta: metav1.ObjectMeta{Name: "ing", Namespace: "default"},
		Spec:       netv1.IngressSpec{Rules: []netv1.IngressRule{{Host: "example.com"}}},
	}

	status := func(objs ...client.Object) []netv1.IngressLoadBalancerIngress {
		c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(objs...).Build()
		domains, err := getDomainsByDomain(context.Background(), c)
		require.NoError(t, err)
		return calculateIngressLoadBalancerIPStatus(ingress, domains)
	}

	want := []netv1.IngressLoadBalancerIngress{{Hostname: "legacy.ngrok-cname.com"}}
	assert.Equal(t, want, status(legacy), "legacy only")
	assert.Equal(t, want, status(legacy, v1.DeepCopy()), "v1 created but not reserved yet")

	backed := v1.DeepCopy()
	backed.Status = ngrokv1.DomainStatus{ID: "rd_1", CNAMETarget: new("legacy.ngrok-cname.com")}
	assert.Equal(t, want, status(legacy, backed), "v1 took over the same reservation")
}
