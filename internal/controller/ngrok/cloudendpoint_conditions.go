package ngrok

import (
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	ngrokv1alpha1 "github.com/ngrok/ngrok-operator/api/ngrok/v1alpha1"
	domainpkg "github.com/ngrok/ngrok-operator/internal/domain"
)

// setCloudEndpointReadyCondition sets the Ready condition
func setCloudEndpointReadyCondition(clep *ngrokv1alpha1.CloudEndpoint, ready bool, reason ngrokv1alpha1.CloudEndpointConditionReadyReason, message string) {
	status := metav1.ConditionTrue
	if !ready {
		status = metav1.ConditionFalse
	}

	condition := metav1.Condition{
		Type:               string(ngrokv1alpha1.CloudEndpointConditionReady),
		Status:             status,
		Reason:             string(reason),
		Message:            message,
		ObservedGeneration: clep.Generation,
	}

	meta.SetStatusCondition(&clep.Status.Conditions, condition)
}

// setCloudEndpointCreatedCondition sets the CloudEndpointCreated condition
func setCloudEndpointCreatedCondition(clep *ngrokv1alpha1.CloudEndpoint, created bool, reason ngrokv1alpha1.CloudEndpointConditionCreatedReason, message string) {
	status := metav1.ConditionTrue
	if !created {
		status = metav1.ConditionFalse
	}

	condition := metav1.Condition{
		Type:               string(ngrokv1alpha1.CloudEndpointConditionCreated),
		Status:             status,
		Reason:             string(reason),
		Message:            message,
		ObservedGeneration: clep.Generation,
	}

	meta.SetStatusCondition(&clep.Status.Conditions, condition)
}

// calculateCloudEndpointReadyCondition calculates the overall Ready condition based on other conditions and domain status
func calculateCloudEndpointReadyCondition(clep *ngrokv1alpha1.CloudEndpoint, domainResult *domainpkg.DomainResult) {
	// Check CloudEndpoint created condition
	cloudEndpointCreated := false
	createdCondition := meta.FindStatusCondition(clep.Status.Conditions, string(ngrokv1alpha1.CloudEndpointConditionCreated))
	if createdCondition != nil && createdCondition.Status == metav1.ConditionTrue {
		cloudEndpointCreated = true
	}

	// Check if domain is ready
	domainReady := domainResult.IsReady

	// Overall ready status
	ready := cloudEndpointCreated && domainReady

	// Determine reason and message based on state
	var reason ngrokv1alpha1.CloudEndpointConditionReadyReason
	var message string
	switch {
	case ready:
		reason = ngrokv1alpha1.CloudEndpointReasonActive
		message = "CloudEndpoint is active and ready"
	case !domainReady:
		// Use the domain's Ready condition reason/message for better context
		if domainResult.ReadyReason != "" {
			reason = ngrokv1alpha1.CloudEndpointConditionReadyReason(domainResult.ReadyReason)
			message = domainResult.ReadyMessage
		} else {
			reason = ngrokv1alpha1.CloudEndpointReasonDomainNotReady
			message = "Domain is not ready"
		}
	case !cloudEndpointCreated:
		// If CloudEndpointCreated condition exists and is False, use its reason/message
		if createdCondition != nil && createdCondition.Status == metav1.ConditionFalse {
			reason = ngrokv1alpha1.CloudEndpointConditionReadyReason(createdCondition.Reason)
			message = createdCondition.Message
		} else {
			reason = ngrokv1alpha1.CloudEndpointReasonPending
			message = "Waiting for CloudEndpoint to be ready"
		}
	default:
		reason = ngrokv1alpha1.CloudEndpointReasonUnknown
		message = "CloudEndpoint is not ready"
	}

	setCloudEndpointReadyCondition(clep, ready, reason, message)
}
