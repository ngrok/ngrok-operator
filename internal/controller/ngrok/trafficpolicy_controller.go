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
	"slices"

	"github.com/go-logr/logr"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/events"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	ngrokv1 "github.com/ngrok/ngrok-operator/api/ngrok/v1"
	ngrokv1alpha1 "github.com/ngrok/ngrok-operator/api/ngrok/v1alpha1"
	trafficpolicypkg "github.com/ngrok/ngrok-operator/internal/trafficpolicy"
	"github.com/ngrok/ngrok-operator/internal/util"
	"github.com/ngrok/ngrok-operator/pkg/managerdriver"
)

const (
	EventPolicyDeprecation        = "PolicyDeprecation"
	EventTrafficPolicyParseFailed = "TrafficPolicyParseFailed"
)

// PolicyReconciler reconciles a traffic-policy custom resource of any served
// kind. T is the resource struct and PT its pointer type, constrained to
// implement trafficpolicypkg.TrafficPolicyResource — which is what lets a
// single implementation allocate, fetch, validate, and status-update either
// kind.
//
// The canonical ngrok.com/v1 TrafficPolicy and the deprecated
// ngrok.k8s.ngrok.com/v1alpha1 NgrokTrafficPolicy are structurally identical,
// so reconciling them differently would only ever be a source of drift. The
// two instantiations below are the entire per-kind surface.
//
// +kubebuilder:rbac:groups=ngrok.com,resources=trafficpolicies,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=ngrok.com,resources=trafficpolicies/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=ngrok.com,resources=trafficpolicies/finalizers,verbs=update
type PolicyReconciler[T any, PT trafficpolicypkg.TrafficPolicyResourcePtr[T]] struct {
	client.Client
	Log      logr.Logger
	Scheme   *runtime.Scheme
	Recorder events.EventRecorder
	Driver   *managerdriver.Driver
}

// TrafficPolicyReconciler reconciles the canonical ngrok.com/v1 TrafficPolicy.
type TrafficPolicyReconciler = PolicyReconciler[ngrokv1.TrafficPolicy, *ngrokv1.TrafficPolicy]

// NgrokTrafficPolicyReconciler reconciles the deprecated
// ngrok.k8s.ngrok.com/v1alpha1 NgrokTrafficPolicy. It runs alongside the
// canonical reconciler for the duration of the passive-migration window.
//
// LEGACY-trafficpolicy-kind: delete this alias at cleanup, along with the
// setup block in cmd/api-manager.go that instantiates it. PolicyReconciler
// itself and the canonical alias above are unaffected.
type NgrokTrafficPolicyReconciler = PolicyReconciler[ngrokv1alpha1.NgrokTrafficPolicy, *ngrokv1alpha1.NgrokTrafficPolicy]

// policyStatusSnapshot captures everything a reconcile pass can change about a
// traffic policy's status. Both served kinds define status as exactly
// {observedGeneration, conditions}, so capturing those two fields is
// equivalent to deep-copying the whole struct — and, unlike the struct itself,
// is reachable through TrafficPolicyResource.
type policyStatusSnapshot struct {
	observedGeneration int64
	conditions         []metav1.Condition
}

// snapshotPolicyStatus records the policy's status before it is mutated.
//
// The clone matters: meta.SetStatusCondition updates an existing condition's
// fields in place, so a snapshot sharing the backing array would always
// compare equal afterwards and the reconciler would never write a status
// update. slices.Clone copies the condition structs into a fresh array, and
// metav1.Condition holds only value types, so the copy is fully independent.
func snapshotPolicyStatus(tp trafficpolicypkg.TrafficPolicyResource) policyStatusSnapshot {
	return policyStatusSnapshot{
		observedGeneration: tp.GetObservedGeneration(),
		conditions:         slices.Clone(*tp.GetConditions()),
	}
}

// differsFrom reports whether tp's status has moved away from the snapshot,
// and therefore needs to be persisted. It compares the conditions in full
// rather than just their Status, so a reason/message change that leaves the
// condition True is still detected.
func (s policyStatusSnapshot) differsFrom(tp trafficpolicypkg.TrafficPolicyResource) bool {
	return s.observedGeneration != tp.GetObservedGeneration() ||
		!reflect.DeepEqual(s.conditions, *tp.GetConditions())
}

// Reconcile parses spec.policy, writes the Valid/Ready conditions, and
// triggers an endpoint sync so any endpoint referencing this policy picks up
// the change.
func (r *PolicyReconciler[T, PT]) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	policy := PT(new(T))
	if err := r.Get(ctx, req.NamespacedName, policy); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	prevStatus := snapshotPolicyStatus(policy)

	parsedTrafficPolicy, parseErr := util.NewTrafficPolicyFromJson(policy.GetPolicy())
	setTrafficPolicyConditions(policy, parsedTrafficPolicy, parseErr)
	policy.SetObservedGeneration(policy.GetGeneration())

	if prevStatus.differsFrom(policy) {
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

// SetupWithManager sets up the controller with the Manager. The controller
// name is derived from T's kind, so the two instantiations register under
// distinct names ("trafficpolicy" and "ngroktrafficpolicy") and do not collide.
func (r *PolicyReconciler[T, PT]) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(PT(new(T))).
		WithEventFilter(predicate.Or(
			predicate.AnnotationChangedPredicate{},
			predicate.GenerationChangedPredicate{},
		)).
		Complete(r)
}
