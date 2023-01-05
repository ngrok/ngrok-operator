package controllers

import (
	"context"
	"fmt"
	"reflect"
	"strings"

	"github.com/go-logr/logr"
	ingressv1alpha1 "github.com/ngrok/ngrok-ingress-controller/api/v1alpha1"
	"github.com/ngrok/ngrok-ingress-controller/pkg/ngrokapidriver"
	v1 "k8s.io/api/core/v1"
	netv1 "k8s.io/api/networking/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
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
	if err := validateIngress(ctx, ingress); err != nil {
		irec.Recorder.Event(ingress, v1.EventTypeWarning, "Invalid ingress, discarding the event.", err.Error())
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
	err := irec.reconcileDomains(ctx, ingress)
	if err != nil {
		irec.Recorder.Event(ingress, v1.EventTypeWarning, "Failed to reconcile reserved domains", err.Error())
		return ctrl.Result{}, err
	}

	err = irec.reconcileTunnels(ctx, ingress)
	if err != nil {
		irec.Recorder.Event(ingress, v1.EventTypeWarning, "Failed to reconcile tunnels", err.Error())
		return ctrl.Result{}, err
	}

	ngrokEdge, err := irec.NgrokAPIDriver.UpdateEdge(ctx, edge)
	if err != nil {
		irec.Recorder.Event(ingress, v1.EventTypeWarning, "Failed to update edge", err.Error())
		return ctrl.Result{}, err
	}
	return ctrl.Result{}, setEdgeId(ctx, irec, ingress, ngrokEdge)
}

func (irec *IngressReconciler) CreateIngress(ctx context.Context, edge *ngrokapidriver.Edge, ingress *netv1.Ingress) (reconcile.Result, error) {
	err := irec.reconcileDomains(ctx, ingress)
	if err != nil {
		irec.Recorder.Event(ingress, v1.EventTypeWarning, "Failed to reconcile reserved domains", err.Error())
		return ctrl.Result{}, err
	}

	err = irec.reconcileTunnels(ctx, ingress)
	if err != nil {
		irec.Recorder.Event(ingress, v1.EventTypeWarning, "Failed to reconcile tunnels", err.Error())
		return ctrl.Result{}, err
	}

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
		WithEventFilter(commonPredicateFilters).
		Complete(irec)
}

// Converts a k8s Ingress Rule to and ngrok Route configuration.
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
			Labels:    backendToLabelMap(httpIngressPath.Backend, namespace),
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

// Converts a k8s ingress object into an ngrok Edge with all its configurations and sub-resources
// TODO: Support multiple Rules per Ingress
func (irec *IngressReconciler) ingressToEdge(ctx context.Context, ingress *netv1.Ingress) (*ngrokapidriver.Edge, error) {
	if ingress == nil {
		return nil, nil
	}

	// An ingress with no rules sends all traffic to a single default backend(.spec.defaultBackend)
	// and must be specified. TODO: Implement this.
	if len(ingress.Spec.Rules) == 0 {
		return nil, nil
	}

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

func (irec *IngressReconciler) reconcileDomains(ctx context.Context, ingress *netv1.Ingress) error {
	reservedDomains, err := irec.ingressToDomains(ctx, ingress)
	if err != nil {
		return err
	}

	for _, reservedDomain := range reservedDomains {
		if err := controllerutil.SetControllerReference(ingress, &reservedDomain, irec.Scheme); err != nil {
			return err
		}

		found := &ingressv1alpha1.Domain{}
		err := irec.Client.Get(ctx, types.NamespacedName{Name: reservedDomain.Name, Namespace: reservedDomain.Namespace}, found)
		if err != nil && errors.IsNotFound(err) {
			err = irec.Create(ctx, &reservedDomain)
			if err != nil {
				return err
			}
		} else if err != nil {
			return err
		}

		if !reflect.DeepEqual(reservedDomain.Spec, found.Spec) {
			found.Spec = reservedDomain.Spec
			err = irec.Update(ctx, found)
			if err != nil {
				return err
			}
		}
	}

	return nil
}

func (irec *IngressReconciler) reconcileTunnels(ctx context.Context, ingress *netv1.Ingress) error {
	tunnels := ingressToTunnels(ingress)

	for _, tunnel := range tunnels {
		if err := controllerutil.SetControllerReference(ingress, &tunnel, irec.Scheme); err != nil {
			return err
		}

		found := &ingressv1alpha1.Tunnel{}
		err := irec.Client.Get(ctx, types.NamespacedName{Name: tunnel.Name, Namespace: tunnel.Namespace}, found)
		if err != nil && errors.IsNotFound(err) {
			err = irec.Create(ctx, &tunnel)
			if err != nil {
				return err
			}
		} else if err != nil {
			return err
		}

		if !reflect.DeepEqual(tunnel.Spec, found.Spec) {
			found.Spec = tunnel.Spec
			err = irec.Update(ctx, found)
			if err != nil {
				return err
			}
		}
	}

	return nil
}

func (irec *IngressReconciler) ingressToDomains(ctx context.Context, ingress *netv1.Ingress) ([]ingressv1alpha1.Domain, error) {
	reservedDomains := make([]ingressv1alpha1.Domain, 0)

	if ingress == nil {
		return reservedDomains, nil
	}

	for _, rule := range ingress.Spec.Rules {
		if rule.Host == "" {
			continue
		}
		reservedDomains = append(reservedDomains, ingressv1alpha1.Domain{
			ObjectMeta: metav1.ObjectMeta{
				Name:      strings.Replace(rule.Host, ".", "-", -1),
				Namespace: ingress.Namespace,
			},
			Spec: ingressv1alpha1.DomainSpec{
				Domain: rule.Host,
			},
		})
	}

	return reservedDomains, nil
}

func ingressToTunnels(ingress *netv1.Ingress) []ingressv1alpha1.Tunnel {
	tunnels := make([]ingressv1alpha1.Tunnel, 0)

	if ingress == nil || len(ingress.Spec.Rules) == 0 {
		return tunnels
	}

	// Tunnels should be unique on a service and port basis so if they are referenced more than once, we
	// only create one tunnel per service and port.
	tunnelMap := make(map[string]ingressv1alpha1.Tunnel)
	for _, rule := range ingress.Spec.Rules {
		if rule.Host == "" {
			continue
		}

		for _, path := range rule.HTTP.Paths {
			serviceName := path.Backend.Service.Name
			servicePort := path.Backend.Service.Port.Number
			tunnelAddr := fmt.Sprintf("%s.%s.%s:%d", serviceName, ingress.Namespace, clusterDomain, servicePort)
			tunnelName := fmt.Sprintf("%s-%d", serviceName, servicePort)

			tunnelMap[tunnelName] = ingressv1alpha1.Tunnel{
				ObjectMeta: metav1.ObjectMeta{
					Name:      tunnelName,
					Namespace: ingress.Namespace,
				},
				Spec: ingressv1alpha1.TunnelSpec{
					ForwardsTo: tunnelAddr,
					Labels:     backendToLabelMap(path.Backend, ingress.Namespace),
				},
			}
		}
	}

	for _, tunnel := range tunnelMap {
		tunnels = append(tunnels, tunnel)
	}

	return tunnels
}
