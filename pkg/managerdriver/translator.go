package managerdriver

import (
	"encoding/json"
	"fmt"

	"github.com/go-logr/logr"
	ngrokv1alpha1 "github.com/ngrok/ngrok-operator/api/ngrok/v1alpha1"
	"github.com/ngrok/ngrok-operator/internal/ir"
	"github.com/ngrok/ngrok-operator/internal/store"
	"github.com/ngrok/ngrok-operator/internal/trafficpolicy"
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
	defaultGatewayMetadata string
	clusterDomain          string

	// We give users the ability to opt-out of requiring ReferenceGrants for cross namespace
	// references when using Gateway API
	disableGatewayReferenceGrants bool
}

// TranslationResult is the final set of translation output resources
type TranslationResult struct {
	AgentEndpoints map[types.NamespacedName]*ngrokv1alpha1.AgentEndpoint
	CloudEndpoints map[types.NamespacedName]*ngrokv1alpha1.CloudEndpoint
}

// NewTranslator creates a new default Translator
func NewTranslator(
	log logr.Logger,
	store store.Storer,
	managedResourceLabels map[string]string,
	defaultIngressMetadata string,
	defaultGatewayMetadata string,
	clusterDomain string,
	disableGatewayReferenceGrants bool,
) Translator {
	return &translator{
		log:                           log,
		store:                         store,
		managedResourceLabels:         managedResourceLabels,
		defaultIngressMetadata:        defaultIngressMetadata,
		defaultGatewayMetadata:        defaultGatewayMetadata,
		clusterDomain:                 clusterDomain,
		disableGatewayReferenceGrants: disableGatewayReferenceGrants,
	}
}

// Translate looks up all relevant stored resources and translates them into the desired output types
func (t *translator) Translate() *TranslationResult {
	ingressVirtualHosts := t.ingressesToIR()
	gatewayVirtualHosts := t.gatewayAPIToIR()

	virtualHosts := make([]*ir.IRVirtualHost, 0, len(ingressVirtualHosts)+len(gatewayVirtualHosts))
	for _, irVHost := range ingressVirtualHosts {
		irVHost.SortRoutes()
		virtualHosts = append(virtualHosts, irVHost)
	}
	for _, irVHost := range gatewayVirtualHosts {
		irVHost.SortRoutes()
		virtualHosts = append(virtualHosts, irVHost)
	}

	cloudEndpoints, agentEndpoints := t.IRToEndpoints(virtualHosts)

	return &TranslationResult{
		AgentEndpoints: agentEndpoints,
		CloudEndpoints: cloudEndpoints,
	}
}

// IRToEndpoints converts a set of IRVirtualHosts into CloudEndpoints and AgentEndpoints
func (t *translator) IRToEndpoints(irVHosts []*ir.IRVirtualHost) (parents map[types.NamespacedName]*ngrokv1alpha1.CloudEndpoint, children map[types.NamespacedName]*ngrokv1alpha1.AgentEndpoint) {
	// Setup a cache for the child endpoints as any given backend may be used across several ingresses, etc.
	childEndpointCache := make(map[ir.IRServiceKey]*ngrokv1alpha1.AgentEndpoint)
	parentEndpoints := map[types.NamespacedName]*ngrokv1alpha1.CloudEndpoint{}

	for _, irVHost := range irVHosts {
		parentEndpoint := buildCloudEndpoint(irVHost)

		if irVHost.TrafficPolicy == nil && len(irVHost.Routes) == 0 {
			t.log.Error(fmt.Errorf("skipping generating endpoints for hostname with no valid traffic policy or routes"),
				"hostname", string(irVHost.Listener.Hostname),
				"generated from resources", irVHost.OwningResources,
			)
			continue
		}

		// If the resource does not have a user-supplied traffic policy, then create a new one as the base
		parentTrafficPolicy := irVHost.TrafficPolicy
		if parentTrafficPolicy == nil {
			parentTrafficPolicy = trafficpolicy.NewTrafficPolicy()
		}

		if irTLSCfg := irVHost.TLSTermination; irTLSCfg != nil {
			tlsCfg := map[string]interface{}{}

			if len(irTLSCfg.MutualTLSCertificateAuthorities) > 0 {
				tlsCfg["mutual_tls_certificate_authorities"] = irTLSCfg.MutualTLSCertificateAuthorities
			}

			if irTLSCfg.ServerCertificate != nil {
				tlsCfg["server_certificate"] = *irTLSCfg.ServerCertificate
			}

			if irTLSCfg.ServerPrivateKey != nil {
				tlsCfg["server_private_key"] = *irTLSCfg.ServerPrivateKey
			}

			for key, val := range irTLSCfg.ExtendedOptions {
				tlsCfg[key] = val
			}

			tlsRule := trafficpolicy.Rule{
				Name: "Gateway-TLS-Termination",
				Actions: []trafficpolicy.Action{{
					Type:   trafficpolicy.ActionType_TerminateTLS,
					Config: tlsCfg,
				}},
			}
			// Prepend to the on_tcp_connect phase
			parentTrafficPolicy.OnTCPConnect = append([]trafficpolicy.Rule{tlsRule}, parentTrafficPolicy.OnTCPConnect...)
		}

		// Generate a traffic policy that has all the rules/actions for our routes and merge it into the existing one
		routingPolicy := t.buildRoutingPolicy(irVHost, irVHost.Routes, childEndpointCache)
		parentTrafficPolicy.Merge(routingPolicy)

		defaultDestinationPolicy := t.buildDefaultDestinationPolicy(irVHost, childEndpointCache)
		parentTrafficPolicy.Merge(defaultDestinationPolicy)

		// Add a default 404 response when no routes are found
		parentTrafficPolicy.AddRuleOnHTTPRequest(buildDefault404TPRule())

		// Marshal the updated TrafficPolicySpec back to JSON
		parentPolicyJSON, err := json.Marshal(parentTrafficPolicy)
		if err != nil {
			t.log.Error(err, "failed to marshal traffic policy for generated CloudEndpoint",
				"hostname", string(irVHost.Listener.Hostname),
				"generated from resources", irVHost.OwningResources,
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

func (t *translator) buildRoutingPolicy(irVHost *ir.IRVirtualHost, routes []*ir.IRRoute, childEndpointCache map[ir.IRServiceKey]*ngrokv1alpha1.AgentEndpoint) *trafficpolicy.TrafficPolicy {
	routingTrafficPolicy := trafficpolicy.NewTrafficPolicy()

	// First, see if any of the traffic policies modify the request. If they do, we need to log and capture the original request data
	captureOriginalParams := false
	for _, irRoute := range routes {
		actionModifiesRequest := func(action trafficpolicy.Action, matchCriteria ir.IRHTTPMatch) bool {
			switch action.Type {
			case trafficpolicy.ActionType_AddHeaders, trafficpolicy.ActionType_RemoveHeaders:
				if len(matchCriteria.Headers) > 0 {
					return true
				}

			case trafficpolicy.ActionType_URLRewrite:
				if matchCriteria.Path != nil {
					return true
				}
			}
			return false
		}

		// Check all the route and backend level traffic policies
		for _, routeTrafficPolicy := range irRoute.TrafficPolicies {
			for _, tpRule := range routeTrafficPolicy.OnHTTPRequest {
				for _, ruleAction := range tpRule.Actions {
					if actionModifiesRequest(ruleAction, irRoute.MatchCriteria) {
						captureOriginalParams = true
						break
					}
				}
			}
		}
		// Also check the traffic policies per-destination
		if !captureOriginalParams {
			for _, irDestination := range irRoute.Destinations {
				for _, destTrafficPolicy := range irDestination.TrafficPolicies {
					for _, tpRule := range destTrafficPolicy.OnHTTPRequest {
						for _, ruleAction := range tpRule.Actions {
							if actionModifiesRequest(ruleAction, irRoute.MatchCriteria) {
								captureOriginalParams = true
								break
							}
						}
					}
				}
			}
		}
	}

	if captureOriginalParams {
		routingTrafficPolicy.AddRuleOnHTTPRequest(trafficpolicy.Rule{
			Name: "Log-Request-Data",
			Actions: []trafficpolicy.Action{{
				Type: trafficpolicy.ActionType_Log,
				Config: map[string]interface{}{
					"metadata": map[string]interface{}{
						"message":               "Capturing original request data with logs before it is modified",
						"endpoint_id":           "${endpoint.id}",
						"original_path":         "${req.url.path}",
						"original_headers":      "${req.headers.encodeJson()}",
						"original_query_params": "${req.url.query_params.encodeJson()}",
					},
				},
			}},
		})
	}

	for _, irRoute := range routes {
		if len(irRoute.Destinations) == 0 && len(irRoute.TrafficPolicies) == 0 {
			t.log.Error(fmt.Errorf("generated route does not have a destination"), "skipping endpoint configuration generation for invalid route, other routes will continue to be processed",
				"generated from resources", irVHost.OwningResources,
				"hostname", string(irVHost.Listener.Hostname),
				"match criteria", irRoute.MatchCriteria,
			)
			continue
		}

		matchExpressions := irMatchCriteriaToTPExpressions(irRoute.MatchCriteria, captureOriginalParams)

		// Add the match criteria expressions to all the traffic policies for this route
		for _, routeTrafficPolicy := range irRoute.TrafficPolicies {
			for _, tpRule := range routeTrafficPolicy.OnHTTPRequest {
				tpRule.Expressions = appendStringUnique(tpRule.Expressions, matchExpressions...)
				routingTrafficPolicy.AddRuleOnHTTPRequest(tpRule)
			}
			for _, tpRule := range routeTrafficPolicy.OnHTTPResponse {
				tpRule.Expressions = appendStringUnique(tpRule.Expressions, matchExpressions...)
				routingTrafficPolicy.AddRuleOnHTTPResponse(tpRule)
			}
			// TCP rules are not supported on a per-route basis
		}

		// Each route can have one or more upstreams that we route to based on the weight, so we
		// need to create a log action first if there is more than one. This will generate a random number based on the weight
		// and log it. Subsequent rules can read that logged value in their expressions to determine which of the weighted
		// upstreams we should route to. If there is only one upstream then we can skip the log rule.
		// This is a workaround since right not set_vars is not released

		if len(irRoute.Destinations) > 1 {
			// Ok, we have more than one destination, so time for some weighted routing trickery
			routeTotalWeight := 0
			for _, irDestination := range irRoute.Destinations {
				if irDestination.Weight == nil {
					defaultWeight := 1
					irDestination.Weight = &defaultWeight
				}
				routeTotalWeight += *irDestination.Weight
			}

			var logWeightCfg map[string]interface{}
			// Make sure the weighted routes log action that stores a random number doesn't erase our captured request data
			if captureOriginalParams {
				logWeightCfg = map[string]interface{}{
					"metadata": map[string]interface{}{
						"message":                   "Logging random number to select weighted route and preserving captured request data",
						"endpoint_id":               "${endpoint.id}",
						"weighted_route_random_num": fmt.Sprintf("${rand.int(0,%d)}", routeTotalWeight-1),
						"original_path":             "${req.url.path}",
						"original_headers":          "${req.headers.encodeJson()}",
						"original_query_params":     "${req.url.query_params.encodeJson()}",
					},
				}
			} else {
				logWeightCfg = map[string]interface{}{
					"metadata": map[string]interface{}{
						"message":                   "Logging random number to select weighted route",
						"endpoint_id":               "${endpoint.id}",
						"weighted_route_random_num": fmt.Sprintf("${rand.int(0,%d)}", routeTotalWeight-1),
					},
				}
			}
			routingTrafficPolicy.AddRuleOnHTTPRequest(trafficpolicy.Rule{
				Name:        "Log-Random-Number",
				Expressions: matchExpressions,
				Actions: []trafficpolicy.Action{{
					Type:   trafficpolicy.ActionType_Log,
					Config: logWeightCfg,
				}},
			})
		}

		currentLowerBound := 0 // starting value for the random number range
		for _, irDestination := range irRoute.Destinations {
			var weightedRouteExpression *string
			if len(irRoute.Destinations) > 1 {
				weight := *irDestination.Weight
				// The destination's range is [currentLowerBound, currentLowerBound + weight - 1]
				currentUpperBound := currentLowerBound + weight
				if currentLowerBound == 0 {
					expr := fmt.Sprintf("int(actions.ngrok.log.metadata['weighted_route_random_num']) <= %d", currentUpperBound-1)
					weightedRouteExpression = &expr
				} else {
					expr := fmt.Sprintf("int(actions.ngrok.log.metadata['weighted_route_random_num']) >= %d && int(actions.ngrok.log.metadata['weighted_route_random_num']) <= %d", currentLowerBound, currentUpperBound-1)
					weightedRouteExpression = &expr
				}
				currentLowerBound = currentUpperBound
			}

			// Each destination can have it's own set of traffic policies that should only run when that route is selected
			for _, destinationTrafficPolicy := range irDestination.TrafficPolicies {
				for _, tpRule := range destinationTrafficPolicy.OnHTTPRequest {
					tpRule.Expressions = appendStringUnique(tpRule.Expressions, matchExpressions...)
					if weightedRouteExpression != nil {
						tpRule.Expressions = appendStringUnique(tpRule.Expressions, *weightedRouteExpression)
					}
					routingTrafficPolicy.AddRuleOnHTTPRequest(tpRule)
				}
				for _, tpRule := range destinationTrafficPolicy.OnHTTPResponse {
					tpRule.Expressions = appendStringUnique(tpRule.Expressions, matchExpressions...)
					if weightedRouteExpression != nil {
						tpRule.Expressions = appendStringUnique(tpRule.Expressions, *weightedRouteExpression)
					}
					routingTrafficPolicy.AddRuleOnHTTPResponse(tpRule)
				}
				// TCP rules are not supported on a per-route basis
			}

			// Finally, if we have an upstream service to route to, build the rule for that
			if irDestination.Upstream != nil {
				service := irDestination.Upstream.Service
				childEndpoint, exists := childEndpointCache[service.Key()]
				if !exists {
					childEndpoint = buildInternalAgentEndpoint(
						service.UID,
						service.Name,
						service.Namespace,
						t.clusterDomain,
						service.Port,
						service.Scheme,
						irVHost.LabelsToAdd,
						irVHost.AnnotationsToAdd,
						irVHost.Metadata,
						service.ClientCertRefs,
					)
					childEndpointCache[service.Key()] = childEndpoint
				}
				// Inject a rule into the traffic policy that will route to the desired upstream on path match for the route
				tpRouteRule := buildEndpointServiceRouteRule("Generated-Route", childEndpoint.Spec.URL)
				tpRouteRule.Expressions = appendStringUnique(tpRouteRule.Expressions, matchExpressions...)
				if weightedRouteExpression != nil {
					tpRouteRule.Expressions = appendStringUnique(tpRouteRule.Expressions, *weightedRouteExpression)
				}
				routingTrafficPolicy.AddRuleOnHTTPRequest(tpRouteRule)
			}
		}
	}
	return routingTrafficPolicy
}

// buildPathMatchExpressionExpressionToTPRule creates an expression for a traffic policy rule to control path matching.
// Normally it will check the request data from the req. variable, but when we have actions such as a url-rewrite that
// modify the request, then we instead need to check the request data from a log action that will store it before it is transformed
func irMatchCriteriaToTPExpressions(matchCriteria ir.IRHTTPMatch, getRequestDataFromLog bool) []string {
	expressions := []string{}

	// 1. Path matching
	if matchCriteria.Path != nil {
		pathType := ir.IRPathType_Prefix // Defult to prefix match
		if matchCriteria.PathType != nil {
			pathType = *matchCriteria.PathType
		}
		switch pathType {
		case ir.IRPathType_Exact:
			if getRequestDataFromLog {
				expressions = append(expressions, fmt.Sprintf("actions.ngrok.log.metadata['original_path'] == '%s'", *matchCriteria.Path))
			} else {
				expressions = append(expressions, fmt.Sprintf("req.url.path == '%s'", *matchCriteria.Path))
			}
		case ir.IRPathType_Prefix:
			fallthrough
		default:
			if getRequestDataFromLog {
				expressions = append(expressions, fmt.Sprintf("actions.ngrok.log.metadata['original_path'].startsWith('%s')", *matchCriteria.Path))
			} else {
				expressions = append(expressions, fmt.Sprintf("req.url.path.startsWith('%s')", *matchCriteria.Path))
			}
		}
	}

	// 2. Header matching
	for _, headerMatch := range matchCriteria.Headers {
		switch headerMatch.ValueType {
		case ir.IRStringValueType_Exact:
			if getRequestDataFromLog {
				expressions = append(expressions, fmt.Sprintf("actions.ngrok.log.metadata['original_headers'].decodeJson().exists_one(x, x == '%s') && actions.ngrok.log.metadata['original_headers'].decodeJson()['%s'].join(',') == '%s'",
					headerMatch.Name,
					headerMatch.Name,
					headerMatch.Value,
				))
			} else {
				expressions = append(expressions, fmt.Sprintf("req.headers.exists_one(x, x == '%s') && req.headers['%s'].join(',') == '%s'",
					headerMatch.Name,
					headerMatch.Name,
					headerMatch.Value,
				))
			}

		case ir.IRStringValueType_Regex:
			if getRequestDataFromLog {
				expressions = append(expressions, fmt.Sprintf("actions.ngrok.log.metadata['original_headers'].decodeJson().exists_one(x, x == '%s') && actions.ngrok.log.metadata['original_headers'].decodeJson()['%s'].join(',').matches('%s')",
					headerMatch.Name,
					headerMatch.Name,
					headerMatch.Value,
				))
			} else {
				expressions = append(expressions, fmt.Sprintf("req.headers.exists_one(x, x == '%s') && req.headers['%s'].join(',').matches('%s')",
					headerMatch.Name,
					headerMatch.Name,
					headerMatch.Value,
				))
			}
		}
	}

	for _, queryParamMatch := range matchCriteria.QueryParams {
		switch queryParamMatch.ValueType {
		case ir.IRStringValueType_Exact:
			if getRequestDataFromLog {
				expressions = append(expressions, fmt.Sprintf("actions.ngrok.log.metadata['original_query_params'].decodeJson().exists_one(x, x == '%s') && actions.ngrok.log.metadata['original_query_params'].decodeJson()['%s'].join(',') == '%s'",
					queryParamMatch.Name,
					queryParamMatch.Name,
					queryParamMatch.Value,
				))
			} else {
				expressions = append(expressions, fmt.Sprintf("req.url.query_params.exists_one(x, x == '%s') && req.url.query_params['%s'].join(',') == '%s'",
					queryParamMatch.Name,
					queryParamMatch.Name,
					queryParamMatch.Value,
				))
			}
		case ir.IRStringValueType_Regex:
			if getRequestDataFromLog {
				expressions = append(expressions, fmt.Sprintf("actions.ngrok.log.metadata['original_query_params'].decodeJson().exists_one(x, x == '%s') && actions.ngrok.log.metadata['original_query_params'].decodeJson()['%s'].join(',').matches('%s')",
					queryParamMatch.Name,
					queryParamMatch.Name,
					queryParamMatch.Value,
				))
			} else {
				expressions = append(expressions, fmt.Sprintf("req.url.query_params.exists_one(x, x == '%s') && req.url.query_params['%s'].join(',').matches('%s')",
					queryParamMatch.Name,
					queryParamMatch.Name,
					queryParamMatch.Value,
				))
			}
		}
	}

	if matchCriteria.Method != nil {
		expressions = append(expressions, fmt.Sprintf("req.method == '%s'", string(*matchCriteria.Method)))
	}

	return expressions
}

// buildEndpointServiceRouteRule constructs a traffic policy rule to route to an internal url that is provided by an internal Agent Endpoint
// which will route any requests it receives to its acompanying upstream service
func buildEndpointServiceRouteRule(name string, url string) trafficpolicy.Rule {
	return trafficpolicy.Rule{
		Name: name,
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

func (t *translator) buildDefaultDestinationPolicy(irVHost *ir.IRVirtualHost, childEndpointCache map[ir.IRServiceKey]*ngrokv1alpha1.AgentEndpoint) *trafficpolicy.TrafficPolicy {
	defaultDestination := irVHost.DefaultDestination
	defaultDestTrafficPolicy := trafficpolicy.NewTrafficPolicy()
	if defaultDestination == nil {
		return defaultDestTrafficPolicy
	}

	// If the default backend has a set of traffic policies, then just append all of their rules
	for _, defaultDestTP := range defaultDestination.TrafficPolicies {
		defaultDestTrafficPolicy.Merge(defaultDestTP)
	}

	// If the default backend has a service, then add a route for it to the end of the traffic policy with no
	// expressions so it matches anything that did not match any prior traffic policy rules
	if upstream := defaultDestination.Upstream; upstream != nil {
		service := upstream.Service
		childEndpoint, exists := childEndpointCache[service.Key()]
		if !exists {
			childEndpoint = buildInternalAgentEndpoint(
				service.UID,
				service.Name,
				service.Namespace,
				t.clusterDomain,
				service.Port,
				service.Scheme,
				irVHost.LabelsToAdd,
				irVHost.AnnotationsToAdd,
				t.defaultIngressMetadata,
				service.ClientCertRefs,
			)
			childEndpointCache[service.Key()] = childEndpoint
		}
		routeRule := buildEndpointServiceRouteRule("Generated-Route-Default-Backend", childEndpoint.Spec.URL)
		defaultDestTrafficPolicy.AddRuleOnHTTPRequest(routeRule)
	}
	return defaultDestTrafficPolicy
}

// buildCloudEndpoint initializes a new CloudEndpoint
func buildCloudEndpoint(irVHost *ir.IRVirtualHost) *ngrokv1alpha1.CloudEndpoint {
	name := ""
	if irVHost.NamePrefix != nil {
		name = fmt.Sprintf("%s-", *irVHost.NamePrefix)
	}
	name += string(irVHost.Listener.Hostname)

	var port *int32
	url := ""
	switch irVHost.Listener.Protocol {
	case ir.IRProtocol_HTTPS:
		url += "https://"
		if irVHost.Listener.Port != 443 {
			port = &irVHost.Listener.Port
		}
	case ir.IRProtocol_HTTP:
		url += "http://"
		if irVHost.Listener.Port != 80 {
			port = &irVHost.Listener.Port
		}
	}
	url += string(irVHost.Listener.Hostname)
	if port != nil {
		url = fmt.Sprintf("%s:%d", url, *port)
	}

	if len(irVHost.AnnotationsToAdd) == 0 {
		irVHost.AnnotationsToAdd = nil
	}
	if len(irVHost.LabelsToAdd) == 0 {
		irVHost.LabelsToAdd = nil
	}

	return &ngrokv1alpha1.CloudEndpoint{
		ObjectMeta: metav1.ObjectMeta{
			Name:        sanitizeStringForK8sName(name),
			Namespace:   irVHost.Namespace,
			Labels:      irVHost.LabelsToAdd,
			Annotations: irVHost.AnnotationsToAdd,
		},
		Spec: ngrokv1alpha1.CloudEndpointSpec{
			URL:            url,
			PoolingEnabled: irVHost.EndpointPoolingEnabled,
			Metadata:       irVHost.Metadata,
			Bindings:       irVHost.Bindings,
		},
	}
}

// buildInternalAgentEndpoint initializes a new internal AgentEndpoint
func buildInternalAgentEndpoint(
	serviceUID string,
	serviceName string,
	namespace string,
	clusterDomain string,
	port int32,
	scheme ir.IRScheme,
	labels map[string]string,
	annotations map[string]string,
	metadata string,
	upstreamClientCertRefs []ir.IRObjectRef,
) *ngrokv1alpha1.AgentEndpoint {
	ret := &ngrokv1alpha1.AgentEndpoint{
		ObjectMeta: metav1.ObjectMeta{
			Name:        internalAgentEndpointName(serviceUID, serviceName, namespace, clusterDomain, port, upstreamClientCertRefs),
			Namespace:   namespace,
			Labels:      labels,
			Annotations: annotations,
		},
		Spec: ngrokv1alpha1.AgentEndpointSpec{
			URL:      internalAgentEndpointURL(serviceUID, serviceName, namespace, clusterDomain, port, upstreamClientCertRefs),
			Metadata: metadata,
			Upstream: ngrokv1alpha1.EndpointUpstream{
				URL: internalAgentEndpointUpstreamURL(serviceName, namespace, clusterDomain, port, scheme),
			},
			Bindings: []string{"internal"},
		},
	}

	for _, certRef := range upstreamClientCertRefs {
		ret.Spec.ClientCertificateRefs = append(ret.Spec.ClientCertificateRefs, ngrokv1alpha1.K8sObjectRefOptionalNamespace{
			Name:      certRef.Name,
			Namespace: &certRef.Namespace,
		})
	}

	return ret
}

// buildDefault404TPRule builds the default rule that will fire if no other rules are matched to return a 404 response
func buildDefault404TPRule() trafficpolicy.Rule {
	return trafficpolicy.Rule{
		Name: "Fallback-404",
		Actions: []trafficpolicy.Action{
			{
				// Basic text for now, but we can add styling/branding later
				Type: trafficpolicy.ActionType_CustomResponse,
				Config: map[string]interface{}{
					"status_code": 404,
					"content":     "No route was found for this ngrok Cloud Endpoint",
					"headers": map[string]string{
						"content-type": "text/plain",
					},
				},
			},
		},
	}
}
