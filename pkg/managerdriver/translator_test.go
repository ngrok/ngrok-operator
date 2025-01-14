package managerdriver

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/go-logr/logr"
	common "github.com/ngrok/ngrok-operator/api/common/v1alpha1"
	ingressv1alpha1 "github.com/ngrok/ngrok-operator/api/ingress/v1alpha1"
	ngrokv1alpha1 "github.com/ngrok/ngrok-operator/api/ngrok/v1alpha1"
	"github.com/ngrok/ngrok-operator/internal/ir"
	"github.com/ngrok/ngrok-operator/internal/testutils"
	"github.com/ngrok/ngrok-operator/internal/trafficpolicy"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	v1 "k8s.io/api/core/v1"
	netv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"
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
			name: "IRVirtualHost with a traffic policy backend",
			irVHosts: []*ir.IRVirtualHost{
				{
					Namespace: "default",
					Hostname:  "example.com",
					Routes: []*ir.IRRoute{
						{
							Path:     "/foo",
							PathType: netv1.PathTypeExact,
							Destination: &ir.IRDestination{
								TrafficPolicy: &trafficpolicy.TrafficPolicy{
									OnHTTPRequest: []trafficpolicy.Rule{{
										Name: "Generated-Route-/foo",
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
												"type": "custom-response",
												"config": {
													"content": "custom response page"
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
			expectedChildren: map[types.NamespacedName]*ngrokv1alpha1.AgentEndpoint{},
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
	var driver *Driver
	var scheme = runtime.NewScheme()
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	utilruntime.Must(ingressv1alpha1.AddToScheme(scheme))
	utilruntime.Must(gatewayv1.AddToScheme(scheme))
	utilruntime.Must(ngrokv1alpha1.AddToScheme(scheme))
	logger := logr.New(logr.Discard().GetSink())
	driver = NewDriver(
		logger,
		scheme,
		testutils.DefaultControllerName,
		types.NamespacedName{
			Name:      "test-manager-name",
			Namespace: "test-manager-namespace",
		},
		WithGatewayEnabled(false),
		WithSyncAllowConcurrent(true),
	)

	ic1 := testutils.NewTestIngressClass("test-ingress-class", true, true)
	i1 := testutils.NewTestIngressV1WithClass("ingress-1", "default", ic1.Name)
	i1.Annotations = map[string]string{
		common.AnnotationUseEndpoints:  "true",
		"k8s.ngrok.com/traffic-policy": "annotation-traffic-policy",
	}
	exactMatch := netv1.PathTypeExact
	prefixMatch := netv1.PathTypePrefix
	i1.Spec.DefaultBackend = &netv1.IngressBackend{
		Resource: &v1.TypedLocalObjectReference{
			Kind: "NgrokTrafficPolicy",
			Name: "default-traffic-policy",
		},
	}
	i1.Spec.Rules = []netv1.IngressRule{
		{
			Host: "example.com",
			IngressRuleValue: netv1.IngressRuleValue{
				HTTP: &netv1.HTTPIngressRuleValue{
					Paths: []netv1.HTTPIngressPath{
						{
							Path:     "/foo",
							PathType: &prefixMatch,
							Backend: netv1.IngressBackend{
								Service: &netv1.IngressServiceBackend{
									Name: "test-service-1",
									Port: netv1.ServiceBackendPort{
										Number: 8080,
									},
								},
							},
						},
						{
							Path:     "/foo/bar",
							PathType: &prefixMatch,
							Backend: netv1.IngressBackend{
								Resource: &v1.TypedLocalObjectReference{
									Kind: "NgrokTrafficPolicy",
									Name: "route-traffic-policy",
								},
							},
						},
					},
				},
			},
		},
		{
			Host: "foo.example.com",
			IngressRuleValue: netv1.IngressRuleValue{
				HTTP: &netv1.HTTPIngressRuleValue{
					Paths: []netv1.HTTPIngressPath{
						{
							Path:     "/test-other-hostname",
							PathType: &exactMatch,
							Backend: netv1.IngressBackend{
								Service: &netv1.IngressServiceBackend{
									Name: "test-service-2",
									Port: netv1.ServiceBackendPort{
										Number: 9090,
									},
								},
							},
						},
					},
				},
			},
		},
	}
	i2 := testutils.NewTestIngressV1WithClass("ingress-2", "default", ic1.Name)
	i2.Annotations = map[string]string{
		common.AnnotationUseEndpoints:  "true",
		"k8s.ngrok.com/traffic-policy": "annotation-traffic-policy",
	}
	i2.Spec.DefaultBackend = &netv1.IngressBackend{
		Resource: &v1.TypedLocalObjectReference{
			Kind: "NgrokTrafficPolicy",
			Name: "default-traffic-policy",
		},
	}
	i2.Spec.Rules = []netv1.IngressRule{
		{
			Host: "example.com",
			IngressRuleValue: netv1.IngressRuleValue{
				HTTP: &netv1.HTTPIngressRuleValue{
					Paths: []netv1.HTTPIngressPath{
						{
							Path:     "/test-merging-ingresses",
							PathType: &exactMatch,
							Backend: netv1.IngressBackend{
								Service: &netv1.IngressServiceBackend{
									Name: "test-service-2",
									Port: netv1.ServiceBackendPort{
										Number: 9090,
									},
								},
							},
						},
					},
				},
			},
		},
	}

	i3 := testutils.NewTestIngressV1WithClass("ingress-3-no-endpoints", "default", ic1.Name)
	i3.Spec.Rules = []netv1.IngressRule{
		{
			Host: "no-endpoints.example.com",
			IngressRuleValue: netv1.IngressRuleValue{
				HTTP: &netv1.HTTPIngressRuleValue{
					Paths: []netv1.HTTPIngressPath{
						{
							Path:     "/foo",
							PathType: &exactMatch,
							Backend: netv1.IngressBackend{
								Service: &netv1.IngressServiceBackend{
									Name: "test-service-1",
									Port: netv1.ServiceBackendPort{
										Number: 8080,
									},
								},
							},
						},
					},
				},
			},
		},
	}

	testService1 := &v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-service-1",
			Namespace: "default",
			UID:       "1234",
		},
		Spec: v1.ServiceSpec{
			Ports: []v1.ServicePort{{
				Port: 8080,
			}},
		},
	}
	testService2 := &v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-service-2",
			Namespace: "default",
			UID:       "5678",
		},
		Spec: v1.ServiceSpec{
			Ports: []v1.ServicePort{{
				Port: 9090,
			}},
		},
	}

	routeTrafficPolicyConfig := &trafficpolicy.TrafficPolicy{
		OnHTTPRequest: []trafficpolicy.Rule{{
			Name: "route-tp-rule",
			Actions: []trafficpolicy.Action{{
				Type: "custom-response",
				Config: map[string]interface{}{
					"content": "route page",
				},
			}},
		}},
	}
	routeTrafficPolicyJSON, err := json.Marshal(routeTrafficPolicyConfig)
	require.NoError(t, err)
	routeTrafficPolicy := &ngrokv1alpha1.NgrokTrafficPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "route-traffic-policy",
			Namespace: "default",
		},
		Spec: ngrokv1alpha1.NgrokTrafficPolicySpec{
			Policy: routeTrafficPolicyJSON,
		},
	}

	annotationTrafficPolicyConfig := &trafficpolicy.TrafficPolicy{
		OnHTTPRequest: []trafficpolicy.Rule{{
			Name: "annotation-tp-rule",
			Expressions: []string{
				"req.url.path.startsWith(\"somestring\")",
			},
			Actions: []trafficpolicy.Action{{
				Type: "custom-response",
				Config: map[string]interface{}{
					"content": "annotation page",
				},
			}},
		}},
	}
	annotationTrafficPolicyJSON, err := json.Marshal(annotationTrafficPolicyConfig)
	require.NoError(t, err)
	annotationTrafficPolicy := &ngrokv1alpha1.NgrokTrafficPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "annotation-traffic-policy",
			Namespace: "default",
		},
		Spec: ngrokv1alpha1.NgrokTrafficPolicySpec{
			Policy: annotationTrafficPolicyJSON,
		},
	}

	defaultTrafficPolicyConfig := &trafficpolicy.TrafficPolicy{
		OnHTTPRequest: []trafficpolicy.Rule{{
			Name: "default-tp-rule",
			Actions: []trafficpolicy.Action{{
				Type: "custom-response",
				Config: map[string]interface{}{
					"content": "default page",
				},
			}},
		}},
	}
	defaultTrafficPolicyJSON, err := json.Marshal(defaultTrafficPolicyConfig)
	require.NoError(t, err)
	defaultTrafficPolicy := &ngrokv1alpha1.NgrokTrafficPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "default-traffic-policy",
			Namespace: "default",
		},
		Spec: ngrokv1alpha1.NgrokTrafficPolicySpec{
			Policy: defaultTrafficPolicyJSON,
		},
	}

	obs := []runtime.Object{&ic1, &i1, &i2, &i3, testService1, testService2, routeTrafficPolicy, annotationTrafficPolicy, defaultTrafficPolicy}
	client := fake.NewClientBuilder().WithScheme(scheme).WithRuntimeObjects(obs...).Build()

	require.NoError(t, driver.Seed(context.Background(), client))

	translator := NewTranslator(
		driver.log,
		driver.store,
		driver.defaultManagedResourceLabels(),
		driver.ingressNgrokMetadata,
		"svc.cluster.local",
	)

	// =========== Validations ===============

	result := translator.Translate()
	require.Equal(t, 2, len(result.AgentEndpoints)) // Two upstream services
	require.Equal(t, 2, len(result.CloudEndpoints)) // Two unique hostnames (excluding the one for the ingress not using endpoints)

	aepService1, exists := result.AgentEndpoints[types.NamespacedName{
		Namespace: "default",
		Name:      "03ac6-test-service-1-default-8080",
	}]
	require.True(t, exists)
	require.NotNil(t, aepService1)

	aepService2, exists := result.AgentEndpoints[types.NamespacedName{
		Namespace: "default",
		Name:      "f8638-test-service-2-default-9090",
	}]
	require.True(t, exists)
	require.NotNil(t, aepService2)

	expectedLabels := map[string]string{
		"k8s.ngrok.com/controller-name":      "test-manager-name",
		"k8s.ngrok.com/controller-namespace": "test-manager-namespace",
	}

	assert.Equal(t, "03ac6-test-service-1-default-8080", aepService1.Name)
	assert.Equal(t, "f8638-test-service-2-default-9090", aepService2.Name)
	assert.Equal(t, expectedLabels, aepService1.Labels)
	assert.Equal(t, expectedLabels, aepService2.Labels)
	assert.Equal(t, ngrokv1alpha1.AgentEndpointSpec{
		URL: "https://03ac6-test-service-1-default-8080.internal",
		Upstream: ngrokv1alpha1.EndpointUpstream{
			URL: "http://test-service-1.default:8080",
		},
	}, aepService1.Spec)
	assert.Equal(t, ngrokv1alpha1.AgentEndpointSpec{
		URL: "https://f8638-test-service-2-default-9090.internal",
		Upstream: ngrokv1alpha1.EndpointUpstream{
			URL: "http://test-service-2.default:9090",
		},
	}, aepService2.Spec)

	clep1, exists := result.CloudEndpoints[types.NamespacedName{
		Namespace: "default",
		Name:      "example.com",
	}]
	require.True(t, exists)
	clep2, exists := result.CloudEndpoints[types.NamespacedName{
		Namespace: "default",
		Name:      "foo.example.com",
	}]
	require.True(t, exists)

	assert.Equal(t, "example.com", clep1.Name)
	assert.Equal(t, "foo.example.com", clep2.Name)
	assert.Equal(t, expectedLabels, aepService1.Labels)
	assert.Equal(t, expectedLabels, aepService2.Labels)

	clep1TP := &trafficpolicy.TrafficPolicy{}
	clep1JSON, err := json.Marshal(clep1.Spec.TrafficPolicy.Policy)
	require.NoError(t, err)
	err = json.Unmarshal(clep1JSON, clep1TP)
	require.NoError(t, err)

	clep2TP := &trafficpolicy.TrafficPolicy{}
	clep2JSON, err := json.Marshal(clep2.Spec.TrafficPolicy.Policy)
	require.NoError(t, err)
	err = json.Unmarshal(clep2JSON, clep2TP)
	require.NoError(t, err)

	expectedCLEP1Policy := &trafficpolicy.TrafficPolicy{
		OnHTTPRequest: []trafficpolicy.Rule{
			{
				Name:        "annotation-tp-rule",
				Expressions: []string{"req.url.path.startsWith(\"somestring\")"},
				Actions: []trafficpolicy.Action{{
					Type: trafficpolicy.ActionType_CustomResponse,
					Config: map[string]interface{}{
						"content": "annotation page",
					},
				}},
			},
			{
				Name:        "Generated-Route-/test-merging-ingresses",
				Expressions: []string{"req.url.path == \"/test-merging-ingresses\""},
				Actions: []trafficpolicy.Action{{
					Type: trafficpolicy.ActionType_ForwardInternal,
					Config: map[string]interface{}{
						"url": "https://f8638-test-service-2-default-9090.internal",
					},
				}},
			},
			{
				Name:        "route-tp-rule",
				Expressions: []string{"req.url.path.startsWith(\"/foo/bar\")"},
				Actions: []trafficpolicy.Action{{
					Type: trafficpolicy.ActionType_CustomResponse,
					Config: map[string]interface{}{
						"content": "route page",
					},
				}},
			},
			{
				Name:        "Generated-Route-/foo",
				Expressions: []string{"req.url.path.startsWith(\"/foo\")"},
				Actions: []trafficpolicy.Action{{
					Type: trafficpolicy.ActionType_ForwardInternal,
					Config: map[string]interface{}{
						"url": "https://03ac6-test-service-1-default-8080.internal",
					},
				}},
			},
			{
				Name: "default-tp-rule",
				Actions: []trafficpolicy.Action{{
					Type: trafficpolicy.ActionType_CustomResponse,
					Config: map[string]interface{}{
						"content": "default page",
					},
				}},
			},
		},
	}
	assert.Equal(t, expectedCLEP1Policy, clep1TP)

	expectedCLEP2Policy := &trafficpolicy.TrafficPolicy{
		OnHTTPRequest: []trafficpolicy.Rule{
			{
				Name:        "annotation-tp-rule",
				Expressions: []string{"req.url.path.startsWith(\"somestring\")"},
				Actions: []trafficpolicy.Action{{
					Type: trafficpolicy.ActionType_CustomResponse,
					Config: map[string]interface{}{
						"content": "annotation page",
					},
				}},
			},
			{
				Name:        "Generated-Route-/test-other-hostname",
				Expressions: []string{"req.url.path == \"/test-other-hostname\""},
				Actions: []trafficpolicy.Action{{
					Type: trafficpolicy.ActionType_ForwardInternal,
					Config: map[string]interface{}{
						"url": "https://f8638-test-service-2-default-9090.internal",
					},
				}},
			},
			{
				Name: "default-tp-rule",
				Actions: []trafficpolicy.Action{{
					Type: trafficpolicy.ActionType_CustomResponse,
					Config: map[string]interface{}{
						"content": "default page",
					},
				}},
			},
		},
	}
	assert.Equal(t, expectedCLEP2Policy, clep2TP)
}
