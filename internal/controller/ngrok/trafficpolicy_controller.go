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
	"reflect"

	"github.com/go-logr/logr"
	ngrokv1 "github.com/ngrok/ngrok-operator/api/ngrok/v1"
	"github.com/ngrok/ngrok-operator/internal/util"
	"github.com/ngrok/ngrok-operator/pkg/managerdriver"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/events"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
)

// TrafficPolicyReconciler reconciles a canonical ngrok.com/v1 TrafficPolicy
// object. It mirrors NgrokTrafficPolicyReconciler (which handles the
// deprecated ngrok.k8s.ngrok.com/v1alpha1 NgrokTrafficPolicy) so both kinds
// go through the same parse-and-condition path while the passive migration
// is in flight.
//
// +kubebuilder:rbac:groups=ngrok.com,resources=trafficpolicies,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=ngrok.com,resources=trafficpolicies/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=ngrok.com,resources=trafficpolicies/finalizers,verbs=update
type TrafficPolicyReconciler struct {
	client.Client
	Log      logr.Logger
	Scheme   *runtime.Scheme
	Recorder events.EventRecorder
	Driver   *managerdriver.Driver
}

// Reconcile parses spec.policy, writes the Valid/Ready conditions, and
// triggers an endpoint sync so any endpoint referencing this policy picks up
// the change.
func (r *TrafficPolicyReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	_ = log.FromContext(ctx)

	policy := &ngrokv1.TrafficPolicy{}
	if err := r.Get(ctx, req.NamespacedName, policy); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	prevStatus := *policy.Status.DeepCopy()

	parsedTrafficPolicy, parseErr := util.NewTrafficPolicyFromJson(policy.Spec.Policy)
	setV1TrafficPolicyConditions(policy, parsedTrafficPolicy, parseErr)
	policy.SetObservedGeneration(policy.Generation)

	if !reflect.DeepEqual(prevStatus, policy.Status) {
		if err := r.Status().Update(ctx, policy); err != nil {
			return ctrl.Result{}, err
		}
	}

	if parseErr != nil {
		r.Recorder.Eventf(policy, nil, v1.EventTypeWarning, EventTrafficPolicyParseFailed, "Validate", "Failed to parse Traffic Policy, possibly malformed.")
		// A malformed policy will not fix itself; wait for a spec change.
		return ctrl.Result{}, nil
	}

	if parsedTrafficPolicy.IsLegacyPolicy() {
		r.Recorder.Eventf(policy, nil, v1.EventTypeWarning, EventPolicyDeprecation, "Validate", "Traffic Policy is using legacy directions: ['inbound', 'outbound']. Update to new phases: ['on_tcp_connect', 'on_http_request', 'on_http_response']")
	}

	if parsedTrafficPolicy.Enabled() != nil {
		r.Recorder.Eventf(policy, nil, v1.EventTypeWarning, EventPolicyDeprecation, "Validate", "Traffic Policy has 'enabled' set. This is a legacy option that will stop being supported soon.")
	}

	return managerdriver.HandleSyncResult(r.Driver.SyncEndpoints(ctx, r.Client))
}

// SetupWithManager sets up the controller with the Manager.
func (r *TrafficPolicyReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&ngrokv1.TrafficPolicy{}).
		WithEventFilter(predicate.Or(
			predicate.AnnotationChangedPredicate{},
			predicate.GenerationChangedPredicate{},
		)).
		Complete(r)
}
