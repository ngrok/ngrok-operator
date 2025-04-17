package managerdriver

import (
	"encoding/json"
	"fmt"
	"net"
	"strings"

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
	gatewayv1alpha2 "sigs.k8s.io/gateway-api/apis/v1alpha2"
)

const (
	// Within the gateway, any keys in the tls.options field with this prefix get added to the terminate-tls action
	TLSOptionKeyPrefix = "k8s.ngrok.com/terminate-tls."
)

var (
	// These keys may not be supplied to the gateway listener's tls.options field since they are supported elsewhere and we don't want conflicts
	TLSOptionKeyReservedKeys = []string{
		"k8s.ngrok.com/terminate-tls.server_private_key",
		"k8s.ngrok.com/terminate-tls.server_certificate",
		"k8s.ngrok.com/terminate-tls.mutual_tls_certificate_authorities",
	}
)

// #region GWAPI to IR

type GatewayMatch struct {
	ParentGateway types.NamespacedName
	Hostname      string
}

// With ingress translation into ir/endpoints, we merge routes for the same hostname across all ingresses, but with
// gateway API, we will only merge routes for the same gateway.
func (t *translator) gatewayAPIToIR() []*ir.IRVirtualHost {
	virtualHostsPerGateway := make(map[types.NamespacedName]map[ir.IRListener]*ir.IRVirtualHost) // We key the list of virtual hosts by the gateway they are for
	upstreamCache := make(map[ir.IRServiceKey]*ir.IRUpstream)                                    // Each unique service/port combo corresponds to one IRUpstream
	gateways := t.store.ListGateways()

	// Add all of the gateways to a map for efficient lookup
	gatewayMap := make(map[types.NamespacedName]*gatewayv1.Gateway)
	for _, gateway := range gateways {
		gatewayMap[types.NamespacedName{
			Name:      gateway.Name,
			Namespace: gateway.Namespace,
		}] = gateway
	}

	httpRoutes := t.store.ListHTTPRoutes()
	for _, httpRoute := range httpRoutes {
		t.HTTPRouteToIR(httpRoute, upstreamCache, gatewayMap, virtualHostsPerGateway)
	}
	tcpRoutes := t.store.ListTCPRoutes()
	for _, tcpRoute := range tcpRoutes {
		t.TCPRouteToIR(tcpRoute, upstreamCache, gatewayMap, virtualHostsPerGateway)
	}
	tlsRoutes := t.store.ListTLSRoutes()
	for _, tlsRoute := range tlsRoutes {
		t.TLSRouteToIR(tlsRoute, upstreamCache, gatewayMap, virtualHostsPerGateway)
	}

	vHostSlice := []*ir.IRVirtualHost{}
	for _, vHostsForGateway := range virtualHostsPerGateway {
		for _, irVirtualHost := range vHostsForGateway {
			vHostSlice = append(vHostSlice, irVirtualHost)
		}
	}

	return vHostSlice
}

func (t *translator) findMatchingVHostsForXRoute(
	virtualHostsPerGateway map[types.NamespacedName]map[ir.IRListener]*ir.IRVirtualHost,
	gatewayMap map[types.NamespacedName]*gatewayv1.Gateway,
	routeName string,
	routeNamespace string,
	routeKind xRouteKind,
	routeParentRefs []gatewayv1.ParentReference,
	routeHostnames ...string,
) map[*ir.IRVirtualHost]bool {
	vHostsMatchingRoute := make(map[*ir.IRVirtualHost]bool)

	// First, go through the route's parentRefs to find matching gateways and figure out which hostnames within those matching
	// gateways this route matches. Along the way, build/update virtual hosts for all the hostnames this route matches
	for _, parentRef := range routeParentRefs {

		// Check matching Gateways for this route
		// The controller already filters the resources based on our gateway class, so no need to check that here
		refNamespace := routeNamespace
		if parentRef.Namespace != nil {
			refNamespace = string(*parentRef.Namespace)
		}

		gatewayKey := types.NamespacedName{
			Name:      string(parentRef.Name),
			Namespace: refNamespace,
		}
		gateway, exists := gatewayMap[gatewayKey]
		if !exists {
			t.log.Error(fmt.Errorf("%s parent ref not found", routeKind), fmt.Sprintf("the %q lists a Gateway parent ref that does not exist", routeKind),
				string(routeKind), fmt.Sprintf("%s.%s", routeName, routeNamespace),
				"parentRef", fmt.Sprintf("%s.%s", string(parentRef.Name), refNamespace),
			)
			continue
		}

		// Grab all the hostnames from the Gateway's addresses field. We don't currently allow IP addresses to be specified here, only hostnames
		gatewayAddressHostnames := []string{}
		for _, gatewayAddress := range gateway.Spec.Addresses {
			if gatewayAddress.Type == nil || *gatewayAddress.Type != gatewayv1.HostnameAddressType {
				t.log.Error(fmt.Errorf("invalid Gateway. non-hostname address in spec.addresses"), "this Gateway will be skipped. only hostname type addresses are supported at the moment. The default type is IPAddress, so the type must explicitly be set to \"Hostname\"",
					"gateway", fmt.Sprintf("%s.%s", gateway.Name, gateway.Namespace),
					"address value", gatewayAddress.Value,
				)
				continue
			}

			if net.ParseIP(gatewayAddress.Value) != nil {
				t.log.Error(fmt.Errorf("ip address used as address value in spec.addresses"), "this Gateway will be skipped. only hostname type addresses are supported at the moment.",
					"gateway", fmt.Sprintf("%s.%s", gateway.Name, gateway.Namespace),
					"address value", gatewayAddress.Value,
				)
				continue
			}

			gatewayAddressHostnames = appendStringUnique(gatewayAddressHostnames, gatewayAddress.Value)
		}

		// We currently require this annotation to be present for an Ingress to be translated into CloudEndpoints/AgentEndpoints, otherwise the default behaviour is to
		// translate it into HTTPSEdges (legacy). A future version will remove support for HTTPSEdges and translation into CloudEndpoints/AgentEndpoints will become the new
		// default behaviour.
		mappingStrategy, err := MappingStrategyAnnotationToIR(gateway)
		if err != nil {
			t.log.Error(err, fmt.Sprintf("failed to check %q annotation. defaulting to using endpoints", annotations.MappingStrategyAnnotation))
		}
		if mappingStrategy == ir.IRMappingStrategy_Edges {
			if routeKind != xRouteKind_HTTPRoute {
				t.log.Error(fmt.Errorf("%ss cannot be used on Gateways with the the %q annotation because Edges are not supported for %ss", routeKind, annotations.MappingStrategyAnnotation, routeKind), "the TCPRoute will be ignored for this Gateway",
					string(routeKind), fmt.Sprintf("%s.%s", routeName, routeNamespace),
					"gateway", fmt.Sprintf("%s.%s", gateway.Name, gateway.Namespace),
				)
			} else {
				t.log.Info(fmt.Sprintf("the Gateway and its routes will be provided by ngrok edges instead of endpoints because of the %q annotation",
					annotations.MappingStrategyAnnotation),
					string(routeKind), fmt.Sprintf("%s.%s", routeName, routeNamespace),
					"gateway", fmt.Sprintf("%s.%s", gateway.Name, gateway.Namespace),
				)
			}
			continue
		}

		useEndpointPooling, err := annotations.ExtractUseEndpointPooling(gateway)
		if err != nil {
			t.log.Error(err, fmt.Sprintf("failed to check %q annotation", annotations.MappingStrategyAnnotation))
		}
		if useEndpointPooling {
			t.log.Info(fmt.Sprintf("the following Gateway and its routes will create endpoint(s) with pooling enabled because of the %q annotation",
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
			annotationTrafficPolicy, tpObjRef, err = trafficPolicyFromModSetAnnotation(t.log, t.store, gateway, true)
			if err != nil {
				t.log.Error(err, "error getting ngrok traffic policy for gateway",
					"gateway", fmt.Sprintf("%s.%s", gateway.Name, gateway.Namespace))
				continue
			}
		}

		bindings, err := annotations.ExtractUseBindings(gateway)
		if err != nil {
			t.log.Error(err, "failed to check bindings annotation for gateway",
				"gateway", fmt.Sprintf("%s.%s", gateway.Name, gateway.Namespace),
			)
			continue
		}
		upstreamClientCertRefs := []ir.IRObjectRef{}
		if gateway.Spec.BackendTLS != nil && gateway.Spec.BackendTLS.ClientCertificateRef != nil {
			certRef := gateway.Spec.BackendTLS.ClientCertificateRef
			if certRef.Namespace == nil {
				certNs := gatewayv1.Namespace(gateway.Namespace)
				certRef.Namespace = &certNs
			}

			if !t.isRefToNamespaceAllowed(gateway.Namespace, "gateway.networking.k8s.io", "Gateway", string(certRef.Name), string(*certRef.Namespace), "", "Secret") {
				t.log.Error(fmt.Errorf("reference to Secret %q is not allowed without a valid ReferenceGrant", fmt.Sprintf("%s.%s", certRef.Name, refNamespace)),
					"Gateway backendTLS.clientCertificateRef is invalid without a ReferenceGrant",
				)
				continue
			}

			upstreamClientCertRefs = append(upstreamClientCertRefs, ir.IRObjectRef{
				Name:      string(certRef.Name),
				Namespace: string(*certRef.Namespace),
			})
		}

		matchingListeners := t.matchGatewayListenersToXRoute(gateway, gatewayAddressHostnames, routeNamespace, routeKind, parentRef.SectionName, parentRef.Port, routeHostnames...)
		for _, matchingListener := range matchingListeners {
			tlsTermCfg := matchingListener.TLS
			if tlsTermCfg != nil {

				switch matchingListener.Protocol {
				case gatewayv1.HTTPSProtocolType:
					if tlsTermCfg.Mode != nil && *tlsTermCfg.Mode == gatewayv1.TLSModePassthrough {
						t.log.Error(fmt.Errorf("TLS passthrough mode is not possible for HTTPS endpoints, you can use a TLS protocol listener for this functionality"), "skipping gateway listener for HTTPRoutes because the tls mode is set to passthrough",
							"gateway", fmt.Sprintf("%s.%s", gateway.Name, gateway.Namespace),
						)
						continue
					}
				// HTTP and TCP endpoints don't do TLS, use HTTPS/TLS endpoints instead
				case gatewayv1.HTTPProtocolType, gatewayv1.TCPProtocolType:
					t.log.Error(fmt.Errorf("TLS termination is not supported for HTTP and TCP listeners. You can use an HTTPS/TLS listener for this functionality"), "skipping gateway listener",
						"gateway", fmt.Sprintf("%s.%s", gateway.Name, gateway.Namespace),
					)
					continue
				}
			}

			var irTLSTermination *ir.IRTLSTermination
			// No TLS termination for TCP and HTTP protocol listeners
			if matchingListener.Protocol == gatewayv1.TLSProtocolType || matchingListener.Protocol == gatewayv1.HTTPSProtocolType {
				irTLSTermination, err = t.gatewayTLSTermConfigToIR(tlsTermCfg, gateway)
				if err != nil {
					t.log.Error(err, "skipping gateway listener with invalid TLS configuration",
						"gateway", fmt.Sprintf("%s.%s", gateway.Name, gateway.Namespace),
					)
					continue
				}
			}

			// Check if this Gateway already has any virtual hosts
			vHostsForCurrentGateway, exists := virtualHostsPerGateway[gatewayKey]
			if !exists {
				// Initialize the underlying map if needed
				vHostsForCurrentGateway = make(map[ir.IRListener]*ir.IRVirtualHost)
				virtualHostsPerGateway[gatewayKey] = vHostsForCurrentGateway
			}

			// If the Gateway specifies spec.addresses, use those for the hostnames for the endpoints, otherwise, use the listener's hostname

			var virtualHostHostnames []string
			if len(gatewayAddressHostnames) == 0 {
				virtualHostHostnames = []string{string(*matchingListener.Hostname)}
			} else {
				virtualHostHostnames = gatewayAddressHostnames
			}

			for _, virtualHostHostname := range virtualHostHostnames {
				// Check if this Gateway already has an irVHost for this specific hostname, otherwise make one
				irListener := ir.IRListener{
					Hostname: ir.IRHostname(virtualHostHostname),
					Port:     int32(matchingListener.Port),
				}

				switch matchingListener.Protocol {
				case gatewayv1.HTTPProtocolType:
					irListener.Protocol = ir.IRProtocol_HTTP
				case gatewayv1.HTTPSProtocolType:
					irListener.Protocol = ir.IRProtocol_HTTPS
				case gatewayv1.TCPProtocolType:
					irListener.Protocol = ir.IRProtocol_TCP
				case gatewayv1.TLSProtocolType:
					irListener.Protocol = ir.IRProtocol_TLS
				default:
					// Ignore other listener protocols
					continue
				}

				irVHost, exists := vHostsForCurrentGateway[irListener]
				if !exists {
					// Add a name prefix with the gateway name so that we can support endpoint pooling across multiple gateways
					namePrefix := fmt.Sprintf("%s.%s", gateway.Name, gateway.Namespace)

					if irListener.Protocol == ir.IRProtocol_TCP || irListener.Protocol == ir.IRProtocol_TLS {
						namePrefix += fmt.Sprintf(".%d", int32(matchingListener.Port))
					}

					irVHost = &ir.IRVirtualHost{
						NamePrefix:             &namePrefix,
						Namespace:              gateway.Namespace,
						Listener:               irListener,
						TLSTermination:         irTLSTermination,
						LabelsToAdd:            t.managedResourceLabels,
						AnnotationsToAdd:       make(map[string]string),
						EndpointPoolingEnabled: useEndpointPooling,
						TrafficPolicy:          annotationTrafficPolicy,
						TrafficPolicyObjRef:    tpObjRef,
						Metadata:               t.defaultGatewayMetadata,
						Bindings:               bindings,
						ClientCertRefs:         upstreamClientCertRefs,
						MappingStrategy:        mappingStrategy,
					}
				}
				irVHost.AddOwningResource(ir.OwningResource{
					Kind:      "Gateway",
					Name:      gateway.Name,
					Namespace: gateway.Namespace,
				})
				irVHost.AddOwningResource(ir.OwningResource{
					Kind:      string(routeKind),
					Name:      routeName,
					Namespace: routeNamespace,
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
	}

	return vHostsMatchingRoute
}

// #region HTTPRoute to IR

// HTTPRouteToIR translates a single HTTPRoute into IR by finding which Gateways it matches and adding the rules from the HTTPRoute
// as routes on the VirtualHost(s)
func (t *translator) HTTPRouteToIR(
	httpRoute *gatewayv1.HTTPRoute,
	upstreamCache map[ir.IRServiceKey]*ir.IRUpstream,
	gatewayMap map[types.NamespacedName]*gatewayv1.Gateway,
	virtualHostsPerGateway map[types.NamespacedName]map[ir.IRListener]*ir.IRVirtualHost,
) {
	var hostnameStrings []string
	for _, h := range httpRoute.Spec.Hostnames {
		hostnameStrings = append(hostnameStrings, string(h))
	}
	vHostsMatchingRoute := t.findMatchingVHostsForXRoute(virtualHostsPerGateway, gatewayMap, httpRoute.Name, httpRoute.Namespace, xRouteKind_HTTPRoute, httpRoute.Spec.ParentRefs, hostnameStrings...)

	// Now that we have a set of the virtual hosts that are applicable to this HTTPRoute, go through and build new routes
	// for all the HTTPRoute rules and add them to the matching virtual hosts

	// Add all the routes we just processed to all matching virtual hosts
	for irVHost := range vHostsMatchingRoute {
		// Note: it would be more efficient to build the routes for the HTTPRoute once, then apply them to all matching virtualHosts, but
		// each Gateway can specify upstream client certificates, so the routes we build are dependent on the current Gateway
		routesToAdd := t.httpRouteRulesToIR(irVHost, httpRoute, upstreamCache)
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

// #region TCPRoute to IR

// TCPRouteToIR translates a single TCPRoute into IR by finding which Gateways it matches and adding the backends for the TCPRoute
// as routes on the VirtualHost(s)
func (t *translator) TCPRouteToIR(
	tcpRoute *gatewayv1alpha2.TCPRoute,
	upstreamCache map[ir.IRServiceKey]*ir.IRUpstream,
	gatewayMap map[types.NamespacedName]*gatewayv1.Gateway,
	virtualHostsPerGateway map[types.NamespacedName]map[ir.IRListener]*ir.IRVirtualHost,
) {
	vHostsMatchingRoute := t.findMatchingVHostsForXRoute(virtualHostsPerGateway, gatewayMap, tcpRoute.Name, tcpRoute.Namespace, xRouteKind_TCPRoute, tcpRoute.Spec.ParentRefs)

	// Now that we have a set of the virtual hosts that are applicable to this TCPRoute, go through and build new routes
	// for all the TCPRoute rules and add them to the matching virtual hosts

	// Add all the routes we just processed to all matching virtual hosts
	for irVHost := range vHostsMatchingRoute {
		// Note: it would be more efficient to build the routes for the TCPRoute once, then apply them to all matching virtualHosts, but
		// each Gateway can specify upstream client certificates, so the routes we build are dependent on the current Gateway
		if routeToAdd := t.tcpRouteRulesToIR(irVHost, tcpRoute, upstreamCache); routeToAdd != nil {
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

// #region TLSRoute to IR

// TLSRouteToIR translates a single TLSRoute into IR by finding which Gateways it matches and adding the backends for the TLSRoute
// as routes on the VirtualHost(s)
func (t *translator) TLSRouteToIR(
	tlsRoute *gatewayv1alpha2.TLSRoute,
	upstreamCache map[ir.IRServiceKey]*ir.IRUpstream,
	gatewayMap map[types.NamespacedName]*gatewayv1.Gateway,
	virtualHostsPerGateway map[types.NamespacedName]map[ir.IRListener]*ir.IRVirtualHost,
) {
	var hostnameStrings []string
	for _, h := range tlsRoute.Spec.Hostnames {
		hostnameStrings = append(hostnameStrings, string(h))
	}
	vHostsMatchingRoute := t.findMatchingVHostsForXRoute(virtualHostsPerGateway, gatewayMap, tlsRoute.Name, tlsRoute.Namespace, xRouteKind_TLSRoute, tlsRoute.Spec.ParentRefs, hostnameStrings...)

	// Now that we have a set of the virtual hosts that are applicable to this TLSRoute, go through and build new routes
	// for all the TLSRoute rules and add them to the matching virtual hosts

	// Add all the routes we just processed to all matching virtual hosts
	for irVHost := range vHostsMatchingRoute {
		// Note: it would be more efficient to build the routes for the TLSRoute once, then apply them to all matching virtualHosts, but
		// each Gateway can specify upstream client certificates, so the routes we build are dependent on the current Gateway
		// TODO: Alice, check nil
		if routeToAdd := t.tlsRouteRulesToIR(irVHost, tlsRoute, upstreamCache); routeToAdd != nil {
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

func (t *translator) tcpRouteRulesToIR(irVHost *ir.IRVirtualHost, tcpRoute *gatewayv1alpha2.TCPRoute, upstreamCache map[ir.IRServiceKey]*ir.IRUpstream) *ir.IRRoute {
	irRoute := &ir.IRRoute{
		TrafficPolicies: []*trafficpolicy.TrafficPolicy{},
	}
	for _, rule := range tcpRoute.Spec.Rules {
		// For each rule create a route
		for _, backendRef := range rule.BackendRefs {
			irDestination, err := t.tcpBackendToIR(tcpRoute.Name, tcpRoute.Namespace, xRouteKind_TCPRoute, backendRef, upstreamCache, irVHost.ClientCertRefs)
			if err != nil {
				t.log.Error(err, "unable to translate TCPRoute backend ref",
					"TCPRoute", fmt.Sprintf("%s.%s", tcpRoute.Name, tcpRoute.Namespace),
				)
				continue
			}
			irRoute.Destinations = append(irRoute.Destinations, irDestination)
		}
	}
	if len(irRoute.Destinations) > 0 {
		return irRoute
	}
	return nil
}

func (t *translator) tlsRouteRulesToIR(irVHost *ir.IRVirtualHost, tlsRoute *gatewayv1alpha2.TLSRoute, upstreamCache map[ir.IRServiceKey]*ir.IRUpstream) *ir.IRRoute {
	irRoute := &ir.IRRoute{
		TrafficPolicies: []*trafficpolicy.TrafficPolicy{},
	}
	for _, rule := range tlsRoute.Spec.Rules {
		for _, backendRef := range rule.BackendRefs {
			irDestination, err := t.tcpBackendToIR(tlsRoute.Name, tlsRoute.Namespace, xRouteKind_TLSRoute, backendRef, upstreamCache, irVHost.ClientCertRefs)
			if err != nil {
				t.log.Error(err, "unable to translate TLSRoute backend ref",
					"TLSRoute", fmt.Sprintf("%s.%s", tlsRoute.Name, tlsRoute.Namespace),
				)
				continue
			}
			irRoute.Destinations = append(irRoute.Destinations, irDestination)
		}
	}
	if len(irRoute.Destinations) > 0 {
		return irRoute
	}
	return nil
}

func (t *translator) httpRouteRulesToIR(irVHost *ir.IRVirtualHost, httpRoute *gatewayv1.HTTPRoute, upstreamCache map[ir.IRServiceKey]*ir.IRUpstream) []*ir.IRRoute {
	routesToAdd := []*ir.IRRoute{}
	for _, rule := range httpRoute.Spec.Rules {
		// For each rule.Match create a route
		for _, match := range rule.Matches {
			irRoute := &ir.IRRoute{
				HTTPMatchCriteria: GatewayAPIHTTPMatchToIR(match),
				TrafficPolicies:   []*trafficpolicy.TrafficPolicy{},
			}

			for _, filter := range rule.Filters {
				// For each GatewayAPI filter for the route, we will inject additional config into the route's traffic policy
				filterTrafficPolicy, err := t.gatewayAPIFilterToTrafficPolicy(filter, httpRoute.Namespace, t.store, irRoute.HTTPMatchCriteria)
				if err != nil {
					t.log.Error(err, "skipping filter with error")
					continue
				}
				irRoute.TrafficPolicies = append(irRoute.TrafficPolicies, filterTrafficPolicy)
			}

			for _, backendRef := range rule.BackendRefs {
				irDestination, err := t.httpRouteBackendToIR(httpRoute, backendRef, upstreamCache, irRoute.HTTPMatchCriteria, irVHost.ClientCertRefs)
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
	return routesToAdd
}

// #region Find Gateway listeners for HTTPRoute

// xRouteKind identifies a type of Gateway API Route that we support translation for
type xRouteKind string

const (
	xRouteKind_HTTPRoute xRouteKind = "HTTPRoute"
	xRouteKind_TCPRoute  xRouteKind = "TCPRoute"
	xRouteKind_TLSRoute  xRouteKind = "TLSRoute"

	// GRPCRoute & UDPRoute not currently supported
)

// matchGatewayListenersToXRoute takes a Gateway and properties of an HTTPRoute/TLSRoute/TCPRoute and figures out which (if any) listeners from the Gateway the route matches
func (t *translator) matchGatewayListenersToXRoute(
	gateway *gatewayv1.Gateway,
	gatewayAddressHostnames []string,
	routeNamespace string,
	routeKind xRouteKind,
	listenerName *gatewayv1.SectionName,
	listenerPort *gatewayv1.PortNumber,
	routeHostnames ...string,
) []gatewayv1.Listener {
	matchingListeners := []gatewayv1.Listener{}

	for _, listener := range gateway.Spec.Listeners {

		// Ignore the current listener if a specific name/port is expected and it doesn't match the current listener
		if listenerName != nil && listener.Name != *listenerName {
			continue
		}

		if listenerPort != nil && listener.Port != *listenerPort {
			continue
		}

		// When allowedRoutes is not specified, only routes in the same namespace as the gateway are allowed
		if listener.AllowedRoutes == nil && (gateway.Namespace != routeNamespace) {
			continue
		}

		if listener.AllowedRoutes != nil {
			allowedKind := true // Default to allowing routes in the same namespace when not specified
			if len(listener.AllowedRoutes.Kinds) > 0 {
				allowedKind = false
				for _, kind := range listener.AllowedRoutes.Kinds {
					if strings.EqualFold(string(kind.Kind), string(routeKind)) {
						allowedKind = true
						break
					}
				}
			}
			if !allowedKind {
				continue
			}

			// Validate namespaces
			allowedNamespace := gateway.Namespace == routeNamespace // By default, only allow those in the same namespace
			if listener.AllowedRoutes.Namespaces != nil {
				nsPolicy := listener.AllowedRoutes.Namespaces.From
				if nsPolicy != nil {
					switch *nsPolicy {
					case gatewayv1.NamespacesFromSame:
						// Default behaviour, do nothing
					case gatewayv1.NamespacesFromAll:
						allowedNamespace = true
					case gatewayv1.NamespacesFromSelector:
						if listener.AllowedRoutes.Namespaces.Selector != nil {
							// Check if the namespace matches the selector
							selector, err := metav1.LabelSelectorAsSelector(listener.AllowedRoutes.Namespaces.Selector)
							if err != nil {
								t.log.Error(err, "unable to parse AllowedRoutes.Namespaces.Selector")
								continue
							}
							// Get the namespace for the current route
							namespace, err := t.store.GetNamespaceV1(routeNamespace)
							if err != nil {
								t.log.Error(err, fmt.Sprintf("unable to validate whether current %q labels match Gateway AllowedRoutes.Namespaces.Selector", routeKind))
								continue
							}
							if !selector.Matches(labels.Set(namespace.Labels)) {
								continue
							}
							allowedNamespace = true
						}
					}
				}
			}
			if !allowedNamespace {
				continue
			}
		}

		// Handle listener hostnames

		// If the gateway does not specify a spec.Addresses, then the hostname of the listener is expected to serve as the address for the endpoint and can't be something with no top level domain (like "*").
		// Due to the way that endpoints work, we must have some kind of a domain to use to generate the endpoints.
		// For TCPRoute/TLSRoute, we allow you to specify them via the spec.Addresses since TCPRoute has no way to configure a hostname otherwise and a Gateway's listener is forbidden from specifying a hostname when the protocol is TCP/TLS
		//
		// To make this make sense for HTTP/HTTPS endpoints, when no Gateway.spec.addresses are provided, the listener must provide a non-nil, non-empty string, hostname to be used for the endpoint. The listeners hostnames become the domains for the endpoints
		// If Gateway.spec.addresses are provided, then the listener's hostname may be left blank to match all of them, but when specified, it must match all of the hostnames in Gateway.spec.addresses. The gateway's spec.addresses become the domains for the endpoints
		//
		// Note: since we are introducing support for Gateway.spec.addresses, I think it would make sense for a future change to make it so that the domains for the endpoints must be specified in the Gateway.spec.addresses field, and the listener.hostname becomes something that is used
		// for host header and SNI matching on requests.

		listenerHostname := "*"
		if len(gatewayAddressHostnames) == 0 {
			if listener.Hostname == nil {
				t.log.Error(fmt.Errorf("gateway has a listener with a nil hostname"), "Gateway listeners with nil hostnames are not supported when Gateway.spec.addresses is empty, gateway listeners must have a valid non-empty hostname other than \"*\". Invalid listeners will be skipped.",
					"gateway", fmt.Sprintf("%s.%s", gateway.Name, gateway.Namespace),
				)
				continue
			}

			if string(*listener.Hostname) == "*" {
				t.log.Error(fmt.Errorf("gateway has a listener with hostname \"*\""), "Gateway listeners with hostname \"*\" are not supported when Gateway.spec.addresses is empty, gateway listeners must have a valid non-empty hostname other than \"*\". Invalid listeners will be skipped.",
					"gateway", fmt.Sprintf("%s.%s", gateway.Name, gateway.Namespace),
				)
				continue
			}

			if listener.Hostname != nil && string(*listener.Hostname) == "" {
				t.log.Error(fmt.Errorf("gateway has a listener with an empty hostname"), "Gateway listeners with empty hostnames are not supported when Gateway.spec.addresses is empty, gateway listeners must have a valid non-empty hostname other than \"*\". Invalid listeners will be skipped.",
					"gateway", fmt.Sprintf("%s.%s", gateway.Name, gateway.Namespace),
				)
				continue
			}
		}

		if listener.Hostname != nil && string(*listener.Hostname) != "" {
			listenerHostname = string(*listener.Hostname)
		}

		// From the Gateway API spec: "The implementation MUST bind all Listeners to every GatewayAddress", so all listener hostnames must match all address hostnames
		for _, gatewgatewayAddressHostname := range gatewayAddressHostnames {
			match, err := doHostGlobsMatch(listenerHostname, gatewgatewayAddressHostname)
			if err != nil {
				t.log.Error(err, "unable to compile hostname glob match for Gateway.spec.address and Gateway listener, this Gateway will be skipped",
					"gateway", fmt.Sprintf("%s.%s", gateway.Name, gateway.Namespace),
					"gateway address", gatewgatewayAddressHostname,
					"listener hostname", listenerHostname,
				)
				return []gatewayv1.Listener{}
			}
			if !match {
				t.log.Error(fmt.Errorf("listener hostname does not match one of the Gateway's spec.addresses"), "all listener hostnames must match all Gateway addresses. this Gateway will be skipped",
					"gateway", fmt.Sprintf("%s.%s", gateway.Name, gateway.Namespace),
					"gateway address", gatewgatewayAddressHostname,
					"listener hostname", listenerHostname,
				)
				return []gatewayv1.Listener{}
			}
		}

		// When the route hostnames are empty, it matches all listeners for all parent gateways
		if len(routeHostnames) == 0 {
			matchingListeners = append(matchingListeners, listener)
			continue
		}

		// Check matches for valid hostnames
		for _, routeHostname := range routeHostnames {
			if routeHostname == "*" {
				matchingListeners = append(matchingListeners, listener)
				break
			}
			match, err := doHostGlobsMatch(listenerHostname, routeHostname)
			if err != nil {
				t.log.Error(err, "unable to compile hostname glob for Gateway listener hostname, this listener will be skipped",
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
func GatewayAPIHTTPMatchToIR(match gatewayv1.HTTPRouteMatch) *ir.IRHTTPMatch {
	// GatewayAPI specifies that when nil, the default path match behaviour should be a prefix match on "/"
	path := "/"
	pathType := ir.IRPathType_Prefix

	if match.Path != nil {
		if match.Path.Value != nil {
			path = *match.Path.Value
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

	ret := &ir.IRHTTPMatch{
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
func (t *translator) gatewayAPIFilterToTrafficPolicy(filter gatewayv1.HTTPRouteFilter, namespace string, store store.Storer, matchCriteria *ir.IRHTTPMatch) (*trafficpolicy.TrafficPolicy, error) {
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
func gwapiRedirectFilterToTrafficPolicy(filter gatewayv1.HTTPRouteFilter, matchCriteria *ir.IRHTTPMatch) (*trafficpolicy.TrafficPolicy, error) {
	redirect := filter.RequestRedirect
	if redirect == nil {
		return nil, fmt.Errorf("filter type specified as RequestRedirect but the section config was nil")
	}

	prefix := ""
	if matchCriteria != nil && matchCriteria.Path != nil {
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
func gwapiURLRewriteFilterToTrafficPolicy(filter gatewayv1.HTTPRouteFilter, matchCriteria *ir.IRHTTPMatch) (*trafficpolicy.TrafficPolicy, error) {
	rewrite := filter.URLRewrite
	if rewrite == nil {
		return nil, fmt.Errorf("filter type specified as URLRewrite but the section config was nil")
	}

	if rewrite.Hostname == nil && rewrite.Path == nil {
		return nil, fmt.Errorf("URLRewrite filter must specify at least one of hostname or path")
	}

	prefix := ""
	if matchCriteria != nil && matchCriteria.Path != nil {
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
func (t *translator) httpRouteBackendToIR(httpRoute *gatewayv1.HTTPRoute, backendRef gatewayv1.HTTPBackendRef, upstreamCache map[ir.IRServiceKey]*ir.IRUpstream, matchCriteria *ir.IRHTTPMatch, upstreamClientCertRefs []ir.IRObjectRef) (*ir.IRDestination, error) {
	destination := &ir.IRDestination{
		TrafficPolicies: []*trafficpolicy.TrafficPolicy{},
	}

	if backendRef.Weight != nil {
		weight := int(*backendRef.Weight)
		destination.Weight = &weight
	}

	for _, filter := range backendRef.Filters {
		filterPolicy, err := t.gatewayAPIFilterToTrafficPolicy(filter, httpRoute.Namespace, t.store, matchCriteria)
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

	serviceNamespace := httpRoute.Namespace
	if backendRef.Namespace != nil && *backendRef.Namespace != "" {
		serviceNamespace = string(*backendRef.Namespace)
		if !t.isRefToNamespaceAllowed(httpRoute.Namespace, "gateway.networking.k8s.io", "HTTPRoute", serviceName, serviceNamespace, "", "Service") {
			return nil, fmt.Errorf("reference to Service %q is not allowed without a valid ReferenceGrant",
				fmt.Sprintf("%s.%s", serviceName, serviceNamespace),
			)
		}
	}

	if backendRef.Port == nil {
		return nil, fmt.Errorf("backendRef supplied to HTTPRoute is missing the required port. name: %q, namespace: %q",
			serviceName,
			serviceNamespace,
		)
	}

	service, err := t.store.GetServiceV1(serviceName, serviceNamespace)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve Service for backendRef name: %q in namespace %q: %w",
			serviceName,
			serviceNamespace,
			err,
		)
	}

	servicePort, err := findServicesPort(t.log, service, netv1.ServiceBackendPort{Number: int32(*backendRef.Port)})
	if err != nil || servicePort == nil {
		return nil, fmt.Errorf("failed to resolve backendRef Service's port. name: %q, namespace: %q: %w",
			serviceName,
			serviceNamespace,
			err,
		)
	}

	portProto, err := getProtoForServicePort(t.log, service, servicePort.Name, ir.IRProtocol_HTTP)
	if err != nil {
		// When this function errors we still get a valid default, so no need to return
		t.log.Error(err, "error getting protocol for HTTPRoute backendRef service port",
			"httpRoute", fmt.Sprintf("%s.%s", httpRoute.Name, httpRoute.Namespace),
			"backendRef", backendRef,
		)
	}

	irScheme, err := protocolStringToIRScheme(portProto)
	if err != nil {
		t.log.Error(err, "error getting scheme from port protocol for HTTPRoute backendRef service port",
			"httpRoute", fmt.Sprintf("%s.%s", httpRoute.Name, httpRoute.Namespace),
			"backendRef", backendRef,
			"service", fmt.Sprintf("%s.%s", service.Name, service.Namespace),
			"port name", servicePort.Name,
			"port number", servicePort.Port,
		)
	}

	irService := ir.IRService{
		UID:       string(service.UID),
		Name:      serviceName,
		Namespace: serviceNamespace,
		Port:      servicePort.Port,
		Scheme:    irScheme,
	}

	// The following is the wording from the Gateway API about supplied client certificate refs
	//  BackendTLS configures TLS settings for when this Gateway is connecting to
	//  backends with TLS
	//
	// _when_ implies to me that it is valid to have upstreams for a gateway with client certs that are not HTTPS and so we should not set these
	// in those cases
	if irScheme == ir.IRScheme_HTTPS {
		irService.ClientCertRefs = upstreamClientCertRefs
	}

	upstream, exists := upstreamCache[irService.Key()]
	if !exists {
		upstream = &ir.IRUpstream{
			Service: irService,
		}
		upstreamCache[irService.Key()] = upstream
	}
	destination.Upstream = upstream

	return destination, nil
}

// #region TCP BackendRef IR

// gwapiRequestHeaderFilterToTrafficPolicy translates a GatewayAPI backendRef into IR
func (t *translator) tcpBackendToIR(routeName string, routeNamespace string, routeKind xRouteKind, backendRef gatewayv1.BackendRef, upstreamCache map[ir.IRServiceKey]*ir.IRUpstream, upstreamClientCertRefs []ir.IRObjectRef) (*ir.IRDestination, error) {
	destination := &ir.IRDestination{
		TrafficPolicies: []*trafficpolicy.TrafficPolicy{},
	}

	if backendRef.Weight != nil {
		weight := int(*backendRef.Weight)
		destination.Weight = &weight
	}

	if backendRef.Kind != nil && !strings.EqualFold(string(*backendRef.Kind), "Service") {
		return nil, fmt.Errorf("invalid backendRef kind supplied to %s. only Service backends are currently supported", routeKind)
	}

	serviceName := string(backendRef.Name)
	if serviceName == "" {
		return destination, nil
	}

	serviceNamespace := routeNamespace
	if backendRef.Namespace != nil && *backendRef.Namespace != "" {
		serviceNamespace = string(*backendRef.Namespace)
		if !t.isRefToNamespaceAllowed(routeNamespace, "gateway.networking.k8s.io", string(routeKind), serviceName, serviceNamespace, "", "Service") {
			return nil, fmt.Errorf("reference to Service %q is not allowed without a valid ReferenceGrant",
				fmt.Sprintf("%s.%s", serviceName, serviceNamespace),
			)
		}
	}

	if backendRef.Port == nil {
		return nil, fmt.Errorf("backendRef supplied to %s is missing the required port. name: %q, namespace: %q",
			routeKind,
			serviceName,
			serviceNamespace,
		)
	}

	service, err := t.store.GetServiceV1(serviceName, serviceNamespace)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve Service for backendRef name: %q in namespace %q: %w",
			serviceName,
			serviceNamespace,
			err,
		)
	}

	servicePort, err := findServicesPort(t.log, service, netv1.ServiceBackendPort{Number: int32(*backendRef.Port)})
	if err != nil || servicePort == nil {
		return nil, fmt.Errorf("failed to resolve backendRef Service's port. name: %q, namespace: %q: %w",
			serviceName,
			serviceNamespace,
			err,
		)
	}

	defaultProto := ir.IRProtocol_TCP
	if routeKind == xRouteKind_TLSRoute {
		defaultProto = ir.IRProtocol_TLS
	}

	portProto, err := getProtoForServicePort(t.log, service, servicePort.Name, defaultProto)
	if err != nil {
		// When this function errors we still get a valid default, so no need to return
		t.log.Error(err, fmt.Sprintf("error getting protocol for %s backendRef service port", routeKind),
			string(routeKind), fmt.Sprintf("%s.%s", routeName, routeNamespace),
			"backendRef", backendRef,
		)
	}

	irScheme, err := protocolStringToIRScheme(portProto)
	if err != nil {
		t.log.Error(err, fmt.Sprintf("error getting scheme from port protocol for %s backendRef service port", routeKind),
			string(routeKind), fmt.Sprintf("%s.%s", routeName, routeNamespace),
			"backendRef", backendRef,
			"service", fmt.Sprintf("%s.%s", service.Name, service.Namespace),
			"port name", servicePort.Name,
			"port number", servicePort.Port,
		)
	}

	irService := ir.IRService{
		UID:       string(service.UID),
		Name:      serviceName,
		Namespace: serviceNamespace,
		Port:      servicePort.Port,
		Scheme:    irScheme,
	}

	// The following is the wording from the Gateway API about supplied client certificate refs
	//  BackendTLS configures TLS settings for when this Gateway is connecting to
	//  backends with TLS
	//
	// _when_ implies to me that it is valid to have upstreams for a gateway with client certs that are not HTTPS and so we should not set these
	// in those cases
	if irScheme == ir.IRScheme_TLS {
		irService.ClientCertRefs = upstreamClientCertRefs
	}

	upstream, exists := upstreamCache[irService.Key()]
	if !exists {
		upstream = &ir.IRUpstream{
			Service: irService,
		}
		upstreamCache[irService.Key()] = upstream
	}
	destination.Upstream = upstream

	return destination, nil
}

// #region GatewayTLS IR

// gwapiRequestHeaderFilterToTrafficPolicy translates a GatewayAPI tls configuration into IR
func (t *translator) gatewayTLSTermConfigToIR(listenerTLS *gatewayv1.GatewayTLSConfig, gateway *gatewayv1.Gateway) (*ir.IRTLSTermination, error) {
	if listenerTLS == nil {
		return nil, nil
	}

	tlsTermCfg := &ir.IRTLSTermination{
		MutualTLSCertificateAuthorities: []string{},
		ExtendedOptions:                 map[string]string{},
	}

	if len(listenerTLS.CertificateRefs) > 0 {
		if len(listenerTLS.CertificateRefs) > 1 {
			t.log.Error(fmt.Errorf("multiple Gateway TLS certificateRefs provided"), "Only the first will be used, multiple are not currently supported")
		}

		certRef := listenerTLS.CertificateRefs[0]
		if certRef.Kind != nil && !strings.EqualFold(string(*certRef.Kind), "Secret") {
			return nil, fmt.Errorf("unsupported kind %q for Gateway TLS config. only core api group secret references are supported", string(*certRef.Kind))
		}

		refNamespace := gateway.Namespace
		if certRef.Namespace != nil {
			refNamespace = string(*certRef.Namespace)
			if !t.isRefToNamespaceAllowed(gateway.Namespace, "gateway.networking.k8s.io", "Gateway", string(certRef.Name), refNamespace, "", "Secret") {
				return nil, fmt.Errorf("reference to Secret %q is not allowed without a valid ReferenceGrant",
					fmt.Sprintf("%s.%s", certRef.Name, refNamespace),
				)
			}
		}

		secret, err := t.store.GetSecretV1(string(certRef.Name), refNamespace)
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
	if listenerTLS.FrontendValidation != nil {
		for _, certRef := range listenerTLS.FrontendValidation.CACertificateRefs {
			refNamespace := gateway.Namespace
			if certRef.Namespace != nil {
				refNamespace = string(*certRef.Namespace)
				if !t.isRefToNamespaceAllowed(gateway.Namespace, "gateway.networking.k8s.io", "Gateway", string(certRef.Name), refNamespace, "", "ConfigMap") {
					return nil, fmt.Errorf("reference to ConfigMap %q is not allowed without a valid ReferenceGrant",
						fmt.Sprintf("%s.%s", certRef.Name, refNamespace),
					)
				}
			}

			if !strings.EqualFold(string(certRef.Kind), "ConfigMap") {
				return nil, fmt.Errorf("unsupported kind %q for Gateway frontend TLS config reference %s. only core api group ConfigMap references are supported",
					certRef.Kind,
					fmt.Sprintf("%s.%s", string(certRef.Name), refNamespace),
				)
			}

			configMap, err := t.store.GetConfigMapV1(string(certRef.Name), refNamespace)
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

	for key, val := range listenerTLS.Options {
		if strings.HasPrefix(string(key), TLSOptionKeyPrefix) {
			for _, reservedKey := range TLSOptionKeyReservedKeys {
				if string(key) == reservedKey {
					return nil, fmt.Errorf("invalid option supplied to listener tls options. %q is a reserved field and may not be provided here", reservedKey)
				}
			}
			keySuffix := strings.TrimPrefix(string(key), TLSOptionKeyPrefix)
			tlsTermCfg.ExtendedOptions[keySuffix] = string(val)
		}
	}

	return tlsTermCfg, nil
}

// isRefToNamespaceAllowed checks if a reference to a target namespace is allowed or not. This is for backendRefs and externalRef filters,
// the Gateway.Listeners.AllowedRoutes has its own logic
func (t *translator) isRefToNamespaceAllowed(fromNamespace, fromGroup, fromKind, toName, toNamespace, toGroup, toKind string) bool {
	if fromNamespace == toNamespace || t.disableGatewayReferenceGrants {
		return true
	}

	grants := t.store.ListReferenceGrants()
	if len(grants) == 0 {
		return false
	}
	for _, grant := range grants {
		if grant.Namespace != toNamespace {
			continue
		}

		allowedTo := false
		for _, grantTo := range grant.Spec.To {
			if !strings.EqualFold(string(grantTo.Group), toGroup) || !strings.EqualFold(string(grantTo.Kind), toKind) {
				continue
			}

			if grantTo.Name != nil && !strings.EqualFold(string(*grantTo.Name), toName) {
				continue
			}
			allowedTo = true
			break
		}
		if !allowedTo {
			continue
		}

		for _, grantFrom := range grant.Spec.From {
			if !strings.EqualFold(string(grantFrom.Group), fromGroup) ||
				!strings.EqualFold(string(grantFrom.Kind), fromKind) ||
				!strings.EqualFold(string(grantFrom.Namespace), fromNamespace) {
				continue
			}
			if allowedTo {
				return true
			}
		}
	}
	return false
}
