package agent

import (
	ngrokv1alpha1 "github.com/ngrok/ngrok-operator/api/ngrok/v1alpha1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// Standard condition types for AgentEndpoint
const (
	ConditionReady                = "Ready"
	ConditionEndpointCreated      = "EndpointCreated"
	ConditionTrafficPolicyApplied = "TrafficPolicyApplied"
	ConditionDomainReady          = "DomainReady"
	ConditionProgressing          = "Progressing"
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
	ReasonProvisioning       = "Provisioning"
)

// setCondition is a generic helper to set any condition on an endpoint
func setCondition(endpoint *ngrokv1alpha1.AgentEndpoint, conditionType string, status metav1.ConditionStatus, reason, message string) {
	condition := metav1.Condition{
		Type:               conditionType,
		Status:             status,
		Reason:             reason,
		Message:            message,
		ObservedGeneration: endpoint.Generation,
	}
	meta.SetStatusCondition(&endpoint.Status.Conditions, condition)
}

// setProgressingCondition sets the Progressing condition
func setProgressingCondition(endpoint *ngrokv1alpha1.AgentEndpoint, progressing bool, reason, message string) {
	status := metav1.ConditionTrue
	if !progressing {
		status = metav1.ConditionFalse
	}
	setCondition(endpoint, ConditionProgressing, status, reason, message)
}

// setReadyCondition sets the Ready condition based on the overall endpoint state
func setReadyCondition(endpoint *ngrokv1alpha1.AgentEndpoint, ready bool, reason, message string) {
	status := metav1.ConditionTrue
	if !ready {
		status = metav1.ConditionFalse
	}
	setCondition(endpoint, ConditionReady, status, reason, message)
}

// setEndpointCreatedCondition sets the EndpointCreated condition
func setEndpointCreatedCondition(endpoint *ngrokv1alpha1.AgentEndpoint, created bool, reason, message string) {
	status := metav1.ConditionTrue
	if !created {
		status = metav1.ConditionFalse
	}
	setCondition(endpoint, ConditionEndpointCreated, status, reason, message)
}

// setTrafficPolicyCondition sets the TrafficPolicyApplied condition
func setTrafficPolicyCondition(endpoint *ngrokv1alpha1.AgentEndpoint, applied bool, reason, message string) {
	status := metav1.ConditionTrue
	if !applied {
		status = metav1.ConditionFalse
	}
	setCondition(endpoint, ConditionTrafficPolicyApplied, status, reason, message)
}

// setDomainReadyCondition sets the DomainReady condition
func setDomainReadyCondition(endpoint *ngrokv1alpha1.AgentEndpoint, ready bool, reason, message string) {
	status := metav1.ConditionTrue
	if !ready {
		status = metav1.ConditionFalse
	}
	setCondition(endpoint, ConditionDomainReady, status, reason, message)
}

// setReconcilingCondition sets a temporary reconciling condition
func setReconcilingCondition(endpoint *ngrokv1alpha1.AgentEndpoint, message string) {
	setReadyCondition(endpoint, false, ReasonReconciling, message)
	setProgressingCondition(endpoint, true, ReasonReconciling, message)
}
