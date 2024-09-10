package controllers

import (
	"context"
	"fmt"
	"strings"

	ingressv1alpha1 "github.com/ngrok/ngrok-operator/api/ingress/v1alpha1"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type IpPolicyResolver struct {
	Client client.Reader
}

func (r *IpPolicyResolver) ValidateIPPolicyNames(ctx context.Context, namespace string, namesOrIds []string) error {
	for _, nameOrId := range namesOrIds {
		if strings.HasPrefix(nameOrId, "ipp_") && len(nameOrId) == 31 {
			// assume this is direct reference to an ngrok object (e.g. by ID), skip it for now
			continue
		}

		policy := new(ingressv1alpha1.IPPolicy)
		if err := r.Client.Get(ctx, types.NamespacedName{Name: nameOrId, Namespace: namespace}, policy); err != nil {
			return err
		}
	}
	return nil
}

// Resolves and IP policy names or IDs to IDs. If the input is not found, it is assumed to be an ID and is returned as is.
func (r *IpPolicyResolver) ResolveIPPolicyNamesorIds(ctx context.Context, namespace string, namesOrIds []string) ([]string, error) {
	m := make(map[string]bool)

	for _, nameOrId := range namesOrIds {
		policy := new(ingressv1alpha1.IPPolicy)
		if err := r.Client.Get(ctx, types.NamespacedName{Name: nameOrId, Namespace: namespace}, policy); err != nil {
			if client.IgnoreNotFound(err) == nil {
				m[nameOrId] = true // assume it's an ID
				continue
			}

			return nil, err // its some other error
		}
		m[policy.Status.ID] = true
	}

	policyIds := []string{}
	for k := range m {
		policyIds = append(policyIds, k)
	}

	return policyIds, nil
}

type SecretResolver struct {
	Client client.Reader
}

func (r *SecretResolver) GetSecret(ctx context.Context, namespace, name, key string) (string, error) {
	secret := &v1.Secret{}
	err := r.Client.Get(ctx, types.NamespacedName{
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
