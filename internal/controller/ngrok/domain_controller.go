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
	"errors"
	"fmt"
	"reflect"
	"strings"
	"time"

	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/events"
	"k8s.io/client-go/util/workqueue"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/go-logr/logr"
	"github.com/ngrok/ngrok-api-go/v9"
	ingressv1alpha1 "github.com/ngrok/ngrok-operator/api/ingress/v1alpha1"
	ngrokv1 "github.com/ngrok/ngrok-operator/api/ngrok/v1"
	basecontroller "github.com/ngrok/ngrok-operator/internal/controller"
	domainpkg "github.com/ngrok/ngrok-operator/internal/domain"
	"github.com/ngrok/ngrok-operator/internal/ngrokapi"
	trafficpolicypkg "github.com/ngrok/ngrok-operator/internal/trafficpolicy"
	"github.com/ngrok/ngrok-operator/internal/util"
)

// wildcardCoverageRecheckInterval is how often a Domain whose reservation was
// skipped in favor of a wildcard re-verifies that the wildcard still exists.
// One List request per covered Domain per hour: 500 subdomains under a single
// wildcard works out to well under one request per second.
const wildcardCoverageRecheckInterval = time.Hour

// Event reasons specific to the migration window.
//
// LEGACY-domain-kind: delete at cleanup.
const (
	// EventDomainSupersededByV1 is recorded on a legacy Domain when its
	// ngrok.com/v1 twin takes over the reservation.
	EventDomainSupersededByV1 = "SupersededByV1"
	// EventDomainReservationKept is recorded when deleting a Domain leaves the
	// reservation in place because its twin still uses it.
	EventDomainReservationKept = "ReservationKept"
	// EventDomainTwinMismatch is recorded on both Domains of a namespace/name
	// pair that name different hostnames.
	EventDomainTwinMismatch = "DomainTwinMismatch"
)

// DomainReconciler reconciles Domains of both kinds: the canonical
// ngrok.com/v1 Domain and the deprecated ingress.k8s.ngrok.com/v1alpha1 one.
//
// The two kinds are reconciled as a pair keyed by namespace/name: For() the
// v1 kind and Watches() the legacy kind with EnqueueRequestForObject, so both
// enqueue the same request and a pair is never reconciled concurrently. A v1
// Domain and a legacy Domain with the same hostname ("matched twins") share
// one reservation, because create() adopts an existing reservation by
// hostname. Each pass keeps two invariants:
//
//  1. Exactly one object writes to the reservation: v1 while it's alive,
//     otherwise legacy. A legacy twin of a v1 that holds the reservation is
//     superseded: Ready=True with reason SupersededByV1, no ngrok API calls,
//     its other status fields left as they were, and a DeprecatedAPIGroup
//     warning when it is edited. It keeps its finalizer until it is deleted.
//  2. A reservation is released only when no alive twin still uses it. The
//     twin is re-read from the API server right before the release, since the
//     two kinds come from separate informers.
//
// At cleanup the legacy watch and twin handling go away and this collapses to
// a single-object reconcile.
type DomainReconciler struct {
	client.Client

	// APIReader reads straight from the API server. It is used only to
	// re-check a twin before releasing a reservation, the one irreversible
	// step. Defaults to the manager's API reader.
	APIReader client.Reader

	Log           logr.Logger
	Scheme        *runtime.Scheme
	Recorder      events.EventRecorder
	DomainsClient ngrokapi.DomainClient
	DrainState    basecontroller.DrainState
}

// +kubebuilder:rbac:groups=ngrok.com,resources=domains,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=ngrok.com,resources=domains/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=ngrok.com,resources=domains/finalizers,verbs=update;patch
// LEGACY-domain-kind: delete the three ingress.k8s.ngrok.com markers below at cleanup.
// +kubebuilder:rbac:groups=ingress.k8s.ngrok.com,resources=domains,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=ingress.k8s.ngrok.com,resources=domains/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=ingress.k8s.ngrok.com,resources=domains/finalizers,verbs=update;patch

// SetupWithManager sets up the controller with the Manager.
func (r *DomainReconciler) SetupWithManager(mgr ctrl.Manager) error {
	if r.DomainsClient == nil {
		return errors.New("DomainsClient must be set")
	}
	if r.APIReader == nil {
		r.APIReader = mgr.GetAPIReader()
	}

	// Attached per source rather than with WithEventFilter, so that a filter
	// meant for one kind can never hide the other kind's events.
	changed := predicate.Or(
		predicate.AnnotationChangedPredicate{},
		predicate.GenerationChangedPredicate{},
	)

	return ctrl.NewControllerManagedBy(mgr).
		// The pre-migration controller's default name, so metrics and logs
		// keep their names across the upgrade.
		Named("domain").
		For(&ngrokv1.Domain{}, builder.WithPredicates(changed)).
		// LEGACY-domain-kind: delete at cleanup.
		Watches(&ingressv1alpha1.Domain{}, &handler.EnqueueRequestForObject{}, builder.WithPredicates(changed)).
		WithOptions(controller.Options{
			// Use a custom rate limiter to exponentially backoff while certificates for domains provision
			RateLimiter: workqueue.NewTypedItemExponentialFailureRateLimiter[reconcile.Request](
				30*time.Second, // baseDelay
				10*time.Minute, // maxDelay
			),
		}).
		Complete(r)
}

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.13.1/pkg/reconcile
func (r *DomainReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := r.Log.WithValues("domain", req.NamespacedName)
	ctx = ctrl.LoggerInto(ctx, log)

	pair, err := domainpkg.GetPair(ctx, r.Client, req.NamespacedName)
	if err != nil {
		return ctrl.Result{}, err
	}
	if pair.Empty() {
		return ctrl.Result{}, nil
	}

	var canonical, legacy domainpkg.Resource
	if pair.V1 != nil {
		canonical = domainpkg.FromV1(pair.V1)
	}
	// LEGACY-domain-kind: BEGIN
	if pair.Legacy != nil {
		if legacy, err = domainpkg.FromLegacy(pair.Legacy); err != nil {
			return ctrl.Result{}, err
		}
	}
	// LEGACY-domain-kind: END

	// Internal domains cannot be reserved via the ngrok API. Each one is
	// handled on its own and takes no further part in the pass.
	if canonical != nil && util.IsInternalDomain(canonical.Hostname()) {
		if err := r.reconcileInternal(ctx, canonical); err != nil {
			return ctrl.Result{}, err
		}
		canonical = nil
	}
	if legacy != nil && util.IsInternalDomain(legacy.Hostname()) { // LEGACY-domain-kind: delete this block at cleanup.
		if err := r.reconcileInternal(ctx, legacy); err != nil {
			return ctrl.Result{}, err
		}
		legacy = nil
	}

	// Skip non-delete work during drain to prevent adding new finalizers.
	if basecontroller.IsDraining(ctx, r.DrainState) && !isDeleting(canonical) && !isDeleting(legacy) {
		log.V(1).Info("Draining, skipping non-delete reconcile")
		return ctrl.Result{}, nil
	}

	// LEGACY-domain-kind: BEGIN
	// A v1 and a legacy Domain that share a name but not a hostname hold
	// unrelated reservations. Reconcile each as if the other didn't exist.
	if canonical != nil && legacy != nil && !pair.Matched() {
		r.warnTwinMismatch(canonical, legacy)
		res1, err1 := r.reconcilePair(ctx, canonical, nil)
		res2, err2 := r.reconcilePair(ctx, legacy, nil)
		if err := errors.Join(err1, err2); err != nil {
			return ctrl.Result{}, err
		}
		return earliest(res1, res2), nil
	}
	// LEGACY-domain-kind: END

	return r.reconcilePair(ctx, canonical, legacy)
}

// reconcilePair runs one pass over a v1 Domain and its matched legacy twin.
// Either may be nil.
func (r *DomainReconciler) reconcilePair(ctx context.Context, canonical, legacy domainpkg.Resource) (ctrl.Result, error) {
	// 1. Deletes. Each one checks whether its twin still uses the
	//    reservation, so the order only decides which delete releases it.
	if isDeleting(canonical) {
		if err := r.finalizeDelete(ctx, canonical, legacy); err != nil {
			return r.errResult(err)
		}
	}
	if isDeleting(legacy) { // LEGACY-domain-kind: delete this block at cleanup.
		if err := r.finalizeDelete(ctx, legacy, canonical); err != nil {
			return r.errResult(err)
		}
	}

	// While draining only deletes run, as in BaseController: no syncs, no new
	// finalizers.
	if basecontroller.IsDraining(ctx, r.DrainState) {
		return ctrl.Result{}, nil
	}

	// 2. The writer: v1 while it's alive, otherwise legacy. A v1 Domain
	//    deleted while a legacy twin exists hands the reservation back to the
	//    twin in this same pass.
	var writer domainpkg.Resource
	switch {
	case canonical != nil && !isDeleting(canonical):
		writer = canonical
	case legacy != nil && !isDeleting(legacy):
		writer = legacy
		r.warnDeprecatedLegacy(legacy) // LEGACY-domain-kind: delete at cleanup.
	default:
		return ctrl.Result{}, nil
	}

	result, err := r.sync(ctx, writer)
	if err != nil {
		return result, err
	}

	// 3. LEGACY-domain-kind: BEGIN
	// Supersede the legacy twin once the v1 Domain holds the reservation.
	// Until then the legacy object is left exactly as it is: readers keep
	// using it, and its finalizer keeps guarding the reservation.
	if writer == canonical && legacy != nil && !isDeleting(legacy) && domainpkg.IsBacked(canonical) {
		if err := r.supersede(ctx, legacy, canonical); err != nil {
			return r.errResult(err)
		}
	}
	// LEGACY-domain-kind: END

	return result, nil
}

// sync creates or updates the writer's reservation and writes its status.
func (r *DomainReconciler) sync(ctx context.Context, domain domainpkg.Resource) (ctrl.Result, error) {
	obj := domain.Object()
	objName := util.ObjToHumanName(obj)

	if err := util.RegisterAndSyncFinalizer(ctx, r.Client, obj); err != nil {
		return ctrl.Result{}, err
	}

	if domain.Status().ID == "" {
		r.Recorder.Eventf(obj, nil, v1.EventTypeNormal, "Creating", "Create", fmt.Sprintf("Creating %s", objName))
		if err := r.create(ctx, domain); err != nil {
			r.Recorder.Eventf(obj, nil, v1.EventTypeWarning, "CreateError", "Create", fmt.Sprintf("Failed to Create %s: %s", objName, err.Error()))
			return r.errResult(err)
		}
		r.Recorder.Eventf(obj, nil, v1.EventTypeNormal, "Created", "Create", fmt.Sprintf("Created %s", objName))
	} else {
		r.Recorder.Eventf(obj, nil, v1.EventTypeNormal, "Updating", "Update", fmt.Sprintf("Updating %s", objName))
		if err := r.update(ctx, domain); err != nil {
			r.Recorder.Eventf(obj, nil, v1.EventTypeWarning, "UpdateError", "Update", fmt.Sprintf("Failed to update %s: %s", objName, err.Error()))
			return r.errResult(err)
		}
		r.Recorder.Eventf(obj, nil, v1.EventTypeNormal, "Updated", "Update", fmt.Sprintf("Updated %s", objName))
	}

	// Requeue the event relying on the controllers custom RateLimiter for exponential backoff
	if !domainpkg.IsReady(domain) {
		return ctrl.Result{Requeue: true}, nil //nolint:staticcheck
	}

	// A wildcard-covered Domain holds no reservation of its own, so nothing in
	// Kubernetes changes if the wildcard reservation disappears from the account,
	// and the watch predicate only fires on annotation/generation changes.
	// Re-check on an interval: status.id is empty, so the recheck goes back
	// through create(), which reserves the domain itself once the wildcard is gone.
	if domain.Status().CoveredByWildcardDomain != nil {
		return ctrl.Result{RequeueAfter: wildcardCoverageRecheckInterval}, nil
	}

	return ctrl.Result{}, nil
}

// finalizeDelete handles a Domain that is being deleted: it releases the
// reservation unless the twin still uses it, then removes the finalizer.
// twin is the matched twin, or nil.
func (r *DomainReconciler) finalizeDelete(ctx context.Context, domain, twin domainpkg.Resource) error {
	obj := domain.Object()
	if !util.HasFinalizer(obj) {
		return nil
	}
	log := ctrl.LoggerFrom(ctx)
	objName := util.ObjToHumanName(obj)

	if releasesReservation(domain) {
		keptFor := ""
		// LEGACY-domain-kind: BEGIN
		switch {
		case twin != nil && !isDeleting(twin):
			keptFor = humanKind(twin)
		default:
			// The two kinds come from separate informers, so the cache can be
			// behind: a v1 Domain applied just before this legacy one was
			// deleted may not be visible yet. Re-read right before the one
			// irreversible step.
			liveTwin, err := r.liveTwin(ctx, domain)
			if err != nil {
				return err
			}
			if liveTwin != nil {
				keptFor = humanKind(liveTwin)
			}
		}
		// LEGACY-domain-kind: END

		if keptFor != "" {
			log.Info("Keeping the domain reservation: its twin still uses it", "twin", keptFor, "id", domain.Status().ID)
			r.Recorder.Eventf(obj, nil, v1.EventTypeNormal, EventDomainReservationKept, "Delete",
				fmt.Sprintf("Kept reservation %s: %s still uses it", domain.Status().ID, keptFor))
		} else {
			r.Recorder.Eventf(obj, nil, v1.EventTypeNormal, "Deleting", "Delete", fmt.Sprintf("Deleting %s", objName))
			if err := r.deleteRemote(ctx, domain); err != nil {
				if !ngrok.IsNotFound(err) {
					r.Recorder.Eventf(obj, nil, v1.EventTypeWarning, "DeleteError", "Delete", fmt.Sprintf("Failed to delete %s: %s", objName, err.Error()))
					return err
				}
				log.Info("Domain reservation not found, assuming it was already deleted", "ID", domain.Status().ID)
			}
			r.Recorder.Eventf(obj, nil, v1.EventTypeNormal, "Deleted", "Delete", fmt.Sprintf("Deleted %s", objName))
		}
	}

	return util.RemoveAndSyncFinalizer(ctx, r.Client, obj)
}

// releasesReservation reports whether deleting domain would release a
// reservation in ngrok.
func releasesReservation(domain domainpkg.Resource) bool {
	// A wildcard-covered Domain holds no reservation of its own. Deleting here
	// would tear down a reservation shared with every other subdomain under the
	// wildcard.
	s := domain.Status()
	return s.ID != "" && s.CoveredByWildcardDomain == nil && domain.ReclaimPolicy() == ngrokv1.DomainReclaimPolicyDelete
}

// liveTwin returns the other kind's Domain at domain's namespace/name, read
// from the API server, if it is alive and names the same hostname.
//
// LEGACY-domain-kind: delete at cleanup.
func (r *DomainReconciler) liveTwin(ctx context.Context, domain domainpkg.Resource) (domainpkg.Resource, error) {
	pair, err := domainpkg.GetPair(ctx, r.APIReader, client.ObjectKeyFromObject(domain.Object()))
	if err != nil {
		return nil, err
	}

	var twin domainpkg.Resource
	switch {
	case domain.IsLegacy() && pair.V1 != nil && domainpkg.IsAlive(pair.V1):
		twin = domainpkg.FromV1(pair.V1)
	case !domain.IsLegacy() && pair.Legacy != nil && domainpkg.IsAlive(pair.Legacy):
		if twin, err = domainpkg.FromLegacy(pair.Legacy); err != nil {
			return nil, err
		}
	default:
		return nil, nil
	}

	if util.NormalizeHostname(twin.Hostname()) != util.NormalizeHostname(domain.Hostname()) {
		return nil, nil
	}
	return twin, nil
}

// supersede marks a legacy Domain as superseded by its v1 twin. It makes no
// ngrok API calls and changes only the Ready condition and observed
// generation; the rest of the status is left as it was when the twin took
// over. The finalizer stays until the legacy object is deleted.
//
// LEGACY-domain-kind: delete at cleanup.
func (r *DomainReconciler) supersede(ctx context.Context, legacy, canonical domainpkg.Resource) error {
	obj := legacy.Object()
	twin := canonical.Object()
	generation := obj.GetGeneration()

	ready := meta.FindStatusCondition(legacy.Status().Conditions, domainpkg.ConditionReady)
	alreadySuperseded := ready != nil && ready.Reason == domainpkg.ReasonSupersededByV1
	edited := generation != legacy.Status().ObservedGeneration

	if alreadySuperseded && !edited {
		return nil
	}

	switch {
	case !alreadySuperseded:
		r.Recorder.Eventf(obj, twin, v1.EventTypeNormal, EventDomainSupersededByV1, "Reconcile",
			fmt.Sprintf("domains.ngrok.com %s/%s now manages this domain's reservation", twin.GetNamespace(), twin.GetName()))
	case edited:
		r.Recorder.Eventf(obj, twin, v1.EventTypeWarning, trafficpolicypkg.EventDeprecatedAPIGroup, "Reconcile",
			fmt.Sprintf("ingress.k8s.ngrok.com/v1alpha1 Domain is deprecated and superseded by domains.ngrok.com %s/%s; this change has no effect, edit that object instead",
				twin.GetNamespace(), twin.GetName()))
	}

	domainpkg.SetSupersededCondition(legacy, twin.GetNamespace(), twin.GetName())
	return r.writeStatus(ctx, legacy, nil)
}

// warnDeprecatedLegacy records a DeprecatedAPIGroup warning on a legacy
// Domain with no v1 twin, once per generation.
//
// LEGACY-domain-kind: delete at cleanup.
func (r *DomainReconciler) warnDeprecatedLegacy(legacy domainpkg.Resource) {
	obj := legacy.Object()
	if obj.GetGeneration() == legacy.Status().ObservedGeneration {
		return
	}
	r.Recorder.Eventf(obj, nil, v1.EventTypeWarning, trafficpolicypkg.EventDeprecatedAPIGroup, "Reconcile",
		"ingress.k8s.ngrok.com/v1alpha1 Domain is deprecated; re-create it as an ngrok.com/v1 Domain with the same name, wait for it to be Ready, then delete this object")
}

// warnTwinMismatch records a warning on both Domains of a pair that share a
// name but not a hostname, once per generation of the v1 object.
//
// LEGACY-domain-kind: delete at cleanup.
func (r *DomainReconciler) warnTwinMismatch(canonical, legacy domainpkg.Resource) {
	if canonical.Object().GetGeneration() == canonical.Status().ObservedGeneration {
		return
	}
	msg := fmt.Sprintf("domains.ngrok.com and domains.ingress.k8s.ngrok.com %s/%s name different hostnames (%q and %q); they are reconciled independently",
		canonical.Object().GetNamespace(), canonical.Object().GetName(), canonical.Hostname(), legacy.Hostname())
	r.Recorder.Eventf(canonical.Object(), nil, v1.EventTypeWarning, EventDomainTwinMismatch, "Reconcile", msg)
	r.Recorder.Eventf(legacy.Object(), nil, v1.EventTypeWarning, EventDomainTwinMismatch, "Reconcile", msg)
}

// reconcileInternal handles a Domain for an internal hostname. Internal
// domains cannot be reserved via the ngrok API: skip reconciliation, but
// ensure the finalizer is removed so deletion can proceed.
func (r *DomainReconciler) reconcileInternal(ctx context.Context, domain domainpkg.Resource) error {
	log := ctrl.LoggerFrom(ctx)
	log.Info("Skipping Domain with internal TLD - internal domains cannot be reserved via ngrok API", "hostname", domain.Hostname())

	obj := domain.Object()
	if util.HasFinalizer(obj) {
		if err := util.RemoveAndSyncFinalizer(ctx, r.Client, obj); err != nil {
			return err
		}
	}
	if isDeleting(domain) {
		return nil
	}
	domainpkg.UpdateConditions(domain, nil, errors.New(".internal domains do not need to be reserved. You can safely delete this Domain CR if you wish"))
	return r.writeStatus(ctx, domain, nil)
}

// errResult maps a reconcile error to a result, keeping the Domain-specific
// retryable ngrok errors.
func (r *DomainReconciler) errResult(err error) (reconcile.Result, error) {
	retryableErrors := []int{
		// Domain still attached to an edge, probably a race condition.
		// Schedule for retry, and hopefully the edge will be gone
		// eventually.
		446,
		// Domain has a dangling CNAME record. Other controllers or operators, such as external-dns, might
		// be managing the DNS records for the domain and in the process of deleting the CNAME record.
		511,
	}
	if ngrok.IsErrorCode(err, retryableErrors...) {
		return ctrl.Result{}, err
	}
	return basecontroller.CtrlResultForErr(err)
}

// writeStatus writes domain's status with BaseController's retry-on-conflict
// logic, and returns origErr (or a StatusError if the write failed).
func (r *DomainReconciler) writeStatus(ctx context.Context, domain domainpkg.Resource, origErr error) error {
	if err := domain.CommitStatus(); err != nil {
		return err
	}
	switch obj := domain.Object().(type) {
	case *ngrokv1.Domain:
		return (&basecontroller.BaseController[*ngrokv1.Domain]{Kube: r.Client, Recorder: r.Recorder}).ReconcileStatus(ctx, obj, origErr)
	case *ingressv1alpha1.Domain: // LEGACY-domain-kind: delete this case at cleanup.
		return (&basecontroller.BaseController[*ingressv1alpha1.Domain]{Kube: r.Client, Recorder: r.Recorder}).ReconcileStatus(ctx, obj, origErr)
	default:
		return fmt.Errorf("unsupported Domain type %T", obj)
	}
}

func (r *DomainReconciler) create(ctx context.Context, domain domainpkg.Resource) error {
	log := ctrl.LoggerFrom(ctx)

	// First check if the reserved domain already exists. The API is sometimes returning dangling CNAME records
	// errors right now, so we'll check if the domain already exists before trying to create it.
	lookup, err := r.findReservedDomainForHostname(ctx, domain.Hostname())
	if err != nil {
		// Set conditions before returning error
		return r.updateStatus(ctx, domain, nil, err)
	}

	// An exact reservation always wins over a covering wildcard: adopt it, and
	// push this Domain's spec to it now rather than on some later update pass.
	// The usual way to get here is a v1 Domain taking over the reservation of
	// its legacy twin, and nothing else would trigger that pass: the status
	// write doesn't bump the generation, and an adopted domain is often Ready.
	if lookup.Exact != nil {
		domain.Status().ID = lookup.Exact.ID
		return r.pushSpec(ctx, domain, lookup.Exact)
	}

	// A wildcard parent reservation already resolves this hostname via DNS and
	// already covers it with the wildcard's certificate, so reserving it
	// individually would only add clutter to the account.
	if lookup.Wildcard != nil && canSkipWildcardCoveredReservation(domain) {
		// Announce only on transition. A covered Domain keeps an empty status.id,
		// so it re-enters create() on every hourly recheck; logging and recording
		// an event each time would be pure noise at the scale this feature exists
		// to serve. Compare by ID so a wildcard that was released and reserved
		// again under the same name still counts as a transition.
		if prev := domain.Status().CoveredByWildcardDomain; prev == nil || prev.ID != lookup.Wildcard.ID {
			log.Info("Skipping domain reservation, already covered by a wildcard reservation",
				"hostname", domain.Hostname(), "wildcard", lookup.Wildcard.Domain)
			r.Recorder.Eventf(domain.Object(), nil, v1.EventTypeNormal, "CoveredByWildcardDomain", "Create",
				fmt.Sprintf("Skipped reserving %s: already covered by wildcard reservation %s",
					domain.Hostname(), lookup.Wildcard.Domain))
		}
		return r.updateStatusForWildcardCoverage(ctx, domain, lookup.Wildcard)
	}

	// Reaching here means the lookup succeeded and the wildcard no longer covers
	// this Domain. Drop the status mirrored from it now, before the Create below,
	// so a failed Create does not leave the Domain claiming coverage (and the
	// wildcard's CNAME target) it no longer has. This is deliberately not done on
	// lookup errors: a transient API failure says nothing about coverage.
	if domain.Status().CoveredByWildcardDomain != nil {
		*domain.Status() = ngrokv1.DomainStatus{Conditions: domain.Status().Conditions}
	}

	// Not found, so we'll create it
	req := &ngrok.ReservedDomainCreate{
		Domain:      domain.Hostname(),
		Description: domain.Description(),
		Metadata:    domain.MetadataAPIString(),
		ResolvesTo:  buildResolvesToRequest(domain.ResolvesTo()),
	}
	resp, err := r.DomainsClient.Create(ctx, req)
	return r.updateStatus(ctx, domain, resp, err)
}

// canSkipWildcardCoveredReservation reports whether a hostname covered by a
// wildcard reservation can go without a reservation of its own.
func canSkipWildcardCoveredReservation(domain domainpkg.Resource) bool {
	// A Domain with explicit resolvesTo targets needs its own reservation:
	// resolvesTo is a property of the reservation, so there is nowhere to record
	// it when we skip the create, and the wildcard's targets may differ.
	return len(domain.ResolvesTo()) == 0
}

func (r *DomainReconciler) update(ctx context.Context, domain domainpkg.Resource) error {
	resp, err := r.DomainsClient.Get(ctx, domain.Status().ID)
	if err != nil {
		// If the domain is gone, clear the status and trigger a re-reconcile
		if ngrok.IsNotFound(err) {
			*domain.Status() = ngrokv1.DomainStatus{}
			return r.writeStatus(ctx, domain, err)
		}

		// Set conditions for other Get errors
		return r.updateStatus(ctx, domain, nil, err)
	}

	return r.pushSpec(ctx, domain, resp)
}

// pushSpec updates the reservation resp to match domain's spec, if it differs,
// and records the result in domain's status.
func (r *DomainReconciler) pushSpec(ctx context.Context, domain domainpkg.Resource, resp *ngrok.ReservedDomain) error {
	// Only update the domain if updatable fields have changed
	specResolvesTo := buildResolvesToRequest(domain.ResolvesTo())
	specMetadata := domain.MetadataAPIString()
	specDescription := domain.Description()
	if specDescription == resp.Description &&
		specMetadata == resp.Metadata &&
		reflect.DeepEqual(specResolvesTo, resp.ResolvesTo) {
		// No changes needed, still update status to ensure conditions are current
		return r.updateStatus(ctx, domain, resp, nil)
	}

	req := &ngrok.ReservedDomainUpdate{
		ID:          domain.Status().ID,
		Description: &specDescription,
		Metadata:    &specMetadata,
		ResolvesTo:  specResolvesTo,
	}
	resp, err := r.DomainsClient.Update(ctx, req)
	return r.updateStatus(ctx, domain, resp, err)
}

// deleteRemote releases domain's reservation, honoring its reclaim policy.
func (r *DomainReconciler) deleteRemote(ctx context.Context, domain domainpkg.Resource) error {
	// finalizeDelete already checks this; this is the belt-and-braces guard at
	// the one place that could destroy shared state.
	if !releasesReservation(domain) {
		return nil
	}

	err := r.DomainsClient.Delete(ctx, domain.Status().ID)
	if err == nil || ngrok.IsNotFound(err) {
		domain.Status().ID = ""
	}
	return err
}

// reservedDomainLookup is the outcome of searching the account's reserved
// domains for something that can serve a hostname.
type reservedDomainLookup struct {
	// Exact is the reservation for the hostname itself, if the account holds one.
	Exact *ngrok.ReservedDomain
	// Wildcard is the reservation for the hostname's wildcard parent
	// (e.g. *.example.com for a.example.com), if the account holds one.
	Wildcard *ngrok.ReservedDomain
}

// findReservedDomainForHostname looks up the reservations that could serve
// hostname: the exact reservation and, when hostname has one, its wildcard
// parent. Both candidates ride in a single filtered List request.
func (r *DomainReconciler) findReservedDomainForHostname(ctx context.Context, hostname string) (reservedDomainLookup, error) {
	var result reservedDomainLookup

	exact := util.NormalizeHostname(hostname)
	candidates := []string{exact}
	wildcard, hasWildcard := util.WildcardParentDomain(hostname)
	if hasWildcard {
		candidates = append(candidates, wildcard)
	}

	// Filter server-side via ngrok's API filtering (https://ngrok.com/docs/api/api-filtering)
	// instead of paging through every reserved domain on the account. `in` keeps
	// this to a single request even with two candidates.
	iter := r.DomainsClient.List(&ngrok.FilteredPaging{
		Filter: new(domainInFilter(candidates)),
	})

	// Sort every returned item rather than taking the first hit: the API makes no
	// ordering guarantee, so we must be able to prefer the exact match over the
	// wildcard regardless of which one arrives first.
	for iter.Next(ctx) {
		item := iter.Item()
		switch {
		case strings.EqualFold(item.Domain, exact):
			result.Exact = item
		case hasWildcard && strings.EqualFold(item.Domain, wildcard):
			result.Wildcard = item
		}
	}

	return result, iter.Err()
}

// domainInFilter builds a CEL filter matching any of the given domain names,
// e.g. `obj.domain in ["a.example.com","*.example.com"]`.
func domainInFilter(domains []string) string {
	quoted := make([]string, len(domains))
	for i, d := range domains {
		quoted[i] = fmt.Sprintf("%q", d)
	}
	return fmt.Sprintf("obj.domain in [%s]", strings.Join(quoted, ","))
}

// updateStatus updates the status fields of the domain resource only if any values have changed
func (r *DomainReconciler) updateStatus(ctx context.Context, domain domainpkg.Resource, ngrokDomain *ngrok.ReservedDomain, createErr error) error {
	if ngrokDomain != nil {
		status := domain.Status()
		status.ID = ngrokDomain.ID
		status.Domain = ngrokDomain.Domain

		// This Domain now holds a reservation of its own, so any previously
		// recorded wildcard coverage no longer applies. Leaving it set would both
		// misreport the domain and, via the deleteRemote() guard, orphan this
		// reservation in the account when the CR goes away.
		status.CoveredByWildcardDomain = nil

		status.CNAMETarget = ngrokDomain.CNAMETarget
		status.ACMEChallengeCNAMETarget = ngrokDomain.ACMEChallengeCNAMETarget
		status.ResolvesTo = buildResolvesToStatus(ngrokDomain.ResolvesTo)

		status.Certificate = buildCertificateInfo(ngrokDomain.Certificate)
		status.CertificateManagementPolicy = buildCertificateManagementPolicy(ngrokDomain.CertificateManagementPolicy)
		status.CertificateManagementStatus = buildCertificateManagementStatus(ngrokDomain.CertificateManagementStatus)
	}

	domainpkg.UpdateConditions(domain, ngrokDomain, createErr)
	return r.writeStatus(ctx, domain, createErr)
}

// updateStatusForWildcardCoverage records that an existing wildcard reservation
// already serves this hostname, instead of reserving it.
//
// This deliberately does not go through updateStatus: that copies the API
// object's own hostname into status.domain, which for a wildcard would publish
// "*.example.com" as this Domain's hostname. Ingress LB status and Gateway
// addresses fall back to status.domain with the "*." prefix trimmed, so doing
// that would advertise the apex.
func (r *DomainReconciler) updateStatusForWildcardCoverage(ctx context.Context, domain domainpkg.Resource, wildcard *ngrok.ReservedDomain) error {
	status := domain.Status()

	// status.id stays empty on purpose. It is the handle deleteRemote() passes to
	// DomainsClient.Delete, and this reservation belongs to the wildcard, which is
	// shared with every other subdomain under it.
	status.ID = ""
	status.CoveredByWildcardDomain = &ngrokv1.DomainStatusWildcardDomain{
		ID:     wildcard.ID,
		Domain: wildcard.Domain,
	}

	// This Domain's own hostname, never the wildcard's.
	status.Domain = domain.Hostname()

	// The wildcard's CNAME target is the record that actually resolves this
	// hostname, so it is the right value for Service and Ingress LB status.
	status.CNAMETarget = wildcard.CNAMETarget

	// Certificate state is genuinely shared: a custom wildcard whose certificate
	// has not provisioned yet cannot serve this hostname either. Mirroring it lets
	// UpdateConditions gate readiness on the wildcard using the same logic
	// it applies to a domain's own reservation.
	status.Certificate = buildCertificateInfo(wildcard.Certificate)
	status.CertificateManagementPolicy = buildCertificateManagementPolicy(wildcard.CertificateManagementPolicy)
	status.CertificateManagementStatus = buildCertificateManagementStatus(wildcard.CertificateManagementStatus)

	// Not mirrored, on purpose:
	//   acmeChallengeCNAMETarget - the ACME challenge record is created once, on
	//     the wildcard. Publishing it here would imply the user must add a second
	//     _acme-challenge record per subdomain.
	//   resolvesTo - describes the wildcard's own routing targets, and create()
	//     declines to skip the reservation when this Domain's spec sets its own.
	status.ACMEChallengeCNAMETarget = nil
	status.ResolvesTo = nil

	domainpkg.UpdateConditions(domain, wildcard, nil)
	return r.writeStatus(ctx, domain, nil)
}

// isDeleting reports whether domain is non-nil and being deleted.
func isDeleting(domain domainpkg.Resource) bool {
	return domain != nil && !domain.Object().GetDeletionTimestamp().IsZero()
}

// humanKind names a Domain by its fully qualified kind, namespace and name.
func humanKind(domain domainpkg.Resource) string {
	resource := "domains.ngrok.com"
	if domain.IsLegacy() { // LEGACY-domain-kind: delete at cleanup.
		resource = "domains.ingress.k8s.ngrok.com"
	}
	obj := domain.Object()
	return fmt.Sprintf("%s %s/%s", resource, obj.GetNamespace(), obj.GetName())
}

// earliest returns the result that asks to be requeued soonest.
//
// LEGACY-domain-kind: delete at cleanup.
func earliest(a, b ctrl.Result) ctrl.Result {
	switch {
	case a.Requeue: //nolint:staticcheck
		return a
	case b.Requeue: //nolint:staticcheck
		return b
	case a.RequeueAfter == 0:
		return b
	case b.RequeueAfter == 0 || a.RequeueAfter < b.RequeueAfter:
		return a
	default:
		return b
	}
}

func buildResolvesToStatus(resolvesTo []ngrok.ReservedDomainResolvesToEntry) []ngrokv1.DomainResolvesToEntry {
	if len(resolvesTo) == 0 {
		return nil
	}

	result := make([]ngrokv1.DomainResolvesToEntry, len(resolvesTo))
	for i, entry := range resolvesTo {
		result[i] = ngrokv1.DomainResolvesToEntry{
			Value: entry.Value,
		}
	}
	return result
}

func buildResolvesToRequest(resolvesTo []ngrokv1.DomainResolvesToEntry) []ngrok.ReservedDomainResolvesToEntry {
	if len(resolvesTo) == 0 {
		return nil
	}

	result := make([]ngrok.ReservedDomainResolvesToEntry, len(resolvesTo))
	for i, entry := range resolvesTo {
		result[i] = ngrok.ReservedDomainResolvesToEntry{
			Value: entry.Value,
		}
	}
	return result
}

func buildCertificateInfo(certificate *ngrok.Ref) *ngrokv1.DomainStatusCertificateInfo {
	if certificate == nil || certificate.ID == "" {
		return nil
	}

	return &ngrokv1.DomainStatusCertificateInfo{
		ID: certificate.ID,
	}
}

func buildCertificateManagementPolicy(policy *ngrok.ReservedDomainCertPolicy) *ngrokv1.DomainStatusCertificateManagementPolicy {
	if policy == nil {
		return nil
	}

	return &ngrokv1.DomainStatusCertificateManagementPolicy{
		Authority:      policy.Authority,
		PrivateKeyType: policy.PrivateKeyType,
	}
}

func buildCertificateManagementStatus(status *ngrok.ReservedDomainCertStatus) *ngrokv1.DomainStatusCertificateManagementStatus {
	if status == nil {
		return nil
	}

	result := &ngrokv1.DomainStatusCertificateManagementStatus{}
	result.RenewsAt = parseRFC3339Pointer(status.RenewsAt)
	result.ProvisioningJob = buildProvisioningJob(status.ProvisioningJob)
	return result
}

func buildProvisioningJob(job *ngrok.ReservedDomainCertJob) *ngrokv1.DomainStatusProvisioningJob {
	if job == nil {
		return nil
	}

	result := &ngrokv1.DomainStatusProvisioningJob{
		Message: job.Msg,
	}

	if job.ErrorCode != nil {
		result.ErrorCode = *job.ErrorCode
	}

	result.StartedAt = parseRFC3339String(job.StartedAt)
	result.RetriesAt = parseRFC3339Pointer(job.RetriesAt)
	return result
}

func parseRFC3339Pointer(value *string) *metav1.Time {
	if value == nil {
		return nil
	}
	return parseRFC3339String(*value)
}

func parseRFC3339String(value string) *metav1.Time {
	if value == "" {
		return nil
	}

	t, err := time.Parse(time.RFC3339, value)
	if err != nil {
		return nil
	}
	return &metav1.Time{Time: t}
}
