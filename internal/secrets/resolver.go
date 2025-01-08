package secrets

import "context"

type Resolver interface {
	GetSecret(ctx context.Context, namespace, name, key string) (string, error)
}
