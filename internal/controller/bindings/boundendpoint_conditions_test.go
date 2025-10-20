/*
MIT License

Copyright (c) 2024 ngrok, Inc.

Permission is hereby granted, free of charge, to any person obtaining a copy
of this software and associated documentation files (the "Software"), to deal
in the Software without restriction, including without limitation the rights
to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
copies of the Software, and to permit persons to whom the Software is
furnished to do so, subject to the following conditions:

The above copyright notice and this permission notice shall be included in all
copies or substantial portions of the Software.

THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
SOFTWARE.
*/

package bindings

import (
	"errors"
	"testing"

	bindingsv1alpha1 "github.com/ngrok/ngrok-operator/api/bindings/v1alpha1"
	"github.com/stretchr/testify/assert"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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

	setServicesCreatedCondition(be, true, ReasonServicesCreated, "Both services created successfully")

	cond := meta.FindStatusCondition(be.Status.Conditions, ConditionTypeServicesCreated)
	assert.NotNil(t, cond)
	assert.Equal(t, metav1.ConditionTrue, cond.Status)
	assert.Equal(t, ReasonServicesCreated, cond.Reason)
	assert.Equal(t, "Both services created successfully", cond.Message)
	assert.Equal(t, int64(1), cond.ObservedGeneration)
}

func TestSetServicesCreatedCondition_Failure(t *testing.T) {
	be := createTestBoundEndpoint("test-be", "ngrok-op")

	setServicesCreatedCondition(be, false, ReasonServiceCreationFailed, "namespaces \"missing\" not found")

	cond := meta.FindStatusCondition(be.Status.Conditions, ConditionTypeServicesCreated)
	assert.NotNil(t, cond)
	assert.Equal(t, metav1.ConditionFalse, cond.Status)
	assert.Equal(t, ReasonServiceCreationFailed, cond.Reason)
	assert.Equal(t, "namespaces \"missing\" not found", cond.Message)
	assert.Equal(t, int64(1), cond.ObservedGeneration)
}

func TestSetConnectivityVerifiedCondition_Success(t *testing.T) {
	be := createTestBoundEndpoint("test-be", "ngrok-op")

	setConnectivityVerifiedCondition(be, true, nil)

	cond := meta.FindStatusCondition(be.Status.Conditions, ConditionTypeConnectivityVerified)
	assert.NotNil(t, cond)
	assert.Equal(t, metav1.ConditionTrue, cond.Status)
	assert.Equal(t, ReasonConnectivityVerified, cond.Reason)
	assert.Equal(t, "Successfully connected to upstream service", cond.Message)
	assert.Equal(t, int64(1), cond.ObservedGeneration)
}

func TestSetConnectivityVerifiedCondition_Failure(t *testing.T) {
	be := createTestBoundEndpoint("test-be", "ngrok-op")

	err := errors.New("dial tcp: lookup my-service.namespace: no such host")
	setConnectivityVerifiedCondition(be, false, err)

	cond := meta.FindStatusCondition(be.Status.Conditions, ConditionTypeConnectivityVerified)
	assert.NotNil(t, cond)
	assert.Equal(t, metav1.ConditionFalse, cond.Status)
	assert.Equal(t, ReasonConnectivityFailed, cond.Reason)
	assert.Contains(t, cond.Message, "dial tcp")
	assert.Equal(t, int64(1), cond.ObservedGeneration)
}

func TestCalculateReadyCondition_AllReady(t *testing.T) {
	be := createTestBoundEndpointWithConditions("test-be", "ngrok-op", []metav1.Condition{
		{
			Type:   ConditionTypeServicesCreated,
			Status: metav1.ConditionTrue,
			Reason: ReasonServicesCreated,
		},
		{
			Type:   ConditionTypeConnectivityVerified,
			Status: metav1.ConditionTrue,
			Reason: ReasonConnectivityVerified,
		},
	})

	calculateReadyCondition(be)

	readyCond := meta.FindStatusCondition(be.Status.Conditions, ConditionTypeReady)
	assert.NotNil(t, readyCond)
	assert.Equal(t, metav1.ConditionTrue, readyCond.Status)
	assert.Equal(t, ReasonBoundEndpointReady, readyCond.Reason)
	assert.Equal(t, "BoundEndpoint is ready", readyCond.Message)
}

func TestCalculateReadyCondition_ServicesNotCreated(t *testing.T) {
	be := createTestBoundEndpointWithConditions("test-be", "ngrok-op", []metav1.Condition{
		{
			Type:    ConditionTypeServicesCreated,
			Status:  metav1.ConditionFalse,
			Reason:  ReasonServiceCreationFailed,
			Message: "Failed to create target service: namespaces \"missing\" not found",
		},
		{
			Type:   ConditionTypeConnectivityVerified,
			Status: metav1.ConditionTrue,
			Reason: ReasonConnectivityVerified,
		},
	})

	calculateReadyCondition(be)

	readyCond := meta.FindStatusCondition(be.Status.Conditions, ConditionTypeReady)
	assert.NotNil(t, readyCond)
	assert.Equal(t, metav1.ConditionFalse, readyCond.Status)
	assert.Equal(t, ReasonServiceCreationFailed, readyCond.Reason)
	assert.Equal(t, "Failed to create target service: namespaces \"missing\" not found", readyCond.Message)
}

func TestCalculateReadyCondition_ServicesNotCreatedMissingCondition(t *testing.T) {
	be := createTestBoundEndpointWithConditions("test-be", "ngrok-op", []metav1.Condition{
		// No ServicesCreated condition
		{
			Type:   ConditionTypeConnectivityVerified,
			Status: metav1.ConditionTrue,
			Reason: ReasonConnectivityVerified,
		},
	})

	calculateReadyCondition(be)

	readyCond := meta.FindStatusCondition(be.Status.Conditions, ConditionTypeReady)
	assert.NotNil(t, readyCond)
	assert.Equal(t, metav1.ConditionFalse, readyCond.Status)
	assert.Equal(t, ReasonServicesNotCreated, readyCond.Reason)
	assert.Equal(t, "Services not yet created", readyCond.Message)
}

func TestCalculateReadyCondition_ConnectivityNotVerified(t *testing.T) {
	be := createTestBoundEndpointWithConditions("test-be", "ngrok-op", []metav1.Condition{
		{
			Type:   ConditionTypeServicesCreated,
			Status: metav1.ConditionTrue,
			Reason: ReasonServicesCreated,
		},
		{
			Type:    ConditionTypeConnectivityVerified,
			Status:  metav1.ConditionFalse,
			Reason:  ReasonConnectivityFailed,
			Message: "dial tcp: lookup my-service.namespace: no such host",
		},
	})

	calculateReadyCondition(be)

	readyCond := meta.FindStatusCondition(be.Status.Conditions, ConditionTypeReady)
	assert.NotNil(t, readyCond)
	assert.Equal(t, metav1.ConditionFalse, readyCond.Status)
	assert.Equal(t, ReasonConnectivityFailed, readyCond.Reason)
	assert.Equal(t, "dial tcp: lookup my-service.namespace: no such host", readyCond.Message)
}

func TestCalculateReadyCondition_ConnectivityNotVerifiedMissingCondition(t *testing.T) {
	be := createTestBoundEndpointWithConditions("test-be", "ngrok-op", []metav1.Condition{
		{
			Type:   ConditionTypeServicesCreated,
			Status: metav1.ConditionTrue,
			Reason: ReasonServicesCreated,
		},
		// No ConnectivityVerified condition
	})

	calculateReadyCondition(be)

	readyCond := meta.FindStatusCondition(be.Status.Conditions, ConditionTypeReady)
	assert.NotNil(t, readyCond)
	assert.Equal(t, metav1.ConditionFalse, readyCond.Status)
	assert.Equal(t, ReasonConnectivityNotVerified, readyCond.Reason)
	assert.Equal(t, "Connectivity not yet verified", readyCond.Message)
}

func TestCalculateReadyCondition_NoConditions(t *testing.T) {
	be := createTestBoundEndpoint("test-be", "ngrok-op")

	calculateReadyCondition(be)

	readyCond := meta.FindStatusCondition(be.Status.Conditions, ConditionTypeReady)
	assert.NotNil(t, readyCond)
	assert.Equal(t, metav1.ConditionFalse, readyCond.Status)
	assert.Equal(t, ReasonServicesNotCreated, readyCond.Reason)
	assert.Equal(t, "Services not yet created", readyCond.Message)
}

func TestCalculateReadyCondition_ServicesCreatedTakesPrecedence(t *testing.T) {
	be := createTestBoundEndpointWithConditions("test-be", "ngrok-op", []metav1.Condition{
		{
			Type:    ConditionTypeServicesCreated,
			Status:  metav1.ConditionFalse,
			Reason:  ReasonServiceCreationFailed,
			Message: "Target namespace not found",
		},
		{
			Type:    ConditionTypeConnectivityVerified,
			Status:  metav1.ConditionFalse,
			Reason:  ReasonConnectivityFailed,
			Message: "Connection timeout",
		},
	})

	calculateReadyCondition(be)

	readyCond := meta.FindStatusCondition(be.Status.Conditions, ConditionTypeReady)
	assert.NotNil(t, readyCond)
	assert.Equal(t, metav1.ConditionFalse, readyCond.Status)
	assert.Equal(t, ReasonServiceCreationFailed, readyCond.Reason)
	assert.Equal(t, "Target namespace not found", readyCond.Message)
}

func TestSetReadyCondition(t *testing.T) {
	be := createTestBoundEndpoint("test-be", "ngrok-op")

	setReadyCondition(be, true, ReasonBoundEndpointReady, "BoundEndpoint is ready")

	readyCond := meta.FindStatusCondition(be.Status.Conditions, ConditionTypeReady)
	assert.NotNil(t, readyCond)
	assert.Equal(t, metav1.ConditionTrue, readyCond.Status)
	assert.Equal(t, ReasonBoundEndpointReady, readyCond.Reason)
	assert.Equal(t, "BoundEndpoint is ready", readyCond.Message)
	assert.Equal(t, int64(1), readyCond.ObservedGeneration)
}

func TestSetReadyCondition_False(t *testing.T) {
	be := createTestBoundEndpoint("test-be", "ngrok-op")

	setReadyCondition(be, false, ReasonServicesNotCreated, "Services not yet created")

	readyCond := meta.FindStatusCondition(be.Status.Conditions, ConditionTypeReady)
	assert.NotNil(t, readyCond)
	assert.Equal(t, metav1.ConditionFalse, readyCond.Status)
	assert.Equal(t, ReasonServicesNotCreated, readyCond.Reason)
	assert.Equal(t, "Services not yet created", readyCond.Message)
	assert.Equal(t, int64(1), readyCond.ObservedGeneration)
}
