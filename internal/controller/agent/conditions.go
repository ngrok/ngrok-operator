package agent

import (
	ngrokv1alpha1 "github.com/ngrok/ngrok-operator/api/ngrok/v1alpha1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// Standard condition types for AgentEndpoint
const (
	ConditionReady           = "Ready"
	ConditionEndpointCreated = "EndpointCreated"
	ConditionTrafficPolicy   = "TrafficPolicyApplied"
	ConditionDomainReady     = "DomainReady"
)

// Standard condition reasons
const (
	ReasonEndpointActive     = "EndpointActive"
	ReasonTrafficPolicyError = "TrafficPolicyError"
	ReasonNgrokAPIError      = "NgrokAPIError"
	ReasonDomainCreating     = "DomainCreating"
	ReasonUpstreamError      = "UpstreamError"
	ReasonEndpointCreated    = "EndpointCreated"
	ReasonConfigError        = "ConfigurationError"
	ReasonReconciling        = "Reconciling"
)

// setReadyCondition sets the Ready condition based on the overall endpoint state
func setReadyCondition(endpoint *ngrokv1alpha1.AgentEndpoint, ready bool, reason, message string) {
	status := metav1.ConditionTrue
	if !ready {
		status = metav1.ConditionFalse
	}

	condition := metav1.Condition{
		Type:               ConditionReady,
		Status:             status,
		Reason:             reason,
		Message:            message,
		ObservedGeneration: endpoint.Generation,
	}

	meta.SetStatusCondition(&endpoint.Status.Conditions, condition)
}

// setEndpointCreatedCondition sets the EndpointCreated condition
func setEndpointCreatedCondition(endpoint *ngrokv1alpha1.AgentEndpoint, created bool, reason, message string) {
	status := metav1.ConditionTrue
	if !created {
		status = metav1.ConditionFalse
	}

	condition := metav1.Condition{
		Type:               ConditionEndpointCreated,
		Status:             status,
		Reason:             reason,
		Message:            message,
		ObservedGeneration: endpoint.Generation,
	}

	meta.SetStatusCondition(&endpoint.Status.Conditions, condition)
}

// setTrafficPolicyCondition sets the TrafficPolicyApplied condition
func setTrafficPolicyCondition(endpoint *ngrokv1alpha1.AgentEndpoint, applied bool, reason, message string) {
	status := metav1.ConditionTrue
	if !applied {
		status = metav1.ConditionFalse
	}

	condition := metav1.Condition{
		Type:               ConditionTrafficPolicy,
		Status:             status,
		Reason:             reason,
		Message:            message,
		ObservedGeneration: endpoint.Generation,
	}

	meta.SetStatusCondition(&endpoint.Status.Conditions, condition)
}

// setDomainReadyCondition sets the DomainReady condition
func setDomainReadyCondition(endpoint *ngrokv1alpha1.AgentEndpoint, ready bool, reason, message string) {
	status := metav1.ConditionTrue
	if !ready {
		status = metav1.ConditionFalse
	}

	condition := metav1.Condition{
		Type:               ConditionDomainReady,
		Status:             status,
		Reason:             reason,
		Message:            message,
		ObservedGeneration: endpoint.Generation,
	}

	meta.SetStatusCondition(&endpoint.Status.Conditions, condition)
}

// setReconcilingCondition sets a temporary reconciling condition
func setReconcilingCondition(endpoint *ngrokv1alpha1.AgentEndpoint, message string) {
	setReadyCondition(endpoint, false, ReasonReconciling, message)
}
