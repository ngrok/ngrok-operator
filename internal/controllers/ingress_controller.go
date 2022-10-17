package controllers

import (
	"context"
	"fmt"

	"github.com/go-logr/logr"
	"github.com/ngrok/ngrok-ingress-controller/pkg/ngrokapidriver"
	v1 "k8s.io/api/core/v1"
	netv1 "k8s.io/api/networking/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

// This implements the Reconciler for the controller-runtime
// https://pkg.go.dev/sigs.k8s.io/controller-runtime#section-readme
type IngressReconciler struct {
	client.Client
	Log            logr.Logger
	Scheme         *runtime.Scheme
	Recorder       record.EventRecorder
	Namespace      string
	NgrokAPIDriver ngrokapidriver.NgrokAPIDriver
}

// +kubebuilder:rbac:groups="",resources=events,verbs=create;patch
// +kubebuilder:rbac:groups="networking.k8s.io",resources=ingresses,verbs=get;list;watch;update
// +kubebuilder:rbac:groups="networking.k8s.io",resources=ingresses/status,verbs=get;list;watch;update
// +kubebuilder:rbac:groups="networking.k8s.io",resources=ingressclasses,verbs=get;list;watch
// +kubebuilder:rbac:groups="",resources=configmaps,verbs=get;list;watch;create;update;delete
// +kubebuilder:rbac:groups="",resources=secrets,verbs=get;list;watch

// This reconcile function is called by the controller-runtime manager.
// It is invoked whenever there is an event that occurs for a resource
// being watched (in our case, ingress objects). If you tail the controller
// logs and delete, update, edit ingress objects, you see the events come in.
func (irec *IngressReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := irec.Log.WithValues("ingress", req.NamespacedName)
	ctx = ctrl.LoggerInto(ctx, log)
	ingress, err := getIngress(ctx, irec.Client, req.NamespacedName)
	if err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}
	// getIngress didn't return the object, so we can't do anything with it
	if ingress == nil {
		return ctrl.Result{}, nil
	}

	err = setFinalizer(ctx, irec, ingress)
	if err != nil {
		irec.Recorder.Event(ingress, v1.EventTypeWarning, "Failed to set finalizer", err.Error())
		return ctrl.Result{}, err
	}

	edge, err := irec.ingressToEdge(ctx, ingress)
	if err != nil {
		irec.Recorder.Event(ingress, v1.EventTypeWarning, "Failed to convert ingress to edge", err.Error())
		return ctrl.Result{}, err
	}

	// The object is being deleted
	if !ingress.ObjectMeta.DeletionTimestamp.IsZero() && controllerutil.ContainsFinalizer(ingress, finalizerName) {
		return irec.DeleteIngress(ctx, edge, ingress)
	}
	// Else its being created or updated
	// Check for a saved edge-id to do a lookup instead of a create
	if ingress.ObjectMeta.Annotations["k8s.ngrok.com/edge-id"] != "" {
		foundEdge, err := irec.NgrokAPIDriver.FindEdge(ctx, ingress.ObjectMeta.Annotations["k8s.ngrok.com/edge-id"])
		if err != nil {
			return ctrl.Result{}, err
		}
		// If the edge isn't found, we need to create it
		if foundEdge == nil {
			return irec.CreateIngress(ctx, edge, ingress)
		}
		// Otherwise, we found the edge, so update it
		irec.Recorder.Event(ingress, v1.EventTypeWarning, "EdgeExists", "Edge already exists")
		return irec.UpdateIngress(ctx, edge, ingress)
	}
	// Otherwise, create it!
	return irec.CreateIngress(ctx, edge, ingress)
}

func (irec *IngressReconciler) DeleteIngress(ctx context.Context, edge *ngrokapidriver.Edge, ingress *netv1.Ingress) (reconcile.Result, error) {
	if err := irec.NgrokAPIDriver.DeleteEdge(ctx, edge); err != nil {
		irec.Recorder.Event(ingress, v1.EventTypeWarning, "Failed to delete edge", err.Error())
		return ctrl.Result{}, err
	}

	// remove the finalizer and let it be fully deleted
	controllerutil.RemoveFinalizer(ingress, finalizerName)
	return ctrl.Result{}, irec.Update(ctx, ingress)
}

func (irec *IngressReconciler) UpdateIngress(ctx context.Context, edge *ngrokapidriver.Edge, ingress *netv1.Ingress) (reconcile.Result, error) {
	ngrokEdge, err := irec.NgrokAPIDriver.UpdateEdge(ctx, edge)
	if err != nil {
		irec.Recorder.Event(ingress, v1.EventTypeWarning, "Failed to update edge", err.Error())
		return ctrl.Result{}, err
	}
	return ctrl.Result{}, setEdgeId(ctx, irec, ingress, ngrokEdge)
}

func (irec *IngressReconciler) CreateIngress(ctx context.Context, edge *ngrokapidriver.Edge, ingress *netv1.Ingress) (reconcile.Result, error) {
	ngrokEdge, err := irec.NgrokAPIDriver.CreateEdge(ctx, edge)
	if err != nil {
		irec.Recorder.Event(ingress, v1.EventTypeWarning, "Failed to create edge", err.Error())
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, setEdgeId(ctx, irec, ingress, ngrokEdge)
}

// Create a new controller using our reconciler and set it up with the manager
func (irec *IngressReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&netv1.Ingress{}).
		Complete(irec)
}

// Converts a k8s Ingress Rule to and Ngrok Route configuration.
func (irec *IngressReconciler) routesPlanner(ctx context.Context, rule netv1.IngressRuleValue, ingressName, namespace string, annotations map[string]string) ([]ngrokapidriver.Route, error) {
	var matchType string
	var ngrokRoutes []ngrokapidriver.Route

	for _, httpIngressPath := range rule.HTTP.Paths {
		switch *httpIngressPath.PathType {
		case netv1.PathTypePrefix:
			matchType = "path_prefix"
		case netv1.PathTypeExact:
			matchType = "exact_path"
		case netv1.PathTypeImplementationSpecific:
			matchType = "path_prefix" // Path Prefix seems like a sane default for most cases
		default:
			return nil, fmt.Errorf("unsupported path type: %v", httpIngressPath.PathType)
		}

		route := ngrokapidriver.Route{
			Match:     httpIngressPath.Path,
			MatchType: matchType,
			Labels:    backendToLabelMap(httpIngressPath.Backend, ingressName, namespace),
		}

		// TODO: This should be replaced, see TODO at top of route_modules.go
		if annotationsToCompression(annotations) {
			route.Compression = true
		}

		if oauth, err := irec.annotationsToOauth(ctx, annotations); err != nil {
			return nil, fmt.Errorf("error configuriong OAuth: %q", err)
		} else if oauth != nil {
			route.GoogleOAuth = *oauth
		}

		ngrokRoutes = append(ngrokRoutes, route)
	}

	return ngrokRoutes, nil
}

// Converts a k8s ingress object into an Ngrok Edge with all its configurations and sub-resources
// TODO: Support multiple Rules per Ingress
func (irec *IngressReconciler) ingressToEdge(ctx context.Context, ingress *netv1.Ingress) (*ngrokapidriver.Edge, error) {
	annotations := ingress.ObjectMeta.GetAnnotations()
	ingressRule := ingress.Spec.Rules[0]

	ngrokRoutes, err := irec.routesPlanner(ctx, ingressRule.IngressRuleValue, ingress.Name, ingress.Namespace, annotations)
	if err != nil {
		return nil, err
	}

	return &ngrokapidriver.Edge{
		Id: ingress.Annotations["k8s.ngrok.com/edge-id"],
		// TODO: Support multiple rules
		Hostport: ingress.Spec.Rules[0].Host + ":443",
		Routes:   ngrokRoutes,
	}, err
}
