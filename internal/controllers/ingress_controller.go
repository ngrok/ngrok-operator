package controllers

import (
	"context"

	"github.com/go-logr/logr"
	ingressv1alpha1 "github.com/ngrok/kubernetes-ingress-controller/api/v1alpha1"
	"github.com/ngrok/kubernetes-ingress-controller/internal/annotations"
	"github.com/ngrok/kubernetes-ingress-controller/internal/store"
	corev1 "k8s.io/api/core/v1"
	netv1 "k8s.io/api/networking/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/source"
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

func (irec *IngressReconciler) SetupWithManager(mgr ctrl.Manager) error {
	storedResources := []client.Object{
		&netv1.IngressClass{},
		&corev1.Service{},
		&ingressv1alpha1.Domain{},
		&ingressv1alpha1.HTTPSEdge{},
		&ingressv1alpha1.Tunnel{},
		&ingressv1alpha1.NgrokModuleSet{},
	}

	builder := ctrl.NewControllerManagedBy(mgr).For(&netv1.Ingress{})
	for _, obj := range storedResources {
		builder = builder.Watches(
			&source.Kind{Type: obj},
			store.NewUpdateStoreHandler(obj.GetObjectKind().GroupVersionKind().Kind, irec.Driver))
	}

	return builder.Complete(irec)
}

// +kubebuilder:rbac:groups="",resources=events,verbs=create;patch
// +kubebuilder:rbac:groups="",resources=configmaps,verbs=get;list;watch;create;update;delete
// +kubebuilder:rbac:groups="",resources=secrets,verbs=get;list;watch
// +kubebuilder:rbac:groups="",resources=services,verbs=get;list;watch
// +kubebuilder:rbac:groups="networking.k8s.io",resources=ingresses,verbs=get;list;watch;update
// +kubebuilder:rbac:groups="networking.k8s.io",resources=ingresses/status,verbs=get;list;watch;update
// +kubebuilder:rbac:groups="networking.k8s.io",resources=ingressclasses,verbs=get;list;watch
// +kubebuilder:rbac:groups=ingress.k8s.ngrok.com,resources=ngrokmodulesets,verbs=get;list;watch

// This reconcile function is called by the controller-runtime manager.
// It is invoked whenever there is an event that occurs for a resource
// being watched (in our case, ingress objects). If you tail the controller
// logs and delete, update, edit ingress objects, you see the events come in.
func (irec *IngressReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := irec.Log.WithValues("ingress", req.NamespacedName)
	ctx = ctrl.LoggerInto(ctx, log)

	ingress := &netv1.Ingress{}
	err := irec.Client.Get(ctx, req.NamespacedName, ingress)
	if err != nil {
		if client.IgnoreNotFound(err) != nil {
			return ctrl.Result{}, err // its a real error
		}
		// Otherwise its a not found errors. If its fully gone, delete it from the store
		if err := irec.Driver.DeleteNamedIngress(req.NamespacedName); err != nil {
			log.Error(err, "Failed to delete ingress from store")
			return ctrl.Result{}, err
		}

		err = irec.Driver.Sync(ctx, irec.Client)
		if err != nil {
			log.Error(err, "Failed to sync after removing ingress from store")
			return ctrl.Result{}, err
		}

		return ctrl.Result{}, nil
	}

	// Ensure the ingress object is up to date in the store
	if ok, err := irec.Driver.UpdateIngress(ingress); err != nil {
		return ctrl.Result{}, err
	} else if !ok {
		return ctrl.Result{}, nil
	}

	if ingress.ObjectMeta.DeletionTimestamp.IsZero() {
		// The object is not being deleted, so register and sync finalizer
		if err := registerAndSyncFinalizer(ctx, irec.Client, ingress); err != nil {
			log.Error(err, "Failed to register finalizer")
			return ctrl.Result{}, err
		}
	} else {
		if hasFinalizer(ingress) {
			log.Info("Deleting ingress from store")
			if err := removeAndSyncFinalizer(ctx, irec.Client, ingress); err != nil {
				log.Error(err, "Failed to remove finalizer")
				return ctrl.Result{}, err
			}
		}

		// Stop reconciliation as the item is being deleted and remove it from the store
		return ctrl.Result{}, irec.Driver.DeleteIngress(ingress)
	}

	err = irec.Driver.Sync(ctx, irec.Client)
	if err != nil {
		log.Error(err, "Failed to sync ingress to store")
	}
	return ctrl.Result{}, err
}
