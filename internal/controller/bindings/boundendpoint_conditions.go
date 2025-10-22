package bindings

import (
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	bindingsv1alpha1 "github.com/ngrok/ngrok-operator/api/bindings/v1alpha1"
	"github.com/ngrok/ngrok-operator/internal/ngrokapi"
)

// setServicesCreatedCondition sets the ServicesCreated condition
func setServicesCreatedCondition(be *bindingsv1alpha1.BoundEndpoint, created bool, reason bindingsv1alpha1.BoundEndpointConditionServicesCreatedReason, message string) {
	status := metav1.ConditionTrue
	if !created {
		status = metav1.ConditionFalse
	}

	condition := metav1.Condition{
		Type:               string(bindingsv1alpha1.BoundEndpointConditionServicesCreated),
		Status:             status,
		Reason:             string(reason),
		Message:            message,
		ObservedGeneration: be.Generation,
	}

	meta.SetStatusCondition(&be.Status.Conditions, condition)
}

// setConnectivityVerifiedCondition sets the ConnectivityVerified condition
func setConnectivityVerifiedCondition(be *bindingsv1alpha1.BoundEndpoint, verified bool, err error) {
	status := metav1.ConditionTrue
	reason := bindingsv1alpha1.BoundEndpointReasonConnectivityVerified
	message := "Successfully connected to upstream service"

	if !verified {
		status = metav1.ConditionFalse
		reason = bindingsv1alpha1.BoundEndpointReasonConnectivityFailed
		message = ngrokapi.SanitizeErrorMessage(err.Error())
	}

	condition := metav1.Condition{
		Type:               string(bindingsv1alpha1.BoundEndpointConditionConnectivityVerified),
		Status:             status,
		Reason:             string(reason),
		Message:            message,
		ObservedGeneration: be.Generation,
	}

	meta.SetStatusCondition(&be.Status.Conditions, condition)
}

// calculateReadyCondition calculates the overall Ready condition based on other conditions
func calculateReadyCondition(be *bindingsv1alpha1.BoundEndpoint) {
	// Check if services were created
	servicesCreatedCondition := meta.FindStatusCondition(be.Status.Conditions, string(bindingsv1alpha1.BoundEndpointConditionServicesCreated))
	servicesCreated := servicesCreatedCondition != nil && servicesCreatedCondition.Status == metav1.ConditionTrue

	// Check if connectivity was verified
	connectivityCondition := meta.FindStatusCondition(be.Status.Conditions, string(bindingsv1alpha1.BoundEndpointConditionConnectivityVerified))
	connectivityVerified := connectivityCondition != nil && connectivityCondition.Status == metav1.ConditionTrue

	// Overall ready status
	ready := servicesCreated && connectivityVerified

	// Determine reason and message based on state
	var reason bindingsv1alpha1.BoundEndpointConditionReadyReason
	var message string
	switch {
	case ready:
		reason = bindingsv1alpha1.BoundEndpointReasonReady
		message = "BoundEndpoint is ready"
	case !servicesCreated:
		if servicesCreatedCondition != nil {
			reason = bindingsv1alpha1.BoundEndpointConditionReadyReason(servicesCreatedCondition.Reason)
			message = servicesCreatedCondition.Message
		} else {
			reason = bindingsv1alpha1.BoundEndpointReasonServicesNotCreated
			message = "Services not yet created"
		}
	case !connectivityVerified:
		if connectivityCondition != nil {
			reason = bindingsv1alpha1.BoundEndpointConditionReadyReason(connectivityCondition.Reason)
			message = connectivityCondition.Message
		} else {
			reason = bindingsv1alpha1.BoundEndpointReasonConnectivityNotVerified
			message = "Connectivity not yet verified"
		}
	default:
		reason = "Unknown"
		message = "BoundEndpoint is not ready"
	}

	setReadyCondition(be, ready, reason, message)
}

// setReadyCondition sets the Ready condition
func setReadyCondition(be *bindingsv1alpha1.BoundEndpoint, ready bool, reason bindingsv1alpha1.BoundEndpointConditionReadyReason, message string) {
	status := metav1.ConditionTrue
	if !ready {
		status = metav1.ConditionFalse
	}

	condition := metav1.Condition{
		Type:               string(bindingsv1alpha1.BoundEndpointConditionReady),
		Status:             status,
		Reason:             string(reason),
		Message:            message,
		ObservedGeneration: be.Generation,
	}

	meta.SetStatusCondition(&be.Status.Conditions, condition)
}
