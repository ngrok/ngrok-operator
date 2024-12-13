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

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"

	"github.com/go-logr/logr"
	ingressv1alpha1 "github.com/ngrok/ngrok-operator/api/ingress/v1alpha1"
	"github.com/ngrok/ngrok-operator/internal/controller"
	"github.com/ngrok/ngrok-operator/pkg/managerdriver"
)

// HTTPRouteReconciler reconciles a HTTPRoute object
type HTTPRouteReconciler struct {
	client.Client

	Log      logr.Logger
	Scheme   *runtime.Scheme
	Recorder record.EventRecorder
	Driver   *managerdriver.Driver
}

// +kubebuilder:rbac:groups=gateway.networking.k8s.io,resources=httproutes,verbs=get;list;watch;update
// +kubebuilder:rbac:groups=gateway.networking.k8s.io,resources=httproutes/status,verbs=get;list;watch;update
// +kubebuilder:rbac:groups="",resources=namespaces,verbs=get;list;watch;update

func (r *HTTPRouteReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := ctrl.LoggerFrom(ctx).WithValues("HTTPRoute", req.NamespacedName)
	ctx = ctrl.LoggerInto(ctx, log)

	httproute := new(gatewayv1.HTTPRoute)
	err := r.Client.Get(ctx, req.NamespacedName, httproute)
	switch {
	case err == nil:
		// all good, continue
	case client.IgnoreNotFound(err) == nil:
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
	default:
		return ctrl.Result{}, err
	}

	httproute, err = r.Driver.UpdateHTTPRoute(httproute)
	if err != nil {
		return ctrl.Result{}, err
	}

	if controller.IsUpsert(httproute) {
		// The object is not being deleted, so register and sync finalizer
		if err := controller.RegisterAndSyncFinalizer(ctx, r.Client, httproute); err != nil {
			log.Error(err, "Failed to register finalizer")
			return ctrl.Result{}, err
		}
	} else {
		log.Info("Deleting httproute from store")
		if controller.HasFinalizer(httproute) {
			if err := controller.RemoveAndSyncFinalizer(ctx, r.Client, httproute); err != nil {
				log.Error(err, "Failed to remove finalizer")
				return ctrl.Result{}, err
			}
		}

		// Remove it from the store
		if err := r.Driver.DeleteHTTPRoute(httproute); err != nil {
			return ctrl.Result{}, err
		}
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
		&gatewayv1.Gateway{},
		&corev1.Service{},
		&ingressv1alpha1.Domain{},
		&ingressv1alpha1.HTTPSEdge{},
		&ingressv1alpha1.Tunnel{},
		//&ingressv1alpha1.NgrokModuleSet{},
	}

	builder := ctrl.NewControllerManagedBy(mgr).For(&gatewayv1.HTTPRoute{})
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
