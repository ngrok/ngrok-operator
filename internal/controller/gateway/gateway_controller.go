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

	ingressv1alpha1 "github.com/ngrok/kubernetes-ingress-controller/api/ingress/v1alpha1"
	//internalerrors "github.com/ngrok/kubernetes-ingress-controller/internal/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"

	"github.com/go-logr/logr"
	"github.com/ngrok/kubernetes-ingress-controller/internal/store"
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
	Driver   *store.Driver
}

// +kubebuilder:rbac:groups="",resources=events,verbs=create;patch
// +kubebuilder:rbac:groups="",resources=configmaps,verbs=get;list;watch;create;update;delete
// +kubebuilder:rbac:groups="",resources=secrets,verbs=get;list;watch
// +kubebuilder:rbac:groups="",resources=services,verbs=get;list;watch
//+kubebuilder:rbac:groups=gateway.networking.k8s.io,resources=gateways,verbs=get;list;watch;update
//+kubebuilder:rbac:groups=gateway.networking.k8s.io,resources=gateways/status,verbs=get;list;watch;update
//+kubebuilder:rbac:groups=gateway.networking.k8s.io,resources=gatewayclasses,verbs=get;list;watch;update
//+kubebuilder:rbac:groups=gateway.networking.k8s.io,resources=gatewayclasses/status,verbs=get;list;watch;update

func (r *GatewayReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := r.Log.WithValues("Gateway", req.NamespacedName)
	ctx = ctrl.LoggerInto(ctx, log)

	gw := new(gatewayv1.Gateway)
	err := r.Client.Get(ctx, req.NamespacedName, gw)
	r.Log.Info("the request", "rq", req)
	switch {
	case err == nil:
		// all good, continue
	case client.IgnoreNotFound(err) == nil:
		if err := r.Driver.DeleteNamedIngress(req.NamespacedName); err != nil {
			log.Error(err, "Failed to delete gateway from store")
			return ctrl.Result{}, err
		}

		err = r.Driver.Sync(ctx, r.Client)
		if err != nil {
			log.Error(err, "Failed to sync after removing ingress from store")
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
	}

	// Ensure the ingress object is up to date in the store
	// Leverage the store to ensure this works off the same data as everything else
	log.V(1).Info("Updating Gateway from store")
	gw, err = r.Driver.UpdateGateway(gw)
	if err != nil {
		log.V(1).Info("FAILED TO UPDATE", "ERROR", err)
		return ctrl.Result{}, err
	}
	//switch {
	//case err == nil:
	//	// all good, continue
	//case internalerrors.IsErrDifferentIngressClass(err):
	//	log.Info("Ingress is not of type ngrok so skipping it")
	//	return ctrl.Result{}, nil
	//case internalerrors.IsErrInvalidIngressSpec(err):
	//	log.Info("Ingress is not valid so skipping it")
	//	return ctrl.Result{}, nil
	//default:
	//	log.Error(err, "Failed to get ingress from store")
	//	return ctrl.Result{}, err
	//}

	if isUpsert(gw) {
		// The object is not being deleted, so register and sync finalizer
		if err := registerAndSyncFinalizer(ctx, r.Client, gw); err != nil {
			log.Error(err, "Failed to register finalizer")
			return ctrl.Result{}, err
		}
	} else {
		log.Info("Deleting gateway from store")
		if hasFinalizer(gw) {
			if err := removeAndSyncFinalizer(ctx, r.Client, gw); err != nil {
				log.Error(err, "Failed to remove finalizer")
				return ctrl.Result{}, err
			}
		}

		// Remove it from the store
		if err := r.Driver.DeleteGateway(gw); err != nil {
			return ctrl.Result{}, err
		}
	}

	log.V(1).Info("SYNCING DRIVER FROM GATEWAY")
	if err := r.Driver.Sync(ctx, r.Client); err != nil {
		log.Error(err, "Faild to sync")
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *GatewayReconciler) SetupWithManager(mgr ctrl.Manager) error {
	storedResources := []client.Object{
		//&gatewayv1.GatewayClass{},
		//&gatewayv1.HTTPRoute{},
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
			store.NewUpdateStoreHandler(
				obj.GetObjectKind().GroupVersionKind().Kind,
				r.Driver,
			),
		)
	}

	return builder.Complete(r)
}
