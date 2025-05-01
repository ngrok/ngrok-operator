package resolvers

import (
	"testing"

	"github.com/stretchr/testify/assert"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestDefaultSecretResolverImplementsSecretResolver(t *testing.T) {
	scheme := runtime.NewScheme()
	fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
	defaultSecretResolver := NewDefaultSecretResovler(fakeClient)
	assert.Implements(t, (*SecretResolver)(nil), defaultSecretResolver)
}

func TestDefaultSecretResolver(t *testing.T) {
	scheme := runtime.NewScheme()
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	secret := &v1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "namespace",
			Name:      "name",
		},
		Data: map[string][]byte{
			"key": []byte("secret-value"),
		},
	}

	fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithRuntimeObjects(secret).Build()
	defaultSecretResolver := NewDefaultSecretResovler(fakeClient)

	value, err := defaultSecretResolver.GetSecret(t.Context(), "namespace", "name", "key")
	assert.NoError(t, err)
	assert.Equal(t, "secret-value", value)
}

func TestStaticSecretResolverImplementsSecretResolver(t *testing.T) {
	StaticSecretResolver := NewStaticSecretResolver()
	assert.Implements(t, (*SecretResolver)(nil), StaticSecretResolver)
}

func TestStaticSecretResolver(t *testing.T) {
	resolver := NewStaticSecretResolver()

	resolver.AddSecret("namespace", "name", "key", "secret-value")
	value, err := resolver.GetSecret(t.Context(), "namespace", "name", "key")
	assert.NoError(t, err)
	assert.Equal(t, "secret-value", value)

	value, err = resolver.GetSecret(t.Context(), "non-existent-namespace", "name", "key")
	assert.EqualError(t, err, "namespace 'non-existent-namespace' not found")
	assert.Empty(t, value)

	value, err = resolver.GetSecret(t.Context(), "namespace", "non-existent-name", "key")
	assert.EqualError(t, err, "secret 'namespace/non-existent-name' not found")
	assert.Empty(t, value)

	value, err = resolver.GetSecret(t.Context(), "namespace", "name", "non-existent-key")
	assert.EqualError(t, err, "key 'non-existent-key' not found in secret 'namespace/name'")
	assert.Empty(t, value)

}
