package resolvers

import (
	"testing"

	ingressv1alpha1 "github.com/ngrok/ngrok-operator/api/ingress/v1alpha1"
	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestDefaultIPPolicyResolverImplementsIPPolicy(t *testing.T) {
	scheme := runtime.NewScheme()
	fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
	defaultIPPolicyResolver := NewDefaultIPPolicyResolver(fakeClient)
	assert.Implements(t, (*IPPolicyResolver)(nil), defaultIPPolicyResolver)
}

func TestDefaultIPPolicyResolver(t *testing.T) {
	scheme := runtime.NewScheme()
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	utilruntime.Must(ingressv1alpha1.AddToScheme(scheme))
	objects := []runtime.Object{
		&ingressv1alpha1.IPPolicy{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: "namespace",
				Name:      "my-ip-policy",
			},
			Spec: ingressv1alpha1.IPPolicySpec{
				Rules: []ingressv1alpha1.IPPolicyRule{},
			},
			Status: ingressv1alpha1.IPPolicyStatus{
				ID: "ipp_111111111122222222223333333",
			},
		},
	}
	fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithRuntimeObjects(objects...).Build()

	defaultIPPolicyResolver := NewDefaultIPPolicyResolver(fakeClient)

	// Test that resolving a policy name returns the ID
	ids, err := defaultIPPolicyResolver.ResolveIPPolicyNamesorIds(t.Context(), "namespace", []string{"my-ip-policy"})
	assert.NoError(t, err)
	assert.Equal(t, []string{"ipp_111111111122222222223333333"}, ids)

	// Test that when the policy name or id looks like an ID, it is returned as is
	ids, err = defaultIPPolicyResolver.ResolveIPPolicyNamesorIds(t.Context(), "namespace", []string{"ipp_99999999922222222223333333"})
	assert.NoError(t, err)
	assert.Equal(t, []string{"ipp_99999999922222222223333333"}, ids)
}

func TestStaticIPPolicyResolverImplementsIPPolicy(t *testing.T) {
	staticIPPolicyResolver := NewStaticIPPolicyResolver()
	assert.Implements(t, (*IPPolicyResolver)(nil), staticIPPolicyResolver)
}
