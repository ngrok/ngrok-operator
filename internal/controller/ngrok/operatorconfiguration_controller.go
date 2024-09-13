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

package ngrok

import (
	"context"
	"errors"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/go-logr/logr"
	ngrokv1beta1 "github.com/ngrok/ngrok-operator/api/ngrok/v1beta1"
	"github.com/ngrok/ngrok-operator/internal/controller"
)

// OperatorConfigurationReconciler reconciles a OperatorConfiguration object
type OperatorConfigurationReconciler struct {
	client.Client
	Scheme     *runtime.Scheme
	controller *controller.BaseController[*ngrokv1beta1.OperatorConfiguration]

	// Namespace is where the ngrok-operator is installed/running
	Namespace string

	Log      logr.Logger
	Recorder record.EventRecorder
}

// +kubebuilder:rbac:groups=ngrok.k8s.ngrok.com,resources=operatorconfigurations,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=ngrok.k8s.ngrok.com,resources=operatorconfigurations/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=ngrok.k8s.ngrok.com,resources=operatorconfigurations/finalizers,verbs=update

// SetupWithManager sets up the controller with the Manager.
func (r *OperatorConfigurationReconciler) SetupWithManager(mgr ctrl.Manager) error {
	r.controller = &controller.BaseController[*ngrokv1beta1.OperatorConfiguration]{
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
		For(&ngrokv1beta1.OperatorConfiguration{}).
		Complete(r)
}

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the OperatorConfiguration object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.18.4/pkg/reconcile
func (r *OperatorConfigurationReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	// TODO(user): your logic here
	// I'm really not sure what should be implemented here...
	// maybe some aggregation and status updates from managers for the different features?

	return r.controller.Reconcile(ctx, req)
}

func (r *OperatorConfigurationReconciler) statusID(cr *ngrokv1beta1.OperatorConfiguration) string {
	return "TODO"
}

func (r *OperatorConfigurationReconciler) create(ctx context.Context, cr *ngrokv1beta1.OperatorConfiguration) error {
	return errors.New("not implemented")
}

func (r *OperatorConfigurationReconciler) update(ctx context.Context, cr *ngrokv1beta1.OperatorConfiguration) error {
	return errors.New("not implemented")
}

func (r *OperatorConfigurationReconciler) delete(ctx context.Context, cr *ngrokv1beta1.OperatorConfiguration) error {
	return errors.New("not implemented")
}

func (r *OperatorConfigurationReconciler) errResult(op controller.BaseControllerOp, cr *ngrokv1beta1.OperatorConfiguration, err error) (ctrl.Result, error) {
	return ctrl.Result{}, err
}
