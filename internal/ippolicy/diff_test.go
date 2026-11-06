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
	"testing"

	"github.com/ngrok/ngrok-api-go/v9"
	"github.com/stretchr/testify/assert"
)

func TestRuleDiff(t *testing.T) {
	remoteRules := []*ngrok.IPPolicyRule{
		{ID: "1", CIDR: "192.168.1.0/25", Action: ruleActionAllow, Description: "a"},
		{ID: "2", CIDR: "192.168.128.0/25", Action: ruleActionDeny, Description: "aa"},
		{ID: "3", CIDR: "172.16.0.0/16", Action: ruleActionAllow, Description: "aaa"},
		{ID: "4", CIDR: "172.17.0.0/16", Action: ruleActionDeny, Description: "aaaa"},
		{ID: "5", CIDR: "172.19.0.0/16", Action: ruleActionAllow, Description: "aaaaa"},
	}

	changedDescriptionRule := Rule{
		CIDR: "172.19.0.0/16", Action: ruleActionAllow, Description: "b",
	}

	specRules := []Rule{
		{CIDR: "192.168.1.0/25", Action: ruleActionDeny},
		{CIDR: "192.168.128.0/25", Action: ruleActionAllow},
		{CIDR: "10.0.0.0/8", Action: ruleActionDeny},
		{CIDR: "172.18.0.0/16", Action: ruleActionAllow},
		changedDescriptionRule,
	}

	diff := newRuleDiff("test", remoteRules, specRules)

	assert.True(t, diff.Next())
	assert.Empty(t, diff.NeedsDelete())
	assert.Empty(t, diff.NeedsUpdate())
	assert.Equal(t, []*ngrok.IPPolicyRuleCreate{{IPPolicyID: "test", CIDR: specRules[2].CIDR, Action: new(ruleActionDeny)}}, diff.NeedsCreate())

	assert.True(t, diff.Next())
	assert.Empty(t, diff.NeedsUpdate())
	assert.Equal(t, []*ngrok.IPPolicyRule{remoteRules[0]}, diff.NeedsDelete())
	assert.Equal(t, []*ngrok.IPPolicyRuleCreate{{IPPolicyID: "test", CIDR: specRules[0].CIDR, Action: new(ruleActionDeny)}}, diff.NeedsCreate())

	assert.True(t, diff.Next())
	assert.Empty(t, diff.NeedsUpdate())
	assert.Equal(t, []*ngrok.IPPolicyRule{remoteRules[1]}, diff.NeedsDelete())
	assert.Equal(t, []*ngrok.IPPolicyRuleCreate{{IPPolicyID: "test", CIDR: specRules[1].CIDR, Action: new(ruleActionAllow)}}, diff.NeedsCreate())

	assert.True(t, diff.Next())
	assert.Empty(t, diff.NeedsUpdate())
	assert.Empty(t, diff.NeedsDelete())
	assert.Equal(t, []*ngrok.IPPolicyRuleCreate{{IPPolicyID: "test", CIDR: specRules[3].CIDR, Action: new(ruleActionAllow)}}, diff.NeedsCreate())

	assert.True(t, diff.Next())
	assert.Empty(t, diff.NeedsUpdate())
	assert.ElementsMatch(t, []*ngrok.IPPolicyRule{remoteRules[2], remoteRules[3]}, diff.NeedsDelete())
	assert.Empty(t, diff.NeedsCreate())

	assert.True(t, diff.Next())
	assert.Empty(t, diff.NeedsDelete())
	assert.Empty(t, diff.NeedsCreate())
	assert.Equal(t, []*ngrok.IPPolicyRuleUpdate{{ID: "5", CIDR: new("172.19.0.0/16"), Description: new("b"), Metadata: new("")}}, diff.NeedsUpdate())

	assert.False(t, diff.Next())
}
