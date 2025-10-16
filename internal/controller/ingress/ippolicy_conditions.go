package ingress

import (
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

// Checks if the IP policy is ready based on other conditions
func IsIPPolicyReady(ipPolicy *ingressv1alpha1.IPPolicy) bool {
	// Check if the CreatedCondition is set to true
	createdCondition := meta.FindStatusCondition(ipPolicy.Status.Conditions, ConditionIPPolicyCreated)
	rulesConfiguredCondition := meta.FindStatusCondition(ipPolicy.Status.Conditions, ConditionIPPolicyRulesConfigured)
	if createdCondition != nil && createdCondition.Status == metav1.ConditionTrue && rulesConfiguredCondition != nil && rulesConfiguredCondition.Status == metav1.ConditionTrue {
		return true
	}

	return false
}

<<<<<<< HEAD
=======
// Checks if Rules Configuration condition has a value
func HasIPPolicyRulesConfiguredCondition(ipPolicy *ingressv1alpha1.IPPolicy) bool {
	rulesConfiguredCondition := meta.FindStatusCondition(ipPolicy.Status.Conditions, ConditionIPPolicyRulesConfigured)
	return rulesConfiguredCondition != nil
}

>>>>>>> 1347be6 (Adding status conditions for the IP policy along with unit tests)
// Checks if Rules Configuration condition is true
func IsIPPolicyRulesConfigured(ipPolicy *ingressv1alpha1.IPPolicy) bool {
	rulesConfiguredCondition := meta.FindStatusCondition(ipPolicy.Status.Conditions, ConditionIPPolicyRulesConfigured)
	return rulesConfiguredCondition != nil && rulesConfiguredCondition.Status == metav1.ConditionTrue
}
