package agent

import (
	ngrokv1alpha1 "github.com/ngrok/ngrok-operator/api/ngrok/v1alpha1"
	domainpkg "github.com/ngrok/ngrok-operator/internal/domain"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// Standard condition types for AgentEndpoint
const (
	ConditionReady           = "Ready"
	ConditionEndpointCreated = "EndpointCreated"
	ConditionTrafficPolicy   = "TrafficPolicyApplied"
)

// Standard condition reasons
const (
	ReasonEndpointActive     = "EndpointActive"
	ReasonTrafficPolicyError = "TrafficPolicyError"
	ReasonNgrokAPIError      = "NgrokAPIError"
	ReasonUpstreamError      = "UpstreamError"
	ReasonEndpointCreated    = "EndpointCreated"
	ReasonConfigError        = "ConfigurationError"
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

// calculateAgentEndpointReadyCondition calculates the overall Ready condition based on other conditions and domain status
func calculateAgentEndpointReadyCondition(aep *ngrokv1alpha1.AgentEndpoint, domainResult *domainpkg.DomainResult) {
	// Check all required conditions
	endpointCreatedCondition := meta.FindStatusCondition(aep.Status.Conditions, ConditionEndpointCreated)
	endpointCreated := endpointCreatedCondition != nil && endpointCreatedCondition.Status == metav1.ConditionTrue

	trafficPolicyCondition := meta.FindStatusCondition(aep.Status.Conditions, ConditionTrafficPolicy)
	trafficPolicyReady := true
	// If traffic policy condition exists and is False, it's not ready
	if trafficPolicyCondition != nil && trafficPolicyCondition.Status == metav1.ConditionFalse {
		trafficPolicyReady = false
	}

	// Check if domain is ready (default to false for safety)
	domainReady := false
	if domainResult != nil {
		domainReady = domainResult.IsReady
	}

	// Overall ready status - all conditions must be true
	ready := endpointCreated && trafficPolicyReady && domainReady

	// Determine reason and message based on state
	var reason, message string
	switch {
	case ready:
		reason = ReasonEndpointActive
		message = "AgentEndpoint is active and ready"
	case !domainReady:
		// Use the domain's Ready condition reason/message for better context
		if domainResult != nil && domainResult.ReadyReason != "" {
			reason = domainResult.ReadyReason
			message = domainResult.ReadyMessage
		} else {
			reason = "DomainNotReady"
			message = "Domain is not ready"
		}
	case !endpointCreated:
		// If EndpointCreated condition exists and is False, use its reason/message
		if endpointCreatedCondition != nil && endpointCreatedCondition.Status == metav1.ConditionFalse {
			reason = endpointCreatedCondition.Reason
			message = endpointCreatedCondition.Message
		} else {
			reason = "Pending"
			message = "Waiting for endpoint creation"
		}
	case !trafficPolicyReady:
		// Use the traffic policy's condition reason/message
		reason = trafficPolicyCondition.Reason
		message = trafficPolicyCondition.Message
	default:
		reason = "Unknown"
		message = "AgentEndpoint is not ready"
	}

	setReadyCondition(aep, ready, reason, message)
}
