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

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/events"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	"github.com/go-logr/logr"
	ingressv1alpha1 "github.com/ngrok/ngrok-operator/api/ingress/v1alpha1"
	"github.com/ngrok/ngrok-operator/internal/controller"
	"github.com/ngrok/ngrok-operator/internal/ippolicy"
	"github.com/ngrok/ngrok-operator/internal/ngrokapi"
)

// IPPolicyReconciler reconciles a IPPolicy object
type IPPolicyReconciler struct {
	client.Client

	Log      logr.Logger
	Scheme   *runtime.Scheme
	Recorder events.EventRecorder

	IPPoliciesClient    ngrokapi.IPPoliciesClient
	IPPolicyRulesClient ngrokapi.IPPolicyRulesClient
	DrainState          controller.DrainState

	controller *controller.BaseController[*ingressv1alpha1.IPPolicy]
	syncer     *ippolicy.Syncer
}

// SetupWithManager sets up the controller with the Manager.
func (r *IPPolicyReconciler) SetupWithManager(mgr ctrl.Manager) error {
	if r.IPPoliciesClient == nil {
		return errors.New("IPPoliciesClient must be set")
	}
	if r.IPPolicyRulesClient == nil {
		return errors.New("IPPolicyRulesClient must be set")
	}

	r.syncer = &ippolicy.Syncer{
		IPPolicies: r.IPPoliciesClient,
		Rules:      r.IPPolicyRulesClient,
		Recorder:   r.Recorder,
	}

	r.controller = &controller.BaseController[*ingressv1alpha1.IPPolicy]{
		Kube:       r.Client,
		Log:        r.Log,
		Recorder:   r.Recorder,
		DrainState: r.DrainState,

		StatusID: func(cr *ingressv1alpha1.IPPolicy) string { return cr.GetID() },
		Create:   r.sync,
		Update:   r.sync,
		Delete:   r.delete,
	}

	return ctrl.NewControllerManagedBy(mgr).
		For(&ingressv1alpha1.IPPolicy{}, builder.WithPredicates(
			predicate.Or(
				predicate.AnnotationChangedPredicate{},
				predicate.GenerationChangedPredicate{},
			),
		)).
		Complete(r)
}

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.14.1/pkg/reconcile
func (r *IPPolicyReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	return r.controller.Reconcile(ctx, req, new(ingressv1alpha1.IPPolicy))
}

func (r *IPPolicyReconciler) sync(ctx context.Context, policy *ingressv1alpha1.IPPolicy) error {
	return r.controller.ReconcileStatus(ctx, policy, r.syncer.Sync(ctx, policy))
}

func (r *IPPolicyReconciler) delete(ctx context.Context, policy *ingressv1alpha1.IPPolicy) error {
	return r.syncer.Delete(ctx, policy)
}
