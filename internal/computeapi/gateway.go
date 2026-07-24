package computeapi

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"strings"
)

// Gateway exposes the Kubernetes API through the identity and permissions of
// its ServiceAccount. Authentication at the public side is performed by the
// mTLS AgentEndpoint; Kubernetes RBAC is the authorization layer.
type Gateway struct {
	TokenFile string
	Proxy     *httputil.ReverseProxy
}

func NewGateway(upstream, tokenFile, caFile string) (*Gateway, error) {
	target, err := url.Parse(upstream)
	if err != nil {
		return nil, fmt.Errorf("parse Kubernetes API URL: %w", err)
	}
	ca, err := os.ReadFile(caFile)
	if err != nil {
		return nil, fmt.Errorf("read Kubernetes CA: %w", err)
	}
	roots := x509.NewCertPool()
	if !roots.AppendCertsFromPEM(ca) {
		return nil, fmt.Errorf("Kubernetes CA contains no certificates")
	}
	proxy := httputil.NewSingleHostReverseProxy(target)
	proxy.Transport = &http.Transport{TLSClientConfig: tlsConfig(roots)}
	proxy.FlushInterval = -1 // flush watch events immediately
	return &Gateway{TokenFile: tokenFile, Proxy: proxy}, nil
}

func tlsConfig(roots *x509.CertPool) *tls.Config {
	return &tls.Config{MinVersion: tls.VersionTLS12, RootCAs: roots}
}

func (g *Gateway) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	token, err := os.ReadFile(g.TokenFile)
	if err != nil {
		http.Error(w, "gateway credentials unavailable", http.StatusServiceUnavailable)
		return
	}
	r.Header.Del("Authorization")
	r.Header.Set("Authorization", "Bearer "+strings.TrimSpace(string(token)))
	g.Proxy.ServeHTTP(w, r)
}
