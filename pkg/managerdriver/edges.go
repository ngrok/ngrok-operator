package managerdriver

import (
	"context"
	"fmt"
	"reflect"
	"strings"

	ingressv1alpha1 "github.com/ngrok/ngrok-operator/api/ingress/v1alpha1"
	netv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"
)

func (d *Driver) SyncEdges(ctx context.Context, c client.Client) error {
	if !d.syncAllowConcurrent {
		if proceed, wait := d.syncStart(true); proceed {
			defer d.syncDone()
		} else {
			return wait(ctx)
		}
	}

	d.log.Info("syncing edges state!!")
	domains := d.calculateDomainSet()
	desiredEdges := d.calculateHTTPSEdges(domains)
	currEdges := &ingressv1alpha1.HTTPSEdgeList{}
	if err := c.List(ctx, currEdges, client.MatchingLabels{
		labelControllerNamespace: d.managerName.Namespace,
		labelControllerName:      d.managerName.Name,
	}); err != nil {
		d.log.Error(err, "error listing edges")
		return err
	}

	if err := d.applyHTTPSEdges(ctx, c, desiredEdges, currEdges.Items); err != nil {
		return err
	}

	return nil
}

func (d *Driver) applyHTTPSEdges(ctx context.Context, c client.Client, desiredEdges map[string]ingressv1alpha1.HTTPSEdge, currentEdges []ingressv1alpha1.HTTPSEdge) error {
	// update or delete edge we don't need anymore
	for _, currEdge := range currentEdges {
		hostports := currEdge.Spec.Hostports

		// If one of the controller-owned edges has more than one hostport, log an error and skip it
		// because we can't determine what to do with it.
		if len(hostports) != 1 {
			d.log.Error(nil, "Existing owned edge has more than 1 hostport", "edge", currEdge, "hostports", hostports)
			continue
		}

		// ngrok only supports https on port 443 and all domains are on port 443
		// so we can safely trim the port from the hostport to get the domain
		domain := strings.TrimSuffix(hostports[0], ":443")

		if desiredEdge, ok := desiredEdges[domain]; ok {
			needsUpdate := false

			if !reflect.DeepEqual(desiredEdge.Spec, currEdge.Spec) {
				currEdge.Spec = desiredEdge.Spec
				needsUpdate = true
			}

			if needsUpdate {
				if err := c.Update(ctx, &currEdge); err != nil {
					d.log.Error(err, "error updating edge", "desiredEdge", desiredEdge, "currEdge", currEdge)
					return err
				}
			}

			// matched and updated the edge, no longer desired
			delete(desiredEdges, domain)
		} else {
			if err := c.Delete(ctx, &currEdge); client.IgnoreNotFound(err) != nil {
				d.log.Error(err, "error deleting edge", "edge", currEdge)
				return err
			}
		}
	}

	// the set of desired edges now only contains new edges, create them
	for _, edge := range desiredEdges {
		if err := c.Create(ctx, &edge); err != nil {
			d.log.Error(err, "error creating edge", "edge", edge)
			return err
		}
	}

	return nil
}

func (d *Driver) calculateHTTPSEdges(domains *domainSet) map[string]ingressv1alpha1.HTTPSEdge {
	edgeMap := make(map[string]ingressv1alpha1.HTTPSEdge, len(domains.edgeIngressDomains))
	for _, domain := range domains.edgeIngressDomains {
		edge := ingressv1alpha1.HTTPSEdge{
			ObjectMeta: metav1.ObjectMeta{
				GenerateName: domain.Name + "-",
				Namespace:    domain.Namespace,
				Labels:       d.edgeLabels(),
			},
			Spec: ingressv1alpha1.HTTPSEdgeSpec{
				Hostports: []string{domain.Spec.Domain + ":443"},
			},
		}
		edge.Spec.Metadata = d.ingressNgrokMetadata
		edgeMap[domain.Spec.Domain] = edge
	}
	d.calculateHTTPSEdgesFromIngress(edgeMap)

	if d.gatewayEnabled {
		gatewayEdgeMap := make(map[string]ingressv1alpha1.HTTPSEdge)
		httproutes := d.store.ListHTTPRoutes()
		gateways := d.store.ListGateways()
		for _, gtw := range gateways {
			gatewayDomains := make(map[string]string)
			for _, listener := range gtw.Spec.Listeners {
				if listener.Hostname == nil {
					continue
				}
				if listener.Protocol != gatewayv1.HTTPSProtocolType || int(listener.Port) != 443 {
					continue
				}
				if _, hasDomain := domains.edgeGatewayDomains[string(*listener.Hostname)]; !hasDomain {
					continue
				}
				gatewayDomains[string(*listener.Hostname)] = string(*listener.Hostname)
			}
			if len(gatewayDomains) == 0 {
				d.log.Info("no usable domains in gateway, may be missing https listener", "gateway", gtw.Name)
				continue
			}
			for _, httproute := range httproutes {
				var routeDomains []string
				for _, parent := range httproute.Spec.ParentRefs {
					if string(parent.Name) != gtw.Name {
						continue
					}
					var domainOverlap []string
					for _, hostname := range httproute.Spec.Hostnames {
						domain := string(hostname)
						if _, hasDomain := gatewayDomains[domain]; hasDomain {
							domainOverlap = append(domainOverlap, domain)
						}
					}
					if len(domainOverlap) == 0 {
						// no hostnames overlap with gateway
						continue
					}
					routeDomains = append(routeDomains, domainOverlap...)
				}
				if len(routeDomains) == 0 {
					// no usable domains in route
					continue
				}
				var hostPorts []string

				for _, domain := range routeDomains {
					hostPorts = append(hostPorts, domain+":443")
				}
				edge := ingressv1alpha1.HTTPSEdge{
					ObjectMeta: metav1.ObjectMeta{
						GenerateName: httproute.Name + "-",
						Namespace:    httproute.Namespace,
						Labels:       d.edgeLabels(),
					},
					Spec: ingressv1alpha1.HTTPSEdgeSpec{
						Hostports: hostPorts,
					},
				}
				edge.Spec.Metadata = d.gatewayNgrokMetadata
				gatewayEdgeMap[routeDomains[0]] = edge

			}
		}
		d.calculateHTTPSEdgesFromGateway(gatewayEdgeMap)

		// merge edge maps
		for k, v := range gatewayEdgeMap {
			edgeMap[k] = v
		}
	}

	return edgeMap
}

func (d *Driver) calculateHTTPSEdgesFromIngress(edgeMap map[string]ingressv1alpha1.HTTPSEdge) {
	ingresses := d.store.ListNgrokIngressesV1()
	for _, ingress := range ingresses {
		// If this annotation is present and "true", then this ingress should result in an endpoint being created and not an edge
		if val, found := ingress.Annotations[annotationUseEndpoints]; found && strings.ToLower(val) == "true" {
			continue
		}

		modSet, err := d.getNgrokModuleSetForIngress(ingress)
		if err != nil {
			d.log.Error(err, "error getting ngrok moduleset for ingress", "ingress", ingress)
			continue
		}

		policyJSON, err := d.getTrafficPolicyJSON(ingress, modSet)
		if err != nil {
			d.log.Error(err, "error marshalling JSON Policy for ingress", "ingress", ingress)
			continue
		}

		for _, rule := range ingress.Spec.Rules {
			edge, ok := edgeMap[rule.Host]
			if !ok {
				d.log.Error(err, "could not find edge associated with rule", "host", rule.Host)
				continue
			}

			if modSet.Modules.TLSTermination != nil && modSet.Modules.TLSTermination.MinVersion != nil {
				edge.Spec.TLSTermination = &ingressv1alpha1.EndpointTLSTerminationAtEdge{
					MinVersion: ptr.Deref(modSet.Modules.TLSTermination.MinVersion, ""),
				}
			}

			if modSet.Modules.MutualTLS != nil {
				edge.Spec.MutualTLS = modSet.Modules.MutualTLS
			}

			// If any rule for an ingress matches, then it applies to this ingress
			for _, httpIngressPath := range rule.HTTP.Paths {
				matchType := "path_prefix"
				if httpIngressPath.PathType != nil {
					switch *httpIngressPath.PathType {
					case netv1.PathTypePrefix:
						matchType = "path_prefix"
					case netv1.PathTypeExact:
						matchType = "exact_path"
					case netv1.PathTypeImplementationSpecific:
						matchType = "path_prefix" // Path Prefix seems like a sane default for most cases
					default:
						d.log.Error(fmt.Errorf("unknown path type"), "unknown path type", "pathType", *httpIngressPath.PathType)
						continue
					}
				}

				// We only support service backends right now. TODO: support resource backends
				if httpIngressPath.Backend.Service == nil {
					continue
				}

				serviceName := httpIngressPath.Backend.Service.Name
				serviceUID, servicePort, err := d.getIngressBackend(*httpIngressPath.Backend.Service, ingress.Namespace)
				if err != nil {
					d.log.Error(err, "could not find port for service", "namespace", ingress.Namespace, "service", serviceName)
					continue
				}

				route := ingressv1alpha1.HTTPSEdgeRouteSpec{
					Match:     httpIngressPath.Path,
					MatchType: matchType,
					Backend: ingressv1alpha1.TunnelGroupBackend{
						Labels: d.ngrokLabels(ingress.Namespace, serviceUID, serviceName, servicePort),
					},
					CircuitBreaker:      modSet.Modules.CircuitBreaker,
					Compression:         modSet.Modules.Compression,
					IPRestriction:       modSet.Modules.IPRestriction,
					Headers:             modSet.Modules.Headers,
					OAuth:               modSet.Modules.OAuth,
					Policy:              policyJSON,
					OIDC:                modSet.Modules.OIDC,
					SAML:                modSet.Modules.SAML,
					WebhookVerification: modSet.Modules.WebhookVerification,
				}
				route.Metadata = d.ingressNgrokMetadata

				// Loop through existing routes and check if any match the path and match type
				// If they do, warn about it and continue replacing it
				for _, existingRoute := range edge.Spec.Routes {
					if existingRoute.Match == route.Match && existingRoute.MatchType == route.MatchType {
						d.log.Info("replacing existing route", "route", existingRoute.Match, "newRoute", route.Match)
						continue
					}
				}

				edge.Spec.Routes = append(edge.Spec.Routes, route)
			}

			edgeMap[rule.Host] = edge
		}
	}
}

func (d *Driver) calculateHTTPSEdgesFromGateway(edgeMap map[string]ingressv1alpha1.HTTPSEdge) {
	gateways := d.store.ListGateways()
	for _, gtw := range gateways {
		// If this annotation is present and "true", then this gateway should result in an endpoint being created and not an edge
		if val, found := gtw.Annotations[annotationUseEndpoints]; found && strings.ToLower(val) == "true" {
			continue
		}

		for _, listener := range gtw.Spec.Listeners {
			if listener.Hostname == nil {
				continue
			}
			allowedRoutes := listener.AllowedRoutes.Kinds
			if len(allowedRoutes) > 0 {
				createHttpsedge := false
				for _, routeKind := range allowedRoutes {
					if routeKind.Kind == "HTTPRoute" {
						createHttpsedge = true
					}
				}
				if !createHttpsedge {
					continue
				}
			}
			domainName := string(*listener.Hostname)
			edge, ok := edgeMap[domainName]
			if !ok {
				continue
			}
			// TODO: Calculate routes from httpRoutes
			// TODO: skip if no backend services
			httproutes := d.store.ListHTTPRoutes()
			for _, httproute := range httproutes {
				for _, parent := range httproute.Spec.ParentRefs {
					if string(parent.Name) != gtw.Name {
						// not our gateway so skip
						continue
					}

					if listener.AllowedRoutes != nil && listener.AllowedRoutes.Namespaces.From != nil {
						switch *listener.AllowedRoutes.Namespaces.From {
						case gatewayv1.NamespacesFromAll:
						case gatewayv1.NamespacesFromSame:
							if httproute.Namespace != gtw.Namespace {
								continue
							}
						case gatewayv1.NamespacesFromSelector:
							if httproute.Namespace != listener.AllowedRoutes.Namespaces.Selector.String() {
								continue
							}
						}
					}

					// matches our gateway
					for _, hostname := range httproute.Spec.Hostnames {
						if string(hostname) != string(*listener.Hostname) {
							// doesn't match this listener
							continue
						}
						// matches gateway and listener
						for _, rule := range httproute.Spec.Rules {
							// TODO: resolve rule.Matches
							// TODO: resolve rule.Filters
							// for v0 we will only resolve the first backendRef
							pathMatch := "/"
							pathMatchType := "path_prefix"
							// first match with a path will be accepted as the route's path
							for _, match := range rule.Matches {
								if match.Path != nil {
									pathMatch = *match.Path.Value
									if *match.Path.Type == gatewayv1.PathMatchExact {
										pathMatchType = "exact_path"
									}
									break
								}
							}
							route := ingressv1alpha1.HTTPSEdgeRouteSpec{
								Match:     pathMatch,     // change based on the rule.match
								MatchType: pathMatchType, // change based on rule.Matches
							}

							// TODO: set with values from rules.Filters + rules.Matches
							// this HTTPRouteRule comes direct from gateway api yaml, and func returns the policy,
							// which goes directly into the edge route in ngrok.
							policy, err := d.createEndpointPolicyForGateway(&rule, httproute.Namespace)
							if err != nil {
								d.log.Error(err, "error creating policy from HTTPRouteRule", "rule", rule)
								continue
							}

							route.Policy = policy

							for idx, backendref := range rule.BackendRefs {
								// currently the ingress controller doesn't support weighted backends
								// so we'll only support one backendref per rule
								// TODO: remove when tested with multiple backends
								if idx > 0 {
									break
								}
								// handle backendref
								refKind := string(*backendref.Kind)
								if refKind != "Service" {
									// only support services currently
									continue
								}

								refName := string(backendref.Name)
								serviceUID, servicePort, err := d.getEdgeBackendRef(backendref.BackendRef, httproute.Namespace)
								if err != nil {
									d.log.Error(err, "could not find port for service", "namespace", httproute.Namespace, "service", refName)
									continue
								}

								route.Backend = ingressv1alpha1.TunnelGroupBackend{
									Labels: d.ngrokLabels(httproute.Namespace, serviceUID, refName, servicePort),
								}

							}
							route.Metadata = d.gatewayNgrokMetadata

							edge.Spec.Routes = append(edge.Spec.Routes, route)
						}
					}
				}
			}

			edgeMap[domainName] = edge
		}
	}
}

func (d *Driver) getIngressBackend(backendSvc netv1.IngressServiceBackend, namespace string) (string, int32, error) {
	service, servicePort, err := d.findBackendServicePort(backendSvc, namespace)
	if err != nil {
		return "", 0, err
	}

	return string(service.UID), servicePort.Port, nil
}

func (d *Driver) getEdgeBackendRef(backendRef gatewayv1.BackendRef, namespace string) (string, int32, error) {
	if backendRef.Namespace != nil && string(*backendRef.Namespace) != namespace {
		return "", 0, fmt.Errorf("namespace %s not supported", string(*backendRef.Namespace))
	}
	service, servicePort, err := d.findBackendRefServicePort(backendRef, namespace)
	if err != nil {
		return "", 0, err
	}

	return string(service.UID), servicePort.Port, nil
}

func (d *Driver) edgeLabels() map[string]string {
	return map[string]string{
		labelControllerNamespace: d.managerName.Namespace,
		labelControllerName:      d.managerName.Name,
	}
}
