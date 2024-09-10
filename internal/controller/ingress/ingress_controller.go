package controllers

import (
	"context"

	"github.com/go-logr/logr"
	ingressv1alpha1 "github.com/ngrok/ngrok-operator/api/ingress/v1alpha1"
	ngrokv1alpha1 "github.com/ngrok/ngrok-operator/api/ngrok/v1alpha1"
	"github.com/ngrok/ngrok-operator/internal/annotations"
	"github.com/ngrok/ngrok-operator/internal/controller/controllers"
	internalerrors "github.com/ngrok/ngrok-operator/internal/errors"
	"github.com/ngrok/ngrok-operator/internal/store"
	corev1 "k8s.io/api/core/v1"
	netv1 "k8s.io/api/networking/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// This implements the Reconciler for the controller-runtime
// https://pkg.go.dev/sigs.k8s.io/controller-runtime#section-readme
type IngressReconciler struct {
	client.Client
	Log                  logr.Logger
	Scheme               *runtime.Scheme
	Recorder             record.EventRecorder
	Namespace            string
	AnnotationsExtractor annotations.Extractor
	Driver               *store.Driver
}

func (r *IngressReconciler) SetupWithManager(mgr ctrl.Manager) error {
	storedResources := []client.Object{
		&netv1.IngressClass{},
		&corev1.Service{},
		&ingressv1alpha1.Domain{},
		&ingressv1alpha1.HTTPSEdge{},
		&ingressv1alpha1.Tunnel{},
		&ingressv1alpha1.NgrokModuleSet{},
		&ngrokv1alpha1.NgrokTrafficPolicy{},
	}

	builder := ctrl.NewControllerManagedBy(mgr).For(&netv1.Ingress{})
	for _, obj := range storedResources {
		builder = builder.Watches(
			obj,
			store.NewUpdateStoreHandler(obj.GetObjectKind().GroupVersionKind().Kind, r.Driver, r.Client))
	}

	return builder.Complete(r)
}

// +kubebuilder:rbac:groups="",resources=events,verbs=create;patch
// +kubebuilder:rbac:groups="",resources=configmaps,verbs=get;list;watch;create;update;delete
// +kubebuilder:rbac:groups="",resources=secrets,verbs=get;list;watch
// +kubebuilder:rbac:groups="",resources=services,verbs=get;list;watch
// +kubebuilder:rbac:groups="networking.k8s.io",resources=ingresses,verbs=get;list;watch;update
// +kubebuilder:rbac:groups="networking.k8s.io",resources=ingresses/status,verbs=get;list;watch;update
// +kubebuilder:rbac:groups="networking.k8s.io",resources=ingressclasses,verbs=get;list;watch
// +kubebuilder:rbac:groups=ingress.k8s.ngrok.com,resources=ngrokmodulesets,verbs=get;list;watch
// +kubebuilder:rbac:groups=ngrok.k8s.ngrok.com,resources=ngroktrafficpolicies,verbs=get;list;watch

// This reconcile function is called by the controller-runtime manager.
// It is invoked whenever there is an event that occurs for a resource
// being watched (in our case, ingress objects). If you tail the controller
// logs and delete, update, edit ingress objects, you see the events come in.
func (r *IngressReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := r.Log.WithValues("ingress", req.NamespacedName)
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

		err = r.Driver.Sync(ctx, r.Client)
		if err != nil {
			log.Error(err, "Failed to sync after removing ingress from store")
			return ctrl.Result{}, err
		}

		return ctrl.Result{}, nil
	default:
		return ctrl.Result{}, err
	}

	// Ensure the ingress object is up to date in the store
	// Leverage the store to ensure this works off the same data as everything else
	ingress, err = r.Driver.UpdateIngress(ingress)
	switch {
	case err == nil:
		// all good, continue
	case internalerrors.IsErrDifferentIngressClass(err):
		log.Info("Ingress is not of type ngrok so skipping it")
		return ctrl.Result{}, nil
	case internalerrors.IsErrInvalidIngressSpec(err):
		log.Info("Ingress is not valid so skipping it")
		return ctrl.Result{}, nil
	default:
		log.Error(err, "Failed to get ingress from store")
		return ctrl.Result{}, err
	}

	if controllers.IsUpsert(ingress) {
		// The object is not being deleted, so register and sync finalizer
		if err := controllers.RegisterAndSyncFinalizer(ctx, r.Client, ingress); err != nil {
			log.Error(err, "Failed to register finalizer")
			return ctrl.Result{}, err
		}
	} else {
		log.Info("Deleting ingress from store")
		if controllers.HasFinalizer(ingress) {
			if err := controllers.RemoveAndSyncFinalizer(ctx, r.Client, ingress); err != nil {
				log.Error(err, "Failed to remove finalizer")
				return ctrl.Result{}, err
			}
		}

		// Remove it from the store
		if err := r.Driver.DeleteIngress(ingress); err != nil {
			return ctrl.Result{}, err
		}
	}

	err = r.Driver.Sync(ctx, r.Client)
	if err != nil {
		log.Error(err, "Failed to sync")
	}

	return ctrl.Result{}, err
}
