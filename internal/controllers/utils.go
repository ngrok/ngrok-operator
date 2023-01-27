package controllers

import (
	"context"

	ingressv1alpha1 "github.com/ngrok/kubernetes-ingress-controller/api/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

const (
	finalizerName = "k8s.ngrok.com/finalizer"
	// TODO: We can technically figure this out by looking at things like our resolv.conf or we can just take this as a helm option
	clusterDomain = "svc.cluster.local"
)

func isDelete(meta metav1.ObjectMeta) bool {
	return meta.DeletionTimestamp != nil && !meta.DeletionTimestamp.IsZero()
}

func hasFinalizer(o client.Object) bool {
	return controllerutil.ContainsFinalizer(o, finalizerName)
}

func addFinalizer(o client.Object) bool {
	return controllerutil.AddFinalizer(o, finalizerName)
}

func removeFinalizer(o client.Object) bool {
	return controllerutil.RemoveFinalizer(o, finalizerName)
}

func registerAndSyncFinalizer(ctx context.Context, c client.Writer, o client.Object) error {
	if !hasFinalizer(o) {
		addFinalizer(o)
		return c.Update(ctx, o)
	}
	return nil
}

func removeAndSyncFinalizer(ctx context.Context, c client.Writer, o client.Object) error {
	removeFinalizer(o)
	return c.Update(ctx, o)
}

type ipPolicyResolver struct {
	client client.Reader
}

// Resolves and IP policy names or IDs to IDs. If the input is not found, it is assumed to be an ID and is returned as is.
func (r *ipPolicyResolver) resolveIPPolicyNamesorIds(ctx context.Context, namespace string, namesOrIds []string) ([]string, error) {
	m := make(map[string]bool)

	for _, nameOrId := range namesOrIds {
		policy := new(ingressv1alpha1.IPPolicy)
		if err := r.client.Get(ctx, types.NamespacedName{Name: nameOrId, Namespace: namespace}, policy); err != nil {
			if client.IgnoreNotFound(err) == nil {
				m[nameOrId] = true // assume it's an ID
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
