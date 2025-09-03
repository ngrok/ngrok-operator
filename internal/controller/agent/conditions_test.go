package agent

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	ngrokv1alpha1 "github.com/ngrok/ngrok-operator/api/ngrok/v1alpha1"
)

func TestConditionSetters(t *testing.T) {
	endpoint := &ngrokv1alpha1.AgentEndpoint{
		ObjectMeta: metav1.ObjectMeta{
			Name:       "test-endpoint",
			Namespace:  "test-namespace",
			Generation: 5,
		},
		Status: ngrokv1alpha1.AgentEndpointStatus{},
	}

	t.Run("setReadyCondition true", func(t *testing.T) {
		setReadyCondition(endpoint, true, ReasonEndpointActive, "Endpoint is ready")

		condition := meta.FindStatusCondition(endpoint.Status.Conditions, ConditionReady)
		assert.NotNil(t, condition)
		assert.Equal(t, metav1.ConditionTrue, condition.Status)
		assert.Equal(t, ReasonEndpointActive, condition.Reason)
		assert.Equal(t, "Endpoint is ready", condition.Message)
		assert.Equal(t, int64(5), condition.ObservedGeneration)
	})

	t.Run("setReadyCondition false", func(t *testing.T) {
		setReadyCondition(endpoint, false, ReasonNgrokAPIError, "Error occurred")

		condition := meta.FindStatusCondition(endpoint.Status.Conditions, ConditionReady)
		assert.NotNil(t, condition)
		assert.Equal(t, metav1.ConditionFalse, condition.Status)
		assert.Equal(t, ReasonNgrokAPIError, condition.Reason)
		assert.Equal(t, "Error occurred", condition.Message)
		assert.Equal(t, int64(5), condition.ObservedGeneration)
	})

	t.Run("setEndpointCreatedCondition", func(t *testing.T) {
		setEndpointCreatedCondition(endpoint, true, ReasonEndpointCreated, "Created successfully")

		condition := meta.FindStatusCondition(endpoint.Status.Conditions, ConditionEndpointCreated)
		assert.NotNil(t, condition)
		assert.Equal(t, metav1.ConditionTrue, condition.Status)
		assert.Equal(t, ReasonEndpointCreated, condition.Reason)
	})

	t.Run("setTrafficPolicyCondition", func(t *testing.T) {
		setTrafficPolicyCondition(endpoint, false, ReasonTrafficPolicyError, "Policy error")

		condition := meta.FindStatusCondition(endpoint.Status.Conditions, ConditionTrafficPolicy)
		assert.NotNil(t, condition)
		assert.Equal(t, metav1.ConditionFalse, condition.Status)
		assert.Equal(t, ReasonTrafficPolicyError, condition.Reason)
	})

	t.Run("setDomainReadyCondition", func(t *testing.T) {
		setDomainReadyCondition(endpoint, true, "DomainReady", "Domain available")

		condition := meta.FindStatusCondition(endpoint.Status.Conditions, ConditionDomainReady)
		assert.NotNil(t, condition)
		assert.Equal(t, metav1.ConditionTrue, condition.Status)
		assert.Equal(t, "DomainReady", condition.Reason)
	})

	t.Run("setReconcilingCondition", func(t *testing.T) {
		setReconcilingCondition(endpoint, "Reconciling now")

		condition := meta.FindStatusCondition(endpoint.Status.Conditions, ConditionReady)
		assert.NotNil(t, condition)
		assert.Equal(t, metav1.ConditionFalse, condition.Status)
		assert.Equal(t, ReasonReconciling, condition.Reason)
		assert.Equal(t, "Reconciling now", condition.Message)
	})
}
