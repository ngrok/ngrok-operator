// Package kubernetesoperator holds helpers for reading the operator's own
// KubernetesOperator resource from components other than its reconciler.
package kubernetesoperator

import (
	"context"

	ngrokv1 "github.com/ngrok/ngrok-operator/api/ngrok/v1"
	ngrokv1alpha1 "github.com/ngrok/ngrok-operator/api/ngrok/v1alpha1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// Lookup returns the KubernetesOperator at key.
//
// The ngrok.com/v1 object is authoritative once it is registered (has a
// status.id) or has started draining. Until then, the legacy
// ngrok.k8s.ngrok.com/v1alpha1 object is returned, converted to v1, when one
// exists: api-manager creates the v1 object
// at startup with an empty status, and readers in other pods would otherwise
// see no bindings endpoint and no drain state during that window. When neither
// is usable, the v1 result is returned as-is (the object, or its NotFound
// error).
func Lookup(ctx context.Context, c client.Reader, key client.ObjectKey) (*ngrokv1.KubernetesOperator, error) {
	ko := &ngrokv1.KubernetesOperator{}
	err := c.Get(ctx, key, ko)
	if err != nil && !apierrors.IsNotFound(err) {
		return nil, err
	}
	// LEGACY-kubernetesoperator-kind (read-side cleanup): collapse Lookup to a
	// plain Get of the v1 object.
	if err == nil && isAuthoritative(ko) {
		return ko, nil
	}

	// LEGACY-kubernetesoperator-kind: BEGIN — fall back to the legacy object
	// until the v1 object registers; drop at cleanup.
	legacy := &ngrokv1alpha1.KubernetesOperator{}
	legacyErr := c.Get(ctx, key, legacy)
	if legacyErr == nil {
		return fromLegacy(legacy), nil
	}
	if !apierrors.IsNotFound(legacyErr) && !meta.IsNoMatchError(legacyErr) {
		return nil, legacyErr
	}
	// LEGACY-kubernetesoperator-kind: END

	if err != nil {
		return nil, err
	}
	return ko, nil
}

// LEGACY-kubernetesoperator-kind: BEGIN — drop at cleanup.

// isAuthoritative reports whether the v1 object should be used over a legacy
// one: once it is registered, or once it has started draining (a drain of an
// object that never registered must still be visible to readers).
func isAuthoritative(ko *ngrokv1.KubernetesOperator) bool {
	return ko.Status.ID != "" ||
		meta.FindStatusCondition(ko.Status.Conditions, ngrokv1.KubernetesOperatorConditionDraining) != nil
}

// fromLegacy copies the fields readers use from a legacy KubernetesOperator.
func fromLegacy(legacy *ngrokv1alpha1.KubernetesOperator) *ngrokv1.KubernetesOperator {
	ko := &ngrokv1.KubernetesOperator{
		ObjectMeta: *legacy.ObjectMeta.DeepCopy(),
		Status: ngrokv1.KubernetesOperatorStatus{
			ID:                      legacy.Status.ID,
			Conditions:              legacy.Status.DeepCopy().Conditions,
			BindingsIngressEndpoint: legacy.Status.BindingsIngressEndpoint,
		},
	}
	if b := legacy.Spec.Binding; b != nil {
		ko.Spec.Binding = &ngrokv1.KubernetesOperatorBinding{
			EndpointSelectors: append([]string(nil), b.EndpointSelectors...),
			IngressEndpoint:   b.IngressEndpoint,
			TlsSecretName:     b.TlsSecretName,
		}
	}
	return ko
}

// LEGACY-kubernetesoperator-kind: END
