package ngrok

import (
	ngrokv1alpha1 "github.com/ngrok/ngrok-operator/api/ngrok/v1alpha1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// Standard condition types for CloudEndpoint
const (
	ConditionReady           = "Ready"
	ConditionEndpointCreated = "EndpointCreated" 
	ConditionTrafficPolicy   = "TrafficPolicyApplied"
	ConditionDomainReady     = "DomainReady"
)

// Standard condition reasons
const (
	ReasonEndpointActive         = "EndpointActive"
	ReasonTrafficPolicyError     = "TrafficPolicyError"
	ReasonNgrokAPIError          = "NgrokAPIError"
	ReasonDomainCreating         = "DomainCreating"
	ReasonEndpointCreated        = "EndpointCreated"
	ReasonConfigError            = "ConfigurationError"
	ReasonReconciling            = "Reconciling"
	ReasonTrafficPolicyApplied   = "TrafficPolicyApplied"
	ReasonReconciliationComplete = "ReconciliationComplete"
)

// setReadyCondition sets the Ready condition based on the overall endpoint state
func setReadyCondition(endpoint *ngrokv1alpha1.CloudEndpoint, ready bool, reason, message string) {
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
func setEndpointCreatedCondition(endpoint *ngrokv1alpha1.CloudEndpoint, created bool, reason, message string) {
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
func setTrafficPolicyCondition(endpoint *ngrokv1alpha1.CloudEndpoint, applied bool, reason, message string) {
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
func setDomainReadyCondition(endpoint *ngrokv1alpha1.CloudEndpoint, ready bool, reason, message string) {
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

// Composite helpers that set multiple related conditions (following domain controller pattern)

// setReconciling sets the resource to reconciling state
func setReconciling(endpoint *ngrokv1alpha1.CloudEndpoint, message string) {
	setReadyCondition(endpoint, false, ReasonReconciling, message)
}

// setDomainWaiting sets domain-related conditions when waiting for domain to be ready
func setDomainWaiting(endpoint *ngrokv1alpha1.CloudEndpoint, message string) {
	setDomainReadyCondition(endpoint, false, ReasonDomainCreating, message)
	setReadyCondition(endpoint, false, ReasonDomainCreating, "Waiting for domain to be ready")
}

// setDomainReady sets domain-related conditions when domain is ready
func setDomainReady(endpoint *ngrokv1alpha1.CloudEndpoint, message string) {
	setDomainReadyCondition(endpoint, true, ReasonEndpointActive, message)
}

// setDomainError sets domain-related conditions when there's a domain error
func setDomainError(endpoint *ngrokv1alpha1.CloudEndpoint, message string) {
	setDomainReadyCondition(endpoint, false, ReasonNgrokAPIError, message)
	setReadyCondition(endpoint, false, ReasonNgrokAPIError, message)
}

// setTrafficPolicyError sets traffic policy conditions when there's a policy error
func setTrafficPolicyError(endpoint *ngrokv1alpha1.CloudEndpoint, message string) {
	setTrafficPolicyCondition(endpoint, false, ReasonTrafficPolicyError, message)
	setReadyCondition(endpoint, false, ReasonTrafficPolicyError, message)
}

// setEndpointSuccess sets all conditions for a successful endpoint creation
func setEndpointSuccess(endpoint *ngrokv1alpha1.CloudEndpoint, message string, hasTrafficPolicy bool) {
	setEndpointCreatedCondition(endpoint, true, ReasonEndpointCreated, "Endpoint successfully created")
	setReadyCondition(endpoint, true, ReasonEndpointActive, message)
	
	if hasTrafficPolicy {
		setTrafficPolicyCondition(endpoint, true, ReasonTrafficPolicyApplied, "Traffic policy successfully applied")
	}
}

// setEndpointCreateFailed sets endpoint conditions when endpoint creation fails
func setEndpointCreateFailed(endpoint *ngrokv1alpha1.CloudEndpoint, reason, message string) {
	setEndpointCreatedCondition(endpoint, false, reason, message)
	setReadyCondition(endpoint, false, reason, message)
}
