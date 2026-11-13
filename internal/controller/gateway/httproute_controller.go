/*
MIT License

Copyright (c) 2022 ngrok, Inc.

Permission is hereby granted, free of charge, to any person obtaining a copy
of this software and associated documentation files (the "Software"), to deal
in the Software without restriction, including without limitation the rights
to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
copies of the Software, and to permit persons to whom the Software is
furnished to do so, subject to the following conditions:

The above copyright notice and this permission notice shall be included in all
copies or substantial portions of the Software.

THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
SOFTWARE.
*/

package gateway

import (
	"context"

	"github.com/go-logr/logr"
	ingressv1alpha1 "github.com/ngrok/ngrok-operator/api/ingress/v1alpha1"
	ngrokv1 "github.com/ngrok/ngrok-operator/api/ngrok/v1"
	ngrokv1alpha1 "github.com/ngrok/ngrok-operator/api/ngrok/v1alpha1"
	"github.com/ngrok/ngrok-operator/internal/controller"
	"github.com/ngrok/ngrok-operator/internal/util"
	"github.com/ngrok/ngrok-operator/pkg/managerdriver"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/events"
	"k8s.io/utils/ptr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"
)

// HTTPRouteReconciler reconciles a HTTPRoute object
type HTTPRouteReconciler struct {
	client.Client

	Log      logr.Logger
	Scheme   *runtime.Scheme
	Recorder events.EventRecorder
	Driver   *managerdriver.Driver
	// DrainState is used to check if the operator is draining.
	// If draining, non-delete reconciles are skipped to prevent new finalizers.
	DrainState controller.DrainState
}

func (r *HTTPRouteReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := ctrl.LoggerFrom(ctx).WithValues("HTTPRoute", req.NamespacedName)
	ctx = ctrl.LoggerInto(ctx, log)

	httproute := new(gatewayv1.HTTPRoute)
	err := r.Client.Get(ctx, req.NamespacedName, httproute)

	if apierrors.IsNotFound(err) {
		if err := r.Driver.DeleteNamedHTTPRoute(req.NamespacedName); err != nil {
			log.Error(err, "Failed to delete httproute from store")
			return ctrl.Result{}, err
		}

		return managerdriver.HandleSyncResult(r.Driver.Sync(ctx, r.Client))
	}

	if err != nil {
		return ctrl.Result{}, err
	}

	if controller.IsDelete(httproute) {
		log.Info("Deleting httproute from store")
		if err := util.RemoveAndSyncFinalizer(ctx, r.Client, httproute); err != nil {
			log.Error(err, "Failed to remove finalizer")
			return ctrl.Result{}, err
		}

		// Remove it from the store
		return ctrl.Result{}, r.Driver.DeleteHTTPRoute(httproute)
	}

	// Per the Gateway API spec, only manage routes that reference our GatewayClass.
	// If no parentRef targets an ngrok-managed Gateway, remove any previously-added
	// finalizer and skip reconciliation entirely.
	owned, err := routeReferencesNgrokGateway(ctx, r.Client, httproute.Namespace, httproute.Spec.ParentRefs)
	if err != nil {
		return ctrl.Result{}, err
	}
	if !owned {
		log.V(1).Info("HTTPRoute does not reference any ngrok-managed Gateway, skipping")
		return ctrl.Result{}, util.RemoveAndSyncFinalizer(ctx, r.Client, httproute)
	}

	// Skip non-delete reconciles during drain to prevent adding new finalizers
	if controller.IsDraining(ctx, r.DrainState) {
		log.V(1).Info("Draining, skipping non-delete reconcile")
		return ctrl.Result{}, nil
	}

	// The object is not being deleted, so register and sync finalizer
	if err := util.RegisterAndSyncFinalizer(ctx, r.Client, httproute); err != nil {
		log.Error(err, "Failed to register finalizer")
		return ctrl.Result{}, err
	}

	// Status is written by the driver at the end of Sync, from the whole store.
	_, err = r.Driver.UpdateHTTPRoute(httproute)
	if err != nil {
		return ctrl.Result{}, err
	}

	return managerdriver.HandleSyncResult(r.Driver.Sync(ctx, r.Client))
}

// storeFedResources lists the kinds this controller keeps fresh in the
// managerdriver store. Every entry is registered with a
// ControllerEventHandler; driver.Seed only populates the store once at boot,
// so a kind missing from this list is resolvable at startup and then silently
// goes stale. Kept as a function so tests can assert its coverage.
//
// The traffic-policy kinds are listed here as well as on the Ingress
// controller on purpose: the Gateway feature set can be enabled with the
// Ingress feature set disabled, and in that configuration this is the only
// controller keeping the store's traffic policies current. HTTPRoute
// extensionRef filters resolve through store.ResolveTrafficPolicy, so without
// these entries a policy edited after boot would never be picked up.
// Registering the same kind from more than one controller is harmless and
// already the norm here — Domain is registered from three.
func storeFedResources() []client.Object {
	return []client.Object{
		&gatewayv1.GatewayClass{},
		&corev1.Service{},
		&ngrokv1.Domain{},
		// LEGACY-domain-kind: drop the legacy Domain entry at cleanup.
		&ingressv1alpha1.Domain{},
		// LEGACY-trafficpolicy-kind: drop the NgrokTrafficPolicy entry at
		// cleanup; the canonical TrafficPolicy entry below stays.
		&ngrokv1alpha1.NgrokTrafficPolicy{},
		&ngrokv1.TrafficPolicy{},
	}
}

// SetupWithManager sets up the controller with the Manager.
func (r *HTTPRouteReconciler) SetupWithManager(mgr ctrl.Manager) error {
	builder := ctrl.NewControllerManagedBy(mgr).
		For(
			&gatewayv1.HTTPRoute{},
			builder.WithPredicates(
				predicate.Or(
					predicate.AnnotationChangedPredicate{},
					predicate.GenerationChangedPredicate{},
				),
			),
		)

	builder = builder.Watches(
		&gatewayv1.Gateway{},
		handler.EnqueueRequestsFromMapFunc(r.findHTTPRouteForGateway),
	)

	for _, obj := range storeFedResources() {
		builder = builder.Watches(
			obj,
			managerdriver.NewControllerEventHandler(
				obj.GetObjectKind().GroupVersionKind().Kind,
				r.Driver,
				r.Client,
			),
		)
	}
	return builder.Complete(r)
}

func (r *HTTPRouteReconciler) findHTTPRouteForGateway(ctx context.Context, o client.Object) []reconcile.Request {
	log := r.Log

	gw, ok := o.(*gatewayv1.Gateway)
	if !ok {
		log.Error(nil, "object is not a Gateway", "object", o)
		return nil
	}

	log = log.WithValues(
		"gateway.name", gw.Name,
		"gateway.namespace", gw.Namespace,
		"gateway.gatewayClassName", gw.Spec.GatewayClassName,
	)

	gwc := &gatewayv1.GatewayClass{}
	err := r.Client.Get(ctx, client.ObjectKey{Name: string(gw.Spec.GatewayClassName)}, gwc)
	if err != nil {
		log.Error(err, "Failed to get GatewayClass", "gatewayClassName", gw.Spec.GatewayClassName)
		return nil
	}

	if !ShouldHandleGatewayClass(gwc) {
		log.V(5).Info("GatewayClass is not handled by this controller, ignoring")
		return nil
	}

	routes := &gatewayv1.HTTPRouteList{}
	err = r.Client.List(ctx, routes)
	if err != nil {
		log.Error(err, "Failed to list HTTPRoutes")
		return nil
	}

	requests := []reconcile.Request{}
	log.V(3).Info("Finding HTTPRoutes for Gateway")
	for _, route := range routes.Items {
		for _, parentRef := range route.Spec.ParentRefs {
			group := ptr.Deref(parentRef.Group, gatewayv1.GroupName)
			if group != gatewayv1.GroupName {
				log.V(5).Info("ParentRef group is not gateway.networking.k8s.io, ignoring", "group", parentRef.Group)
				continue
			}

			kind := ptr.Deref(parentRef.Kind, gatewayv1.Kind("Gateway"))
			if kind != "Gateway" {
				log.V(5).Info("ParentRef kind is not Gateway, ignoring", "kind", parentRef.Kind)
				continue
			}

			parentRefNamespace := string(ptr.Deref(parentRef.Namespace, gatewayv1.Namespace(route.Namespace)))
			if string(parentRef.Name) != gw.Name || parentRefNamespace != gw.Namespace {
				log.V(5).Info("ParentRef does not match Gateway, ignoring", "parentRef", parentRef)
				continue
			}

			requests = append(requests, reconcile.Request{
				Namespace: route.Namespace,
				Name:      route.Name,
			})
			break // Only enqueue the route once per parentRef
		}
	}

	return requests
}
