package managerdriver

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/go-logr/logr"
	"github.com/ngrok/ngrok-operator/internal/annotations"
	"github.com/ngrok/ngrok-operator/internal/ir"
	"github.com/ngrok/ngrok-operator/internal/store"
	"github.com/ngrok/ngrok-operator/internal/trafficpolicy"
	corev1 "k8s.io/api/core/v1"
	netv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/types"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"
)

const (
	GatewayTLSOption_MinVersion   = "k8s.ngrok.com/min_version"
	GatewayTLSOption_MaxVersion   = "k8s.ngrok.com/max_version"
	GatewayTLSOption_MTLSStrategy = "k8s.ngrok.com/mutual_tls_verification_strategy"
)

// #region GWAPI to IR

type GatewayMatch struct {
	ParentGateway types.NamespacedName
	Hostname      string
}

// With ingress translation into ir/endpoints, we merge routes for the same hostname across all ingresses, but with
// gateway API, we will only merge routes for the same gateway.
func (t *translator) gatewayAPIToIR() []*ir.IRVirtualHost {
	// Note: currently we don't do anything with Gateway.Spec.Addresses. It might be possible to support them, but
	// given the nature of how ngrok endpoints work, I'm not sure it makes much sense for us to support this extended field

	// TODO: (Alice) add support for gateway.BackendTLS in a follow-up. It requires changes in the AgentEndpoint fields and handling

	virtualHostsPerGateway := make(map[types.NamespacedName]map[ir.IRListener]*ir.IRVirtualHost) // We key the list of virtual hosts by the gateway they are for
	upstreamCache := make(map[ir.IRService]*ir.IRUpstream)                                       // Each unique service/port combo corresponds to one IRUpstream
	gateways := t.store.ListGateways()
	httpRoutes := t.store.ListHTTPRoutes()

	// Add all of the gateways to a map for efficient lookup
	gatewayMap := make(map[types.NamespacedName]*gatewayv1.Gateway)
	for _, gateway := range gateways {
		gatewayMap[types.NamespacedName{
			Name:      gateway.Name,
			Namespace: gateway.Namespace,
		}] = gateway
	}

	for _, httpRoute := range httpRoutes {
		t.HTTPRouteToIR(httpRoute, upstreamCache, gatewayMap, virtualHostsPerGateway)
	}

	vHostSlice := []*ir.IRVirtualHost{}
	for _, vHostsForGateway := range virtualHostsPerGateway {
		for _, irVirtualHost := range vHostsForGateway {
			vHostSlice = append(vHostSlice, irVirtualHost)
		}
	}

	return vHostSlice
}

// #region HTTPRoute to IR

// HTTPRouteToIR translates a single HTTPRoute into IR by finding which Gateways it matches and adding the rules from the HTTPRoute
// as routes on the VirtualHost(s)
func (t *translator) HTTPRouteToIR(
	httpRoute *gatewayv1.HTTPRoute,
	upstreamCache map[ir.IRService]*ir.IRUpstream,
	gatewayMap map[types.NamespacedName]*gatewayv1.Gateway,
	virtualHostsPerGateway map[types.NamespacedName]map[ir.IRListener]*ir.IRVirtualHost,
) {
	vHostsMatchingRoute := make(map[*ir.IRVirtualHost]bool)

	// First, go through the HTTPRoute's parentRefs to find matching gateways and figure out which hostnames within those matching
	// gateways this HTTPRoute matches. Along the way, build/update virtual hosts for all the hostnames this HTTPRoute matches
	for _, parentRef := range httpRoute.Spec.ParentRefs {

		// Check matching Gateways for this HTTPRoute
		// The controller already filters the resources based on our gateway class, so no need to check that here
		refNamespace := string(httpRoute.Namespace)
		if parentRef.Namespace != nil {
			refNamespace = string(*parentRef.Namespace)
		}

		gatewayKey := types.NamespacedName{
			Name:      string(parentRef.Name),
			Namespace: refNamespace,
		}
		gateway, exists := gatewayMap[gatewayKey]
		if !exists {
			t.log.Error(fmt.Errorf("HTTPRoute parent ref not found"), "the HTTPRoute lists a Gateway parent ref that does not exist",
				"httproute", fmt.Sprintf("%s.%s", httpRoute.Name, httpRoute.Namespace),
				"parentRef", fmt.Sprintf("%s.%s", string(parentRef.Name), refNamespace),
			)
			continue
		}

		// We currently require this annotation to be present for an Ingress to be translated into CloudEndpoints/AgentEndpoints, otherwise the default behaviour is to
		// translate it into HTTPSEdges (legacy). A future version will remove support for HTTPSEdges and translation into CloudEndpoints/AgentEndpoints will become the new
		// default behaviour.
		useEndpoints, err := annotations.ExtractUseEndpoints(gateway)
		if err != nil {
			t.log.Error(err, fmt.Sprintf("failed to check %q annotation. defaulting to using edges", annotations.MappingStrategyAnnotation))
		}
		if !useEndpoints {
			t.log.Info(fmt.Sprintf("the Gateway and its HTTPRoutes will be provided by ngrok edges instead of endpoints because of the %q annotation",
				annotations.MappingStrategyAnnotation),
				"gateway", fmt.Sprintf("%s.%s", gateway.Name, gateway.Namespace),
			)
			continue
		}

		useEndpointPooling, err := annotations.ExtractUseEndpointPooling(gateway)
		if err != nil {
			t.log.Error(err, fmt.Sprintf("failed to check %q annotation", annotations.MappingStrategyAnnotation))
		}
		if useEndpointPooling {
			t.log.Info(fmt.Sprintf("the following Gateway and its HTTPRoutes will create endpoint(s) with pooling enabled because of the %q annotation",
				annotations.MappingStrategyAnnotation),
				"gateway", fmt.Sprintf("%s.%s", gateway.Name, gateway.Namespace),
			)
		}

		annotationTrafficPolicy, tpObjRef, err := trafficPolicyFromAnnotation(t.store, gateway)
		if err != nil {
			t.log.Error(err, "error getting ngrok traffic policy for gateway",
				"gateway", fmt.Sprintf("%s.%s", gateway.Name, gateway.Namespace))
			continue
		}

		// If we don't have a native traffic policy from annotations, see if one was provided from a moduleset annotation
		if annotationTrafficPolicy == nil {
			annotationTrafficPolicy, tpObjRef, err = trafficPolicyFromModSetAnnotation(t.log, t.store, gateway, useEndpoints)
			if err != nil {
				t.log.Error(err, "error getting ngrok traffic policy for gateway",
					"gateway", fmt.Sprintf("%s.%s", gateway.Name, gateway.Namespace))
				continue
			}
		}

		matchingListeners := matchingGatewayListenersForHTTPRoute(t.log, gateway, httpRoute)
		for _, matchingListener := range matchingListeners {
			tlsTermCfg := matchingListener.TLS
			if tlsTermCfg != nil {
				if tlsTermCfg.Mode != nil && *tlsTermCfg.Mode == gatewayv1.TLSModePassthrough {
					t.log.Error(fmt.Errorf("TLS passthrough mode is not currently supported"), "skipping gateway listener for HTTPRoutes because the tls mode is set to passthrough",
						"gateway", fmt.Sprintf("%s.%s", gateway.Name, gateway.Namespace),
					)
					continue
				}
			}
			irTLSTermination, err := gatewayTLSConfigToIR(t.log, t.store, tlsTermCfg, gateway)
			if err != nil {
				t.log.Error(err, "skipping gateway listener with invalid TLS configuration",
					"gateway", fmt.Sprintf("%s.%s", gateway.Name, gateway.Namespace),
				)
				continue
			}

			// Check if this Gateway already has any virtual hosts
			vHostsForCurrentGateway, exists := virtualHostsPerGateway[gatewayKey]
			if !exists {
				// Initialize the underlying map if needed
				vHostsForCurrentGateway = make(map[ir.IRListener]*ir.IRVirtualHost)
				virtualHostsPerGateway[gatewayKey] = vHostsForCurrentGateway
			}

			// Check if this Gateway already has an irVHost for this specific hostname, otherwise make one
			irListener := ir.IRListener{
				Hostname: ir.IRHostname(*matchingListener.Hostname),
				Port:     int32(matchingListener.Port),
			}

			switch matchingListener.Protocol {
			case gatewayv1.HTTPProtocolType:
				irListener.Protocol = ir.IRProtocol_HTTP
			case gatewayv1.HTTPSProtocolType:
				irListener.Protocol = ir.IRProtocol_HTTPS
			default:
				t.log.Error(fmt.Errorf("gateway with unsupported listener protocol"), "currently only HTTPRoutes are supported. listeners with TCP/TLS/UDP protocols will be skipped",
					"gateway", fmt.Sprintf("%s.%s", gateway.Name, gateway.Namespace),
				)
				continue
			}

			irVHost, exists := vHostsForCurrentGateway[irListener]
			if !exists {
				// Add a name prefix with the gateway name so that we can support endpoint pooling across multiple gateways
				namePrefix := fmt.Sprintf("%s.%s", gateway.Name, gateway.Namespace)
				irVHost = &ir.IRVirtualHost{
					NamePrefix:             &namePrefix,
					Namespace:              gateway.Namespace,
					Listener:               irListener,
					TLSTermination:         irTLSTermination,
					LabelsToAdd:            t.managedResourceLabels,
					AnnotationsToAdd:       make(map[string]string),
					EndpointPoolingEnabled: useEndpointPooling,
					TrafficPolicy:          annotationTrafficPolicy,
					TrafficPolicyObj:       tpObjRef,
					Metadata:               t.defaultGatewayMetadata,
				}
			}
			irVHost.AddOwningResource(ir.OwningResource{
				Kind:      "Gateway",
				Name:      gateway.Name,
				Namespace: gateway.Namespace,
			})
			irVHost.AddOwningResource(ir.OwningResource{
				Kind:      "HTTPRoute",
				Name:      httpRoute.Name,
				Namespace: httpRoute.Namespace,
			})
			if gateway.Spec.Infrastructure != nil {
				for key, val := range gateway.Spec.Infrastructure.Labels {
					irVHost.LabelsToAdd[string(key)] = string(val)
				}
				for key, val := range gateway.Spec.Infrastructure.Annotations {
					irVHost.AnnotationsToAdd[string(key)] = string(val)
				}
			}
			vHostsMatchingRoute[irVHost] = true
			vHostsForCurrentGateway[irListener] = irVHost
		}
	}

	// Now that we have a set of the virtual hosts that are applicable to this HTTPRoute, go through and build new routes
	// for all the HTTPRoute rules and add them to the matching virtual hosts
	routesToAdd := []*ir.IRRoute{}
	for _, rule := range httpRoute.Spec.Rules {
		// For each rule.Match create a route
		for _, match := range rule.Matches {
			irRoute := &ir.IRRoute{
				MatchCriteria:   GatewayAPIHTTPMatchToIR(match),
				TrafficPolicies: []*trafficpolicy.TrafficPolicy{},
			}

			for _, filter := range rule.Filters {
				// For each GatewayAPI filter for the route, we will inject additional config into the route's traffic policy
				filterTrafficPolicy, err := gatewayAPIFilterToTrafficPolicy(filter, httpRoute.Namespace, t.store, irRoute.MatchCriteria)
				if err != nil {
					t.log.Error(err, "skipping filter with error")
					continue
				}
				irRoute.TrafficPolicies = append(irRoute.TrafficPolicies, filterTrafficPolicy)
			}

			for _, backendRef := range rule.BackendRefs {
				irDestination, err := t.httpRouteBackendToIR(httpRoute, backendRef, upstreamCache, irRoute.MatchCriteria)
				if err != nil {
					t.log.Error(err, "unable to translate HTTPRoute backend ref",
						"HTTPRoute", fmt.Sprintf("%s.%s", httpRoute.Name, httpRoute.Namespace),
					)
					continue
				}
				irRoute.Destinations = append(irRoute.Destinations, irDestination)
			}

			if len(irRoute.TrafficPolicies) > 0 || len(irRoute.Destinations) > 0 {
				routesToAdd = append(routesToAdd, irRoute)
			}
		}
	}

	// Add all the routes we just processed to all matching virtual hosts
	for irVHost := range vHostsMatchingRoute {
		for _, routeToAdd := range routesToAdd {
			for _, destination := range routeToAdd.Destinations {
				// Inherit all the virtual host's owning resources
				if destination.Upstream != nil {
					for _, owningResource := range irVHost.OwningResources {
						destination.Upstream.AddOwningResource(owningResource)
					}
				}
			}
			irVHost.Routes = append(irVHost.Routes, routeToAdd)
		}
	}
}

// #region Find Gateway listners for HTTPRoute

// matchingGatewayListenersForHTTPRoute takes a Gateway and an HTTPRoute and figures out which (if any) listeners from the Gateway the HTTPRoute matches
func matchingGatewayListenersForHTTPRoute(log logr.Logger, gateway *gatewayv1.Gateway, httpRoute *gatewayv1.HTTPRoute) []gatewayv1.Listener {
	matchingListeners := []gatewayv1.Listener{}

	for _, listener := range gateway.Spec.Listeners {
		// When allowedRoutes is not specified, only routes in the same namespace as the gateway are allowed
		if listener.AllowedRoutes == nil && (gateway.Namespace != httpRoute.Namespace) {
			return matchingListeners
		}

		if listener.AllowedRoutes != nil {
			allowedKind := true // Default to allowing HTTPRoutes in the same namespace when not specified
			if len(listener.AllowedRoutes.Kinds) > 0 {
				allowedKind = false
				for _, kind := range listener.AllowedRoutes.Kinds {
					if kind.Kind == "HTTPRoute" {
						allowedKind = true
						break
					}
				}
			}
			if !allowedKind {
				return matchingListeners
			}

			// Validate namespaces
			if listener.AllowedRoutes.Namespaces != nil {
				nsPolicy := listener.AllowedRoutes.Namespaces.From
				if nsPolicy != nil {
					switch *nsPolicy {
					case gatewayv1.NamespacesFromSame:
						if httpRoute.Namespace != gateway.Namespace {
							continue
						}
					case gatewayv1.NamespacesFromSelector:
						if listener.AllowedRoutes.Namespaces.Selector == nil {
							continue
						}
						// Check if the namespace matches the selector
						selector, err := metav1.LabelSelectorAsSelector(listener.AllowedRoutes.Namespaces.Selector)
						if err != nil || !selector.Matches(labels.Set(httpRoute.Labels)) {
							continue
						}
					}
				}
			}
		}

		// Handle listener hostnames
		if listener.Hostname == nil {
			log.Error(fmt.Errorf("gateway has a listener with a nil hostname"), "Gateway listeners with nil hostnames are not supported, gateway listeners must have a valid non-empty hostname other than \"*\". Invalid listeners will be skipped.",
				"gateway", fmt.Sprintf("%s.%s", gateway.Name, gateway.Namespace),
			)
			continue
		}

		listenerHostname := string(*listener.Hostname)
		if listenerHostname == "*" {
			log.Error(fmt.Errorf("gateway has a listener with hostname \"*\""), "Gateway listeners with hostname \"*\" are not supported, gateway listeners must have a valid non-empty hostname other than \"*\". Invalid listeners will be skipped.",
				"gateway", fmt.Sprintf("%s.%s", gateway.Name, gateway.Namespace),
			)
			continue
		}
		if listenerHostname == "" {
			log.Error(fmt.Errorf("gateway has a listener with an empty hostname"), "Gateway listeners with empty hostnames are not supported, gateway listeners must have a valid non-empty hostname other than \"*\". Invalid listeners will be skipped.",
				"gateway", fmt.Sprintf("%s.%s", gateway.Name, gateway.Namespace),
			)
			continue
		}

		// When the HTTPRoute hostnames are empty, it matches all listeners for all parent gateways
		if len(httpRoute.Spec.Hostnames) == 0 {
			matchingListeners = append(matchingListeners, listener)
			return matchingListeners
		}

		// Check matches for valid hostnames
		for _, routeHostname := range httpRoute.Spec.Hostnames {
			if routeHostname == "*" {
				matchingListeners = append(matchingListeners, listener)
				break
			}
			match, err := doHostGlobsMatch(listenerHostname, string(routeHostname))
			if err != nil {
				log.Error(err, "unable to compile hostname glob for Gateway listener hostname, this listener will be skipped",
					"gateway", fmt.Sprintf("%s.%s", gateway.Name, gateway.Namespace),
					"listener hostname", listenerHostname,
				)
				continue
			}
			if match {
				matchingListeners = append(matchingListeners, listener)
			}
		}
	}

	return matchingListeners
}

// #region HTTPMatch to IR

// GatewayAPIHTTPMatchToIR translates an HTTPRouteMatch into an IRHTTPMatch
func GatewayAPIHTTPMatchToIR(match gatewayv1.HTTPRouteMatch) ir.IRHTTPMatch {
	// GatewayAPI specifies that when nil, the default path match behaviour should be a prefix match on "/"
	path := "/"
	pathType := ir.IRPathType_Prefix

	if match.Path != nil {
		if match.Path.Value != nil {
			path = string(*match.Path.Value)
		}
		if match.Path.Type != nil {
			switch *match.Path.Type {
			// We already default to prefix match, no need to check for it
			case gatewayv1.PathMatchExact:
				pathType = ir.IRPathType_Exact
			case gatewayv1.PathMatchRegularExpression:
				pathType = ir.IRPathType_Regex
			}
		}
	}

	requiredHeaders := []ir.IRHeaderMatch{}
	for _, header := range match.Headers {
		headerValueType := ir.IRStringValueType_Exact
		if header.Type != nil && *header.Type == gatewayv1.HeaderMatchRegularExpression {
			headerValueType = ir.IRStringValueType_Regex
		}
		requiredHeaders = append(requiredHeaders, ir.IRHeaderMatch{
			Name:      string(header.Name),
			Value:     header.Value,
			ValueType: headerValueType,
		})
	}

	requiredQueryParams := []ir.IRQueryParamMatch{}
	for _, queryParam := range match.QueryParams {
		queryValueType := ir.IRStringValueType_Exact
		if queryParam.Type != nil && *queryParam.Type == gatewayv1.QueryParamMatchRegularExpression {
			queryValueType = ir.IRStringValueType_Regex
		}
		requiredQueryParams = append(requiredQueryParams, ir.IRQueryParamMatch{
			Name:      string(queryParam.Name),
			Value:     queryParam.Value,
			ValueType: queryValueType,
		})
	}

	ret := ir.IRHTTPMatch{
		Path:        &path,
		PathType:    &pathType,
		Headers:     requiredHeaders,
		QueryParams: requiredQueryParams,
	}

	if match.Method != nil {
		ret.Method = gatewayMethodToIR(match.Method)
	}

	return ret
}

// gatewayMethodToIR translates a GatewayAPI HTTPMethod into an IRMethodMatch
func gatewayMethodToIR(method *gatewayv1.HTTPMethod) *ir.IRMethodMatch {
	if method == nil {
		return nil
	}

	var requiredMethod ir.IRMethodMatch
	switch *method {
	case gatewayv1.HTTPMethodGet:
		requiredMethod = ir.IRMethodMatch_Get
	case gatewayv1.HTTPMethodHead:
		requiredMethod = ir.IRMethodMatch_Head
	case gatewayv1.HTTPMethodPost:
		requiredMethod = ir.IRMethodMatch_Post
	case gatewayv1.HTTPMethodPut:
		requiredMethod = ir.IRMethodMatch_Put
	case gatewayv1.HTTPMethodDelete:
		requiredMethod = ir.IRMethodMatch_Delete
	case gatewayv1.HTTPMethodConnect:
		requiredMethod = ir.IRMethodMatch_Connect
	case gatewayv1.HTTPMethodOptions:
		requiredMethod = ir.IRMethodMatch_Options
	case gatewayv1.HTTPMethodTrace:
		requiredMethod = ir.IRMethodMatch_Trace
	case gatewayv1.HTTPMethodPatch:
		requiredMethod = ir.IRMethodMatch_Patch
	}
	return &requiredMethod
}

// #region GWAPI Filters translation

// gatewayAPIFilterToTrafficPolicy translates Gateway API filters into traffic policy config
func gatewayAPIFilterToTrafficPolicy(filter gatewayv1.HTTPRouteFilter, namespace string, store store.Storer, matchCriteria ir.IRHTTPMatch) (*trafficpolicy.TrafficPolicy, error) {
	sharedErr := fmt.Errorf("unable to convert gateway API filter to traffic policy config")

	switch filter.Type {
	case gatewayv1.HTTPRouteFilterRequestHeaderModifier:
		return gwapiRequestHeaderFilterToTrafficPolicy(filter)
	case gatewayv1.HTTPRouteFilterResponseHeaderModifier:
		return gwapiResponseHeaderFilterToTrafficPolicy(filter)
	case gatewayv1.HTTPRouteFilterRequestRedirect:
		return gwapiRedirectFilterToTrafficPolicy(filter, matchCriteria)
	case gatewayv1.HTTPRouteFilterURLRewrite:
		return gwapiURLRewriteFilterToTrafficPolicy(filter, matchCriteria)
	case gatewayv1.HTTPRouteFilterRequestMirror:
		// TODO: (Alice) this can be supported when http-request is finished (at least for HTTP/HTTPS upstreams)
		return nil, fmt.Errorf("%w: request mirror filters are not currently supported", sharedErr)
	case gatewayv1.HTTPRouteFilterExtensionRef:
		extensionRef := filter.ExtensionRef
		if extensionRef == nil {
			return nil, fmt.Errorf("%w: filter type specified as ExtensionRef but the section config was nil", sharedErr)
		}
		if !strings.EqualFold(string(extensionRef.Kind), "NgrokTrafficPolicy") {
			return nil, fmt.Errorf("%w: extension ref filter has unknown kind %q. only NgrokTrafficPolicy is currently supported", sharedErr, string(extensionRef.Kind))
		}

		if group := string(extensionRef.Group); group != "" && !strings.EqualFold(group, "ngrok.k8s.ngrok.com") {
			return nil, fmt.Errorf("%w: extension ref filter has unknown group %q. only \"ngrok.k8s.ngrok.com\" is currently supported", sharedErr, group)
		}

		routePolicyCfg, err := store.GetNgrokTrafficPolicyV1(string(extensionRef.Name), namespace)
		if err != nil {
			return nil, fmt.Errorf("unable to resolve traffic policy backend for ingress rule: %w", err)
		}

		var routeTrafficPolicy trafficpolicy.TrafficPolicy
		if len(routePolicyCfg.Spec.Policy) > 0 {
			if err := json.Unmarshal(routePolicyCfg.Spec.Policy, &routeTrafficPolicy); err != nil {
				return nil, fmt.Errorf("failed to unmarshal traffic policy: %w. raw traffic policy: %v", err, routePolicyCfg.Spec.Policy)
			}
		}

		if len(routeTrafficPolicy.OnTCPConnect) != 0 {
			return nil, fmt.Errorf("traffic policies supplied as external ref filters may not contain any on_tcp_connect rules as there is no way to only run them for certain routes")
		}

		return &routeTrafficPolicy, nil
	}

	return nil, fmt.Errorf("%w: filter type %q could not be identified", sharedErr, string(filter.Type))
}

// #region Request Header Filter

// gwapiRequestHeaderFilterToTrafficPolicy translates a GatewayAPI request header filter into traffic policy config
func gwapiRequestHeaderFilterToTrafficPolicy(filter gatewayv1.HTTPRouteFilter) (*trafficpolicy.TrafficPolicy, error) {
	requestHeaders := filter.RequestHeaderModifier
	if requestHeaders == nil {
		return nil, fmt.Errorf("filter type specified as RequestHeaderModifier but the section config was nil")
	}

	headersToRemove := []string{}
	headersToAdd := make(map[string]string)

	headersToRemove = append(headersToRemove, requestHeaders.Remove...)

	for _, header := range requestHeaders.Add {
		headersToAdd[string(header.Name)] = header.Value
	}

	for _, header := range requestHeaders.Set {
		// Since the traffic policy add header operation always adds/appends, we need to remove them first for the "Set" behaviour
		headersToRemove = append(headersToRemove, string(header.Name))
		headersToAdd[string(header.Name)] = header.Value
	}

	ret := trafficpolicy.NewTrafficPolicy()
	ret.OnHTTPRequest = []trafficpolicy.Rule{{
		Name: "GatewayAPI-Request-Header-Filter",
		Actions: []trafficpolicy.Action{
			{
				Type: trafficpolicy.ActionType_RemoveHeaders,
				Config: map[string]interface{}{
					"headers": headersToRemove,
				},
			},
			{
				Type: trafficpolicy.ActionType_AddHeaders,
				Config: map[string]interface{}{
					"headers": headersToAdd,
				},
			},
		},
	}}

	return ret, nil
}

// #region Response Header Filter

// gwapiRequestHeaderFilterToTrafficPolicy translates a GatewayAPI response header filter into traffic policy config
func gwapiResponseHeaderFilterToTrafficPolicy(filter gatewayv1.HTTPRouteFilter) (*trafficpolicy.TrafficPolicy, error) {
	responseHeaders := filter.ResponseHeaderModifier
	if responseHeaders == nil {
		return nil, fmt.Errorf("filter type specified as ResponseHeaderModifier but the section config was nil")
	}

	headersToRemove := []string{}
	headersToAdd := make(map[string]string)

	headersToRemove = append(headersToRemove, responseHeaders.Remove...)

	for _, header := range responseHeaders.Add {
		headersToAdd[string(header.Name)] = header.Value
	}

	for _, header := range responseHeaders.Set {
		// Since the traffic policy add header operation always adds/appends, we need to remove them first for the "Set" behaviour
		headersToRemove = append(headersToRemove, string(header.Name))
		headersToAdd[string(header.Name)] = header.Value
	}

	ret := trafficpolicy.NewTrafficPolicy()
	ret.OnHTTPResponse = []trafficpolicy.Rule{{
		Name: "GatewayAPI-Response-Header-Filter",
		Actions: []trafficpolicy.Action{
			{
				Type: trafficpolicy.ActionType_RemoveHeaders,
				Config: map[string]interface{}{
					"headers": headersToRemove,
				},
			},
			{
				Type: trafficpolicy.ActionType_AddHeaders,
				Config: map[string]interface{}{
					"headers": headersToAdd,
				},
			},
		},
	}}

	return ret, nil
}

// #region Redirect Filter

// gwapiRequestHeaderFilterToTrafficPolicy translates a GatewayAPI redirect filter into traffic policy config
func gwapiRedirectFilterToTrafficPolicy(filter gatewayv1.HTTPRouteFilter, matchCriteria ir.IRHTTPMatch) (*trafficpolicy.TrafficPolicy, error) {
	redirect := filter.RequestRedirect
	if redirect == nil {
		return nil, fmt.Errorf("filter type specified as RequestRedirect but the section config was nil")
	}

	prefix := ""
	if matchCriteria.Path != nil {
		prefix = *matchCriteria.Path
		prefix = strings.ReplaceAll(prefix, "/", `\/`)
	}

	from := fmt.Sprintf(`^(?P<scheme>[a-zA-Z][a-zA-Z0-9+\-.]*):\/\/(?P<hostname>[^\/:]+)(?P<port>:\d+)?(?P<prefix>%s)(?P<remaining>.*)$`, prefix)
	toScheme := "$1://"
	toHostname := "$2"
	toPort := "$3"
	toPrefix := "$4"
	toRemainingPath := "$5"

	if redirect.Scheme != nil {
		toScheme = fmt.Sprintf("%s://", *redirect.Scheme)
	}

	if redirect.Hostname != nil {
		toHostname = string(*redirect.Hostname)
	}

	if redirect.Path != nil {
		switch redirect.Path.Type {
		case gatewayv1.FullPathHTTPPathModifier:
			if redirect.Path.ReplaceFullPath == nil {
				return nil, fmt.Errorf("ReplaceFullPath type specified but replaceFullPath is nil")
			}
			toPrefix = ""
			toRemainingPath = *redirect.Path.ReplaceFullPath
		case gatewayv1.PrefixMatchHTTPPathModifier:
			if redirect.Path.ReplacePrefixMatch == nil {
				return nil, fmt.Errorf("ReplacePrefixMatch type specified but replacePrefixPatch is nil")
			}
			toPrefix = *redirect.Path.ReplacePrefixMatch
		}
	}

	if redirect.Port != nil {
		portVal := int(*redirect.Port)
		toPort = fmt.Sprintf(":%d", portVal)
		if redirect.Scheme != nil && *redirect.Scheme != "" {
			switch strings.ToLower(*redirect.Scheme) {
			case "http":
				if portVal == 80 {
					toPort = "$3"
				}
			case "https":
				if portVal != 443 {
					toPort = "$3"
				}
			}
		}
	}

	statusCode := 302
	if redirect.StatusCode != nil {
		statusCode = *redirect.StatusCode
	}

	ret := trafficpolicy.NewTrafficPolicy()
	ret.OnHTTPRequest = []trafficpolicy.Rule{{
		Name: "GatewayAPI-Redirect-Filter",
		Actions: []trafficpolicy.Action{
			{
				Type: trafficpolicy.ActionType_Redirect,
				Config: map[string]interface{}{
					"from": from,
					"to": fmt.Sprintf("%s%s%s%s%s",
						toScheme,
						toHostname,
						toPort,
						toPrefix,
						toRemainingPath,
					),
					"status_code": statusCode,
				},
			},
		},
	}}

	return ret, nil
}

// #region URL Rewrite Filter

// gwapiRequestHeaderFilterToTrafficPolicy translates a GatewayAPI url rewrite filter into traffic policy config
func gwapiURLRewriteFilterToTrafficPolicy(filter gatewayv1.HTTPRouteFilter, matchCriteria ir.IRHTTPMatch) (*trafficpolicy.TrafficPolicy, error) {
	rewrite := filter.URLRewrite
	if rewrite == nil {
		return nil, fmt.Errorf("filter type specified as URLRewrite but the section config was nil")
	}

	if rewrite.Hostname == nil && rewrite.Path == nil {
		return nil, fmt.Errorf("URLRewrite filter must specify at least one of hostname or path")
	}

	prefix := ""
	if matchCriteria.Path != nil {
		prefix = *matchCriteria.Path
		prefix = strings.ReplaceAll(prefix, "/", `\/`)
	}

	from := fmt.Sprintf(`^(?P<scheme>[a-zA-Z][a-zA-Z0-9+\-.]*):\/\/(?P<hostname>[^\/:]+)(?P<port>:\d+)?(?P<prefix>%s)(?P<remaining>.*)$`, prefix)
	toScheme := "$1://"
	toHostname := "$2"
	toPort := "$3"
	toPrefix := "$4"
	toRemainingPath := "$5"

	if rewrite.Hostname != nil {
		toHostname = string(*rewrite.Hostname)
	}

	if rewrite.Path != nil {
		switch rewrite.Path.Type {
		case gatewayv1.FullPathHTTPPathModifier:
			if rewrite.Path.ReplaceFullPath == nil {
				return nil, fmt.Errorf("ReplaceFullPath type specified but replaceFullPath is nil")
			}
			toPrefix = ""
			toRemainingPath = *rewrite.Path.ReplaceFullPath

		case gatewayv1.PrefixMatchHTTPPathModifier:
			if rewrite.Path.ReplacePrefixMatch == nil {
				return nil, fmt.Errorf("ReplacePrefixMatch type specified but replacePrefixMatch is nil")
			}
			toPrefix = *rewrite.Path.ReplacePrefixMatch
		}
	}

	ret := trafficpolicy.NewTrafficPolicy()
	ret.OnHTTPRequest = []trafficpolicy.Rule{{
		Name: "GatewayAPI-URL-Rewrite-Filter",
		Actions: []trafficpolicy.Action{
			{
				Type: trafficpolicy.ActionType_URLRewrite,
				Config: map[string]interface{}{
					"from": from,
					"to": fmt.Sprintf("%s%s%s%s%s",
						toScheme,
						toHostname,
						toPort,
						toPrefix,
						toRemainingPath,
					),
				},
			},
		},
	}}

	return ret, nil
}

// #region HTTPRoute BackendRef IR

// gwapiRequestHeaderFilterToTrafficPolicy translates a GatewayAPI backendRef into IR
func (t *translator) httpRouteBackendToIR(httpRoute *gatewayv1.HTTPRoute, backendRef gatewayv1.HTTPBackendRef, upstreamCache map[ir.IRService]*ir.IRUpstream, matchCriteria ir.IRHTTPMatch) (*ir.IRDestination, error) {
	destination := &ir.IRDestination{
		TrafficPolicies: []*trafficpolicy.TrafficPolicy{},
	}

	if backendRef.Weight != nil {
		weight := int(*backendRef.Weight)
		destination.Weight = &weight
	}

	for _, filter := range backendRef.Filters {
		filterPolicy, err := gatewayAPIFilterToTrafficPolicy(filter, httpRoute.Namespace, t.store, matchCriteria)
		if err != nil {
			t.log.Error(err, "unable to process HTTPRoute backendRef filter",
				"httpRoute", fmt.Sprintf("%s.%s", httpRoute.Name, httpRoute.Namespace),
				"backendRef", backendRef,
				"filter", filter,
			)
			continue
		}
		destination.TrafficPolicies = append(destination.TrafficPolicies, filterPolicy)
	}

	if backendRef.Kind != nil && !strings.EqualFold(string(*backendRef.Kind), "Service") {
		return nil, fmt.Errorf("invalid backendRef kind supplied to HTTPRoute. only Service backends are currently supported")
	}

	serviceName := string(backendRef.Name)
	if serviceName == "" {
		return destination, nil
	}

	if backendRef.Port == nil {
		return nil, fmt.Errorf("backendRef supplied to HTTPRoute is missing the required port. name: %q, namespace: %q",
			serviceName,
			httpRoute.Namespace,
		)
	}

	// TODO: (Alice) add support for referenceGrants on the namespace in a follow-up

	service, err := t.store.GetServiceV1(serviceName, httpRoute.Namespace)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve Service for backendRef name: %q in namespace %q: %w",
			serviceName,
			httpRoute.Namespace,
			err,
		)
	}

	servicePort, err := findServicesPort(t.log, service, netv1.ServiceBackendPort{Number: int32(*backendRef.Port)})
	if err != nil || servicePort == nil {
		return nil, fmt.Errorf("failed to resolve backendRef Service's port. name: %q, namespace: %q: %w",
			serviceName,
			httpRoute.Namespace,
			err,
		)
	}

	irService := ir.IRService{
		UID:       string(service.UID),
		Name:      serviceName,
		Namespace: httpRoute.Namespace,
		Port:      servicePort.Port,
	}
	upstream, exists := upstreamCache[irService]
	if !exists {
		upstream = &ir.IRUpstream{
			Service: irService,
		}
		upstreamCache[irService] = upstream
	}
	destination.Upstream = upstream

	return destination, nil
}

// #region GatewayTLS IR

// gwapiRequestHeaderFilterToTrafficPolicy translates a GatewayAPI tls configuration into IR
func gatewayTLSConfigToIR(log logr.Logger, store store.Storer, tlsCfg *gatewayv1.GatewayTLSConfig, gateway *gatewayv1.Gateway) (*trafficpolicy.TLSTerminationConfig, error) {
	if tlsCfg == nil {
		return nil, nil
	}

	tlsTermCfg := &trafficpolicy.TLSTerminationConfig{
		MutualTLSCertificateAuthorities: []string{},
	}

	if len(tlsCfg.CertificateRefs) > 0 {
		if len(tlsCfg.CertificateRefs) > 1 {
			log.Error(fmt.Errorf("multiple Gateway TLS certificateRefs provided"), "Only the first will be used, multiple are not currently supported")
		}

		certRef := tlsCfg.CertificateRefs[0]
		if certRef.Kind != nil && !strings.EqualFold(string(*certRef.Kind), "Secret") {
			return nil, fmt.Errorf("unsupported kind %q for Gateway TLS config. only core api group secret references are supported", string(*certRef.Kind))
		}

		refNamespace := gateway.Namespace
		if certRef.Namespace != nil {
			refNamespace = string(*certRef.Namespace)
			// TODO: (Alice) add support for referenceGrants on the namespace in a follow-up
		}

		secret, err := store.GetSecretV1(string(certRef.Name), refNamespace)
		if err != nil {
			return nil, fmt.Errorf("%w: unable to resolve secret reference %q in Gateway TLS config",
				err,
				fmt.Sprintf("%s.%s", string(certRef.Name), refNamespace),
			)
		}

		if secret.Type != corev1.SecretTypeTLS {
			return nil, fmt.Errorf("secret %q is not of type kubernetes.io/tls (got: %q)",
				fmt.Sprintf("%s.%s", string(certRef.Name), refNamespace),
				secret.Type,
			)
		}

		// Pull out the private key (tls.key)
		privateKeyData, ok := secret.Data[corev1.TLSPrivateKeyKey]
		if !ok {
			return nil, fmt.Errorf("secret %q is missing tls.key data",
				fmt.Sprintf("%s.%s", string(certRef.Name), refNamespace),
			)
		}
		privateKey := string(privateKeyData)

		// Pull out the certificate (tls.crt)
		certData, ok := secret.Data[corev1.TLSCertKey]
		if !ok {
			return nil, fmt.Errorf("secret %q is missing tls.crt data",
				fmt.Sprintf("%s.%s", string(certRef.Name), refNamespace),
			)
		}
		cert := string(certData)

		tlsTermCfg.ServerCertificate = &cert
		tlsTermCfg.ServerPrivateKey = &privateKey
	}

	// Next, check if there is mTLS config
	if tlsCfg.FrontendValidation != nil {
		for _, certRef := range tlsCfg.FrontendValidation.CACertificateRefs {
			refNamespace := gateway.Namespace
			if certRef.Namespace != nil {
				refNamespace = string(*certRef.Namespace)
				// TODO: (Alice) add support for referenceGrants on the namespace in a follow-up
			}

			if !strings.EqualFold(string(certRef.Kind), "ConfigMap") {
				return nil, fmt.Errorf("unsupported kind %q for Gateway frontend TLS config reference %s. only core api group ConfigMap references are supported",
					certRef.Kind,
					fmt.Sprintf("%s.%s", string(certRef.Name), refNamespace),
				)
			}

			configMap, err := store.GetConfigMapV1(string(certRef.Name), refNamespace)
			if err != nil {
				return nil, fmt.Errorf("%w: unable to resolve ConfigMap reference %q in Gateway frontend TLS config",
					err,
					fmt.Sprintf("%s.%s", string(certRef.Name), refNamespace),
				)
			}

			ca, exists := configMap.Data["ca.crt"]
			if !exists {
				return nil, fmt.Errorf("configmap %q is missing ca.crt data",
					fmt.Sprintf("%s.%s", string(certRef.Name), refNamespace),
				)
			}

			tlsTermCfg.MutualTLSCertificateAuthorities = append(tlsTermCfg.MutualTLSCertificateAuthorities, ca)
		}
	}

	if minTLSVersion, exists := tlsCfg.Options[GatewayTLSOption_MinVersion]; exists {
		minTLSVersionString := string(minTLSVersion)
		tlsTermCfg.MinVersion = &minTLSVersionString
	}
	if maxTLSVersion, exists := tlsCfg.Options[GatewayTLSOption_MaxVersion]; exists {
		maxTLSVersionString := string(maxTLSVersion)
		tlsTermCfg.MaxVersion = &maxTLSVersionString
	}
	if mtlsStrat, exists := tlsCfg.Options[GatewayTLSOption_MTLSStrategy]; exists {
		mtlsStratString := string(mtlsStrat)
		tlsTermCfg.MutualTLSVerificationStrategy = &mtlsStratString
	}

	return tlsTermCfg, nil
}
