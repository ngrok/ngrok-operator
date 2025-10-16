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
func TestIPPolicyDiff_EmptyRemoteRules(t *testing.T) {
	specRules := []ingressv1alpha1.IPPolicyRule{
		{CIDR: "10.0.0.0/8", Action: IPPolicyRuleActionAllow},
		{CIDR: "192.168.1.0/25", Action: IPPolicyRuleActionDeny},
	}
	diff := newIPPolicyDiff("test", nil, specRules)

	assert.True(t, diff.Next())
	assert.Empty(t, diff.NeedsDelete())
	assert.Empty(t, diff.NeedsUpdate())
	assert.Equal(t, []*ngrok.IPPolicyRuleCreate{
		{IPPolicyID: "test", CIDR: specRules[0].CIDR, Action: ptr.To(IPPolicyRuleActionAllow)},
	}, diff.NeedsCreate())

	assert.True(t, diff.Next())
	assert.Empty(t, diff.NeedsDelete())
	assert.Empty(t, diff.NeedsUpdate())
	assert.Equal(t, []*ngrok.IPPolicyRuleCreate{
		{IPPolicyID: "test", CIDR: specRules[1].CIDR, Action: ptr.To(IPPolicyRuleActionDeny)},
	}, diff.NeedsCreate())

	assert.False(t, diff.Next())
}

func TestIPPolicyDiff_EmptySpecRules(t *testing.T) {
	remoteRules := []*ngrok.IPPolicyRule{
		{ID: "1", CIDR: "10.0.0.0/8", Action: IPPolicyRuleActionAllow, Description: "desc"},
		{ID: "2", CIDR: "192.168.1.0/25", Action: IPPolicyRuleActionDeny, Description: "desc2"},
	}
	diff := newIPPolicyDiff("test", remoteRules, nil)

	assert.True(t, diff.Next())
	assert.Empty(t, diff.NeedsUpdate())
	assert.Equal(t, []*ngrok.IPPolicyRule{remoteRules[0], remoteRules[1]}, diff.NeedsDelete())
	assert.Empty(t, diff.NeedsCreate())

	assert.False(t, diff.Next())
}

func TestIPPolicyDiff_NoChanges(t *testing.T) {
	remoteRules := []*ngrok.IPPolicyRule{
		{ID: "1", CIDR: "10.0.0.0/8", Action: IPPolicyRuleActionAllow, Description: "desc"},
		{ID: "2", CIDR: "192.168.1.0/25", Action: IPPolicyRuleActionDeny, Description: "desc2"},
	}
	specRules := []ingressv1alpha1.IPPolicyRule{
		{CIDR: "10.0.0.0/8", Action: IPPolicyRuleActionAllow, Description: "desc"},
		{CIDR: "192.168.1.0/25", Action: IPPolicyRuleActionDeny, Description: "desc2"},
	}
	diff := newIPPolicyDiff("test", remoteRules, specRules)

	assert.False(t, diff.Next())
}

func TestIPPolicyDiff_MetadataChange(t *testing.T) {
	remoteRules := []*ngrok.IPPolicyRule{
		{ID: "1", CIDR: "10.0.0.0/8", Action: IPPolicyRuleActionAllow, Description: "desc", Metadata: "oldmeta"},
	}
	specRules := []ingressv1alpha1.IPPolicyRule{
		{CIDR: "10.0.0.0/8", Action: IPPolicyRuleActionAllow, Description: "desc"},
	}
	diff := newIPPolicyDiff("test", remoteRules, specRules)

	assert.True(t, diff.Next())
	assert.Empty(t, diff.NeedsDelete())
	assert.Empty(t, diff.NeedsCreate())
	assert.Equal(t, []*ngrok.IPPolicyRuleUpdate{
		{ID: "1", CIDR: ptr.To("10.0.0.0/8"), Description: ptr.To("desc"), Metadata: ptr.To("")},
	}, diff.NeedsUpdate())

	assert.False(t, diff.Next())
}

func TestIPPolicyDiff_ActionChangeOnly(t *testing.T) {
	remoteRules := []*ngrok.IPPolicyRule{
		{ID: "1", CIDR: "10.0.0.0/8", Action: IPPolicyRuleActionAllow, Description: "desc"},
	}
	specRules := []ingressv1alpha1.IPPolicyRule{
		{CIDR: "10.0.0.0/8", Action: IPPolicyRuleActionDeny, Description: "desc"},
	}
	diff := newIPPolicyDiff("test", remoteRules, specRules)

	assert.True(t, diff.Next())
	assert.Empty(t, diff.NeedsUpdate())
	assert.Equal(t, []*ngrok.IPPolicyRule{remoteRules[0]}, diff.NeedsDelete())
	assert.Equal(t, []*ngrok.IPPolicyRuleCreate{
		{IPPolicyID: "test", CIDR: specRules[0].CIDR, Action: ptr.To(IPPolicyRuleActionDeny)},
	}, diff.NeedsCreate())

	assert.False(t, diff.Next())
}

func TestCreateIPPolicyRules_CreatesRules(t *testing.T) {
	client := &mockNgrokClient{}
	ipPolicyID := "test-policy"
	rules := []ingressv1alpha1.IPPolicyRule{
		{CIDR: "10.0.0.0/8", Action: IPPolicyRuleActionAllow, Description: "desc1"},
		{CIDR: "192.168.1.0/24", Action: IPPolicyRuleActionDeny, Description: "desc2"},
	}

	created := []*ngrok.IPPolicyRuleCreate{}
	client.On("CreateIPPolicyRule", mock.Anything).Run(func(args mock.Arguments) {
		created = append(created, args.Get(0).(*ngrok.IPPolicyRuleCreate))
	}).Return(&ngrok.IPPolicyRule{}, nil)

	err := createIPPolicyRules(client, ipPolicyID, rules)
	assert.NoError(t, err)
	assert.Len(t, created, 2)
	assert.Equal(t, ipPolicyID, created[0].IPPolicyID)
	assert.Equal(t, "10.0.0.0/8", created[0].CIDR)
	assert.Equal(t, IPPolicyRuleActionAllow, *created[0].Action)
	assert.Equal(t, "desc1", created[0].Description)
	assert.Equal(t, "192.168.1.0/24", created[1].CIDR)
	assert.Equal(t, IPPolicyRuleActionDeny, *created[1].Action)
	assert.Equal(t, "desc2", created[1].Description)
}

func TestUpdateIPPolicyRules_UpdatesRules(t *testing.T) {
	client := &mockNgrokClient{}
	updates := []*ngrok.IPPolicyRuleUpdate{
		{ID: "1", CIDR: ptr.To("10.0.0.0/8"), Description: ptr.To("newdesc")},
		{ID: "2", CIDR: ptr.To("192.168.1.0/24"), Action: ptr.To(IPPolicyRuleActionDeny)},
	}

	updated := []*ngrok.IPPolicyRuleUpdate{}
	client.On("UpdateIPPolicyRule", mock.Anything).Run(func(args mock.Arguments) {
		updated = append(updated, args.Get(0).(*ngrok.IPPolicyRuleUpdate))
	}).Return(&ngrok.IPPolicyRule{}, nil)

	err := updateIPPolicyRules(client, updates)
	assert.NoError(t, err)
	assert.Len(t, updated, 2)
	assert.Equal(t, "1", updated[0].ID)
	assert.Equal(t, "newdesc", *updated[0].Description)
	assert.Equal(t, "2", updated[1].ID)
	assert.Equal(t, IPPolicyRuleActionDeny, *updated[1].Action)
}

func TestCreateOrUpdateIPPolicyRules_CreatesAndUpdates(t *testing.T) {
	client := &mockNgrokClient{}
	ipPolicyID := "test-policy"
	createRules := []ingressv1alpha1.IPPolicyRule{
		{CIDR: "10.0.0.0/8", Action: IPPolicyRuleActionAllow, Description: "desc1"},
	}
	updateRules := []*ngrok.IPPolicyRuleUpdate{
		{ID: "2", CIDR: ptr.To("192.168.1.0/24"), Action: ptr.To(IPPolicyRuleActionDeny)},
	}

	created := []*ngrok.IPPolicyRuleCreate{}
	updated := []*ngrok.IPPolicyRuleUpdate{}
	client.On("CreateIPPolicyRule", mock.Anything).Run(func(args mock.Arguments) {
		created = append(created, args.Get(0).(*ngrok.IPPolicyRuleCreate))
	}).Return(&ngrok.IPPolicyRule{}, nil)
	client.On("UpdateIPPolicyRule", mock.Anything).Run(func(args mock.Arguments) {
		updated = append(updated, args.Get(0).(*ngrok.IPPolicyRuleUpdate))
	}).Return(&ngrok.IPPolicyRule{}, nil)

	err := createOrUpdateIPPolicyRules(client, ipPolicyID, createRules, updateRules)
	assert.NoError(t, err)
	assert.Len(t, created, 1)
	assert.Len(t, updated, 1)
	assert.Equal(t, "10.0.0.0/8", created[0].CIDR)
	assert.Equal(t, "2", updated[0].ID)
}

// Mock client for ngrok API
type mockNgrokClient struct {
	mock.Mock
}

func (m *mockNgrokClient) CreateIPPolicyRule(rule *ngrok.IPPolicyRuleCreate) (*ngrok.IPPolicyRule, error) {
	args := m.Called(rule)
	return args.Get(0).(*ngrok.IPPolicyRule), args.Error(1)
}

func (m *mockNgrokClient) UpdateIPPolicyRule(rule *ngrok.IPPolicyRuleUpdate) (*ngrok.IPPolicyRule, error) {
	args := m.Called(rule)
	return args.Get(0).(*ngrok.IPPolicyRule), args.Error(1)
}
