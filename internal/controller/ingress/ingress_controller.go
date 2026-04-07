package ingress

import (
	"context"

	"github.com/go-logr/logr"
	ingressv1alpha1 "github.com/ngrok/ngrok-operator/api/ingress/v1alpha1"
	ngrokv1alpha1 "github.com/ngrok/ngrok-operator/api/ngrok/v1alpha1"
	"github.com/ngrok/ngrok-operator/internal/controller"
	internalerrors "github.com/ngrok/ngrok-operator/internal/errors"
	"github.com/ngrok/ngrok-operator/internal/util"
	"github.com/ngrok/ngrok-operator/pkg/managerdriver"
	corev1 "k8s.io/api/core/v1"
	netv1 "k8s.io/api/networking/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/events"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// This implements the Reconciler for the controller-runtime
// https://pkg.go.dev/sigs.k8s.io/controller-runtime#section-readme
type IngressReconciler struct {
	client.Client
	Log       logr.Logger
	Scheme    *runtime.Scheme
	Recorder  events.EventRecorder
	Namespace string
	Driver    *managerdriver.Driver
	// DrainState is used to check if the operator is draining.
	// If draining, non-delete reconciles are skipped to prevent new finalizers.
	DrainState controller.DrainState
}

func (r *IngressReconciler) SetupWithManager(mgr ctrl.Manager) error {
	storedResources := []client.Object{
		&netv1.IngressClass{},
		&corev1.Service{},
		&ingressv1alpha1.Domain{},
		&ngrokv1alpha1.NgrokTrafficPolicy{},
	}

	builder := ctrl.NewControllerManagedBy(mgr).For(&netv1.Ingress{})
	for _, obj := range storedResources {
		builder = builder.Watches(
			obj,
			managerdriver.NewControllerEventHandler(obj.GetObjectKind().GroupVersionKind().Kind, r.Driver, r.Client))
	}

	return builder.Complete(r)
}

// This reconcile function is called by the controller-runtime manager.
// It is invoked whenever there is an event that occurs for a resource
// being watched (in our case, ingress objects). If you tail the controller
// logs and delete, update, edit ingress objects, you see the events come in.
func (r *IngressReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := ctrl.LoggerFrom(ctx).WithValues("ingress", req.NamespacedName)
	ctx = ctrl.LoggerInto(ctx, log)

	ingress := &netv1.Ingress{}
	err := r.Client.Get(ctx, req.NamespacedName, ingress)
	switch {
	case err == nil:
		// all good, continue
	case client.IgnoreNotFound(err) == nil:
		if err := r.Driver.DeleteNamedIngress(req.NamespacedName); err != nil {
			log.Error(err, "Failed to delete ingress from store")
			return ctrl.Result{}, err
		}

		return managerdriver.HandleSyncResult(r.Driver.Sync(ctx, r.Client))
	default:
		return ctrl.Result{}, err
	}

	// Store the originally found ingress separately to use later
	// incase there is an error updating and finding it below
	originalFoundIngress := ingress

	// Ensure the ingress object is up to date in the store
	// Leverage the store to ensure this works off the same data as everything else
	ingress, err = r.Driver.UpdateIngress(ingress)
	switch {
	case err == nil:
		// all good, continue
	case internalerrors.IsErrDifferentIngressClass(err):
		log.Info("Ingress is not of type ngrok so skipping it")
		return ctrl.Result{}, nil
	case internalerrors.IsErrorNoDefaultIngressClassFound(err):
		r.Recorder.Eventf(originalFoundIngress, nil, "Warning", "NoDefaultIngressClassFound", "Reconcile", "No ingress class found for this controller")
		return ctrl.Result{}, nil
	case internalerrors.IsErrInvalidIngressSpec(err):
		r.Recorder.Eventf(originalFoundIngress, nil, "Warning", "InvalidIngressSpec", "Reconcile", "Ingress is not valid so skipping it: %v", err)
		return ctrl.Result{}, nil
	default:
		r.Recorder.Eventf(originalFoundIngress, nil, "Warning", "FailedGetIngress", "Reconcile", "Failed to get ingress from store: %v", err)
		return ctrl.Result{}, err
	}

	// If being deleted, remove finalizer and delete from store
	if controller.IsDelete(ingress) {
		log.Info("Deleting ingress from store")
		if err := util.RemoveAndSyncFinalizer(ctx, r.Client, ingress); err != nil {
			log.Error(err, "Failed to remove finalizer")
			return ctrl.Result{}, err
		}

		// Remove it from the store
		if err := r.Driver.DeleteIngress(ingress); err != nil {
			return ctrl.Result{}, err
		}

		return managerdriver.HandleSyncResult(r.Driver.Sync(ctx, r.Client))
	}

	// Skip non-delete reconciles during drain to prevent adding new finalizers
	if controller.IsDraining(ctx, r.DrainState) {
		log.V(1).Info("Draining, skipping non-delete reconcile")
		// Remove from store so no new resources are created for this ingress
		if err := r.Driver.DeleteIngress(ingress); err != nil {
			log.Error(err, "Failed to delete ingress from store during drain")
			return ctrl.Result{}, err
		}
		return ctrl.Result{}, nil
	}

	// The object is not being deleted, so register and sync finalizer
	if err := util.RegisterAndSyncFinalizer(ctx, r.Client, ingress); err != nil {
		log.Error(err, "Failed to register finalizer")
		return ctrl.Result{}, err
	}

	return managerdriver.HandleSyncResult(r.Driver.Sync(ctx, r.Client))
}
