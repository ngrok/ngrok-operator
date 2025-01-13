package managerdriver

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"

	common "github.com/ngrok/ngrok-operator/api/common/v1alpha1"
	"github.com/ngrok/ngrok-operator/internal/ir"
	"github.com/ngrok/ngrok-operator/internal/trafficpolicy"
	netv1 "k8s.io/api/networking/v1"
)

// ingressesToIR fetches all stored ingresses and translates them into IR for futher processing and translation
func (t *translator) ingressesToIR() []*ir.IRVirtualHost {
	hostCache := make(map[ir.IRHostname]*ir.IRVirtualHost) // Each unique hostname corresponds to one IRVirtualHost
	upstreamCache := make(map[ir.IRService]*ir.IRUpstream) // Each unique service/port combo corresponds to one IRUpstream

	// The following two maps keep track of traffic policy annotations and ingress backends for hostnames
	// so that we can handle the case where two ingresses bringing different ones for the same hostname as an error
	hostnameDefaultDestinations := make(map[ir.IRHostname]*ir.IRDestination)
	hostnameAnnotationPolicies := make(map[ir.IRHostname]*trafficpolicy.TrafficPolicy)

	irVHosts := []*ir.IRVirtualHost{}
	ingresses := t.store.ListNgrokIngressesV1()
	for _, ingress := range ingresses {
		// We currently require this annotation to be present for an Ingress to be translated into CloudEndpoints/AgentEndpoints, otherwise the default behaviour is to
		// translate it into HTTPSEdges (legacy). A future version will remove support for HTTPSEdges and translation into CloudEndpoints/AgentEndpoints will become the new
		// default behaviour.
		if !common.HasUseEndpointsAnnotation(ingress.Annotations) {
			t.log.Info("The following ingress will be provided by ngrok edges instead of endpoints because it is missing the use-endpoints annotation",
				"ingress", fmt.Sprintf("%s.%s", ingress.Name, ingress.Namespace),
			)
			continue
		}

		// We don't support modulesets on endpoints or currently support converting a moduleset to a traffic policy, but still try to allow
		// a moduleset that supplies a traffic policy with an error log to let users know that any other moduleset fields will be ignored
		ingressModuleSet, err := getNgrokModuleSetForIngress(ingress, t.store)
		if err != nil {
			t.log.Error(err, "error getting ngrok moduleset for ingress", "ingress", ingress)
			continue
		}

		// We always get back a moduleset from the above function, check if it is empty or not
		if modules := ingressModuleSet.Modules; modules.CircuitBreaker != nil ||
			modules.Compression != nil ||
			modules.Headers != nil ||
			modules.IPRestriction != nil ||
			modules.OAuth != nil ||
			modules.Policy != nil ||
			modules.OIDC != nil ||
			modules.SAML != nil ||
			modules.TLSTermination != nil ||
			modules.MutualTLS != nil ||
			modules.WebhookVerification != nil {
			if common.HasUseEndpointsAnnotation(ingress.Annotations) {
				t.log.Error(fmt.Errorf("ngrok moduleset supplied to ingress with annotation to use endpoints instead of edges"), "ngrok moduleset are not supported on endpoints. prefer using a traffic policy directly. any fields other than supplying a traffic policy using the module set will be ignored",
					"ingress", fmt.Sprintf("%s.%s", ingress.Name, ingress.Namespace),
				)
			}
		}

		ingressTrafficPolicyCfg, err := getNgrokTrafficPolicyForIngress(ingress, t.store)
		if err != nil {
			t.log.Error(err, "error getting ngrok traffic policy for ingress",
				"ingress", fmt.Sprintf("%s.%s", ingress.Name, ingress.Namespace),
			)
			continue
		}

		var ingressTrafficPolicy *trafficpolicy.TrafficPolicy
		switch {
		case ingressTrafficPolicyCfg != nil:
			tmp := &trafficpolicy.TrafficPolicy{}
			if err := json.Unmarshal(ingressTrafficPolicyCfg.Spec.Policy, tmp); err != nil {
				t.log.Error(err, "failed to unmarshal traffic policy",
					"ingress", fmt.Sprintf("%s.%s", ingress.Name, ingress.Namespace),
					"policy", ingressTrafficPolicyCfg.Spec.Policy,
				)
				continue
			}
			ingressTrafficPolicy = tmp
		case ingressModuleSet.Modules.Policy != nil:
			tpJSON, err := json.Marshal(ingressModuleSet.Modules.Policy)
			if err != nil {
				t.log.Error(err, "cannot convert module-set policy json",
					"ingress", ingress,
					"policy", ingressModuleSet.Modules.Policy,
				)
				continue
			}
			if err := json.Unmarshal(tpJSON, ingressTrafficPolicy); err != nil {
				t.log.Error(err, "failed to unmarshal traffic policy from module set",
					"ingress", fmt.Sprintf("%s.%s", ingress.Name, ingress.Namespace),
					"policy", ingressModuleSet.Modules.Policy,
				)
				continue
			}
		}

		var defaultDestination *ir.IRDestination
		if ingress.Spec.DefaultBackend != nil {
			defaultDestination, err = t.ingressBackendToIR(ingress, ingress.Spec.DefaultBackend, upstreamCache)
			if err != nil {
				t.log.Error(err, "unable to resolve ingress default backend",
					"ingress", fmt.Sprintf("%s.%s", ingress.Name, ingress.Namespace),
				)
				return irVHosts
			}
		}

		newIRVHosts := t.ingressToIR(
			ingress,
			ingressTrafficPolicy,
			defaultDestination,
			hostCache,
			upstreamCache,
			hostnameDefaultDestinations,
			hostnameAnnotationPolicies,
		)
		irVHosts = append(irVHosts, newIRVHosts...)
	}
	return irVHosts
}

// ingressToIR translates a single ingress into IR but needs input caches for the hosts and services so that we do not generate duplicate IR for them
func (t *translator) ingressToIR(
	ingress *netv1.Ingress,
	ingressTP *trafficpolicy.TrafficPolicy,
	defaultDestination *ir.IRDestination,
	hostCache map[ir.IRHostname]*ir.IRVirtualHost,
	upstreamCache map[ir.IRService]*ir.IRUpstream,
	hostnameDefaultDestinations map[ir.IRHostname]*ir.IRDestination,
	hostnameAnnotationPolicies map[ir.IRHostname]*trafficpolicy.TrafficPolicy,
) []*ir.IRVirtualHost {
	irVHosts := []*ir.IRVirtualHost{}

	for _, rule := range ingress.Spec.Rules {
		ruleHostname := rule.Host
		if ruleHostname == "" {
			t.log.Error(fmt.Errorf("skipping converting ingress rule into cloud and agent endpoints because the rule.host is empty"),
				"empty host in ingress spec rule",
				"ingress", fmt.Sprintf("%s.%s", ingress.Name, ingress.Namespace),
			)
			continue
		}

		// Check for clashing default backends and annotation traffic policies for this hostname
		if defaultDestination != nil {
			if current, exists := hostnameDefaultDestinations[ir.IRHostname(ruleHostname)]; exists {
				if !reflect.DeepEqual(current, defaultDestination) {
					t.log.Error(fmt.Errorf("different ingress default backends provided for the same hostname"),
						"when using the same hostname across multiple ingresses, ensure that they do not use different default backends. the existing default backend for the hostname will not be overwritten",
						"current ingress", fmt.Sprintf("%s.%s", ingress.Name, ingress.Namespace),
						"hostname", ruleHostname,
					)
					defaultDestination = current
				}
			}
			hostnameDefaultDestinations[ir.IRHostname(ruleHostname)] = defaultDestination
		}
		if ingressTP != nil {
			if current, exists := hostnameAnnotationPolicies[ir.IRHostname(ruleHostname)]; exists {
				if !reflect.DeepEqual(current, ingressTP) {
					t.log.Error(fmt.Errorf("different traffic policy annotations provided for the same hostname"),
						"when using the same hostname across multiple ingresses, ensure that they do not use different traffic policies provided via annotations. the existing traffic policy for the hostname will not be overwitten",
						"current ingress", fmt.Sprintf("%s.%s", ingress.Name, ingress.Namespace),
						"hostname", ruleHostname,
					)
					ingressTP = current
				}
			}
			hostnameAnnotationPolicies[ir.IRHostname(ruleHostname)] = ingressTP
		}

		// Make a new IRVirtualHost for this hostname unless we have one in the cache
		owningResource := ir.OwningResource{
			Kind:      "Ingress",
			Name:      ingress.Name,
			Namespace: ingress.Namespace,
		}
		irVHost, exists := hostCache[ir.IRHostname(ruleHostname)]
		if !exists {
			irVHost = &ir.IRVirtualHost{
				Namespace:          ingress.Namespace,
				OwningResources:    []ir.OwningResource{owningResource},
				Hostname:           ruleHostname,
				TrafficPolicy:      ingressTP,
				Routes:             []*ir.IRRoute{},
				DefaultDestination: defaultDestination,
			}
			hostCache[ir.IRHostname(ruleHostname)] = irVHost
		} else {
			if irVHost.Namespace != ingress.Namespace {
				t.log.Error(fmt.Errorf("unable to convert ingress rule into cloud and agent endpoints. the domain (%q) is already being used by another ingress in a different namespace. you will need to either consolidate them, ensure they are in the same namespace, or use a different domain for one of them", ruleHostname),
					"ingress to endpoint conversion error",
					"ingress", fmt.Sprintf("%s.%s", ingress.Name, ingress.Namespace),
				)
				continue
			}
			irVHost.AddOwningResource(owningResource)
		}

		if rule.HTTP == nil {
			t.log.Info("skipping generating endpoints for ingress rule with empty http section")
			continue
		}

		irRoutes := t.ingressPathsToIR(ingress, ruleHostname, rule.HTTP.Paths, upstreamCache)
		irVHost.Routes = append(irVHost.Routes, irRoutes...)
		irVHosts = append(irVHosts, irVHost)
	}
	return irVHosts
}

// ingressPathsToIR constructs IRRoutes for the path matches under a given ingress rule
func (t *translator) ingressPathsToIR(ingress *netv1.Ingress, ruleHostname string, ingressPaths []netv1.HTTPIngressPath, upstreamCache map[ir.IRService]*ir.IRUpstream) []*ir.IRRoute {
	irRoutes := []*ir.IRRoute{}
	for _, pathMatch := range ingressPaths {
		destination, err := t.ingressBackendToIR(ingress, &pathMatch.Backend, upstreamCache)
		if err != nil {
			t.log.Error(err, "ingress rule could not be successfully processed. other ingress rules will continue to be evaluated",
				"ingress", fmt.Sprintf("%s.%s", ingress.Name, ingress.Namespace),
				"hostname", ruleHostname,
				"path", pathMatch.Path,
			)
			continue
		}

		irRoutes = append(irRoutes, &ir.IRRoute{
			Path:        pathMatch.Path,
			PathType:    getPathMatchType(t.log, pathMatch.PathType),
			Destination: destination,
		})
	}
	return irRoutes
}

// ingressBackendToIR constructs an IRDestination from an ingress backend. Currently only service and traffic policies are supported
func (t *translator) ingressBackendToIR(ingress *netv1.Ingress, backend *netv1.IngressBackend, upstreamCache map[ir.IRService]*ir.IRUpstream) (*ir.IRDestination, error) {
	// First check if we are supplying a traffic policy as the backend
	if resourceRef := backend.Resource; resourceRef != nil {
		if strings.ToLower(resourceRef.Kind) != "ngroktrafficpolicy" {
			return nil, fmt.Errorf("ingress backend resource reference to unsupported kind: %q. currently only NgrokTrafficPolicy is supported for resource backends", resourceRef.Kind)
		}

		if resourceRef.APIGroup != nil && *resourceRef.APIGroup != "ngrok.k8s.ngrok.com" {
			return nil, fmt.Errorf("ingress backend resource to invalid group: %q. currently only NgrokTrafficPolicy is supported for resource backends with API Group \"ngrok.k8s.ngrok.com\"", *resourceRef.APIGroup)
		}

		routePolicyCfg, err := t.store.GetNgrokTrafficPolicyV1(resourceRef.Name, ingress.Namespace)
		if err != nil {
			return nil, fmt.Errorf("unable to resolve traffic policy backend for ingress rule: %w", err)
		}

		var routeTrafficPolicy trafficpolicy.TrafficPolicy
		if len(routePolicyCfg.Spec.Policy) > 0 {
			if err := json.Unmarshal(routePolicyCfg.Spec.Policy, &routeTrafficPolicy); err != nil {
				return nil, fmt.Errorf("failed to unmarshal traffic policy: %w. raw traffic policy: %v", err, routePolicyCfg.Spec.Policy)
			}
		}

		return &ir.IRDestination{
			TrafficPolicy: &routeTrafficPolicy,
		}, nil
	}

	// If the backend is not a traffic policy, then it must be a service
	if backend.Service == nil {
		return nil, fmt.Errorf("ingress backend is invalid. Not an NgrokTrafficPolicy or service")
	}

	serviceName := backend.Service.Name
	service, err := t.store.GetServiceV1(serviceName, ingress.Namespace)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve backend service name: %q in namespace %q: %w",
			serviceName,
			ingress.Namespace,
			err,
		)
	}

	servicePort, err := findServicesPort(t.log, service, backend.Service.Port)
	if err != nil || servicePort == nil {
		return nil, fmt.Errorf("failed to resolve backend service's port. name: %q, namespace: %q: %w",
			serviceName,
			ingress.Namespace,
			err,
		)
	}

	destination := ir.IRService{
		UID:       string(service.UID),
		Name:      serviceName,
		Namespace: ingress.Namespace,
		Port:      servicePort.Port,
	}
	owningResource := ir.OwningResource{
		Kind:      "Ingress",
		Name:      ingress.Name,
		Namespace: ingress.Namespace,
	}
	upstream, exists := upstreamCache[destination]
	if !exists {
		upstream = &ir.IRUpstream{
			Service:         destination,
			OwningResources: []ir.OwningResource{owningResource},
		}
		upstreamCache[destination] = upstream
	} else {
		upstream.AddOwningResource(owningResource)
	}

	return &ir.IRDestination{
		Upstream: upstream,
	}, nil
}
