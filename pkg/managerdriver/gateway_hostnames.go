package managerdriver

import (
	"strings"

	"k8s.io/apimachinery/pkg/types"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"
)

// GatewayHostnames decides which hostnames a Gateway's endpoints use.
//
// ngrok routes traffic by hostname, so every HTTP, HTTPS and TLS endpoint the
// operator creates needs one. A listener's hostname is optional in the Gateway
// API, where an empty one means "every hostname". The endpoints for such a
// listener take their hostnames from the routes attached to it, and when a
// route has none either, from the domain ngrok assigned the Gateway (see
// gateway_address.go).
type GatewayHostnames struct {
	// Assigned holds the domain ngrok assigned each Gateway, by Gateway UID.
	// It is read from the cluster at the start of every sync.
	Assigned map[types.UID]string

	// ConformanceSuffix is appended to every hostname read from a Gateway or
	// route. It exists only so the Gateway API conformance suite, whose
	// hostnames (example.com) belong to other ngrok accounts, can run against
	// domains this account can reserve. It is never set in production.
	ConformanceSuffix string
}

// Map returns the hostname an endpoint uses for a hostname written in a
// Gateway API resource. Only ConformanceSuffix changes it.
func (h GatewayHostnames) Map(hostname string) string {
	if h.ConformanceSuffix == "" || hostname == "" {
		return hostname
	}
	return hostname + "." + h.ConformanceSuffix
}

// Listener returns a listener's mapped hostname, or "" when the listener
// accepts every hostname.
func (h GatewayHostnames) Listener(l gatewayv1.Listener) string {
	if l.Hostname == nil {
		return ""
	}
	return h.Map(string(*l.Hostname))
}

// intersectHostnames returns the hostnames that both a listener hostname and
// a route hostname match, following the Gateway API rules: "" matches
// everything, and "*.example.com" matches any name ending in ".example.com"
// (at any depth) but not "example.com" itself.
//
// The result is the more specific of the two, or ok=false when they share
// no hostname. Two empty inputs give "", ok=true.
func intersectHostnames(listener, route string) (string, bool) {
	switch {
	case listener == "":
		return route, true
	case route == "":
		return listener, true
	case listener == route:
		return listener, true
	}

	lWild, rWild := strings.HasPrefix(listener, "*."), strings.HasPrefix(route, "*.")
	switch {
	case lWild && !rWild:
		if strings.HasSuffix(route, listener[1:]) {
			return route, true
		}
	case !lWild && rWild:
		if strings.HasSuffix(listener, route[1:]) {
			return listener, true
		}
	case lWild && rWild:
		// The longer wildcard is inside the shorter one, if either is.
		if strings.HasSuffix(route, listener[1:]) {
			return route, true
		}
		if strings.HasSuffix(listener, route[1:]) {
			return listener, true
		}
	}
	return "", false
}

// hostnameCovers reports whether a request for host is matched by pattern,
// where pattern may be "" (everything) or a "*." wildcard.
func hostnameCovers(pattern, host string) bool {
	if pattern == "" || pattern == host {
		return true
	}
	if !strings.HasPrefix(pattern, "*.") {
		return false
	}
	// A wildcard covers names below it; the more specific wildcard
	// *.foo.example.com is also covered by *.example.com.
	return strings.HasSuffix(host, pattern[1:])
}
