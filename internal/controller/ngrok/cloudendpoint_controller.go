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
	"fmt"
	"time"

	"github.com/go-logr/logr"
	"github.com/ngrok/ngrok-api-go/v9"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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
	ngrokv1alpha1 "github.com/ngrok/ngrok-operator/api/ngrok/v1alpha1"
	"github.com/ngrok/ngrok-operator/internal/cloudendpoint"
	"github.com/ngrok/ngrok-operator/internal/controller"
	"github.com/ngrok/ngrok-operator/internal/controller/labels"
	domainpkg "github.com/ngrok/ngrok-operator/internal/domain"
	"github.com/ngrok/ngrok-operator/internal/ngrokapi"
	trafficpolicypkg "github.com/ngrok/ngrok-operator/internal/trafficpolicy"
	"github.com/ngrok/ngrok-operator/internal/util"
)

const (
	// EventCloudEndpointAdopted is emitted on a ngrok.com/v1 CloudEndpoint
	// when it takes over its legacy twin's remote endpoint.
	EventCloudEndpointAdopted = "CloudEndpointAdopted"
	// EventCloudEndpointIDConflict is emitted on both twins when they hold
	// different remote IDs. Neither remote endpoint is deleted.
	//
	// LEGACY-cloudendpoint-kind: delete at cleanup.
	EventCloudEndpointIDConflict = "CloudEndpointIDConflict"
)

// CloudEndpointReconciler reconciles cloud endpoints of both served kinds: the
// canonical ngrok.com/v1 CloudEndpoint and the deprecated
// ngrok.k8s.ngrok.com/v1alpha1 CloudEndpoint.
//
// The two kinds are reconciled together, keyed by the namespace/name they
// share, never by URL: pooling lets distinct objects share a URL, and each
// keeps its own endpoint. A legacy and a v1 CloudEndpoint with the same name
// are "twins": the v1 object takes over ("adopts") the legacy object's remote
// ID, so migrating doesn't create a second, separately billed endpoint.
// Because both kinds enqueue the same request key, controller-runtime never
// reconciles a pair concurrently.
//
// This implements the twin rules in specs/crd-group-migration.md. In short:
//  1. A remote endpoint is deleted only when no surviving object (one that
//     exists and isn't being deleted) holds its ID, and the check fails
//     closed (see cloudEndpointTwinDecision).
//  2. While a v1 object is alive, it is the only one that calls the ngrok
//     API. The legacy twin is passive, even if it holds a different ID.
//  3. An adopted ID is persisted on the v1 object, which already has its
//     finalizer, before any other step, so before the legacy object's
//     finalizer can be released.
//
// LEGACY-cloudendpoint-kind: in 1.0 (R2 in specs/crd-group-migration.md),
// drop the legacy watch, index and list halves, the legacy half of
// cloudEndpointPair, and the adopt/passive/twin-check branches. What remains
// is a single-object reconcile, plus R2's one-way late adoption of a legacy ID.
type CloudEndpointReconciler struct {
	client.Client

	// APIReader reads from the API server, bypassing the cache. It is used
	// only to re-check the twin right before deleting a remote endpoint, the
	// one step that can't be undone. Defaults to the manager's API reader.
	//
	// LEGACY-cloudendpoint-kind: delete at cleanup.
	APIReader client.Reader

	Scheme *runtime.Scheme
	syncer *cloudendpoint.Syncer

	Log            logr.Logger
	Recorder       events.EventRecorder
	NgrokClientset ngrokapi.Clientset
	DrainState     controller.DrainState

	ControllerLabels           labels.ControllerLabelValues
	DefaultDomainReclaimPolicy *ngrokv1.DomainReclaimPolicy
	DomainManager              *domainpkg.Manager
	TrafficPolicyManager       *trafficpolicypkg.Manager
}

// +kubebuilder:rbac:groups=ngrok.com,resources=cloudendpoints,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=ngrok.com,resources=cloudendpoints/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=ngrok.com,resources=cloudendpoints/finalizers,verbs=update;patch
// LEGACY-cloudendpoint-kind: drop the three ngrok.k8s.ngrok.com markers at cleanup.
// +kubebuilder:rbac:groups=ngrok.k8s.ngrok.com,resources=cloudendpoints,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=ngrok.k8s.ngrok.com,resources=cloudendpoints/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=ngrok.k8s.ngrok.com,resources=cloudendpoints/finalizers,verbs=update;patch

// SetupWithManager sets up the controller with the Manager.
// It also sets up a Field Indexer to index Cloud Endpoints by their Traffic Policy name.
func (r *CloudEndpointReconciler) SetupWithManager(mgr ctrl.Manager) error {
	if r.NgrokClientset == nil {
		return errors.New("NgrokClientset is required")
	}
	if r.APIReader == nil {
		r.APIReader = mgr.GetAPIReader()
	}

	// Initialize domain manager if not already set
	if r.DomainManager == nil {
		if err := labels.ValidateControllerLabelValues(r.ControllerLabels); err != nil {
			return err
		}

		opts := []domainpkg.ManagerOption{
			domainpkg.WithControllerLabels(r.ControllerLabels),
			domainpkg.WithAPIReader(mgr.GetAPIReader()),
		}

		if r.DefaultDomainReclaimPolicy != nil {
			opts = append(opts, domainpkg.WithDefaultDomainReclaimPolicy(*r.DefaultDomainReclaimPolicy))
		}

		dm, err := domainpkg.NewManager(r.Client, r.Recorder, opts...)
		if err != nil {
			return err
		}
		r.DomainManager = dm
	}

	if r.TrafficPolicyManager == nil {
		r.TrafficPolicyManager = trafficpolicypkg.NewManager(r.Client, r.Recorder)
	}

	r.syncer = &cloudendpoint.Syncer{
		Endpoints:       r.NgrokClientset.Endpoints(),
		Domains:         r.DomainManager,
		TrafficPolicies: r.TrafficPolicyManager,
		Recorder:        r.Recorder,
	}

	kinds := []client.Object{
		&ngrokv1.CloudEndpoint{},
		&ngrokv1alpha1.CloudEndpoint{}, // LEGACY-cloudendpoint-kind: delete at cleanup.
	}
	for _, kind := range kinds {
		if err := mgr.GetFieldIndexer().IndexField(context.Background(), kind, trafficpolicypkg.RefIndex, indexCloudEndpointTrafficPolicyRefs); err != nil {
			return err
		}
	}

	changed := builder.WithPredicates(predicate.Or(
		predicate.AnnotationChangedPredicate{},
		predicate.GenerationChangedPredicate{},
	))

	return ctrl.NewControllerManagedBy(mgr).
		// Keep the name the legacy-only controller had, so its metrics and
		// log names don't change across the upgrade.
		Named("cloudendpoint").
		For(&ngrokv1.CloudEndpoint{}, changed).
		// LEGACY-cloudendpoint-kind: delete this watch at cleanup. It enqueues
		// the same namespace/name key as the v1 twin, which is what makes the
		// two reconcile as a pair.
		Watches(&ngrokv1alpha1.CloudEndpoint{}, &handler.EnqueueRequestForObject{}, changed).
		// LEGACY-trafficpolicy-kind: BEGIN
		// Watch the deprecated ngrok.k8s.ngrok.com/v1alpha1 NgrokTrafficPolicy
		// so endpoints resolving through the fallback path re-enqueue on
		// legacy-kind updates. Delete this Watches call at cleanup.
		Watches(
			&ngrokv1alpha1.NgrokTrafficPolicy{},
			r.enqueueFor(r.findCloudEndpointForTrafficPolicy),
		).
		// LEGACY-trafficpolicy-kind: END
		Watches(
			// Canonical ngrok.com/v1 TrafficPolicy. Same mapper — the index
			// key is namespace/name only, so both kinds enqueue identically.
			&ngrokv1.TrafficPolicy{},
			r.enqueueFor(r.findCloudEndpointForTrafficPolicy),
		).
		Watches(
			&ngrokv1.Domain{},
			r.enqueueFor(r.findCloudEndpointsForDomain),
		).
		// LEGACY-domain-kind: delete this Watches call at cleanup.
		Watches(
			&ingressv1alpha1.Domain{},
			r.enqueueFor(r.findCloudEndpointsForDomain),
		).
		Complete(r)
}

// enqueueFor wraps a map function so it gets the controller's logger from its
// context.
func (r *CloudEndpointReconciler) enqueueFor(f handler.MapFunc) handler.EventHandler {
	return handler.EnqueueRequestsFromMapFunc(func(ctx context.Context, obj client.Object) []ctrl.Request {
		return f(ctrl.LoggerInto(ctx, r.Log), obj)
	})
}

// indexCloudEndpointTrafficPolicyRefs returns the composite key for the
// TrafficPolicy this CloudEndpoint depends on so updates to that policy
// requeue the endpoint. Inline-only CloudEndpoints produce no index entry
// because they have no external ref to watch.
func indexCloudEndpointTrafficPolicyRefs(o client.Object) []string {
	clep, ok := o.(cloudendpoint.Resource)
	if !ok {
		return nil
	}
	if k := trafficpolicypkg.IndexKey(clep); k != "" {
		return []string{k}
	}
	return nil
}

// #region Reconcile

// Reconcile runs one pass over the pair of CloudEndpoints named by req. The
// steps run in a fixed order, and each one sees the pair as the previous step
// left it.
func (r *CloudEndpointReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := r.Log.WithValues("cloudendpoint", req.NamespacedName)
	ctx = ctrl.LoggerInto(ctx, log)

	pair, err := loadCloudEndpointPair(ctx, r.Client, req.NamespacedName)
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
		// Finalizers go on before adoption, so v1 never holds an adopted ID
		// without one. Deleting such a v1 object would skip processDeletes,
		// and a legacy delete pass that still sees it in the cache would keep
		// the remote endpoint and release the legacy finalizer, orphaning it.
		for _, ep := range pair.surviving() {
			if err := util.RegisterAndSyncFinalizer(ctx, r.Client, ep); err != nil {
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
// yet, so migrating doesn't create a second endpoint. The ID is written to
// v1's status right after its finalizer and before anything else happens
// (invariant 3). Adoption also runs
// when the legacy twin is being deleted: re-stamp, apply, delete is exactly
// the migration path.
//
// LEGACY-cloudendpoint-kind: delete at cleanup.
func (r *CloudEndpointReconciler) adopt(ctx context.Context, pair *cloudEndpointPair) error {
	canonical, legacy := pair.v1, pair.legacy
	if canonical == nil || controller.IsDelete(canonical) || canonical.GetID() != "" || legacy == nil || legacy.GetID() == "" {
		return nil
	}

	return r.adoptID(ctx, canonical, legacy)
}

// adoptID gives canonical its legacy twin's remote ID and persists it.
//
// The write is a plain status update, not writeStatus: on a conflict,
// writeStatus re-applies the in-memory status over the newer one, which could
// replace an ID v1 got in the meantime and orphan that billed endpoint. A
// conflict is returned instead, so the pass requeues and re-reads the pair.
//
// LEGACY-cloudendpoint-kind: delete at cleanup.
func (r *CloudEndpointReconciler) adoptID(ctx context.Context, canonical *ngrokv1.CloudEndpoint, legacy *ngrokv1alpha1.CloudEndpoint) error {
	prevID, id := canonical.GetID(), legacy.GetID()
	canonical.SetID(id)
	canonical.SetAssignedURL(legacy.Status.AssignedURL)
	if err := r.Client.Status().Update(ctx, canonical); err != nil {
		return fmt.Errorf("persisting the adopted remote endpoint %s: %w", id, err)
	}

	msg := fmt.Sprintf("Took over cloud endpoint %s from ngrok.k8s.ngrok.com/v1alpha1 CloudEndpoint %s", id, legacy.GetName())
	if prevID != "" {
		msg += fmt.Sprintf(", replacing %s, which no longer exists", prevID)
	}
	r.Recorder.Eventf(canonical, legacy, v1.EventTypeNormal, EventCloudEndpointAdopted, "Adopt", msg)
	return nil
}

// processDeletes handles every object in the pair that is being deleted. The
// remote endpoint is deleted only when no surviving object holds its ID
// (invariant 1). A non-zero result means "requeue and stop this pass".
func (r *CloudEndpointReconciler) processDeletes(ctx context.Context, pair *cloudEndpointPair, draining bool) (ctrl.Result, error) {
	log := ctrl.LoggerFrom(ctx)
	deletedIDs := map[string]bool{}

	for _, ep := range pair.deleting() {
		if !util.HasFinalizer(ep) {
			continue
		}

		// An endpoint deleted right after it was created can reach here before the cache has the
		// status write that recorded its ID. Without the ID the remote endpoint would be left
		// behind, holding its URL, so read the object from the API server before giving up on it.
		if ep.GetID() == "" {
			if id, err := r.freshID(ctx, ep); err != nil {
				return ctrl.Result{}, err
			} else if id != "" {
				ep.SetID(id)
			}
		}

		if id := ep.GetID(); id != "" && !deletedIDs[id] && !pair.survivorHolds(id) {
			// LEGACY-cloudendpoint-kind: BEGIN
			// The two kinds come from separate informers, so the cached twin
			// can be stale. Re-read it from the API server before the one
			// irreversible step.
			twin := twinOf(ep)
			err := r.APIReader.Get(ctx, client.ObjectKeyFromObject(ep), twin)
			switch cloudEndpointTwinDecision(err, twin, id, draining) {
			case twinRequeue:
				return ctrl.Result{}, fmt.Errorf("re-reading the twin of %s before deleting its remote endpoint: %w", ep.GetName(), err)
			case twinWait:
				log.Info("A v1 twin the cache hasn't seen yet may still adopt this remote endpoint; requeueing before deleting it", "id", id)
				return ctrl.Result{RequeueAfter: time.Second}, nil
			case twinKeepRemote:
				log.Info("A live twin holds, or may hold, this remote endpoint; releasing the finalizer without deleting it", "id", id)
				if err := util.RemoveAndSyncFinalizer(ctx, r.Client, ep); err != nil {
					return ctrl.Result{}, err
				}
				continue
			case twinDeleteRemote:
			}
			// LEGACY-cloudendpoint-kind: END

			name := util.ObjToHumanName(ep)
			r.Recorder.Eventf(ep, nil, v1.EventTypeNormal, "Deleting", "Delete", fmt.Sprintf("Deleting %s", name))
			if err := r.syncer.Delete(ctx, ep); err != nil && !ngrok.IsNotFound(err) {
				r.Recorder.Eventf(ep, nil, v1.EventTypeWarning, "DeleteError", "Delete", fmt.Sprintf("Failed to delete %s: %s", name, err.Error()))
				return r.errResult(ep, err)
			}
			deletedIDs[id] = true
			r.Recorder.Eventf(ep, nil, v1.EventTypeNormal, "Deleted", "Delete", fmt.Sprintf("Deleted %s", name))
		}

		if err := util.RemoveAndSyncFinalizer(ctx, r.Client, ep); err != nil {
			return ctrl.Result{}, err
		}
	}
	return ctrl.Result{}, nil
}

// freshID returns the ID recorded on ep's latest version in the API server, or "" when it has none or
// is already gone.
func (r *CloudEndpointReconciler) freshID(ctx context.Context, ep cloudendpoint.Resource) (string, error) {
	fresh, ok := ep.DeepCopyObject().(cloudendpoint.Resource)
	if !ok {
		return "", nil
	}
	if err := r.APIReader.Get(ctx, client.ObjectKeyFromObject(ep), fresh); err != nil {
		return "", client.IgnoreNotFound(err)
	}
	return fresh.GetID(), nil
}

// twinOf returns an empty object of the other kind, to read ep's twin into.
//
// LEGACY-cloudendpoint-kind: delete at cleanup.
func twinOf(ep cloudendpoint.Resource) cloudendpoint.Resource {
	if _, ok := ep.(*ngrokv1alpha1.CloudEndpoint); ok {
		return &ngrokv1.CloudEndpoint{}
	}
	return &ngrokv1alpha1.CloudEndpoint{}
}

// cloudEndpointTwinAction is what to do with a deleted object's remote
// endpoint, given the uncached read of its twin.
//
// LEGACY-cloudendpoint-kind: delete at cleanup.
type cloudEndpointTwinAction int

const (
	// twinDeleteRemote: no live twin holds the ID.
	twinDeleteRemote cloudEndpointTwinAction = iota
	// twinKeepRemote: a live twin holds the ID, or might (the read was
	// Forbidden). Release the finalizer without an ngrok call.
	twinKeepRemote
	// twinWait: a live v1 twin has no ID yet and may still adopt this one.
	twinWait
	// twinRequeue: the twin couldn't be read. Try again.
	twinRequeue
)

// cloudEndpointTwinDecision decides what happens to the remote endpoint id of
// a deleted object, from the uncached read of its twin (err, twin). The read
// is interpreted strictly (fail closed):
//   - NotFound, or the twin's kind isn't served (NoMatch): no twin.
//   - Forbidden: treat the twin as present. On rollback, RBAC reverts before
//     the CRDs do, so this happens in practice.
//   - Any other error: requeue.
//
// LEGACY-cloudendpoint-kind: delete at cleanup.
func cloudEndpointTwinDecision(err error, twin cloudendpoint.Resource, id string, draining bool) cloudEndpointTwinAction {
	switch {
	case apierrors.IsNotFound(err), meta.IsNoMatchError(err):
		return twinDeleteRemote
	case apierrors.IsForbidden(err):
		return twinKeepRemote
	case err != nil:
		return twinRequeue
	}

	if controller.IsDelete(twin) {
		return twinDeleteRemote
	}
	if twin.GetID() == id {
		return twinKeepRemote
	}
	// Only a v1 twin adopts, so only a legacy delete waits for one. Never wait
	// while draining: drain doesn't adopt.
	if _, twinIsV1 := twin.(*ngrokv1.CloudEndpoint); twinIsV1 && twin.GetID() == "" && !draining {
		return twinWait
	}
	return twinDeleteRemote
}

// syncSurvivors pushes the spec of the surviving writer to the ngrok API (v1
// while it's alive, otherwise legacy), and marks a superseded legacy twin as
// passive.
func (r *CloudEndpointReconciler) syncSurvivors(ctx context.Context, pair *cloudEndpointPair) (ctrl.Result, error) {
	canonical, legacy := pair.survivingV1(), pair.survivingLegacy()
	if canonical == nil {
		if legacy == nil {
			return ctrl.Result{}, nil
		}
		return r.errResult(legacy, r.sync(ctx, legacy))
	}

	// LEGACY-cloudendpoint-kind: delete this block at cleanup.
	if legacy != nil {
		if err := r.adoptOverDeadID(ctx, canonical, legacy); err != nil {
			return controller.CtrlResultForErr(err)
		}
	}

	err := r.sync(ctx, canonical)

	// LEGACY-cloudendpoint-kind: delete this block at cleanup.
	// When v1's status write failed, its new state isn't persisted, so the
	// legacy twin doesn't follow it this pass.
	if _, statusFailed := errors.AsType[controller.StatusError](err); legacy != nil && !statusFailed {
		if werr := r.syncPassive(ctx, legacy, canonical); werr != nil && err == nil {
			return controller.CtrlResultForErr(werr)
		}
	}

	return r.errResult(canonical, err)
}

// adoptOverDeadID hands v1 its legacy twin's remote ID when they differ and
// v1's own remote endpoint no longer exists, instead of letting the sync
// create a new one. This happens after a rollback: the old operator creates a
// new endpoint for the legacy object, then the roll-forward finds v1 still
// holding the ID of one that was since deleted. If the legacy endpoint is
// gone too, nothing is adopted and the sync's normal recreate path runs.
//
// LEGACY-cloudendpoint-kind: delete at cleanup.
func (r *CloudEndpointReconciler) adoptOverDeadID(ctx context.Context, canonical *ngrokv1.CloudEndpoint, legacy *ngrokv1alpha1.CloudEndpoint) error {
	if canonical.GetID() == "" || legacy.GetID() == "" || legacy.GetID() == canonical.GetID() {
		return nil
	}
	gone, err := r.remoteGone(ctx, canonical.GetID())
	if err != nil || !gone {
		return err
	}
	if gone, err := r.remoteGone(ctx, legacy.GetID()); err != nil || gone {
		return err
	}
	return r.adoptID(ctx, canonical, legacy)
}

// remoteGone reports whether the ngrok API says the endpoint id doesn't exist.
//
// LEGACY-cloudendpoint-kind: delete at cleanup.
func (r *CloudEndpointReconciler) remoteGone(ctx context.Context, id string) (bool, error) {
	_, err := r.syncer.Endpoints.Get(ctx, id)
	if ngrok.IsNotFound(err) {
		return true, nil
	}
	return false, err
}

// syncPassive updates a legacy twin's status while v1 is the writer. The
// status is written only when
// it changed: every v1 reconcile gets here, and each legacy request draws a
// deprecation warning from the API server.
//
// The write is a plain status update, not writeStatus, for the reason given
// on adoptID: legacy may come from a cache that hasn't seen the ID its own
// reconcile just wrote, and writeStatus would put the stale status back over
// it on a conflict, orphaning the endpoint. A conflict requeues the pass
// instead.
//
// LEGACY-cloudendpoint-kind: delete at cleanup.
func (r *CloudEndpointReconciler) syncPassive(ctx context.Context, legacy *ngrokv1alpha1.CloudEndpoint, canonical *ngrokv1.CloudEndpoint) error {
	before := legacy.Status.DeepCopy()

	if id := legacy.GetID(); id != "" && id != canonical.GetID() {
		// The twins hold different IDs. This is derived from the remote state
		// on every pass rather than from what this pass's sync changed, so a
		// failed legacy write is repaired on the next pass.
		gone, err := r.remoteGone(ctx, id)
		if err != nil {
			return err
		}
		if gone {
			// Legacy points at a dead endpoint, e.g. v1 replaced one deleted
			// outside the operator. Legacy follows v1 (or clears its ID, if v1
			// has none), so the dead ID is never adopted again and a rollback
			// reuses v1's endpoint instead of creating yet another one.
			legacy.SetID(canonical.GetID())
			legacy.SetAssignedURL(canonical.Status.AssignedURL)
		} else if canonical.GetID() != "" {
			// Both endpoints exist, e.g. both were created before the v1
			// object could adopt. Either may be serving traffic, so neither is
			// deleted. v1 is still the only writer, so the legacy one is left
			// unmanaged until a twin is deleted.
			msg := fmt.Sprintf("ngrok.com/v1 CloudEndpoint %s and ngrok.k8s.ngrok.com/v1alpha1 CloudEndpoint %s hold different remote endpoints (%s and %s). Only %s is managed; neither is deleted automatically. Delete the object whose endpoint you no longer need.",
				canonical.GetName(), legacy.GetName(), canonical.GetID(), id, canonical.GetID())
			r.Recorder.Eventf(canonical, legacy, v1.EventTypeWarning, EventCloudEndpointIDConflict, "Reconcile", msg)
			r.Recorder.Eventf(legacy, canonical, v1.EventTypeWarning, EventCloudEndpointIDConflict, "Reconcile", msg)
		}
	}

	r.setPassive(legacy, canonical)
	legacy.Status.ObservedGeneration = legacy.Generation
	if equality.Semantic.DeepEqual(before, &legacy.Status) {
		return nil
	}
	return r.Client.Status().Update(ctx, legacy)
}

// setPassive marks a legacy CloudEndpoint whose v1 twin manages the remote
// endpoint. It stays Ready, since the endpoint is served, so alerts on legacy
// Ready=False don't fire mid-migration. The rest of its status is left as the
// legacy object last wrote it.
//
// LEGACY-cloudendpoint-kind: delete at cleanup.
func (r *CloudEndpointReconciler) setPassive(legacy *ngrokv1alpha1.CloudEndpoint, canonical *ngrokv1.CloudEndpoint) {
	ready := meta.FindStatusCondition(legacy.Status.Conditions, cloudendpoint.ConditionCloudEndpointReady)
	becomingPassive := ready == nil || ready.Reason != cloudendpoint.ReasonSupersededByV1

	cloudendpoint.SetReadyCondition(legacy, true, cloudendpoint.ReasonSupersededByV1, fmt.Sprintf(
		"Superseded by cloudendpoints.ngrok.com %s/%s, which now manages this endpoint. Changes to this object have no effect; delete it once you've migrated.",
		canonical.GetNamespace(), canonical.GetName()))

	// Operator-generated objects are migrated by the operator; the user can't
	// act on a warning about them.
	if isOperatorManagedCloudEndpoint(legacy) {
		return
	}
	if becomingPassive || legacy.GetGeneration() != legacy.Status.ObservedGeneration {
		r.Recorder.Eventf(legacy, canonical, v1.EventTypeWarning, trafficpolicypkg.EventDeprecatedAPIGroup, "Reconcile",
			"ngrok.k8s.ngrok.com/v1alpha1 CloudEndpoint is deprecated and superseded by cloudendpoints.ngrok.com %s; edit that object instead",
			canonical.GetName())
	}
}

// isOperatorManagedCloudEndpoint reports whether the operator generated ep,
// either from an Ingress or Gateway (a controller-name label) or from a
// Service (controller owner reference).
//
// LEGACY-cloudendpoint-kind: delete at cleanup.
func isOperatorManagedCloudEndpoint(ep client.Object) bool {
	l := ep.GetLabels()
	if l[labels.ControllerName] != "" || l[labels.LegacyControllerName] != "" {
		return true
	}
	owner := metav1.GetControllerOf(ep)
	return owner != nil && owner.Kind == "Service"
}

// sync creates or updates ep's remote endpoint and writes ep's status. It
// returns the sync error, or a StatusError if the status write failed.
func (r *CloudEndpointReconciler) sync(ctx context.Context, ep cloudendpoint.Resource) error {
	name := util.ObjToHumanName(ep)
	if ep.GetID() == "" {
		r.Recorder.Eventf(ep, nil, v1.EventTypeNormal, "Creating", "Create", fmt.Sprintf("Creating %s", name))
		if err := r.writeStatus(ctx, ep, r.syncer.Sync(ctx, ep)); err != nil {
			r.Recorder.Eventf(ep, nil, v1.EventTypeWarning, "CreateError", "Create", fmt.Sprintf("Failed to Create %s: %s", name, err.Error()))
			return err
		}
		r.Recorder.Eventf(ep, nil, v1.EventTypeNormal, "Created", "Create", fmt.Sprintf("Created %s", name))
		return nil
	}

	r.Recorder.Eventf(ep, nil, v1.EventTypeNormal, "Updating", "Update", fmt.Sprintf("Updating %s", name))
	if err := r.writeStatus(ctx, ep, r.syncer.Sync(ctx, ep)); err != nil {
		r.Recorder.Eventf(ep, nil, v1.EventTypeWarning, "UpdateError", "Update", fmt.Sprintf("Failed to update %s: %s", name, err.Error()))
		return err
	}
	r.Recorder.Eventf(ep, nil, v1.EventTypeNormal, "Updated", "Update", fmt.Sprintf("Updated %s", name))
	return nil
}

// errResult maps a sync or delete error to the reconcile result.
func (r *CloudEndpointReconciler) errResult(ep cloudendpoint.Resource, err error) (ctrl.Result, error) {
	retryableErrors := []int{
		// 18016 and 18017 are state based errors that can happen when endpoint pooling for a given URL
		// disagrees with an already active endpoint with the same URL. Since this state can change in ngrok when moving
		// between agent and cloud endpoints, we need to retry on this 400, instead of assuming its terminal like we
		// do for other 400s.
		//
		// Ref:
		//  * https://ngrok.com/docs/errors/err_ngrok_18016/
		//  * https://ngrok.com/docs/errors/err_ngrok_18017/
		18016,
		18017,
	}
	if ngrok.IsErrorCode(err, retryableErrors...) {
		// The conflicting endpoint is usually one the operator is replacing for the same URL, such as
		// the public AgentEndpoint a hostname used before it got a CloudEndpoint, and goes away within
		// seconds. Retry on a short fixed delay rather than the growing error backoff, which would
		// stretch the gap in service each time the retry comes too early.
		r.Log.Info("another endpoint still holds this URL; retrying", "name", ep.GetName(), "namespace", ep.GetNamespace(), "error", err.Error())
		return ctrl.Result{RequeueAfter: 2 * time.Second}, nil
	}
	if errors.Is(err, domainpkg.ErrDomainNotReady) {
		return ctrl.Result{RequeueAfter: 10 * time.Second}, nil
	}
	if errors.Is(err, trafficpolicypkg.ErrInvalidConfig) || errors.Is(err, trafficpolicypkg.ErrInvalidPolicyJSON) {
		r.Recorder.Eventf(ep, nil, v1.EventTypeWarning, "ConfigError", "Reconcile", err.Error())
		r.Log.Error(err, "invalid TrafficPolicy configuration", "name", ep.GetName(), "namespace", ep.GetNamespace())
		return ctrl.Result{}, nil // Do not requeue
	}
	if errors.Is(err, trafficpolicypkg.ErrTrafficPolicyNotFound) {
		// Terminal: the condition is already False and an event was
		// emitted during Resolve. Don't requeue — the TrafficPolicy
		// watch re-enqueues this endpoint when the policy is (re)created.
		r.Log.Info("referenced TrafficPolicy not found; awaiting (re)creation", "name", ep.GetName(), "namespace", ep.GetNamespace())
		return ctrl.Result{}, nil
	}
	return controller.CtrlResultForErr(err)
}

// writeStatus writes ep's status with BaseController's retry-on-conflict
// logic, and returns origErr (or a StatusError if the write failed).
func (r *CloudEndpointReconciler) writeStatus(ctx context.Context, ep cloudendpoint.Resource, origErr error) error {
	switch obj := ep.(type) {
	case *ngrokv1.CloudEndpoint:
		return (&controller.BaseController[*ngrokv1.CloudEndpoint]{Kube: r.Client, Recorder: r.Recorder}).ReconcileStatus(ctx, obj, origErr)
	case *ngrokv1alpha1.CloudEndpoint: // LEGACY-cloudendpoint-kind: delete this case at cleanup.
		return (&controller.BaseController[*ngrokv1alpha1.CloudEndpoint]{Kube: r.Client, Recorder: r.Recorder}).ReconcileStatus(ctx, obj, origErr)
	default:
		return fmt.Errorf("unsupported CloudEndpoint type %T", ep)
	}
}

// #region Helper Functions

// listCloudEndpoints lists the CloudEndpoints of both kinds.
func (r *CloudEndpointReconciler) listCloudEndpoints(ctx context.Context, opts ...client.ListOption) ([]cloudendpoint.Resource, error) {
	var out []cloudendpoint.Resource

	var v1List ngrokv1.CloudEndpointList
	if err := r.Client.List(ctx, &v1List, opts...); err != nil {
		return nil, err
	}
	for i := range v1List.Items {
		out = append(out, &v1List.Items[i])
	}

	// LEGACY-cloudendpoint-kind: BEGIN
	var legacyList ngrokv1alpha1.CloudEndpointList
	if err := r.Client.List(ctx, &legacyList, opts...); err != nil {
		return nil, err
	}
	for i := range legacyList.Items {
		out = append(out, &legacyList.Items[i])
	}
	// LEGACY-cloudendpoint-kind: END

	return out, nil
}

// findCloudEndpointForTrafficPolicy returns reconcile requests for every
// CloudEndpoint that references the supplied TrafficPolicy via
// spec.trafficPolicy.targetRef. A twin pair maps to one namespace/name key,
// so it may appear twice; the workqueue deduplicates it.
//
// The mapper is kind-agnostic by construction: it accepts anything satisfying
// trafficpolicypkg.TrafficPolicyResource, which today means both the canonical
// ngrok.com/v1 TrafficPolicy and the deprecated
// ngrok.k8s.ngrok.com/v1alpha1 NgrokTrafficPolicy. That works because the
// endpoint index key is namespace/name only, with no kind component. Testing
// against the interface rather than a list of concrete types means this
// function needs no edit when the legacy kind is dropped.
func (r *CloudEndpointReconciler) findCloudEndpointForTrafficPolicy(ctx context.Context, o client.Object) []ctrl.Request {
	if _, ok := o.(trafficpolicypkg.TrafficPolicyResource); !ok {
		return nil
	}

	endpoints, err := r.listCloudEndpoints(ctx, client.MatchingFields{trafficpolicypkg.RefIndex: trafficpolicypkg.LookupKey(o)})
	if err != nil {
		r.Log.Error(err, "failed to list CloudEndpoints using index")
		return nil
	}

	requests := make([]ctrl.Request, 0, len(endpoints))
	for _, ep := range endpoints {
		requests = append(requests, ctrl.Request{NamespacedName: client.ObjectKeyFromObject(ep)})
	}
	return requests
}

// findCloudEndpointsForDomain searches for any CloudEndpoint CRs that reference a particular Domain
func (r *CloudEndpointReconciler) findCloudEndpointsForDomain(ctx context.Context, domain client.Object) []ctrl.Request {
	if _, ok := domainpkg.HostnameOf(domain); !ok {
		return nil
	}

	endpoints, err := r.listCloudEndpoints(ctx, client.InNamespace(domain.GetNamespace()))
	if err != nil {
		return nil
	}

	var requests []ctrl.Request
	for _, ep := range endpoints {
		if ep.GetDomainRef().Matches(domain) {
			requests = append(requests, ctrl.Request{
				NamespacedName: client.ObjectKeyFromObject(ep),
			})
		}
	}
	return requests
}

// cloudEndpointPair holds the CloudEndpoints of both kinds that share one
// namespace/name. Either may be nil.
type cloudEndpointPair struct {
	v1 *ngrokv1.CloudEndpoint
	// LEGACY-cloudendpoint-kind: delete at cleanup.
	legacy *ngrokv1alpha1.CloudEndpoint
}

func loadCloudEndpointPair(ctx context.Context, c client.Reader, key types.NamespacedName) (*cloudEndpointPair, error) {
	pair := &cloudEndpointPair{}

	canonical := &ngrokv1.CloudEndpoint{}
	if err := c.Get(ctx, key, canonical); err == nil {
		pair.v1 = canonical
	} else if client.IgnoreNotFound(err) != nil {
		return nil, err
	}

	legacy := &ngrokv1alpha1.CloudEndpoint{}
	if err := c.Get(ctx, key, legacy); err == nil {
		pair.legacy = legacy
	} else if client.IgnoreNotFound(err) != nil {
		return nil, err
	}

	return pair, nil
}

// all returns the objects that exist, v1 first.
func (p *cloudEndpointPair) all() []cloudendpoint.Resource {
	var out []cloudendpoint.Resource
	if p.v1 != nil {
		out = append(out, p.v1)
	}
	if p.legacy != nil {
		out = append(out, p.legacy)
	}
	return out
}

// surviving returns the objects that exist and aren't being deleted.
func (p *cloudEndpointPair) surviving() []cloudendpoint.Resource {
	var out []cloudendpoint.Resource
	for _, o := range p.all() {
		if controller.IsUpsert(o) {
			out = append(out, o)
		}
	}
	return out
}

// deleting returns the objects that are being deleted.
func (p *cloudEndpointPair) deleting() []cloudendpoint.Resource {
	var out []cloudendpoint.Resource
	for _, o := range p.all() {
		if controller.IsDelete(o) {
			out = append(out, o)
		}
	}
	return out
}

// survivorHolds reports whether a surviving object holds the remote ID id.
func (p *cloudEndpointPair) survivorHolds(id string) bool {
	for _, o := range p.surviving() {
		if o.GetID() == id {
			return true
		}
	}
	return false
}

func (p *cloudEndpointPair) survivingV1() *ngrokv1.CloudEndpoint {
	if p.v1 != nil && controller.IsUpsert(p.v1) {
		return p.v1
	}
	return nil
}

func (p *cloudEndpointPair) survivingLegacy() *ngrokv1alpha1.CloudEndpoint {
	if p.legacy != nil && controller.IsUpsert(p.legacy) {
		return p.legacy
	}
	return nil
}
