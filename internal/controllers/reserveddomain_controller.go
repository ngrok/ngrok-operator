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
	"os"

	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"

	"github.com/go-logr/logr"
	"github.com/ngrok/ngrok-api-go/v5"
	"github.com/ngrok/ngrok-api-go/v5/reserved_domains"
	ingressv1alpha1 "github.com/ngrok/ngrok-ingress-controller/api/v1alpha1"
)

// ReservedDomainReconciler reconciles a ReservedDomain object
type ReservedDomainReconciler struct {
	client.Client

	Log                   logr.Logger
	Scheme                *runtime.Scheme
	Recorder              record.EventRecorder
	ReservedDomainsClient *reserved_domains.Client
}

//+kubebuilder:rbac:groups=ingress.k8s.ngrok.com,resources=reserveddomains,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=ingress.k8s.ngrok.com,resources=reserveddomains/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=ingress.k8s.ngrok.com,resources=reserveddomains/finalizers,verbs=update

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the ReservedDomain object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.13.1/pkg/reconcile
func (r *ReservedDomainReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := r.Log.WithValues("V1Alpha1ReservedDomain", req.NamespacedName)

	domain := new(ingressv1alpha1.ReservedDomain)
	if err := r.Get(ctx, req.NamespacedName, domain); err != nil {
		log.Error(err, "unable to fetch ReservedDomain")
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	if domain == nil {
		return ctrl.Result{}, nil
	}

	if domain.ObjectMeta.DeletionTimestamp.IsZero() {
		if err := registerAndSyncFinalizer(ctx, r.Client, domain); err != nil {
			return ctrl.Result{}, err
		}
	} else {
		// The object is being deleted
		if hasFinalizer(domain) {
			if domain.Status.ID != "" {
				r.Recorder.Event(domain, v1.EventTypeNormal, "Deleting", fmt.Sprintf("Deleting Reserved Domain %s", domain.Name))
				// Question: Do we actually want to delete the reserved domains for real? Or maybe just delete the resource and have the user delete the reserved domain from
				// the ngrok dashboard manually?
				if err := r.ReservedDomainsClient.Delete(ctx, domain.Status.ID); err != nil {
					r.Recorder.Event(domain, v1.EventTypeWarning, "FailedDelete", fmt.Sprintf("Failed to delete Reserved Domain %s: %s", domain.Name, err.Error()))
					return ctrl.Result{}, err
				}

				removeFinalizer(domain)
				if err := r.Update(ctx, domain); err != nil {
					return ctrl.Result{}, err
				}
				r.Recorder.Event(domain, v1.EventTypeNormal, "Deleted", fmt.Sprintf("Deleted Reserved Domain %s", domain.Name))
			}

			// We don't have the ID, so can't delete the resource. We'll just remove the finalizer for now.
			removeFinalizer(domain)
			if err := r.Update(ctx, domain); err != nil {
				return ctrl.Result{}, err
			}
		}
	}

	if domain.Status.ID != "" {
		if err := r.updateExternalResources(ctx, domain); err != nil {
			r.Recorder.Event(domain, v1.EventTypeWarning, "UpdateFailed", fmt.Sprintf("Failed to update Reserved Domain %s: %s", domain.Name, err.Error()))
			return ctrl.Result{}, err
		}

		r.Recorder.Event(domain, v1.EventTypeNormal, "Updated", fmt.Sprintf("Updated Reserved Domain %s", domain.Name))

	} else {
		// Create
		if err := r.createExternalResources(ctx, domain); err != nil {
			r.Recorder.Event(domain, v1.EventTypeWarning, "CreateFailed", fmt.Sprintf("Failed to create Reserved Domain %s: %s", domain.Name, err.Error()))
			return ctrl.Result{}, err
		}

		r.Recorder.Event(domain, v1.EventTypeNormal, "Created", fmt.Sprintf("Created Reserved Domain %s", domain.Name))
	}

	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *ReservedDomainReconciler) SetupWithManager(mgr ctrl.Manager) error {
	c, err := controller.New("reserved-domain-controller", mgr, controller.Options{
		Reconciler: r,
		LogConstructor: func(_ *reconcile.Request) logr.Logger {
			return r.Log
		},
	})

	if err != nil {
		return err
	}

	if err := c.Watch(
		&source.Kind{Type: &ingressv1alpha1.ReservedDomain{}},
		&handler.EnqueueRequestForObject{},
		commonPredicateFilters,
	); err != nil {
		return err
	}

	apiKey := os.Getenv("NGROK_API_KEY")
	if apiKey == "" {
		return fmt.Errorf("NGROK_API_KEY environment variable must be set")
	}
	config := ngrok.NewClientConfig(
		apiKey,
		ngrok.WithUserAgent("ngrok-ingress-controller/v1-alpha"),
	)
	r.ReservedDomainsClient = reserved_domains.NewClient(config)

	return nil
}

func (r *ReservedDomainReconciler) registerFinalizer(ctx context.Context, domain *ingressv1alpha1.ReservedDomain) error {
	if hasFinalizer(domain) {
		// Finalizer already exists, nothing to do
		return nil
	}

	addFinalizer(domain)
	return r.Update(ctx, domain)
}

// Deletes the external resources associated with the ReservedDomain. This is just the reserved domain itself.
func (r *ReservedDomainReconciler) deleteExternalResources(ctx context.Context, domain *ingressv1alpha1.ReservedDomain) error {
	return r.ReservedDomainsClient.Delete(ctx, domain.Status.ID)
}

func (r *ReservedDomainReconciler) createExternalResources(ctx context.Context, domain *ingressv1alpha1.ReservedDomain) error {
	req := &ngrok.ReservedDomainCreate{
		Domain:      domain.Spec.Domain,
		Region:      domain.Spec.Region,
		Description: domain.Spec.Description,
		Metadata:    domain.Spec.Metadata,
	}
	resp, err := r.ReservedDomainsClient.Create(ctx, req)
	if err != nil {
		// Let's check if the domain already exists. If it does, we'll just update the status fields
		resp, err = r.findReservedDomainByHostname(ctx, domain.Spec.Domain)
		if err != nil {
			return err
		}
	}

	return r.updateStatus(ctx, domain, resp)
}

func (r *ReservedDomainReconciler) updateExternalResources(ctx context.Context, domain *ingressv1alpha1.ReservedDomain) error {
	resp, err := r.ReservedDomainsClient.Get(ctx, domain.Status.ID)
	if err != nil {
		return err
	}

	// TODO: Implement update logic. Right now we just get the reserved domain and update the status
	// fields

	return r.updateStatus(ctx, domain, resp)
}

// finds the reserved domain by the hostname. If it doesn't exist, returns nil
func (r *ReservedDomainReconciler) findReservedDomainByHostname(ctx context.Context, domainName string) (*ngrok.ReservedDomain, error) {
	iter := r.ReservedDomainsClient.List(&ngrok.Paging{})
	for iter.Next(ctx) {
		domain := iter.Item()
		if domain.Domain == domainName {
			return domain, nil
		}
	}
	return nil, nil
}

func (r *ReservedDomainReconciler) updateStatus(ctx context.Context, domain *ingressv1alpha1.ReservedDomain, ngrokDomain *ngrok.ReservedDomain) error {
	domain.Status.ID = ngrokDomain.ID
	domain.Status.CNAMETarget = ngrokDomain.CNAMETarget
	domain.Status.URI = ngrokDomain.URI
	domain.Status.Domain = ngrokDomain.Domain
	domain.Status.Region = ngrokDomain.Region

	return r.Status().Update(ctx, domain)
}
