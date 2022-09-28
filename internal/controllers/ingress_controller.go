package controllers

import (
	"context"
	"fmt"

	"github.com/go-logr/logr"
	"github.com/ngrok/ngrok-api-go/v4"
	"github.com/ngrok/ngrok-ingress-controller/pkg/ngrokapidriver"
	v1 "k8s.io/api/core/v1"
	netv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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

	edge, err := IngressToEdge(ctx, ingress)
	if err != nil {
		irec.Recorder.Event(ingress, v1.EventTypeWarning, "Failed to convert ingress to edge", err.Error())
		return ctrl.Result{}, err
	}

	// The object is being deleted
	if !ingress.ObjectMeta.DeletionTimestamp.IsZero() && controllerutil.ContainsFinalizer(ingress, finalizerName) {
		return irec.DeleteIngress(ctx, *edge, ingress)
	}
	// Else its being created or updated
	// Check for a saved edge-id to do a lookup instead of a create
	if ingress.ObjectMeta.Annotations["k8s.ngrok.com/edge-id"] != "" {
		_, err := irec.NgrokAPIDriver.FindEdge(ctx, ingress.ObjectMeta.Annotations["k8s.ngrok.com/edge-id"])
		if err == nil {
			irec.Recorder.Event(ingress, v1.EventTypeWarning, "EdgeExists", "Edge already exists")
			// TODO: Provide update functionality. Right now, its create/delete
			return irec.UpdateIngress(ctx, *edge, ingress)
		}
		if !ngrok.IsNotFound(err) {
			irec.Recorder.Event(ingress, v1.EventTypeWarning, "Failed to find edge", err.Error())
			return ctrl.Result{}, err
		}
	}
	// Otherwise, create it!
	return irec.CreateIngress(ctx, *edge, ingress)
}

func (irec *IngressReconciler) DeleteIngress(ctx context.Context, edge ngrokapidriver.Edge, ingress *netv1.Ingress) (reconcile.Result, error) {
	if err := irec.NgrokAPIDriver.DeleteEdge(ctx, edge); err != nil {
		irec.Recorder.Event(ingress, v1.EventTypeWarning, "Failed to delete edge", err.Error())
		return ctrl.Result{}, err
	}

	// remove the finalizer and let it be fully deleted
	controllerutil.RemoveFinalizer(ingress, finalizerName)
	return ctrl.Result{}, irec.Update(ctx, ingress)
}

func (irec *IngressReconciler) UpdateIngress(ctx context.Context, edge ngrokapidriver.Edge, ingress *netv1.Ingress) (reconcile.Result, error) {
	// TODO: Provide update functionality. Right now, its create/delete
	// return ctrl.Result{}, ir.NgrokAPIDriver.UpdateEdge(ctx, foundEdge)
	return ctrl.Result{}, nil
}

func (irec *IngressReconciler) CreateIngress(ctx context.Context, edge ngrokapidriver.Edge, ingress *netv1.Ingress) (reconcile.Result, error) {
	ngrokEdge, err := irec.NgrokAPIDriver.CreateEdge(ctx, edge)
	if err != nil {
		irec.Recorder.Event(ingress, v1.EventTypeWarning, "Failed to create edge", err.Error())
		return ctrl.Result{}, err
	}

	irec.Recorder.Event(ingress, v1.EventTypeNormal, "CreatedEdge", "Created edge "+ngrokEdge.ID)
	ingress.ObjectMeta.Annotations["k8s.ngrok.com/edge-id"] = ngrokEdge.ID

	err = irec.Update(ctx, ingress)
	if err != nil {
		irec.Recorder.Event(ingress, v1.EventTypeWarning, "Failed to update ingress", err.Error())
		return ctrl.Result{}, err
	}
	err = setStatus(ctx, irec, ingress, ngrokEdge.ID)
	if err != nil {
		irec.Recorder.Event(ingress, v1.EventTypeWarning, "Failed to set status", err.Error())
		return ctrl.Result{}, err
	}
	return ctrl.Result{}, nil
}

// Create a new controller using our reconciler and set it up with the manager
func (irec *IngressReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&netv1.Ingress{}).
		Complete(irec)
}

// TODO: This may not actually be needed. Edges for example, don't have names
// that need to be unique. I think you just can't have multiple edges using the
// same hostports. Once I confirm this, we can probably get rid of this
//
// LogicalEdgeNamespace returns a string that can be used to namespace api
// resources in the ngrok api. The namespace would be used to control load balancing
// between clusters. This function should be only called by the leader to avoid multiple
// controllers attempting a read/write operation on the same config map without a lock.
func (irec *IngressReconciler) LogicalEdgeNamespace(ctx context.Context) (string, error) {
	configMapName := "ngrok-ingress-controller-edge-namespace"
	configMapKey := "edge-namespace"
	// TODO: This should be configurable by the user eventually or random. For now, be consistent for testing
	newName := "devenv-users"
	config := &v1.ConfigMap{}
	// Try to find the existing config map
	err := irec.Get(ctx, client.ObjectKey{Name: configMapName, Namespace: irec.Namespace}, config)
	if err == nil {
		if val, ok := config.Data[configMapKey]; ok {
			return val, nil
		} else {
			panic("Config map is missing the key " + configMapKey + " which shouldn't be possible")
		}
	}

	// Valid non "not found" error
	if client.IgnoreNotFound(err) != nil {
		return "", err
	}

	// If its not found, try to make it
	if err := irec.Create(ctx, &v1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      configMapName,
			Namespace: irec.Namespace,
		},
		Data: map[string]string{
			configMapKey: newName,
		},
	}); err != nil {
		return "", err
	}
	return newName, nil
}

// Converts a k8s Ingress Rule to and Ngrok Route configuration.
func routesPlanner(rule netv1.IngressRuleValue, ingressName, namespace string) ([]ngrokapidriver.Route, error) {
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
		ngrokRoutes = append(ngrokRoutes, ngrokapidriver.Route{
			Match:     httpIngressPath.Path,
			MatchType: matchType,
			Labels:    backendToLabelMap(httpIngressPath.Backend, ingressName, namespace),
		})
	}

	return ngrokRoutes, nil
}

// Converts a k8s ingress object into an Ngrok Edge with all its configurations and sub-resources
// TODO: Support multiple Rules per Ingress
func IngressToEdge(ctx context.Context, ingress *netv1.Ingress) (*ngrokapidriver.Edge, error) {
	ingressRule := ingress.Spec.Rules[0]
	ngrokRoutes, err := routesPlanner(ingressRule.IngressRuleValue, ingress.Name, ingress.Namespace)

	return &ngrokapidriver.Edge{
		Id: ingress.Annotations["k8s.ngrok.com/edge-id"],
		// TODO: Support multiple rules
		Hostport: ingress.Spec.Rules[0].Host + ":443",
		Labels: map[string]string{
			"k8s.ngrok.com/ingress-name":      ingress.Name,
			"k8s.ngrok.com/ingress-namespace": ingress.Namespace,
		},
		Routes: ngrokRoutes,
	}, err
}
