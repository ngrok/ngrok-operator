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
	"errors"
	"fmt"
	"reflect"

	"github.com/go-logr/logr"
	ingressv1alpha1 "github.com/ngrok/ngrok-operator/api/ingress/v1alpha1"
	"github.com/ngrok/ngrok-operator/internal/controller"
	"github.com/ngrok/ngrok-operator/internal/util"
	"github.com/ngrok/ngrok-operator/pkg/managerdriver"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
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
	Recorder record.EventRecorder
	Driver   *managerdriver.Driver
	// DrainState is used to check if the operator is draining.
	// If draining, non-delete reconciles are skipped to prevent new finalizers.
	DrainState controller.DrainState
}

// +kubebuilder:rbac:groups=gateway.networking.k8s.io,resources=httproutes,verbs=get;list;watch;update
// +kubebuilder:rbac:groups=gateway.networking.k8s.io,resources=httproutes/status,verbs=get;list;watch;update
// +kubebuilder:rbac:groups="",resources=namespaces,verbs=get;list;watch;update

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

		err = r.Driver.Sync(ctx, r.Client)
		if err != nil {
			log.Error(err, "Failed to sync after removing httproute from store")
			return ctrl.Result{}, err
		}

		return ctrl.Result{}, nil
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

	// Validate the HTTPRoute before updating the store
	_ = r.validateHTTPRoute(ctx, httproute)

	// Update the HTTPRoute in the store if it passes validation
	_, err = r.Driver.UpdateHTTPRoute(httproute)
	if err != nil {
		return ctrl.Result{}, err
	}

	if err := r.Driver.Sync(ctx, r.Client); err != nil {
		log.Error(err, "Failed to sync")
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *HTTPRouteReconciler) SetupWithManager(mgr ctrl.Manager) error {
	storedResources := []client.Object{
		&gatewayv1.GatewayClass{},
		&corev1.Service{},
		&ingressv1alpha1.Domain{},
	}

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

	for _, obj := range storedResources {
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

var (
	ErrValidation          = errors.New("validation")
	ErrParentRefNotFound   = errors.New("parentRefs not found")
	ErrRouteGKNotSupported = errors.New("route group kind not supported")
)

func (r *HTTPRouteReconciler) validateHTTPRoute(ctx context.Context, route *gatewayv1.HTTPRoute) error {
	log := ctrl.LoggerFrom(ctx)

	parentRefsAccepted, err := r.validateRouteParentRefs(ctx, route)
	if err != nil {
		return err
	}

	route.Status.RouteStatus = gatewayv1.RouteStatus{
		Parents: parentRefsAccepted,
	}

	err = r.Client.Status().Update(ctx, route)
	if err != nil {
		return fmt.Errorf("failed to update httproute status: %w", err)
	}

	// Check to make sure that all parentRefs have been accepted
	log.V(3).Info("Checking if all parentRefs have been accepted", "parents", route.Status.RouteStatus.Parents)
	for _, parentStatus := range route.Status.RouteStatus.Parents {
		for _, cond := range parentStatus.Conditions {
			if cond.Status != metav1.ConditionTrue {
				return fmt.Errorf("%w: route has not been accepted by all parentRefs", ErrValidation)
			}
		}
	}

	log.V(3).Info("All parentRefs have been accepted", "parents", route.Status.RouteStatus.Parents)
	return nil
}

func (r *HTTPRouteReconciler) validateRouteParentRefs(ctx context.Context, route *gatewayv1.HTTPRoute) ([]gatewayv1.RouteParentStatus, error) {
	log := ctrl.LoggerFrom(ctx)

	log.V(5).Info("Validating route parentRefs")

	if len(route.Spec.ParentRefs) == 0 {
		return nil, ErrParentRefNotFound
	}

	parentStatuses := []gatewayv1.RouteParentStatus{}

	for _, parentRef := range route.Spec.ParentRefs {
		parentStatus := gatewayv1.RouteParentStatus{
			ParentRef:      parentRef,
			ControllerName: ControllerName,
			Conditions:     []metav1.Condition{},
		}

		// Find & use existing conditions for this parentRef so we preserve previous conditions
		for _, s := range route.Status.RouteStatus.Parents {
			if !reflect.DeepEqual(s.ParentRef, parentRef) {
				continue
			}

			parentStatus.Conditions = s.Conditions
		}

		group := ptr.Deref(parentRef.Group, gatewayv1.GroupName)

		var cnd metav1.Condition

		switch group {
		case gatewayv1.GroupName:
			parentRefName := string(parentRef.Name)
			// Get the parentRef namespace if supplied. If it's not supplied, use the route namespace.
			parentRefNamespace := string(ptr.Deref(parentRef.Namespace, gatewayv1.Namespace(route.Namespace)))
			parentRefLog := log.WithValues("parentRef", types.NamespacedName{
				Name:      parentRefName,
				Namespace: parentRefNamespace,
			})

			// TODO: Get the gateway from the store to limit the number of API calls
			gw := &gatewayv1.Gateway{}
			err := r.Client.Get(ctx, types.NamespacedName{
				Name:      parentRefName,
				Namespace: parentRefNamespace,
			}, gw)

			if err != nil {
				if client.IgnoreNotFound(err) != nil {
					// Some other error besides not found
					parentRefLog.Error(err, "Failed to get gateway")
					break
				}

				log.Error(err, "Gateway not found", "parentRef", parentRefName, "namespace", parentRefNamespace)
				cnd = r.newCondition(
					route,
					gatewayv1.RouteConditionAccepted,
					gatewayv1.RouteReasonNoMatchingParent,
					"",
				)
				break
			}

			// Find the listener that matches the parentRef
			noMatchingParent := true
			for _, listener := range gw.Spec.Listeners {
				if parentRef.Port != nil && *parentRef.Port != listener.Port {
					continue
				}

				if parentRef.SectionName != nil && *parentRef.SectionName != listener.Name {
					continue
				}

				noMatchingParent = false
			}

			var reason gatewayv1.RouteConditionReason
			if noMatchingParent {
				reason = gatewayv1.RouteReasonNoMatchingParent
			} else {
				reason = gatewayv1.RouteReasonAccepted
			}

			cnd = r.newCondition(
				route,
				gatewayv1.RouteConditionAccepted,
				reason,
				"",
			)

		case "":
			// TODO: From the spec:
			//    To set the core API group (such as for a "Service" kind referent),
			//    Group must be explicitly set to "".
			fallthrough
		default:
			cnd = r.newCondition(
				route,
				gatewayv1.RouteConditionAccepted,
				gatewayv1.RouteReasonInvalidKind,
				fmt.Sprintf("Group '%s' is not supported", group),
			)
		}

		meta.SetStatusCondition(&parentStatus.Conditions, cnd)
		parentStatuses = append(parentStatuses, parentStatus)
	}

	return parentStatuses, nil
}

func (r *HTTPRouteReconciler) newCondition(route *gatewayv1.HTTPRoute, t gatewayv1.RouteConditionType, reason gatewayv1.RouteConditionReason, msg string) metav1.Condition {
	status := metav1.ConditionTrue
	if reason != gatewayv1.RouteReasonAccepted && reason != gatewayv1.RouteReasonResolvedRefs {
		status = metav1.ConditionFalse
	}
	return metav1.Condition{
		Type:               string(t),
		Status:             status,
		ObservedGeneration: route.Generation,
		Reason:             string(reason),
		Message:            msg,
	}
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
	err = r.Client.List(ctx, &gatewayv1.HTTPRouteList{})
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

			if string(parentRef.Name) != gw.Name || (parentRef.Namespace != nil && string(*parentRef.Namespace) != gw.Namespace) {
				log.V(5).Info("ParentRef does not match Gateway, ignoring", "parentRef", parentRef)
				continue
			}

			requests = append(requests, reconcile.Request{
				NamespacedName: client.ObjectKey{
					Namespace: route.Namespace,
					Name:      route.Name,
				},
			})
			break // Only enqueue the route once per parentRef
		}
	}

	return requests
}
