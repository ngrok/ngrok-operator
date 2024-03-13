package controllers

import (
	"context"
	"fmt"
	"strings"

	ingressv1alpha1 "github.com/ngrok/kubernetes-ingress-controller/api/ingress/v1alpha1"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

const (
	finalizerName = "k8s.ngrok.com/finalizer"
)

func IsUpsert(o client.Object) bool {
	return o.GetDeletionTimestamp().IsZero()
}

func IsDelete(o client.Object) bool {
	return !o.GetDeletionTimestamp().IsZero()
}

func HasFinalizer(o client.Object) bool {
	return controllerutil.ContainsFinalizer(o, finalizerName)
}

func AddFinalizer(o client.Object) bool {
	return controllerutil.AddFinalizer(o, finalizerName)
}

func RemoveFinalizer(o client.Object) bool {
	return controllerutil.RemoveFinalizer(o, finalizerName)
}

func RegisterAndSyncFinalizer(ctx context.Context, c client.Writer, o client.Object) error {
	if !HasFinalizer(o) {
		AddFinalizer(o)
		return c.Update(ctx, o)
	}
	return nil
}

func RemoveAndSyncFinalizer(ctx context.Context, c client.Writer, o client.Object) error {
	RemoveFinalizer(o)
	return c.Update(ctx, o)
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
