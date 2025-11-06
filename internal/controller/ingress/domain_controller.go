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
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"
	"k8s.io/client-go/util/workqueue"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/go-logr/logr"
	"github.com/ngrok/ngrok-api-go/v7"
	"github.com/ngrok/ngrok-operator/api/ingress/v1alpha1"
	basecontroller "github.com/ngrok/ngrok-operator/internal/controller"
	"github.com/ngrok/ngrok-operator/internal/ngrokapi"
)

// DomainReconciler reconciles a Domain object
type DomainReconciler struct {
	client.Client

	Log           logr.Logger
	Scheme        *runtime.Scheme
	Recorder      record.EventRecorder
	DomainsClient ngrokapi.DomainClient

	controller *basecontroller.BaseController[*v1alpha1.Domain]
}

// SetupWithManager sets up the controller with the Manager.
func (r *DomainReconciler) SetupWithManager(mgr ctrl.Manager) error {
	if r.DomainsClient == nil {
		return errors.New("DomainsClient must be set")
	}

	r.controller = &basecontroller.BaseController[*v1alpha1.Domain]{
		Kube:     r.Client,
		Log:      r.Log,
		Recorder: r.Recorder,

		StatusID: func(cr *v1alpha1.Domain) string { return cr.Status.ID },
		ClearStatus: func(cr *v1alpha1.Domain) {
			cr.Status = v1alpha1.DomainStatus{}
		},
		Create: r.create,
		Update: r.update,
		Delete: r.delete,
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

// +kubebuilder:rbac:groups=ingress.k8s.ngrok.com,resources=domains,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=ingress.k8s.ngrok.com,resources=domains/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=ingress.k8s.ngrok.com,resources=domains/finalizers,verbs=update

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.13.1/pkg/reconcile
func (r *DomainReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	result, err := r.controller.Reconcile(ctx, req, new(v1alpha1.Domain))
	if err != nil {
		return result, err
	}

	// Get the updated domain to check if we need requeuing
	domain := &v1alpha1.Domain{}
	if err := r.Get(ctx, req.NamespacedName, domain); err != nil {
		return result, client.IgnoreNotFound(err)
	}

	// Requeue if the domain is not ready
	if !IsDomainReady(domain) {
		// Requeue the event relying on the controllers custom RateLimiter for exponential backoff
		return ctrl.Result{Requeue: true}, nil
	}

	return result, nil
}

func (r *DomainReconciler) create(ctx context.Context, domain *v1alpha1.Domain) error {
	// First check if the reserved domain already exists. The API is sometimes returning dangling CNAME records
	// errors right now, so we'll check if the domain already exists before trying to create it.
	resp, err := r.findReservedDomainByHostname(ctx, domain.Spec.Domain)
	if err != nil {
		// Set conditions before returning error
		return r.updateStatus(ctx, domain, nil, err)
	}

	// Not found, so we'll create it
	if resp == nil {
		req := &ngrok.ReservedDomainCreate{
			Domain:      domain.Spec.Domain,
			Region:      domain.Spec.Region,
			Description: domain.Spec.Description,
			Metadata:    domain.Spec.Metadata,
		}
		resp, err = r.DomainsClient.Create(ctx, req)
		if err != nil {
			return r.updateStatus(ctx, domain, resp, err)
		}
	}

	return r.updateStatus(ctx, domain, resp, nil)
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

	// Only update the domain if the description or metadata has changed
	// These are the only fields that can be updated that we write to.
	if domain.Spec.Description == resp.Description && domain.Spec.Metadata == resp.Metadata {
		// No changes needed, still update status to ensure conditions are current
		return r.updateStatus(ctx, domain, resp, nil)
	}

	req := &ngrok.ReservedDomainUpdate{
		ID:          domain.Status.ID,
		Description: &domain.Spec.Description,
		Metadata:    &domain.Spec.Metadata,
	}
	resp, err = r.DomainsClient.Update(ctx, req)
	return r.updateStatus(ctx, domain, resp, err)
}

func (r *DomainReconciler) delete(ctx context.Context, domain *v1alpha1.Domain) error {
	if domain.Spec.ReclaimPolicy != v1alpha1.DomainReclaimPolicyDelete {
		return nil
	}

	return r.DomainsClient.Delete(ctx, domain.Status.ID)
}

// finds the reserved domain by the hostname. If it doesn't exist, returns nil
func (r *DomainReconciler) findReservedDomainByHostname(ctx context.Context, domainName string) (*ngrok.ReservedDomain, error) {
	iter := r.DomainsClient.List(&ngrok.Paging{})
	for iter.Next(ctx) {
		domain := iter.Item()
		if domain.Domain == domainName {
			return domain, nil
		}
	}
	return nil, nil
}

// updateStatus updates the status fields of the domain resource only if any values have changed
func (r *DomainReconciler) updateStatus(ctx context.Context, domain *v1alpha1.Domain, ngrokDomain *ngrok.ReservedDomain, createErr error) error {
	if ngrokDomain != nil {
		domain.Status.ID = ngrokDomain.ID
		domain.Status.Region = ngrokDomain.Region
		domain.Status.Domain = ngrokDomain.Domain
		domain.Status.CNAMETarget = ngrokDomain.CNAMETarget
		domain.Status.ACMEChallengeCNAMETarget = ngrokDomain.ACMEChallengeCNAMETarget

		domain.Status.Certificate = buildCertificateInfo(ngrokDomain.Certificate)
		domain.Status.CertificateManagementPolicy = buildCertificateManagementPolicy(ngrokDomain.CertificateManagementPolicy)
		domain.Status.CertificateManagementStatus = buildCertificateManagementStatus(ngrokDomain.CertificateManagementStatus)
	}

	updateDomainConditions(domain, ngrokDomain, createErr)
	return r.controller.ReconcileStatus(ctx, domain, createErr)
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
