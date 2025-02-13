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

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	"github.com/go-logr/logr"
	"github.com/ngrok/ngrok-operator/pkg/managerdriver"
)

// NamespaceReconciler reconciles Namespaces
type NamespaceReconciler struct {
	client.Client

	Log      logr.Logger
	Scheme   *runtime.Scheme
	Recorder record.EventRecorder
	Driver   *managerdriver.Driver
}

// +kubebuilder:rbac:groups="",resources=namespaces,verbs=get;list;watch

func (r *NamespaceReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := ctrl.LoggerFrom(ctx).WithValues("Namespace", req.Name)
	ctx = ctrl.LoggerInto(ctx, log)

	var namespace corev1.Namespace
	err := r.Get(ctx, req.NamespacedName, &namespace)

	switch {
	case err == nil:
		_, err := r.Driver.UpdateNamespace(&namespace)
		if err != nil {
			log.Error(err, "failed to update Namespace in store")
			return ctrl.Result{}, err
		}
	case client.IgnoreNotFound(err) == nil:
		if err := r.Driver.DeleteNamespace(req.NamespacedName.Name); err != nil {
			log.Error(err, "failed to delete Namespace from store")
			return ctrl.Result{}, err
		}
	default:
		return ctrl.Result{}, err
	}

	err = r.Driver.Sync(ctx, r.Client)
	if err != nil {
		log.Error(err, "failed to sync after reconciling Namespace",
			"Namespace", namespace.Name,
		)
		return ctrl.Result{}, err
	}
	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager
func (r *NamespaceReconciler) SetupWithManager(mgr ctrl.Manager) error {
	storedResources := []client.Object{
		&corev1.Namespace{},
	}

	builder := ctrl.NewControllerManagedBy(mgr).For(&corev1.Namespace{})
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
