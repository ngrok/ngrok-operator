package domain

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/api/errors"
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

	result, err := manager.EnsureDomainExists(context.TODO(), endpoint, DomainCheckParams{
		URL:      endpoint.Spec.URL,
		Bindings: endpoint.Spec.Bindings,
	})

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

	result, err := manager.EnsureDomainExists(context.TODO(), endpoint, DomainCheckParams{
		URL:      endpoint.Spec.URL,
		Bindings: endpoint.Spec.Bindings,
	})

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

	result, err := manager.EnsureDomainExists(context.TODO(), endpoint, DomainCheckParams{
		URL:      endpoint.Spec.URL,
		Bindings: endpoint.Spec.Bindings,
	})

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

	result, err := manager.EnsureDomainExists(context.TODO(), endpoint, DomainCheckParams{
		URL:      endpoint.Spec.URL,
		Bindings: endpoint.Spec.Bindings,
	})

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

	result, err := manager.EnsureDomainExists(context.TODO(), endpoint, DomainCheckParams{
		URL:      endpoint.Spec.URL,
		Bindings: endpoint.Spec.Bindings,
	})

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

	result, err := manager.EnsureDomainExists(context.TODO(), endpoint, DomainCheckParams{
		URL:      endpoint.Spec.URL,
		Bindings: endpoint.Spec.Bindings,
	})

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

	result, err := manager.EnsureDomainExists(context.TODO(), endpoint, DomainCheckParams{
		URL:      endpoint.Spec.URL,
		Bindings: endpoint.Spec.Bindings,
	})

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

	result, err := manager.EnsureDomainExists(context.TODO(), endpoint, DomainCheckParams{
		URL:      endpoint.Spec.URL,
		Bindings: endpoint.Spec.Bindings,
	})

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

// TestManager_EnsureDomainExists_SkipsKubernetesBinding_AgentEndpoint tests that endpoints with kubernetes binding skip domain creation
func TestManager_EnsureDomainExists_SkipsKubernetesBinding_AgentEndpoint(t *testing.T) {
	scheme := setupScheme()
	client := fake.NewClientBuilder().WithScheme(scheme).Build()
	manager := createTestManager(client)

	endpoint := &ngrokv1alpha1.AgentEndpoint{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "k8s-bound-endpoint",
			Namespace: "default",
		},
		Spec: ngrokv1alpha1.AgentEndpointSpec{
			URL:      "http://aws.demo",
			Bindings: []string{"kubernetes"},
		},
		Status: ngrokv1alpha1.AgentEndpointStatus{
			Conditions: []metav1.Condition{},
		},
	}

	result, err := manager.EnsureDomainExists(context.TODO(), endpoint, DomainCheckParams{
		URL:      endpoint.Spec.URL,
		Bindings: endpoint.Spec.Bindings,
	})

	// Kubernetes-bound endpoints should skip domain creation and be marked as ready
	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.True(t, result.IsReady)
	assert.Nil(t, result.Domain)
	assert.Nil(t, endpoint.GetDomainRef())

	// Should set ready condition with Kubernetes binding message
	conditions := endpoint.GetConditions()
	assert.Len(t, *conditions, 1)
	assert.Equal(t, ConditionDomainReady, (*conditions)[0].Type)
	assert.Equal(t, metav1.ConditionTrue, (*conditions)[0].Status)
	assert.Contains(t, (*conditions)[0].Message, "Kubernetes binding")

	// Verify no Domain CRD was created
	var domains ingressv1alpha1.DomainList
	err = client.List(context.TODO(), &domains)
	assert.NoError(t, err)
	assert.Empty(t, domains.Items)
}

// TestManager_EnsureDomainExists_SkipsKubernetesBinding_CloudEndpoint tests that CloudEndpoints with kubernetes binding skip domain creation
func TestManager_EnsureDomainExists_SkipsKubernetesBinding_CloudEndpoint(t *testing.T) {
	scheme := setupScheme()
	client := fake.NewClientBuilder().WithScheme(scheme).Build()
	manager := createTestManager(client)

	endpoint := &ngrokv1alpha1.CloudEndpoint{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "k8s-bound-endpoint",
			Namespace: "default",
		},
		Spec: ngrokv1alpha1.CloudEndpointSpec{
			URL:      "http://aws.demo",
			Bindings: []string{"kubernetes"},
		},
		Status: ngrokv1alpha1.CloudEndpointStatus{
			Conditions: []metav1.Condition{},
		},
	}

	result, err := manager.EnsureDomainExists(context.TODO(), endpoint, DomainCheckParams{
		URL:      endpoint.Spec.URL,
		Bindings: endpoint.Spec.Bindings,
	})

	// Kubernetes-bound endpoints should skip domain creation and be marked as ready
	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.True(t, result.IsReady)
	assert.Nil(t, result.Domain)
	assert.Nil(t, endpoint.GetDomainRef())

	// Should set ready condition with Kubernetes binding message
	conditions := endpoint.GetConditions()
	assert.Len(t, *conditions, 1)
	assert.Equal(t, ConditionDomainReady, (*conditions)[0].Type)
	assert.Equal(t, metav1.ConditionTrue, (*conditions)[0].Status)
	assert.Contains(t, (*conditions)[0].Message, "Kubernetes binding")

	// Verify no Domain CRD was created
	var domains ingressv1alpha1.DomainList
	err = client.List(context.TODO(), &domains)
	assert.NoError(t, err)
	assert.Empty(t, domains.Items)
}

// TestManager_EnsureDomainExists_SkipsInternalBinding_AgentEndpoint tests that endpoints with internal binding skip domain creation
func TestManager_EnsureDomainExists_SkipsInternalBinding_AgentEndpoint(t *testing.T) {
	scheme := setupScheme()
	client := fake.NewClientBuilder().WithScheme(scheme).Build()
	manager := createTestManager(client)

	endpoint := &ngrokv1alpha1.AgentEndpoint{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "internal-bound-endpoint",
			Namespace: "default",
		},
		Spec: ngrokv1alpha1.AgentEndpointSpec{
			URL:      "http://internal.demo",
			Bindings: []string{"internal"},
		},
		Status: ngrokv1alpha1.AgentEndpointStatus{
			Conditions: []metav1.Condition{},
		},
	}

	result, err := manager.EnsureDomainExists(context.TODO(), endpoint, DomainCheckParams{
		URL:      endpoint.Spec.URL,
		Bindings: endpoint.Spec.Bindings,
	})

	// Internal-bound endpoints should skip domain creation and be marked as ready
	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.True(t, result.IsReady)
	assert.Nil(t, result.Domain)
	assert.Nil(t, endpoint.GetDomainRef())

	// Should set ready condition with internal binding message
	conditions := endpoint.GetConditions()
	assert.Len(t, *conditions, 1)
	assert.Equal(t, ConditionDomainReady, (*conditions)[0].Type)
	assert.Equal(t, metav1.ConditionTrue, (*conditions)[0].Status)
	assert.Contains(t, (*conditions)[0].Message, "internal binding")

	// Verify no Domain CRD was created
	var domains ingressv1alpha1.DomainList
	err = client.List(context.TODO(), &domains)
	assert.NoError(t, err)
	assert.Empty(t, domains.Items)
}

// TestManager_EnsureDomainExists_SkipsInternalBinding_CloudEndpoint tests that CloudEndpoints with internal binding skip domain creation
func TestManager_EnsureDomainExists_SkipsInternalBinding_CloudEndpoint(t *testing.T) {
	scheme := setupScheme()
	client := fake.NewClientBuilder().WithScheme(scheme).Build()
	manager := createTestManager(client)

	endpoint := &ngrokv1alpha1.CloudEndpoint{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "internal-bound-endpoint",
			Namespace: "default",
		},
		Spec: ngrokv1alpha1.CloudEndpointSpec{
			URL:      "http://internal.demo",
			Bindings: []string{"internal"},
		},
		Status: ngrokv1alpha1.CloudEndpointStatus{
			Conditions: []metav1.Condition{},
		},
	}

	result, err := manager.EnsureDomainExists(context.TODO(), endpoint, DomainCheckParams{
		URL:      endpoint.Spec.URL,
		Bindings: endpoint.Spec.Bindings,
	})

	// Internal-bound endpoints should skip domain creation and be marked as ready
	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.True(t, result.IsReady)
	assert.Nil(t, result.Domain)
	assert.Nil(t, endpoint.GetDomainRef())

	// Should set ready condition with internal binding message
	conditions := endpoint.GetConditions()
	assert.Len(t, *conditions, 1)
	assert.Equal(t, ConditionDomainReady, (*conditions)[0].Type)
	assert.Equal(t, metav1.ConditionTrue, (*conditions)[0].Status)
	assert.Contains(t, (*conditions)[0].Message, "internal binding")

	// Verify no Domain CRD was created
	var domains ingressv1alpha1.DomainList
	err = client.List(context.TODO(), &domains)
	assert.NoError(t, err)
	assert.Empty(t, domains.Items)
}

// TestManager_EnsureDomainExists_KubernetesBinding_DeletesStaleDomain tests that kubernetes binding cleans up existing domains
func TestManager_EnsureDomainExists_KubernetesBinding_DeletesStaleDomain(t *testing.T) {
	scheme := setupScheme()

	// Create an existing domain that should be cleaned up
	existingDomain := createReadyDomain("example-com", "default", "example.com")
	client := fake.NewClientBuilder().WithScheme(scheme).WithObjects(existingDomain).Build()
	manager := createTestManager(client)

	// Create endpoint with kubernetes binding but with a stale domainRef
	endpoint := &ngrokv1alpha1.AgentEndpoint{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-endpoint",
			Namespace: "default",
		},
		Spec: ngrokv1alpha1.AgentEndpointSpec{
			URL:      "http://example.com",
			Bindings: []string{"kubernetes"},
		},
		Status: ngrokv1alpha1.AgentEndpointStatus{
			DomainRef: &ngrokv1alpha1.K8sObjectRefOptionalNamespace{
				Name: "example-com",
			},
			Conditions: []metav1.Condition{},
		},
	}

	result, err := manager.EnsureDomainExists(context.TODO(), endpoint, DomainCheckParams{
		URL:      endpoint.Spec.URL,
		Bindings: endpoint.Spec.Bindings,
	})

	// Should skip domain creation and mark as ready
	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.True(t, result.IsReady)
	assert.Nil(t, endpoint.GetDomainRef())

	// Domain should have been deleted
	var domain ingressv1alpha1.Domain
	err = client.Get(context.TODO(), types.NamespacedName{Name: "example-com", Namespace: "default"}, &domain)
	assert.True(t, errors.IsNotFound(err), "Domain should have been deleted")
}

// TestManager_EnsureDomainExists_MixedBindings_SkipsDomain tests that endpoints with mixed bindings (including kubernetes) skip domain creation
func TestManager_EnsureDomainExists_MixedBindings_SkipsDomain(t *testing.T) {
	scheme := setupScheme()
	client := fake.NewClientBuilder().WithScheme(scheme).Build()
	manager := createTestManager(client)

	endpoint := createTestEndpoint("mixed-binding-endpoint", "default", "https://example.com")
	endpoint.Spec.Bindings = []string{"kubernetes", "public"}

	result, err := manager.EnsureDomainExists(context.TODO(), endpoint, DomainCheckParams{
		URL:      endpoint.Spec.URL,
		Bindings: endpoint.Spec.Bindings,
	})

	// Mixed bindings should skip domain creation since it has a Kubernetes binding
	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.True(t, result.IsReady)
	assert.Nil(t, result.Domain)

	// Should NOT set domain ref (nil when kubernetes binding is present)
	domainRef := endpoint.GetDomainRef()
	assert.Nil(t, domainRef)

	// Should set ready condition with Kubernetes binding message
	conditions := endpoint.GetConditions()
	assert.Len(t, *conditions, 1)
	assert.Equal(t, ConditionDomainReady, (*conditions)[0].Type)
	assert.Equal(t, metav1.ConditionTrue, (*conditions)[0].Status)
	assert.Contains(t, (*conditions)[0].Message, "Kubernetes binding")

	// Verify no Domain CRD was created
	var domains ingressv1alpha1.DomainList
	err = client.List(context.TODO(), &domains)
	assert.NoError(t, err)
	assert.Empty(t, domains.Items)
}

// TestEndpointReferencesDomain tests the EndpointReferencesDomain helper function
func TestEndpointReferencesDomain(t *testing.T) {
	testCases := []struct {
		name           string
		endpoint       func() *ngrokv1alpha1.AgentEndpoint
		domain         *ingressv1alpha1.Domain
		expectedResult bool
	}{
		{
			name: "matching name and namespace",
			endpoint: func() *ngrokv1alpha1.AgentEndpoint {
				ep := createTestEndpoint("test-endpoint", "default", "https://example.com")
				ns := "default"
				ep.Status.DomainRef = &ngrokv1alpha1.K8sObjectRefOptionalNamespace{
					Name:      "example-com",
					Namespace: &ns,
				}
				return ep
			},
			domain: &ingressv1alpha1.Domain{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "example-com",
					Namespace: "default",
				},
			},
			expectedResult: true,
		},
		{
			name: "matching name with nil namespace (defaults to same namespace)",
			endpoint: func() *ngrokv1alpha1.AgentEndpoint {
				ep := createTestEndpoint("test-endpoint", "default", "https://example.com")
				ep.Status.DomainRef = &ngrokv1alpha1.K8sObjectRefOptionalNamespace{
					Name:      "example-com",
					Namespace: nil,
				}
				return ep
			},
			domain: &ingressv1alpha1.Domain{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "example-com",
					Namespace: "default",
				},
			},
			expectedResult: true,
		},
		{
			name: "matching name with empty namespace (defaults to same namespace)",
			endpoint: func() *ngrokv1alpha1.AgentEndpoint {
				ep := createTestEndpoint("test-endpoint", "default", "https://example.com")
				emptyNs := ""
				ep.Status.DomainRef = &ngrokv1alpha1.K8sObjectRefOptionalNamespace{
					Name:      "example-com",
					Namespace: &emptyNs,
				}
				return ep
			},
			domain: &ingressv1alpha1.Domain{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "example-com",
					Namespace: "default",
				},
			},
			expectedResult: true,
		},
		{
			name: "different namespace",
			endpoint: func() *ngrokv1alpha1.AgentEndpoint {
				ep := createTestEndpoint("test-endpoint", "default", "https://example.com")
				ns := "other-namespace"
				ep.Status.DomainRef = &ngrokv1alpha1.K8sObjectRefOptionalNamespace{
					Name:      "example-com",
					Namespace: &ns,
				}
				return ep
			},
			domain: &ingressv1alpha1.Domain{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "example-com",
					Namespace: "default",
				},
			},
			expectedResult: false,
		},
		{
			name: "different name",
			endpoint: func() *ngrokv1alpha1.AgentEndpoint {
				ep := createTestEndpoint("test-endpoint", "default", "https://example.com")
				ns := "default"
				ep.Status.DomainRef = &ngrokv1alpha1.K8sObjectRefOptionalNamespace{
					Name:      "different-domain",
					Namespace: &ns,
				}
				return ep
			},
			domain: &ingressv1alpha1.Domain{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "example-com",
					Namespace: "default",
				},
			},
			expectedResult: false,
		},
		{
			name: "nil domain ref",
			endpoint: func() *ngrokv1alpha1.AgentEndpoint {
				return createTestEndpoint("test-endpoint", "default", "https://example.com")
			},
			domain: &ingressv1alpha1.Domain{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "example-com",
					Namespace: "default",
				},
			},
			expectedResult: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := tc.endpoint().GetDomainRef().Matches(tc.domain)
			assert.Equal(t, tc.expectedResult, result)
		})
	}
}

func setupScheme() *runtime.Scheme {
	scheme := runtime.NewScheme()
	_ = ngrokv1alpha1.AddToScheme(scheme)
	_ = ingressv1alpha1.AddToScheme(scheme)
	return scheme
}
