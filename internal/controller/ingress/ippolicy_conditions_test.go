package ingress

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	ingressv1alpha1 "github.com/ngrok/ngrok-operator/api/ingress/v1alpha1"
)

// test ip policy not created yet
func TestIpPolicyNotCreated(t *testing.T) {
	test := struct {
		name     string
		ipPolicy *ingressv1alpha1.IPPolicy
	}{
		name:     "IP Policy Ready",
		ipPolicy: &ingressv1alpha1.IPPolicy{},
	}

	t.Run("IP Policy Ready", func(t *testing.T) {
		updateIPPolicyConditions(test.ipPolicy)

		// Check Ready condition
		readyCondition := meta.FindStatusCondition(test.ipPolicy.Status.Conditions, ConditionIPPolicyReady)
		assert.NotNil(t, readyCondition)
		assert.Equal(t, metav1.ConditionFalse, readyCondition.Status)
		assert.Equal(t, ReasonIPPolicyCreationFailed, readyCondition.Reason)
		assert.Equal(t, "IP Policy not yet created", readyCondition.Message)

		// Check IPPolicyCreated condition
		createdCondition := meta.FindStatusCondition(test.ipPolicy.Status.Conditions, ConditionIPPolicyCreated)
		assert.NotNil(t, createdCondition)
		assert.Equal(t, metav1.ConditionFalse, createdCondition.Status)
		assert.Equal(t, ReasonIPPolicyCreationFailed, createdCondition.Reason)
		assert.Equal(t, "IP Policy not yet created", createdCondition.Message)

		// Check RulesConfigured condition
		rulesConfiguredCondition := meta.FindStatusCondition(test.ipPolicy.Status.Conditions, ConditionIPPolicyRulesConfigured)
		assert.NotNil(t, rulesConfiguredCondition)
		assert.Equal(t, metav1.ConditionFalse, rulesConfiguredCondition.Status)
		assert.Equal(t, ReasonIPPolicyCreationFailed, rulesConfiguredCondition.Reason)
		assert.Equal(t, "IP Policy not yet created", rulesConfiguredCondition.Message)

	})
}

// test ip policy rules not configured yet
func TestIpPolicyRulesNotConfigured(t *testing.T) {
	test := struct {
		name     string
		ipPolicy *ingressv1alpha1.IPPolicy
	}{
		name: "IP Policy Ready",
		ipPolicy: &ingressv1alpha1.IPPolicy{
			ObjectMeta: metav1.ObjectMeta{
				Name:       "test-ip-policy",
				Generation: 1,
			},
			Spec: ingressv1alpha1.IPPolicySpec{},
			Status: ingressv1alpha1.IPPolicyStatus{
				ID: "ip_123",
			},
		},
	}

	t.Run("IP Policy Ready", func(t *testing.T) {
		updateIPPolicyConditions(test.ipPolicy)

		// Check Ready condition
		readyCondition := meta.FindStatusCondition(test.ipPolicy.Status.Conditions, ConditionIPPolicyReady)
		assert.NotNil(t, readyCondition)
		assert.Equal(t, metav1.ConditionFalse, readyCondition.Status)
		assert.Equal(t, ReasonIPPolicyRulesConfigurationError, readyCondition.Reason)
		assert.Equal(t, "No rules configured for IP Policy", readyCondition.Message)

		// Check IPPolicyCreated condition
		createdCondition := meta.FindStatusCondition(test.ipPolicy.Status.Conditions, ConditionIPPolicyCreated)
		assert.NotNil(t, createdCondition)
		assert.Equal(t, metav1.ConditionTrue, createdCondition.Status)
		assert.Equal(t, ReasonIPPolicyCreated, createdCondition.Reason)
		assert.Equal(t, "IP Policy successfully created", createdCondition.Message)

		// Check RulesConfigured condition
		rulesConfiguredCondition := meta.FindStatusCondition(test.ipPolicy.Status.Conditions, ConditionIPPolicyRulesConfigured)
		assert.NotNil(t, rulesConfiguredCondition)
		assert.Equal(t, metav1.ConditionFalse, rulesConfiguredCondition.Status)
		assert.Equal(t, ReasonIPPolicyRulesConfigurationError, rulesConfiguredCondition.Reason)
		assert.Equal(t, "No rules configured for IP Policy", rulesConfiguredCondition.Message)

	})
}

// test ip policy rule have invalid cidr
func TestIpPolicyRuleInvalidCIDR(t *testing.T) {
	test := struct {
		name     string
		ipPolicy *ingressv1alpha1.IPPolicy
	}{
		name: "IP Policy Invalid CIDR",
		ipPolicy: &ingressv1alpha1.IPPolicy{
			ObjectMeta: metav1.ObjectMeta{
				Name:       "test-ip-policy-invalid-cidr",
				Generation: 1,
			},
			Spec: ingressv1alpha1.IPPolicySpec{
				Rules: []ingressv1alpha1.IPPolicyRule{
					{
						Action: IPPolicyRuleActionAllow,
						CIDR:   "1.2.3.155/323",
					},
				},
			},
			Status: ingressv1alpha1.IPPolicyStatus{
				ID: "ip_456",
			},
		},
	}

	t.Run("IP Policy Invalid CIDR", func(t *testing.T) {
		updateIPPolicyConditions(test.ipPolicy)

		// Check Ready condition
		readyCondition := meta.FindStatusCondition(test.ipPolicy.Status.Conditions, ConditionIPPolicyReady)
		assert.NotNil(t, readyCondition)
		assert.Equal(t, metav1.ConditionFalse, readyCondition.Status)
		assert.Equal(t, ReasonIPPolicyInvalidCIDR, readyCondition.Reason)
		assert.Equal(t, "Invalid CIDR in IP Policy rules", readyCondition.Message)

		// Check IPPolicyCreated condition
		createdCondition := meta.FindStatusCondition(test.ipPolicy.Status.Conditions, ConditionIPPolicyCreated)
		assert.NotNil(t, createdCondition)
		assert.Equal(t, metav1.ConditionTrue, createdCondition.Status)
		assert.Equal(t, ReasonIPPolicyCreated, createdCondition.Reason)
		assert.Equal(t, "IP Policy successfully created", createdCondition.Message)

		// Check RulesConfigured condition
		rulesConfiguredCondition := meta.FindStatusCondition(test.ipPolicy.Status.Conditions, ConditionIPPolicyRulesConfigured)
		assert.NotNil(t, rulesConfiguredCondition)
		assert.Equal(t, metav1.ConditionFalse, rulesConfiguredCondition.Status)
		assert.Equal(t, ReasonIPPolicyInvalidCIDR, rulesConfiguredCondition.Reason)
		assert.Equal(t, "Invalid CIDR in IP Policy rules", rulesConfiguredCondition.Message)

	})
}

// test ip policy ready
func TestIpPolicyReady(t *testing.T) {
	test := struct {
		name     string
		ipPolicy *ingressv1alpha1.IPPolicy
	}{
		name: "IP Policy Ready",
		ipPolicy: &ingressv1alpha1.IPPolicy{
			ObjectMeta: metav1.ObjectMeta{
				Name:       "test-ip-policy",
				Generation: 1,
			},
			Spec: ingressv1alpha1.IPPolicySpec{
				Rules: []ingressv1alpha1.IPPolicyRule{
					{
						Action: IPPolicyRuleActionAllow,
						CIDR:   "1.2.3.155/32",
					},
				},
			},
			Status: ingressv1alpha1.IPPolicyStatus{
				ID: "ip_123",
			},
		},
	}

	t.Run("IP Policy Ready", func(t *testing.T) {
		updateIPPolicyConditions(test.ipPolicy)

		// Check Ready condition
		readyCondition := meta.FindStatusCondition(test.ipPolicy.Status.Conditions, ConditionIPPolicyReady)
		assert.NotNil(t, readyCondition)
		assert.Equal(t, metav1.ConditionTrue, readyCondition.Status)
		assert.Equal(t, ReasonIPPolicyActive, readyCondition.Reason)
		assert.Equal(t, "IP Policy is active", readyCondition.Message)

		// Check IPPolicyCreated condition
		createdCondition := meta.FindStatusCondition(test.ipPolicy.Status.Conditions, ConditionIPPolicyCreated)
		assert.NotNil(t, createdCondition)
		assert.Equal(t, metav1.ConditionTrue, createdCondition.Status)
		assert.Equal(t, ReasonIPPolicyCreated, createdCondition.Reason)
		assert.Equal(t, "IP Policy successfully created", createdCondition.Message)

		// Check RulesConfigured condition
		rulesConfiguredCondition := meta.FindStatusCondition(test.ipPolicy.Status.Conditions, ConditionIPPolicyRulesConfigured)
		assert.NotNil(t, rulesConfiguredCondition)
		assert.Equal(t, metav1.ConditionTrue, rulesConfiguredCondition.Status)
		assert.Equal(t, ReasonIPPolicyRulesConfigured, rulesConfiguredCondition.Reason)
		assert.Equal(t, "All rules configured for IP Policy", rulesConfiguredCondition.Message)

	})
}
