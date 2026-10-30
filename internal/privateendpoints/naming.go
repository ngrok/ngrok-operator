// Package privateendpoints holds the naming and URL rules shared by the
// PrivateEndpoint poller, controller, and forwarder.
package privateendpoints

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/url"
	"strconv"
	"strings"

	"k8s.io/apimachinery/pkg/util/validation"

	ngrokv1 "github.com/ngrok/ngrok-operator/api/ngrok/v1"
)

const (
	ManagedByLabel     = "ngrok.com/managed-by"
	ManagedByValue     = "private-endpoint-poller"
	ForwarderComponent = "private-endpoint-forwarder"
	serviceSuffix      = "-ngrok-direct"
)

var defaultPorts = map[ngrokv1.PrivateEndpointScheme]int32{
	ngrokv1.PrivateEndpointSchemeHTTP:  80,
	ngrokv1.PrivateEndpointSchemeHTTPS: 443,
	ngrokv1.PrivateEndpointSchemeTLS:   443,
}

// ParseURL turns an endpoint URL into a PrivateEndpointSpec.
func ParseURL(raw string) (ngrokv1.PrivateEndpointSpec, error) {
	u, err := url.Parse(raw)
	if err != nil {
		return ngrokv1.PrivateEndpointSpec{}, fmt.Errorf("parsing endpoint url %q: %w", raw, err)
	}
	scheme := ngrokv1.PrivateEndpointScheme(strings.ToLower(u.Scheme))
	switch scheme {
	case ngrokv1.PrivateEndpointSchemeHTTP, ngrokv1.PrivateEndpointSchemeHTTPS, ngrokv1.PrivateEndpointSchemeTLS, ngrokv1.PrivateEndpointSchemeTCP:
	default:
		return ngrokv1.PrivateEndpointSpec{}, fmt.Errorf("endpoint url %q: unsupported scheme %q", raw, u.Scheme)
	}
	host := NormalizeHost(u.Hostname())
	if host == "" {
		return ngrokv1.PrivateEndpointSpec{}, fmt.Errorf("endpoint url %q: missing hostname", raw)
	}
	port, ok := defaultPorts[scheme]
	if p := u.Port(); p != "" {
		n, err := strconv.ParseUint(p, 10, 16)
		if err != nil || n == 0 {
			return ngrokv1.PrivateEndpointSpec{}, fmt.Errorf("endpoint url %q: invalid port %q", raw, p)
		}
		port, ok = int32(n), true
	}
	if !ok {
		return ngrokv1.PrivateEndpointSpec{}, fmt.Errorf("endpoint url %q: %s requires a port", raw, scheme)
	}
	return ngrokv1.PrivateEndpointSpec{URL: raw, Scheme: scheme, Hostname: host, Port: port}, nil
}

// privateTLD is the only TLD whose private endpoints are mirrored. *.internal
// is not supported: its names collide with cloud provider DNS and with the
// operator's own Ingress/Gateway AgentEndpoints.
const privateTLD = ".ngrok.direct"

// IsPrivateHostname reports whether h is under privateTLD.
func IsPrivateHostname(h string) bool {
	return strings.HasSuffix(NormalizeHost(h), privateTLD)
}

// ServiceName returns the in-cluster Service name that CoreDNS rewrites
// hostname to: foo.ngrok.direct becomes foo-ngrok-direct. Only single-label
// hostnames are supported, because Service names can't contain dots. Keep in
// sync with the CoreDNS rewrite rule in scripts/kind-private-endpoints-dns.sh.
func ServiceName(hostname string) (string, error) {
	label, ok := strings.CutSuffix(NormalizeHost(hostname), privateTLD)
	if !ok {
		return "", fmt.Errorf("hostname %q is not under %s", hostname, privateTLD)
	}
	name := label + serviceSuffix
	// name alone isn't enough: a label ending in '-' (foo-) still makes a
	// valid name (foo--ngrok-direct), so the label is checked too.
	if len(validation.IsDNS1035Label(label)) > 0 || len(validation.IsDNS1035Label(name)) > 0 {
		return "", fmt.Errorf("hostname %q can't be mapped to a Service name: only single-label names made of lowercase letters, digits and '-', starting with a letter, are supported", hostname)
	}
	return name, nil
}

func NormalizeHost(name string) string {
	return strings.TrimSuffix(strings.ToLower(name), ".")
}

func shortHash(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])[:16]
}

// CRName names the PrivateEndpoint for spec as <hostname>-<scheme>-<port>,
// e.g. foo.ngrok.direct-https-443, so it reads like the URL it mirrors. URLs
// that differ only in case or an explicit default port share a name. A
// hostname that can't form a valid object name falls back to a hash.
func CRName(spec ngrokv1.PrivateEndpointSpec) string {
	name := fmt.Sprintf("%s-%s-%d", spec.Hostname, spec.Scheme, spec.Port)
	if len(validation.IsDNS1123Subdomain(name)) > 0 {
		return "pe-" + shortHash(name)
	}
	return name
}

// HostnameForService reverses ServiceName: foo-ngrok-direct is
// foo.ngrok.direct.
func HostnameForService(name string) (string, bool) {
	label, ok := strings.CutSuffix(name, serviceSuffix)
	if !ok || label == "" {
		return "", false
	}
	return label + privateTLD, true
}
