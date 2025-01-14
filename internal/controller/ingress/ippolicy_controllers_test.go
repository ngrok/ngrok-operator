package ingress

import (
	"testing"

	"github.com/ngrok/ngrok-api-go/v7"
	ingressv1alpha1 "github.com/ngrok/ngrok-operator/api/ingress/v1alpha1"
	"github.com/stretchr/testify/assert"
	"k8s.io/utils/ptr"
)

func TestIPPolicyDiff(t *testing.T) {
	remoteRules := []*ngrok.IPPolicyRule{
		{ID: "1", CIDR: "192.168.1.0/25", Action: IPPolicyRuleActionAllow, Description: "a"},    // 2. Rule changed from allow to deny
		{ID: "2", CIDR: "192.168.128.0/25", Action: IPPolicyRuleActionDeny, Description: "aa"},  // 3. Rule changed from deny to allow
		{ID: "3", CIDR: "172.16.0.0/16", Action: IPPolicyRuleActionAllow, Description: "aaa"},   // 5. Allow Rule that will no longer exist
		{ID: "4", CIDR: "172.17.0.0/16", Action: IPPolicyRuleActionDeny, Description: "aaaa"},   // 5. Deny Rule that will no longer exist
		{ID: "5", CIDR: "172.19.0.0/16", Action: IPPolicyRuleActionAllow, Description: "aaaaa"}, // 6. Just changing description
	}
	changedDescriptionRule := ingressv1alpha1.IPPolicyRule{CIDR: "172.19.0.0/16", Action: IPPolicyRuleActionAllow}
	changedDescriptionRule.Description = "b"

	specRules := []ingressv1alpha1.IPPolicyRule{
		{CIDR: "192.168.1.0/25", Action: IPPolicyRuleActionDeny},    // 2. Rule changed from allow to deny
		{CIDR: "192.168.128.0/25", Action: IPPolicyRuleActionAllow}, // 3. Rule changed from deny to allow
		{CIDR: "10.0.0.0/8", Action: IPPolicyRuleActionDeny},        // 1. New Rule to be denied
		{CIDR: "172.18.0.0/16", Action: IPPolicyRuleActionAllow},    // 4. New Rule to be allowed
		changedDescriptionRule, // 6. Just changing description
	}

	diff := newIPPolicyDiff("test", remoteRules, specRules)

	assert.True(t, diff.Next())
	assert.Empty(t, diff.NeedsDelete())
	assert.Empty(t, diff.NeedsUpdate())
	assert.Equal(t, []*ngrok.IPPolicyRuleCreate{
		{IPPolicyID: "test", CIDR: specRules[2].CIDR, Action: ptr.To(IPPolicyRuleActionDeny)}},
		diff.NeedsCreate(),
	)

	assert.True(t, diff.Next())
	assert.Empty(t, diff.NeedsUpdate())
	assert.Equal(t, []*ngrok.IPPolicyRule{remoteRules[0]}, diff.NeedsDelete())
	assert.Equal(t, []*ngrok.IPPolicyRuleCreate{
		{IPPolicyID: "test", CIDR: specRules[0].CIDR, Action: ptr.To(IPPolicyRuleActionDeny)},
	}, diff.NeedsCreate())

	assert.True(t, diff.Next())
	assert.Empty(t, diff.NeedsUpdate())
	assert.Equal(t, []*ngrok.IPPolicyRule{remoteRules[1]}, diff.NeedsDelete())
	assert.Equal(t, []*ngrok.IPPolicyRuleCreate{
		{IPPolicyID: "test", CIDR: specRules[1].CIDR, Action: ptr.To(IPPolicyRuleActionAllow)},
	}, diff.NeedsCreate())

	assert.True(t, diff.Next())
	assert.Empty(t, diff.NeedsUpdate())
	assert.Equal(t, []*ngrok.IPPolicyRule{}, diff.NeedsDelete())
	assert.Equal(t, []*ngrok.IPPolicyRuleCreate{
		{IPPolicyID: "test", CIDR: specRules[3].CIDR, Action: ptr.To(IPPolicyRuleActionAllow)},
	}, diff.NeedsCreate())

	assert.True(t, diff.Next())
	assert.Empty(t, diff.NeedsUpdate())
	assert.Equal(t, []*ngrok.IPPolicyRule{remoteRules[2], remoteRules[3]}, diff.NeedsDelete())
	assert.Equal(t, []*ngrok.IPPolicyRuleCreate{}, diff.NeedsCreate())

	assert.True(t, diff.Next())
	assert.Empty(t, diff.NeedsDelete())
	assert.Empty(t, diff.NeedsCreate())
	assert.Equal(t, []*ngrok.IPPolicyRuleUpdate{
		{ID: "5", CIDR: ptr.To("172.19.0.0/16"), Description: ptr.To("b"), Metadata: ptr.To("")},
	}, diff.NeedsUpdate())

	assert.False(t, diff.Next())
}
