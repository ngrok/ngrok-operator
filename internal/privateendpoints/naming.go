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

	ngrokv1 "github.com/ngrok/ngrok-operator/api/ngrok/v1"
)

const (
	ManagedByLabel     = "ngrok.com/managed-by"
	ManagedByValue     = "private-endpoint-poller"
	HostLabel          = "ngrok.com/private-endpoint-host"
	ForwarderComponent = "private-endpoint-forwarder"
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

// IsPrivateHostname reports whether h is under a private endpoint TLD.
func IsPrivateHostname(h string) bool {
	h = NormalizeHost(h)
	return strings.HasSuffix(h, ".internal") || strings.HasSuffix(h, ".ngrok.direct")
}

// SharedEligible reports whether the endpoint can be served by the shared
// listener, which demuxes by Host header on 80 and by SNI on 443.
func SharedEligible(s ngrokv1.PrivateEndpointSpec) bool {
	switch s.Scheme {
	case ngrokv1.PrivateEndpointSchemeHTTP:
		return s.Port == 80
	case ngrokv1.PrivateEndpointSchemeHTTPS, ngrokv1.PrivateEndpointSchemeTLS:
		return s.Port == 443
	}
	return false
}

func NormalizeHost(name string) string {
	return strings.TrimSuffix(strings.ToLower(name), ".")
}

func shortHash(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])[:16]
}

func CRName(url string) string { return "pe-" + shortHash(url) }

func HostKey(hostname string) string { return shortHash(NormalizeHost(hostname)) }

func HostServiceName(hostKey string) string { return "pe-host-" + hostKey }
