package agent

import (
	"github.com/ngrok/ngrok-operator/internal/controller/conditions"
	domainpkg "github.com/ngrok/ngrok-operator/internal/domain"
	trafficpolicypkg "github.com/ngrok/ngrok-operator/internal/trafficpolicy"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// Standard condition types for AgentEndpoint
const (
	ConditionReady           = "Ready"
	ConditionEndpointCreated = "EndpointCreated"
	// ConditionTrafficPolicy is sourced from the shared trafficpolicy package
	// so both endpoint controllers report the same condition type.
	ConditionTrafficPolicy = trafficpolicypkg.ConditionTrafficPolicy
)

// Standard condition reasons
const (
	ReasonEndpointActive     = "EndpointActive"
	ReasonTrafficPolicyError = trafficpolicypkg.ReasonTrafficPolicyError
	ReasonNgrokAPIError      = "NgrokAPIError"
	ReasonUpstreamError      = "UpstreamError"
	ReasonEndpointCreated    = "EndpointCreated"
	ReasonConfigError        = "ConfigurationError"
	ReasonDomainNotReady     = "DomainNotReady"
	ReasonPending            = "Pending"
	ReasonUnknown            = "Unknown"
)

// setReadyCondition sets the Ready condition based on the overall endpoint state
func setReadyCondition(endpoint agentEndpoint, ready bool, reason, message string) {
	conditions.Set(endpoint.GetConditions(), endpoint.GetGeneration(), ConditionReady, ready, reason, message)
}

// setEndpointCreatedCondition sets the EndpointCreated condition
func setEndpointCreatedCondition(endpoint agentEndpoint, created bool, reason, message string) {
	conditions.Set(endpoint.GetConditions(), endpoint.GetGeneration(), ConditionEndpointCreated, created, reason, message)
}

// calculateAgentEndpointReadyCondition calculates the overall Ready condition based on other conditions and domain status
func calculateAgentEndpointReadyCondition(aep agentEndpoint, domainResult *domainpkg.DomainResult) {
	// Check all required conditions
	endpointCreatedCondition := meta.FindStatusCondition(*aep.GetConditions(), ConditionEndpointCreated)
	endpointCreated := endpointCreatedCondition != nil && endpointCreatedCondition.Status == metav1.ConditionTrue

	trafficPolicyCondition := meta.FindStatusCondition(*aep.GetConditions(), ConditionTrafficPolicy)
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
			reason = ReasonDomainNotReady
			message = "Domain is not ready"
		}
	case !trafficPolicyReady:
		// A failed traffic-policy resolution (e.g. TrafficPolicyNotFound) is the
		// actionable root cause and is surfaced before the generic "not yet
		// created" pending state, since policy resolution gates creation.
		reason = trafficPolicyCondition.Reason
		message = trafficPolicyCondition.Message
	case !endpointCreated:
		// If EndpointCreated condition exists and is False, use its reason/message
		if endpointCreatedCondition != nil && endpointCreatedCondition.Status == metav1.ConditionFalse {
			reason = endpointCreatedCondition.Reason
			message = endpointCreatedCondition.Message
		} else {
			reason = ReasonPending
			message = "Waiting for endpoint creation"
		}
	default:
		reason = ReasonUnknown
		message = "AgentEndpoint is not ready"
	}

	setReadyCondition(aep, ready, reason, message)
}
