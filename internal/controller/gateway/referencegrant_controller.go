/*
MIT License

Copyright (c) 2025 ngrok, Inc.

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
	"fmt"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	gatewayv1beta1 "sigs.k8s.io/gateway-api/apis/v1beta1"

	"github.com/go-logr/logr"
	"github.com/ngrok/ngrok-operator/pkg/managerdriver"
)

// ReferenceGrantReconciler reconciles ReferenceGrants
type ReferenceGrantReconciler struct {
	client.Client

	Log      logr.Logger
	Scheme   *runtime.Scheme
	Recorder record.EventRecorder
	Driver   *managerdriver.Driver
}

// +kubebuilder:rbac:groups=gateway.networking.k8s.io,resources=referencegrants,verbs=get;list;watch

func (r *ReferenceGrantReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := ctrl.LoggerFrom(ctx).WithValues("ReferenceGrant", req.Name)
	ctx = ctrl.LoggerInto(ctx, log)

	var referenceGrant gatewayv1beta1.ReferenceGrant
	err := r.Get(ctx, req.NamespacedName, &referenceGrant)

	switch {
	case err == nil:
		_, err := r.Driver.UpdateReferenceGrant(&referenceGrant)
		if err != nil {
			log.Error(err, "failed to update ReferenceGrant in store")
			return ctrl.Result{}, err
		}
	case client.IgnoreNotFound(err) == nil:
		if err := r.Driver.DeleteReferenceGrant(req.NamespacedName); err != nil {
			log.Error(err, "failed to delete ReferenceGrant from store")
			return ctrl.Result{}, err
		}
	default:
		return ctrl.Result{}, err
	}

	err = r.Driver.Sync(ctx, r.Client)
	if err != nil {
		log.Error(err, "failed to sync after reconciling ReferenceGrant",
			"ReferenceGrant", fmt.Sprintf("%s.%s", req.Name, req.Namespace),
		)
		return ctrl.Result{}, err
	}
	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *ReferenceGrantReconciler) SetupWithManager(mgr ctrl.Manager) error {
	storedResources := []client.Object{
		&gatewayv1beta1.ReferenceGrant{},
	}

	builder := ctrl.NewControllerManagedBy(mgr).For(&gatewayv1beta1.ReferenceGrant{})
	for _, obj := range storedResources {
		builder = builder.Watches(
			obj,
			managerdriver.NewControllerEventHandler(
				obj.GetObjectKind().GroupVersionKind().Kind,
				r.Driver,
				r.Client,
			),
		).WithEventFilter(
			predicate.Or(
				predicate.GenerationChangedPredicate{},
			),
		)
	}
	return builder.Complete(r)
}
