package ngrok

import (
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	ngrokv1alpha1 "github.com/ngrok/ngrok-operator/api/ngrok/v1alpha1"
	domainpkg "github.com/ngrok/ngrok-operator/internal/domain"
)

const (
	// condition types for CloudEndpoint
	ConditionCloudEndpointReady   = "Ready"
	ConditionCloudEndpointCreated = "CloudEndpointCreated"

	// condition reasons for CloudEndpoint
	ReasonCloudEndpointActive         = "CloudEndpointActive"
	ReasonCloudEndpointCreated        = "CloudEndpointCreated"
	ReasonCloudEndpointCreationFailed = "CloudEndpointCreationFailed"
)

// setCloudEndpointReadyCondition sets the Ready condition
func setCloudEndpointReadyCondition(clep *ngrokv1alpha1.CloudEndpoint, ready bool, reason, message string) {
	status := metav1.ConditionTrue
	if !ready {
		status = metav1.ConditionFalse
	}

	condition := metav1.Condition{
		Type:               ConditionCloudEndpointReady,
		Status:             status,
		Reason:             reason,
		Message:            message,
		ObservedGeneration: clep.Generation,
	}

	meta.SetStatusCondition(&clep.Status.Conditions, condition)
}

// setCloudEndpointCreatedCondition sets the CloudEndpointCreated condition
func setCloudEndpointCreatedCondition(clep *ngrokv1alpha1.CloudEndpoint, created bool, reason, message string) {
	status := metav1.ConditionTrue
	if !created {
		status = metav1.ConditionFalse
	}

	condition := metav1.Condition{
		Type:               ConditionCloudEndpointCreated,
		Status:             status,
		Reason:             reason,
		Message:            message,
		ObservedGeneration: clep.Generation,
	}

	meta.SetStatusCondition(&clep.Status.Conditions, condition)
}

// calculateCloudEndpointReadyCondition calculates the overall Ready condition based on other conditions and domain status
func calculateCloudEndpointReadyCondition(clep *ngrokv1alpha1.CloudEndpoint, domainResult *domainpkg.DomainResult) {
	// Check CloudEndpoint created condition
	cloudEndpointCreated := false
	createdCondition := meta.FindStatusCondition(clep.Status.Conditions, ConditionCloudEndpointCreated)
	if createdCondition != nil && createdCondition.Status == metav1.ConditionTrue {
		cloudEndpointCreated = true
	}

	// Check if domain is ready
	domainReady := domainResult.IsReady

	// Overall ready status
	ready := cloudEndpointCreated && domainReady

	// Determine reason and message based on state
	var reason, message string
	switch {
	case ready:
		reason = ReasonCloudEndpointActive
		message = "Cloud endpoint is active and ready"
	case !cloudEndpointCreated:
		// If CloudEndpointCreated condition exists and is False, use its reason/message
		if createdCondition != nil && createdCondition.Status == metav1.ConditionFalse {
			reason = createdCondition.Reason
			message = createdCondition.Message
		} else {
			reason = "Pending"
			message = "Waiting for domain to be ready"
		}
	case !domainReady:
		// Use the domain's Ready condition reason/message for better context
		if domainResult.ReadyReason != "" {
			reason = domainResult.ReadyReason
			message = domainResult.ReadyMessage
		} else {
			reason = "DomainNotReady"
			message = "Domain is not ready"
		}
	default:
		reason = "Unknown"
		message = "Cloud endpoint is not ready"
	}

	setCloudEndpointReadyCondition(clep, ready, reason, message)
}
