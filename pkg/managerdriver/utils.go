package managerdriver

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/go-logr/logr"
	"github.com/gobwas/glob"
	common "github.com/ngrok/ngrok-operator/api/common/v1alpha1"
	ingressv1alpha1 "github.com/ngrok/ngrok-operator/api/ingress/v1alpha1"
	ngrokv1alpha1 "github.com/ngrok/ngrok-operator/api/ngrok/v1alpha1"
	"github.com/ngrok/ngrok-operator/internal/annotations"
	"github.com/ngrok/ngrok-operator/internal/errors"
	"github.com/ngrok/ngrok-operator/internal/ir"
	"github.com/ngrok/ngrok-operator/internal/store"
	"github.com/ngrok/ngrok-operator/internal/util"
	corev1 "k8s.io/api/core/v1"
	netv1 "k8s.io/api/networking/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"
)

// hasDefaultManagedResourceLabels takes input labels and the manager name/namespace to see if the label map contains
// the labels that indicate that the resource the labels are on is managed or user-created
func hasDefaultManagedResourceLabels(labels map[string]string, managerName, managerNamespace string) bool {
	val, exists := labels[labelControllerNamespace]
	if !exists || val != managerNamespace {
		return false
	}

	val, exists = labels[labelControllerName]
	if !exists || val != managerName {
		return false
	}

	return true
}

// internalAgentEndpointName builds a string for the name of an internal AgentEndpoint
func internalAgentEndpointName(serviceUID, serviceName, namespace, clusterDomain string, upstreamPort int32, clientCertRefs []ir.IRObjectRef) string {
	uidHash := sha256.Sum256([]byte(serviceUID))
	hashHex := hex.EncodeToString(uidHash[:])

	// When using upstream certs, add a tls hash to the name so that the name for the upstream service is unique.
	// You may have something like two different gateways that have different client certs for the same upstream service.
	// This is an unlikely but valid use-case
	tlsSuffix := ""
	if len(clientCertRefs) > 0 {
		tlsStr := ""
		for _, certRef := range clientCertRefs {
			tlsStr += fmt.Sprintf("%s.%s", certRef.Name, certRef.Namespace)
		}

		tlsHash := sha256.Sum256([]byte(tlsStr))
		tlsHashHex := hex.EncodeToString(tlsHash[:])
		tlsSuffix = fmt.Sprintf("tls-%s", tlsHashHex[:5])
	}

	ret := fmt.Sprintf("%s-%s-%s",
		hashHex[:5],
		serviceName,
		namespace,
	)
	if tlsSuffix != "" {
		ret += fmt.Sprintf("-%s",
			tlsSuffix,
		)
	}

	// Unless we are using a custom cluster domain, leave it out of any generated stuff to keep names more readable
	if clusterDomain != common.DefaultClusterDomain && clusterDomain != "" {
		ret += fmt.Sprintf("-%s", clusterDomain)
	}

	ret += fmt.Sprintf("-%d", upstreamPort)
	return sanitizeStringForK8sName(ret)
}

// internalAgentEndpointURL builds a URL string for an internal endpoint
func internalAgentEndpointURL(serviceUID, serviceName, namespace, clusterDomain string, port int32, clientCertRefs []ir.IRObjectRef) string {
	uidHash := sha256.Sum256([]byte(serviceUID))
	hashHex := hex.EncodeToString(uidHash[:])

	ret := fmt.Sprintf("https://%s-%s-%s",
		sanitizeStringForURL(hashHex[:5]),
		sanitizeStringForURL(serviceName),
		sanitizeStringForURL(namespace),
	)

	tlsSuffix := ""
	if len(clientCertRefs) > 0 {
		tlsStr := ""
		for _, certRef := range clientCertRefs {
			tlsStr += fmt.Sprintf("%s.%s", certRef.Name, certRef.Namespace)
		}

		tlsHash := sha256.Sum256([]byte(tlsStr))
		tlsHashHex := hex.EncodeToString(tlsHash[:])
		tlsSuffix = fmt.Sprintf("tls-%s", tlsHashHex[:5])
	}

	if tlsSuffix != "" {
		ret += fmt.Sprintf("-%s",
			tlsSuffix,
		)
	}

	// Unless we are using a custom cluster domain, leave it out of any generated stuff to keep names more readable
	if clusterDomain != common.DefaultClusterDomain && clusterDomain != "" {
		ret += fmt.Sprintf("-%s", sanitizeStringForURL(clusterDomain))
	}

	ret += fmt.Sprintf("-%d", port)

	// Even though . is a valid character, trim them so we don't hit the
	// limit on subdomains for endpoint URLs.
	ret = strings.ReplaceAll(ret, ".", "-")

	ret += ".internal"
	return ret
}

// internalAgentEndpointUpstreamURL builds a URL string for an internal AgentEndpoint's upstream url
func internalAgentEndpointUpstreamURL(serviceName, namespace, clusterDomain string, port int32) string {
	ret := fmt.Sprintf("http://%s.%s",
		sanitizeStringForURL(serviceName),
		sanitizeStringForURL(namespace),
	)

	// Unless we are using a custom cluster domain, leave it out of any generated stuff to keep names more readable
	if clusterDomain != common.DefaultClusterDomain && clusterDomain != "" {
		ret += fmt.Sprintf("-%s", clusterDomain)
	}

	ret += fmt.Sprintf(":%d", port)
	return ret
}

// Takes an input string and sanitizes any characters not valid for part of a Kubernetes resource name
func sanitizeStringForK8sName(s string) string {
	// Replace '*' with 'wildcard'
	s = strings.ReplaceAll(s, "*", "wildcard")

	// Convert to lowercase
	s = strings.ToLower(s)

	// Replace all invalid characters with '-'
	invalidChars := regexp.MustCompile(`[^a-z0-9.-]+`)
	s = invalidChars.ReplaceAllString(s, "-")

	// Trim leading invalid characters
	leadingInvalid := regexp.MustCompile(`^[^a-z0-9]+`)
	s = leadingInvalid.ReplaceAllString(s, "")

	// Trim trailing invalid characters
	trailingInvalid := regexp.MustCompile(`[^a-z0-9]+$`)
	s = trailingInvalid.ReplaceAllString(s, "")

	// If empty, default to "default"
	if s == "" {
		s = "default"
	}

	// Enforce max length
	if len(s) > 63 {
		hashBytes := sha256.Sum256([]byte(s))
		hash := hex.EncodeToString(hashBytes[:])[:8]
		truncateLength := 63 - len(hash) - 1
		if truncateLength > 0 {
			s = s[:truncateLength] + "-" + hash
		} else {
			s = hash
		}
	}

	return s
}

// Takes an input string and sanitized any characters not valid for part of a URL
func sanitizeStringForURL(s string) string {
	// Replace '*' with 'wildcard'
	s = strings.ReplaceAll(s, "*", "wildcard")

	// Replace invalid chars with '-'
	invalidURLChars := regexp.MustCompile(`[^a-zA-Z0-9._~-]`)
	s = invalidURLChars.ReplaceAllString(s, "-")

	return s
}

func getPortAppProtocol(service *corev1.Service, port *corev1.ServicePort) (string, error) {
	if port.AppProtocol == nil {
		return "", nil
	}

	switch proto := *port.AppProtocol; proto {
	case "k8s.ngrok.com/http2", "kubernetes.io/h2c":
		return "http2", nil
	case "":
		return "", nil
	default:
		return "", fmt.Errorf("unsupported appProtocol: '%s', must be 'k8s.ngrok.com/http2', 'kubernetes.io/h2c' or ''. From: %s service: %s", proto, service.Namespace, service.Name)
	}
}

// Generates a labels map for matching ngrok Routes to Agent Tunnels
func ngrokLabels(namespace, serviceUID, serviceName string, port int32) map[string]string {
	return map[string]string{
		labelNamespace:  namespace,
		labelServiceUID: serviceUID,
		labelService:    serviceName,
		labelPort:       strconv.Itoa(int(port)),
	}
}

func getPortAnnotatedProtocol(log logr.Logger, service *corev1.Service, portName string) (string, error) {
	if service.Annotations != nil {
		annotation := service.Annotations["k8s.ngrok.com/app-protocols"]
		if annotation != "" {
			log.V(3).Info("Annotated app-protocols found", "annotation", annotation, "namespace", service.Namespace, "service", service.Name, "portName", portName)
			m := map[string]string{}
			err := json.Unmarshal([]byte(annotation), &m)
			if err != nil {
				return "", fmt.Errorf("could not parse protocol annotation: '%s' from: %s service: %s", annotation, service.Namespace, service.Name)
			}

			if protocol, ok := m[portName]; ok {
				log.V(3).Info("Found protocol for port name", "protocol", protocol, "namespace", service.Namespace, "service", service.Name)
				// only allow cases through where we are sure of intent
				switch upperProto := strings.ToUpper(protocol); upperProto {
				case "HTTP", "HTTPS":
					return upperProto, nil
				default:
					return "", fmt.Errorf("unhandled protocol annotation: '%s', must be 'HTTP' or 'HTTPS'. From: %s service: %s", upperProto, service.Namespace, service.Name)
				}
			}
		}
	}
	return "HTTP", nil
}

func findServicesPort(log logr.Logger, service *corev1.Service, backendSvcPort netv1.ServiceBackendPort) (*corev1.ServicePort, error) {
	for _, port := range service.Spec.Ports {
		if (backendSvcPort.Number > 0 && port.Port == backendSvcPort.Number) || port.Name == backendSvcPort.Name {
			log.V(3).Info("Found matching port for service", "namespace", service.Namespace, "service", service.Name, "port.name", port.Name, "port.number", port.Port)
			return &port, nil
		}
	}
	return nil, fmt.Errorf("could not find matching port for service %s, backend port %v, name %s", service.Name, backendSvcPort.Number, backendSvcPort.Name)
}

func findBackendRefServicesPort(log logr.Logger, service *corev1.Service, backendRef *gatewayv1.BackendRef) (*corev1.ServicePort, error) {
	for _, port := range service.Spec.Ports {
		if (int32(*backendRef.Port) > 0 && port.Port == int32(*backendRef.Port)) || port.Name == string(backendRef.Name) {
			log.V(3).Info("Found matching port for service", "namespace", service.Namespace, "service", service.Name, "port.name", port.Name, "port.number", port.Port)
			return &port, nil
		}
	}
	return nil, fmt.Errorf("could not find matching port for service %s, backend port %v, name %s", service.Name, int32(*backendRef.Port), string(backendRef.Name))
}

func calculateIngressLoadBalancerIPStatus(log logr.Logger, ing *netv1.Ingress, c client.Reader) []netv1.IngressLoadBalancerIngress {
	ingressHosts := map[string]bool{}
	for _, rule := range ing.Spec.Rules {
		ingressHosts[rule.Host] = true
	}

	domains := &ingressv1alpha1.DomainList{}
	if err := c.List(context.Background(), domains); err != nil {
		log.Error(err, "failed to list domains")
		return []netv1.IngressLoadBalancerIngress{}
	}

	domainsByDomain := map[string]ingressv1alpha1.Domain{}
	for _, domain := range domains.Items {
		domainsByDomain[domain.Spec.Domain] = domain
	}

	status := []netv1.IngressLoadBalancerIngress{}

	for host := range ingressHosts {
		d, ok := domainsByDomain[host]
		if !ok {
			continue
		}

		var hostname string

		switch {
		// Custom domain
		case d.Status.CNAMETarget != nil:
			hostname = *d.Status.CNAMETarget
		// ngrok managed domain
		default:
			// Trim the wildcard prefix if it exists for ngrok managed domains
			hostname = strings.TrimPrefix(d.Status.Domain, "*.")
		}

		if hostname != "" {
			status = append(status, netv1.IngressLoadBalancerIngress{
				Hostname: hostname,
			})
		}
	}

	return status
}

// extractPolicy parses the policy message into a format such that it can be combined with policy from other filters.
// If the legacy "inbound/outbound" format is detected, inbound remaps to `on_http_request`, outbound remaps to
// `on_http_response`. This is safe so long as HTTP Edges are the only ones supported on the gateway API.
func extractPolicy(jsonMessage json.RawMessage) (util.TrafficPolicy, error) {
	extensionRefTrafficPolicy, err := util.NewTrafficPolicyFromJson(jsonMessage)
	if err != nil {
		return nil, err
	}

	if extensionRefTrafficPolicy.IsLegacyPolicy() {
		extensionRefTrafficPolicy.ConvertLegacyDirectionsToPhases()
	}

	return extensionRefTrafficPolicy, nil
}

func getNgrokTrafficPolicyForIngress(ing *netv1.Ingress, resources store.Storer) (*ngrokv1alpha1.NgrokTrafficPolicy, error) {
	policy, err := annotations.ExtractNgrokTrafficPolicyFromAnnotations(ing)
	if err != nil {
		if errors.IsMissingAnnotations(err) {
			return nil, nil
		}
		return nil, err
	}

	return resources.GetNgrokTrafficPolicyV1(policy, ing.Namespace)
}

// Given an object, it will resolve any ngrok modulesets defined on the object's annotations to the
// CRDs and then will merge them in to a single moduleset
func getNgrokModuleSetForObject(obj client.Object, resources store.Storer) (*ingressv1alpha1.NgrokModuleSet, error) {
	computedModSet := &ingressv1alpha1.NgrokModuleSet{}

	modules, err := annotations.ExtractNgrokModuleSetsFromAnnotations(obj)
	if err != nil {
		if errors.IsMissingAnnotations(err) {
			return computedModSet, nil
		}
		return computedModSet, err
	}

	for _, module := range modules {
		resolvedMod, err := resources.GetNgrokModuleSetV1(module, obj.GetNamespace())
		if err != nil {
			return computedModSet, err
		}
		computedModSet.Merge(resolvedMod)
	}

	return computedModSet, nil
}

// netv1PathTypeToIR validates an ingress
func netv1PathTypeToIR(log logr.Logger, pathType *netv1.PathType) ir.IRPathMatchType {
	if pathType == nil {
		return ir.IRPathType_Prefix
	}

	switch *pathType {
	case netv1.PathTypePrefix, netv1.PathTypeImplementationSpecific:
		return ir.IRPathType_Prefix
	case netv1.PathTypeExact:
		return ir.IRPathType_Exact
	default:
		log.Error(fmt.Errorf("unknown path type, defaulting to prefix match"), "unknown path type", "pathType", *pathType)
		return ir.IRPathType_Prefix
	}
}

// appendStringUnique will append a string to the string slice if it does not already exist
func appendStringUnique(existing []string, newItems ...string) []string {
	uniqueMap := make(map[string]struct{})

	for _, item := range existing {
		uniqueMap[item] = struct{}{}
	}

	for _, newItem := range newItems {
		if _, exists := uniqueMap[newItem]; !exists {
			existing = append(existing, newItem)
		}
	}

	return existing
}

func doHostGlobsMatch(hostname1 string, hostname2 string) (bool, error) {
	hostname1IsGlob := strings.Contains(hostname1, "*")
	hostname2IsGlob := strings.Contains(hostname2, "*")

	switch {
	// If they are both globs, hostname1 wins and hostname2 must match it
	case hostname1IsGlob && hostname2IsGlob:
		fallthrough
	case hostname1IsGlob:
		host1Glob, err := glob.Compile(hostname1)
		if err != nil {
			return false, err
		}
		return host1Glob.Match(hostname2), nil
	case hostname2IsGlob:
		host2Glob, err := glob.Compile(hostname2)
		if err != nil {
			return false, err
		}
		return host2Glob.Match(hostname1), nil
	default:
		return hostname1 == hostname2, nil
	}
}
