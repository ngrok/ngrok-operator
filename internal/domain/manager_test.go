package domain

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	ingressv1alpha1 "github.com/ngrok/ngrok-operator/api/ingress/v1alpha1"
	ngrokv1alpha1 "github.com/ngrok/ngrok-operator/api/ngrok/v1alpha1"
)

// Helper function to create a test endpoint
func createTestEndpoint(name, namespace, url string) *ngrokv1alpha1.AgentEndpoint {
	return &ngrokv1alpha1.AgentEndpoint{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Spec: ngrokv1alpha1.AgentEndpointSpec{
			URL: url,
		},
		Status: ngrokv1alpha1.AgentEndpointStatus{
			Conditions: []metav1.Condition{},
		},
	}
}

// Helper function to create a test manager
func createTestManager(client client.Client) *Manager {
	return &Manager{
		Client:   client,
		Recorder: record.NewFakeRecorder(10),
	}
}

// Helper function to create a ready domain
func createReadyDomain(name, namespace, domainName string) *ingressv1alpha1.Domain {
	return &ingressv1alpha1.Domain{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Spec: ingressv1alpha1.DomainSpec{
			Domain: domainName,
		},
		Status: ingressv1alpha1.DomainStatus{
			ID: "domain-123", // Required for IsDomainReady
			Conditions: []metav1.Condition{
				{
					Type:    "Ready",
					Status:  metav1.ConditionTrue,
					Reason:  "DomainActive",
					Message: "Domain is active",
				},
			},
		},
	}
}

// Helper function to create a not-ready domain
func createNotReadyDomain(name, namespace, domainName string) *ingressv1alpha1.Domain {
	return &ingressv1alpha1.Domain{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Spec: ingressv1alpha1.DomainSpec{
			Domain: domainName,
		},
		Status: ingressv1alpha1.DomainStatus{
			Conditions: []metav1.Condition{
				{
					Type:    "Ready",
					Status:  metav1.ConditionFalse,
					Reason:  "Provisioning",
					Message: "Domain is being provisioned",
				},
			},
		},
	}
}

func TestManager_EnsureDomainExists_SkipsTCPDomains(t *testing.T) {
	scheme := setupScheme()
	client := fake.NewClientBuilder().WithScheme(scheme).Build()
	manager := createTestManager(client)

	endpoint := createTestEndpoint("test-endpoint", "default", "tcp://1.tcp.ngrok.io:12345")

	result, err := manager.EnsureDomainExists(context.TODO(), endpoint, endpoint.Spec.URL)

	// TCP domains should be skipped and marked as ready
	assert.NoError(t, err)
	assert.True(t, result.IsReady)
	assert.Nil(t, result.Domain)
	assert.Nil(t, endpoint.GetDomainRef())

	// Should set ready condition
	conditions := endpoint.GetConditions()
	assert.Len(t, *conditions, 1)
	assert.Equal(t, ConditionDomainReady, (*conditions)[0].Type)
	assert.Equal(t, metav1.ConditionTrue, (*conditions)[0].Status)
}

func TestManager_EnsureDomainExists_SkipsInternalDomains(t *testing.T) {
	scheme := setupScheme()
	client := fake.NewClientBuilder().WithScheme(scheme).Build()
	manager := createTestManager(client)

	endpoint := createTestEndpoint("test-endpoint", "default", "https://api.service.internal")

	result, err := manager.EnsureDomainExists(context.TODO(), endpoint, endpoint.Spec.URL)

	// Internal domains should be skipped and marked as ready
	assert.NoError(t, err)
	assert.True(t, result.IsReady)
	assert.Nil(t, result.Domain)
	assert.Nil(t, endpoint.GetDomainRef())

	// Should set ready condition
	conditions := endpoint.GetConditions()
	assert.Len(t, *conditions, 1)
	assert.Equal(t, ConditionDomainReady, (*conditions)[0].Type)
	assert.Equal(t, metav1.ConditionTrue, (*conditions)[0].Status)
}

// TestManager_EnsureDomainExists_InvalidURL tests URL parsing errors
func TestManager_EnsureDomainExists_InvalidURL(t *testing.T) {
	scheme := setupScheme()
	client := fake.NewClientBuilder().WithScheme(scheme).Build()
	manager := createTestManager(client)

	endpoint := createTestEndpoint("test-endpoint", "default", "://invalid-url")

	result, err := manager.EnsureDomainExists(context.TODO(), endpoint, endpoint.Spec.URL)

	// Should return error for invalid URL
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to parse URL")
	assert.Nil(t, result)

	// Should set error condition
	conditions := endpoint.GetConditions()
	assert.Len(t, *conditions, 1)
	assert.Equal(t, ConditionDomainReady, (*conditions)[0].Type)
	assert.Equal(t, metav1.ConditionFalse, (*conditions)[0].Status)
}

// TestManager_EnsureDomainExists_ExistingDomainReady tests when domain exists and is ready
func TestManager_EnsureDomainExists_ExistingDomainReady(t *testing.T) {
	scheme := setupScheme()
	existingDomain := createReadyDomain("example-com", "default", "example.com")
	client := fake.NewClientBuilder().WithScheme(scheme).WithObjects(existingDomain).Build()
	manager := createTestManager(client)

	endpoint := createTestEndpoint("test-endpoint", "default", "https://example.com")

	result, err := manager.EnsureDomainExists(context.TODO(), endpoint, endpoint.Spec.URL)

	// Should find existing domain and return ready
	assert.NoError(t, err)
	assert.True(t, result.IsReady)
	assert.NotNil(t, result.Domain)
	assert.Equal(t, existingDomain.Name, result.Domain.Name)

	// Should set domain ref
	domainRef := endpoint.GetDomainRef()
	assert.NotNil(t, domainRef)
	assert.Equal(t, "example-com", domainRef.Name)
	assert.Equal(t, "default", *domainRef.Namespace)

	// Should propagate domain's ready condition
	conditions := endpoint.GetConditions()
	assert.Len(t, *conditions, 1)
	assert.Equal(t, ConditionDomainReady, (*conditions)[0].Type)
	assert.Equal(t, metav1.ConditionTrue, (*conditions)[0].Status)
}

// TestManager_EnsureDomainExists_ExistingDomainNotReady tests when domain exists but is not ready
func TestManager_EnsureDomainExists_ExistingDomainNotReady(t *testing.T) {
	scheme := setupScheme()
	existingDomain := createNotReadyDomain("example-com", "default", "example.com")
	client := fake.NewClientBuilder().WithScheme(scheme).WithObjects(existingDomain).Build()
	manager := createTestManager(client)

	endpoint := createTestEndpoint("test-endpoint", "default", "https://example.com")

	result, err := manager.EnsureDomainExists(context.TODO(), endpoint, endpoint.Spec.URL)

	// Should find existing domain but return not ready (no error)
	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.False(t, result.IsReady)
	assert.NotNil(t, result.Domain)
	assert.Equal(t, existingDomain.Name, result.Domain.Name)

	// Should set domain ref
	domainRef := endpoint.GetDomainRef()
	assert.NotNil(t, domainRef)
	assert.Equal(t, "example-com", domainRef.Name)
	assert.Equal(t, "default", *domainRef.Namespace)

	// Should propagate domain's not-ready condition
	conditions := endpoint.GetConditions()
	assert.Len(t, *conditions, 1)
	assert.Equal(t, ConditionDomainReady, (*conditions)[0].Type)
	assert.Equal(t, metav1.ConditionFalse, (*conditions)[0].Status)
}

// TestManager_EnsureDomainExists_ExistingDomainNoReadyCondition tests when domain exists but has no Ready condition
func TestManager_EnsureDomainExists_ExistingDomainNoReadyCondition(t *testing.T) {
	scheme := setupScheme()
	existingDomain := &ingressv1alpha1.Domain{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "example-com",
			Namespace: "default",
		},
		Spec: ingressv1alpha1.DomainSpec{
			Domain: "example.com",
		},
		Status: ingressv1alpha1.DomainStatus{
			Conditions: []metav1.Condition{},
		},
	}
	client := fake.NewClientBuilder().WithScheme(scheme).WithObjects(existingDomain).Build()
	manager := createTestManager(client)

	endpoint := createTestEndpoint("test-endpoint", "default", "https://example.com")

	result, err := manager.EnsureDomainExists(context.TODO(), endpoint, endpoint.Spec.URL)

	// Should find existing domain but return not ready (no Ready condition, no error)
	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.False(t, result.IsReady)
	assert.NotNil(t, result.Domain)
	assert.Equal(t, existingDomain.Name, result.Domain.Name)

	// Should set domain ref
	domainRef := endpoint.GetDomainRef()
	assert.NotNil(t, domainRef)
	assert.Equal(t, "example-com", domainRef.Name)
	assert.Equal(t, "default", *domainRef.Namespace)

	// Should set creating condition
	conditions := endpoint.GetConditions()
	assert.Len(t, *conditions, 1)
	assert.Equal(t, ConditionDomainReady, (*conditions)[0].Type)
	assert.Equal(t, metav1.ConditionFalse, (*conditions)[0].Status)
}

// TestManager_EnsureDomainExists_CreateNewDomain tests creating a new domain
func TestManager_EnsureDomainExists_CreateNewDomain(t *testing.T) {
	scheme := setupScheme()
	client := fake.NewClientBuilder().WithScheme(scheme).Build()
	manager := createTestManager(client)

	endpoint := createTestEndpoint("test-endpoint", "default", "https://example.com")

	result, err := manager.EnsureDomainExists(context.TODO(), endpoint, endpoint.Spec.URL)

	// Should create new domain and return not ready (no error)
	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.False(t, result.IsReady)
	assert.NotNil(t, result.Domain)
	assert.Equal(t, "example-com", result.Domain.Name)
	assert.Equal(t, "example.com", result.Domain.Spec.Domain)

	// Should set domain ref
	domainRef := endpoint.GetDomainRef()
	assert.NotNil(t, domainRef)
	assert.Equal(t, "example-com", domainRef.Name)
	assert.Equal(t, "default", *domainRef.Namespace)

	// Should set creating condition
	conditions := endpoint.GetConditions()
	assert.Len(t, *conditions, 1)
	assert.Equal(t, ConditionDomainReady, (*conditions)[0].Type)
	assert.Equal(t, metav1.ConditionFalse, (*conditions)[0].Status)

	// Verify domain was created in the client
	var createdDomain ingressv1alpha1.Domain
	err = client.Get(context.TODO(), types.NamespacedName{Name: "example-com", Namespace: "default"}, &createdDomain)
	assert.NoError(t, err)
	assert.Equal(t, "example.com", createdDomain.Spec.Domain)
}

// TestManager_EnsureDomainExists_CreateNewDomainWithReclaimPolicy tests creating a new domain with default reclaim policy
func TestManager_EnsureDomainExists_CreateNewDomainWithReclaimPolicy(t *testing.T) {
	scheme := setupScheme()
	client := fake.NewClientBuilder().WithScheme(scheme).Build()
	reclaimPolicy := ingressv1alpha1.DomainReclaimPolicyRetain
	manager := &Manager{
		Client:                     client,
		Recorder:                   record.NewFakeRecorder(10),
		DefaultDomainReclaimPolicy: &reclaimPolicy,
	}

	endpoint := createTestEndpoint("test-endpoint", "default", "https://example.com")

	result, err := manager.EnsureDomainExists(context.TODO(), endpoint, endpoint.Spec.URL)

	// Should create domain and return not ready (no error)
	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.False(t, result.IsReady)
	assert.NotNil(t, result.Domain)

	// Verify domain was created with reclaim policy
	var createdDomain ingressv1alpha1.Domain
	err = client.Get(context.TODO(), types.NamespacedName{Name: "example-com", Namespace: "default"}, &createdDomain)
	assert.NoError(t, err)
	assert.Equal(t, ingressv1alpha1.DomainReclaimPolicyRetain, createdDomain.Spec.ReclaimPolicy)
}

// TestManager_setDomainCondition tests the setDomainCondition method
func TestManager_setDomainCondition(t *testing.T) {
	manager := &Manager{}
	endpoint := createTestEndpoint("test-endpoint", "default", "https://example.com")

	// Test setting ready condition
	manager.setDomainCondition(endpoint, true, "TestReason", "Test message")

	conditions := endpoint.GetConditions()
	require.Len(t, *conditions, 1)
	assert.Equal(t, ConditionDomainReady, (*conditions)[0].Type)
	assert.Equal(t, metav1.ConditionTrue, (*conditions)[0].Status)
	assert.Equal(t, "TestReason", (*conditions)[0].Reason)
	assert.Equal(t, "Test message", (*conditions)[0].Message)
	assert.Equal(t, int64(0), (*conditions)[0].ObservedGeneration)

	// Test setting not ready condition (should replace previous)
	manager.setDomainCondition(endpoint, false, "TestReason2", "Test message 2")

	conditions = endpoint.GetConditions()
	require.Len(t, *conditions, 1) // Should replace the previous condition
	assert.Equal(t, ConditionDomainReady, (*conditions)[0].Type)
	assert.Equal(t, metav1.ConditionFalse, (*conditions)[0].Status)
	assert.Equal(t, "TestReason2", (*conditions)[0].Reason)
	assert.Equal(t, "Test message 2", (*conditions)[0].Message)
}

func setupScheme() *runtime.Scheme {
	scheme := runtime.NewScheme()
	_ = ngrokv1alpha1.AddToScheme(scheme)
	_ = ingressv1alpha1.AddToScheme(scheme)
	return scheme
}
