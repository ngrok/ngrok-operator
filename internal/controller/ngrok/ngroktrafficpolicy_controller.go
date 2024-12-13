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

package ngrok

import (
	"context"

	"github.com/go-logr/logr"
	ngrokv1alpha1 "github.com/ngrok/ngrok-operator/api/ngrok/v1alpha1"
	"github.com/ngrok/ngrok-operator/internal/events"
	"github.com/ngrok/ngrok-operator/internal/util"
	"github.com/ngrok/ngrok-operator/pkg/managerdriver"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
)

// NgrokTrafficPolicyReconciler reconciles a NgrokTrafficPolicy object
type NgrokTrafficPolicyReconciler struct {
	client.Client
	Log      logr.Logger
	Scheme   *runtime.Scheme
	Recorder record.EventRecorder
	Driver   *managerdriver.Driver
}

//+kubebuilder:rbac:groups=ngrok.k8s.ngrok.com,resources=ngroktrafficpolicies,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=ngrok.k8s.ngrok.com,resources=ngroktrafficpolicies/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=ngrok.k8s.ngrok.com,resources=ngroktrafficpolicies/finalizers,verbs=update

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the NgrokTrafficPolicy object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.16.3/pkg/reconcile
func (r *NgrokTrafficPolicyReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	_ = log.FromContext(ctx)

	policy := &ngrokv1alpha1.NgrokTrafficPolicy{}
	if err := r.Get(ctx, req.NamespacedName, policy); err != nil {
		return ctrl.Result{}, err
	}

	parsedTrafficPolicy, err := util.NewTrafficPolicyFromJson(policy.Spec.Policy)
	if err != nil {
		r.Recorder.Eventf(policy, v1.EventTypeWarning, events.TrafficPolicyParseFailed, "Failed to parse Traffic Policy, possibly malformed.")
		return ctrl.Result{}, err
	}

	if parsedTrafficPolicy.IsLegacyPolicy() {
		r.Recorder.Eventf(policy, v1.EventTypeWarning, events.PolicyDeprecation, "Traffic Policy is using legacy directions: ['inbound', 'outbound']. Update to new phases: ['on_tcp_connect', 'on_http_request', 'on_http_response']")
	}

	if parsedTrafficPolicy.Enabled() != nil {
		r.Recorder.Eventf(policy, v1.EventTypeWarning, events.PolicyDeprecation, "Traffic Policy has 'enabled' set. This is a legacy option that will stop being supported soon.")
	}

	err = r.Driver.SyncEdges(ctx, r.Client)
	return ctrl.Result{}, err
}

// SetupWithManager sets up the controller with the Manager.
func (r *NgrokTrafficPolicyReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&ngrokv1alpha1.NgrokTrafficPolicy{}).
		WithEventFilter(predicate.Or(
			predicate.AnnotationChangedPredicate{},
			predicate.GenerationChangedPredicate{},
		)).
		Complete(r)
}
