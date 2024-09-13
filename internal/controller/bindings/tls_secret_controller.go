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
	"errors"

	"github.com/go-logr/logr"
	"github.com/ngrok/ngrok-operator/internal/controller"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// TlsSecretReconciler reconciles a Secret object
type TlsSecretReconciler struct {
	client.Client
	Scheme     *runtime.Scheme
	controller *controller.BaseController[*corev1.Secret]

	Log      logr.Logger
	Recorder record.EventRecorder

	// Namespace where the TLS Secret is managed
	Namespace string
}

// TODO(hkatz) figure this out
var DefaultTlsSecret = &corev1.Secret{}

// +kubebuilder:rbac:groups=k8s.ngrok.com,resources=secrets,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=k8s.ngrok.com,resources=secrets/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=k8s.ngrok.com,resources=secrets/finalizers,verbs=update

// SetupWithManager sets up the controller with the Manager.
func (r *TlsSecretReconciler) SetupWithManager(mgr ctrl.Manager) error {
	r.controller = &controller.BaseController[*corev1.Secret]{
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
		// Uncomment the following line adding a pointer to an instance of the controlled resource as an argument
		For(DefaultTlsSecret).
		Complete(r)
}

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the Secret object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.18.4/pkg/reconcile
func (r *TlsSecretReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	// TODO(user): your logic here
	// Implement
	// - Generate CSR + Key
	// - Submit CSR to ngrok CA (API)
	// - Creating a new secret with type=tls
	// - Handle error when re-submitting CSR

	return r.controller.Reconcile(ctx, req, &corev1.Secret{})
}

func (r *TlsSecretReconciler) statusID(cr *corev1.Secret) string {
	return "TODO"
}

func (r *TlsSecretReconciler) create(ctx context.Context, cr *corev1.Secret) error {
	return errors.New("not implemented")
}

func (r *TlsSecretReconciler) update(ctx context.Context, cr *corev1.Secret) error {
	return errors.New("not implemented")
}

func (r *TlsSecretReconciler) delete(ctx context.Context, cr *corev1.Secret) error {
	return errors.New("not implemented")
}

func (r *TlsSecretReconciler) errResult(op controller.BaseControllerOp, cr *corev1.Secret, err error) (ctrl.Result, error) {
	return ctrl.Result{}, err
}
