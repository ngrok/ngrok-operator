package domain

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	ingressv1alpha1 "github.com/ngrok/ngrok-operator/api/ingress/v1alpha1"
	ngrokv1alpha1 "github.com/ngrok/ngrok-operator/api/ngrok/v1alpha1"
)

func TestManager_EnsureDomainExists_SkipsTCPDomains(t *testing.T) {
	// Set up fake client and manager
	scheme := setupScheme()
	client := fake.NewClientBuilder().WithScheme(scheme).Build()
	recorder := record.NewFakeRecorder(10)

	manager := &Manager{
		Client:   client,
		Recorder: recorder,
	}

	// Create test endpoint with TCP URL
	endpoint := &ngrokv1alpha1.AgentEndpoint{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-endpoint",
			Namespace: "default",
		},
		Spec: ngrokv1alpha1.AgentEndpointSpec{
			URL: "tcp://1.tcp.ngrok.io:12345",
		},
		Status: ngrokv1alpha1.AgentEndpointStatus{
			Conditions: []metav1.Condition{},
		},
	}

	// Test the domain manager
	result, err := manager.EnsureDomainExists(context.TODO(), endpoint, endpoint.Spec.URL)

	// Should not create a domain and should return ready
	assert.NoError(t, err)
	assert.True(t, result.IsReady)
	assert.Equal(t, "tcp", result.ReadyReason)
	assert.Nil(t, result.Domain)
	assert.Nil(t, endpoint.GetDomainRef())

	// Check that DomainReady condition was set
	conditions := endpoint.GetConditions()
	assert.Len(t, *conditions, 1)
	assert.Equal(t, ConditionDomainReady, (*conditions)[0].Type)
	assert.Equal(t, metav1.ConditionTrue, (*conditions)[0].Status)
}

func TestManager_EnsureDomainExists_SkipsInternalDomains(t *testing.T) {
	// Set up fake client and manager
	scheme := setupScheme()
	client := fake.NewClientBuilder().WithScheme(scheme).Build()
	recorder := record.NewFakeRecorder(10)

	manager := &Manager{
		Client:   client,
		Recorder: recorder,
	}

	// Create test endpoint with internal URL
	endpoint := &ngrokv1alpha1.AgentEndpoint{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-endpoint",
			Namespace: "default",
		},
		Spec: ngrokv1alpha1.AgentEndpointSpec{
			URL: "https://api.service.internal",
		},
		Status: ngrokv1alpha1.AgentEndpointStatus{
			Conditions: []metav1.Condition{},
		},
	}

	// Test the domain manager
	result, err := manager.EnsureDomainExists(context.TODO(), endpoint, endpoint.Spec.URL)

	// Should not create a domain and should return ready
	assert.NoError(t, err)
	assert.True(t, result.IsReady)
	assert.Equal(t, "internal", result.ReadyReason)
	assert.Nil(t, result.Domain)
	assert.Nil(t, endpoint.GetDomainRef())

	// Check that DomainReady condition was set
	conditions := endpoint.GetConditions()
	assert.Len(t, *conditions, 1)
	assert.Equal(t, ConditionDomainReady, (*conditions)[0].Type)
	assert.Equal(t, metav1.ConditionTrue, (*conditions)[0].Status)
}

func setupScheme() *runtime.Scheme {
	scheme := runtime.NewScheme()
	_ = ngrokv1alpha1.AddToScheme(scheme)
	_ = ingressv1alpha1.AddToScheme(scheme)
	return scheme
}
