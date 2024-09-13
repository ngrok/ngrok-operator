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

package bindings

import (
	"context"

	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/go-logr/logr"
	bindingsv1alpha1 "github.com/ngrok/ngrok-operator/api/bindings/v1alpha1"
	"github.com/ngrok/ngrok-operator/internal/controller"
)

// EndpointBindingReconciler reconciles a EndpointBinding object
type EndpointBindingReconciler struct {
	client.Client
	Scheme     *runtime.Scheme
	controller *controller.BaseController[*bindingsv1alpha1.EndpointBinding]

	Log      logr.Logger
	Recorder record.EventRecorder
}

// +kubebuilder:rbac:groups=bindings.k8s.ngrok.com,resources=endpointbindings,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=bindings.k8s.ngrok.com,resources=endpointbindings/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=bindings.k8s.ngrok.com,resources=endpointbindings/finalizers,verbs=update

// SetupWithManager sets up the controller with the Manager.
func (r *EndpointBindingReconciler) SetupWithManager(mgr ctrl.Manager) error {
	r.controller = &controller.BaseController[*bindingsv1alpha1.EndpointBinding]{
		Kube:     r.Client,
		Log:      r.Log,
		Recorder: r.Recorder,

		StatusID:  r.statusID,
		Create:    r.create,
		Update:    r.update,
		Delete:    r.delete,
		ErrResult: r.errResult,
	}

	return ctrl.NewControllerManagedBy(mgr).
		For(&bindingsv1alpha1.EndpointBinding{}).
		Complete(r)
}

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the EndpointBinding object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.18.4/pkg/reconcile
func (r *EndpointBindingReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	// TODO(user): your logic here
	// Implement the following:
	// - Update/Create kind: Service (ngrok-op namespace)
	// - Update/Create kind: Service (external, target namespace)
	// - Update the Pod Forwarders mapping and restart anything
	// - Update the EndpointBinding status

	return r.controller.Reconcile(ctx, req)
}

func (r *EndpointBindingReconciler) statusID(cr *bindingsv1alpha1.EndpointBinding) string {
	return "TODO"
}

func (r *EndpointBindingReconciler) create(ctx context.Context, cr *bindingsv1alpha1.EndpointBinding) error {
	r.Recorder.Event(cr, v1.EventTypeWarning, "Created", "TODO Implement me")
	return nil
}

func (r *EndpointBindingReconciler) update(ctx context.Context, cr *bindingsv1alpha1.EndpointBinding) error {
	r.Recorder.Event(cr, v1.EventTypeWarning, "Updated", "TODO Implement me")
	return nil
}

func (r *EndpointBindingReconciler) delete(ctx context.Context, cr *bindingsv1alpha1.EndpointBinding) error {
	r.Recorder.Event(cr, v1.EventTypeWarning, "Deleted", "TODO Implement me")
	return nil
}

func (r *EndpointBindingReconciler) errResult(op controller.BaseControllerOp, cr *bindingsv1alpha1.EndpointBinding, err error) (ctrl.Result, error) {
	return ctrl.Result{}, err
}
