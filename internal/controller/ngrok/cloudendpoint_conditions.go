package ngrok

import (
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	ngrokv1alpha1 "github.com/ngrok/ngrok-operator/api/ngrok/v1alpha1"
	"github.com/ngrok/ngrok-operator/internal/controller/conditions"
	domainpkg "github.com/ngrok/ngrok-operator/internal/domain"
	trafficpolicypkg "github.com/ngrok/ngrok-operator/internal/trafficpolicy"
)

const (
	// condition types for CloudEndpoint
	ConditionCloudEndpointReady   = "Ready"
	ConditionCloudEndpointCreated = "CloudEndpointCreated"
	// ConditionTrafficPolicy is sourced from the shared trafficpolicy package
	// so both endpoint controllers report the same condition type.
	ConditionTrafficPolicy = trafficpolicypkg.ConditionTrafficPolicy

	// condition reasons for CloudEndpoint
	ReasonCloudEndpointActive         = "CloudEndpointActive"
	ReasonCloudEndpointCreated        = "CloudEndpointCreated"
	ReasonCloudEndpointCreationFailed = "CloudEndpointCreationFailed"
)

// setCloudEndpointReadyCondition sets the Ready condition
func setCloudEndpointReadyCondition(clep *ngrokv1alpha1.CloudEndpoint, ready bool, reason, message string) {
	conditions.Set(&clep.Status.Conditions, clep.Generation, ConditionCloudEndpointReady, ready, reason, message)
}

// setCloudEndpointCreatedCondition sets the CloudEndpointCreated condition
func setCloudEndpointCreatedCondition(clep *ngrokv1alpha1.CloudEndpoint, created bool, reason, message string) {
	conditions.Set(&clep.Status.Conditions, clep.Generation, ConditionCloudEndpointCreated, created, reason, message)
}

// calculateCloudEndpointReadyCondition calculates the overall Ready condition
// based on the per-sub-component conditions (CloudEndpointCreated,
// TrafficPolicyApplied) and the domain status.
func calculateCloudEndpointReadyCondition(clep *ngrokv1alpha1.CloudEndpoint, domainResult *domainpkg.DomainResult) {
	// Check CloudEndpoint created condition
	cloudEndpointCreated := false
	createdCondition := meta.FindStatusCondition(clep.Status.Conditions, ConditionCloudEndpointCreated)
	if createdCondition != nil && createdCondition.Status == metav1.ConditionTrue {
		cloudEndpointCreated = true
	}

	// TrafficPolicy condition is optional — only blocks Ready when it exists
	// and is False (matches the AgentEndpoint logic).
	trafficPolicyCondition := meta.FindStatusCondition(clep.Status.Conditions, ConditionTrafficPolicy)
	trafficPolicyReady := true
	if trafficPolicyCondition != nil && trafficPolicyCondition.Status == metav1.ConditionFalse {
		trafficPolicyReady = false
	}

	// Check if domain is ready (default to false for safety)
	domainReady := false
	if domainResult != nil {
		domainReady = domainResult.IsReady
	}

	// Overall ready status — all required sub-conditions must be true
	ready := cloudEndpointCreated && trafficPolicyReady && domainReady

	// Determine reason and message based on state
	var reason, message string
	switch {
	case ready:
		reason = ReasonCloudEndpointActive
		message = "CloudEndpoint is active and ready"
	case !domainReady:
		// Use the domain's Ready condition reason/message for better context
		if domainResult != nil && domainResult.ReadyReason != "" {
			reason = domainResult.ReadyReason
			message = domainResult.ReadyMessage
		} else {
			reason = "DomainNotReady"
			message = "Domain is not ready"
		}
	case !cloudEndpointCreated:
		// If CloudEndpointCreated condition exists and is False, use its reason/message
		if createdCondition != nil && createdCondition.Status == metav1.ConditionFalse {
			reason = createdCondition.Reason
			message = createdCondition.Message
		} else {
			reason = "Pending"
			message = "Waiting for CloudEndpoint to be ready"
		}
	case !trafficPolicyReady:
		reason = trafficPolicyCondition.Reason
		message = trafficPolicyCondition.Message
	default:
		reason = "Unknown"
		message = "CloudEndpoint is not ready"
	}

	setCloudEndpointReadyCondition(clep, ready, reason, message)
}
