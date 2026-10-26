package ngrok

import (
	"context"

	ngrokv1 "github.com/ngrok/ngrok-operator/api/ngrok/v1"
	ngrokv1alpha1 "github.com/ngrok/ngrok-operator/api/ngrok/v1alpha1"
	"github.com/ngrok/ngrok-operator/internal/util"
	v1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

// LEGACY-kubernetesoperator-kind: delete this whole file at cleanup, together
// with the legacy Watches and the neutralizeLegacy call in
// kubernetesoperator_controller.go.

const (
	// AnnotationSupersededBy marks a legacy KubernetesOperator that the operator
	// no longer reconciles, naming the ngrok.com/v1 object that replaced it.
	AnnotationSupersededBy = "ngrok.com/superseded-by"

	// ReasonLegacyKubernetesOperatorSuperseded is the event reason emitted on the
	// v1 KubernetesOperator when its legacy twin's finalizer is removed.
	ReasonLegacyKubernetesOperatorSuperseded = "LegacyKubernetesOperatorSuperseded"
)

// neutralizeLegacy makes the legacy ngrok.k8s.ngrok.com/v1alpha1 object at the
// same key inert: it removes the operator finalizer, so deleting it can never
// run a drain, and marks it superseded. It never deletes the object, because an
// older operator started by a helm rollback recreates and relies on it.
// Older operators re-add the finalizer whenever they reconcile, so this runs on
// every reconcile, triggered by a watch on the legacy kind.
func (r *KubernetesOperatorReconciler) neutralizeLegacy(ctx context.Context, canonical *ngrokv1.KubernetesOperator) error {
	if !canonical.DeletionTimestamp.IsZero() {
		return nil
	}

	legacy := &ngrokv1alpha1.KubernetesOperator{}
	if err := r.Client.Get(ctx, client.ObjectKeyFromObject(canonical), legacy); err != nil {
		if apierrors.IsNotFound(err) || meta.IsNoMatchError(err) {
			return nil
		}
		return err
	}

	supersededBy := "kubernetesoperators.ngrok.com/" + canonical.Name
	original := legacy.DeepCopy()
	removedFinalizer := util.RemoveFinalizer(legacy)
	if !removedFinalizer && legacy.Annotations[AnnotationSupersededBy] == supersededBy {
		return nil
	}
	if legacy.Annotations == nil {
		legacy.Annotations = map[string]string{}
	}
	legacy.Annotations[AnnotationSupersededBy] = supersededBy

	if err := r.Client.Patch(ctx, legacy, client.MergeFrom(original)); err != nil {
		return client.IgnoreNotFound(err)
	}

	if removedFinalizer {
		ctrl.LoggerFrom(ctx).Info("removed the finalizer from the legacy KubernetesOperator; it is no longer reconciled",
			"legacy", client.ObjectKeyFromObject(legacy).String())
		r.Recorder.Eventf(canonical, nil, v1.EventTypeNormal, ReasonLegacyKubernetesOperatorSuperseded, "Supersede",
			"Removed the finalizer from the legacy ngrok.k8s.ngrok.com/v1alpha1 KubernetesOperator; it is no longer reconciled")
	}
	return nil
}

// legacyKubernetesOperatorToRequest maps events on the legacy kind onto the
// v1 object with the same namespace and name.
func legacyKubernetesOperatorToRequest(_ context.Context, obj client.Object) []reconcile.Request {
	return []reconcile.Request{{NamespacedName: client.ObjectKeyFromObject(obj)}}
}
