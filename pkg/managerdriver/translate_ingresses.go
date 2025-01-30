package managerdriver

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"

	"github.com/go-logr/logr"
	"github.com/ngrok/ngrok-operator/internal/annotations"
	"github.com/ngrok/ngrok-operator/internal/errors"
	"github.com/ngrok/ngrok-operator/internal/ir"
	"github.com/ngrok/ngrok-operator/internal/store"
	"github.com/ngrok/ngrok-operator/internal/trafficpolicy"
	netv1 "k8s.io/api/networking/v1"
)

// ingressesToIR fetches all stored ingresses and translates them into IR for futher processing and translation
func (t *translator) ingressesToIR() []*ir.IRVirtualHost {
	hostCache := make(map[ir.IRHostname]*ir.IRVirtualHost) // Each unique hostname corresponds to one IRVirtualHost
	upstreamCache := make(map[ir.IRService]*ir.IRUpstream) // Each unique service/port combo corresponds to one IRUpstream

	ingresses := t.store.ListNgrokIngressesV1()
	for _, ingress := range ingresses {
		// We currently require this annotation to be present for an Ingress to be translated into CloudEndpoints/AgentEndpoints, otherwise the default behaviour is to
		// translate it into HTTPSEdges (legacy). A future version will remove support for HTTPSEdges and translation into CloudEndpoints/AgentEndpoints will become the new
		// default behaviour.
		useEndpoints, err := annotations.ExtractUseEndpoints(ingress)
		if err != nil {
			t.log.Error(err, fmt.Sprintf("failed to check %q annotation. defaulting to using edges", annotations.MappingStrategyAnnotation))
		}
		if !useEndpoints {
			t.log.Info(fmt.Sprintf("the following ingress will be provided by ngrok edges instead of endpoints because of the %q annotation",
				annotations.MappingStrategyAnnotation),
				"ingress", fmt.Sprintf("%s.%s", ingress.Name, ingress.Namespace),
			)
			continue
		}

		useEndpointPooling, err := annotations.ExtractUseEndpointPooling(ingress)
		if err != nil {
			t.log.Error(err, fmt.Sprintf("failed to check %q annotation", annotations.MappingStrategyAnnotation))
		}
		if useEndpointPooling {
			t.log.Info(fmt.Sprintf("the following ingress will create endpoint(s) with pooling enabled because of the %q annotation",
				annotations.MappingStrategyAnnotation),
				"ingress", fmt.Sprintf("%s.%s", ingress.Name, ingress.Namespace),
			)
		}

		annotationTrafficPolicy, tpObjRef, err := trafficPolicyFromIngressAnnotation(t.store, ingress)
		if err != nil {
			t.log.Error(err, "error getting ngrok traffic policy for ingress",
				"ingress", fmt.Sprintf("%s.%s", ingress.Name, ingress.Namespace))
			continue
		}

		// If we don't have a native traffic policy from annotations, see if one was provided from a moduleset annotation
		if annotationTrafficPolicy == nil {
			annotationTrafficPolicy, tpObjRef, err = trafficPolicyFromIngressModSetAnnotation(t.log, t.store, ingress, useEndpoints)
			if err != nil {
				t.log.Error(err, "error getting ngrok traffic policy for ingress",
					"ingress", fmt.Sprintf("%s.%s", ingress.Name, ingress.Namespace))
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
				continue
			}
		}

		t.ingressToIR(
			ingress,
			defaultDestination,
			hostCache,
			upstreamCache,
			useEndpointPooling,
			annotationTrafficPolicy,
			tpObjRef,
		)
	}

	// Convert the cache of all the hosts we've built into a list
	ret := []*ir.IRVirtualHost{}
	for _, irVHost := range hostCache {
		ret = append(ret, irVHost)
	}
	return ret
}

// ingressToIR translates a single ingress into IR and stores entries in the cache. Caches are used so that we do not generate duplicate IR for hostnames/services
func (t *translator) ingressToIR(
	ingress *netv1.Ingress,
	defaultDestination *ir.IRDestination,
	hostCache map[ir.IRHostname]*ir.IRVirtualHost,
	upstreamCache map[ir.IRService]*ir.IRUpstream,
	endpointPoolingEnabled bool,
	annotationTrafficPolicy *trafficpolicy.TrafficPolicy,
	annotationTrafficPolicyRef *ir.OwningResource,
) {
	for _, rule := range ingress.Spec.Rules {
		ruleHostname := rule.Host
		if ruleHostname == "" {
			t.log.Error(fmt.Errorf("skipping converting ingress rule into cloud and agent endpoints because the rule.host is empty"),
				"empty host in ingress spec rule",
				"ingress", fmt.Sprintf("%s.%s", ingress.Name, ingress.Namespace),
			)
			continue
		}

		// Make a new IRVirtualHost for this hostname unless we have one in the cache
		owningResource := ir.OwningResource{
			Kind:      "Ingress",
			Name:      ingress.Name,
			Namespace: ingress.Namespace,
		}
		irVHost, exists := hostCache[ir.IRHostname(ruleHostname)]
		if exists {
			// If we already have a virtual host for this hostname, the traffic policy config must be the same as the one we are currently processing
			if !reflect.DeepEqual(irVHost.TrafficPolicyObj, annotationTrafficPolicyRef) {
				t.log.Error(fmt.Errorf("different traffic policy annotations provided for the same hostname"),
					"when using the same hostname across multiple ingresses, ensure that they do not use different traffic policies provided via annotations",
					"current ingress", fmt.Sprintf("%s.%s", ingress.Name, ingress.Namespace),
					"hostname", ruleHostname,
				)
				continue
			}
			// They must have the same configuration for whether or not to pool endpoints
			if irVHost.EndpointPoolingEnabled != endpointPoolingEnabled {
				t.log.Error(fmt.Errorf("different endpoint pooling annotations provided for the same hostname"),
					"when using the same hostname across multiple ingresses, ensure that they all enable or all disable endpoint pooling",
					"current ingress", fmt.Sprintf("%s.%s", ingress.Name, ingress.Namespace),
					"hostname", ruleHostname,
				)
				continue
			}

			// They must share the same namespace
			if irVHost.Namespace != ingress.Namespace {
				t.log.Error(fmt.Errorf("unable to convert ingress rule into cloud and agent endpoints. the domain (%q) is already being used by another ingress in a different namespace. you will need to either consolidate them, ensure they are in the same namespace, or use a different domain for one of them", ruleHostname),
					"ingress to endpoint conversion error",
					"ingress", fmt.Sprintf("%s.%s", ingress.Name, ingress.Namespace),
					"namespace the hostname is already in-use in", irVHost.Namespace,
				)
				continue
			}

			// They must have the same default backend
			if !reflect.DeepEqual(irVHost.DefaultDestination, defaultDestination) {
				t.log.Error(fmt.Errorf("different ingress default backends provided for the same hostname"),
					"when using the same hostname across multiple ingresses, ensure that they do not use different default backends. the existing default backend for the hostname will not be overwritten",
					"current ingress", fmt.Sprintf("%s.%s", ingress.Name, ingress.Namespace),
					"hostname", ruleHostname,
				)
				continue
			}

			// The current and existing configurations match, add the new owning ingress reference and keep going
			irVHost.AddOwningResource(owningResource)
		} else {
			// Make a deep copy of the ingress traffic policy so that we don't taint it for subsequent rules
			var ruleTrafficPolicy *trafficpolicy.TrafficPolicy
			if annotationTrafficPolicy != nil {
				var err error
				ruleTrafficPolicy, err = annotationTrafficPolicy.DeepCopy()
				if err != nil {
					t.log.Error(err, "failed to copy traffic policy from ingress",
						"ingress", fmt.Sprintf("%s.%s", ingress.Name, ingress.Namespace),
					)
					continue
				}
			}

			irVHost = &ir.IRVirtualHost{
				Namespace:              ingress.Namespace,
				OwningResources:        []ir.OwningResource{owningResource},
				Hostname:               ruleHostname,
				TrafficPolicy:          ruleTrafficPolicy,
				TrafficPolicyObj:       annotationTrafficPolicyRef,
				Routes:                 []*ir.IRRoute{},
				DefaultDestination:     defaultDestination,
				EndpointPoolingEnabled: endpointPoolingEnabled,
			}
			hostCache[ir.IRHostname(ruleHostname)] = irVHost
		}

		if rule.HTTP == nil {
			t.log.Info("skipping generating endpoints for ingress rule with empty http section")
			continue
		}

		irRoutes := t.ingressPathsToIR(ingress, ruleHostname, rule.HTTP.Paths, upstreamCache)
		irVHost.Routes = append(irVHost.Routes, irRoutes...)

		hostCache[ir.IRHostname(ruleHostname)] = irVHost
	}
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

func trafficPolicyFromIngressAnnotation(store store.Storer, ingress *netv1.Ingress) (tp *trafficpolicy.TrafficPolicy, objRef *ir.OwningResource, err error) {
	tpName, err := annotations.ExtractNgrokTrafficPolicyFromAnnotations(ingress)
	if err != nil {
		if errors.IsMissingAnnotations(err) {
			return nil, nil, nil
		}
		return nil, nil, fmt.Errorf("error getting ngrok traffic policy for ingress %q: %w",
			fmt.Sprintf("%s.%s", ingress.Name, ingress.Namespace),
			err,
		)
	}

	tpObj, err := store.GetNgrokTrafficPolicyV1(tpName, ingress.Namespace)
	if err != nil {
		return nil, nil, fmt.Errorf("unable to load traffic policy for ingress from annotations. name: %q, namespace: %q: %w",
			tpName,
			ingress.Namespace,
			err,
		)
	}

	trafficPolicyCfg := &trafficpolicy.TrafficPolicy{}
	if err := json.Unmarshal(tpObj.Spec.Policy, trafficPolicyCfg); err != nil {
		return nil, nil, fmt.Errorf("%w, failed to unmarshal traffic policy for ingress %q, traffic policy config: %v",
			err,
			fmt.Sprintf("%s.%s", ingress.Name, ingress.Namespace),
			tpObj.Spec.Policy,
		)
	}
	return trafficPolicyCfg, &ir.OwningResource{
		Kind:      "NgrokTrafficPolicy",
		Name:      tpObj.Name,
		Namespace: tpObj.Namespace,
	}, nil
}

func trafficPolicyFromIngressModSetAnnotation(log logr.Logger, store store.Storer, ingress *netv1.Ingress, useEndpoints bool) (tp *trafficpolicy.TrafficPolicy, objRef *ir.OwningResource, err error) {
	// We don't support modulesets on endpoints or currently support converting a moduleset to a traffic policy, but still try to allow
	// a moduleset that supplies a traffic policy with an error log to let users know that any other moduleset fields will be ignored
	ingressModuleSet, err := getNgrokModuleSetForIngress(ingress, store)
	if err != nil {
		return nil, nil, err
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
		if useEndpoints {
			log.Error(fmt.Errorf("ngrok moduleset supplied to ingress with annotation to use endpoints instead of edges"), "ngrok moduleset are not supported on endpoints. prefer using a traffic policy directly. any fields other than supplying a traffic policy using the module set will be ignored",
				"ingress", fmt.Sprintf("%s.%s", ingress.Name, ingress.Namespace),
			)
		}
	}

	if ingressModuleSet.Modules.Policy == nil {
		return nil, nil, nil
	}

	tpJSON, err := json.Marshal(ingressModuleSet.Modules.Policy)
	if err != nil {
		return nil, nil, fmt.Errorf("%w: cannot convert module-set policy json for ingress %q, moduleset policy: %v",
			err,
			fmt.Sprintf("%s.%s", ingress.Name, ingress.Namespace),
			ingressModuleSet.Modules.Policy,
		)
	}
	var ingressTrafficPolicy *trafficpolicy.TrafficPolicy
	if err := json.Unmarshal(tpJSON, ingressTrafficPolicy); err != nil {
		return nil, nil, fmt.Errorf("%w: failed to unmarshal traffic policy from module set for ingress %q, moduleset policy: %v",
			err,
			fmt.Sprintf("%s.%s", ingress.Name, ingress.Namespace),
			ingressModuleSet.Modules.Policy,
		)
	}

	return ingressTrafficPolicy, &ir.OwningResource{
		Kind:      "NgrokModuleSet",
		Name:      ingressModuleSet.Name,
		Namespace: ingressModuleSet.Namespace,
	}, nil
}
