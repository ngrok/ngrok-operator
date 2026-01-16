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

// TCPRouteReconciler reconciles a TCPRoute object
type TCPRouteReconciler struct {
	client.Client

	Log      logr.Logger
	Scheme   *runtime.Scheme
	Recorder record.EventRecorder
	Driver   *managerdriver.Driver
	// DrainState is used to check if the operator is draining.
	// If draining, non-delete reconciles are skipped to prevent new finalizers.
	DrainState controller.DrainState
}

// +kubebuilder:rbac:groups=gateway.networking.k8s.io,resources=tcproutes,verbs=get;list;watch;update
// +kubebuilder:rbac:groups=gateway.networking.k8s.io,resources=tcproutes/status,verbs=get;list;watch;update

func (r *TCPRouteReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := ctrl.LoggerFrom(ctx).WithValues("TCPRoute", req.NamespacedName)
	ctx = ctrl.LoggerInto(ctx, log)

	tcpRoute := new(gatewayv1alpha2.TCPRoute)
	err := r.Client.Get(ctx, req.NamespacedName, tcpRoute)
	switch {
	case err == nil:
		// all good, continue
	case client.IgnoreNotFound(err) == nil:
		if err := r.Driver.DeleteNamedTCPRoute(req.NamespacedName); err != nil {
			log.Error(err, "failed to delete TCPRoute from store")
			return ctrl.Result{}, err
		}

		err = r.Driver.Sync(ctx, r.Client)
		if err != nil {
			log.Error(err, "failed to sync after removing TCPRoute from store")
			return ctrl.Result{}, err
		}

		return ctrl.Result{}, nil
	default:
		return ctrl.Result{}, err
	}

	tcpRoute, err = r.Driver.UpdateTCPRoute(tcpRoute)
	if err != nil {
		return ctrl.Result{}, err
	}

	if controller.IsUpsert(tcpRoute) {
		// Skip non-delete reconciles during drain to prevent adding new finalizers
		if controller.IsDraining(ctx, r.DrainState) {
			log.V(1).Info("Draining, skipping non-delete reconcile")
			return ctrl.Result{}, nil
		}

		// The object is not being deleted, so register and sync finalizer
		if err := controller.RegisterAndSyncFinalizer(ctx, r.Client, tcpRoute); err != nil {
			log.Error(err, "Failed to register finalizer")
			return ctrl.Result{}, err
		}
	} else {
		log.Info("deleting TCPRoute from store")
		if err := controller.RemoveAndSyncFinalizer(ctx, r.Client, tcpRoute); err != nil {
			log.Error(err, "Failed to remove finalizer")
			return ctrl.Result{}, err
		}

		// Remove it from the store
		if err := r.Driver.DeleteTCPRoute(tcpRoute); err != nil {
			return ctrl.Result{}, err
		}
	}

	if err := r.Driver.Sync(ctx, r.Client); err != nil {
		log.Error(err, "failed to sync after reconciling TCPRoutes")
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *TCPRouteReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).For(&gatewayv1alpha2.TCPRoute{}).Complete(r)
}
