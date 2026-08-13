package ingress

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	ingressv1alpha1 "github.com/ngrok/ngrok-operator/api/ingress/v1alpha1"
)

// Tests that ipPolicy condition ready is set correctly
func TestSetIPPolicyReadyCondition(t *testing.T) {
	ipPolicy := &ingressv1alpha1.IPPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:       "test-ip-policy",
			Generation: 1,
		},
	}

	setIPPolicyReadyCondition(ipPolicy, true, ReasonIPPolicyActive, "IP Policy is active")

	condition := meta.FindStatusCondition(ipPolicy.Status.Conditions, ConditionIPPolicyReady)
	if condition == nil {
		t.Fatal("Expected Ready condition to be set")
	}
	assert.Equal(t, metav1.ConditionTrue, condition.Status)
	assert.Equal(t, ReasonIPPolicyActive, condition.Reason)
	assert.Equal(t, "IP Policy is active", condition.Message)
	assert.Equal(t, int64(1), condition.ObservedGeneration)

	setIPPolicyReadyCondition(ipPolicy, false, ReasonIPPolicyCreationFailed, "Failed to create IP Policy")

	condition = meta.FindStatusCondition(ipPolicy.Status.Conditions, ConditionIPPolicyReady)
	if condition == nil {
		t.Fatal("Expected Ready condition to be set")
	}
	assert.Equal(t, metav1.ConditionFalse, condition.Status)
	assert.Equal(t, ReasonIPPolicyCreationFailed, condition.Reason)
	assert.Equal(t, "Failed to create IP Policy", condition.Message)
	assert.Equal(t, int64(1), condition.ObservedGeneration)
}

// Tests that ipPolicy condition created is set correctly
func TestSetIPPolicyCreatedCondition(t *testing.T) {
	ipPolicy := &ingressv1alpha1.IPPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:       "test-ip-policy",
			Generation: 1,
		},
	}

	setIPPolicyCreatedCondition(ipPolicy, true, ReasonIPPolicyCreated, "IP Policy has been created")

	condition := meta.FindStatusCondition(ipPolicy.Status.Conditions, ConditionIPPolicyCreated)
	if condition == nil {
		t.Fatal("Expected Created condition to be set")
	}
	assert.Equal(t, metav1.ConditionTrue, condition.Status)
	assert.Equal(t, ReasonIPPolicyCreated, condition.Reason)
	assert.Equal(t, "IP Policy has been created", condition.Message)
	assert.Equal(t, int64(1), condition.ObservedGeneration)

	setIPPolicyCreatedCondition(ipPolicy, false, ReasonIPPolicyCreationFailed, "Failed to create IP Policy")

	condition = meta.FindStatusCondition(ipPolicy.Status.Conditions, ConditionIPPolicyCreated)
	if condition == nil {
		t.Fatal("Expected Created condition to be set")
	}
	assert.Equal(t, metav1.ConditionFalse, condition.Status)
	assert.Equal(t, ReasonIPPolicyCreationFailed, condition.Reason)
	assert.Equal(t, "Failed to create IP Policy", condition.Message)
	assert.Equal(t, int64(1), condition.ObservedGeneration)
}

// Tests that ipPolicy condition rules configured is set correctly
func TestSetIPPolicyRulesConfiguredCondition(t *testing.T) {
	ipPolicy := &ingressv1alpha1.IPPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:       "test-ip-policy",
			Generation: 1,
		},
	}

	setIPPolicyRulesConfiguredCondition(ipPolicy, true, ReasonIPPolicyRulesConfigured, "IP Policy rules have been configured")

	condition := meta.FindStatusCondition(ipPolicy.Status.Conditions, ConditionIPPolicyRulesConfigured)
	if condition == nil {
		t.Fatal("Expected RulesConfigured condition to be set")
	}
	assert.Equal(t, metav1.ConditionTrue, condition.Status)
	assert.Equal(t, ReasonIPPolicyRulesConfigured, condition.Reason)
	assert.Equal(t, "IP Policy rules have been configured", condition.Message)
	assert.Equal(t, int64(1), condition.ObservedGeneration)

	setIPPolicyRulesConfiguredCondition(ipPolicy, false, ReasonIPPolicyRulesConfigurationError, "Failed to configure IP Policy rules")

	condition = meta.FindStatusCondition(ipPolicy.Status.Conditions, ConditionIPPolicyRulesConfigured)
	if condition == nil {
		t.Fatal("Expected RulesConfigured condition to be set")
	}
	assert.Equal(t, metav1.ConditionFalse, condition.Status)
	assert.Equal(t, ReasonIPPolicyRulesConfigurationError, condition.Reason)
	assert.Equal(t, "Failed to configure IP Policy rules", condition.Message)
	assert.Equal(t, int64(1), condition.ObservedGeneration)
}
