package ngrok

import (
	"context"
	"fmt"

	ngrokv1 "github.com/ngrok/ngrok-operator/api/ngrok/v1"
	ngrokv1alpha1 "github.com/ngrok/ngrok-operator/api/ngrok/v1alpha1"
	"github.com/ngrok/ngrok-operator/internal/util"
	v1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

// LEGACY-kubernetesoperator-kind: delete this whole file at cleanup, together
// with the legacy Watches and the reconcileLegacy call in
// kubernetesoperator_controller.go.

const (
	// ReasonSupersededByV1 is the Ready condition reason set on a legacy
	// KubernetesOperator whose ngrok.com/v1 twin the operator now reconciles.
	ReasonSupersededByV1 = "SupersededByV1"

	// ReasonDeprecatedAPIGroup is the warning event reason emitted on a legacy
	// KubernetesOperator when it is first marked superseded.
	ReasonDeprecatedAPIGroup = "DeprecatedAPIGroup"

	// ReasonLegacyTwinSkippedDrain is the event reason emitted when the v1
	// KubernetesOperator is deleted while its legacy twin still exists, and the
	// drain is skipped.
	ReasonLegacyTwinSkippedDrain = "LegacyTwinSkippedDrain"
)

// reconcileLegacy keeps the legacy ngrok.k8s.ngrok.com/v1alpha1 object at the
// same key passive while the v1 object is the writer. It never calls the ngrok
// API and never deletes the legacy object: a helm rollback hands the cluster
// back to an older operator that relies on it, finalizer and ID intact.
//   - A live legacy object is marked superseded (Ready=False, SupersededByV1).
//   - A deleted legacy object has its finalizer released once the v1 object
//     holds the ID, so "apply v1, then delete legacy" never hangs and never
//     drains.
func (r *KubernetesOperatorReconciler) reconcileLegacy(ctx context.Context, canonical *ngrokv1.KubernetesOperator) error {
	legacy := &ngrokv1alpha1.KubernetesOperator{}
	if err := r.Client.Get(ctx, client.ObjectKeyFromObject(canonical), legacy); err != nil {
		if apierrors.IsNotFound(err) || meta.IsNoMatchError(err) {
			return nil
		}
		return err
	}

	if !legacy.DeletionTimestamp.IsZero() {
		if canonical.Status.ID == "" || !util.HasFinalizer(legacy) {
			return nil
		}
		original := legacy.DeepCopy()
		util.RemoveFinalizer(legacy)
		if err := r.Client.Patch(ctx, legacy, client.MergeFrom(original)); err != nil {
			return client.IgnoreNotFound(err)
		}
		ctrl.LoggerFrom(ctx).Info("released the finalizer of the deleted legacy KubernetesOperator; the v1 object holds the ID",
			"legacy", client.ObjectKeyFromObject(legacy).String(), "id", canonical.Status.ID)
		return nil
	}

	ready := meta.FindStatusCondition(legacy.Status.Conditions, ngrokv1alpha1.KubernetesOperatorConditionReady)
	if ready != nil && ready.Status == metav1.ConditionFalse && ready.Reason == ReasonSupersededByV1 {
		return nil
	}
	supersededBy := "kubernetesoperators.ngrok.com/" + canonical.Name
	meta.SetStatusCondition(&legacy.Status.Conditions, metav1.Condition{
		Type:               ngrokv1alpha1.KubernetesOperatorConditionReady,
		Status:             metav1.ConditionFalse,
		Reason:             ReasonSupersededByV1,
		Message:            "Superseded by " + supersededBy + "; this object is no longer reconciled",
		ObservedGeneration: legacy.Generation,
	})
	if err := r.Client.Status().Update(ctx, legacy); err != nil {
		return client.IgnoreNotFound(err)
	}
	r.Recorder.Eventf(legacy, canonical, v1.EventTypeWarning, ReasonDeprecatedAPIGroup, "Supersede",
		"ngrok.k8s.ngrok.com/v1alpha1 KubernetesOperator is deprecated; the operator now manages %s. No action is needed", supersededBy)
	return nil
}

// legacyKubernetesOperatorToRequest maps events on the legacy kind onto the
// v1 object with the same namespace and name.
func legacyKubernetesOperatorToRequest(_ context.Context, obj client.Object) []reconcile.Request {
	return []reconcile.Request{{NamespacedName: client.ObjectKeyFromObject(obj)}}
}

// shouldSkipDrain reports whether deleting the v1 KubernetesOperator must not
// drain. While a legacy twin with the same name exists, the deletion is not an
// uninstall: a helm rollback deletes the v1 CRD (and with it the v1 object)
// and hands the cluster back to an older operator that reconciles the legacy
// object. The uninstall hook deletes the legacy object first, so a real
// uninstall finds no twin. When the answer is uncertain (the legacy object
// cannot be read), the drain is skipped. A drain that has already started
// always continues. The read is uncached so a stale cache cannot skip a real
// uninstall's drain.
func (r *KubernetesOperatorReconciler) shouldSkipDrain(ctx context.Context, ko *ngrokv1.KubernetesOperator) (bool, error) {
	if meta.IsStatusConditionTrue(ko.Status.Conditions, ngrokv1.KubernetesOperatorConditionDraining) {
		return false, nil
	}

	err := r.APIReader.Get(ctx, client.ObjectKeyFromObject(ko), &ngrokv1alpha1.KubernetesOperator{})
	switch {
	case err == nil:
		return true, nil
	case apierrors.IsNotFound(err), meta.IsNoMatchError(err):
		return false, nil
	case apierrors.IsForbidden(err):
		ctrl.LoggerFrom(ctx).Info("cannot read the legacy KubernetesOperator; skipping the drain to be safe", "error", err.Error())
		return true, nil
	default:
		return false, fmt.Errorf("getting legacy KubernetesOperator: %w", err)
	}
}
