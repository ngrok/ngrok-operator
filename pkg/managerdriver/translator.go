package managerdriver

import (
	"encoding/json"
	"fmt"

	"github.com/go-logr/logr"
	ngrokv1alpha1 "github.com/ngrok/ngrok-operator/api/ngrok/v1alpha1"
	"github.com/ngrok/ngrok-operator/internal/ir"
	"github.com/ngrok/ngrok-operator/internal/store"
	"github.com/ngrok/ngrok-operator/internal/trafficpolicy"
	netv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

// Translator is responsible for translating kubernetes resources, first into IR internally, and then translating the IR
// into the desired output resource types. This separates the responsibilities of translation out of the Driver so that it can focus on
// resource storage/updates/deletes
// TODO (Alice): generate mocks for use in tests
type Translator interface {
	Translate() *TranslationResult
}

// translator is the default implementation of the Translator interface
type translator struct {
	log                    logr.Logger
	store                  store.Storer
	managedResourceLabels  map[string]string
	defaultIngressMetadata string
	clusterDomain          string
}

// TranslationResult is the final set of translation output resources
type TranslationResult struct {
	AgentEndpoints map[types.NamespacedName]*ngrokv1alpha1.AgentEndpoint
	CloudEndpoints map[types.NamespacedName]*ngrokv1alpha1.CloudEndpoint
}

// NewTranslator creates a new default Translator
func NewTranslator(log logr.Logger, store store.Storer, managedResourceLabels map[string]string, defaultIngressMetadata string, clusterDomain string) Translator {
	return &translator{
		log:                    log,
		store:                  store,
		managedResourceLabels:  managedResourceLabels,
		defaultIngressMetadata: defaultIngressMetadata,
		clusterDomain:          clusterDomain,
	}
}

// Translate looks up all relevant stored resources and translates them into the desired output types
func (t *translator) Translate() *TranslationResult {
	ingressVHosts := t.ingressesToIR()
	for _, vHost := range ingressVHosts {
		// Sort the routes for each virtual host so they are ordered best match
		vHost.SortRoutes()
	}

	// TODO (Alice): implement Gateway API support for endpoints

	cloudEndpoints, agentEndpoints := t.IRToEndpoints(ingressVHosts)

	return &TranslationResult{
		AgentEndpoints: agentEndpoints,
		CloudEndpoints: cloudEndpoints,
	}
}

// IRToEndpoints converts a set of IRVirtualHosts into CloudEndpoints and AgentEndpoints
func (t *translator) IRToEndpoints(irVHosts []*ir.IRVirtualHost) (parents map[types.NamespacedName]*ngrokv1alpha1.CloudEndpoint, children map[types.NamespacedName]*ngrokv1alpha1.AgentEndpoint) {
	// Setup a cache for the child endpoints as any given backend may be used across several ingresses, etc.
	childEndpointCache := make(map[ir.IRService]*ngrokv1alpha1.AgentEndpoint)

	// No cache is necessary for the parent endpoints as each irVHost already corresponds to a unique hostname
	parentEndpoints := map[types.NamespacedName]*ngrokv1alpha1.CloudEndpoint{}

	for _, irVHost := range irVHosts {
		parentEndpoint := buildCloudEndpoint(
			irVHost.Namespace,
			irVHost.Hostname,
			t.managedResourceLabels,
			t.defaultIngressMetadata,
		)

		// If the resource does not have a user-supplied traffic policy, then create a new one as the base
		parentTrafficPolicy := irVHost.TrafficPolicy
		if parentTrafficPolicy == nil {
			parentTrafficPolicy = trafficpolicy.NewTrafficPolicy()
		}
		for _, irRoute := range irVHost.Routes {
			if irRoute.Destination == nil {
				t.log.Error(fmt.Errorf("generated route does not have a destination"), "skipping endpoint configuration generation for invalid route, other routes will continue to be processed",
					"generated from resources", irVHost.OwningResources,
					"hostname", irVHost.Hostname,
					"path", irRoute.Path,
				)
				continue
			}
			switch {
			case irRoute.Destination.Upstream != nil:
				service := irRoute.Destination.Upstream.Service
				childEndpoint, exists := childEndpointCache[service]
				if !exists {
					childEndpoint = buildInternalAgentEndpoint(
						service.UID,
						service.Name,
						service.Namespace,
						t.clusterDomain,
						service.Port,
						t.managedResourceLabels,
						t.defaultIngressMetadata,
					)
					childEndpointCache[service] = childEndpoint
				}

				// Inject a rule into the traffic policy that will route to the desired upstream on path match for the route
				tpRouteRule := buildEndpointServiceRouteRule(irRoute.Path, childEndpoint.Spec.URL)
				tpRouteRule.Expressions = appendStringUnique(tpRouteRule.Expressions, buildPathMatchExpressionExpressionToTPRule(irRoute.Path, irRoute.PathType))
				parentTrafficPolicy.OnHTTPRequest = append(parentTrafficPolicy.OnHTTPRequest, tpRouteRule)
			case irRoute.Destination.TrafficPolicy != nil:
				routePolicy := irRoute.Destination.TrafficPolicy
				// First, append the route policy rules for the following phases to the parent since we don't generate any config for these phases
				parentTrafficPolicy.OnTCPConnect = append(parentTrafficPolicy.OnTCPConnect, routePolicy.OnTCPConnect...)

				// Add an expresison to all rules on the on_http_request/response phases for the route traffic policy to make sure they only run for this path
				for i, tpRouteRule := range routePolicy.OnHTTPRequest {
					tpRouteRule.Expressions = appendStringUnique(tpRouteRule.Expressions, buildPathMatchExpressionExpressionToTPRule(irRoute.Path, irRoute.PathType))
					routePolicy.OnHTTPRequest[i] = tpRouteRule
				}
				for i, tpRouteRule := range routePolicy.OnHTTPResponse {
					tpRouteRule.Expressions = appendStringUnique(tpRouteRule.Expressions, buildPathMatchExpressionExpressionToTPRule(irRoute.Path, irRoute.PathType))
					routePolicy.OnHTTPResponse[i] = tpRouteRule
				}
				parentTrafficPolicy.OnHTTPRequest = append(parentTrafficPolicy.OnHTTPRequest, routePolicy.OnHTTPRequest...)
				parentTrafficPolicy.OnHTTPResponse = append(parentTrafficPolicy.OnHTTPResponse, routePolicy.OnHTTPResponse...)
			default:
				t.log.Error(fmt.Errorf("generated route does not have a valid backend"), "skipping endpoint configuration generation for invalid route, other routes will continue to be processed",
					"generated from resources", irVHost.OwningResources,
					"hostname", irVHost.Hostname,
					"path", irRoute.Path,
				)
				continue
			}
		}

		t.injectEndpointDefaultDestinationTPConfig(parentTrafficPolicy, irVHost.DefaultDestination, childEndpointCache)

		// Marshal the updated TrafficPolicySpec back to JSON
		parentPolicyJSON, err := json.Marshal(parentTrafficPolicy)
		if err != nil {
			t.log.Error(err, "failed to marshal updated traffic policy for CloudEndpoint generated from Ingress hostname",
				"ingress to endpoint conversion error",
				"hostname", irVHost.Hostname,
			)
			continue
		}

		parentEndpoint.Spec.TrafficPolicy = &ngrokv1alpha1.NgrokTrafficPolicySpec{
			Policy: json.RawMessage(parentPolicyJSON),
		}

		parentEndpoints[types.NamespacedName{
			Name:      parentEndpoint.Name,
			Namespace: parentEndpoint.Namespace,
		}] = parentEndpoint
	}

	// Return all of the generated CloudEndpoints and AgentEndpoints as maps keyed by namespaced name for easier lookup
	childEndpoints := make(map[types.NamespacedName]*ngrokv1alpha1.AgentEndpoint)
	for _, childEndpoint := range childEndpointCache {
		childEndpoints[types.NamespacedName{
			Name:      childEndpoint.Name,
			Namespace: childEndpoint.Namespace,
		}] = childEndpoint
	}

	return parentEndpoints, childEndpoints
}

// buildPathMatchExpressionExpressionToTPRule creates an expression for a traffic policy rule to control path matching
func buildPathMatchExpressionExpressionToTPRule(path string, pathType netv1.PathType) string {
	switch pathType {
	case netv1.PathTypeExact:
		return fmt.Sprintf("req.url.path == \"%s\"", path)
	case netv1.PathTypePrefix:
		fallthrough
	default:
		return fmt.Sprintf("req.url.path.startsWith(\"%s\")", path)
	}
}

// buildEndpointServiceRouteRule constructs a traffic policy rule to route to an internal url that is provided by an internal Agent Endpoint
// which will route any requests it receives to its acompanying upstream service
func buildEndpointServiceRouteRule(routeName string, url string) trafficpolicy.Rule {
	return trafficpolicy.Rule{
		Name: fmt.Sprintf("Generated-Route-%s", routeName),
		Actions: []trafficpolicy.Action{
			{
				Type: trafficpolicy.ActionType_ForwardInternal,
				Config: map[string]interface{}{
					"url": url,
				},
			},
		},
	}
}

// injectEndpointDefaultDestinationTPConfig takes a "parent" traffic policy and will inject configuration into it to handle the case where none of the rules in the "parent"
// traffic policy were matched. The "default destination" can be an upstream service which will recieve all requests that do not match a rule in the parent policy, or another traffic policy
// which will append all of it's rules to the rules in the parent policy.
func (t *translator) injectEndpointDefaultDestinationTPConfig(parentPolicy *trafficpolicy.TrafficPolicy, defaultDestination *ir.IRDestination, childEndpointCache map[ir.IRService]*ngrokv1alpha1.AgentEndpoint) {
	if defaultDestination == nil {
		return
	}

	// If the default backend is a traffic policy, then just append all of it's rules to the parent traffic policy
	if defaultTP := defaultDestination.TrafficPolicy; defaultTP != nil {
		parentPolicy.OnHTTPRequest = append(parentPolicy.OnHTTPRequest, defaultTP.OnHTTPRequest...)
		parentPolicy.OnHTTPResponse = append(parentPolicy.OnHTTPResponse, defaultTP.OnHTTPResponse...)
		parentPolicy.OnTCPConnect = append(parentPolicy.OnTCPConnect, defaultTP.OnTCPConnect...)
		return
	}

	// If the default backend is a service, then add a route for it to the end of the parent traffic policy with no
	// expressions so it matches anything that did not match any prior traffic policy rules
	if upstream := defaultDestination.Upstream; upstream != nil {
		service := upstream.Service
		childEndpoint, exists := childEndpointCache[service]
		if !exists {
			childEndpoint = buildInternalAgentEndpoint(
				service.UID,
				service.Name,
				service.Namespace,
				t.clusterDomain,
				service.Port,
				t.managedResourceLabels,
				t.defaultIngressMetadata,
			)
			childEndpointCache[service] = childEndpoint
		}
		routeRule := buildEndpointServiceRouteRule("Default-Backend", childEndpoint.Spec.URL)
		parentPolicy.OnHTTPRequest = append(parentPolicy.OnHTTPRequest, routeRule)
	}
}

// buildCloudEndpoint initializes a new CloudEndpoint
func buildCloudEndpoint(namespace, hostname string, labels map[string]string, metadata string) *ngrokv1alpha1.CloudEndpoint {
	return &ngrokv1alpha1.CloudEndpoint{
		ObjectMeta: metav1.ObjectMeta{
			Name:      sanitizeStringForK8sName(hostname),
			Namespace: namespace,
			Labels:    labels,
		},
		Spec: ngrokv1alpha1.CloudEndpointSpec{
			URL:      "https://" + hostname,
			Metadata: metadata,
		},
	}
}

// buildInternalAgentEndpoint initializes a new internal AgentEndpoint
func buildInternalAgentEndpoint(serviceUID, serviceName, namespace, clusterDomain string, port int32, labels map[string]string, metadata string) *ngrokv1alpha1.AgentEndpoint {
	return &ngrokv1alpha1.AgentEndpoint{
		ObjectMeta: metav1.ObjectMeta{
			Name:      internalAgentEndpointName(serviceUID, serviceName, namespace, clusterDomain, port),
			Namespace: namespace,
			Labels:    labels,
		},
		Spec: ngrokv1alpha1.AgentEndpointSpec{
			URL:      internalAgentEndpointURL(serviceUID, serviceName, namespace, clusterDomain, port),
			Metadata: metadata,
			Upstream: ngrokv1alpha1.EndpointUpstream{
				URL: internalAgentEndpointUpstreamURL(serviceName, namespace, clusterDomain, port),
			},
		},
	}
}
