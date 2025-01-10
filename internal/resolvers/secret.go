package resolvers

import (
	"context"
	"fmt"

	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// SecretResolver is an interface for resolving secrets. It can be used when you want to resolve a namespaced secret with a key
// to the secret value. By using an interface, the behavior of the secret resolver implementation can easily be swapped
// out for testing.
type SecretResolver interface {
	GetSecret(ctx context.Context, namespace, name, key string) (string, error)
}

// DefaultSecretResovler is a secret resolver that resolves secrets from the Kubernetes API.
type DefaultSecretResovler struct {
	client client.Reader
}

// NewDefaultSecretResovler creates a new DefaultSecretResovler with the given client.
func NewDefaultSecretResovler(client client.Reader) SecretResolver {
	return &DefaultSecretResovler{client: client}
}

// GetSecret resolves a secret from the Kubernetes API given a namespace, name, and key.
func (r *DefaultSecretResovler) GetSecret(ctx context.Context, namespace, name, key string) (string, error) {
	secret := &v1.Secret{}
	err := r.client.Get(ctx, types.NamespacedName{
		Namespace: namespace,
		Name:      name,
	}, secret)
	if err != nil {
		return "", err
	}

	value, ok := secret.Data[key]
	if !ok {
		return "", fmt.Errorf("secret '%s/%s' does not contain key '%s'", namespace, name, key)
	}
	return string(value), nil
}

// StaticSecretResolver is a secret resolver that resolves secrets from a static map.
type StaticSecretResolver struct {
	// map[namespace]map[secretName]map[key]value
	secrets map[string]map[string]map[string]string
}

func NewStaticSecretResolver() *StaticSecretResolver {
	return &StaticSecretResolver{
		secrets: make(map[string]map[string]map[string]string),
	}
}

func (r *StaticSecretResolver) AddSecret(namespace, name, key, value string) {
	if r.secrets == nil {
		r.secrets = make(map[string]map[string]map[string]string)
	}
	nsSecrets, ok := r.secrets[namespace]
	if !ok {
		nsSecrets = make(map[string]map[string]string)
		r.secrets[namespace] = nsSecrets
	}
	secret, ok := nsSecrets[name]
	if !ok {
		secret = make(map[string]string)
		nsSecrets[name] = secret
	}
	secret[key] = value
}

func (r *StaticSecretResolver) GetSecret(ctx context.Context, namespace, name, key string) (string, error) {
	nsSecrets, ok := r.secrets[namespace]
	if !ok {
		return "", fmt.Errorf("namespace '%s' not found", namespace)
	}
	secret, ok := nsSecrets[name]
	if !ok {
		return "", fmt.Errorf("secret '%s/%s' not found", namespace, name)
	}
	value, ok := secret[key]
	if !ok {
		return "", fmt.Errorf("key '%s' not found in secret '%s/%s'", key, namespace, name)
	}
	return value, nil
}
