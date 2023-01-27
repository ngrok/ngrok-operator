package controllers

import (
	"context"
	"fmt"
	"reflect"
	"strings"
	"time"

	"github.com/go-logr/logr"
	ingressv1alpha1 "github.com/ngrok/kubernetes-ingress-controller/api/v1alpha1"
	"github.com/ngrok/kubernetes-ingress-controller/internal/annotations"
	internalerrors "github.com/ngrok/kubernetes-ingress-controller/internal/errors"
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
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
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
}

// Create a new controller using our reconciler and set it up with the manager
func (irec *IngressReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&netv1.Ingress{}).
		Owns(&ingressv1alpha1.HTTPSEdge{}).
		Owns(&ingressv1alpha1.Tunnel{}).
		Watches(
			&source.Kind{Type: &ingressv1alpha1.Domain{}},
			handler.EnqueueRequestsFromMapFunc(irec.listIngressesForDomain),
		).
		WithEventFilter(
			predicate.Funcs{
				DeleteFunc: deleteFuncPredicateFilter,
			},
		).
		Complete(irec)
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

	if ingress.ObjectMeta.DeletionTimestamp.IsZero() {
		// The object is not being deleted, so register and sync finalizer
		if err := registerAndSyncFinalizer(ctx, irec.Client, ingress); err != nil {
			log.Error(err, "Failed to register finalizer")
			return ctrl.Result{}, err
		}
	} else {
		// The object is being deleted
		if hasFinalizer(ingress) {
			log.Info("Deleting ingress")

			if err = irec.DeleteDependents(ctx, ingress); err != nil {
				return ctrl.Result{}, err
			}

			if err := removeAndSyncFinalizer(ctx, irec.Client, ingress); err != nil {
				log.Error(err, "Failed to remove finalizer")
				return ctrl.Result{}, err
			}
		}

		// Stop reconciliation as the item is being deleted
		return ctrl.Result{}, nil
	}

	return irec.reconcileAll(ctx, ingress)
}

func (irec *IngressReconciler) DeleteDependents(ctx context.Context, ingress *netv1.Ingress) error {
	// TODO: Currently this controller "owns" the HTTPSEdge and Tunnel objects so deleting an ingress
	// will delete the HTTPSEdge and Tunnel objects. Once multiple ingress objects combine to form 1 edge
	// this logic will need to be smarter
	return nil
}

func (irec *IngressReconciler) reconcileAll(ctx context.Context, ingress *netv1.Ingress) (reconcile.Result, error) {
	err := irec.reconcileDomains(ctx, ingress)
	if err != nil {
		if internalerrors.IsNotAllDomainsReadyYet(err) {
			irec.Recorder.Event(ingress, v1.EventTypeNormal, "Provisioning domains", "Waiting for domains to be ready")
			return ctrl.Result{RequeueAfter: 5 * time.Second}, nil
		}
		irec.Recorder.Event(ingress, v1.EventTypeWarning, "Failed to reconcile reserved domains", err.Error())
		return ctrl.Result{}, err
	}

	err = irec.reconcileTunnels(ctx, ingress)
	if err != nil {
		irec.Recorder.Event(ingress, v1.EventTypeWarning, "Failed to reconcile tunnels", err.Error())
		return ctrl.Result{}, err
	}

	err = irec.reconcileEdges(ctx, ingress)
	if err != nil {
		irec.Recorder.Event(ingress, v1.EventTypeWarning, "Failed to reconcile edges", err.Error())
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

// Converts a k8s Ingress Rule to and ngrok Route configuration.
func (irec *IngressReconciler) routesPlanner(ctx context.Context, ingress *netv1.Ingress, parsedRouteModules *annotations.RouteModules) ([]ingressv1alpha1.HTTPSEdgeRouteSpec, error) {
	namespace := ingress.Namespace
	rule := ingress.Spec.Rules[0]

	var matchType string
	var ngrokRoutes []ingressv1alpha1.HTTPSEdgeRouteSpec

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

		route := ingressv1alpha1.HTTPSEdgeRouteSpec{
			Match:     httpIngressPath.Path,
			MatchType: matchType,
			Backend: ingressv1alpha1.TunnelGroupBackend{
				Labels: backendToLabelMap(httpIngressPath.Backend, namespace),
			},
			Compression:   parsedRouteModules.Compression,
			IPRestriction: parsedRouteModules.IPRestriction,
			Headers:       parsedRouteModules.Headers,
		}

		ngrokRoutes = append(ngrokRoutes, route)
	}

	return ngrokRoutes, nil
}

// Converts a k8s ingress object into an ngrok Edge with all its configurations and sub-resources
// TODO: Support multiple Rules per Ingress
func (irec *IngressReconciler) ingressToEdge(ctx context.Context, ingress *netv1.Ingress) (*ingressv1alpha1.HTTPSEdge, error) {
	if ingress == nil {
		return nil, nil
	}

	// An ingress with no rules sends all traffic to a single default backend(.spec.defaultBackend)
	// and must be specified. TODO: Implement this.
	if len(ingress.Spec.Rules) == 0 {
		return nil, nil
	}

	parsedRouteModules := irec.AnnotationsExtractor.Extract(ingress)

	ngrokRoutes, err := irec.routesPlanner(ctx, ingress, parsedRouteModules)
	if err != nil {
		return nil, err
	}

	return &ingressv1alpha1.HTTPSEdge{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: ingress.Namespace,
			Name:      ingress.Name,
		},
		Spec: ingressv1alpha1.HTTPSEdgeSpec{
			Hostports:      []string{ingress.Spec.Rules[0].Host + ":443"},
			Routes:         ngrokRoutes,
			TLSTermination: parsedRouteModules.TLSTermination,
		},
	}, nil
}

func (irec *IngressReconciler) reconcileEdges(ctx context.Context, ingress *netv1.Ingress) error {
	edge, err := irec.ingressToEdge(ctx, ingress)
	if err != nil {
		return err
	}

	if err := controllerutil.SetControllerReference(ingress, edge, irec.Scheme); err != nil {
		return err
	}

	found := &ingressv1alpha1.HTTPSEdge{}
	err = irec.Client.Get(ctx, types.NamespacedName{Name: edge.Name, Namespace: edge.Namespace}, found)
	if err != nil && errors.IsNotFound(err) {
		err = irec.Create(ctx, edge)
		if err != nil {
			return err
		}
	} else if err != nil {
		return err
	}

	if !reflect.DeepEqual(edge.Spec, found.Spec) {
		found.Spec = edge.Spec
		err = irec.Update(ctx, found)
		if err != nil {
			return err
		}
	}

	return nil
}

func (irec *IngressReconciler) reconcileDomains(ctx context.Context, ingress *netv1.Ingress) error {
	reservedDomains, err := irec.ingressToDomains(ctx, ingress)
	if err != nil {
		return err
	}

	loadBalancerIngressStatuses := []netv1.IngressLoadBalancerIngress{}
	hasDomainsWithoutStatus := false

	for _, reservedDomain := range reservedDomains {
		found := &ingressv1alpha1.Domain{}
		err := irec.Client.Get(ctx, types.NamespacedName{Name: reservedDomain.Name, Namespace: reservedDomain.Namespace}, found)
		if err != nil {
			if !errors.IsNotFound(err) {
				return err
			}

			irec.Log.Info("Creating domain", "namespace", reservedDomain.Namespace, "name", reservedDomain.Name)
			err = irec.Create(ctx, &reservedDomain)
			if err != nil {
				return err
			}
		}

		if !reflect.DeepEqual(reservedDomain.Spec, found.Spec) {
			found.Spec = reservedDomain.Spec
			err = irec.Update(ctx, found)
			if err != nil {
				return err
			}
		}

		var loadBalancerHostname string
		if found.Status.CNAMETarget != nil {
			loadBalancerHostname = *found.Status.CNAMETarget
		} else if found.Status.Domain != "" {
			loadBalancerHostname = found.Status.Domain
		} else {
			hasDomainsWithoutStatus = true
		}

		loadBalancerIngressStatuses = append(loadBalancerIngressStatuses, netv1.IngressLoadBalancerIngress{
			Hostname: loadBalancerHostname,
		})
	}

	if hasDomainsWithoutStatus {
		return internalerrors.NewNotAllDomainsReadyYetError()
	}

	irec.Log.Info("Updating Ingress status with load balancer ingress statuses")
	ingress.Status.LoadBalancer.Ingress = loadBalancerIngressStatuses
	return irec.Status().Update(ctx, ingress)
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

// listIngressesForDomains returns a list of ingresses that reference the given domain.
func (irec *IngressReconciler) listIngressesForDomain(obj client.Object) []reconcile.Request {
	irec.Log.Info("Listing ingresses for domain to determine if they need to be reconciled")
	domain, ok := obj.(*ingressv1alpha1.Domain)
	if !ok {
		irec.Log.Error(nil, "failed to convert object to domain", "object", obj)
		return []reconcile.Request{}
	}

	ingresses := &netv1.IngressList{}
	if err := irec.Client.List(context.Background(), ingresses); err != nil {
		irec.Log.Error(err, "failed to list ingresses for domain", "domain", domain.Spec.Domain)
		return []reconcile.Request{}
	}

	recs := []reconcile.Request{}

	for _, ingress := range ingresses.Items {
		for _, rule := range ingress.Spec.Rules {
			if rule.Host == domain.Status.Domain {
				recs = append(recs, reconcile.Request{
					NamespacedName: types.NamespacedName{
						Name:      ingress.GetName(),
						Namespace: ingress.GetNamespace(),
					},
				})
				break
			}
		}
	}

	irec.Log.Info("Domain change triggered ingress reconciliation", "count", len(recs), "domain", domain.Spec.Domain)
	return recs
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
