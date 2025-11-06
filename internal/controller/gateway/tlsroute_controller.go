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

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	gatewayv1alpha2 "sigs.k8s.io/gateway-api/apis/v1alpha2"

	"github.com/go-logr/logr"
	"github.com/ngrok/ngrok-operator/internal/controller"
	"github.com/ngrok/ngrok-operator/pkg/managerdriver"
)

// TLSRouteReconciler reconciles a TLSRoute object
type TLSRouteReconciler struct {
	client.Client

	Log      logr.Logger
	Scheme   *runtime.Scheme
	Recorder record.EventRecorder
	Driver   *managerdriver.Driver
}

// +kubebuilder:rbac:groups=gateway.networking.k8s.io,resources=tlsroutes,verbs=get;list;watch;update
// +kubebuilder:rbac:groups=gateway.networking.k8s.io,resources=tlsroutes/status,verbs=get;list;watch;update

func (r *TLSRouteReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := ctrl.LoggerFrom(ctx).WithValues("TLSRoute", req.NamespacedName)
	ctx = ctrl.LoggerInto(ctx, log)

	tlsRoute := new(gatewayv1alpha2.TLSRoute)
	err := r.Client.Get(ctx, req.NamespacedName, tlsRoute)
	switch {
	case err == nil:
		// all good, continue
	case client.IgnoreNotFound(err) == nil:
		if err := r.Driver.DeleteNamedTLSRoute(req.NamespacedName); err != nil {
			log.Error(err, "failed to delete TLSRoute from store")
			return ctrl.Result{}, err
		}

		err = r.Driver.Sync(ctx, r.Client)
		if err != nil {
			log.Error(err, "failed to sync after removing TLSRoute from store")
			return ctrl.Result{}, err
		}

		return ctrl.Result{}, nil
	default:
		return ctrl.Result{}, err
	}

	if controller.IsCleanedUp(tlsRoute) {
		log.V(3).Info("Finalizer not present, skipping cleanup as already done")
		return ctrl.Result{}, nil
	}

	if controller.IsDelete(tlsRoute) || controller.HasCleanupAnnotation(tlsRoute) {
		log.Info("deleting TLSRoute from store")
		if err := r.Driver.DeleteTLSRoute(tlsRoute); err != nil {
			return ctrl.Result{}, err
		}

		tlsRoute.Status = gatewayv1alpha2.TLSRouteStatus{
			RouteStatus: gatewayv1alpha2.RouteStatus{
				Parents: []gatewayv1alpha2.RouteParentStatus{},
			},
		}

		if err := r.Client.Status().Update(ctx, tlsRoute); err != nil {
			log.Error(err, "Failed to update TLSRoute status")
			return ctrl.Result{}, err
		}

		if err := controller.RemoveAndSyncFinalizer(ctx, r.Client, tlsRoute); err != nil {
			log.Error(err, "Failed to remove finalizer")
			return ctrl.Result{}, err
		}

		return ctrl.Result{}, nil
	}

	tlsRoute, err = r.Driver.UpdateTLSRoute(tlsRoute)
	if err != nil {
		return ctrl.Result{}, err
	}

	// The object is not being deleted, so register and sync finalizer
	if err := controller.RegisterAndSyncFinalizer(ctx, r.Client, tlsRoute); err != nil {
		log.Error(err, "Failed to register finalizer")
		return ctrl.Result{}, err
	}

	if err := r.Driver.Sync(ctx, r.Client); err != nil {
		log.Error(err, "failed to sync after reconciling TLSRoutes")
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *TLSRouteReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).For(&gatewayv1alpha2.TLSRoute{}).Complete(r)
}
