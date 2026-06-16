package ingress

import (
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	ingressv1alpha1 "github.com/ngrok/ngrok-operator/api/ingress/v1alpha1"
	"github.com/ngrok/ngrok-operator/internal/controller/conditions"
)

const (
	// condition types for IPPolicy
	ConditionIPPolicyReady           = "Ready"
	ConditionIPPolicyCreated         = "IPPolicyCreated"
	ConditionIPPolicyRulesConfigured = "RulesConfigured"

	// condition reasons for IPPolicy
	ReasonIPPolicyActive                  = "IPPolicyActive"
	ReasonIPPolicyCreated                 = "IPPolicyCreated"
	ReasonIPPolicyRulesConfigured         = "IPPolicyRulesConfigured"
	ReasonIPPolicyRulesConfigurationError = "IPPolicyRulesConfigurationError"
	ReasonIPPolicyInvalidCIDR             = "IPPolicyInvalidCIDR"
	ReasonIPPolicyCreationFailed          = "IPPolicyCreationFailed"
)

// setIPPolicyReadyCondition sets the Ready condition based on the overall IP policy state
func setIPPolicyReadyCondition(ipPolicy *ingressv1alpha1.IPPolicy, ready bool, reason, message string) {
	conditions.Set(&ipPolicy.Status.Conditions, ipPolicy.Generation, ConditionIPPolicyReady, ready, reason, message)
}

// setIPPolicyCreatedCondition sets the IPPolicyCreated condition
func setIPPolicyCreatedCondition(ipPolicy *ingressv1alpha1.IPPolicy, created bool, reason, message string) {
	conditions.Set(&ipPolicy.Status.Conditions, ipPolicy.Generation, ConditionIPPolicyCreated, created, reason, message)
}

// setIPPolicyRulesConfiguredCondition sets the RulesConfigured condition
func setIPPolicyRulesConfiguredCondition(ipPolicy *ingressv1alpha1.IPPolicy, configured bool, reason, message string) {
	conditions.Set(&ipPolicy.Status.Conditions, ipPolicy.Generation, ConditionIPPolicyRulesConfigured, configured, reason, message)
}

// sets the Ready condition based on the other conditions
func calculateIPPolicyReadyCondition(ipPolicy *ingressv1alpha1.IPPolicy) {
	// check IP Policy created condition
	ipPolicyCreated := hasIPPolicyCreatedCondition(ipPolicy)

	// check IP Policy rules configured condition
	ipPolicyRulesConfigured := false
	rulesConfiguredCondition := meta.FindStatusCondition(ipPolicy.Status.Conditions, ConditionIPPolicyRulesConfigured)
	if rulesConfiguredCondition != nil && rulesConfiguredCondition.Status == metav1.ConditionTrue {
		ipPolicyRulesConfigured = true
	}

	switch {
	case ipPolicyCreated && ipPolicyRulesConfigured:
		setIPPolicyReadyCondition(ipPolicy, true, ReasonIPPolicyActive, "IP Policy is active")
	case ipPolicyCreated && !ipPolicyRulesConfigured:
		setIPPolicyReadyCondition(ipPolicy, false, ReasonIPPolicyRulesConfigurationError, "IP Policy rules are not configured")
	default:
		setIPPolicyReadyCondition(ipPolicy, false, ReasonIPPolicyCreationFailed, "IP Policy is not ready")
	}
}

func hasIPPolicyCreatedCondition(ipPolicy *ingressv1alpha1.IPPolicy) bool {
	if ipPolicy == nil {
		return false
	}
	createdCondition := meta.FindStatusCondition(ipPolicy.Status.Conditions, ConditionIPPolicyCreated)
	return createdCondition != nil && createdCondition.Status == metav1.ConditionTrue
}
