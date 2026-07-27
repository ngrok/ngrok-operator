package computeapi

import (
	"crypto/sha256"
	"encoding/base64"
	"io"
	"net/http"
	"net/http/httptest"
	"net/http/httputil"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestGatewayBearerAuthentication(t *testing.T) {
	dir := t.TempDir()
	tokenFile := filepath.Join(dir, "service-account-token")
	hashFile := filepath.Join(dir, "access-key-sha256")
	require.NoError(t, os.WriteFile(tokenFile, []byte("kubernetes-token"), 0o600))
	writeAccessKeyHash(t, hashFile, "compute-key")

	var upstreamAuthorization string
	proxy := &httputil.ReverseProxy{
		Director: func(_ *http.Request) {},
		Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
			upstreamAuthorization = r.Header.Get("Authorization")
			return &http.Response{
				StatusCode: http.StatusNoContent,
				Header:     make(http.Header),
				Body:       io.NopCloser(strings.NewReader("")),
			}, nil
		}),
	}

	gateway := &Gateway{
		TokenFile:     tokenFile,
		TokenHashFile: hashFile,
		Proxy:         proxy,
	}

	for _, tc := range []struct {
		name          string
		authorization string
		wantStatus    int
	}{
		{name: "valid", authorization: "Bearer compute-key", wantStatus: http.StatusNoContent},
		{name: "wrong key", authorization: "Bearer wrong", wantStatus: http.StatusUnauthorized},
		{name: "missing", wantStatus: http.StatusUnauthorized},
		{name: "wrong scheme", authorization: "Basic compute-key", wantStatus: http.StatusUnauthorized},
	} {
		t.Run(tc.name, func(t *testing.T) {
			upstreamAuthorization = ""
			req := httptest.NewRequest(http.MethodGet, "https://compute.internal/version", nil)
			req.Header.Set("Authorization", tc.authorization)
			resp := httptest.NewRecorder()
			gateway.ServeHTTP(resp, req)
			require.Equal(t, tc.wantStatus, resp.Code)
			if tc.wantStatus == http.StatusNoContent {
				require.Equal(t, "Bearer kubernetes-token", upstreamAuthorization)
			} else {
				require.Empty(t, upstreamAuthorization)
				require.Equal(t, "Bearer", resp.Header().Get("WWW-Authenticate"))
			}
		})
	}
}

func TestGatewayReadsRotatedAccessKeyHash(t *testing.T) {
	hashFile := filepath.Join(t.TempDir(), "access-key-sha256")
	writeAccessKeyHash(t, hashFile, "first")
	gateway := &Gateway{TokenHashFile: hashFile}
	require.True(t, gateway.authorized("Bearer first"))

	writeAccessKeyHash(t, hashFile, "second")
	require.False(t, gateway.authorized("Bearer first"))
	require.True(t, gateway.authorized("Bearer second"))
}

func TestGatewayRejectsUnreadableOrMalformedVerifier(t *testing.T) {
	gateway := &Gateway{TokenHashFile: filepath.Join(t.TempDir(), "missing")}
	require.False(t, gateway.authorized("Bearer key"))

	require.NoError(t, os.WriteFile(gateway.TokenHashFile, []byte("not-base64"), 0o600))
	require.False(t, gateway.authorized("Bearer key"))
}

func writeAccessKeyHash(t *testing.T, path, accessKey string) {
	t.Helper()
	hash := sha256.Sum256([]byte(accessKey))
	require.NoError(t, os.WriteFile(path, []byte(base64.RawURLEncoding.EncodeToString(hash[:])), 0o600))
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) {
	return f(r)
}
