package bindings

import (
	"errors"
	"testing"

	bindingsv1alpha1 "github.com/ngrok/ngrok-operator/api/bindings/v1alpha1"
	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/ngrok/ngrok-operator/internal/controller/conditions"
)

func createTestBoundEndpoint(name, namespace string) *bindingsv1alpha1.BoundEndpoint {
	return &bindingsv1alpha1.BoundEndpoint{
		ObjectMeta: metav1.ObjectMeta{
			Name:       name,
			Namespace:  namespace,
			Generation: 1,
		},
		Status: bindingsv1alpha1.BoundEndpointStatus{
			Conditions: []metav1.Condition{},
		},
	}
}

func createTestBoundEndpointWithConditions(name, namespace string, conditions []metav1.Condition) *bindingsv1alpha1.BoundEndpoint {
	be := createTestBoundEndpoint(name, namespace)
	be.Status.Conditions = conditions
	return be
}

func TestSetServicesCreatedCondition_Success(t *testing.T) {
	be := createTestBoundEndpoint("test-be", "ngrok-op")

	setServicesCreatedCondition(be, true, bindingsv1alpha1.BoundEndpointReasonServicesCreated, "Both services created successfully")

	cond := conditions.FindCondition(be.Status.Conditions, bindingsv1alpha1.BoundEndpointConditionServicesCreated)
	assert.NotNil(t, cond)
	assert.Equal(t, metav1.ConditionTrue, cond.Status)
	assert.Equal(t, string(bindingsv1alpha1.BoundEndpointReasonServicesCreated), cond.Reason)
	assert.Equal(t, "Both services created successfully", cond.Message)
	assert.Equal(t, int64(1), cond.ObservedGeneration)
}

func TestSetServicesCreatedCondition_Failure(t *testing.T) {
	be := createTestBoundEndpoint("test-be", "ngrok-op")

	setServicesCreatedCondition(be, false, bindingsv1alpha1.BoundEndpointReasonServiceCreationFailed, "namespaces \"missing\" not found")

	cond := conditions.FindCondition(be.Status.Conditions, bindingsv1alpha1.BoundEndpointConditionServicesCreated)
	assert.NotNil(t, cond)
	assert.Equal(t, metav1.ConditionFalse, cond.Status)
	assert.Equal(t, string(bindingsv1alpha1.BoundEndpointReasonServiceCreationFailed), cond.Reason)
	assert.Equal(t, "namespaces \"missing\" not found", cond.Message)
	assert.Equal(t, int64(1), cond.ObservedGeneration)
}

func TestSetConnectivityVerifiedCondition_Success(t *testing.T) {
	be := createTestBoundEndpoint("test-be", "ngrok-op")

	setConnectivityVerifiedCondition(be, true, nil)

	cond := conditions.FindCondition(be.Status.Conditions, bindingsv1alpha1.BoundEndpointConditionConnectivityVerified)
	assert.NotNil(t, cond)
	assert.Equal(t, metav1.ConditionTrue, cond.Status)
	assert.Equal(t, string(bindingsv1alpha1.BoundEndpointReasonConnectivityVerified), cond.Reason)
	assert.Equal(t, "Successfully connected to upstream service", cond.Message)
	assert.Equal(t, int64(1), cond.ObservedGeneration)
}

func TestSetConnectivityVerifiedCondition_Failure(t *testing.T) {
	be := createTestBoundEndpoint("test-be", "ngrok-op")

	err := errors.New("dial tcp: lookup my-service.namespace: no such host")
	setConnectivityVerifiedCondition(be, false, err)

	cond := conditions.FindCondition(be.Status.Conditions, bindingsv1alpha1.BoundEndpointConditionConnectivityVerified)
	assert.NotNil(t, cond)
	assert.Equal(t, metav1.ConditionFalse, cond.Status)
	assert.Equal(t, string(bindingsv1alpha1.BoundEndpointReasonConnectivityFailed), cond.Reason)
	assert.Contains(t, cond.Message, "dial tcp")
	assert.Equal(t, int64(1), cond.ObservedGeneration)
}

func TestCalculateReadyCondition_AllReady(t *testing.T) {
	be := createTestBoundEndpointWithConditions("test-be", "ngrok-op", []metav1.Condition{
		{
			Type:   string(bindingsv1alpha1.BoundEndpointConditionServicesCreated),
			Status: metav1.ConditionTrue,
			Reason: string(bindingsv1alpha1.BoundEndpointReasonServicesCreated),
		},
		{
			Type:   string(bindingsv1alpha1.BoundEndpointConditionConnectivityVerified),
			Status: metav1.ConditionTrue,
			Reason: string(bindingsv1alpha1.BoundEndpointReasonConnectivityVerified),
		},
	})

	calculateReadyCondition(be)

	readyCond := conditions.FindCondition(be.Status.Conditions, bindingsv1alpha1.BoundEndpointConditionReady)
	assert.NotNil(t, readyCond)
	assert.Equal(t, metav1.ConditionTrue, readyCond.Status)
	assert.Equal(t, string(bindingsv1alpha1.BoundEndpointReasonReady), readyCond.Reason)
	assert.Equal(t, "BoundEndpoint is ready", readyCond.Message)
}

func TestCalculateReadyCondition_ServicesNotCreated(t *testing.T) {
	be := createTestBoundEndpointWithConditions("test-be", "ngrok-op", []metav1.Condition{
		{
			Type:    string(bindingsv1alpha1.BoundEndpointConditionServicesCreated),
			Status:  metav1.ConditionFalse,
			Reason:  string(bindingsv1alpha1.BoundEndpointReasonServiceCreationFailed),
			Message: "Failed to create target service: namespaces \"missing\" not found",
		},
		{
			Type:   string(bindingsv1alpha1.BoundEndpointConditionConnectivityVerified),
			Status: metav1.ConditionTrue,
			Reason: string(bindingsv1alpha1.BoundEndpointReasonConnectivityVerified),
		},
	})

	calculateReadyCondition(be)

	readyCond := conditions.FindCondition(be.Status.Conditions, bindingsv1alpha1.BoundEndpointConditionReady)
	assert.NotNil(t, readyCond)
	assert.Equal(t, metav1.ConditionFalse, readyCond.Status)
	assert.Equal(t, string(bindingsv1alpha1.BoundEndpointReasonServiceCreationFailed), readyCond.Reason)
	assert.Equal(t, "Failed to create target service: namespaces \"missing\" not found", readyCond.Message)
}

func TestCalculateReadyCondition_ServicesNotCreatedMissingCondition(t *testing.T) {
	be := createTestBoundEndpointWithConditions("test-be", "ngrok-op", []metav1.Condition{
		// No ServicesCreated condition
		{
			Type:   string(bindingsv1alpha1.BoundEndpointConditionConnectivityVerified),
			Status: metav1.ConditionTrue,
			Reason: string(bindingsv1alpha1.BoundEndpointReasonConnectivityVerified),
		},
	})

	calculateReadyCondition(be)

	readyCond := conditions.FindCondition(be.Status.Conditions, bindingsv1alpha1.BoundEndpointConditionReady)
	assert.NotNil(t, readyCond)
	assert.Equal(t, metav1.ConditionFalse, readyCond.Status)
	assert.Equal(t, string(bindingsv1alpha1.BoundEndpointReasonServicesNotCreated), readyCond.Reason)
	assert.Equal(t, "Services not yet created", readyCond.Message)
}

func TestCalculateReadyCondition_ConnectivityNotVerified(t *testing.T) {
	be := createTestBoundEndpointWithConditions("test-be", "ngrok-op", []metav1.Condition{
		{
			Type:   string(bindingsv1alpha1.BoundEndpointConditionServicesCreated),
			Status: metav1.ConditionTrue,
			Reason: string(bindingsv1alpha1.BoundEndpointReasonServicesCreated),
		},
		{
			Type:    string(bindingsv1alpha1.BoundEndpointConditionConnectivityVerified),
			Status:  metav1.ConditionFalse,
			Reason:  string(bindingsv1alpha1.BoundEndpointReasonConnectivityFailed),
			Message: "dial tcp: lookup my-service.namespace: no such host",
		},
	})

	calculateReadyCondition(be)

	readyCond := conditions.FindCondition(be.Status.Conditions, bindingsv1alpha1.BoundEndpointConditionReady)
	assert.NotNil(t, readyCond)
	assert.Equal(t, metav1.ConditionFalse, readyCond.Status)
	assert.Equal(t, string(bindingsv1alpha1.BoundEndpointReasonConnectivityFailed), readyCond.Reason)
	assert.Equal(t, "dial tcp: lookup my-service.namespace: no such host", readyCond.Message)
}

func TestCalculateReadyCondition_ConnectivityNotVerifiedMissingCondition(t *testing.T) {
	be := createTestBoundEndpointWithConditions("test-be", "ngrok-op", []metav1.Condition{
		{
			Type:   string(bindingsv1alpha1.BoundEndpointConditionServicesCreated),
			Status: metav1.ConditionTrue,
			Reason: string(bindingsv1alpha1.BoundEndpointReasonServicesCreated),
		},
		// No ConnectivityVerified condition
	})

	calculateReadyCondition(be)

	readyCond := conditions.FindCondition(be.Status.Conditions, bindingsv1alpha1.BoundEndpointConditionReady)
	assert.NotNil(t, readyCond)
	assert.Equal(t, metav1.ConditionFalse, readyCond.Status)
	assert.Equal(t, string(bindingsv1alpha1.BoundEndpointReasonConnectivityNotVerified), readyCond.Reason)
	assert.Equal(t, "Connectivity not yet verified", readyCond.Message)
}

func TestCalculateReadyCondition_NoConditions(t *testing.T) {
	be := createTestBoundEndpoint("test-be", "ngrok-op")

	calculateReadyCondition(be)

	readyCond := conditions.FindCondition(be.Status.Conditions, bindingsv1alpha1.BoundEndpointConditionReady)
	assert.NotNil(t, readyCond)
	assert.Equal(t, metav1.ConditionFalse, readyCond.Status)
	assert.Equal(t, string(bindingsv1alpha1.BoundEndpointReasonServicesNotCreated), readyCond.Reason)
	assert.Equal(t, "Services not yet created", readyCond.Message)
}

func TestCalculateReadyCondition_ServicesCreatedTakesPrecedence(t *testing.T) {
	be := createTestBoundEndpointWithConditions("test-be", "ngrok-op", []metav1.Condition{
		{
			Type:    string(bindingsv1alpha1.BoundEndpointConditionServicesCreated),
			Status:  metav1.ConditionFalse,
			Reason:  string(bindingsv1alpha1.BoundEndpointReasonServiceCreationFailed),
			Message: "Target namespace not found",
		},
		{
			Type:    string(bindingsv1alpha1.BoundEndpointConditionConnectivityVerified),
			Status:  metav1.ConditionFalse,
			Reason:  string(bindingsv1alpha1.BoundEndpointReasonConnectivityFailed),
			Message: "Connection timeout",
		},
	})

	calculateReadyCondition(be)

	readyCond := conditions.FindCondition(be.Status.Conditions, bindingsv1alpha1.BoundEndpointConditionReady)
	assert.NotNil(t, readyCond)
	assert.Equal(t, metav1.ConditionFalse, readyCond.Status)
	assert.Equal(t, string(bindingsv1alpha1.BoundEndpointReasonServiceCreationFailed), readyCond.Reason)
	assert.Equal(t, "Target namespace not found", readyCond.Message)
}

func TestSetReadyCondition(t *testing.T) {
	be := createTestBoundEndpoint("test-be", "ngrok-op")

	setReadyCondition(be, true, bindingsv1alpha1.BoundEndpointReasonReady, "BoundEndpoint is ready")

	readyCond := conditions.FindCondition(be.Status.Conditions, bindingsv1alpha1.BoundEndpointConditionReady)
	assert.NotNil(t, readyCond)
	assert.Equal(t, metav1.ConditionTrue, readyCond.Status)
	assert.Equal(t, string(bindingsv1alpha1.BoundEndpointReasonReady), readyCond.Reason)
	assert.Equal(t, "BoundEndpoint is ready", readyCond.Message)
	assert.Equal(t, int64(1), readyCond.ObservedGeneration)
}

func TestSetReadyCondition_False(t *testing.T) {
	be := createTestBoundEndpoint("test-be", "ngrok-op")

	setReadyCondition(be, false, bindingsv1alpha1.BoundEndpointReasonServicesNotCreated, "Services not yet created")

	readyCond := conditions.FindCondition(be.Status.Conditions, bindingsv1alpha1.BoundEndpointConditionReady)
	assert.NotNil(t, readyCond)
	assert.Equal(t, metav1.ConditionFalse, readyCond.Status)
	assert.Equal(t, string(bindingsv1alpha1.BoundEndpointReasonServicesNotCreated), readyCond.Reason)
	assert.Equal(t, "Services not yet created", readyCond.Message)
	assert.Equal(t, int64(1), readyCond.ObservedGeneration)
}
