package domain

import (
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
	"github.com/ngrok/ngrok-operator/internal/controller/labels"
)

// Test setup helpers

func setupScheme() *runtime.Scheme {
	scheme := runtime.NewScheme()
	_ = ngrokv1alpha1.AddToScheme(scheme)
	_ = ingressv1alpha1.AddToScheme(scheme)
	return scheme
}

func newTestManager(t *testing.T, objs ...client.Object) (*Manager, client.Client) {
	t.Helper()
	scheme := setupScheme()
	c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(objs...).Build()
	m, err := NewManager(c, record.NewFakeRecorder(10),
		WithControllerLabels(
			labels.ControllerLabelValues{
				Namespace: "test-namespace",
				Name:      "test-controller",
			},
		),
	)
	require.NoError(t, err)
	return m, c
}

func newTestManagerWithOpts(t *testing.T, opts []ManagerOption, objs ...client.Object) (*Manager, client.Client) {
	t.Helper()
	m, c := newTestManager(t, objs...)
	for _, opt := range opts {
		opt(m)
	}
	return m, c
}

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
			ID: "domain-123",
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

// Assertion helpers

func assertDomainCondition(t *testing.T, ep ngrokv1alpha1.EndpointWithDomain, status metav1.ConditionStatus, msgContains string) {
	t.Helper()
	conds := ep.GetConditions()
	require.Len(t, *conds, 1)
	c := (*conds)[0]
	assert.Equal(t, ConditionDomainReady, c.Type)
	assert.Equal(t, status, c.Status)
	if msgContains != "" {
		assert.Contains(t, c.Message, msgContains)
	}
}

func assertDomainRef(t *testing.T, ep ngrokv1alpha1.EndpointWithDomain, wantName, wantNamespace string) {
	t.Helper()
	ref := ep.GetDomainRef()
	require.NotNil(t, ref)
	assert.Equal(t, wantName, ref.Name)
	if wantNamespace != "" {
		require.NotNil(t, ref.Namespace)
		assert.Equal(t, wantNamespace, *ref.Namespace)
	}
}

func assertNoDomainRef(t *testing.T, ep ngrokv1alpha1.EndpointWithDomain) {
	t.Helper()
	assert.Nil(t, ep.GetDomainRef())
}

func assertNoDomainCreated(t *testing.T, c client.Client) {
	t.Helper()
	var domains ingressv1alpha1.DomainList
	require.NoError(t, c.List(t.Context(), &domains))
	assert.Empty(t, domains.Items)
}

func assertDomainLabels(t *testing.T, c client.Client, name, namespace string, expectedLabels map[string]string) {
	t.Helper()
	var domain ingressv1alpha1.Domain
	require.NoError(t, c.Get(t.Context(), types.NamespacedName{Name: name, Namespace: namespace}, &domain))
	domainLabels := domain.GetLabels()
	for k, v := range expectedLabels {
		assert.Equal(t, v, domainLabels[k])
	}
}

// Tests

func TestManager_EnsureDomainExists_SpecialURLs(t *testing.T) {
	tests := []struct {
		name          string
		url           string
		expectErr     bool
		expectReady   bool
		expectNilDom  bool
		condStatus    metav1.ConditionStatus
		condMsgSubstr string
	}{
		{
			name:         "tcp domain is skipped and ready",
			url:          "tcp://1.tcp.ngrok.io:12345",
			expectReady:  true,
			expectNilDom: true,
			condStatus:   metav1.ConditionTrue,
		},
		{
			name:         "tcp custom domain is skipped and ready",
			url:          "tcp://custom.example.com:8080",
			expectReady:  true,
			expectNilDom: true,
			condStatus:   metav1.ConditionTrue,
		},
		{
			name:         "tcp custom domain with non-standard port is skipped and ready",
			url:          "tcp://game-server.mycompany.io:25565",
			expectReady:  true,
			expectNilDom: true,
			condStatus:   metav1.ConditionTrue,
		},
		{
			name:         "tcp with specific IP is skipped and ready",
			url:          "tcp://192.168.1.100:12345",
			expectReady:  true,
			expectNilDom: true,
			condStatus:   metav1.ConditionTrue,
		},
		{
			name:         "internal domain is skipped and ready",
			url:          "https://api.service.internal",
			expectReady:  true,
			expectNilDom: true,
			condStatus:   metav1.ConditionTrue,
		},
		{
			name:          "invalid URL returns error",
			url:           "://invalid-url",
			expectErr:     true,
			expectNilDom:  true,
			condStatus:    metav1.ConditionFalse,
			condMsgSubstr: "failed to parse URL",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			manager, _ := newTestManager(t)
			endpoint := createTestEndpoint("test-endpoint", "default", tt.url)

			result, err := manager.EnsureDomainExists(t.Context(), endpoint)

			if tt.expectErr {
				require.Error(t, err)
				assert.Nil(t, result)
			} else {
				require.NoError(t, err)
				require.NotNil(t, result)
				assert.Equal(t, tt.expectReady, result.IsReady)
				if tt.expectNilDom {
					assert.Nil(t, result.Domain)
				}
			}
			assertNoDomainRef(t, endpoint)
			assertDomainCondition(t, endpoint, tt.condStatus, tt.condMsgSubstr)
		})
	}
}

func TestManager_EnsureDomainExists_ExistingDomainReady(t *testing.T) {
	existingDomain := createReadyDomain("example-com", "default", "example.com")
	manager, _ := newTestManager(t, existingDomain)
	endpoint := createTestEndpoint("test-endpoint", "default", "https://example.com")

	result, err := manager.EnsureDomainExists(t.Context(), endpoint)

	require.NoError(t, err)
	require.NotNil(t, result)
	assert.True(t, result.IsReady)
	assert.Equal(t, existingDomain.Name, result.Domain.Name)
	assertDomainRef(t, endpoint, "example-com", "default")
	assertDomainCondition(t, endpoint, metav1.ConditionTrue, "")
}

func TestManager_EnsureDomainExists_ExistingDomainNotReady(t *testing.T) {
	existingDomain := createNotReadyDomain("example-com", "default", "example.com")
	manager, _ := newTestManager(t, existingDomain)
	endpoint := createTestEndpoint("test-endpoint", "default", "https://example.com")

	result, err := manager.EnsureDomainExists(t.Context(), endpoint)

	require.NoError(t, err)
	require.NotNil(t, result)
	assert.False(t, result.IsReady)
	assert.Equal(t, existingDomain.Name, result.Domain.Name)
	assertDomainRef(t, endpoint, "example-com", "default")
	assertDomainCondition(t, endpoint, metav1.ConditionFalse, "")
}

func TestManager_EnsureDomainExists_ExistingDomainNoReadyCondition(t *testing.T) {
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
	manager, _ := newTestManager(t, existingDomain)
	endpoint := createTestEndpoint("test-endpoint", "default", "https://example.com")

	result, err := manager.EnsureDomainExists(t.Context(), endpoint)

	require.NoError(t, err)
	require.NotNil(t, result)
	assert.False(t, result.IsReady)
	assert.Equal(t, existingDomain.Name, result.Domain.Name)
	assertDomainRef(t, endpoint, "example-com", "default")
	assertDomainCondition(t, endpoint, metav1.ConditionFalse, "")
}

func TestManager_EnsureDomainExists_CreateNewDomain(t *testing.T) {
	manager, c := newTestManager(t)
	endpoint := createTestEndpoint("test-endpoint", "default", "https://example.com")

	result, err := manager.EnsureDomainExists(t.Context(), endpoint)

	require.NoError(t, err)
	require.NotNil(t, result)
	assert.False(t, result.IsReady)
	assert.Equal(t, "example-com", result.Domain.Name)
	assert.Equal(t, "example.com", result.Domain.Spec.Domain)
	assertDomainRef(t, endpoint, "example-com", "default")
	assertDomainCondition(t, endpoint, metav1.ConditionFalse, "")

	var createdDomain ingressv1alpha1.Domain
	require.NoError(t, c.Get(t.Context(), types.NamespacedName{Name: "example-com", Namespace: "default"}, &createdDomain))
	assert.Equal(t, "example.com", createdDomain.Spec.Domain)
	assertDomainLabels(t, c, "example-com", "default", map[string]string{
		labels.ControllerNamespace: "test-namespace",
		labels.ControllerName:      "test-controller",
	})
}

func TestManager_EnsureDomainExists_CreateNewDomainWithReclaimPolicy(t *testing.T) {
	reclaimPolicy := ingressv1alpha1.DomainReclaimPolicyRetain
	manager, c := newTestManagerWithOpts(t, []ManagerOption{WithDefaultDomainReclaimPolicy(reclaimPolicy)})
	endpoint := createTestEndpoint("test-endpoint", "default", "https://example.com")

	result, err := manager.EnsureDomainExists(t.Context(), endpoint)

	require.NoError(t, err)
	require.NotNil(t, result)
	assert.False(t, result.IsReady)

	var createdDomain ingressv1alpha1.Domain
	require.NoError(t, c.Get(t.Context(), types.NamespacedName{Name: "example-com", Namespace: "default"}, &createdDomain))
	assert.Equal(t, ingressv1alpha1.DomainReclaimPolicyRetain, createdDomain.Spec.ReclaimPolicy)
}

func TestManager_setDomainCondition(t *testing.T) {
	manager, _ := newTestManager(t)
	endpoint := createTestEndpoint("test-endpoint", "default", "https://example.com")

	manager.setDomainCondition(endpoint, true, "TestReason", "Test message")

	conditions := endpoint.GetConditions()
	require.Len(t, *conditions, 1)
	c := (*conditions)[0]
	assert.Equal(t, ConditionDomainReady, c.Type)
	assert.Equal(t, metav1.ConditionTrue, c.Status)
	assert.Equal(t, "TestReason", c.Reason)
	assert.Equal(t, "Test message", c.Message)
	assert.Equal(t, int64(0), c.ObservedGeneration)

	manager.setDomainCondition(endpoint, false, "TestReason2", "Test message 2")

	conditions = endpoint.GetConditions()
	require.Len(t, *conditions, 1)
	c = (*conditions)[0]
	assert.Equal(t, metav1.ConditionFalse, c.Status)
	assert.Equal(t, "TestReason2", c.Reason)
	assert.Equal(t, "Test message 2", c.Message)
}

func TestManager_EnsureDomainExists_SkipsKubernetesBinding(t *testing.T) {
	tests := []struct {
		name     string
		endpoint ngrokv1alpha1.EndpointWithDomain
	}{
		{
			name: "AgentEndpoint",
			endpoint: &ngrokv1alpha1.AgentEndpoint{
				ObjectMeta: metav1.ObjectMeta{Name: "k8s-bound-endpoint", Namespace: "default"},
				Spec:       ngrokv1alpha1.AgentEndpointSpec{URL: "http://aws.demo", Bindings: []string{"kubernetes"}},
				Status:     ngrokv1alpha1.AgentEndpointStatus{Conditions: []metav1.Condition{}},
			},
		},
		{
			name: "CloudEndpoint",
			endpoint: &ngrokv1alpha1.CloudEndpoint{
				ObjectMeta: metav1.ObjectMeta{Name: "k8s-bound-endpoint", Namespace: "default"},
				Spec:       ngrokv1alpha1.CloudEndpointSpec{URL: "http://aws.demo", Bindings: []string{"kubernetes"}},
				Status:     ngrokv1alpha1.CloudEndpointStatus{Conditions: []metav1.Condition{}},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			manager, c := newTestManager(t)

			result, err := manager.EnsureDomainExists(t.Context(), tt.endpoint)

			require.NoError(t, err)
			require.NotNil(t, result)
			assert.True(t, result.IsReady)
			assert.Nil(t, result.Domain)
			assertNoDomainRef(t, tt.endpoint)
			assertDomainCondition(t, tt.endpoint, metav1.ConditionTrue, "Kubernetes binding")
			assertNoDomainCreated(t, c)
		})
	}
}

func TestManager_EnsureDomainExists_SkipsInternalBinding(t *testing.T) {
	tests := []struct {
		name     string
		endpoint ngrokv1alpha1.EndpointWithDomain
	}{
		{
			name: "AgentEndpoint",
			endpoint: &ngrokv1alpha1.AgentEndpoint{
				ObjectMeta: metav1.ObjectMeta{Name: "internal-bound-endpoint", Namespace: "default"},
				Spec:       ngrokv1alpha1.AgentEndpointSpec{URL: "http://internal.demo", Bindings: []string{"internal"}},
				Status:     ngrokv1alpha1.AgentEndpointStatus{Conditions: []metav1.Condition{}},
			},
		},
		{
			name: "CloudEndpoint",
			endpoint: &ngrokv1alpha1.CloudEndpoint{
				ObjectMeta: metav1.ObjectMeta{Name: "internal-bound-endpoint", Namespace: "default"},
				Spec:       ngrokv1alpha1.CloudEndpointSpec{URL: "http://internal.demo", Bindings: []string{"internal"}},
				Status:     ngrokv1alpha1.CloudEndpointStatus{Conditions: []metav1.Condition{}},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			manager, c := newTestManager(t)

			result, err := manager.EnsureDomainExists(t.Context(), tt.endpoint)

			require.NoError(t, err)
			require.NotNil(t, result)
			assert.True(t, result.IsReady)
			assert.Nil(t, result.Domain)
			assertNoDomainRef(t, tt.endpoint)
			assertDomainCondition(t, tt.endpoint, metav1.ConditionTrue, "internal binding")
			assertNoDomainCreated(t, c)
		})
	}
}

func TestManager_EnsureDomainExists_SkipsTCPURLs(t *testing.T) {
	tests := []struct {
		name     string
		endpoint ngrokv1alpha1.EndpointWithDomain
	}{
		{
			name: "AgentEndpoint with tcp ngrok URL",
			endpoint: &ngrokv1alpha1.AgentEndpoint{
				ObjectMeta: metav1.ObjectMeta{Name: "tcp-endpoint", Namespace: "default"},
				Spec:       ngrokv1alpha1.AgentEndpointSpec{URL: "tcp://1.tcp.ngrok.io:12345"},
				Status:     ngrokv1alpha1.AgentEndpointStatus{Conditions: []metav1.Condition{}},
			},
		},
		{
			name: "AgentEndpoint with custom tcp URL",
			endpoint: &ngrokv1alpha1.AgentEndpoint{
				ObjectMeta: metav1.ObjectMeta{Name: "tcp-custom-endpoint", Namespace: "default"},
				Spec:       ngrokv1alpha1.AgentEndpointSpec{URL: "tcp://custom.example.com:8080"},
				Status:     ngrokv1alpha1.AgentEndpointStatus{Conditions: []metav1.Condition{}},
			},
		},
		{
			name: "CloudEndpoint with tcp ngrok URL",
			endpoint: &ngrokv1alpha1.CloudEndpoint{
				ObjectMeta: metav1.ObjectMeta{Name: "tcp-endpoint", Namespace: "default"},
				Spec:       ngrokv1alpha1.CloudEndpointSpec{URL: "tcp://1.tcp.ngrok.io:12345"},
				Status:     ngrokv1alpha1.CloudEndpointStatus{Conditions: []metav1.Condition{}},
			},
		},
		{
			name: "CloudEndpoint with custom tcp URL",
			endpoint: &ngrokv1alpha1.CloudEndpoint{
				ObjectMeta: metav1.ObjectMeta{Name: "tcp-custom-endpoint", Namespace: "default"},
				Spec:       ngrokv1alpha1.CloudEndpointSpec{URL: "tcp://custom.example.com:8080"},
				Status:     ngrokv1alpha1.CloudEndpointStatus{Conditions: []metav1.Condition{}},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			manager, c := newTestManager(t)

			result, err := manager.EnsureDomainExists(t.Context(), tt.endpoint)

			require.NoError(t, err)
			require.NotNil(t, result)
			assert.True(t, result.IsReady)
			assert.Nil(t, result.Domain)
			assertNoDomainRef(t, tt.endpoint)
			assertDomainCondition(t, tt.endpoint, metav1.ConditionTrue, "TCP")
			assertNoDomainCreated(t, c)
		})
	}
}

func TestManager_EnsureDomainExists_KubernetesBinding_DeletesStaleDomain(t *testing.T) {
	existingDomain := createReadyDomain("example-com", "default", "example.com")
	manager, c := newTestManager(t, existingDomain)

	endpoint := &ngrokv1alpha1.AgentEndpoint{
		ObjectMeta: metav1.ObjectMeta{Name: "test-endpoint", Namespace: "default"},
		Spec:       ngrokv1alpha1.AgentEndpointSpec{URL: "http://example.com", Bindings: []string{"kubernetes"}},
		Status: ngrokv1alpha1.AgentEndpointStatus{
			DomainRef:  &ngrokv1alpha1.K8sObjectRefOptionalNamespace{Name: "example-com"},
			Conditions: []metav1.Condition{},
		},
	}

	result, err := manager.EnsureDomainExists(t.Context(), endpoint)

	require.NoError(t, err)
	require.NotNil(t, result)
	assert.True(t, result.IsReady)
	assertNoDomainRef(t, endpoint)

	var domain ingressv1alpha1.Domain
	err = c.Get(t.Context(), types.NamespacedName{Name: "example-com", Namespace: "default"}, &domain)
	assert.True(t, errors.IsNotFound(err), "Domain should have been deleted")
}

func TestManager_EnsureDomainExists_MixedBindings_SkipsDomain(t *testing.T) {
	manager, c := newTestManager(t)
	endpoint := createTestEndpoint("mixed-binding-endpoint", "default", "https://example.com")
	endpoint.Spec.Bindings = []string{"kubernetes", "public"}

	result, err := manager.EnsureDomainExists(t.Context(), endpoint)

	require.NoError(t, err)
	require.NotNil(t, result)
	assert.True(t, result.IsReady)
	assert.Nil(t, result.Domain)
	assertNoDomainRef(t, endpoint)
	assertDomainCondition(t, endpoint, metav1.ConditionTrue, "Kubernetes binding")
	assertNoDomainCreated(t, c)
}

func TestEndpointReferencesDomain(t *testing.T) {
	tests := []struct {
		name     string
		endpoint func() *ngrokv1alpha1.AgentEndpoint
		domain   *ingressv1alpha1.Domain
		want     bool
	}{
		{
			name: "matching name and namespace",
			endpoint: func() *ngrokv1alpha1.AgentEndpoint {
				ep := createTestEndpoint("test-endpoint", "default", "https://example.com")
				ns := "default"
				ep.Status.DomainRef = &ngrokv1alpha1.K8sObjectRefOptionalNamespace{Name: "example-com", Namespace: &ns}
				return ep
			},
			domain: &ingressv1alpha1.Domain{ObjectMeta: metav1.ObjectMeta{Name: "example-com", Namespace: "default"}},
			want:   true,
		},
		{
			name: "matching name with nil namespace",
			endpoint: func() *ngrokv1alpha1.AgentEndpoint {
				ep := createTestEndpoint("test-endpoint", "default", "https://example.com")
				ep.Status.DomainRef = &ngrokv1alpha1.K8sObjectRefOptionalNamespace{Name: "example-com", Namespace: nil}
				return ep
			},
			domain: &ingressv1alpha1.Domain{ObjectMeta: metav1.ObjectMeta{Name: "example-com", Namespace: "default"}},
			want:   true,
		},
		{
			name: "matching name with empty namespace",
			endpoint: func() *ngrokv1alpha1.AgentEndpoint {
				ep := createTestEndpoint("test-endpoint", "default", "https://example.com")
				emptyNs := ""
				ep.Status.DomainRef = &ngrokv1alpha1.K8sObjectRefOptionalNamespace{Name: "example-com", Namespace: &emptyNs}
				return ep
			},
			domain: &ingressv1alpha1.Domain{ObjectMeta: metav1.ObjectMeta{Name: "example-com", Namespace: "default"}},
			want:   true,
		},
		{
			name: "different namespace",
			endpoint: func() *ngrokv1alpha1.AgentEndpoint {
				ep := createTestEndpoint("test-endpoint", "default", "https://example.com")
				ns := "other-namespace"
				ep.Status.DomainRef = &ngrokv1alpha1.K8sObjectRefOptionalNamespace{Name: "example-com", Namespace: &ns}
				return ep
			},
			domain: &ingressv1alpha1.Domain{ObjectMeta: metav1.ObjectMeta{Name: "example-com", Namespace: "default"}},
			want:   false,
		},
		{
			name: "different name",
			endpoint: func() *ngrokv1alpha1.AgentEndpoint {
				ep := createTestEndpoint("test-endpoint", "default", "https://example.com")
				ns := "default"
				ep.Status.DomainRef = &ngrokv1alpha1.K8sObjectRefOptionalNamespace{Name: "different-domain", Namespace: &ns}
				return ep
			},
			domain: &ingressv1alpha1.Domain{ObjectMeta: metav1.ObjectMeta{Name: "example-com", Namespace: "default"}},
			want:   false,
		},
		{
			name: "nil domain ref",
			endpoint: func() *ngrokv1alpha1.AgentEndpoint {
				return createTestEndpoint("test-endpoint", "default", "https://example.com")
			},
			domain: &ingressv1alpha1.Domain{ObjectMeta: metav1.ObjectMeta{Name: "example-com", Namespace: "default"}},
			want:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.endpoint().GetDomainRef().Matches(tt.domain)
			assert.Equal(t, tt.want, result)
		})
	}
}

func TestManager_EnsureDomainExists_ControllerLabels(t *testing.T) {
	tests := []struct {
		name           string
		existingLabels map[string]string
		wantLabels     map[string]string
	}{
		{
			name:           "adds labels to domain without labels",
			existingLabels: nil,
			wantLabels: map[string]string{
				labels.ControllerNamespace: "test-namespace",
				labels.ControllerName:      "test-controller",
			},
		},
		{
			name: "preserves existing controller labels",
			existingLabels: map[string]string{
				labels.ControllerNamespace: "test-namespace",
				labels.ControllerName:      "test-controller",
				"custom-label":             "custom-value",
			},
			wantLabels: map[string]string{
				labels.ControllerNamespace: "test-namespace",
				labels.ControllerName:      "test-controller",
				"custom-label":             "custom-value",
			},
		},
		{
			name: "adds controller labels preserving custom labels",
			existingLabels: map[string]string{
				"custom-label": "custom-value",
			},
			wantLabels: map[string]string{
				labels.ControllerNamespace: "test-namespace",
				labels.ControllerName:      "test-controller",
				"custom-label":             "custom-value",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			existingDomain := createReadyDomain("example-com", "default", "example.com")
			if tt.existingLabels != nil {
				existingDomain.SetLabels(tt.existingLabels)
			}
			manager, c := newTestManager(t, existingDomain)
			endpoint := createTestEndpoint("test-endpoint", "default", "https://example.com")

			result, err := manager.EnsureDomainExists(t.Context(), endpoint)

			require.NoError(t, err)
			require.NotNil(t, result)
			assert.True(t, result.IsReady)
			assertDomainLabels(t, c, "example-com", "default", tt.wantLabels)
		})
	}
}
