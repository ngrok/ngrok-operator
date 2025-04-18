package managerdriver

import (
	"encoding/json"
	"fmt"

	"github.com/go-logr/logr"
	ngrokv1alpha1 "github.com/ngrok/ngrok-operator/api/ngrok/v1alpha1"
	"github.com/ngrok/ngrok-operator/internal/annotations"
	"github.com/ngrok/ngrok-operator/internal/annotations/parser"
	"github.com/ngrok/ngrok-operator/internal/errors"
	"github.com/ngrok/ngrok-operator/internal/ir"
	"github.com/ngrok/ngrok-operator/internal/store"
	"github.com/ngrok/ngrok-operator/internal/trafficpolicy"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
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

// Takes a set of IRVirtualHosts and iterates over them to figure out which can and cannot be collapsed according to their mapping strategies
func validateMappingStrategies(irVHosts []*ir.IRVirtualHost) {

	virtualHostsForService := make(map[ir.IRServiceKey]map[*ir.IRVirtualHost]bool)
	for _, irVHost := range irVHosts {
		for _, irRoute := range irVHost.Routes {
			for _, irDestination := range irRoute.Destinations {
				if irDestination.Upstream == nil {
					continue
				}
				svcKey := irDestination.Upstream.Service.Key()
				if _, exists := virtualHostsForService[svcKey]; !exists {
					virtualHostsForService[svcKey] = make(map[*ir.IRVirtualHost]bool)
				}
				virtualHostsForService[svcKey][irVHost] = true
			}
		}
		if irVHost.DefaultDestination != nil && irVHost.DefaultDestination.Upstream != nil {
			svcKey := irVHost.DefaultDestination.Upstream.Service.Key()
			if _, exists := virtualHostsForService[svcKey]; !exists {
				virtualHostsForService[svcKey] = make(map[*ir.IRVirtualHost]bool)
			}
			virtualHostsForService[svcKey][irVHost] = true
		}
	}

	collapsableServices := make(map[ir.IRServiceKey]bool)
	for irServiceKey, vHostsForService := range virtualHostsForService {
		// For each service, if it is only used on a single virtual host, then it can become a public AgentEndpoint
		// if more than one virtual host routes to it, it must become an internal AgentEndpoint regardless of the mapping-strategy annotation

		for vHost := range vHostsForService {
			if len(vHostsForService) == 1 && vHost.MappingStrategy == ir.IRMappingStrategy_EndpointsCollapsed {
				collapsableServices[irServiceKey] = true
			}
		}
	}

	// Iterate over all the hosts, and if their mapping strategy is collapsed endpoints, make sure they can actually be collapsed, otherwise set them to endpoints-verbose
	for _, irVHost := range irVHosts {
		if irVHost.MappingStrategy != ir.IRMappingStrategy_EndpointsCollapsed {
			continue
		}

		// If there are no upstream services for the virtual host, it doesn't make sense to collapse it into an AgentEndpoint.
		// This might happen in some situations, for example, an Ingress that only routes to NgrokTrafficPolicy resources
		if irVHost.UniqueServiceCount() == 0 {
			continue
		}

		// We already identified which services are only used on one virtual host and can be collapsed into a public AgentEndpoint,
		// so if any of those are on this virtual host, then we can collapse. We only collapse with one, if there are more than one unique services only used on this virtual host, the the others become regular internal agent endpoints.
		for _, irRoute := range irVHost.Routes {
			for _, irDestination := range irRoute.Destinations {
				if irDestination.Upstream == nil {
					continue
				}
				svcKey := irDestination.Upstream.Service.Key()
				if collapsable, exists := collapsableServices[svcKey]; exists && collapsable {
					// Keep track of the service that we are collapsing into so that we can adjust routing rules later for the "local" service and the others
					irVHost.CollapseIntoServiceKey = &svcKey
					continue
				}
			}
		}

		// If there wasn't a unique service in the routes, check the default destination
		if irVHost.DefaultDestination != nil && irVHost.DefaultDestination.Upstream != nil {
			svcKey := irVHost.DefaultDestination.Upstream.Service.Key()
			if collapsable, exists := collapsableServices[svcKey]; exists && collapsable {
				irVHost.CollapseIntoServiceKey = &svcKey
				return
			}
		}
	}
}

// IRToEndpoints converts a set of IRVirtualHosts into CloudEndpoints and AgentEndpoints
func (t *translator) IRToEndpoints(irVHosts []*ir.IRVirtualHost) (cloudEndpoints map[types.NamespacedName]*ngrokv1alpha1.CloudEndpoint, agentEndpoints map[types.NamespacedName]*ngrokv1alpha1.AgentEndpoint) {
	// Setup a cache for the agent endpoints as any given backend may be used across several ingresses, etc.
	agentEndpointCache := make(map[ir.IRServiceKey]*ngrokv1alpha1.AgentEndpoint)
	cloudEndpoints = make(map[types.NamespacedName]*ngrokv1alpha1.CloudEndpoint)

	validateMappingStrategies(irVHosts)
	for _, irVHost := range irVHosts {
		if irVHost.TrafficPolicy == nil && len(irVHost.Routes) == 0 {
			t.log.Error(fmt.Errorf("skipping generating endpoints for hostname with no valid traffic policy or routes"),
				"hostname", string(irVHost.Listener.Hostname),
				"generated from resources", irVHost.OwningResources,
			)
			continue
		}

		// If the resource does not have a user-supplied traffic policy, then create a new one as the base
		listenerTrafficPolicy := irVHost.TrafficPolicy
		if listenerTrafficPolicy == nil {
			listenerTrafficPolicy = trafficpolicy.NewTrafficPolicy()
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
			listenerTrafficPolicy.OnTCPConnect = append([]trafficpolicy.Rule{tlsRule}, listenerTrafficPolicy.OnTCPConnect...)
		}

		// Generate a traffic policy that has all the rules/actions for our routes and merge it into the existing one
		routingPolicy := t.buildRoutingPolicy(irVHost, agentEndpointCache)
		listenerTrafficPolicy.Merge(routingPolicy)

		defaultDestinationPolicy, err := t.buildDefaultDestinationPolicy(irVHost, agentEndpointCache)
		if err != nil {
			t.log.Error(err, "failed to default destination traffic policy",
				"hostname", irVHost.Listener.Hostname,
				"port", irVHost.Listener.Port,
				"protocol", irVHost.Listener.Protocol,
				"generated from resources", irVHost.OwningResources,
			)
			continue
		}
		listenerTrafficPolicy.Merge(defaultDestinationPolicy)

		switch irVHost.Listener.Protocol {
		case ir.IRProtocol_HTTP, ir.IRProtocol_HTTPS:
			// Add a default 404 response when no routes are found
			default404Rule := buildDefault404TPRule()
			// If we're collapsing into an AgentEndpoint, make sure this action only runs when we didn't match the local service
			if irVHost.CollapseIntoServiceKey != nil {
				default404Rule.Expressions = appendStringUnique(default404Rule.Expressions, "vars.request_matched_local_svc == false")
			}
			listenerTrafficPolicy.AddRuleOnHTTPRequest(default404Rule)
		case ir.IRProtocol_TCP, ir.IRProtocol_TLS:
			// Not needed for TLS/TCP endpoints
		}

		// Marshal the updated TrafficPolicySpec back to JSON
		listenerPolicyJSON, err := json.Marshal(listenerTrafficPolicy)
		if err != nil {
			t.log.Error(err, "failed to marshal traffic policy for generated CloudEndpoint",
				"hostname", string(irVHost.Listener.Hostname),
				"port", irVHost.Listener.Port,
				"protocol", irVHost.Listener.Protocol,
				"generated from resources", irVHost.OwningResources,
			)
			continue
		}

		// Determine whether we are using a CloudEndpoint or AgentEndpoint to listen for requests
		if irVHost.CollapseIntoServiceKey != nil {
			// If this is a collapsed AgentEndpoint, we might not need a traffic policy
			if listenerTrafficPolicy != nil && !listenerTrafficPolicy.IsEmpty() {
				if agentEndpoint, exists := agentEndpointCache[*irVHost.CollapseIntoServiceKey]; exists {
					agentEndpoint.Spec.TrafficPolicy = &ngrokv1alpha1.TrafficPolicyCfg{
						Inline: json.RawMessage(listenerPolicyJSON),
					}
				}
			}
		} else {
			cloudEndpoint, err := buildCloudEndpoint(irVHost)
			if err != nil {
				t.log.Error(err, "failed to build CloudEndpoint",
					"hostname", irVHost.Listener.Hostname,
					"port", irVHost.Listener.Port,
					"protocol", irVHost.Listener.Protocol,
					"generated from resources", irVHost.OwningResources,
				)
				continue
			}
			cloudEndpoint.Spec.TrafficPolicy = &ngrokv1alpha1.NgrokTrafficPolicySpec{
				Policy: json.RawMessage(listenerPolicyJSON),
			}
			cloudEndpoints[types.NamespacedName{
				Name:      cloudEndpoint.Name,
				Namespace: cloudEndpoint.Namespace,
			}] = cloudEndpoint
		}
	}

	// Return all of the generated CloudEndpoints and AgentEndpoints as maps keyed by namespaced name for easier lookup
	agentEndpoints = make(map[types.NamespacedName]*ngrokv1alpha1.AgentEndpoint)
	for _, agentEndpoint := range agentEndpointCache {
		agentEndpoints[types.NamespacedName{
			Name:      agentEndpoint.Name,
			Namespace: agentEndpoint.Namespace,
		}] = agentEndpoint
	}

	return cloudEndpoints, agentEndpoints
}

func (t *translator) buildRoutingPolicy(irVHost *ir.IRVirtualHost, agentEndpointCache map[ir.IRServiceKey]*ngrokv1alpha1.AgentEndpoint) *trafficpolicy.TrafficPolicy {
	routingTrafficPolicy := trafficpolicy.NewTrafficPolicy()
	// If we're collapsing this into an AgentEndpoint, set a variable so we know if routes match the local service or not since there could be other routes
	if irVHost.CollapseIntoServiceKey != nil {
		switch irVHost.Listener.Protocol {
		case ir.IRProtocol_HTTP, ir.IRProtocol_HTTPS:
			routingTrafficPolicy.AddRuleOnHTTPRequest(buildRouteLocallyVarRule("Initialize-Local-Service-Match", false))
		case ir.IRProtocol_TCP, ir.IRProtocol_TLS:
			// This is only necessary for tcp:// and tls:// endpoints if we're balancing between multiple upstreams since othherwise they don't have match criteria
			// TCP and TLS endpoints only ever have one route since there are no match criteria, but we might have more than one backend (weighted)
			if len(irVHost.Routes) == 1 && len(irVHost.Routes[0].Destinations) > 1 {
				routingTrafficPolicy.AddRuleOnTCPConnect(buildRouteLocallyVarRule("Initialize-Local-Service-Match", false))
			}
		}
	}

	// First, see if any of the traffic policies modify the request. If they do, we need to capture the original request data
	captureOriginalParams := false
	for _, irRoute := range irVHost.Routes {
		actionModifiesRequest := func(action trafficpolicy.Action, matchCriteria *ir.IRHTTPMatch) bool {
			if matchCriteria == nil {
				return false
			}
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
					if actionModifiesRequest(ruleAction, irRoute.HTTPMatchCriteria) {
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
							if actionModifiesRequest(ruleAction, irRoute.HTTPMatchCriteria) {
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
			Name: "Capture-Original-Request-Data",
			Actions: []trafficpolicy.Action{{
				Type: trafficpolicy.ActionType_SetVars,
				Config: map[string]interface{}{
					"vars": []map[string]interface{}{{
						"original_path": "${req.url.path}",
					}, {
						"original_headers": "${req.headers.encodeJson()}",
					}, {
						"original_query_params": "${req.url.query_params.encodeJson()}",
					}},
				},
			}},
		})
	}

	for _, irRoute := range irVHost.Routes {
		if len(irRoute.Destinations) == 0 && len(irRoute.TrafficPolicies) == 0 {
			t.log.Error(fmt.Errorf("generated route does not have a destination"), "skipping endpoint configuration generation for invalid route, other routes will continue to be processed",
				"generated from resources", irVHost.OwningResources,
				"hostname", string(irVHost.Listener.Hostname),
				"match criteria", irRoute.HTTPMatchCriteria,
			)
			continue
		}

		matchExpressions := irMatchCriteriaToTPExpressions(irRoute.HTTPMatchCriteria, captureOriginalParams)
		if irVHost.CollapseIntoServiceKey != nil {
			// If we're collapsing this into an AgentEndpoint, add an expression to make sure that none of the prior rules have matched the local service.
			// For example, an Ingress with a rule /paththatislonger that routes to the local service and a rule /path that matches something else, we don't want to bother
			// running the things for the shorter path after we already matched the longer more specific rule. Unlike routing to other endpoints with forward-internal, when we are routing to the local service
			// we have to finish evaluating all the other traffic policy rules, whereas forward-internal "terminates" the traffic policy early.
			matchExpressions = appendStringUnique(matchExpressions, "vars.request_matched_local_svc == false")
		}

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
		// need to create a set-vars action first if there is more than one. This will generate a random number based on the weight
		// and store it. Subsequent rules can read that set variable in their expressions to determine which of the weighted
		// upstreams we should route to. If there is only one upstream then we can skip the set-vars rule.
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

			// Make sure the weighted routes set-vars action that stores a random number doesn't erase our captured request data
			setvarWeightCfg := map[string]interface{}{
				"vars": []map[string]interface{}{{
					"weighted_route_random_num": fmt.Sprintf("${rand.int(0,%d)}", routeTotalWeight-1),
				}},
			}

			randomNumRule := trafficpolicy.Rule{
				Name:        "Gen-Random-Number",
				Expressions: matchExpressions,
				Actions: []trafficpolicy.Action{{
					Type:   trafficpolicy.ActionType_SetVars,
					Config: setvarWeightCfg,
				}},
			}

			switch irVHost.Listener.Protocol {
			case ir.IRProtocol_HTTP, ir.IRProtocol_HTTPS:
				routingTrafficPolicy.AddRuleOnHTTPRequest(randomNumRule)
			case ir.IRProtocol_TCP, ir.IRProtocol_TLS:
				routingTrafficPolicy.AddRuleOnTCPConnect(randomNumRule)
			}

		}

		currentLowerBound := 0 // starting value for the random number range
		for _, irDestination := range irRoute.Destinations {
			var weightedRouteExpression *string
			if len(irRoute.Destinations) > 1 {
				weight := *irDestination.Weight
				// The destination's range is [currentLowerBound, currentLowerBound + weight - 1]
				currentUpperBound := currentLowerBound + weight
				if currentLowerBound == 0 {
					expr := fmt.Sprintf("int(vars.weighted_route_random_num) <= %d", currentUpperBound-1)
					weightedRouteExpression = &expr
				} else {
					expr := fmt.Sprintf("int(vars.weighted_route_random_num) >= %d && int(vars.weighted_route_random_num) <= %d", currentLowerBound, currentUpperBound-1)
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
				irService := irDestination.Upstream.Service
				agentEndpoint, exists := agentEndpointCache[irService.Key()]
				if !exists {
					var err error
					agentEndpoint, err = buildAgentEndpoint(
						irVHost,
						irService,
						t.clusterDomain,
						irVHost.Metadata)
					if err != nil {
						t.log.Error(err, "failed to build AgentEndpoint",
							"hostname", irVHost.Listener.Hostname,
							"port", irVHost.Listener.Port,
							"protocol", irVHost.Listener.Protocol,
							"generated from resources", irDestination.Upstream.OwningResources,
						)
						continue
					}
					agentEndpointCache[irService.Key()] = agentEndpoint
				}

				// If we're collapsing this upstream into a public AgentEndpoint then we need to set a variable for whether or not the request matched the local service instead of using
				// forward-internal to forward to another endpoint

				var tpRouteRule trafficpolicy.Rule
				if irVHost.CollapseIntoServiceKey != nil && irService.Key() == *irVHost.CollapseIntoServiceKey {
					// Inject a rule into the traffic policy that will set a variable if we matched the local service
					tpRouteRule = buildRouteLocallyVarRule("Generated-Local-Service-Route", true)
				} else {
					// Inject a rule into the traffic policy that will route to the desired upstream on path match for the route
					tpRouteRule = buildEndpointServiceRouteRule("Generated-Route", agentEndpoint.Spec.URL)
				}
				tpRouteRule.Expressions = appendStringUnique(tpRouteRule.Expressions, matchExpressions...)
				if weightedRouteExpression != nil {
					tpRouteRule.Expressions = appendStringUnique(tpRouteRule.Expressions, *weightedRouteExpression)
				}

				switch irVHost.Listener.Protocol {
				case ir.IRProtocol_HTTP, ir.IRProtocol_HTTPS:
					routingTrafficPolicy.AddRuleOnHTTPRequest(tpRouteRule)
				case ir.IRProtocol_TCP, ir.IRProtocol_TLS:
					// This is only necessary for tcp:// and tls:// endpoints if we're balancing between multiple upstreams since othherwise they don't have match criteria
					if len(irVHost.Routes) == 1 && len(irVHost.Routes[0].Destinations) > 1 {
						routingTrafficPolicy.AddRuleOnTCPConnect(tpRouteRule)
					}
				}
			}
		}
	}
	return routingTrafficPolicy
}

// buildPathMatchExpressionExpressionToTPRule creates an expression for a traffic policy rule to control path matching.
// Normally it will check the request data from the req. variable, but when we have actions such as a url-rewrite that
// modify the request, then we instead need to check the request data from a set-vars action that will store it before it is transformed
func irMatchCriteriaToTPExpressions(matchCriteria *ir.IRHTTPMatch, getRequestDataFromVar bool) []string {
	expressions := []string{}

	if matchCriteria == nil {
		return expressions
	}

	// 1. Path matching
	if matchCriteria.Path != nil {
		pathType := ir.IRPathType_Prefix // Defult to prefix match
		if matchCriteria.PathType != nil {
			pathType = *matchCriteria.PathType
		}
		switch pathType {
		case ir.IRPathType_Exact:
			if getRequestDataFromVar {
				expressions = appendStringUnique(expressions, fmt.Sprintf("vars.original_path == '%s'", *matchCriteria.Path))
			} else {
				expressions = appendStringUnique(expressions, fmt.Sprintf("req.url.path == '%s'", *matchCriteria.Path))
			}
		case ir.IRPathType_Prefix:
			fallthrough
		default:
			if getRequestDataFromVar {
				expressions = appendStringUnique(expressions, fmt.Sprintf("vars.original_path.startsWith('%s')", *matchCriteria.Path))
			} else {
				expressions = appendStringUnique(expressions, fmt.Sprintf("req.url.path.startsWith('%s')", *matchCriteria.Path))
			}
		}
	}

	// 2. Header matching
	for _, headerMatch := range matchCriteria.Headers {
		switch headerMatch.ValueType {
		case ir.IRStringValueType_Exact:
			if getRequestDataFromVar {
				expressions = appendStringUnique(expressions, fmt.Sprintf("vars.original_headers.decodeJson().exists_one(x, x == '%s') && vars.original_headers.decodeJson()['%s'].join(',') == '%s'",
					headerMatch.Name,
					headerMatch.Name,
					headerMatch.Value,
				))
			} else {
				expressions = appendStringUnique(expressions, fmt.Sprintf("req.headers.exists_one(x, x == '%s') && req.headers['%s'].join(',') == '%s'",
					headerMatch.Name,
					headerMatch.Name,
					headerMatch.Value,
				))
			}

		case ir.IRStringValueType_Regex:
			if getRequestDataFromVar {
				expressions = appendStringUnique(expressions, fmt.Sprintf("vars.original_headers.decodeJson().exists_one(x, x == '%s') && vars.original_headers.decodeJson()['%s'].join(',').matches('%s')",
					headerMatch.Name,
					headerMatch.Name,
					headerMatch.Value,
				))
			} else {
				expressions = appendStringUnique(expressions, fmt.Sprintf("req.headers.exists_one(x, x == '%s') && req.headers['%s'].join(',').matches('%s')",
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
			if getRequestDataFromVar {
				expressions = appendStringUnique(expressions, fmt.Sprintf("vars.original_query_params.decodeJson().exists_one(x, x == '%s') && vars.original_query_params.decodeJson()['%s'].join(',') == '%s'",
					queryParamMatch.Name,
					queryParamMatch.Name,
					queryParamMatch.Value,
				))
			} else {
				expressions = appendStringUnique(expressions, fmt.Sprintf("req.url.query_params.exists_one(x, x == '%s') && req.url.query_params['%s'].join(',') == '%s'",
					queryParamMatch.Name,
					queryParamMatch.Name,
					queryParamMatch.Value,
				))
			}
		case ir.IRStringValueType_Regex:
			if getRequestDataFromVar {
				expressions = appendStringUnique(expressions, fmt.Sprintf("vars.original_query_params.decodeJson().exists_one(x, x == '%s') && vars.original_query_params.decodeJson()['%s'].join(',').matches('%s')",
					queryParamMatch.Name,
					queryParamMatch.Name,
					queryParamMatch.Value,
				))
			} else {
				expressions = appendStringUnique(expressions, fmt.Sprintf("req.url.query_params.exists_one(x, x == '%s') && req.url.query_params['%s'].join(',').matches('%s')",
					queryParamMatch.Name,
					queryParamMatch.Name,
					queryParamMatch.Value,
				))
			}
		}
	}

	if matchCriteria.Method != nil {
		expressions = appendStringUnique(expressions, fmt.Sprintf("req.method == '%s'", string(*matchCriteria.Method)))
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

// buildRouteLocallyVarRule builds a set-vars action that sets the request_matched_local_svc var to true.
// It is used in place of forward-internal when an AgentEndpoint has a route for its own service.
// This allows you to translate things like an Ingress that has a single rule for /foo for a service into a single AgentEndpoint.
// In this example, we want to route to the local service for the agent endpoint if the /foo path is matched, and otherwise fall into the default 404 response rule
func buildRouteLocallyVarRule(name string, value bool) trafficpolicy.Rule {
	return trafficpolicy.Rule{
		Name: name,
		Actions: []trafficpolicy.Action{
			{
				Type: trafficpolicy.ActionType_SetVars,
				Config: map[string]interface{}{
					"vars": []map[string]interface{}{{
						"request_matched_local_svc": value,
					}},
				},
			},
		},
	}
}

func (t *translator) buildDefaultDestinationPolicy(irVHost *ir.IRVirtualHost, agentEndpointCache map[ir.IRServiceKey]*ngrokv1alpha1.AgentEndpoint) (*trafficpolicy.TrafficPolicy, error) {
	defaultDestination := irVHost.DefaultDestination
	defaultDestTrafficPolicy := trafficpolicy.NewTrafficPolicy()
	if defaultDestination == nil {
		return defaultDestTrafficPolicy, nil
	}

	// If the default backend has a set of traffic policies, then just append all of their rules
	for _, defaultDestTP := range defaultDestination.TrafficPolicies {
		for _, onResRule := range defaultDestTP.OnHTTPRequest {
			onResRule.Expressions = appendStringUnique(onResRule.Expressions, "vars.request_matched_local_svc == false")
		}
		for _, onTCPRule := range defaultDestTP.OnHTTPResponse {
			onTCPRule.Expressions = appendStringUnique(onTCPRule.Expressions, "vars.request_matched_local_svc == false")
		}

		defaultDestTrafficPolicy.Merge(defaultDestTP)
	}

	// If the default backend has a service, then add a route for it to the end of the traffic policy with no
	// expressions so it matches anything that did not match any prior traffic policy rules
	if upstream := defaultDestination.Upstream; upstream != nil {
		irService := upstream.Service
		agentEndpoint, exists := agentEndpointCache[irService.Key()]
		if !exists {
			var err error
			agentEndpoint, err = buildAgentEndpoint(
				irVHost,
				irService,
				t.clusterDomain,
				t.defaultIngressMetadata,
			)
			if err != nil {
				return nil, fmt.Errorf("failed to build AgentEndpoint. upstream generated from resources: %v, err: %w", upstream.OwningResources, err)
			}
			agentEndpointCache[irService.Key()] = agentEndpoint
		}
		// If we're collapsing this into an AgentEndpoint, we only want to run this when a request did not match the local service
		var routeRule trafficpolicy.Rule
		if irVHost.CollapseIntoServiceKey != nil && irService.Key() == *irVHost.CollapseIntoServiceKey {
			routeRule = buildRouteLocallyVarRule("Generated-Route-Default-Backend", true)
		} else {
			routeRule = buildEndpointServiceRouteRule("Generated-Route-Default-Backend", agentEndpoint.Spec.URL)
		}
		if irVHost.CollapseIntoServiceKey != nil {
			routeRule.Expressions = appendStringUnique(routeRule.Expressions, "vars.request_matched_local_svc == false")
		}
		defaultDestTrafficPolicy.AddRuleOnHTTPRequest(routeRule)
	}
	return defaultDestTrafficPolicy, nil
}

func buildPublicURL(irVHost *ir.IRVirtualHost) (string, error) {
	url := ""

	scheme, err := protocolStringToIRScheme(irVHost.Listener.Protocol)
	if err != nil {
		return "", err
	}

	url += string(scheme)

	var port *int32
	switch irVHost.Listener.Protocol {
	case ir.IRProtocol_HTTPS:
		if irVHost.Listener.Port != 443 {
			port = &irVHost.Listener.Port
		}
	case ir.IRProtocol_HTTP:
		if irVHost.Listener.Port != 80 {
			port = &irVHost.Listener.Port
		}
	case ir.IRProtocol_TCP, ir.IRProtocol_TLS:
		// for tls:// and tcp:// we always add the port
		port = &irVHost.Listener.Port
	}
	url += string(irVHost.Listener.Hostname)
	if port != nil {
		url = fmt.Sprintf("%s:%d", url, *port)
	}
	return url, nil
}

// buildCloudEndpoint initializes a new CloudEndpoint
func buildCloudEndpoint(irVHost *ir.IRVirtualHost) (*ngrokv1alpha1.CloudEndpoint, error) {
	name := ""
	if irVHost.NamePrefix != nil {
		name = fmt.Sprintf("%s-", *irVHost.NamePrefix)
	}
	name += string(irVHost.Listener.Hostname)

	if len(irVHost.AnnotationsToAdd) == 0 {
		irVHost.AnnotationsToAdd = nil
	}
	if len(irVHost.LabelsToAdd) == 0 {
		irVHost.LabelsToAdd = nil
	}

	publicURL, err := buildPublicURL(irVHost)
	if err != nil {
		return nil, fmt.Errorf("failed to build public URL for CloudEndpoint, err: %w", err)
	}

	return &ngrokv1alpha1.CloudEndpoint{
		ObjectMeta: metav1.ObjectMeta{
			Name:        sanitizeStringForK8sName(name),
			Namespace:   irVHost.Namespace,
			Labels:      irVHost.LabelsToAdd,
			Annotations: irVHost.AnnotationsToAdd,
		},
		Spec: ngrokv1alpha1.CloudEndpointSpec{
			URL:            publicURL,
			PoolingEnabled: irVHost.EndpointPoolingEnabled,
			Metadata:       irVHost.Metadata,
			Bindings:       irVHost.Bindings,
		},
	}, nil
}

// buildAgentEndpoint initializes a new AgentEndpoint
func buildAgentEndpoint(
	irVHost *ir.IRVirtualHost,
	irService ir.IRService,
	clusterDomain string,
	metadata string,
) (*ngrokv1alpha1.AgentEndpoint, error) {
	bindings := []string{}
	var url string
	if irVHost.CollapseIntoServiceKey != nil && irService.Key() == *irVHost.CollapseIntoServiceKey {
		publicURL, err := buildPublicURL(irVHost)
		if err != nil {
			return nil, fmt.Errorf("failed to build public URL for AgentEndpoint, err: %w", err)
		}
		url = publicURL
		bindings = irVHost.Bindings
	} else {
		internalURL, err := buildInternalEndpointURL(irVHost.Listener.Protocol, irService.UID, irService.Name, irService.Namespace, clusterDomain, irService.Port, irService.ClientCertRefs)
		if err != nil {
			return nil, fmt.Errorf("failed to build internal URL for AgentEndpoint, err: %w", err)
		}
		url = internalURL
	}

	if len(irVHost.AnnotationsToAdd) == 0 {
		irVHost.AnnotationsToAdd = nil
	}
	if len(irVHost.LabelsToAdd) == 0 {
		irVHost.LabelsToAdd = nil
	}

	ret := &ngrokv1alpha1.AgentEndpoint{
		ObjectMeta: metav1.ObjectMeta{
			Name:        internalAgentEndpointName(irService.UID, irService.Name, irService.Namespace, clusterDomain, irService.Port, irService.ClientCertRefs),
			Namespace:   irService.Namespace,
			Labels:      irVHost.LabelsToAdd,
			Annotations: irVHost.AnnotationsToAdd,
		},
		Spec: ngrokv1alpha1.AgentEndpointSpec{
			URL:      url,
			Metadata: metadata,
			Upstream: ngrokv1alpha1.EndpointUpstream{
				URL:      agentEndpointUpstreamURL(irService.Name, irService.Namespace, clusterDomain, irService.Port, irService.Scheme),
				Protocol: irService.Protocol,
			},
		},
	}

	if len(bindings) > 0 {
		ret.Spec.Bindings = bindings
	}

	for _, certRef := range irService.ClientCertRefs {
		ret.Spec.ClientCertificateRefs = append(ret.Spec.ClientCertificateRefs, ngrokv1alpha1.K8sObjectRefOptionalNamespace{
			Name:      certRef.Name,
			Namespace: &certRef.Namespace,
		})
	}

	return ret, nil
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
					"content":     "No route was found for this ngrok Endpoint",
					"headers": map[string]string{
						"content-type": "text/plain",
					},
				},
			},
		},
	}
}

// MappingStrategyAnnotationToIR checks the supplied object for the mapping strategy annotation and returns the appropriate mapping strategy enum if it is set, or falls back to the default strategy
func MappingStrategyAnnotationToIR(obj client.Object) (ir.IRMappingStrategy, error) {
	val, err := parser.GetStringAnnotation(annotations.MappingStrategyAnnotationKey, obj)
	if err != nil {
		if errors.IsMissingAnnotations(err) {
			return ir.IRMappingStrategy_EndpointsDefault, nil
		}
		return ir.IRMappingStrategy_EndpointsDefault, err
	}

	switch val {
	case string(annotations.MappingStrategy_Edges):
		return ir.IRMappingStrategy_Edges, nil
	case string(annotations.MappingStrategy_EndpointsDefault), string(ir.IRMappingStrategy_EndpointsCollapsed):
		return ir.IRMappingStrategy_EndpointsDefault, nil
	case string(annotations.MappingStrategy_EndpointsVerbose):
		return ir.IRMappingStrategy_EndpointsVerbose, nil
	}

	return ir.IRMappingStrategy_EndpointsDefault, fmt.Errorf("invalid value %q for %q annotation, defaulting to %q", val, annotations.MappingStrategyAnnotation, annotations.MappingStrategy_EndpointsDefault)
}
