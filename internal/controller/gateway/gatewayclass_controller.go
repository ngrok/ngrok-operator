/*
MIT License

Copyright (c) 2024 ngrok, Inc.

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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"
)

// GatewayClassReconciler reconciles a GatewayClass object
type GatewayClassReconciler struct {
	client.Client

	Log      logr.Logger
	Scheme   *runtime.Scheme
	Recorder record.EventRecorder
}

// +kubebuilder:rbac:groups=gateway.networking.k8s.io,resources=gatewayclasses,verbs=get;list;watch;patch;update
// +kubebuilder:rbac:groups=gateway.networking.k8s.io,resources=gatewayclasses/status,verbs=patch;update
// +kubebuilder:rbac:groups=gateway.networking.k8s.io,resources=gatewayclasses/finalizers,verbs=patch;update
// +kubebuilder:rbac:groups=gateway.networking.k8s.io,resources=gateways,verbs=get;list;watch

// SetupWithManager sets up the reconciler with the Manager
func (r *GatewayClassReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&gatewayv1.GatewayClass{},
			builder.WithPredicates(
				predicate.NewPredicateFuncs(func(o client.Object) bool {
					switch v := o.(type) {
					case *gatewayv1.GatewayClass:
						return shouldHandleGatewayClass(v)
					default:
					}
					r.Log.V(1).Info("Filtering out object", "object", o)
					return false
				}),
			),
		).
		Watches(
			&gatewayv1.Gateway{},
			handler.EnqueueRequestsFromMapFunc(r.findGatewayClassForGateway),
		).
		// WithEventFilter filters out events. It applies to all events, including those from watches.
		WithEventFilter(
			predicate.Or(
				predicate.GenerationChangedPredicate{},
			),
		).
		Complete(r)
}

// Reconcile reconciles a GatewayClass object ctrl.Request
func (r *GatewayClassReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := r.Log.WithValues("gatewayclass", req.NamespacedName)
	ctrl.LoggerInto(ctx, log)

	gwc := &gatewayv1.GatewayClass{}
	if err := r.Get(ctx, req.NamespacedName, gwc); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	if !shouldHandleGatewayClass(gwc) {
		return ctrl.Result{}, nil
	}

	log.V(1).Info("Reconciling GatewayClass")

	// Accept the GatewayClass if it is not already accepted
	if err := r.reconcileAcceptedCondition(ctx, gwc); err != nil {
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, r.reconcileGatewayExistsFinalizer(ctx, gwc)
}

// reconcileAcceptedCondition makes sure that the GatewayClass has an accepted condition
func (r *GatewayClassReconciler) reconcileAcceptedCondition(ctx context.Context, gwc *gatewayv1.GatewayClass) error {
	log := ctrl.LoggerFrom(ctx)

	if gatewayClassIsAccepted(gwc) {
		log.V(1).Info("GatewayClass already accepted")
		return nil
	}

	gwc.Status.Conditions = appendGatewayClassCondition(gwc.Status.Conditions, metav1.Condition{
		Type:               string(gatewayv1.GatewayClassConditionStatusAccepted),
		Status:             metav1.ConditionTrue,
		Reason:             string(gatewayv1.GatewayClassReasonAccepted),
		Message:            "gatewayclass accepted by the ngrok controller",
		LastTransitionTime: metav1.Now(),
		ObservedGeneration: gwc.Generation,
	})

	log.V(1).Info("Accepting GatewayClass")
	return r.Status().Update(ctx, gwc)
}

// reconcileGatewayExistsFinalizer adds the GatewayClassGatewayExistsFinalizer if there are gateways that reference this GatewayClass.
// It removes the finalizer if there are no gateways that reference this GatewayClass.
func (r *GatewayClassReconciler) reconcileGatewayExistsFinalizer(ctx context.Context, gwc *gatewayv1.GatewayClass) error {
	log := ctrl.LoggerFrom(ctx)

	// Filter out gateways that are not of this GatewayClass
	log.V(3).Info("Finding gateways for GatewayClass", "gatewayclass", gwc.Name)
	gatewayList := &gatewayv1.GatewayList{}
	if err := r.List(ctx, gatewayList); err != nil {
		return err
	}

	filtered := []gatewayv1.Gateway{}
	for _, gw := range gatewayList.Items {
		if string(gw.Spec.GatewayClassName) == gwc.Name {
			filtered = append(filtered, gw)
		}
	}

	log.V(3).Info("Filtered gateways for GatewayClass", "matching", filtered)

	if len(filtered) == 0 {
		if controllerutil.ContainsFinalizer(gwc, gatewayv1.GatewayClassFinalizerGatewaysExist) {
			log.V(1).Info("Removing finalizer", "finalizer", gatewayv1.GatewayClassFinalizerGatewaysExist)

			patch := client.MergeFrom(gwc.DeepCopy())
			controllerutil.RemoveFinalizer(gwc, gatewayv1.GatewayClassFinalizerGatewaysExist)
			return r.Patch(ctx, gwc, patch)
		}

		return nil
	}

	if !controllerutil.ContainsFinalizer(gwc, gatewayv1.GatewayClassFinalizerGatewaysExist) {
		log.V(1).Info("Adding finalizer", "finalizer", gatewayv1.GatewayClassFinalizerGatewaysExist)

		patch := client.MergeFrom(gwc.DeepCopy())
		controllerutil.AddFinalizer(gwc, gatewayv1.GatewayClassFinalizerGatewaysExist)
		return r.Patch(ctx, gwc, patch)
	}

	log.V(1).Info("Finalizers match expected state")
	return nil
}

// findGatewayClassForGateway returns a reconcile.Request for the GatewayClass of the given gateway. It is used by
// the watch on Gateway objects to trigger a reconciliation of the GatewayClass.
func (r *GatewayClassReconciler) findGatewayClassForGateway(_ context.Context, o client.Object) []reconcile.Request {
	log := r.Log

	gw, ok := o.(*gatewayv1.Gateway)
	if !ok {
		log.Error(nil, "object is not a Gateway", "object", o)
		return nil
	}

	log = log.WithValues("gateway.name", gw.Name, "gateway.namespace", gw.Namespace, "gateway.gatewayClassName", gw.Spec.GatewayClassName)

	if gw.Spec.GatewayClassName == "" {
		log.V(1).Info("Gateway does not have a GatewayClassName, ignoring")
		return nil
	}

	log.V(1).Info("Enqueueing request for gatewayclass")
	return []reconcile.Request{
		{
			NamespacedName: client.ObjectKey{
				Namespace: "",
				Name:      string(gw.Spec.GatewayClassName),
			},
		},
	}
}

// shouldHandleGatewayClass returns true if the GatewayClass should be handled by this controller
// based on the ControllerName field, false otherwise.
func shouldHandleGatewayClass(gatewayClass *gatewayv1.GatewayClass) bool {
	return gatewayClass.Spec.ControllerName == ControllerName
}

// appendGatewayClassCondition appends a new condition to the list of conditions, replacing any existing condition with the same type.
func appendGatewayClassCondition(conditions []metav1.Condition, newCondition metav1.Condition) []metav1.Condition {
	newConditions := []metav1.Condition{}
	for _, c := range conditions {
		if c.Type != newCondition.Type {
			newConditions = append(newConditions, c)
		}
	}
	return append(newConditions, newCondition)
}

func gatewayClassIsAccepted(gwc *gatewayv1.GatewayClass) bool {
	for _, condition := range gwc.Status.Conditions {
		if condition.Type == string(gatewayv1.GatewayClassConditionStatusAccepted) &&
			condition.Status == metav1.ConditionTrue &&
			condition.Reason == string(gatewayv1.GatewayClassReasonAccepted) &&
			condition.ObservedGeneration == gwc.Generation {
			return true
		}
	}
	return false
}
