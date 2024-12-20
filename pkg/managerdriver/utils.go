package managerdriver

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"regexp"
	"strings"
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
func internalAgentEndpointName(upstreamService, upstreamNamespace, clusterDomain string, upstreamPort int32) string {
	return sanitizeStringForK8sName(fmt.Sprintf("i-%s-%s-%s-%d",
		upstreamService,
		upstreamNamespace,
		clusterDomain,
		upstreamPort,
	))
}

// internalAgentEndpointURL builds a URL string for an internal endpoint
func internalAgentEndpointURL(serviceName, namespace, clusterDomain string, port int32) string {
	ret := fmt.Sprintf("https://%s-%s-%s-%d",
		sanitizeStringForURL(serviceName),
		sanitizeStringForURL(namespace),
		sanitizeStringForURL(clusterDomain),
		port,
	)

	// Even though . is a valid character, trim them so we don't hit the
	// limit on subdomains for endpoint URLs.
	ret = strings.ReplaceAll(ret, ".", "-")

	ret += ".internal"
	return ret
}

// internalAgentEndpointUpstreamURL builds a URL string for an internal AgentEndpoint's upstream url
func internalAgentEndpointUpstreamURL(serviceName, namespace, clusterDomain string, port int32) string {
	return fmt.Sprintf("http://%s.%s.%s:%d",
		sanitizeStringForURL(serviceName),
		sanitizeStringForURL(namespace),
		sanitizeStringForURL(clusterDomain),
		port,
	)
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

// appendToLabel appends a value to a comma-separated label.
func appendToLabel(labels map[string]string, key, value string) {
	if existing, exists := labels[key]; exists {
		labels[key] = existing + "," + value
	} else {
		labels[key] = value
	}
}
