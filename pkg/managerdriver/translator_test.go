package managerdriver

import (
	"encoding/json"
	"testing"

	ngrokv1alpha1 "github.com/ngrok/ngrok-operator/api/ngrok/v1alpha1"
	"github.com/ngrok/ngrok-operator/internal/ir"
	"github.com/ngrok/ngrok-operator/internal/trafficpolicy"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	netv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

func TestBuildInternalAgentEndpoint(t *testing.T) {
	testCases := []struct {
		name             string
		serviceUID       string
		serviceName      string
		namespace        string
		clusterDomain    string
		port             int32
		labels           map[string]string
		metadata         string
		expectedName     string
		expectedURL      string
		expectedUpstream string
	}{
		{
			name:             "Default cluster domain",
			serviceUID:       "abc123",
			serviceName:      "test-service",
			namespace:        "default",
			clusterDomain:    "cluster.local",
			port:             8080,
			labels:           map[string]string{"app": "test"},
			metadata:         "metadata-test",
			expectedName:     "6ca13-test-service-default-cluster.local-8080",
			expectedURL:      "https://6ca13-test-service-default-cluster-local-8080.internal",
			expectedUpstream: "http://test-service.default-cluster.local:8080",
		},
		{
			name:             "Custom cluster domain",
			serviceUID:       "xyz789",
			serviceName:      "another-service",
			namespace:        "custom-namespace",
			clusterDomain:    "custom.domain",
			port:             9090,
			labels:           map[string]string{"env": "prod"},
			metadata:         "prod-metadata",
			expectedName:     "5a464-another-service-custom-namespace-custom.domain-9090",
			expectedURL:      "https://5a464-another-service-custom-namespace-custom-domain-9090.internal",
			expectedUpstream: "http://another-service.custom-namespace-custom.domain:9090",
		},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			result := buildInternalAgentEndpoint(tc.serviceUID, tc.serviceName, tc.namespace, tc.clusterDomain, tc.port, tc.labels, tc.metadata)
			assert.Equal(t, tc.expectedName, result.Name, "unexpected name for test case: %s", tc.name)
			assert.Equal(t, tc.namespace, result.Namespace, "unexpected namespace for test case: %s", tc.name)
			assert.Equal(t, tc.labels, result.Labels, "unexpected labels for test case: %s", tc.name)
			assert.Equal(t, tc.metadata, result.Spec.Metadata, "unexpected metadata for test case: %s", tc.name)
			assert.Equal(t, tc.expectedURL, result.Spec.URL, "unexpected URL for test case: %s", tc.name)
			assert.Equal(t, tc.expectedUpstream, result.Spec.Upstream.URL, "unexpected upstream URL for test case: %s", tc.name)
		})
	}
}

func TestBuildCloudEndpoint(t *testing.T) {
	testCases := []struct {
		name           string
		namespace      string
		hostname       string
		labels         map[string]string
		metadata       string
		expectedName   string
		expectedLabels map[string]string
		expectedMeta   string
	}{
		{
			name:           "Basic setup",
			namespace:      "default",
			hostname:       "cloud-host",
			labels:         map[string]string{"app": "cloud"},
			metadata:       "test-metadata",
			expectedName:   "cloud-host",
			expectedLabels: map[string]string{"app": "cloud"},
			expectedMeta:   "test-metadata",
		},
		{
			name:           "Custom namespace and labels",
			namespace:      "production",
			hostname:       "custom-host",
			labels:         map[string]string{"env": "prod"},
			metadata:       "prod-metadata",
			expectedName:   "custom-host",
			expectedLabels: map[string]string{"env": "prod"},
			expectedMeta:   "prod-metadata",
		},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			result := buildCloudEndpoint(tc.namespace, tc.hostname, tc.labels, tc.metadata)
			assert.Equal(t, tc.expectedName, result.Name, "unexpected name for test case: %s", tc.name)
			assert.Equal(t, tc.namespace, result.Namespace, "unexpected namespace for test case: %s", tc.name)
			assert.Equal(t, tc.expectedLabels, result.Labels, "unexpected labels for test case: %s", tc.name)
			assert.Equal(t, tc.expectedMeta, result.Spec.Metadata, "unexpected metadata for test case: %s", tc.name)
		})
	}
}

func TestInjectEndpointDefaultDestinationTPConfig(t *testing.T) {
	testCases := []struct {
		name                 string
		parentPolicy         *trafficpolicy.TrafficPolicy
		defaultDestination   *ir.IRDestination
		childEndpointCache   map[ir.IRService]*ngrokv1alpha1.AgentEndpoint
		expectedParentPolicy *trafficpolicy.TrafficPolicy
		expectedCacheKeys    []ir.IRService
	}{
		{
			name:                 "No default destination",
			parentPolicy:         &trafficpolicy.TrafficPolicy{},
			defaultDestination:   nil,
			childEndpointCache:   map[ir.IRService]*ngrokv1alpha1.AgentEndpoint{},
			expectedParentPolicy: &trafficpolicy.TrafficPolicy{},
			expectedCacheKeys:    []ir.IRService{},
		},
		{
			name:         "Default destination is a traffic policy",
			parentPolicy: &trafficpolicy.TrafficPolicy{},
			defaultDestination: &ir.IRDestination{
				TrafficPolicy: &trafficpolicy.TrafficPolicy{
					OnHTTPRequest: []trafficpolicy.Rule{{
						Name: "Fallback 404 Rule",
						Actions: []trafficpolicy.Action{{
							Type: "custom-response",
							Config: map[string]interface{}{
								"content": "Fallback 404 page",
								"headers": map[string]string{
									"content-type": "text/plain",
								},
							},
						}},
					}},
				},
			},
			childEndpointCache: map[ir.IRService]*ngrokv1alpha1.AgentEndpoint{},
			expectedParentPolicy: &trafficpolicy.TrafficPolicy{
				OnHTTPRequest: []trafficpolicy.Rule{
					{
						Name: "Fallback 404 Rule",
						Actions: []trafficpolicy.Action{{
							Type: "custom-response",
							Config: map[string]interface{}{
								"content": "Fallback 404 page",
								"headers": map[string]string{
									"content-type": "text/plain",
								},
							},
						}},
					},
				},
			},
			expectedCacheKeys: []ir.IRService{},
		},
		{
			name:         "Default destination is an upstream service",
			parentPolicy: &trafficpolicy.TrafficPolicy{},
			defaultDestination: &ir.IRDestination{
				Upstream: &ir.IRUpstream{
					Service: ir.IRService{
						UID:       "service-uid",
						Namespace: "default",
						Name:      "test-service",
						Port:      8080,
					},
				},
			},
			childEndpointCache: map[ir.IRService]*ngrokv1alpha1.AgentEndpoint{},
			expectedParentPolicy: &trafficpolicy.TrafficPolicy{
				OnHTTPRequest: []trafficpolicy.Rule{{
					Name: "Generated-Route-Default-Backend",
					Actions: []trafficpolicy.Action{{
						Type: "forward-internal",
						Config: map[string]interface{}{
							"url": "https://62d2f-test-service-default-cluster-local-8080.internal",
						},
					}},
				}},
			},
			expectedCacheKeys: []ir.IRService{
				{
					UID:       "service-uid",
					Namespace: "default",
					Name:      "test-service",
					Port:      8080,
				},
			},
		},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			translator := &translator{
				clusterDomain:          "cluster.local",
				managedResourceLabels:  map[string]string{"app": "test"},
				defaultIngressMetadata: "test-metadata",
			}

			translator.injectEndpointDefaultDestinationTPConfig(tc.parentPolicy, tc.defaultDestination, tc.childEndpointCache)

			// Assertions
			assert.Equal(t, tc.expectedParentPolicy, tc.parentPolicy, "unexpected parentPolicy for test case: %s", tc.name)

			cacheKeys := make([]ir.IRService, 0, len(tc.childEndpointCache))
			for key := range tc.childEndpointCache {
				cacheKeys = append(cacheKeys, key)
			}
			assert.ElementsMatch(t, tc.expectedCacheKeys, cacheKeys, "unexpected cache keys for test case: %s", tc.name)
		})
	}
}

func TestBuildEndpointServiceRouteRule(t *testing.T) {
	testCases := []struct {
		name           string
		routeName      string
		url            string
		expectedResult trafficpolicy.Rule
	}{
		{
			name:      "Valid route and URL",
			routeName: "Default-Backend",
			url:       "http://example.com",
			expectedResult: trafficpolicy.Rule{
				Name: "Generated-Route-Default-Backend",
				Actions: []trafficpolicy.Action{
					{
						Type: trafficpolicy.ActionType_ForwardInternal,
						Config: map[string]interface{}{
							"url": "http://example.com",
						},
					},
				},
			},
		},
		{
			name:      "Empty route name",
			routeName: "",
			url:       "http://empty-route-name.com",
			expectedResult: trafficpolicy.Rule{
				Name: "Generated-Route-",
				Actions: []trafficpolicy.Action{
					{
						Type: trafficpolicy.ActionType_ForwardInternal,
						Config: map[string]interface{}{
							"url": "http://empty-route-name.com",
						},
					},
				},
			},
		},
		{
			name:      "Empty URL",
			routeName: "Route-With-Empty-URL",
			url:       "",
			expectedResult: trafficpolicy.Rule{
				Name: "Generated-Route-Route-With-Empty-URL",
				Actions: []trafficpolicy.Action{
					{
						Type: trafficpolicy.ActionType_ForwardInternal,
						Config: map[string]interface{}{
							"url": "",
						},
					},
				},
			},
		},
		{
			name:      "Special characters in route name",
			routeName: "Special_Char!@#",
			url:       "http://special-url.com",
			expectedResult: trafficpolicy.Rule{
				Name: "Generated-Route-Special_Char!@#",
				Actions: []trafficpolicy.Action{
					{
						Type: trafficpolicy.ActionType_ForwardInternal,
						Config: map[string]interface{}{
							"url": "http://special-url.com",
						},
					},
				},
			},
		},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			result := buildEndpointServiceRouteRule(tc.routeName, tc.url)

			assert.Equal(t, tc.expectedResult.Name, result.Name, "unexpected rule name for test case: %s", tc.name)
			assert.Equal(t, len(tc.expectedResult.Actions), len(result.Actions), "unexpected number of actions for test case: %s", tc.name)

			// Check action type and config
			for i, expectedAction := range tc.expectedResult.Actions {
				assert.Equal(t, expectedAction.Type, result.Actions[i].Type, "unexpected action type for test case: %s", tc.name)
				assert.Equal(t, expectedAction.Config, result.Actions[i].Config, "unexpected action config for test case: %s", tc.name)
			}
		})
	}
}

func TestBuildPathMatchExpressionExpressionToTPRule(t *testing.T) {
	testCases := []struct {
		name         string
		path         string
		pathType     netv1.PathType
		expectedExpr string
	}{
		{
			name:         "Exact match path",
			path:         "/exact/path",
			pathType:     netv1.PathTypeExact,
			expectedExpr: `req.url.path == "/exact/path"`,
		},
		{
			name:         "Prefix match path",
			path:         "/prefix/path",
			pathType:     netv1.PathTypePrefix,
			expectedExpr: `req.url.path.startsWith("/prefix/path")`,
		},
		{
			name:         "Default case for unsupported PathType",
			path:         "/unsupported/path",
			pathType:     netv1.PathType("UnsupportedPathType"),
			expectedExpr: `req.url.path.startsWith("/unsupported/path")`,
		},
		{
			name:         "Empty path with Exact match",
			path:         "",
			pathType:     netv1.PathTypeExact,
			expectedExpr: `req.url.path == ""`,
		},
		{
			name:         "Empty path with Prefix match",
			path:         "",
			pathType:     netv1.PathTypePrefix,
			expectedExpr: `req.url.path.startsWith("")`,
		},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			result := buildPathMatchExpressionExpressionToTPRule(tc.path, tc.pathType)
			assert.Equal(t, tc.expectedExpr, result, "unexpected expression for test case: %s", tc.name)
		})
	}
}

func TestIRToEndpoints(t *testing.T) {
	testCases := []struct {
		name             string
		irVHosts         []*ir.IRVirtualHost
		expectedParents  map[types.NamespacedName]*ngrokv1alpha1.CloudEndpoint
		expectedChildren map[types.NamespacedName]*ngrokv1alpha1.AgentEndpoint
	}{
		{
			name: "IRVirtualHost with an upstream service",
			irVHosts: []*ir.IRVirtualHost{
				{
					Namespace: "default",
					Hostname:  "example.com",
					Routes: []*ir.IRRoute{
						{
							Path:     "/foo",
							PathType: netv1.PathTypeExact,
							Destination: &ir.IRDestination{
								Upstream: &ir.IRUpstream{
									Service: ir.IRService{
										UID:       "service-uid-1",
										Name:      "test-service",
										Namespace: "default",
										Port:      8080,
									},
								},
							},
						},
					},
				},
			},
			expectedParents: map[types.NamespacedName]*ngrokv1alpha1.CloudEndpoint{
				{
					Name:      "example.com",
					Namespace: "default",
				}: {
					ObjectMeta: metav1.ObjectMeta{
						Name:      "example.com",
						Namespace: "default",
					},
					Spec: ngrokv1alpha1.CloudEndpointSpec{
						TrafficPolicy: &ngrokv1alpha1.NgrokTrafficPolicySpec{
							Policy: json.RawMessage(`{
								"on_http_request": [
									{
										"name": "Generated-Route-/foo",
										"actions": [
											{
												"type": "forward-internal",
												"config": {
													"url": "https://3fa4b-test-service-default-cluster-local-8080.internal"
												}
											}
										],
										"expressions": [
											"req.url.path == \"/foo\""
										]
									}
								]
							}`),
						},
					},
				},
			},
			expectedChildren: map[types.NamespacedName]*ngrokv1alpha1.AgentEndpoint{
				{
					Name:      "3fa4b-test-service-default-cluster.local-8080",
					Namespace: "default",
				}: {
					ObjectMeta: metav1.ObjectMeta{
						Name:      "3fa4b-test-service-default-cluster.local-8080",
						Namespace: "default",
					},
					Spec: ngrokv1alpha1.AgentEndpointSpec{
						URL: "https://3fa4b-test-service-default-cluster-local-8080.internal",
					},
				},
			},
		},
		{
			name: "IRVirtualHost with service default backend",
			irVHosts: []*ir.IRVirtualHost{
				{
					Namespace: "default",
					Hostname:  "example.com",
					Routes: []*ir.IRRoute{
						{
							Path:     "/foo",
							PathType: netv1.PathTypeExact,
							Destination: &ir.IRDestination{
								Upstream: &ir.IRUpstream{
									Service: ir.IRService{
										UID:       "service-uid-1",
										Name:      "test-service",
										Namespace: "default",
										Port:      8080,
									},
								},
							},
						},
					},
					DefaultDestination: &ir.IRDestination{
						Upstream: &ir.IRUpstream{
							Service: ir.IRService{
								UID:       "default-service-uid",
								Name:      "default-service",
								Namespace: "default",
								Port:      8080,
							},
						},
					},
				},
			},
			expectedParents: map[types.NamespacedName]*ngrokv1alpha1.CloudEndpoint{
				{
					Name:      "example.com",
					Namespace: "default",
				}: {
					ObjectMeta: metav1.ObjectMeta{
						Name:      "example.com",
						Namespace: "default",
					},
					Spec: ngrokv1alpha1.CloudEndpointSpec{
						TrafficPolicy: &ngrokv1alpha1.NgrokTrafficPolicySpec{
							Policy: json.RawMessage(`{
								"on_http_request": [
									{
										"name": "Generated-Route-/foo",
										"actions": [
											{
												"type": "forward-internal",
												"config": {
													"url": "https://3fa4b-test-service-default-cluster-local-8080.internal"
												}
											}
										],
										"expressions": [
											"req.url.path == \"/foo\""
										]
									},
									{
										"name": "Generated-Route-Default-Backend",
										"actions": [
											{
												"type": "forward-internal",
												"config": {
													"url": "https://d542b-default-service-default-cluster-local-8080.internal"
												}
											}
										]
									}
								]
							}`),
						},
					},
				},
			},
			expectedChildren: map[types.NamespacedName]*ngrokv1alpha1.AgentEndpoint{
				{
					Name:      "3fa4b-test-service-default-cluster.local-8080",
					Namespace: "default",
				}: {
					ObjectMeta: metav1.ObjectMeta{
						Name:      "3fa4b-test-service-default-cluster.local-8080",
						Namespace: "default",
					},
					Spec: ngrokv1alpha1.AgentEndpointSpec{
						URL: "https://3fa4b-test-service-default-cluster-local-8080.internal",
					},
				},
				{
					Name:      "d542b-default-service-default-cluster.local-8080",
					Namespace: "default",
				}: {
					ObjectMeta: metav1.ObjectMeta{
						Name:      "d542b-default-service-default-cluster.local-8080",
						Namespace: "default",
					},
					Spec: ngrokv1alpha1.AgentEndpointSpec{
						URL: "https://d542b-default-service-default-cluster-local-8080.internal",
					},
				},
			},
		},
		{
			name: "IRVirtualHost with traffic policy default backend",
			irVHosts: []*ir.IRVirtualHost{
				{
					Namespace: "default",
					Hostname:  "example.com",
					Routes: []*ir.IRRoute{
						{
							Path:     "/foo",
							PathType: netv1.PathTypeExact,
							Destination: &ir.IRDestination{
								Upstream: &ir.IRUpstream{
									Service: ir.IRService{
										UID:       "service-uid-1",
										Name:      "test-service",
										Namespace: "default",
										Port:      8080,
									},
								},
							},
						},
					},
					DefaultDestination: &ir.IRDestination{
						TrafficPolicy: &trafficpolicy.TrafficPolicy{
							OnHTTPRequest: []trafficpolicy.Rule{{
								Name: "Generated-Route-Default-Backend",
								Actions: []trafficpolicy.Action{{
									Type: "custom-response",
									Config: map[string]interface{}{
										"content": "custom response page",
									},
								}},
							}},
						},
					},
				},
			},
			expectedParents: map[types.NamespacedName]*ngrokv1alpha1.CloudEndpoint{
				{
					Name:      "example.com",
					Namespace: "default",
				}: {
					ObjectMeta: metav1.ObjectMeta{
						Name:      "example.com",
						Namespace: "default",
					},
					Spec: ngrokv1alpha1.CloudEndpointSpec{
						TrafficPolicy: &ngrokv1alpha1.NgrokTrafficPolicySpec{
							Policy: json.RawMessage(`{
								"on_http_request": [
									{
										"name": "Generated-Route-/foo",
										"actions": [
											{
												"type": "forward-internal",
												"config": {
													"url": "https://3fa4b-test-service-default-cluster-local-8080.internal"
												}
											}
										],
										"expressions": [
											"req.url.path == \"/foo\""
										]
									},
									{
										"name": "Generated-Route-Default-Backend",
										"actions": [
											{
												"type": "custom-response",
												"config": {
													"content": "custom response page"
												}
											}
										]
									}
								]
							}`),
						},
					},
				},
			},
			expectedChildren: map[types.NamespacedName]*ngrokv1alpha1.AgentEndpoint{
				{
					Name:      "3fa4b-test-service-default-cluster.local-8080",
					Namespace: "default",
				}: {
					ObjectMeta: metav1.ObjectMeta{
						Name:      "3fa4b-test-service-default-cluster.local-8080",
						Namespace: "default",
					},
					Spec: ngrokv1alpha1.AgentEndpointSpec{
						URL: "https://3fa4b-test-service-default-cluster-local-8080.internal",
					},
				},
			},
		},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			translator := &translator{
				clusterDomain:          "cluster.local",
				managedResourceLabels:  map[string]string{"app": "test"},
				defaultIngressMetadata: "test-metadata",
			}

			parents, children := translator.IRToEndpoints(tc.irVHosts)

			// Assert parents
			assert.Equal(t, len(tc.expectedParents), len(parents), "unexpected number of parent endpoints for test case: %s", tc.name)
			for key, expectedParent := range tc.expectedParents {
				actualParent, exists := parents[key]
				require.True(t, exists, "expected parent endpoint does not exist for test case: %s", tc.name)

				// Normalize and compare traffic policy JSON
				var expectedPolicy, actualPolicy map[string]interface{}
				err := json.Unmarshal(expectedParent.Spec.TrafficPolicy.Policy, &expectedPolicy)
				require.NoError(t, err, "failed to unmarshal expected traffic policy for test case: %s", tc.name)

				err = json.Unmarshal(actualParent.Spec.TrafficPolicy.Policy, &actualPolicy)
				require.NoError(t, err, "failed to unmarshal actual traffic policy for test case: %s", tc.name)

				assert.Equal(t, expectedPolicy, actualPolicy, "unexpected traffic policy for parent endpoint in test case: %s", tc.name)
			}

			// Assert children
			assert.Equal(t, len(tc.expectedChildren), len(children), "unexpected number of child endpoints for test case: %s", tc.name)
			for key, expectedChild := range tc.expectedChildren {
				actualChild, exists := children[key]
				require.True(t, exists, "expected child endpoint does not exist for test case: %s. Actual children: %v", tc.name, children)
				assert.Equal(t, expectedChild.Name, actualChild.Name, "unexpected child endpoint name for test case: %s", tc.name)
				assert.Equal(t, expectedChild.Spec.URL, actualChild.Spec.URL, "unexpected child endpoint URL for test case: %s", tc.name)
			}
		})
	}
}

func TestTranslate(t *testing.T) {
}
