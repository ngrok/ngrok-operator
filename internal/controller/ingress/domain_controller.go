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
	"time"

	v1 "k8s.io/api/core/v1"
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

	// Determine if we need to requeue based on domain state
	if r.shouldRequeue(domain) {
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
		return err
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
			return err
		}
	}

	return r.updateStatus(ctx, domain, resp)
}

func (r *DomainReconciler) update(ctx context.Context, domain *v1alpha1.Domain) error {
	resp, err := r.DomainsClient.Get(ctx, domain.Status.ID)
	if err != nil {
		// If the domain is gone, clear the status and trigger a re-reconcile
		if ngrok.IsNotFound(err) {
			domain.Status = v1alpha1.DomainStatus{}
			return r.controller.ReconcileStatus(ctx, domain, err)
		}

		return err
	}

	if domain.Equal(resp) {
		return nil
	}

	req := &ngrok.ReservedDomainUpdate{
		ID:          domain.Status.ID,
		Description: &domain.Spec.Description,
		Metadata:    &domain.Spec.Metadata,
	}
	resp, err = r.DomainsClient.Update(ctx, req)
	if err != nil {
		return err
	}
	return r.updateStatus(ctx, domain, resp)
}

func (r *DomainReconciler) delete(ctx context.Context, domain *v1alpha1.Domain) error {
	if domain.Spec.ReclaimPolicy != v1alpha1.DomainReclaimPolicyDelete {
		return nil
	}

	err := r.DomainsClient.Delete(ctx, domain.Status.ID)
	if err == nil || ngrok.IsNotFound(err) {
		domain.Status.ID = ""
	}
	return err
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
func (r *DomainReconciler) updateStatus(ctx context.Context, domain *v1alpha1.Domain, ngrokDomain *ngrok.ReservedDomain) error {

	domain.Status.ID = ngrokDomain.ID
	domain.Status.Region = ngrokDomain.Region
	domain.Status.Domain = ngrokDomain.Domain
	domain.Status.URI = ngrokDomain.URI
	domain.Status.CNAMETarget = ngrokDomain.CNAMETarget
	domain.Status.ACMEChallengeCNAMETarget = ngrokDomain.ACMEChallengeCNAMETarget

	// Set certificate information
	if ngrokDomain.Certificate != nil {
		domain.Status.Certificate = &v1alpha1.DomainStatusCertificateInfo{
			ID:  ngrokDomain.Certificate.ID,
			URI: ngrokDomain.Certificate.URI,
		}
	} else {
		domain.Status.Certificate = nil
	}

	// Set certificate management policy
	if ngrokDomain.CertificateManagementPolicy != nil {
		domain.Status.CertificateManagementPolicy = &v1alpha1.DomainStatusCertificateManagementPolicy{
			Authority:      ngrokDomain.CertificateManagementPolicy.Authority,
			PrivateKeyType: ngrokDomain.CertificateManagementPolicy.PrivateKeyType,
		}
	} else {
		domain.Status.CertificateManagementPolicy = nil
	}

	// Set certificate management status
	if ngrokDomain.CertificateManagementStatus != nil {
		status := &v1alpha1.DomainStatusCertificateManagementStatus{}

		// Parse renewal time if present
		if ngrokDomain.CertificateManagementStatus.RenewsAt != nil && *ngrokDomain.CertificateManagementStatus.RenewsAt != "" {
			if t, err := time.Parse(time.RFC3339, *ngrokDomain.CertificateManagementStatus.RenewsAt); err == nil {
				status.RenewsAt = &metav1.Time{Time: t}
			}
		}

		// Handle provisioning job
		if ngrokDomain.CertificateManagementStatus.ProvisioningJob != nil {
			job := &v1alpha1.DomainStatusProvisioningJob{
				Message: ngrokDomain.CertificateManagementStatus.ProvisioningJob.Msg,
			}

			if ngrokDomain.CertificateManagementStatus.ProvisioningJob.ErrorCode != nil {
				job.ErrorCode = *ngrokDomain.CertificateManagementStatus.ProvisioningJob.ErrorCode
			}

			if ngrokDomain.CertificateManagementStatus.ProvisioningJob.StartedAt != "" {
				if t, err := time.Parse(time.RFC3339, ngrokDomain.CertificateManagementStatus.ProvisioningJob.StartedAt); err == nil {
					job.StartedAt = &metav1.Time{Time: t}
				}
			}

			if ngrokDomain.CertificateManagementStatus.ProvisioningJob.RetriesAt != nil && *ngrokDomain.CertificateManagementStatus.ProvisioningJob.RetriesAt != "" {
				if t, err := time.Parse(time.RFC3339, *ngrokDomain.CertificateManagementStatus.ProvisioningJob.RetriesAt); err == nil {
					job.RetriesAt = &metav1.Time{Time: t}
				}
			}

			status.ProvisioningJob = job
		}

		domain.Status.CertificateManagementStatus = status
	} else {
		domain.Status.CertificateManagementStatus = nil
	}

	updateDomainConditions(domain, ngrokDomain)
	r.Recorder.Event(domain, v1.EventTypeNormal, "Updated", fmt.Sprintf("Updating Domain %s", domain.Name))
	return r.controller.ReconcileStatus(ctx, domain, nil)
}

// shouldRequeue determines if a domain should be requeued for status polling
// Uses certificate management data from ngrok API instead of hardcoded domain patterns
func (r *DomainReconciler) shouldRequeue(domain *v1alpha1.Domain) bool {
	// No requeue for domains that failed creation (no ID)
	if domain.Status.ID == "" {
		r.Log.V(1).Info("Not requeuing domain without ID", "domain", domain.Name)
		return false
	}

	// No requeue needed for ready domains
	if isDomainReady(domain) {
		r.Log.V(1).Info("Not requeuing ready domain", "domain", domain.Name)
		return false
	}

	// Check if domain needs certificate management (based on API response data)
	if r.needsCertificatePolling(domain) {
		r.Log.V(1).Info("Requeuing domain for certificate polling", "domain", domain.Name,
			"has_cert_policy", domain.Status.CertificateManagementPolicy != nil,
			"has_cert_status", domain.Status.CertificateManagementStatus != nil,
			"has_cert", domain.Status.Certificate != nil)
		return true
	}

	// Requeue if we don't have conditions set yet (initial setup)
	if len(domain.Status.Conditions) == 0 {
		r.Log.V(1).Info("Requeuing domain to set initial conditions", "domain", domain.Name)
		return true
	}

	r.Log.V(1).Info("Not requeuing domain - appears ready", "domain", domain.Name)
	return false
}

// needsCertificatePolling determines if a domain needs certificate status polling
// based on the actual certificate management data from ngrok API
func (r *DomainReconciler) needsCertificatePolling(domain *v1alpha1.Domain) bool {
	// If there's a certificate management policy, this is a custom domain that may need polling
	if domain.Status.CertificateManagementPolicy != nil {
		// If there's an active provisioning job, definitely needs polling
		if domain.Status.CertificateManagementStatus != nil &&
			domain.Status.CertificateManagementStatus.ProvisioningJob != nil {
			return true
		}

		// If no certificate yet but has policy, might need polling
		if domain.Status.Certificate == nil {
			return true
		}
	}

	// ngrok-managed domains (no certificate management policy) don't need polling
	return false
}
