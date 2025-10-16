package bindings

import (
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	bindingsv1alpha1 "github.com/ngrok/ngrok-operator/api/bindings/v1alpha1"
	"github.com/ngrok/ngrok-operator/internal/ngrokapi"
)

const (
	// Condition types for BoundEndpoint
	ConditionTypeReady                   = "Ready"
	ConditionTypeServicesCreated         = "ServicesCreated"
	ConditionTypeConnectivityVerified    = "ConnectivityVerified"
)

const (
	// Reasons for Ready condition
	ReasonBoundEndpointReady          = "BoundEndpointReady"
	ReasonServicesNotCreated          = "ServicesNotCreated"
	ReasonConnectivityNotVerified     = "ConnectivityNotVerified"

	// Reasons for ServicesCreated condition
	ReasonServicesCreated             = "ServicesCreated"
	ReasonServiceCreationFailed       = "ServiceCreationFailed"

	// Reasons for ConnectivityVerified condition
	ReasonConnectivityVerified        = "ConnectivityVerified"
	ReasonConnectivityFailed          = "ConnectivityFailed"
)

// setServicesCreatedCondition sets the ServicesCreated condition
func setServicesCreatedCondition(be *bindingsv1alpha1.BoundEndpoint, created bool, reason, message string) {
	status := metav1.ConditionTrue
	if !created {
		status = metav1.ConditionFalse
	}

	condition := metav1.Condition{
		Type:               ConditionTypeServicesCreated,
		Status:             status,
		Reason:             reason,
		Message:            message,
		ObservedGeneration: be.Generation,
	}

	meta.SetStatusCondition(&be.Status.Conditions, condition)
}

// setConnectivityVerifiedCondition sets the ConnectivityVerified condition
func setConnectivityVerifiedCondition(be *bindingsv1alpha1.BoundEndpoint, verified bool, err error) {
	status := metav1.ConditionTrue
	reason := ReasonConnectivityVerified
	message := "Successfully connected to upstream service"

	if !verified {
		status = metav1.ConditionFalse
		reason = ReasonConnectivityFailed
		message = ngrokapi.SanitizeErrorMessage(err.Error())
	}

	condition := metav1.Condition{
		Type:               ConditionTypeConnectivityVerified,
		Status:             status,
		Reason:             reason,
		Message:            message,
		ObservedGeneration: be.Generation,
	}

	meta.SetStatusCondition(&be.Status.Conditions, condition)
}

// calculateReadyCondition calculates the overall Ready condition based on other conditions
func calculateReadyCondition(be *bindingsv1alpha1.BoundEndpoint) {
	// Check if services were created
	servicesCreatedCondition := meta.FindStatusCondition(be.Status.Conditions, ConditionTypeServicesCreated)
	servicesCreated := servicesCreatedCondition != nil && servicesCreatedCondition.Status == metav1.ConditionTrue

	// Check if connectivity was verified
	connectivityCondition := meta.FindStatusCondition(be.Status.Conditions, ConditionTypeConnectivityVerified)
	connectivityVerified := connectivityCondition != nil && connectivityCondition.Status == metav1.ConditionTrue

	// Overall ready status
	ready := servicesCreated && connectivityVerified

	// Determine reason and message based on state
	var reason, message string
	switch {
	case ready:
		reason = ReasonBoundEndpointReady
		message = "BoundEndpoint is ready"
	case !servicesCreated:
		if servicesCreatedCondition != nil {
			reason = servicesCreatedCondition.Reason
			message = servicesCreatedCondition.Message
		} else {
			reason = ReasonServicesNotCreated
			message = "Services not yet created"
		}
	case !connectivityVerified:
		if connectivityCondition != nil {
			reason = connectivityCondition.Reason
			message = connectivityCondition.Message
		} else {
			reason = ReasonConnectivityNotVerified
			message = "Connectivity not yet verified"
		}
	default:
		reason = "Unknown"
		message = "BoundEndpoint is not ready"
	}

	setReadyCondition(be, ready, reason, message)
}

// setReadyCondition sets the Ready condition
func setReadyCondition(be *bindingsv1alpha1.BoundEndpoint, ready bool, reason, message string) {
	status := metav1.ConditionTrue
	if !ready {
		status = metav1.ConditionFalse
	}

	condition := metav1.Condition{
		Type:               ConditionTypeReady,
		Status:             status,
		Reason:             reason,
		Message:            message,
		ObservedGeneration: be.Generation,
	}

	meta.SetStatusCondition(&be.Status.Conditions, condition)
}
