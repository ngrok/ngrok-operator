package ingress

import (
	"testing"

	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	ingressv1alpha1 "github.com/ngrok/ngrok-operator/api/ingress/v1alpha1"
	"github.com/ngrok/ngrok-operator/internal/controller/conditions"
)

// Tests that ipPolicy condition ready is set correctly
func TestSetIPPolicyReadyCondition(t *testing.T) {
	ipPolicy := &ingressv1alpha1.IPPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:       "test-ip-policy",
			Generation: 1,
		},
	}

	setIPPolicyReadyCondition(ipPolicy, true, ingressv1alpha1.IPPolicyReasonActive, "IP Policy is active")

	condition := conditions.FindCondition(ipPolicy.Status.Conditions, ingressv1alpha1.IPPolicyConditionReady)
	if condition == nil {
		t.Fatal("Expected Ready condition to be set")
	}
	assert.Equal(t, metav1.ConditionTrue, condition.Status)
	assert.Equal(t, string(ingressv1alpha1.IPPolicyReasonActive), condition.Reason)
	assert.Equal(t, "IP Policy is active", condition.Message)
	assert.Equal(t, int64(1), condition.ObservedGeneration)

	setIPPolicyReadyCondition(ipPolicy, false, ingressv1alpha1.IPPolicyReasonCreationFailed, "Failed to create IP Policy")

	condition = conditions.FindCondition(ipPolicy.Status.Conditions, ingressv1alpha1.IPPolicyConditionReady)
	if condition == nil {
		t.Fatal("Expected Ready condition to be set")
	}
	assert.Equal(t, metav1.ConditionFalse, condition.Status)
	assert.Equal(t, string(ingressv1alpha1.IPPolicyReasonCreationFailed), condition.Reason)
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

	setIPPolicyCreatedCondition(ipPolicy, true, ingressv1alpha1.IPPolicyCreatedReasonCreated, "IP Policy has been created")

	condition := conditions.FindCondition(ipPolicy.Status.Conditions, ingressv1alpha1.IPPolicyConditionCreated)
	if condition == nil {
		t.Fatal("Expected Created condition to be set")
	}
	assert.Equal(t, metav1.ConditionTrue, condition.Status)
	assert.Equal(t, string(ingressv1alpha1.IPPolicyCreatedReasonCreated), condition.Reason)
	assert.Equal(t, "IP Policy has been created", condition.Message)
	assert.Equal(t, int64(1), condition.ObservedGeneration)

	setIPPolicyCreatedCondition(ipPolicy, false, ingressv1alpha1.IPPolicyCreatedReasonCreationFailed, "Failed to create IP Policy")

	condition = conditions.FindCondition(ipPolicy.Status.Conditions, ingressv1alpha1.IPPolicyConditionCreated)
	if condition == nil {
		t.Fatal("Expected Created condition to be set")
	}
	assert.Equal(t, metav1.ConditionFalse, condition.Status)
	assert.Equal(t, string(ingressv1alpha1.IPPolicyCreatedReasonCreationFailed), condition.Reason)
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

	setIPPolicyRulesConfiguredCondition(ipPolicy, true, ingressv1alpha1.IPPolicyRulesConfiguredReasonConfigured, "IP Policy rules have been configured")

	condition := conditions.FindCondition(ipPolicy.Status.Conditions, ingressv1alpha1.IPPolicyConditionRulesConfigured)
	if condition == nil {
		t.Fatal("Expected RulesConfigured condition to be set")
	}
	assert.Equal(t, metav1.ConditionTrue, condition.Status)
	assert.Equal(t, string(ingressv1alpha1.IPPolicyRulesConfiguredReasonConfigured), condition.Reason)
	assert.Equal(t, "IP Policy rules have been configured", condition.Message)
	assert.Equal(t, int64(1), condition.ObservedGeneration)

	setIPPolicyRulesConfiguredCondition(ipPolicy, false, ingressv1alpha1.IPPolicyRulesConfiguredReasonConfigurationError, "Failed to configure IP Policy rules")

	condition = conditions.FindCondition(ipPolicy.Status.Conditions, ingressv1alpha1.IPPolicyConditionRulesConfigured)
	if condition == nil {
		t.Fatal("Expected RulesConfigured condition to be set")
	}
	assert.Equal(t, metav1.ConditionFalse, condition.Status)
	assert.Equal(t, string(ingressv1alpha1.IPPolicyRulesConfiguredReasonConfigurationError), condition.Reason)
	assert.Equal(t, "Failed to configure IP Policy rules", condition.Message)
	assert.Equal(t, int64(1), condition.ObservedGeneration)
}
