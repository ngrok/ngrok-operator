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

	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"

	"github.com/ngrok/ngrok-operator/internal/controller"
	"github.com/ngrok/ngrok-operator/internal/util"
	"github.com/ngrok/ngrok-operator/pkg/managerdriver"
)

// routeOps carries everything reconcileRoute needs for one kind of Gateway API
// route. The TCPRoute and TLSRoute reconcilers are otherwise identical, so their
// shared logic lives in reconcileRoute and only these per-kind pieces differ.
type routeOps[T client.Object] struct {
	client     client.Client
	driver     *managerdriver.Driver
	drainState controller.DrainState

	// kind names the route type in log messages and keys, e.g. "TCPRoute".
	kind string

	// obj is a freshly allocated route for reconcileRoute to fetch into.
	obj T

	// parentRefs reads the route's parentRefs. Each route kind declares them on
	// its own spec type, so there is no shared accessor to call.
	parentRefs func(T) []gatewayv1.ParentReference

	// deleteNamed drops a route from the driver's store when the object itself
	// is already gone from the cluster.
	deleteNamed func(types.NamespacedName) error

	// deleteObj drops a route we still hold.
	deleteObj func(T) error

	// update upserts a route into the driver's store.
	update func(T) error
}

// reconcileRoute is the shared reconcile body for the Gateway API route kinds
// the operator supports.
func reconcileRoute[T client.Object](ctx context.Context, req ctrl.Request, ops routeOps[T]) (ctrl.Result, error) {
	log := ctrl.LoggerFrom(ctx).WithValues(ops.kind, req.NamespacedName)
	ctx = ctrl.LoggerInto(ctx, log)

	route := ops.obj
	err := ops.client.Get(ctx, req.NamespacedName, route)
	switch {
	case err == nil:
		// all good, continue
	case client.IgnoreNotFound(err) == nil:
		if err := ops.deleteNamed(req.NamespacedName); err != nil {
			log.Error(err, "failed to delete "+ops.kind+" from store")
			return ctrl.Result{}, err
		}

		return managerdriver.HandleSyncResult(ops.driver.Sync(ctx, ops.client))
	default:
		return ctrl.Result{}, err
	}

	if controller.IsDelete(route) {
		log.Info("deleting " + ops.kind + " from store")
		if err := util.RemoveAndSyncFinalizer(ctx, ops.client, route); err != nil {
			log.Error(err, "Failed to remove finalizer")
			return ctrl.Result{}, err
		}

		if err := ops.deleteObj(route); err != nil {
			return ctrl.Result{}, err
		}

		if err := ops.driver.Sync(ctx, ops.client); err != nil {
			log.Error(err, "failed to sync after deleting "+ops.kind+" from store")
			return ctrl.Result{}, err
		}
		return ctrl.Result{}, nil
	}

	// Per the Gateway API spec, only manage routes that reference our GatewayClass.
	// If no parentRef targets an ngrok-managed Gateway, remove any previously-added
	// finalizer and skip reconciliation entirely.
	owned, err := routeReferencesNgrokGateway(ctx, ops.client, route.GetNamespace(), ops.parentRefs(route))
	if err != nil {
		return ctrl.Result{}, err
	}
	if !owned {
		log.V(1).Info(ops.kind + " does not reference any ngrok-managed Gateway, skipping")
		return ctrl.Result{}, util.RemoveAndSyncFinalizer(ctx, ops.client, route)
	}

	// Skip non-delete reconciles during drain to prevent adding new finalizers
	if controller.IsDraining(ctx, ops.drainState) {
		log.V(1).Info("Draining, skipping non-delete reconcile")
		return ctrl.Result{}, nil
	}

	// The object is not being deleted, so register and sync finalizer
	if err := util.RegisterAndSyncFinalizer(ctx, ops.client, route); err != nil {
		log.Error(err, "Failed to register finalizer")
		return ctrl.Result{}, err
	}

	if err := ops.update(route); err != nil {
		return ctrl.Result{}, err
	}

	return managerdriver.HandleSyncResult(ops.driver.Sync(ctx, ops.client))
}
