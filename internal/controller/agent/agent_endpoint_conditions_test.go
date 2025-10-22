package agent

import (
	"testing"

	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	ngrokv1alpha1 "github.com/ngrok/ngrok-operator/api/ngrok/v1alpha1"
	"github.com/ngrok/ngrok-operator/internal/controller/conditions"
	domainpkg "github.com/ngrok/ngrok-operator/internal/domain"
)

// Helper function to create a test AgentEndpoint
func createTestAgentEndpoint(name, namespace string) *ngrokv1alpha1.AgentEndpoint {
	return &ngrokv1alpha1.AgentEndpoint{
		ObjectMeta: metav1.ObjectMeta{
			Name:       name,
			Namespace:  namespace,
			Generation: 1,
		},
		Status: ngrokv1alpha1.AgentEndpointStatus{
			Conditions: []metav1.Condition{},
		},
	}
}

// Helper function to create a test AgentEndpoint with conditions
func createTestAgentEndpointWithConditions(name, namespace string, conditions []metav1.Condition) *ngrokv1alpha1.AgentEndpoint {
	endpoint := createTestAgentEndpoint(name, namespace)
	endpoint.Status.Conditions = conditions
	return endpoint
}

// Helper function to create a ready domain result
func createReadyDomainResult() *domainpkg.DomainResult {
	return &domainpkg.DomainResult{
		IsReady:      true,
		ReadyReason:  "DomainActive",
		ReadyMessage: "Domain is ready",
	}
}

// Helper function to create a not-ready domain result
func createNotReadyDomainResult(reason, message string) *domainpkg.DomainResult {
	return &domainpkg.DomainResult{
		IsReady:      false,
		ReadyReason:  reason,
		ReadyMessage: message,
	}
}

func TestCalculateAgentEndpointReadyCondition_AllReady(t *testing.T) {
	endpoint := createTestAgentEndpointWithConditions("test-endpoint", "default", []metav1.Condition{
		{
			Type:   string(ngrokv1alpha1.AgentEndpointConditionEndpointCreated),
			Status: metav1.ConditionTrue,
			Reason: string(ngrokv1alpha1.AgentEndpointReasonEndpointCreated),
		},
		{
			Type:   string(ngrokv1alpha1.AgentEndpointConditionTrafficPolicy),
			Status: metav1.ConditionTrue,
			Reason: string(ngrokv1alpha1.AgentEndpointReasonTrafficPolicyApplied),
		},
	})
	domainResult := createReadyDomainResult()

	calculateAgentEndpointReadyCondition(endpoint, domainResult)

	readyCondition := conditions.FindCondition(endpoint.Status.Conditions, ngrokv1alpha1.AgentEndpointConditionReady)
	assert.NotNil(t, readyCondition)
	assert.Equal(t, metav1.ConditionTrue, readyCondition.Status)
	assert.Equal(t, string(ngrokv1alpha1.AgentEndpointReasonActive), readyCondition.Reason)
	assert.Equal(t, "AgentEndpoint is active and ready", readyCondition.Message)
}

func TestCalculateAgentEndpointReadyCondition_DomainNotReady(t *testing.T) {
	endpoint := createTestAgentEndpointWithConditions("test-endpoint", "default", []metav1.Condition{
		{
			Type:   string(ngrokv1alpha1.AgentEndpointConditionEndpointCreated),
			Status: metav1.ConditionTrue,
			Reason: string(ngrokv1alpha1.AgentEndpointReasonEndpointCreated),
		},
		{
			Type:   string(ngrokv1alpha1.AgentEndpointConditionTrafficPolicy),
			Status: metav1.ConditionTrue,
			Reason: string(ngrokv1alpha1.AgentEndpointReasonTrafficPolicyApplied),
		},
	})
	domainResult := createNotReadyDomainResult("ProvisioningError", "Certificate provisioning in progress")

	calculateAgentEndpointReadyCondition(endpoint, domainResult)

	readyCondition := conditions.FindCondition(endpoint.Status.Conditions, ngrokv1alpha1.AgentEndpointConditionReady)
	assert.NotNil(t, readyCondition)
	assert.Equal(t, metav1.ConditionFalse, readyCondition.Status)
	assert.Equal(t, "ProvisioningError", readyCondition.Reason)
	assert.Equal(t, "Certificate provisioning in progress", readyCondition.Message)
}

func TestCalculateAgentEndpointReadyCondition_DomainNotReadyNoReason(t *testing.T) {
	endpoint := createTestAgentEndpointWithConditions("test-endpoint", "default", []metav1.Condition{
		{
			Type:   string(ngrokv1alpha1.AgentEndpointConditionEndpointCreated),
			Status: metav1.ConditionTrue,
			Reason: string(ngrokv1alpha1.AgentEndpointReasonEndpointCreated),
		},
		{
			Type:   string(ngrokv1alpha1.AgentEndpointConditionTrafficPolicy),
			Status: metav1.ConditionTrue,
			Reason: string(ngrokv1alpha1.AgentEndpointReasonTrafficPolicyApplied),
		},
	})
	domainResult := &domainpkg.DomainResult{
		IsReady: false,
		// No ReadyReason or ReadyMessage
	}

	calculateAgentEndpointReadyCondition(endpoint, domainResult)

	readyCondition := conditions.FindCondition(endpoint.Status.Conditions, ngrokv1alpha1.AgentEndpointConditionReady)
	assert.NotNil(t, readyCondition)
	assert.Equal(t, metav1.ConditionFalse, readyCondition.Status)
	assert.Equal(t, string(ngrokv1alpha1.AgentEndpointReasonDomainNotReady), readyCondition.Reason)
	assert.Equal(t, "Domain is not ready", readyCondition.Message)
}

func TestCalculateAgentEndpointReadyCondition_EndpointNotCreated(t *testing.T) {
	endpoint := createTestAgentEndpointWithConditions("test-endpoint", "default", []metav1.Condition{
		{
			Type:    string(ngrokv1alpha1.AgentEndpointConditionEndpointCreated),
			Status:  metav1.ConditionFalse,
			Reason:  string(ngrokv1alpha1.AgentEndpointReasonNgrokAPIError),
			Message: "Failed to create endpoint",
		},
		{
			Type:   string(ngrokv1alpha1.AgentEndpointConditionTrafficPolicy),
			Status: metav1.ConditionTrue,
			Reason: string(ngrokv1alpha1.AgentEndpointReasonTrafficPolicyApplied),
		},
	})
	domainResult := createReadyDomainResult()

	calculateAgentEndpointReadyCondition(endpoint, domainResult)

	readyCondition := conditions.FindCondition(endpoint.Status.Conditions, ngrokv1alpha1.AgentEndpointConditionReady)
	assert.NotNil(t, readyCondition)
	assert.Equal(t, metav1.ConditionFalse, readyCondition.Status)
	assert.Equal(t, string(ngrokv1alpha1.AgentEndpointReasonNgrokAPIError), readyCondition.Reason)
	assert.Equal(t, "Failed to create endpoint", readyCondition.Message)
}

func TestCalculateAgentEndpointReadyCondition_EndpointNotCreatedNoCondition(t *testing.T) {
	endpoint := createTestAgentEndpointWithConditions("test-endpoint", "default", []metav1.Condition{
		{
			Type:   string(ngrokv1alpha1.AgentEndpointConditionTrafficPolicy),
			Status: metav1.ConditionTrue,
			Reason: string(ngrokv1alpha1.AgentEndpointReasonTrafficPolicyApplied),
		},
		// No EndpointCreated condition
	})
	domainResult := createReadyDomainResult()

	calculateAgentEndpointReadyCondition(endpoint, domainResult)

	readyCondition := conditions.FindCondition(endpoint.Status.Conditions, ngrokv1alpha1.AgentEndpointConditionReady)
	assert.NotNil(t, readyCondition)
	assert.Equal(t, metav1.ConditionFalse, readyCondition.Status)
	assert.Equal(t, string(ngrokv1alpha1.AgentEndpointReasonPending), readyCondition.Reason)
	assert.Equal(t, "Waiting for endpoint creation", readyCondition.Message)
}

func TestCalculateAgentEndpointReadyCondition_TrafficPolicyNotReady(t *testing.T) {
	endpoint := createTestAgentEndpointWithConditions("test-endpoint", "default", []metav1.Condition{
		{
			Type:   string(ngrokv1alpha1.AgentEndpointConditionEndpointCreated),
			Status: metav1.ConditionTrue,
			Reason: string(ngrokv1alpha1.AgentEndpointReasonEndpointCreated),
		},
		{
			Type:    string(ngrokv1alpha1.AgentEndpointConditionTrafficPolicy),
			Status:  metav1.ConditionFalse,
			Reason:  string(ngrokv1alpha1.AgentEndpointReasonTrafficPolicyError),
			Message: "Traffic policy validation failed",
		},
	})
	domainResult := createReadyDomainResult()

	calculateAgentEndpointReadyCondition(endpoint, domainResult)

	readyCondition := conditions.FindCondition(endpoint.Status.Conditions, ngrokv1alpha1.AgentEndpointConditionReady)
	assert.NotNil(t, readyCondition)
	assert.Equal(t, metav1.ConditionFalse, readyCondition.Status)
	assert.Equal(t, string(ngrokv1alpha1.AgentEndpointReasonTrafficPolicyError), readyCondition.Reason)
	assert.Equal(t, "Traffic policy validation failed", readyCondition.Message)
}

func TestCalculateAgentEndpointReadyCondition_TrafficPolicyNotSet(t *testing.T) {
	endpoint := createTestAgentEndpointWithConditions("test-endpoint", "default", []metav1.Condition{
		{
			Type:   string(ngrokv1alpha1.AgentEndpointConditionEndpointCreated),
			Status: metav1.ConditionTrue,
			Reason: string(ngrokv1alpha1.AgentEndpointReasonEndpointCreated),
		},
		// No TrafficPolicy condition - should be considered ready
	})
	domainResult := createReadyDomainResult()

	calculateAgentEndpointReadyCondition(endpoint, domainResult)

	readyCondition := conditions.FindCondition(endpoint.Status.Conditions, ngrokv1alpha1.AgentEndpointConditionReady)
	assert.NotNil(t, readyCondition)
	assert.Equal(t, metav1.ConditionTrue, readyCondition.Status)
	assert.Equal(t, string(ngrokv1alpha1.AgentEndpointReasonActive), readyCondition.Reason)
	assert.Equal(t, "AgentEndpoint is active and ready", readyCondition.Message)
}

func TestCalculateAgentEndpointReadyCondition_MultipleIssues(t *testing.T) {
	// Domain not ready should take precedence over other issues
	endpoint := createTestAgentEndpointWithConditions("test-endpoint", "default", []metav1.Condition{
		{
			Type:    string(ngrokv1alpha1.AgentEndpointConditionEndpointCreated),
			Status:  metav1.ConditionFalse,
			Reason:  string(ngrokv1alpha1.AgentEndpointReasonNgrokAPIError),
			Message: "Failed to create endpoint",
		},
		{
			Type:    string(ngrokv1alpha1.AgentEndpointConditionTrafficPolicy),
			Status:  metav1.ConditionFalse,
			Reason:  string(ngrokv1alpha1.AgentEndpointReasonTrafficPolicyError),
			Message: "Traffic policy validation failed",
		},
	})
	domainResult := createNotReadyDomainResult("ProvisioningError", "Certificate provisioning in progress")

	calculateAgentEndpointReadyCondition(endpoint, domainResult)

	readyCondition := conditions.FindCondition(endpoint.Status.Conditions, ngrokv1alpha1.AgentEndpointConditionReady)
	assert.NotNil(t, readyCondition)
	assert.Equal(t, metav1.ConditionFalse, readyCondition.Status)
	assert.Equal(t, "ProvisioningError", readyCondition.Reason)
	assert.Equal(t, "Certificate provisioning in progress", readyCondition.Message)
}
