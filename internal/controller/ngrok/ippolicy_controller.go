/*
MIT License

Copyright (c) 2026 ngrok, Inc.

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
	"fmt"
	"time"

	"github.com/go-logr/logr"
	"github.com/ngrok/ngrok-api-go/v9"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/events"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	ingressv1alpha1 "github.com/ngrok/ngrok-operator/api/ingress/v1alpha1"
	ngrokv1 "github.com/ngrok/ngrok-operator/api/ngrok/v1"
	"github.com/ngrok/ngrok-operator/internal/controller"
	"github.com/ngrok/ngrok-operator/internal/ippolicy"
	"github.com/ngrok/ngrok-operator/internal/ngrokapi"
	"github.com/ngrok/ngrok-operator/internal/trafficpolicy"
	"github.com/ngrok/ngrok-operator/internal/util"
)

const (
	// EventIPPolicyAdopted is emitted on a ngrok.com/v1 IPPolicy when it takes
	// over its legacy twin's remote policy.
	EventIPPolicyAdopted = "Adopted"
	// EventIPPolicyIDConflict is emitted on both twins when they hold
	// different remote IDs. The operator leaves both alone.
	EventIPPolicyIDConflict = "IPPolicyIDConflict"
)

// IPPolicyReconciler reconciles IP policies of both served kinds: the
// canonical ngrok.com/v1 IPPolicy and the deprecated
// ingress.k8s.ngrok.com/v1alpha1 IPPolicy.
//
// The two kinds are reconciled together, keyed by the namespace/name they
// share. A legacy policy and a v1 policy with the same name are "twins": the
// v1 policy is what a user creates when migrating, and it takes over
// ("adopts") the legacy policy's remote ID so traffic policies referencing
// that ID keep working. Because both kinds enqueue the same request key,
// controller-runtime never reconciles a pair concurrently, and every twin
// decision is made in one sequential pass.
//
// Invariants:
//  1. A remote IP policy is deleted only when no surviving object (one that
//     exists and isn't being deleted) holds its ID.
//  2. Exactly one object writes to a given remote ID: the v1 object if it is
//     alive and holds the ID, otherwise the legacy one.
//  3. An adopted ID is persisted on the v1 object before the legacy object's
//     finalizer can be released.
//
// LEGACY-ippolicy-kind: at cleanup, drop the legacy watch, the legacy half of
// ipPolicyPair, and the adopt/passive/shared-ID branches; what remains is a
// single-object reconcile.
type IPPolicyReconciler struct {
	client.Client

	// APIReader reads from the API server, bypassing the cache. It is used
	// only to re-check the twin right before deleting a remote policy, the
	// one step that can't be undone. Defaults to the manager's API reader.
	APIReader client.Reader

	Log      logr.Logger
	Scheme   *runtime.Scheme
	Recorder events.EventRecorder

	IPPoliciesClient    ngrokapi.IPPoliciesClient
	IPPolicyRulesClient ngrokapi.IPPolicyRulesClient
	DrainState          controller.DrainState

	syncer *ippolicy.Syncer
}

// +kubebuilder:rbac:groups=ngrok.com,resources=ippolicies,verbs=get;list;watch;update;patch;delete
// +kubebuilder:rbac:groups=ngrok.com,resources=ippolicies/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=ngrok.com,resources=ippolicies/finalizers,verbs=update;patch
// LEGACY-ippolicy-kind: drop the three ingress.k8s.ngrok.com markers at cleanup.
// +kubebuilder:rbac:groups=ingress.k8s.ngrok.com,resources=ippolicies,verbs=get;list;watch;update;patch;delete
// +kubebuilder:rbac:groups=ingress.k8s.ngrok.com,resources=ippolicies/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=ingress.k8s.ngrok.com,resources=ippolicies/finalizers,verbs=update;patch

// SetupWithManager sets up the controller with the Manager.
func (r *IPPolicyReconciler) SetupWithManager(mgr ctrl.Manager) error {
	if r.IPPoliciesClient == nil {
		return errors.New("IPPoliciesClient must be set")
	}
	if r.IPPolicyRulesClient == nil {
		return errors.New("IPPolicyRulesClient must be set")
	}
	if r.APIReader == nil {
		r.APIReader = mgr.GetAPIReader()
	}
	r.init()

	changed := builder.WithPredicates(predicate.Or(
		predicate.AnnotationChangedPredicate{},
		predicate.GenerationChangedPredicate{},
	))

	return ctrl.NewControllerManagedBy(mgr).
		// Keep the name the legacy-only controller had, so its metrics and
		// log names don't change across the upgrade.
		Named("ippolicy").
		For(&ngrokv1.IPPolicy{}, changed).
		// LEGACY-ippolicy-kind: delete this watch at cleanup. It enqueues the
		// same namespace/name key as the v1 twin, which is what makes the two
		// reconcile as a pair.
		Watches(&ingressv1alpha1.IPPolicy{}, &handler.EnqueueRequestForObject{}, changed).
		Complete(r)
}

func (r *IPPolicyReconciler) init() {
	r.syncer = &ippolicy.Syncer{
		IPPolicies: r.IPPoliciesClient,
		Rules:      r.IPPolicyRulesClient,
		Recorder:   r.Recorder,
	}
}

// Reconcile runs one pass over the pair of IP policies named by req. The
// steps run in a fixed order, and each one sees the pair as the previous step
// left it.
func (r *IPPolicyReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := r.Log.WithValues("ippolicy", req.NamespacedName)
	ctx = ctrl.LoggerInto(ctx, log)

	pair, err := loadIPPolicyPair(ctx, r.Client, req.NamespacedName)
	if err != nil {
		return ctrl.Result{}, err
	}
	if len(pair.all()) == 0 {
		return ctrl.Result{}, nil
	}

	// Like BaseController: while draining, only deletes are processed, so no
	// new finalizers get added and no adoption happens.
	draining := controller.IsDraining(ctx, r.DrainState)
	if draining && len(pair.deleting()) == 0 {
		log.V(1).Info("Draining, skipping non-delete reconcile")
		return ctrl.Result{}, nil
	}

	if !draining {
		for _, p := range pair.surviving() {
			if err := util.RegisterAndSyncFinalizer(ctx, r.Client, p); err != nil {
				return ctrl.Result{}, err
			}
		}

		if err := r.adopt(ctx, pair); err != nil {
			return controller.CtrlResultForErr(err)
		}
	}

	if res, err := r.processDeletes(ctx, pair, draining); err != nil || !res.IsZero() {
		return res, err
	}

	if draining {
		return ctrl.Result{}, nil
	}
	return r.syncSurvivors(ctx, pair)
}

// adopt hands the legacy twin's remote ID to a v1 twin that doesn't have one
// yet, so migrating doesn't create a second remote policy with a new ID. The
// ID is written to v1's status before anything else happens (invariant 3).
// Adoption also runs when the legacy twin is being deleted: re-stamp, apply,
// delete is exactly the migration path.
//
// LEGACY-ippolicy-kind: delete at cleanup.
func (r *IPPolicyReconciler) adopt(ctx context.Context, pair *ipPolicyPair) error {
	v1, legacy := pair.v1, pair.legacy
	if v1 == nil || controller.IsDelete(v1) || v1.GetID() != "" || legacy == nil || legacy.GetID() == "" {
		return nil
	}

	id := legacy.GetID()
	v1.SetID(id)
	r.Recorder.Eventf(v1, legacy, corev1.EventTypeNormal, EventIPPolicyAdopted, "Adopt",
		"Took over IP policy %s from ingress.k8s.ngrok.com/v1alpha1 IPPolicy %s", id, legacy.GetName())
	return r.writeStatus(ctx, v1, nil)
}

// processDeletes handles every object in the pair that is being deleted. The
// remote policy is deleted only when no surviving object holds its ID
// (invariant 1). A non-zero result means "requeue and stop this pass".
func (r *IPPolicyReconciler) processDeletes(ctx context.Context, pair *ipPolicyPair, draining bool) (ctrl.Result, error) {
	log := ctrl.LoggerFrom(ctx)
	deletedIDs := map[string]bool{}

	for _, p := range pair.deleting() {
		if !util.HasFinalizer(p) {
			continue
		}

		if id := p.GetID(); id != "" && !deletedIDs[id] && !pair.survivorHolds(id) {
			blocked, err := r.liveTwinBlocksDelete(ctx, p, draining)
			if err != nil {
				return ctrl.Result{}, err
			}
			if blocked {
				log.Info("A twin the cache hasn't seen yet may still need this remote policy; requeueing before deleting it", "id", id)
				return ctrl.Result{RequeueAfter: time.Second}, nil
			}

			r.Recorder.Eventf(p, nil, corev1.EventTypeNormal, "Deleting", "Delete", "Deleting remote IP policy %s", id)
			if err := r.syncer.Delete(ctx, p); err != nil && !ngrok.IsNotFound(err) {
				r.Recorder.Eventf(p, nil, corev1.EventTypeWarning, "DeleteError", "Delete", "Failed to delete remote IP policy %s: %s", id, err.Error())
				return controller.CtrlResultForErr(err)
			}
			deletedIDs[id] = true
			r.Recorder.Eventf(p, nil, corev1.EventTypeNormal, "Deleted", "Delete", "Deleted remote IP policy %s", id)
		}

		if err := util.RemoveAndSyncFinalizer(ctx, r.Client, p); err != nil {
			return ctrl.Result{}, err
		}
	}
	return ctrl.Result{}, nil
}

// liveTwinBlocksDelete re-reads p's twin from the API server, bypassing the
// cache. The two kinds come from separate informers, so a pass triggered by
// the legacy delete can run before the v1 create reaches the cache. Deleting
// the remote then would strand the v1 twin, so if the live twin holds the ID,
// or (outside a drain) is alive and could still adopt it, the delete waits.
func (r *IPPolicyReconciler) liveTwinBlocksDelete(ctx context.Context, p ippolicy.IPPolicyResource, draining bool) (bool, error) {
	var twin ippolicy.IPPolicyResource
	switch p.(type) {
	case *ngrokv1.IPPolicy:
		twin = &ingressv1alpha1.IPPolicy{}
	default:
		twin = &ngrokv1.IPPolicy{}
	}
	if err := r.APIReader.Get(ctx, client.ObjectKeyFromObject(p), twin); err != nil {
		return false, client.IgnoreNotFound(err)
	}
	if controller.IsDelete(twin) {
		return false, nil
	}
	return twin.GetID() == p.GetID() || (twin.GetID() == "" && !draining), nil
}

// syncSurvivors pushes the spec of each surviving writer to the ngrok API
// (invariant 2), and marks a superseded legacy twin as passive.
func (r *IPPolicyReconciler) syncSurvivors(ctx context.Context, pair *ipPolicyPair) (ctrl.Result, error) {
	v1, legacy := pair.survivingV1(), pair.survivingLegacy()

	if v1 != nil {
		prevID := v1.GetID()
		err := r.syncer.Sync(ctx, v1)

		// The remote policy is gone (deleted outside the operator) and the
		// sync reset v1's status. If the legacy twin points at the same
		// remote, clear it too, or the next pass would adopt the dead ID again.
		// LEGACY-ippolicy-kind: delete this block at cleanup.
		if v1.GetID() == "" && prevID != "" && legacy != nil && legacy.GetID() == prevID {
			legacy.ResetStatus()
			if serr := r.writeStatus(ctx, legacy, nil); serr != nil {
				return controller.CtrlResultForErr(serr)
			}
		}

		if err := r.writeStatus(ctx, v1, err); err != nil {
			return controller.CtrlResultForErr(err)
		}
	}

	if legacy == nil {
		return ctrl.Result{}, nil
	}

	// LEGACY-ippolicy-kind: delete this switch at cleanup.
	switch {
	case v1 != nil && (legacy.GetID() == "" || legacy.GetID() == v1.GetID()):
		r.setPassive(legacy, v1)
		return controller.CtrlResultForErr(r.writeStatus(ctx, legacy, nil))
	case v1 != nil:
		// Both twins hold different IDs, e.g. v1 got its own remote policy
		// before adoption existed. Either ID may be referenced by traffic
		// policies, so neither is touched; both stay writers on their own ID.
		msg := fmt.Sprintf("ngrok.com/v1 IPPolicy %s and ingress.k8s.ngrok.com/v1alpha1 IPPolicy %s manage different remote IP policies (%s and %s). Neither is changed automatically; delete the one you no longer need.",
			v1.GetName(), legacy.GetName(), v1.GetID(), legacy.GetID())
		r.Recorder.Eventf(v1, legacy, corev1.EventTypeWarning, EventIPPolicyIDConflict, "Reconcile", msg)
		r.Recorder.Eventf(legacy, v1, corev1.EventTypeWarning, EventIPPolicyIDConflict, "Reconcile", msg)
	}

	return controller.CtrlResultForErr(r.writeStatus(ctx, legacy, r.syncer.Sync(ctx, legacy)))
}

// setPassive marks a legacy policy whose v1 twin manages the remote policy.
// It stays Ready, since the remote policy is healthy, so alerts on legacy
// Ready=False don't fire mid-migration.
//
// LEGACY-ippolicy-kind: delete at cleanup.
func (r *IPPolicyReconciler) setPassive(legacy *ingressv1alpha1.IPPolicy, v1 *ngrokv1.IPPolicy) {
	ippolicy.SetReadyCondition(legacy, true, ippolicy.ReasonSupersededByV1, fmt.Sprintf(
		"Superseded by ippolicies.ngrok.com %s/%s, which now manages this IP policy. Changes to this object have no effect; delete it once you've migrated.",
		v1.GetNamespace(), v1.GetName()))

	if legacy.GetGeneration() != legacy.GetObservedGeneration() {
		r.Recorder.Eventf(legacy, v1, corev1.EventTypeWarning, trafficpolicy.EventDeprecatedAPIGroup, "Reconcile",
			"ingress.k8s.ngrok.com/v1alpha1 IPPolicy is deprecated and superseded by ippolicies.ngrok.com %s; edit that object instead",
			v1.GetName())
	}
}

// writeStatus writes p's status with BaseController's retry-on-conflict
// logic, and returns origErr (or a StatusError if the write failed).
func (r *IPPolicyReconciler) writeStatus(ctx context.Context, p ippolicy.IPPolicyResource, origErr error) error {
	switch obj := p.(type) {
	case *ngrokv1.IPPolicy:
		return (&controller.BaseController[*ngrokv1.IPPolicy]{Kube: r.Client, Recorder: r.Recorder}).ReconcileStatus(ctx, obj, origErr)
	case *ingressv1alpha1.IPPolicy: // LEGACY-ippolicy-kind: delete this case at cleanup.
		return (&controller.BaseController[*ingressv1alpha1.IPPolicy]{Kube: r.Client, Recorder: r.Recorder}).ReconcileStatus(ctx, obj, origErr)
	default:
		return fmt.Errorf("unsupported IP policy type %T", p)
	}
}

// ipPolicyPair holds the IP policies of both kinds that share one
// namespace/name. Either may be nil.
type ipPolicyPair struct {
	v1 *ngrokv1.IPPolicy
	// LEGACY-ippolicy-kind: delete at cleanup.
	legacy *ingressv1alpha1.IPPolicy
}

func loadIPPolicyPair(ctx context.Context, c client.Reader, key types.NamespacedName) (*ipPolicyPair, error) {
	pair := &ipPolicyPair{}

	v1 := &ngrokv1.IPPolicy{}
	if err := c.Get(ctx, key, v1); err == nil {
		pair.v1 = v1
	} else if client.IgnoreNotFound(err) != nil {
		return nil, err
	}

	legacy := &ingressv1alpha1.IPPolicy{}
	if err := c.Get(ctx, key, legacy); err == nil {
		pair.legacy = legacy
	} else if client.IgnoreNotFound(err) != nil {
		return nil, err
	}

	return pair, nil
}

// all returns the objects that exist, v1 first.
func (p *ipPolicyPair) all() []ippolicy.IPPolicyResource {
	var out []ippolicy.IPPolicyResource
	if p.v1 != nil {
		out = append(out, p.v1)
	}
	if p.legacy != nil {
		out = append(out, p.legacy)
	}
	return out
}

// surviving returns the objects that exist and aren't being deleted.
func (p *ipPolicyPair) surviving() []ippolicy.IPPolicyResource {
	var out []ippolicy.IPPolicyResource
	for _, o := range p.all() {
		if controller.IsUpsert(o) {
			out = append(out, o)
		}
	}
	return out
}

// deleting returns the objects that are being deleted.
func (p *ipPolicyPair) deleting() []ippolicy.IPPolicyResource {
	var out []ippolicy.IPPolicyResource
	for _, o := range p.all() {
		if controller.IsDelete(o) {
			out = append(out, o)
		}
	}
	return out
}

// survivorHolds reports whether a surviving object holds the remote ID id.
func (p *ipPolicyPair) survivorHolds(id string) bool {
	for _, o := range p.surviving() {
		if o.GetID() == id {
			return true
		}
	}
	return false
}

func (p *ipPolicyPair) survivingV1() *ngrokv1.IPPolicy {
	if p.v1 != nil && controller.IsUpsert(p.v1) {
		return p.v1
	}
	return nil
}

func (p *ipPolicyPair) survivingLegacy() *ingressv1alpha1.IPPolicy {
	if p.legacy != nil && controller.IsUpsert(p.legacy) {
		return p.legacy
	}
	return nil
}
