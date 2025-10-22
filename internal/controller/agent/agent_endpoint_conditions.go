package agent

import (
	ngrokv1alpha1 "github.com/ngrok/ngrok-operator/api/ngrok/v1alpha1"
	domainpkg "github.com/ngrok/ngrok-operator/internal/domain"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// setReadyCondition sets the Ready condition based on the overall endpoint state
func setReadyCondition(endpoint *ngrokv1alpha1.AgentEndpoint, ready bool, reason ngrokv1alpha1.AgentEndpointConditionReadyReason, message string) {
	status := metav1.ConditionTrue
	if !ready {
		status = metav1.ConditionFalse
	}

	condition := metav1.Condition{
		Type:               string(ngrokv1alpha1.AgentEndpointConditionReady),
		Status:             status,
		Reason:             string(reason),
		Message:            message,
		ObservedGeneration: endpoint.Generation,
	}

	meta.SetStatusCondition(&endpoint.Status.Conditions, condition)
}

// setEndpointCreatedCondition sets the EndpointCreated condition
func setEndpointCreatedCondition(endpoint *ngrokv1alpha1.AgentEndpoint, created bool, reason ngrokv1alpha1.AgentEndpointConditionEndpointCreatedReason, message string) {
	status := metav1.ConditionTrue
	if !created {
		status = metav1.ConditionFalse
	}

	condition := metav1.Condition{
		Type:               string(ngrokv1alpha1.AgentEndpointConditionEndpointCreated),
		Status:             status,
		Reason:             string(reason),
		Message:            message,
		ObservedGeneration: endpoint.Generation,
	}

	meta.SetStatusCondition(&endpoint.Status.Conditions, condition)
}

// setTrafficPolicyCondition sets the TrafficPolicyApplied condition
func setTrafficPolicyCondition(endpoint *ngrokv1alpha1.AgentEndpoint, applied bool, reason ngrokv1alpha1.AgentEndpointConditionTrafficPolicyReason, message string) {
	status := metav1.ConditionTrue
	if !applied {
		status = metav1.ConditionFalse
	}

	condition := metav1.Condition{
		Type:               string(ngrokv1alpha1.AgentEndpointConditionTrafficPolicy),
		Status:             status,
		Reason:             string(reason),
		Message:            message,
		ObservedGeneration: endpoint.Generation,
	}

	meta.SetStatusCondition(&endpoint.Status.Conditions, condition)
}

// setReconcilingCondition sets a temporary reconciling condition
func setReconcilingCondition(endpoint *ngrokv1alpha1.AgentEndpoint, message string) {
	setReadyCondition(endpoint, false, ngrokv1alpha1.AgentEndpointReasonReconciling, message)
}

// calculateAgentEndpointReadyCondition calculates the overall Ready condition based on other conditions and domain status
func calculateAgentEndpointReadyCondition(aep *ngrokv1alpha1.AgentEndpoint, domainResult *domainpkg.DomainResult) {
	// Check all required conditions
	endpointCreatedCondition := meta.FindStatusCondition(aep.Status.Conditions, string(ngrokv1alpha1.AgentEndpointConditionEndpointCreated))
	endpointCreated := endpointCreatedCondition != nil && endpointCreatedCondition.Status == metav1.ConditionTrue

	trafficPolicyCondition := meta.FindStatusCondition(aep.Status.Conditions, string(ngrokv1alpha1.AgentEndpointConditionTrafficPolicy))
	trafficPolicyReady := true
	// If traffic policy condition exists and is False, it's not ready
	if trafficPolicyCondition != nil && trafficPolicyCondition.Status == metav1.ConditionFalse {
		trafficPolicyReady = false
	}

	// Check if domain is ready
	domainReady := domainResult.IsReady

	// Overall ready status - all conditions must be true
	ready := endpointCreated && trafficPolicyReady && domainReady

	// Determine reason and message based on state
	var reason ngrokv1alpha1.AgentEndpointConditionReadyReason
	var message string
	switch {
	case ready:
		reason = ngrokv1alpha1.AgentEndpointReasonActive
		message = "AgentEndpoint is active and ready"
	case !domainReady:
		// Use the domain's Ready condition reason/message for better context
		if domainResult.ReadyReason != "" {
			reason = ngrokv1alpha1.AgentEndpointConditionReadyReason(domainResult.ReadyReason)
			message = domainResult.ReadyMessage
		} else {
			reason = ngrokv1alpha1.AgentEndpointReasonDomainNotReady
			message = "Domain is not ready"
		}
	case !endpointCreated:
		// If EndpointCreated condition exists and is False, use its reason/message
		if endpointCreatedCondition != nil && endpointCreatedCondition.Status == metav1.ConditionFalse {
			reason = ngrokv1alpha1.AgentEndpointConditionReadyReason(endpointCreatedCondition.Reason)
			message = endpointCreatedCondition.Message
		} else {
			reason = ngrokv1alpha1.AgentEndpointReasonPending
			message = "Waiting for endpoint creation"
		}
	case !trafficPolicyReady:
		// Use the traffic policy's condition reason/message
		reason = ngrokv1alpha1.AgentEndpointConditionReadyReason(trafficPolicyCondition.Reason)
		message = trafficPolicyCondition.Message
	default:
		reason = ngrokv1alpha1.AgentEndpointReasonUnknown
		message = "AgentEndpoint is not ready"
	}

	setReadyCondition(aep, ready, reason, message)
}
