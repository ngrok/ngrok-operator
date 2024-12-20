package managerdriver

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"sort"
	"strings"

	ngrokv1alpha1 "github.com/ngrok/ngrok-operator/api/ngrok/v1alpha1"
	netv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	labelFromIngresses = "k8s.ngrok.com/from-ingresses"
)

// getPathMatchType validates an ingress
func (d *Driver) getPathMatchType(pathType *netv1.PathType) netv1.PathType {
	if pathType == nil {
		return netv1.PathTypePrefix
	}

	switch *pathType {
	case netv1.PathTypePrefix, netv1.PathTypeImplementationSpecific:
		return netv1.PathTypePrefix
	case netv1.PathTypeExact:
		return netv1.PathTypeExact
	default:
		d.log.Error(fmt.Errorf("unknown path type, defaulting to prefix match"), "unknown path type", "pathType", *pathType)
		return netv1.PathTypePrefix
	}
}

func (d *Driver) SyncEndpoints(ctx context.Context, c client.Client) error {
	if !d.syncAllowConcurrent {
		if proceed, wait := d.syncStart(true); proceed {
			defer d.syncDone()
		} else {
			return wait(ctx)
		}
	}

	d.log.Info("syncing cloud and agent endpoints state!!")
	desiredCloudEndpoints, desiredAgentEndpoints := d.calculateEndpoints()
	currentAgentEndpoints := &ngrokv1alpha1.AgentEndpointList{}
	currentCloudEndpoints := &ngrokv1alpha1.CloudEndpointList{}

	if err := c.List(ctx, currentAgentEndpoints, client.MatchingLabels{
		labelControllerNamespace: d.managerName.Namespace,
		labelControllerName:      d.managerName.Name,
	}); err != nil {
		d.log.Error(err, "error listing agent endpoints")
		return err
	}

	if err := c.List(ctx, currentCloudEndpoints, client.MatchingLabels{
		labelControllerNamespace: d.managerName.Namespace,
		labelControllerName:      d.managerName.Name,
	}); err != nil {
		d.log.Error(err, "error listing cloud endpoints")
		return err
	}

	if err := d.applyAgentEndpoints(ctx, c, desiredAgentEndpoints, currentAgentEndpoints.Items); err != nil {
		d.log.Error(err, "applying agent endpoints")
		return err
	}
	if err := d.applyCloudEndpoints(ctx, c, desiredCloudEndpoints, currentCloudEndpoints.Items); err != nil {
		d.log.Error(err, "applying cloud endpoints")
		return err
	}

	return nil
}

func (d *Driver) applyAgentEndpoints(ctx context.Context, c client.Client, desired map[types.NamespacedName]*ngrokv1alpha1.AgentEndpoint, current []ngrokv1alpha1.AgentEndpoint) error {
	// update or delete agent endpoints we don't need anymore
	for _, currAEP := range current {

		// If this AgentEndpoint is created by the user and not owned/managed by the operator then ignore it
		if !hasDefaultManagedResourceLabels(currAEP.Labels, d.managerName.Name, d.managerName.Namespace) {
			continue
		}

		objectKey := types.NamespacedName{
			Name:      currAEP.Name,
			Namespace: currAEP.Namespace,
		}
		if desiredAEP, exists := desired[objectKey]; exists {
			needsUpdate := false

			if !reflect.DeepEqual(desiredAEP.Spec, currAEP.Spec) {
				currAEP.Spec = desiredAEP.Spec
				needsUpdate = true
			}

			if needsUpdate {
				if err := c.Update(ctx, &currAEP); err != nil {
					d.log.Error(err, "error updating agent endpoint", "desired", desiredAEP, "current", currAEP)
					return err
				}
			}

			// matched and updated the agent endpoint, no longer desired
			delete(desired, objectKey)
		} else {
			if err := c.Delete(ctx, &currAEP); client.IgnoreNotFound(err) != nil {
				d.log.Error(err, "error deleting agent endpoint", "current agent endpoints", currAEP)
				return err
			}
		}
	}

	// the set of desired agent endpoints now only contains new agent endpoints, create them
	for _, agentEndpoint := range desired {
		if err := c.Create(ctx, agentEndpoint); err != nil {
			d.log.Error(err, "error creating agent endpoint", "agent endpoint", agentEndpoint)
			return err
		}
	}

	return nil
}

func (d *Driver) applyCloudEndpoints(ctx context.Context, c client.Client, desired map[types.NamespacedName]*ngrokv1alpha1.CloudEndpoint, current []ngrokv1alpha1.CloudEndpoint) error {
	// update or delete cloud endpoints we don't need anymore
	for _, currCLEP := range current {

		// If this CloudEndpoint is created by the user and not owned/managed by the operator then ignore it
		if !hasDefaultManagedResourceLabels(currCLEP.Labels, d.managerName.Name, d.managerName.Namespace) {
			continue
		}

		objectKey := types.NamespacedName{
			Name:      currCLEP.Name,
			Namespace: currCLEP.Namespace,
		}
		if desiredAEP, exists := desired[objectKey]; exists {
			needsUpdate := false

			if !reflect.DeepEqual(desiredAEP.Spec, currCLEP.Spec) {
				currCLEP.Spec = desiredAEP.Spec
				needsUpdate = true
			}

			if needsUpdate {
				if err := c.Update(ctx, &currCLEP); err != nil {
					d.log.Error(err, "error updating cloud endpoint", "desired", desiredAEP, "current", currCLEP)
					return err
				}
			}

			// matched and updated the cloud endpoint, no longer desired
			delete(desired, objectKey)
		} else {
			if err := c.Delete(ctx, &currCLEP); client.IgnoreNotFound(err) != nil {
				d.log.Error(err, "error deleting cloud endpoint", "cloud endpoint", currCLEP)
				return err
			}
		}
	}

	// the set of desired cloud endpoints now only contains new cloud endpoints, create them
	for _, cloudEndpoint := range desired {
		if err := c.Create(ctx, cloudEndpoint); err != nil {
			d.log.Error(err, "error creating cloud endpoint", "cloud endpoint", cloudEndpoint)
			return err
		}
	}

	return nil
}

// calculateEndpoints calculates all of the AgentEndpoints and CloudEndpoints from ingress and GatewayAPI resources and returns unified maps for each type
func (d *Driver) calculateEndpoints() (map[types.NamespacedName]*ngrokv1alpha1.CloudEndpoint, map[types.NamespacedName]*ngrokv1alpha1.AgentEndpoint) {
	cloudEndpoints := make(map[types.NamespacedName]*ngrokv1alpha1.CloudEndpoint)
	agentEndpoints := make(map[types.NamespacedName]*ngrokv1alpha1.AgentEndpoint)

	ingressCloudEndpoints, ingressAgentEndpoints := d.ingressesToEndpoints()
	for key, val := range ingressCloudEndpoints {
		cloudEndpoints[key] = val
	}
	for key, val := range ingressAgentEndpoints {
		agentEndpoints[key] = val
	}

	if d.gatewayEnabled {
		gatewayCloudEndpoints, gatewayAgentEndpoints := d.gatewayAPIToEndpoints()
		for key, val := range gatewayCloudEndpoints {
			cloudEndpoints[key] = val
		}
		for key, val := range gatewayAgentEndpoints {
			agentEndpoints[key] = val
		}
	}

	return cloudEndpoints, agentEndpoints
}

func (d *Driver) gatewayAPIToEndpoints() (parents map[types.NamespacedName]*ngrokv1alpha1.CloudEndpoint, children map[types.NamespacedName]*ngrokv1alpha1.AgentEndpoint) {
	// TODO: implement Gateway API support
	return nil, nil
}

func (d *Driver) ingressesToEndpoints() (parents map[types.NamespacedName]*ngrokv1alpha1.CloudEndpoint, children map[types.NamespacedName]*ngrokv1alpha1.AgentEndpoint) {
	ingresses := d.store.ListNgrokIngressesV1()

	// Setup a couple structs that will only be used within this function to help us construct a  "route table" so that we can properly
	// convert and merge all applicable ingresses into the correct set of CloudEndpoints and AgentEndpoints. For example, there may be
	// more than one ingress using the same hostname, so we need to traverse all of them before constructing our traffic policy routing rules.
	type UpstreamKey struct {
		Namespace string
		Name      string
		Port      int32
	}

	type PathMatch struct {
		Path          string
		PathMatchType netv1.PathType
	}

	childEndpointCache := make(map[UpstreamKey]*ngrokv1alpha1.AgentEndpoint)
	parentEndpointCache := make(map[string]*ngrokv1alpha1.CloudEndpoint) // Key is the domain
	routeTable := make(map[string]map[PathMatch]UpstreamKey)             // First key is the domain

	for _, ingress := range ingresses {
		// We currently require this annotation to be present for an Ingress to be translated into CloudEndpoints/AgentEndpoints, otherwise the default behaviour is to
		// translate it into HTTPSEdges (legacy). A future version will remove support for HTTPSEdges and translation into CloudEndpoints/AgentEndpoints will become the new
		// default behaviour.
		if val, exists := ingress.Annotations[annotationUseEndpoints]; !exists || strings.ToLower(val) != "true" {
			continue
		}

		// We don't support modulesets on endpoints or currently support converting a moduleset to a traffic policy, but still try to allow
		// a moduleset that supplies a traffic policy with an error log to let users know that any other moduleset fields will be ignored
		ingressModuleSet, err := d.getNgrokModuleSetForIngress(ingress)
		if err != nil {
			d.log.Error(err, "error getting ngrok moduleset for ingress", "ingress", ingress)
			continue
		}

		if ingressModuleSet != nil {
			d.log.Info("ngrok moduleset supplied to ingress with annotation to use endpoints instead of edges. ngrok moduleset are not supported on endpoints. prefer using a traffic policy directly. any fields other than supplying a traffic policy using the module set will be ignored",
				"ingress", fmt.Sprintf("%s.%s", ingress.Name, ingress.Namespace),
			)
		}

		ingressTrafficPolicyJSON, err := d.getTrafficPolicyJSON(ingress, ingressModuleSet)
		if err != nil {
			d.log.Error(err, "error marshalling JSON Policy for ingress",
				"ingress", ingress,
			)
			continue
		}

		for _, rule := range ingress.Spec.Rules {
			hostname := rule.Host
			if hostname == "" {
				d.log.Error(fmt.Errorf("skipping converting ingress rule into cloud and agent endpoints because the rule.host is empty"),
					"empty host in ingress spec rule",
					"ingress", fmt.Sprintf("%s.%s", ingress.Name, ingress.Namespace),
				)
				continue
			}

			// Make a new CloudEndpoint for this hostname unless we already have one in the cache
			parentEndpoint, exists := parentEndpointCache[hostname]
			if !exists {
				parentEndpoint = buildCloudEndpoint(ingress.Namespace, hostname, d.defaultManagedResourceLabels(), d.ingressNgrokMetadata)
				parentEndpointCache[hostname] = parentEndpoint
			} else {
				// If we found one, make sure that the existing CloudEndpoint is in the same namespace as the ingress we are currently processing
				if parentEndpoint.Namespace != ingress.Namespace {
					d.log.Error(fmt.Errorf("unable to convert ingress rule into cloud and agent endpoints. the domain (%q) is already being used by another ingress in a different namespace. you will need to either consolidate them, ensure they are in the same namespace, or use a different domain for one of them", hostname),
						"ingress to endpoint conversion error",
						"ingress", fmt.Sprintf("%s.%s", ingress.Name, ingress.Namespace),
					)
					continue
				}
			}
			appendToLabel(parentEndpoint.Labels, labelFromIngresses, ingress.Name)

			// Initialize routeTable entry for this hostname
			if _, exists := routeTable[hostname]; !exists {
				routeTable[hostname] = make(map[PathMatch]UpstreamKey)
			}

			if len(ingressTrafficPolicyJSON) > 0 {
				parentEndpoint.Spec.TrafficPolicy = &ngrokv1alpha1.NgrokTrafficPolicySpec{
					Policy: ingressTrafficPolicyJSON,
				}
			}

			// Go through all of the paths for this rule, creating child AgentEndpoints for each upstream that the parent CloudEndpoint will route to
			// using traffic policy configuration
			for _, pathMatch := range rule.HTTP.Paths {

				// We only support service backends right now. TODO: support resource backends
				if pathMatch.Backend.Service == nil {
					d.log.Error(fmt.Errorf("skipping building endpoint for ingress rule since the backend is not a service. currently only service backends are supported. any other valid rules from this ingress will continue to be processed"),
						"ingress to endpoint conversion error",
						"ingress", fmt.Sprintf("%s.%s", ingress.Name, ingress.Namespace),
						"rule host", hostname,
						"rule path", pathMatch.Path,
					)
					continue
				}

				pathMatchType := d.getPathMatchType(pathMatch.PathType)
				serviceName := pathMatch.Backend.Service.Name
				_, servicePort, err := d.getIngressBackend(*pathMatch.Backend.Service, ingress.Namespace)
				if err != nil {
					d.log.Error(fmt.Errorf("skipping building endpoint for ingress rule. unable to extrace service name and port from ingress rule. any other valid rules from this ingress will continue to be processed"),
						"ingress to endpoint conversion error",
						"ingress", fmt.Sprintf("%s.%s", ingress.Name, ingress.Namespace),
						"rule host", hostname,
						"rule path", pathMatch.Path,
					)
				}

				upstreamKey := UpstreamKey{
					Namespace: ingress.Namespace,
					Name:      serviceName,
					Port:      servicePort,
				}

				// Create a new AgentEndpoint for this upstream unless we already have one in the cache
				childEndpoint, exists := childEndpointCache[upstreamKey]
				if !exists {
					childEndpoint = buildInternalAgentEndpoint(serviceName, ingress.Namespace, d.clusterDomain, servicePort, d.defaultManagedResourceLabels(), d.ingressNgrokMetadata)
					childEndpointCache[upstreamKey] = childEndpoint
				}
				appendToLabel(childEndpoint.Labels, labelFromIngresses, ingress.Name)

				// Add the information to the route table
				pathMatch := PathMatch{
					Path:          pathMatch.Path,
					PathMatchType: pathMatchType,
				}
				routeTable[hostname][pathMatch] = upstreamKey
			}
		}
	}

	// Use the route table to construct traffic policy configuration for the parent CloudEndpoints so that they route to the correct AgentEndpoints
	for hostname, routeMap := range routeTable {
		parentEndpoint, exists := parentEndpointCache[hostname]
		if !exists {
			d.log.Error(fmt.Errorf("a CloudEndpoint should have been created for hostname but could not be found. this is a bug and not a result of the supplied ingress configuration"),
				"ingress to endpoint conversion error",
				"hostname", hostname,
			)
			continue
		}

		// Get an interface from the traffic policy for the CloudEndpoint. If there is no user-supplied traffic policy we can append to, then a new one will be created
		trafficPolicySpec, err := d.rawMessageToTrafficPolicy(parentEndpoint.Spec.TrafficPolicy.Policy)
		if err != nil {
			d.log.Error(err, "error processing traffic policy from generated CloudEndpoint",
				"ingress to endpoint conversion error",
				"hostname", hostname,
			)
			continue
		}

		// Initialize a list of routing rules for the current hostname with the destination AgentEndpoint so
		// that the routing rules can be properly sorted. We want to make sure that
		//   1. We always generate the same ordering of route rules for any given set of ingresses
		//   2. Since traffic policy rules are executed in-order, we need to order them in a way that results in best-match routing
		type TrafficPolicyRoute struct {
			PathMatch     PathMatch
			UpstreamKey   UpstreamKey
			AgentEndpoint *ngrokv1alpha1.AgentEndpoint
		}
		routes := []TrafficPolicyRoute{}

		for pathMatch, upstreamKey := range routeMap {
			agentEndpoint, exists := childEndpointCache[upstreamKey]
			if !exists {
				d.log.Error(fmt.Errorf("an AgentEndpoint should have been created for ingress path rule but could not be found. this is a bug and not a result of the supplied ingress configuration"),
					"ingress to endpoint conversion error",
					"hostname", hostname,
					"path", pathMatch.Path,
					"path match type", pathMatch.PathMatchType,
				)
				continue
			}
			routes = append(routes, TrafficPolicyRoute{
				PathMatch:     pathMatch,
				UpstreamKey:   upstreamKey,
				AgentEndpoint: agentEndpoint,
			})
		}

		// Sort the routes
		sort.SliceStable(routes, func(i, j int) bool {
			// Exact matches before prefix matches
			if routes[i].PathMatch.PathMatchType != routes[j].PathMatch.PathMatchType {
				return routes[i].PathMatch.PathMatchType == netv1.PathTypeExact
			}

			// Then, longer paths before shorter paths
			if len(routes[i].PathMatch.Path) != len(routes[j].PathMatch.Path) {
				return len(routes[i].PathMatch.Path) > len(routes[j].PathMatch.Path)
			}

			// Finally, lexicographical order
			return routes[i].PathMatch.Path < routes[j].PathMatch.Path
		})

		// Generate traffic policy rules for routing
		for _, route := range routes {

			policyRule := map[string]interface{}{
				"name": fmt.Sprintf("Generated-Route-%s", route.PathMatch.Path),
				"actions": []map[string]interface{}{
					{
						"type": "forward-internal",
						"config": map[string]interface{}{
							"binding": "internal",
							"url":     route.AgentEndpoint.Spec.URL,
						},
					},
				},
			}

			switch route.PathMatch.PathMatchType {
			case netv1.PathTypeExact:
				policyRule["expressions"] = []string{fmt.Sprintf("req.url.path == \"%s\"", route.PathMatch.Path)}
			case netv1.PathTypePrefix:
				policyRule["expressions"] = []string{fmt.Sprintf("req.url.path.startsWith(\"%s\")", route.PathMatch.Path)}
			}

			// Append the new policy rule to the "on_http_request" phase if it already exists, otherwise create it
			if existing, ok := trafficPolicySpec["on_http_request"]; ok {
				if existingList, valid := existing.([]interface{}); valid {
					// If it exists and is a []interface{}, append the new rule
					trafficPolicySpec["on_http_request"] = append(existingList, policyRule)
				} else {
					// If it exists but is not the expected type, log an error and reinitialize it
					d.log.Error(fmt.Errorf("unexpected type for on_http_request"),
						"on_http_request is not a []interface{}, reinitializing with policyRule",
						"existing", existing,
					)
					trafficPolicySpec["on_http_request"] = []interface{}{policyRule}
				}
			} else {
				// If it doesn't exist, initialize it with the new rule
				trafficPolicySpec["on_http_request"] = []interface{}{policyRule}
			}
		}

		// Marshal the updated TrafficPolicySpec back to JSON
		updatedPolicyJSON, err := json.Marshal(trafficPolicySpec)
		if err != nil {
			d.log.Error(err, "failed to marshal updated traffic policy for CloudEndpoint generated from Ingress hostname",
				"ingress to endpoint conversion error",
				"hostname", hostname,
			)
			continue
		}

		parentEndpoint.Spec.TrafficPolicy = &ngrokv1alpha1.NgrokTrafficPolicySpec{
			Policy: json.RawMessage(updatedPolicyJSON),
		}
	}

	// Return all of the generated CloudEndpoints and AgentEndpoints as maps keyed by namespaced name for easier lookup
	childEndpoints := make(map[types.NamespacedName]*ngrokv1alpha1.AgentEndpoint)
	for _, childEndpoint := range childEndpointCache {
		childEndpoints[types.NamespacedName{
			Name:      childEndpoint.Name,
			Namespace: childEndpoint.Namespace,
		}] = childEndpoint
	}

	parentEndpoints := make(map[types.NamespacedName]*ngrokv1alpha1.CloudEndpoint)
	for _, parentEndpoint := range parentEndpointCache {
		parentEndpoints[types.NamespacedName{
			Name:      parentEndpoint.Name,
			Namespace: parentEndpoint.Namespace,
		}] = parentEndpoint
	}

	return parentEndpoints, childEndpoints
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
func buildInternalAgentEndpoint(serviceName, namespace, clusterDomain string, port int32, labels map[string]string, metadata string) *ngrokv1alpha1.AgentEndpoint {
	return &ngrokv1alpha1.AgentEndpoint{
		ObjectMeta: metav1.ObjectMeta{
			Name:      internalAgentEndpointName(serviceName, namespace, clusterDomain, port),
			Namespace: namespace,
			Labels:    labels,
		},
		Spec: ngrokv1alpha1.AgentEndpointSpec{
			URL:      internalAgentEndpointURL(serviceName, namespace, clusterDomain, port),
			Metadata: metadata,
			Upstream: ngrokv1alpha1.EndpointUpstream{
				URL: internalAgentEndpointUpstreamURL(serviceName, namespace, clusterDomain, port),
			},
		},
	}
}

// rawMessageToTrafficPolicy will retrieve existing TrafficPolicySpec from json.RawMessage or create a new empty one
func (d *Driver) rawMessageToTrafficPolicy(existingPolicy json.RawMessage) (map[string]interface{}, error) {
	var policySpec map[string]interface{}
	if len(existingPolicy) > 0 {
		if err := json.Unmarshal(existingPolicy, &policySpec); err != nil {
			return nil, fmt.Errorf("failed to unmarshal existing traffic policy: %v", err)
		}
	}
	if policySpec == nil {
		// Ensure policySpec is always initialized
		policySpec = make(map[string]interface{})
	}
	return policySpec, nil
}
