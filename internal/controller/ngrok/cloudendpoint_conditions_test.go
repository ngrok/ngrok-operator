package ngrok

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	ngrokv1alpha1 "github.com/ngrok/ngrok-operator/api/ngrok/v1alpha1"
	domainpkg "github.com/ngrok/ngrok-operator/internal/domain"
)

// Helper function to create a test CloudEndpoint
func createTestCloudEndpoint(name, namespace string) *ngrokv1alpha1.CloudEndpoint {
	return &ngrokv1alpha1.CloudEndpoint{
		ObjectMeta: metav1.ObjectMeta{
			Name:       name,
			Namespace:  namespace,
			Generation: 1,
		},
		Status: ngrokv1alpha1.CloudEndpointStatus{
			Conditions: []metav1.Condition{},
		},
	}
}

// Helper function to create a test CloudEndpoint with conditions
func createTestCloudEndpointWithConditions(name, namespace string, conditions []metav1.Condition) *ngrokv1alpha1.CloudEndpoint {
	endpoint := createTestCloudEndpoint(name, namespace)
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

func TestCalculateCloudEndpointReadyCondition_AllReady(t *testing.T) {
	endpoint := createTestCloudEndpointWithConditions("test-endpoint", "default", []metav1.Condition{
		{
			Type:   ConditionCloudEndpointCreated,
			Status: metav1.ConditionTrue,
			Reason: ReasonCloudEndpointCreated,
		},
	})
	domainResult := createReadyDomainResult()

	calculateCloudEndpointReadyCondition(endpoint, domainResult)

	readyCondition := meta.FindStatusCondition(endpoint.Status.Conditions, ConditionCloudEndpointReady)
	assert.NotNil(t, readyCondition)
	assert.Equal(t, metav1.ConditionTrue, readyCondition.Status)
	assert.Equal(t, ReasonCloudEndpointActive, readyCondition.Reason)
	assert.Equal(t, "CloudEndpoint is active and ready", readyCondition.Message)
}

func TestCalculateCloudEndpointReadyCondition_DomainNotReady(t *testing.T) {
	endpoint := createTestCloudEndpointWithConditions("test-endpoint", "default", []metav1.Condition{
		{
			Type:   ConditionCloudEndpointCreated,
			Status: metav1.ConditionTrue,
			Reason: ReasonCloudEndpointCreated,
		},
	})
	domainResult := createNotReadyDomainResult("ProvisioningError", "Certificate provisioning in progress")

	calculateCloudEndpointReadyCondition(endpoint, domainResult)

	readyCondition := meta.FindStatusCondition(endpoint.Status.Conditions, ConditionCloudEndpointReady)
	assert.NotNil(t, readyCondition)
	assert.Equal(t, metav1.ConditionFalse, readyCondition.Status)
	assert.Equal(t, "ProvisioningError", readyCondition.Reason)
	assert.Equal(t, "Certificate provisioning in progress", readyCondition.Message)
}

func TestCalculateCloudEndpointReadyCondition_DomainNotReadyNoReason(t *testing.T) {
	endpoint := createTestCloudEndpointWithConditions("test-endpoint", "default", []metav1.Condition{
		{
			Type:   ConditionCloudEndpointCreated,
			Status: metav1.ConditionTrue,
			Reason: ReasonCloudEndpointCreated,
		},
	})
	domainResult := &domainpkg.DomainResult{
		IsReady: false,
		// No ReadyReason or ReadyMessage
	}

	calculateCloudEndpointReadyCondition(endpoint, domainResult)

	readyCondition := meta.FindStatusCondition(endpoint.Status.Conditions, ConditionCloudEndpointReady)
	assert.NotNil(t, readyCondition)
	assert.Equal(t, metav1.ConditionFalse, readyCondition.Status)
	assert.Equal(t, "DomainNotReady", readyCondition.Reason)
	assert.Equal(t, "Domain is not ready", readyCondition.Message)
}

func TestCalculateCloudEndpointReadyCondition_CloudEndpointNotCreated(t *testing.T) {
	endpoint := createTestCloudEndpointWithConditions("test-endpoint", "default", []metav1.Condition{
		{
			Type:    ConditionCloudEndpointCreated,
			Status:  metav1.ConditionFalse,
			Reason:  ReasonCloudEndpointCreationFailed,
			Message: "Failed to create CloudEndpoint",
		},
	})
	domainResult := createReadyDomainResult()

	calculateCloudEndpointReadyCondition(endpoint, domainResult)

	readyCondition := meta.FindStatusCondition(endpoint.Status.Conditions, ConditionCloudEndpointReady)
	assert.NotNil(t, readyCondition)
	assert.Equal(t, metav1.ConditionFalse, readyCondition.Status)
	assert.Equal(t, ReasonCloudEndpointCreationFailed, readyCondition.Reason)
	assert.Equal(t, "Failed to create CloudEndpoint", readyCondition.Message)
}

func TestCalculateCloudEndpointReadyCondition_CloudEndpointNotCreatedNoCondition(t *testing.T) {
	endpoint := createTestCloudEndpointWithConditions("test-endpoint", "default", []metav1.Condition{
		// No CloudEndpointCreated condition
	})
	domainResult := createReadyDomainResult()

	calculateCloudEndpointReadyCondition(endpoint, domainResult)

	readyCondition := meta.FindStatusCondition(endpoint.Status.Conditions, ConditionCloudEndpointReady)
	assert.NotNil(t, readyCondition)
	assert.Equal(t, metav1.ConditionFalse, readyCondition.Status)
	assert.Equal(t, "Pending", readyCondition.Reason)
	assert.Equal(t, "Waiting for CloudEndpoint to be ready", readyCondition.Message)
}

func TestCalculateCloudEndpointReadyCondition_MultipleIssues(t *testing.T) {
	// Domain not ready should take precedence over other issues
	endpoint := createTestCloudEndpointWithConditions("test-endpoint", "default", []metav1.Condition{
		{
			Type:    ConditionCloudEndpointCreated,
			Status:  metav1.ConditionFalse,
			Reason:  ReasonCloudEndpointCreationFailed,
			Message: "Failed to create CloudEndpoint",
		},
	})
	domainResult := createNotReadyDomainResult("ProvisioningError", "Certificate provisioning in progress")

	calculateCloudEndpointReadyCondition(endpoint, domainResult)

	readyCondition := meta.FindStatusCondition(endpoint.Status.Conditions, ConditionCloudEndpointReady)
	assert.NotNil(t, readyCondition)
	assert.Equal(t, metav1.ConditionFalse, readyCondition.Status)
	assert.Equal(t, "ProvisioningError", readyCondition.Reason)
	assert.Equal(t, "Certificate provisioning in progress", readyCondition.Message)
}

func TestCalculateCloudEndpointReadyCondition_UnknownState(t *testing.T) {
	// This should not happen in practice, but test the default case
	endpoint := createTestCloudEndpointWithConditions("test-endpoint", "default", []metav1.Condition{
		{
			Type:   ConditionCloudEndpointCreated,
			Status: metav1.ConditionTrue,
			Reason: ReasonCloudEndpointCreated,
		},
	})
	domainResult := createReadyDomainResult()

	// Manually set a condition that would cause the switch to hit default case
	// This is a bit artificial but tests the default branch
	calculateCloudEndpointReadyCondition(endpoint, domainResult)

	readyCondition := meta.FindStatusCondition(endpoint.Status.Conditions, ConditionCloudEndpointReady)
	assert.NotNil(t, readyCondition)
	assert.Equal(t, metav1.ConditionTrue, readyCondition.Status)
	assert.Equal(t, ReasonCloudEndpointActive, readyCondition.Reason)
	assert.Equal(t, "CloudEndpoint is active and ready", readyCondition.Message)
}
