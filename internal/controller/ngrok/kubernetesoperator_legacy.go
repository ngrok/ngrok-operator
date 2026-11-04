package ngrok

import (
	"context"
	"fmt"
	"time"

	"github.com/ngrok/ngrok-api-go/v9"
	ngrokv1 "github.com/ngrok/ngrok-operator/api/ngrok/v1"
	ngrokv1alpha1 "github.com/ngrok/ngrok-operator/api/ngrok/v1alpha1"
	"github.com/ngrok/ngrok-operator/internal/util"
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

	// ReasonLegacyTwinSkippedDrain is the event reason emitted when the v1
	// KubernetesOperator is deleted while its legacy twin still exists, and the
	// drain is skipped.
	ReasonLegacyTwinSkippedDrain = "LegacyTwinSkippedDrain"

	// legacyReleaseRetry is how often a deleted legacy object is rechecked
	// while the v1 object has not yet recorded its ID.
	legacyReleaseRetry = 2 * time.Second
)

// reconcileLegacy keeps the legacy ngrok.k8s.ngrok.com/v1alpha1 object at the
// same key passive while the v1 object is the writer. It never calls the ngrok
// API and never deletes the legacy object: a helm rollback hands the cluster
// back to an older operator that relies on it, finalizer and ID intact.
//   - A live legacy object is marked superseded (Ready=False, SupersededByV1).
//     No warning event: the operator, not the user, owns this object, so there
//     is nothing for anyone to act on.
//   - A deleted legacy object has its finalizer released once the v1 object
//     holds the ID (requeueing until it does), so "apply v1, then delete
//     legacy" never hangs and never drains.
func (r *KubernetesOperatorReconciler) reconcileLegacy(ctx context.Context, canonical *ngrokv1.KubernetesOperator) (ctrl.Result, error) {
	legacy := &ngrokv1alpha1.KubernetesOperator{}
	if err := r.Client.Get(ctx, client.ObjectKeyFromObject(canonical), legacy); err != nil {
		if apierrors.IsNotFound(err) || meta.IsNoMatchError(err) {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}

	if !legacy.DeletionTimestamp.IsZero() {
		if !util.HasFinalizer(legacy) {
			return ctrl.Result{}, nil
		}
		// The cached v1 object can lag the status write that adopted the ID,
		// and that write does not re-trigger this reconcile, so poll for it.
		if canonical.Status.ID == "" {
			return ctrl.Result{RequeueAfter: legacyReleaseRetry}, nil
		}
		original := legacy.DeepCopy()
		util.RemoveFinalizer(legacy)
		if err := r.Client.Patch(ctx, legacy, client.MergeFrom(original)); err != nil {
			return ctrl.Result{}, client.IgnoreNotFound(err)
		}
		ctrl.LoggerFrom(ctx).Info("released the finalizer of the deleted legacy KubernetesOperator; the v1 object holds the ID",
			"legacy", client.ObjectKeyFromObject(legacy).String(), "id", canonical.Status.ID)
		return ctrl.Result{}, nil
	}

	ready := meta.FindStatusCondition(legacy.Status.Conditions, ngrokv1alpha1.KubernetesOperatorConditionReady)
	if ready != nil && ready.Status == metav1.ConditionFalse && ready.Reason == ReasonSupersededByV1 {
		return ctrl.Result{}, nil
	}
	supersededBy := "kubernetesoperators.ngrok.com/" + canonical.Name
	meta.SetStatusCondition(&legacy.Status.Conditions, metav1.Condition{
		Type:               ngrokv1alpha1.KubernetesOperatorConditionReady,
		Status:             metav1.ConditionFalse,
		Reason:             ReasonSupersededByV1,
		Message:            "Superseded by " + supersededBy + "; this object is no longer reconciled",
		ObservedGeneration: legacy.Generation,
	})
	return ctrl.Result{}, client.IgnoreNotFound(r.Client.Status().Update(ctx, legacy))
}

// legacyTwinRegistration returns the ngrok KubernetesOperator registered by
// the legacy twin, so the v1 object adopts that exact ID rather than whichever
// registration findExisting matches first. It returns nil when there is no
// legacy twin with an ID, or when that ID no longer exists in the ngrok API.
func (r *KubernetesOperatorReconciler) legacyTwinRegistration(ctx context.Context, ko *ngrokv1.KubernetesOperator) (*ngrok.KubernetesOperator, error) {
	legacy := &ngrokv1alpha1.KubernetesOperator{}
	if err := r.Client.Get(ctx, client.ObjectKeyFromObject(ko), legacy); err != nil {
		if apierrors.IsNotFound(err) || meta.IsNoMatchError(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("getting legacy KubernetesOperator: %w", err)
	}
	if legacy.Status.ID == "" {
		return nil, nil
	}

	k8sOp, err := r.NgrokClientset.KubernetesOperators().Get(ctx, legacy.Status.ID)
	if ngrok.IsNotFound(err) {
		return nil, nil
	}
	return k8sOp, err
}

// legacyKubernetesOperatorToRequest maps events on the legacy kind onto the
// v1 object with the same namespace and name.
func legacyKubernetesOperatorToRequest(_ context.Context, obj client.Object) []reconcile.Request {
	return []reconcile.Request{{NamespacedName: client.ObjectKeyFromObject(obj)}}
}

// shouldSkipDrain reports whether deleting the v1 KubernetesOperator must not
// drain. While a legacy twin with the same name exists, the deletion is not an
// uninstall: the uninstall hook deletes the legacy object first, so a real
// uninstall finds no twin. The keep annotation stops helm rollback from
// deleting the v1 object, so this is a safety net for a hand deletion or a
// CRD removed some other way, after which an older operator reconciles the
// legacy object. A drain that has already started always continues. The read
// is uncached so a stale cache cannot skip a real uninstall's drain.
func (r *KubernetesOperatorReconciler) shouldSkipDrain(ctx context.Context, ko *ngrokv1.KubernetesOperator) (bool, error) {
	if meta.IsStatusConditionTrue(ko.Status.Conditions, ngrokv1.KubernetesOperatorConditionDraining) {
		return false, nil
	}

	err := r.APIReader.Get(ctx, client.ObjectKeyFromObject(ko), &ngrokv1alpha1.KubernetesOperator{})
	if apierrors.IsForbidden(err) {
		ctrl.LoggerFrom(ctx).Info("cannot read the legacy KubernetesOperator; skipping the drain to be safe", "error", err.Error())
	}
	return legacyTwinReadSkipsDrain(err)
}

// legacyTwinReadSkipsDrain interprets the read of the legacy twin, failing
// closed (twin rule 5): a twin that exists or cannot be read skips the drain,
// a twin that is absent or not served drains, and any other error requeues.
func legacyTwinReadSkipsDrain(err error) (bool, error) {
	switch {
	case err == nil, apierrors.IsForbidden(err):
		return true, nil
	case apierrors.IsNotFound(err), meta.IsNoMatchError(err):
		return false, nil
	default:
		return false, fmt.Errorf("getting legacy KubernetesOperator: %w", err)
	}
}
