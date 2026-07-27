package computeapi

import (
	"crypto/sha256"
	"crypto/subtle"
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"fmt"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"strings"
)

// Gateway exposes the Kubernetes API through the identity and permissions of
// its ServiceAccount. Compute authenticates with an ephemeral bearer key, and
// Kubernetes RBAC is the authorization layer.
type Gateway struct {
	TokenFile     string
	TokenHashFile string
	Proxy         *httputil.ReverseProxy
}

func NewGateway(upstream, tokenFile, tokenHashFile, caFile string) (*Gateway, error) {
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
	return &Gateway{TokenFile: tokenFile, TokenHashFile: tokenHashFile, Proxy: proxy}, nil
}

func tlsConfig(roots *x509.CertPool) *tls.Config {
	return &tls.Config{MinVersion: tls.VersionTLS12, RootCAs: roots}
}

func (g *Gateway) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if !g.authorized(r.Header.Get("Authorization")) {
		w.Header().Set("WWW-Authenticate", "Bearer")
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	token, err := os.ReadFile(g.TokenFile)
	if err != nil {
		http.Error(w, "gateway credentials unavailable", http.StatusServiceUnavailable)
		return
	}
	r.Header.Del("Authorization")
	r.Header.Set("Authorization", "Bearer "+strings.TrimSpace(string(token)))
	g.Proxy.ServeHTTP(w, r)
}

func (g *Gateway) authorized(authorization string) bool {
	const bearerPrefix = "Bearer "
	if !strings.HasPrefix(authorization, bearerPrefix) {
		return false
	}
	expectedHashText, err := os.ReadFile(g.TokenHashFile)
	if err != nil {
		return false
	}
	expectedHash, err := base64.RawURLEncoding.DecodeString(strings.TrimSpace(string(expectedHashText)))
	if err != nil || len(expectedHash) != sha256.Size {
		return false
	}
	actualHash := sha256.Sum256([]byte(strings.TrimPrefix(authorization, bearerPrefix)))
	return subtle.ConstantTimeCompare(actualHash[:], expectedHash) == 1
}
