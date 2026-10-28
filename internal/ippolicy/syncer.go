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

package ippolicy

import (
	"context"
	"fmt"

	"github.com/ngrok/ngrok-api-go/v9"
	v1 "k8s.io/api/core/v1"
	"k8s.io/client-go/tools/events"
	ctrl "sigs.k8s.io/controller-runtime"

	"github.com/ngrok/ngrok-operator/internal/ngrokapi"
)

// Syncer pushes one IP-policy resource's spec to the ngrok API: the policy
// itself and its rules. It works on any kind through IPPolicyResource, and
// only changes the object in memory (status ID and conditions). Writing the
// status back is the caller's job, so a reconciler that handles several
// objects in one pass decides when each status write happens.
type Syncer struct {
	IPPolicies ngrokapi.IPPoliciesClient
	Rules      ngrokapi.IPPolicyRulesClient
	Recorder   events.EventRecorder
}

// Sync creates the remote policy if p has no ID yet, otherwise updates it,
// then configures its rules. If the remote policy has disappeared, p's
// status is reset so the next reconcile recreates it.
func (s *Syncer) Sync(ctx context.Context, p IPPolicyResource) error {
	if p.GetID() == "" {
		return s.create(ctx, p)
	}
	return s.update(ctx, p)
}

// Delete deletes p's remote policy. A policy that is already gone counts as
// deleted. On success, p's ID is cleared.
func (s *Syncer) Delete(ctx context.Context, p IPPolicyResource) error {
	err := s.IPPolicies.Delete(ctx, p.GetID())
	if err == nil || ngrok.IsNotFound(err) {
		p.SetID("")
	}
	return err
}

func (s *Syncer) create(ctx context.Context, p IPPolicyResource) error {
	remotePolicy, err := s.IPPolicies.Create(ctx, &ngrok.IPPolicyCreate{
		Description: p.GetDescription(),
		Metadata:    p.GetMetadataAPIString(),
	})
	if err != nil {
		SetCreatedCondition(p, false, ReasonIPPolicyCreationFailed, err.Error())
		return err
	}

	p.SetID(remotePolicy.ID)
	SetCreatedCondition(p, true, ReasonIPPolicyCreated, "IP Policy successfully created")

	err = s.createOrUpdateRules(ctx, p)

	CalculateReadyCondition(p)

	return err
}

func (s *Syncer) update(ctx context.Context, p IPPolicyResource) error {
	remotePolicy, err := s.IPPolicies.Get(ctx, p.GetID())
	if err != nil {
		if ngrok.IsNotFound(err) {
			p.ResetStatus() // Clear status so we recreate
		} else {
			SetCreatedCondition(p, false, ReasonIPPolicyCreationFailed, err.Error())
			SetRulesConfiguredCondition(p, false, ReasonIPPolicyCreationFailed, err.Error())
			SetReadyCondition(p, false, ReasonIPPolicyCreationFailed, err.Error())
		}

		return err
	}

	// If we found the remote policy, set the Created condition to true
	// if it was not already set.
	SetCreatedCondition(p, true, ReasonIPPolicyCreated, "IP Policy already exists")

	if remotePolicy.Description != p.GetDescription() ||
		remotePolicy.Metadata != p.GetMetadataAPIString() {
		s.Recorder.Eventf(p, nil, v1.EventTypeNormal, "Updating", "Update", fmt.Sprintf("Updating IPPolicy %s", p.GetName()))
		_, err := s.IPPolicies.Update(ctx, &ngrok.IPPolicyUpdate{
			ID:          p.GetID(),
			Description: new(p.GetDescription()),
			Metadata:    new(p.GetMetadataAPIString()),
		})
		if err != nil {
			SetReadyCondition(p, false, ReasonIPPolicyCreationFailed, err.Error())
			return err
		}
		s.Recorder.Eventf(p, nil, v1.EventTypeNormal, "Updated", "Update", fmt.Sprintf("Updated IPPolicy %s", p.GetName()))
	}

	err = s.createOrUpdateRules(ctx, p)

	CalculateReadyCondition(p)

	return err
}

func (s *Syncer) createOrUpdateRules(ctx context.Context, p IPPolicyResource) error {
	log := ctrl.LoggerFrom(ctx)
	policyID := p.GetID()

	specRules, err := DecodeRules(p)
	if err != nil {
		SetRulesConfiguredCondition(p, false, ReasonIPPolicyRulesConfigurationError, err.Error())
		return err
	}

	remoteRules, err := s.getRemotePolicyRules(ctx, policyID)
	if err != nil {
		return err
	}

	if len(remoteRules) == 0 {
		SetRulesConfiguredCondition(p, false, ReasonIPPolicyRulesConfigurationError, "No rules configured for IP Policy")
	}

	iter := newRuleDiff(policyID, remoteRules, specRules)

	setConditionsBasedOnErr := func(err error) {
		if err == nil {
			return
		}

		if ngrok.IsErrorCode(err, ngrokapi.NgrokOpErrInvalidCIDR.Code) {
			SetRulesConfiguredCondition(p, false, ReasonIPPolicyInvalidCIDR, err.Error())
			SetReadyCondition(p, false, ReasonIPPolicyInvalidCIDR, err.Error())
			return
		}

		SetRulesConfiguredCondition(p, false, ReasonIPPolicyRulesConfigurationError, err.Error())
		SetReadyCondition(p, false, ReasonIPPolicyRulesConfigurationError, err.Error())
	}

	for iter.Next() {
		for _, d := range iter.NeedsDelete() {
			log.V(3).Info("Deleting IP Policy Rule", "id", d.ID, "policy.id", policyID, "cidr", d.CIDR, "action", d.Action)
			if err := s.Rules.Delete(ctx, d.ID); err != nil {
				setConditionsBasedOnErr(err)
				return err
			}
			log.V(3).Info("Deleted IP Policy Rule", "id", d.ID)
		}

		for _, c := range iter.NeedsCreate() {
			log.V(3).Info("Creating IP Policy Rule", "policy.id", policyID, "cidr", c.CIDR, "action", c.Action)
			rule, err := s.Rules.Create(ctx, c)
			if err != nil {
				setConditionsBasedOnErr(err)
				return err
			}
			log.V(3).Info("Created IP Policy Rule", "id", rule.ID, "policy.id", policyID, "cidr", rule.CIDR, "action", rule.Action)
		}

		for _, u := range iter.NeedsUpdate() {
			log.V(3).Info("Updating IP Policy Rule", "id", u.ID, "policy.id", policyID, "cidr", u.CIDR, "metadata", u.Metadata, "description", u.Description)
			rule, err := s.Rules.Update(ctx, u)
			if err != nil {
				setConditionsBasedOnErr(err)
				return err
			}
			log.V(3).Info("Updated IP Policy Rule", "id", rule.ID, "policy.id", policyID)
		}
	}

	SetRulesConfiguredCondition(p, true, ReasonIPPolicyRulesConfigured, "All rules configured for IP Policy")
	return nil
}

func (s *Syncer) getRemotePolicyRules(ctx context.Context, policyID string) ([]*ngrok.IPPolicyRule, error) {
	iter := s.Rules.List(&ngrok.FilteredPaging{})
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
