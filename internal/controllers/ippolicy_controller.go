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
	"k8s.io/utils/pointer"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/go-logr/logr"
	ingressv1alpha1 "github.com/ngrok/kubernetes-ingress-controller/api/v1alpha1"
	"github.com/ngrok/ngrok-api-go/v5"
	"github.com/ngrok/ngrok-api-go/v5/ip_policies"
	"github.com/ngrok/ngrok-api-go/v5/ip_policy_rules"
)

const (
	IPPolicyRuleActionAllow = "allow"
	IPPolicyRuleActionDeny  = "deny"
)

// IPPolicyReconciler reconciles a IPPolicy object
type IPPolicyReconciler struct {
	client.Client

	Log      logr.Logger
	Scheme   *runtime.Scheme
	Recorder record.EventRecorder

	IPPoliciesClient    *ip_policies.Client
	IPPolicyRulesClient *ip_policy_rules.Client
}

// SetupWithManager sets up the controller with the Manager.
func (r *IPPolicyReconciler) SetupWithManager(mgr ctrl.Manager) error {
	if r.IPPoliciesClient == nil {
		return fmt.Errorf("IPPoliciesClient must be set")
	}
	if r.IPPolicyRulesClient == nil {
		return fmt.Errorf("IPPolicyRulesClient must be set")
	}

	return ctrl.NewControllerManagedBy(mgr).
		For(&ingressv1alpha1.IPPolicy{}).
		Complete(r)
}

//+kubebuilder:rbac:groups=ingress.k8s.ngrok.com,resources=ippolicies,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=ingress.k8s.ngrok.com,resources=ippolicies/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=ingress.k8s.ngrok.com,resources=ippolicies/finalizers,verbs=update

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.14.1/pkg/reconcile
func (r *IPPolicyReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := r.Log.WithValues("V1Alpha1IPPolicy", req.NamespacedName)

	policy := new(ingressv1alpha1.IPPolicy)
	if err := r.Get(ctx, req.NamespacedName, policy); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	if policy.ObjectMeta.DeletionTimestamp.IsZero() {
		if err := registerAndSyncFinalizer(ctx, r.Client, policy); err != nil {
			return ctrl.Result{}, err
		}
	} else {
		// The object is being deleted
		if hasFinalizer(policy) {
			if policy.Status.ID != "" {
				log.Info("Deleting IP Policy", "ID", policy.Status.ID)
				r.Recorder.Event(policy, v1.EventTypeNormal, "Deleting", fmt.Sprintf("Deleting policy %s", policy.Name))
				if err := r.IPPoliciesClient.Delete(ctx, policy.Status.ID); err != nil {
					if !ngrok.IsNotFound(err) {
						r.Recorder.Event(policy, v1.EventTypeWarning, "FailedDelete", fmt.Sprintf("Failed to delete IPPolicy %s: %s", policy.Name, err.Error()))
						return ctrl.Result{}, err
					}
					log.Info("Domain not found, assuming it was already deleted", "ID", policy.Status.ID)
				}
				policy.Status.ID = ""
			}

			if err := removeAndSyncFinalizer(ctx, r.Client, policy); err != nil {
				return ctrl.Result{}, err
			}
		}

		r.Recorder.Event(policy, v1.EventTypeNormal, "Deleted", fmt.Sprintf("Deleted IPPolicy %s", policy.Name))

		// Stop reconciliation as the item is being deleted
		return ctrl.Result{}, nil
	}

	if err := r.createOrUpdateIPPolicy(ctx, policy); err != nil {
		return ctrl.Result{}, err
	}
	return ctrl.Result{}, r.createOrUpdateIPPolicyRules(ctx, policy)
}

func (r *IPPolicyReconciler) deleteRemoteResoures(ctx context.Context, policy *ingressv1alpha1.IPPolicy) error {
	return r.IPPoliciesClient.Delete(ctx, policy.Status.ID)
}

func (r *IPPolicyReconciler) createOrUpdateIPPolicy(ctx context.Context, policy *ingressv1alpha1.IPPolicy) error {
	if policy.Status.ID == "" {
		r.Recorder.Event(policy, v1.EventTypeNormal, "Creating", fmt.Sprintf("Creating IPPolicy %s", policy.Name))
		// Create the IP Policy since it doesn't exist
		remotePolicy, err := r.IPPoliciesClient.Create(ctx, &ngrok.IPPolicyCreate{
			Description: policy.Spec.Description,
			Metadata:    policy.Spec.Metadata,
		})
		if err != nil {
			return err
		}
		r.Recorder.Event(policy, v1.EventTypeNormal, "Created", fmt.Sprintf("Created IPPolicy %s", policy.Name))
		policy.Status.ID = remotePolicy.ID
		return r.Status().Update(ctx, policy)
	}

	// Update the IP Policy since it already exists
	remotePolicy, err := r.IPPoliciesClient.Get(ctx, policy.Status.ID)
	if err != nil {
		if ngrok.IsNotFound(err) {
			policy.Status.ID = ""
			return r.Status().Update(ctx, policy)
		}
		return err
	}
	if remotePolicy.Description != policy.Spec.Description ||
		remotePolicy.Metadata != policy.Spec.Metadata {
		r.Recorder.Event(policy, v1.EventTypeNormal, "Updating", fmt.Sprintf("Updating IPPolicy %s", policy.Name))
		_, err := r.IPPoliciesClient.Update(ctx, &ngrok.IPPolicyUpdate{
			ID:          policy.Status.ID,
			Description: pointer.String(policy.Spec.Description),
			Metadata:    pointer.String(policy.Spec.Metadata),
		})
		if err != nil {
			return err
		}
		r.Recorder.Event(policy, v1.EventTypeNormal, "Updated", fmt.Sprintf("Updated IPPolicy %s", policy.Name))
	}

	return nil
}

func (r *IPPolicyReconciler) createOrUpdateIPPolicyRules(ctx context.Context, policy *ingressv1alpha1.IPPolicy) error {
	remoteRules, err := r.getRemotePolicyRules(ctx, policy.Status.ID)
	if err != nil {
		return err
	}
	diff := newIPPolicyDiff(remoteRules, policy.Spec.Rules)
	for _, rule := range diff.needCreate {
		rule.IPPolicyID = policy.Status.ID
		_, err := r.IPPolicyRulesClient.Create(ctx, rule)
		if err != nil {
			return err
		}
	}

	for _, rule := range diff.needDelete {
		err := r.IPPolicyRulesClient.Delete(ctx, rule.ID)
		if err != nil {
			return err
		}
	}

	return nil
}

func (r *IPPolicyReconciler) getRemotePolicyRules(ctx context.Context, policyID string) ([]*ngrok.IPPolicyRule, error) {
	iter := r.IPPolicyRulesClient.List(&ngrok.Paging{})
	rules := make([]*ngrok.IPPolicyRule, 0)

	for iter.Next(ctx) {
		rule := iter.Item()
		// Filter to only rules that contain this policy ID
		if rule.IPPolicy.ID == policyID {
			rules = append(rules, rule)
		}
	}

	return rules, iter.Err()
}

type IPPolicyDiff struct {
	needCreate []*ngrok.IPPolicyRuleCreate
	needDelete []*ngrok.IPPolicyRule
}

func newIPPolicyDiff(remote []*ngrok.IPPolicyRule, spec []ingressv1alpha1.IPPolicyRule) *IPPolicyDiff {
	remoteDeny := make(map[string]*ngrok.IPPolicyRule)
	remoteAllow := make(map[string]*ngrok.IPPolicyRule)
	specDeny := make(map[string]ingressv1alpha1.IPPolicyRule)
	specAllow := make(map[string]ingressv1alpha1.IPPolicyRule)

	for _, rule := range remote {
		if rule.Action == IPPolicyRuleActionDeny {
			remoteDeny[rule.CIDR] = rule
		} else {
			remoteAllow[rule.CIDR] = rule
		}
	}

	for _, rule := range spec {
		if rule.Action == IPPolicyRuleActionDeny {
			specDeny[rule.CIDR] = rule
		} else {
			specAllow[rule.CIDR] = rule
		}
	}

	diff := &IPPolicyDiff{
		needCreate: make([]*ngrok.IPPolicyRuleCreate, 0),
		needDelete: make([]*ngrok.IPPolicyRule, 0),
	}

	for cidr, specRule := range specAllow {
		if _, ok := remoteAllow[cidr]; !ok {
			diff.needCreate = append(diff.needCreate, &ngrok.IPPolicyRuleCreate{
				Action:      pointer.String(IPPolicyRuleActionAllow),
				Description: specRule.Description,
				Metadata:    specRule.Metadata,
				CIDR:        cidr,
			})
		}
	}

	for cidr, specRule := range specDeny {
		if _, ok := remoteDeny[cidr]; !ok {
			diff.needCreate = append(diff.needCreate, &ngrok.IPPolicyRuleCreate{
				Action:      pointer.String(IPPolicyRuleActionDeny),
				Description: specRule.Description,
				Metadata:    specRule.Metadata,
				CIDR:        cidr,
			})
		}
	}

	for cidr, rule := range remoteAllow {
		if _, ok := specAllow[cidr]; !ok {
			diff.needDelete = append(diff.needDelete, rule)
		}
	}

	for cidr, rule := range remoteDeny {
		if _, ok := specDeny[cidr]; !ok {
			diff.needDelete = append(diff.needDelete, rule)
		}
	}

	return diff
}
