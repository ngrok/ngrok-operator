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
	"github.com/ngrok/ngrok-api-go/v9"
)

const (
	ruleActionAllow = "allow"
	ruleActionDeny  = "deny"
)

// ruleDiff represents the diff between the remote and spec rules for an IPPolicy.
// From the ngrok docs:
//
//	"IP Restrictions allow you to attach one or more IP policies to the route.
//	 If multiple IP policies are attached, a connection will be allowed only if
//	 its source IP matches at least one policy with an 'allow' action and
//	 does not match any policy with a 'deny' action."
//
// This provides an iterator of the rules that need to be created,updated, and deleted in order to update the remote securely.
type ruleDiff struct {
	idx int

	policyID string

	remoteDeny  map[string]*ngrok.IPPolicyRule
	remoteAllow map[string]*ngrok.IPPolicyRule
	specDeny    map[string]Rule
	specAllow   map[string]Rule

	creates []*ngrok.IPPolicyRuleCreate
	deletes []*ngrok.IPPolicyRule
	updates []*ngrok.IPPolicyRuleUpdate
}

func newRuleDiff(policyID string, remote []*ngrok.IPPolicyRule, spec []Rule) *ruleDiff {
	diff := &ruleDiff{
		policyID:    policyID,
		remoteDeny:  make(map[string]*ngrok.IPPolicyRule),
		remoteAllow: make(map[string]*ngrok.IPPolicyRule),
		specDeny:    make(map[string]Rule),
		specAllow:   make(map[string]Rule),
	}

	// Group the remote rules by their CIDR
	for _, rule := range remote {
		if rule.Action == ruleActionDeny {
			diff.remoteDeny[rule.CIDR] = rule
		} else {
			diff.remoteAllow[rule.CIDR] = rule
		}
	}

	// Group the spec rules by their CIDR
	for _, rule := range spec {
		if rule.Action == ruleActionDeny {
			diff.specDeny[rule.CIDR] = rule
		} else {
			diff.specAllow[rule.CIDR] = rule
		}
	}

	return diff
}

func (d *ruleDiff) Next() bool {
	defer func() { d.idx++ }()

	// Reset the diff
	d.creates = make([]*ngrok.IPPolicyRuleCreate, 0)
	d.deletes = make([]*ngrok.IPPolicyRule, 0)
	d.updates = make([]*ngrok.IPPolicyRuleUpdate, 0)

	switch d.idx {
	case 0: // Create all new deny rules that don't exist in the remote with a matching CIDR.
		for cidr, rule := range d.specDeny {
			if !d.existsInRemote(cidr) {
				d.creates = append(d.creates, d.createRule(rule))
			}
		}
		return true
	case 1: // Delete any allow rules with matching CIDRs that will be changing to deny rules. Then create the deny rules.
		for cidr, rule := range d.specDeny {
			if _, ok := d.remoteAllow[cidr]; ok {
				d.deletes = append(d.deletes, d.remoteAllow[cidr])
				d.creates = append(d.creates, d.createRule(rule))
			}
		}
		return true
	case 2: // Delete any deny rules with matching CIDRs that will be changing to allow rules. Then create the allow rules.
		for cidr, rule := range d.specAllow {
			if _, ok := d.remoteDeny[cidr]; ok {
				d.deletes = append(d.deletes, d.remoteDeny[cidr])
				d.creates = append(d.creates, d.createRule(rule))
			}
		}
		return true
	case 3: // Create all new allow rules that don't exist in the remote with a matching CIDR.
		for cidr, rule := range d.specAllow {
			if !d.existsInRemote(cidr) {
				d.creates = append(d.creates, d.createRule(rule))
			}
		}
		return true
	case 4: // Delete any remaining rules that are not in the spec.
		for cidr, rule := range d.remoteAllow {
			if !d.existsInSpec(cidr) {
				d.deletes = append(d.deletes, rule)
			}
		}
		for cidr, rule := range d.remoteDeny {
			if !d.existsInSpec(cidr) {
				d.deletes = append(d.deletes, rule)
			}
		}
		return true
	case 5: // Update any rules that exist in the spec and remote but have only different metadata/description.
		for cidr, rule := range d.specAllow {
			if remoteRule, ok := d.remoteAllow[cidr]; ok {
				d.addUpdateIfNeeded(rule, remoteRule)
			}
		}
		for cidr, rule := range d.specDeny {
			if remoteRule, ok := d.remoteDeny[cidr]; ok {
				d.addUpdateIfNeeded(rule, remoteRule)
			}
		}
		return true
	default:
	}

	return false
}

func (d *ruleDiff) NeedsCreate() []*ngrok.IPPolicyRuleCreate {
	return d.creates
}

func (d *ruleDiff) NeedsDelete() []*ngrok.IPPolicyRule {
	return d.deletes
}

func (d *ruleDiff) NeedsUpdate() []*ngrok.IPPolicyRuleUpdate {
	return d.updates
}

// existsInSpec returns true if the CIDR exists in the spec for either an allow or deny rule.
func (d *ruleDiff) existsInSpec(cidr string) bool {
	_, okDeny := d.specDeny[cidr]
	_, okAllow := d.specAllow[cidr]
	return okDeny || okAllow
}

// existsInRemote returns true if the CIDR exists in the remote for either an allow or deny rule.
func (d *ruleDiff) existsInRemote(cidr string) bool {
	_, okDeny := d.remoteDeny[cidr]
	_, okAllow := d.remoteAllow[cidr]
	return okDeny || okAllow
}

func (d *ruleDiff) createRule(rule Rule) *ngrok.IPPolicyRuleCreate {
	return &ngrok.IPPolicyRuleCreate{
		IPPolicyID:  d.policyID,
		CIDR:        rule.CIDR,
		Action:      new(rule.Action),
		Metadata:    rule.MetadataAPIString(),
		Description: rule.Description,
	}
}

func (d *ruleDiff) addUpdateIfNeeded(rule Rule, remoteRule *ngrok.IPPolicyRule) {
	updatedNeeded := rule.CIDR == remoteRule.CIDR &&
		rule.Action == remoteRule.Action &&
		(rule.MetadataAPIString() != remoteRule.Metadata || rule.Description != remoteRule.Description)
	if !updatedNeeded {
		return
	}

	d.updates = append(d.updates, &ngrok.IPPolicyRuleUpdate{
		ID:          remoteRule.ID,
		Metadata:    new(rule.MetadataAPIString()),
		Description: new(rule.Description),
		CIDR:        new(rule.CIDR),
	})
}
