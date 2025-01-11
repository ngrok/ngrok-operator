package common

import "strings"

type ApplicationProtocol string

const (
	ApplicationProtocol_HTTP1 ApplicationProtocol = "http1"
	ApplicationProtocol_HTTP2 ApplicationProtocol = "http2"
)

func (t ApplicationProtocol) IsKnown() bool {
	switch t {
	case ApplicationProtocol_HTTP1, ApplicationProtocol_HTTP2:
		return true
	default:
		return false
	}
}

type ProxyProtocolVersion string

const (
	ProxyProtocolVersion_1 ProxyProtocolVersion = "1"
	ProxyProtocolVersion_2 ProxyProtocolVersion = "2"
)

func (t ProxyProtocolVersion) IsKnown() bool {
	switch t {
	case ProxyProtocolVersion_1, ProxyProtocolVersion_2:
		return true
	default:
		return false
	}
}

const (
	DefaultClusterDomain = "svc.cluster.local"

	// When this annotation is present on an Ingress/Gateway resource and set to "true", that Ingress/Gateway
	// will cause an endpoint to be created instead of an edge
	AnnotationUseEndpoints = "k8s.ngrok.com/use-endpoints"
)

// hasUseEndpointsAnnotation checks whether or not a set of annotations has the correct annotation for configuring an
// ingress/gateway to use endpoints instead of edges
func HasUseEndpointsAnnotation(annotations map[string]string) bool {
	if val, exists := annotations[AnnotationUseEndpoints]; exists && strings.ToLower(val) == "true" {
		return true
	}
	return false
}
