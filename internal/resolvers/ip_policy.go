package resolvers

import (
	"context"
	"fmt"
	"strings"

	ingressv1alpha1 "github.com/ngrok/ngrok-operator/api/ingress/v1alpha1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// IPPolicyResolver is an interface for resolving IP policies. It can be used when you want to resolve a namespaced IP policy to a
// ngrok IP policy ID. By using an interface, the behavior of the IP policy resolver implementation can easily be swapped
// out for testing.
type IPPolicyResolver interface {
	ValidateIPPolicyNames(ctx context.Context, namespace string, namesOrIds []string) error
	ResolveIPPolicyNamesorIds(ctx context.Context, namespace string, namesOrIds []string) ([]string, error)
}

// DefaultIPPolicyResolver is an IP policy resolver that resolves IP policies from the Kubernetes API.
type DefaultIPPolicyResolver struct {
	client client.Reader
}

// NewDefaultIPPolicyResolver creates a new DefaultIPPolicyResolver with the given client.
func NewDefaultIPPolicyResolver(client client.Reader) *DefaultIPPolicyResolver {
	return &DefaultIPPolicyResolver{client: client}
}

// Resolves and IP policy names or IDs to IDs. If the input is not found, it is assumed to be an ID and is returned as is.
func (r *DefaultIPPolicyResolver) ResolveIPPolicyNamesorIds(ctx context.Context, namespace string, namesOrIds []string) ([]string, error) {
	resolved := make([]string, len(namesOrIds))

	for i, nameOrId := range namesOrIds {
		policy := new(ingressv1alpha1.IPPolicy)
		if err := r.client.Get(ctx, types.NamespacedName{Name: nameOrId, Namespace: namespace}, policy); err != nil {
			if client.IgnoreNotFound(err) == nil {
				resolved[i] = nameOrId // assume it's an ID if not found
				continue
			}

			return nil, err // its some other error
		}
		resolved[i] = policy.Status.ID
	}

	return resolved, nil
}

// ValidateIPPolicyNames validates that the IP policy names or IDs exist in the Kubernetes API.
func (r *DefaultIPPolicyResolver) ValidateIPPolicyNames(ctx context.Context, namespace string, namesOrIds []string) error {
	for _, nameOrId := range namesOrIds {
		if strings.HasPrefix(nameOrId, "ipp_") && len(nameOrId) == 31 {
			// assume this is direct reference to an ngrok object (e.g. by ID), skip it for now
			continue
		}

		policy := new(ingressv1alpha1.IPPolicy)
		if err := r.client.Get(ctx, types.NamespacedName{Name: nameOrId, Namespace: namespace}, policy); err != nil {
			return err
		}
	}
	return nil
}

// StaticIPPolicyResolver is an IP policy resolver that resolves IP policies from a static map.
type StaticIPPolicyResolver struct {
	// map of map[namespace]map[ipPolicyName]ipPolicyID
	ipPolicies map[string]map[string]string
}

// NewStaticIPPolicyResolver creates a new StaticIPPolicyResolver.
func NewStaticIPPolicyResolver() *StaticIPPolicyResolver {
	return &StaticIPPolicyResolver{ipPolicies: make(map[string]map[string]string)}
}

// AddIPPolicy adds an IP policy to the static map.
func (r *StaticIPPolicyResolver) AddIPPolicy(namespace, name, id string) {
	if _, ok := r.ipPolicies[namespace]; !ok {
		r.ipPolicies[namespace] = make(map[string]string)
	}
	r.ipPolicies[namespace][name] = id
}

// ResolveIPPolicyNamesorIds resolves IP policy names or IDs to IDs. If the input is not found, it is assumed to be an ID
func (r *StaticIPPolicyResolver) ResolveIPPolicyNamesorIds(ctx context.Context, namespace string, namesOrIds []string) ([]string, error) {
	nsPolicies, ok := r.ipPolicies[namespace]
	if !ok {
		return nil, fmt.Errorf("namespace %s not found", namespace)
	}

	resolved := make([]string, len(namesOrIds))

	for i, nameOrId := range namesOrIds {
		if id, ok := nsPolicies[nameOrId]; ok {
			resolved[i] = id
			continue
		}

		// assume it's an ID if not found
		resolved[i] = nameOrId
	}

	return resolved, nil
}

// ValidateIPPolicyNames validates that the IP policy names or IDs exist in the static map.
func (r *StaticIPPolicyResolver) ValidateIPPolicyNames(ctx context.Context, namespace string, namesOrIds []string) error {
	nsPolicies, ok := r.ipPolicies[namespace]
	if !ok {
		return fmt.Errorf("namespace %s not found", namespace)
	}

	for _, nameOrId := range namesOrIds {
		if strings.HasPrefix(nameOrId, "ipp_") && len(nameOrId) == 31 {
			// assume this is direct reference to an ngrok object (e.g. by ID), skip it for now
			continue
		}

		if _, ok := nsPolicies[nameOrId]; !ok {
			return fmt.Errorf("policy %s not found in namespace %s", nameOrId, namespace)
		}
	}
	return nil
}
