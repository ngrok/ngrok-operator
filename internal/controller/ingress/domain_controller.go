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

package ingress

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"time"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/events"
	"k8s.io/client-go/util/workqueue"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/go-logr/logr"
	"github.com/ngrok/ngrok-api-go/v9"
	commonv1alpha1 "github.com/ngrok/ngrok-operator/api/common/v1alpha1"
	"github.com/ngrok/ngrok-operator/api/ingress/v1alpha1"
	basecontroller "github.com/ngrok/ngrok-operator/internal/controller"
	"github.com/ngrok/ngrok-operator/internal/ngrokapi"
	"github.com/ngrok/ngrok-operator/internal/util"
)

// wildcardCoverageRecheckInterval is how often a Domain whose reservation was
// skipped in favor of a wildcard re-verifies that the wildcard still exists.
// One List request per covered Domain per hour: 500 subdomains under a single
// wildcard works out to well under one request per second.
const wildcardCoverageRecheckInterval = time.Hour

// DomainReconciler reconciles a Domain object
type DomainReconciler struct {
	client.Client

	Log           logr.Logger
	Scheme        *runtime.Scheme
	Recorder      events.EventRecorder
	DomainsClient ngrokapi.DomainClient
	DrainState    basecontroller.DrainState

	controller *basecontroller.BaseController[*v1alpha1.Domain]
}

// SetupWithManager sets up the controller with the Manager.
func (r *DomainReconciler) SetupWithManager(mgr ctrl.Manager) error {
	if r.DomainsClient == nil {
		return errors.New("DomainsClient must be set")
	}

	r.controller = &basecontroller.BaseController[*v1alpha1.Domain]{
		Kube:       r.Client,
		Log:        r.Log,
		Recorder:   r.Recorder,
		DrainState: r.DrainState,

		StatusID: func(cr *v1alpha1.Domain) string { return cr.Status.ID },
		Create:   r.create,
		Update:   r.update,
		Delete:   r.delete,
		ErrResult: func(_ basecontroller.BaseControllerOp, _ *v1alpha1.Domain, err error) (reconcile.Result, error) {
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
		},
	}

	return ctrl.NewControllerManagedBy(mgr).
		For(&v1alpha1.Domain{}).
		WithEventFilter(predicate.Or(
			predicate.AnnotationChangedPredicate{},
			predicate.GenerationChangedPredicate{},
		)).
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

	// Get the domain first to check if it's internal
	domain := &v1alpha1.Domain{}
	if err := r.Get(ctx, req.NamespacedName, domain); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	// Internal domains cannot be reserved via the ngrok API.
	// Skip reconciliation but ensure the finalizer is removed so deletion can proceed.
	if util.IsInternalDomain(domain.Spec.Domain) {
		log.Info("Skipping Domain with internal TLD - internal domains cannot be reserved via ngrok API", "hostname", domain.Spec.Domain)

		if util.HasFinalizer(domain) {
			if err := util.RemoveAndSyncFinalizer(ctx, r.Client, domain); err != nil {
				return ctrl.Result{}, err
			}
		}
		updateDomainConditions(domain, nil, errors.New(".internal domains do not need to be reserved. You can safely delete this Domain CR if you wish"))
		return ctrl.Result{}, r.controller.ReconcileStatus(ctx, domain, nil)
	}

	result, err := r.controller.Reconcile(ctx, req, new(v1alpha1.Domain))
	if err != nil {
		return result, err
	}

	// Get the updated domain to check if we need requeuing
	if err = r.Get(ctx, req.NamespacedName, domain); err != nil {
		return result, client.IgnoreNotFound(err)
	}

	// Requeue if the domain is not ready
	if !IsDomainReady(domain) {
		// Requeue the event relying on the controllers custom RateLimiter for exponential backoff
		return ctrl.Result{Requeue: true}, nil //nolint:staticcheck
	}

	// A wildcard-covered Domain holds no reservation of its own, so nothing in
	// Kubernetes changes if the wildcard reservation disappears from the account,
	// and the watch predicate only fires on annotation/generation changes.
	// Re-check on an interval: status.id is empty, so the recheck goes back
	// through create(), which reserves the domain itself once the wildcard is gone.
	if domain.Status.CoveredByWildcardDomain != "" {
		return ctrl.Result{RequeueAfter: wildcardCoverageRecheckInterval}, nil
	}

	return result, nil
}

func (r *DomainReconciler) create(ctx context.Context, domain *v1alpha1.Domain) error {
	log := ctrl.LoggerFrom(ctx)

	// First check if the reserved domain already exists. The API is sometimes returning dangling CNAME records
	// errors right now, so we'll check if the domain already exists before trying to create it.
	lookup, err := r.findReservedDomainForHostname(ctx, domain.Spec.Domain)
	if err != nil {
		// Set conditions before returning error
		return r.updateStatus(ctx, domain, nil, err)
	}

	// An exact reservation always wins over a covering wildcard: adopt it.
	if lookup.Exact != nil {
		return r.updateStatus(ctx, domain, lookup.Exact, nil)
	}

	// A wildcard parent reservation already resolves this hostname via DNS and
	// already covers it with the wildcard's certificate, so reserving it
	// individually would only add clutter to the account.
	if lookup.Wildcard != nil && canSkipWildcardCoveredReservation(domain) {
		// Announce only on transition. A covered Domain keeps an empty status.id,
		// so it re-enters create() on every hourly recheck; logging and recording
		// an event each time would be pure noise at the scale this feature exists
		// to serve.
		if domain.Status.CoveredByWildcardDomain != lookup.Wildcard.Domain {
			log.Info("Skipping domain reservation, already covered by a wildcard reservation",
				"hostname", domain.Spec.Domain, "wildcard", lookup.Wildcard.Domain)
			r.Recorder.Eventf(domain, nil, v1.EventTypeNormal, "CoveredByWildcardDomain", "Create",
				fmt.Sprintf("Skipped reserving %s: already covered by wildcard reservation %s",
					domain.Spec.Domain, lookup.Wildcard.Domain))
		}
		return r.updateStatusForWildcardCoverage(ctx, domain, lookup.Wildcard)
	}

	// Not found, so we'll create it
	req := &ngrok.ReservedDomainCreate{
		Domain:      domain.Spec.Domain,
		Description: domain.Spec.Description,
		Metadata:    commonv1alpha1.MetadataAPIString(domain.Spec.Metadata),
		ResolvesTo:  buildResolvesToRequest(domain.Spec.GetResolvesTo()),
	}
	resp, err := r.DomainsClient.Create(ctx, req)
	return r.updateStatus(ctx, domain, resp, err)
}

// canSkipWildcardCoveredReservation reports whether a hostname covered by a
// wildcard reservation can go without a reservation of its own.
func canSkipWildcardCoveredReservation(domain *v1alpha1.Domain) bool {
	// A Domain with explicit resolvesTo targets needs its own reservation:
	// resolvesTo is a property of the reservation, so there is nowhere to record
	// it when we skip the create, and the wildcard's targets may differ.
	return len(domain.Spec.GetResolvesTo()) == 0
}

func (r *DomainReconciler) update(ctx context.Context, domain *v1alpha1.Domain) error {
	resp, err := r.DomainsClient.Get(ctx, domain.Status.ID)
	if err != nil {
		// If the domain is gone, clear the status and trigger a re-reconcile
		if ngrok.IsNotFound(err) {
			domain.Status = v1alpha1.DomainStatus{}
			return r.controller.ReconcileStatus(ctx, domain, err)
		}

		// Set conditions for other Get errors
		return r.updateStatus(ctx, domain, nil, err)
	}

	// Only update the domain if updatable fields have changed
	specResolvesTo := buildResolvesToRequest(domain.Spec.GetResolvesTo())
	specMetadata := commonv1alpha1.MetadataAPIString(domain.Spec.Metadata)
	if domain.Spec.Description == resp.Description &&
		specMetadata == resp.Metadata &&
		reflect.DeepEqual(specResolvesTo, resp.ResolvesTo) {
		// No changes needed, still update status to ensure conditions are current
		return r.updateStatus(ctx, domain, resp, nil)
	}

	req := &ngrok.ReservedDomainUpdate{
		ID:          domain.Status.ID,
		Description: &domain.Spec.Description,
		Metadata:    &specMetadata,
		ResolvesTo:  specResolvesTo,
	}
	resp, err = r.DomainsClient.Update(ctx, req)
	return r.updateStatus(ctx, domain, resp, err)
}

func (r *DomainReconciler) delete(ctx context.Context, domain *v1alpha1.Domain) error {
	// A wildcard-covered Domain holds no reservation of its own. Deleting here
	// would tear down a reservation shared with every other subdomain under the
	// wildcard. BaseController already skips Delete when status.id is empty; this
	// is the belt-and-braces guard at the one place that could destroy shared state.
	if domain.Status.ID == "" || domain.Status.CoveredByWildcardDomain != "" {
		return nil
	}

	if domain.Spec.ReclaimPolicy != v1alpha1.DomainReclaimPolicyDelete {
		return nil
	}

	err := r.DomainsClient.Delete(ctx, domain.Status.ID)
	if err == nil || ngrok.IsNotFound(err) {
		domain.Status.ID = ""
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
func (r *DomainReconciler) updateStatus(ctx context.Context, domain *v1alpha1.Domain, ngrokDomain *ngrok.ReservedDomain, createErr error) error {
	if ngrokDomain != nil {
		domain.Status.ID = ngrokDomain.ID
		domain.Status.Domain = ngrokDomain.Domain

		// This Domain now holds a reservation of its own, so any previously
		// recorded wildcard coverage no longer applies. Leaving it set would both
		// misreport the domain and, via the delete() guard, orphan this
		// reservation in the account when the CR goes away.
		domain.Status.CoveredByWildcardDomain = ""

		domain.Status.CNAMETarget = ngrokDomain.CNAMETarget
		domain.Status.ACMEChallengeCNAMETarget = ngrokDomain.ACMEChallengeCNAMETarget
		domain.Status.ResolvesTo = buildResolvesToStatus(ngrokDomain.ResolvesTo)

		domain.Status.Certificate = buildCertificateInfo(ngrokDomain.Certificate)
		domain.Status.CertificateManagementPolicy = buildCertificateManagementPolicy(ngrokDomain.CertificateManagementPolicy)
		domain.Status.CertificateManagementStatus = buildCertificateManagementStatus(ngrokDomain.CertificateManagementStatus)
	}

	updateDomainConditions(domain, ngrokDomain, createErr)
	return r.controller.ReconcileStatus(ctx, domain, createErr)
}

// updateStatusForWildcardCoverage records that an existing wildcard reservation
// already serves this hostname, instead of reserving it.
//
// This deliberately does not go through updateStatus: that copies the API
// object's own hostname into status.domain, which for a wildcard would publish
// "*.example.com" as this Domain's hostname. Ingress LB status and Gateway
// addresses fall back to status.domain with the "*." prefix trimmed, so doing
// that would advertise the apex.
func (r *DomainReconciler) updateStatusForWildcardCoverage(ctx context.Context, domain *v1alpha1.Domain, wildcard *ngrok.ReservedDomain) error {
	// status.id stays empty on purpose. It is the handle delete() passes to
	// DomainsClient.Delete, and this reservation belongs to the wildcard, which is
	// shared with every other subdomain under it.
	domain.Status.ID = ""
	domain.Status.CoveredByWildcardDomain = wildcard.Domain

	// This Domain's own hostname, never the wildcard's.
	domain.Status.Domain = domain.Spec.Domain

	// The wildcard's CNAME target is the record that actually resolves this
	// hostname, so it is the right value for Service and Ingress LB status.
	domain.Status.CNAMETarget = wildcard.CNAMETarget

	// Certificate state is genuinely shared: a custom wildcard whose certificate
	// has not provisioned yet cannot serve this hostname either. Mirroring it lets
	// updateDomainConditions gate readiness on the wildcard using the same logic
	// it applies to a domain's own reservation.
	domain.Status.Certificate = buildCertificateInfo(wildcard.Certificate)
	domain.Status.CertificateManagementPolicy = buildCertificateManagementPolicy(wildcard.CertificateManagementPolicy)
	domain.Status.CertificateManagementStatus = buildCertificateManagementStatus(wildcard.CertificateManagementStatus)

	// Not mirrored, on purpose:
	//   acmeChallengeCNAMETarget - the ACME challenge record is created once, on
	//     the wildcard. Publishing it here would imply the user must add a second
	//     _acme-challenge record per subdomain.
	//   resolvesTo - describes the wildcard's own routing targets, and create()
	//     declines to skip the reservation when this Domain's spec sets its own.
	domain.Status.ACMEChallengeCNAMETarget = nil
	domain.Status.ResolvesTo = nil

	updateDomainConditions(domain, wildcard, nil)
	return r.controller.ReconcileStatus(ctx, domain, nil)
}

func buildResolvesToStatus(resolvesTo []ngrok.ReservedDomainResolvesToEntry) []v1alpha1.DomainResolvesToEntry {
	if len(resolvesTo) == 0 {
		return nil
	}

	result := make([]v1alpha1.DomainResolvesToEntry, len(resolvesTo))
	for i, entry := range resolvesTo {
		result[i] = v1alpha1.DomainResolvesToEntry{
			Value: entry.Value,
		}
	}
	return result
}

func buildResolvesToRequest(resolvesTo []v1alpha1.DomainResolvesToEntry) []ngrok.ReservedDomainResolvesToEntry {
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

func buildCertificateInfo(certificate *ngrok.Ref) *v1alpha1.DomainStatusCertificateInfo {
	if certificate == nil || certificate.ID == "" {
		return nil
	}

	return &v1alpha1.DomainStatusCertificateInfo{
		ID: certificate.ID,
	}
}

func buildCertificateManagementPolicy(policy *ngrok.ReservedDomainCertPolicy) *v1alpha1.DomainStatusCertificateManagementPolicy {
	if policy == nil {
		return nil
	}

	return &v1alpha1.DomainStatusCertificateManagementPolicy{
		Authority:      policy.Authority,
		PrivateKeyType: policy.PrivateKeyType,
	}
}

func buildCertificateManagementStatus(status *ngrok.ReservedDomainCertStatus) *v1alpha1.DomainStatusCertificateManagementStatus {
	if status == nil {
		return nil
	}

	result := &v1alpha1.DomainStatusCertificateManagementStatus{}
	result.RenewsAt = parseRFC3339Pointer(status.RenewsAt)
	result.ProvisioningJob = buildProvisioningJob(status.ProvisioningJob)
	return result
}

func buildProvisioningJob(job *ngrok.ReservedDomainCertJob) *v1alpha1.DomainStatusProvisioningJob {
	if job == nil {
		return nil
	}

	result := &v1alpha1.DomainStatusProvisioningJob{
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
