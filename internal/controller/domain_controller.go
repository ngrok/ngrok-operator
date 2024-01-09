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

package controllers

import (
	"context"
	"fmt"

	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/go-logr/logr"
	ingressv1alpha1 "github.com/ngrok/kubernetes-ingress-controller/api/v1alpha1"
	"github.com/ngrok/ngrok-api-go/v5"
	"github.com/ngrok/ngrok-api-go/v5/reserved_domains"
)

// DomainReconciler reconciles a Domain object
type DomainReconciler struct {
	client.Client

	Log           logr.Logger
	Scheme        *runtime.Scheme
	Recorder      record.EventRecorder
	DomainsClient *reserved_domains.Client

	controller *baseController[*ingressv1alpha1.Domain]
}

// SetupWithManager sets up the controller with the Manager.
func (r *DomainReconciler) SetupWithManager(mgr ctrl.Manager) error {
	if r.DomainsClient == nil {
		return fmt.Errorf("DomainsClient must be set")
	}

	r.controller = &baseController[*ingressv1alpha1.Domain]{
		Kube:     r.Client,
		Log:      r.Log,
		Recorder: r.Recorder,

		kubeType: "v1alpha1.Domain",
		statusID: func(cr *ingressv1alpha1.Domain) string { return cr.Status.ID },
		create:   r.create,
		update:   r.update,
		delete:   r.delete,
		errResult: func(op baseControllerOp, cr *ingressv1alpha1.Domain, err error) (reconcile.Result, error) {
			// Domain still attached to an edge, probably a race condition.
			// Schedule for retry, and hopefully the edge will be gone
			// eventually.
			if ngrok.IsErrorCode(err, 446) {
				return ctrl.Result{}, err
			}
			return reconcileResultFromError(err)
		},
	}

	return ctrl.NewControllerManagedBy(mgr).
		For(&ingressv1alpha1.Domain{}).
		WithEventFilter(commonPredicateFilters).
		Complete(r)
}

//+kubebuilder:rbac:groups=ingress.k8s.ngrok.com,resources=domains,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=ingress.k8s.ngrok.com,resources=domains/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=ingress.k8s.ngrok.com,resources=domains/finalizers,verbs=update

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.13.1/pkg/reconcile
func (r *DomainReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	return r.controller.reconcile(ctx, req, new(ingressv1alpha1.Domain))
}

func (r *DomainReconciler) create(ctx context.Context, domain *ingressv1alpha1.Domain) error {
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

func (r *DomainReconciler) update(ctx context.Context, domain *ingressv1alpha1.Domain) error {
	resp, err := r.DomainsClient.Get(ctx, domain.Status.ID)
	if err != nil {
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

func (r *DomainReconciler) delete(ctx context.Context, domain *ingressv1alpha1.Domain) error {
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
func (r *DomainReconciler) updateStatus(ctx context.Context, domain *ingressv1alpha1.Domain, ngrokDomain *ngrok.ReservedDomain) error {
	if domain.Equal(ngrokDomain) {
		return nil
	}
	domain.SetStatus(ngrokDomain)
	r.Recorder.Event(domain, v1.EventTypeNormal, "Updated", fmt.Sprintf("Updating Domain %s", domain.Name))
	return r.Status().Update(ctx, domain)
}
