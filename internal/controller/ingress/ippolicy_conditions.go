package ingress

import (
	"net"

	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	ingressv1alpha1 "github.com/ngrok/ngrok-operator/api/ingress/v1alpha1"
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

// setReadyCOndition sets the Ready condition based on the overall IP policy state
func setIPPolicyReadyCondition(ipPolicy *ingressv1alpha1.IPPolicy, ready bool, reason, message string) {
	status := metav1.ConditionTrue
	if !ready {
		status = metav1.ConditionFalse
	}

	condition := metav1.Condition{
		Type:               ConditionIPPolicyReady,
		Status:             status,
		Reason:             reason,
		Message:            message,
		ObservedGeneration: ipPolicy.Generation,
	}

	meta.SetStatusCondition(&ipPolicy.Status.Conditions, condition)
}

// setIPPolicyCreatedCondition sets the IPPolicyCreated condition
func setIPPolicyCreatedCondition(ipPolicy *ingressv1alpha1.IPPolicy, created bool, reason, message string) {
	status := metav1.ConditionTrue
	if !created {
		status = metav1.ConditionFalse
	}

	condition := metav1.Condition{
		Type:               ConditionIPPolicyCreated,
		Status:             status,
		Reason:             reason,
		Message:            message,
		ObservedGeneration: ipPolicy.Generation,
	}

	meta.SetStatusCondition(&ipPolicy.Status.Conditions, condition)
}

// setIPPolicyRulesConfiguredCondition sets the RulesConfigured condition
func setIPPolicyRulesConfiguredCondition(ipPolicy *ingressv1alpha1.IPPolicy, configured bool, reason, message string) {
	status := metav1.ConditionTrue
	if !configured {
		status = metav1.ConditionFalse
	}

	condition := metav1.Condition{
		Type:               ConditionIPPolicyRulesConfigured,
		Status:             status,
		Reason:             reason,
		Message:            message,
		ObservedGeneration: ipPolicy.Generation,
	}

	meta.SetStatusCondition(&ipPolicy.Status.Conditions, condition)
}

// updateIPPolicyCondition updates all IP policy conditions based on the ngrok IP Policy state
func updateIPPolicyConditions(ipPolicy *ingressv1alpha1.IPPolicy) {
	// Check if the IP policy was created
	if ipPolicy.Status.ID == "" {
		message := "IP Policy not yet created"
		setIPPolicyCreatedCondition(ipPolicy, false, ReasonIPPolicyCreationFailed, message)
		setIPPolicyReadyCondition(ipPolicy, false, ReasonIPPolicyCreationFailed, message)
		setIPPolicyRulesConfiguredCondition(ipPolicy, false, ReasonIPPolicyCreationFailed, message)
		return
	}

	setIPPolicyCreatedCondition(ipPolicy, true, ReasonIPPolicyCreated, "IP Policy successfully created")

	// Check if rules are configured
	ngrokIPPolicyRules := ipPolicy.Spec.Rules
	if len(ngrokIPPolicyRules) == 0 {
		message := "No rules configured for IP Policy"
		setIPPolicyRulesConfiguredCondition(ipPolicy, false, ReasonIPPolicyRulesConfigurationError, message)
		setIPPolicyReadyCondition(ipPolicy, false, ReasonIPPolicyRulesConfigurationError, message)
		return
	}

	// Check for invalid CIDRs in rules
	for _, rule := range ngrokIPPolicyRules {
		if !isValidCIDR(rule.CIDR) {
			message := "Invalid CIDR in IP Policy rules"
			setIPPolicyRulesConfiguredCondition(ipPolicy, false, ReasonIPPolicyInvalidCIDR, message)
			setIPPolicyReadyCondition(ipPolicy, false, ReasonIPPolicyInvalidCIDR, message)
			return
		}
	}

	setIPPolicyRulesConfiguredCondition(ipPolicy, true, ReasonIPPolicyRulesConfigured, "All rules configured for IP Policy")

	// If all conditions are met, set the IP policy as ready
	setIPPolicyReadyCondition(ipPolicy, true, ReasonIPPolicyActive, "IP Policy is active")
}

// isValidCIDR checks if a CIDR string is valid
func isValidCIDR(cidr string) bool {
	_, _, err := net.ParseCIDR(cidr)
	return err == nil
}
