package ingress

import (
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	ingressv1alpha1 "github.com/ngrok/ngrok-operator/api/ingress/v1alpha1"
)

// setIPPolicyReadyCondition sets the Ready condition based on the overall IP policy state
func setIPPolicyReadyCondition(ipPolicy *ingressv1alpha1.IPPolicy, ready bool, reason ingressv1alpha1.IPPolicyConditionReadyReason, message string) {
	status := metav1.ConditionTrue
	if !ready {
		status = metav1.ConditionFalse
	}

	condition := metav1.Condition{
		Type:               string(ingressv1alpha1.IPPolicyConditionReady),
		Status:             status,
		Reason:             string(reason),
		Message:            message,
		ObservedGeneration: ipPolicy.Generation,
	}

	meta.SetStatusCondition(&ipPolicy.Status.Conditions, condition)
}

// setIPPolicyCreatedCondition sets the IPPolicyCreated condition
func setIPPolicyCreatedCondition(ipPolicy *ingressv1alpha1.IPPolicy, created bool, reason ingressv1alpha1.IPPolicyConditionCreatedReason, message string) {
	status := metav1.ConditionTrue
	if !created {
		status = metav1.ConditionFalse
	}

	condition := metav1.Condition{
		Type:               string(ingressv1alpha1.IPPolicyConditionCreated),
		Status:             status,
		Reason:             string(reason),
		Message:            message,
		ObservedGeneration: ipPolicy.Generation,
	}

	meta.SetStatusCondition(&ipPolicy.Status.Conditions, condition)
}

// setIPPolicyRulesConfiguredCondition sets the RulesConfigured condition
func setIPPolicyRulesConfiguredCondition(ipPolicy *ingressv1alpha1.IPPolicy, configured bool, reason ingressv1alpha1.IPPolicyConditionRulesConfiguredReason, message string) {
	status := metav1.ConditionTrue
	if !configured {
		status = metav1.ConditionFalse
	}

	condition := metav1.Condition{
		Type:               string(ingressv1alpha1.IPPolicyConditionRulesConfigured),
		Status:             status,
		Reason:             string(reason),
		Message:            message,
		ObservedGeneration: ipPolicy.Generation,
	}

	meta.SetStatusCondition(&ipPolicy.Status.Conditions, condition)
}

// sets the Ready condition based on the other conditions
func calculateIPPolicyReadyCondition(ipPolicy *ingressv1alpha1.IPPolicy) {
	// check IP Policy created condition
	ipPolicyCreated := false
	createdCondition := meta.FindStatusCondition(ipPolicy.Status.Conditions, string(ingressv1alpha1.IPPolicyConditionCreated))
	if createdCondition != nil && createdCondition.Status == metav1.ConditionTrue {
		ipPolicyCreated = true
	}

	// check IP Policy rules configured condition
	ipPolicyRulesConfigured := false
	rulesConfiguredCondition := meta.FindStatusCondition(ipPolicy.Status.Conditions, string(ingressv1alpha1.IPPolicyConditionRulesConfigured))
	if rulesConfiguredCondition != nil && rulesConfiguredCondition.Status == metav1.ConditionTrue {
		ipPolicyRulesConfigured = true
	}

	switch {
	case ipPolicyCreated && ipPolicyRulesConfigured:
		setIPPolicyReadyCondition(ipPolicy, true, ingressv1alpha1.IPPolicyReasonActive, "IP Policy is active")
	case ipPolicyCreated && !ipPolicyRulesConfigured:
		setIPPolicyReadyCondition(ipPolicy, false, ingressv1alpha1.IPPolicyReasonRulesConfigurationError, "IP Policy rules are not configured")
	default:
		setIPPolicyReadyCondition(ipPolicy, false, ingressv1alpha1.IPPolicyReasonCreationFailed, "IP Policy is not ready")
	}

}
