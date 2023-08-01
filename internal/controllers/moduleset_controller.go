package controllers

import (
	"context"

	"github.com/go-logr/logr"
	ingressv1alpha1 "github.com/ngrok/kubernetes-ingress-controller/api/v1alpha1"
	"github.com/ngrok/kubernetes-ingress-controller/internal/store"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type ModuleSetReconciler struct {
	client.Client

	Log      logr.Logger
	Scheme   *runtime.Scheme
	Recorder record.EventRecorder
	Driver   *store.Driver
}

func (r *ModuleSetReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&ingressv1alpha1.NgrokModuleSet{}).
		WithEventFilter(commonPredicateFilters).
		Complete(r)
}

// +kubebuilder:rbac:groups=ingress.k8s.ngrok.com,resources=ngrokmodulesets,verbs=get;list;watch

// This reconcile function is called by the controller-runtime manager.
// It is invoked whenever there is an event that occurs for a resource
// being watched (in our case, NgrokModuleSets). If you tail the controller
// logs and delete, update, edit ingress objects, you see the events come in.
func (r *ModuleSetReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	err := r.Driver.SyncEdges(ctx, r.Client)
	return ctrl.Result{}, err
}
