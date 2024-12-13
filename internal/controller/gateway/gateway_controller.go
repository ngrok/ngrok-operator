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
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"

	"github.com/go-logr/logr"
	ingressv1alpha1 "github.com/ngrok/ngrok-operator/api/ingress/v1alpha1"
	"github.com/ngrok/ngrok-operator/internal/controller"
	"github.com/ngrok/ngrok-operator/pkg/managerdriver"
)

const (
	ControllerName gatewayv1.GatewayController = "ngrok.com/gateway-controller"
)

// GatewayReconciler reconciles a Gateway object
type GatewayReconciler struct {
	client.Client

	Log      logr.Logger
	Scheme   *runtime.Scheme
	Recorder record.EventRecorder
	Driver   *managerdriver.Driver
}

// +kubebuilder:rbac:groups="",resources=events,verbs=create;patch
// +kubebuilder:rbac:groups="",resources=configmaps,verbs=get;list;watch;create;update;delete
// +kubebuilder:rbac:groups="",resources=secrets,verbs=get;list;watch
// +kubebuilder:rbac:groups="",resources=services,verbs=get;list;watch
// +kubebuilder:rbac:groups=gateway.networking.k8s.io,resources=gateways,verbs=get;list;watch;update
// +kubebuilder:rbac:groups=gateway.networking.k8s.io,resources=gateways/status,verbs=get;list;watch;update
// +kubebuilder:rbac:groups=gateway.networking.k8s.io,resources=gatewayclasses,verbs=get;list;watch;update
// +kubebuilder:rbac:groups=gateway.networking.k8s.io,resources=gatewayclasses/status,verbs=get;list;watch;update

func (r *GatewayReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := ctrl.LoggerFrom(ctx).WithValues("Gateway", req.NamespacedName)
	ctx = ctrl.LoggerInto(ctx, log)

	gw := new(gatewayv1.Gateway)
	err := r.Client.Get(ctx, req.NamespacedName, gw)
	switch {
	case err == nil:
		// all good, continue
	case client.IgnoreNotFound(err) == nil:
		if err := r.Driver.DeleteNamedGateway(req.NamespacedName); err != nil {
			log.Error(err, "Failed to delete gateway from store")
			return ctrl.Result{}, err
		}

		err = r.Driver.Sync(ctx, r.Client)
		if err != nil {
			log.Error(err, "Failed to sync after removing gateway from store")
			return ctrl.Result{}, err
		}

		return ctrl.Result{}, nil
	default:
		return ctrl.Result{}, err
	}

	log.V(1).Info("verifying gatewayclass")
	gwClass := &gatewayv1.GatewayClass{}
	if err := r.Client.Get(ctx, client.ObjectKey{Name: string(gw.Spec.GatewayClassName)}, gwClass); err != nil {
		log.V(1).Info("could not retrieve gatewayclass for gateway", "gatewayclass", gwClass.Spec.ControllerName)
		return ctrl.Result{}, nil
	}
	if gwClass.Spec.ControllerName != ControllerName {
		log.V(1).Info("unsupported gatewayclass controllername, ignoring", "gatewayclass", gwClass.Name, "controllername", gwClass.Spec.ControllerName)

		return ctrl.Result{}, nil
	}

	gw, err = r.Driver.UpdateGateway(gw)
	if err != nil {
		return ctrl.Result{}, err
	}

	if controller.IsUpsert(gw) {
		// The object is not being deleted, so register and sync finalizer
		if err := controller.RegisterAndSyncFinalizer(ctx, r.Client, gw); err != nil {
			log.Error(err, "Failed to register finalizer")
			return ctrl.Result{}, err
		}
	} else {
		log.Info("Deleting gateway from store")
		if controller.HasFinalizer(gw) {
			if err := controller.RemoveAndSyncFinalizer(ctx, r.Client, gw); err != nil {
				log.Error(err, "Failed to remove finalizer")
				return ctrl.Result{}, err
			}
		}

		// Remove it from the store
		if err := r.Driver.DeleteGateway(gw); err != nil {
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
func (r *GatewayReconciler) SetupWithManager(mgr ctrl.Manager) error {
	storedResources := []client.Object{
		&gatewayv1.GatewayClass{},
		&gatewayv1.HTTPRoute{},
		//&corev1.Service{},
		&ingressv1alpha1.Domain{},
		&ingressv1alpha1.HTTPSEdge{},
		//&ingressv1alpha1.Tunnel{},
		//&ingressv1alpha1.NgrokModuleSet{},
	}

	builder := ctrl.NewControllerManagedBy(mgr).For(&gatewayv1.Gateway{})
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
