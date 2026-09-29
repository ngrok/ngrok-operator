# Private Endpoints in Kubernetes — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Pods in a cluster running the operator reach ngrok private endpoints (`*.ngrok.direct`, `*.internal`) by their real URL, over http/https/tls/tcp, with all egress over private dial.

**Architecture:** api-manager polls `Endpoints.List`, mirrors each private endpoint URL into a `PrivateEndpoint` CR (`ngrok.com/v1`), and a host-keyed controller gives each CR the ClusterIP that DNS should return: the shared forwarder Service for http:80 / https:443 / tls:443 hostnames, or a per-hostname Service otherwise. A new `private-endpoint-forwarder` Deployment reads the CRs and serves DNS for `internal.` / `ngrok.direct.`, a Host/SNI-demuxing shared listener, and per-port listeners, forwarding every connection with `privatedial.Dialer.DialContext`.

**Tech Stack:** Go 1.27, controller-runtime, kubebuilder markers, ngrok-api-go v9, `golang.ngrok.com/ngrok/privatedial`, `github.com/miekg/dns` v1, Helm 4 + helm-unittest, kind.

**Spec:** `specs/private-endpoints/design.md`

## Global Constraints

- CRD group/version: `ngrok.com/v1`, kind `PrivateEndpoint`, namespaced, lives only in the operator namespace (`POD_NAMESPACE`).
- CR name `pe-<first 16 hex of sha256(url)>`; labels `ngrok.com/managed-by: private-endpoint-poller` and `ngrok.com/private-endpoint-host: <first 16 hex of sha256(lowercased hostname)>`.
- Poll interval 10s. API list error ⇒ zero creates, zero deletes that tick.
- Only endpoints whose hostname ends in `.internal` or `.ngrok.direct` and whose `Bindings` does not contain `kubernetes` are mirrored.
- DNS TTL 5s. Unknown `ngrok.direct.` ⇒ NXDOMAIN. Unknown `internal.` ⇒ forwarded upstream. Before first sync ⇒ SERVFAIL.
- Private dial: `privatedial.ProtocolQUIC` forced (H2 transport panics on Go 1.27). Default server `quic.connect-endpoint.ngrok.com:443`. Needs UDP/443 egress.
- Forwarder authenticates with `NGROK_ACCESS_TOKEN` from credentials secret key `PRIVATE_ENDPOINTS_ACCESS_TOKEN` (value `credentials.privateEndpoints.accessToken`, falling back to `credentials.accessToken`).
- Forwarder pod uses `dnsPolicy: Default` so its own upstream DNS never loops back through CoreDNS into itself.
- Forwarder container ports: DNS 5353 (udp+tcp), http 8000, https 8443, per-endpoint 20000–20999. Services map 53→5353, 80→8000, 443→8443.
- Helm gate: `privateEndpoints.enabled` (default `false`). Bindings keep working untouched when both are on.
- Go version stays in `flake.nix` only. No `golang:` base image.
- Tests: table-driven, `testify` for unit tests; envtest via `testutils.OperatorCRDPath` for the controller. `fmt.Errorf("...: %w", err)`.
- Commits on branch `alex/private-endpoints-poc`, message explains why, ends with `Co-Authored-By: Claude Opus 5.5 (1M context) <noreply@anthropic.com>`. Never amend.

## Review Focus

1. A hostname with both a shared-eligible and a non-shared endpoint (`http://foo.internal` + `tcp://foo.internal:6379`): both must be reachable, since DNS returns one IP per name. Pinned in Task 4 (`mixed host goes dedicated`).
2. ngrok API fails mid-list (or returns an error on the first page): no PrivateEndpoints get deleted. Pinned in Task 3 (`list error keeps CRs`).
3. Query names in mixed case or with/without the trailing dot (`FOO.Internal.`), and an unknown `.internal` name like `metadata.google.internal`: must answer / forward, never NXDOMAIN the latter. Pinned in Task 6.
4. A client connects to :80/:443 and sends nothing (health checkers, port scanners): the connection must be closed after the peek timeout, not held forever. Pinned in Task 7 (`idle client is closed`).
5. An endpoint goes away and its forwarder port is later reallocated to a different endpoint: old listener closed, new one routes to the new target. Pinned in Task 7 (`port listeners follow sync`).

## File Structure

```
api/ngrok/v1/privateendpoint_types.go            CRD types
internal/privateendpoints/naming.go              URL parsing, names, labels, shared-eligibility (shared by all components)
internal/privateendpoints/naming_test.go
internal/controller/privateendpoints/poller.go   ngrok API → CRs
internal/controller/privateendpoints/poller_test.go
internal/controller/privateendpoints/controller.go  CRs → Services + status (host-keyed)
internal/controller/privateendpoints/controller_test.go (envtest, TestMain)
internal/privateendpoints/forwarder/table.go     in-memory routing table
internal/privateendpoints/forwarder/dns.go       DNS handler
internal/privateendpoints/forwarder/peek.go      Host / SNI peeking
internal/privateendpoints/forwarder/proxy.go     listeners + byte pipe
internal/privateendpoints/forwarder/syncer.go    CR informer → table + listeners
internal/privateendpoints/forwarder/*_test.go
cmd/private-endpoint-forwarder.go                subcommand
cmd/api-manager.go                               feature flag wiring (modify)
helm/ngrok-operator/templates/private-endpoints/{deployment,rbac,services}.yaml
helm/ngrok-operator/tests/private-endpoints/*_test.yaml
scripts/kind-private-endpoints-dns.sh
tools/make/deploy.mk, tools/make/kind.mk         make targets (modify)
specs/private-endpoints/findings.md              probe + e2e results
```

## Deviations from the spec (recorded in design.md in Task 2)

- `ngrok-api-go` v9 `endpoints.Client.List` takes `*ngrok.Paging`, which has no `Filter`. Filtering is client-side (hostname suffix + not `kubernetes`-bound).
- Controller is keyed by hostname, not by CR, and owns one Service per hostname that needs one. Reason: DNS can return one IP per name, so every endpoint on a hostname must share an IP. A hostname goes "dedicated" if any of its endpoints is not http:80 / https:443 / tls:443; then all its endpoints (including http:80) go through per-port forwarder listeners. No finalizer: Services are deleted when the last CR for the hostname disappears.
- `status.clusterIP` is set for every Ready CR (shared Service IP or the hostname Service IP); `status.forwarderPort` only for dedicated ones. The forwarder needs no Service RBAC.

---

### Task 1: Validate private dial against real endpoints (throwaway probe)

Riskiest assumptions first: what `Endpoints.List` returns for private endpoints, and whether private-dialing an `https://…ngrok.direct` endpoint on 443 yields a TLS session with ngrok's real cert.

**Files:**
- Create (not committed): `_probe/main.go`
- Modify: `go.mod`, `go.sum` (privatedial dependency, committed)
- Create: `specs/private-endpoints/findings.md`

**Interfaces:** Produces: findings that Tasks 3 and 10 rely on (bindings value, URL shapes, https behavior). Adds module `golang.ngrok.com/ngrok/privatedial`.

- [ ] **Step 1: Add dependency**

```bash
go get golang.ngrok.com/ngrok/privatedial@v0.0.0-20260616091145-e2b70148b6de
```

- [ ] **Step 2: Start test endpoints outside the cluster** (in separate terminals, from a machine with the ngrok agent and a PAT-backed account; pick a unique `$U`)

```bash
python3 -m http.server 8080
ngrok http 8080 --url http://pe-probe-$U.internal
ngrok http 8080 --url https://pe-probe-$U.ngrok.direct
```

If `.ngrok.direct` URL syntax is rejected, check `ngrok http --help` / ngrok docs for private-endpoint flags and record the working command in findings.md.

- [ ] **Step 3: Write probe**

```go
package main

import (
	"bufio"
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"net/http"
	"os"
	"time"

	"github.com/ngrok/ngrok-api-go/v9"
	"github.com/ngrok/ngrok-api-go/v9/endpoints"
	"golang.ngrok.com/ngrok/privatedial"
)

func main() {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	token := os.Getenv("NGROK_ACCESS_TOKEN")

	iter := endpoints.NewClient(ngrok.NewClientConfig(token)).List(nil)
	for iter.Next(ctx) {
		ep := iter.Item()
		fmt.Printf("url=%s scheme=%s host=%s port=%d type=%s bindings=%v pooling=%v\n",
			ep.URL, ep.Scheme, ep.Host, ep.Port, ep.Type, ep.Bindings, ep.PoolingEnabled)
	}
	if err := iter.Err(); err != nil {
		fmt.Println("list error:", err)
	}

	d := privatedial.New(privatedial.Config{
		QUICServerAddr: "quic.connect-endpoint.ngrok.com:443",
		ForceProtocol:  privatedial.ProtocolQUIC,
		AuthToken:      token,
	})
	for _, target := range os.Args[1:] { // host:port
		host, port, _ := net.SplitHostPort(target)
		c, err := d.DialContext(ctx, "tcp", target)
		if err != nil {
			fmt.Println(target, "dial error:", err)
			continue
		}
		var conn net.Conn = c
		if port == "443" {
			tc := tls.Client(c, &tls.Config{ServerName: host})
			if err := tc.HandshakeContext(ctx); err != nil {
				fmt.Println(target, "tls error:", err)
				continue
			}
			cert := tc.ConnectionState().PeerCertificates[0]
			fmt.Println(target, "cert subject:", cert.Subject, "issuer:", cert.Issuer, "dns:", cert.DNSNames)
			conn = tc
		}
		fmt.Fprintf(conn, "GET / HTTP/1.1\r\nHost: %s\r\nConnection: close\r\n\r\n", host)
		resp, err := http.ReadResponse(bufio.NewReader(conn), nil)
		if err != nil {
			fmt.Println(target, "http error:", err)
			continue
		}
		fmt.Println(target, "status:", resp.Status)
		conn.Close()
	}
}
```

- [ ] **Step 4: Run**

```bash
go run ./_probe pe-probe-$U.internal:80 pe-probe-$U.ngrok.direct:443
```

Expected: both endpoints listed (note `bindings` value and whether `Host`/`Port` are populated); `:80` prints `status: 200 OK`; `:443` prints a publicly-trusted cert for the `.ngrok.direct` name, then `200 OK`.

- [ ] **Step 5: Record findings, then decide**

Write `specs/private-endpoints/findings.md` with a `## Probe (Task 1)` section: exact agent commands, raw probe output, `bindings` value for each, cert subject/issuer. **If the https dial fails or returns a non-trusted cert, stop and report to the user before Task 2** — the shared :443 SNI passthrough design depends on it.

- [ ] **Step 6: Clean up and commit**

```bash
rm -rf _probe
go mod tidy   # privatedial may be dropped as unused; re-add in Task 8 if so
git add go.mod go.sum specs/private-endpoints/findings.md
git commit -m "Record private dial probe against real private endpoints

The shared :443 listener assumes ngrok terminates https endpoints with
their real cert over private dial; this checks that before building on it.

Co-Authored-By: Claude Opus 5.5 (1M context) <noreply@anthropic.com>"
```

---

### Task 2: PrivateEndpoint CRD and naming helpers

**Files:**
- Create: `api/ngrok/v1/privateendpoint_types.go`
- Modify: `api/ngrok/v1/groupversion_info.go` (register types)
- Create: `internal/privateendpoints/naming.go`, `internal/privateendpoints/naming_test.go`
- Generated: `api/ngrok/v1/zz_generated.deepcopy.go`, `helm/ngrok-crds/templates/ngrok.com_privateendpoints.yaml`
- Modify: `specs/private-endpoints/design.md` (add "Deviations" section, copied from this plan)

**Interfaces:**
- Produces (package `ngrokv1`): `PrivateEndpoint`, `PrivateEndpointList`, `PrivateEndpointSpec{URL string; Scheme PrivateEndpointScheme; Hostname string; Port int32}`, `PrivateEndpointStatus{ObservedGeneration int64; ClusterIP string; ForwarderPort int32; Conditions []metav1.Condition}`, consts `PrivateEndpointSchemeHTTP/HTTPS/TLS/TCP`, `PrivateEndpointConditionReady = "Ready"`.
- Produces (package `privateendpoints`, import path `github.com/ngrok/ngrok-operator/internal/privateendpoints`): consts `ManagedByLabel`, `ManagedByValue`, `HostLabel`, `ForwarderComponent = "private-endpoint-forwarder"`; funcs `ParseURL(raw string) (ngrokv1.PrivateEndpointSpec, error)`, `IsPrivateHostname(h string) bool`, `SharedEligible(s ngrokv1.PrivateEndpointSpec) bool`, `CRName(url string) string`, `HostKey(hostname string) string`, `HostServiceName(hostKey string) string`, `NormalizeHost(name string) string`.

- [ ] **Step 1: Write CRD types**

`api/ngrok/v1/privateendpoint_types.go` (copy the MIT header from `trafficpolicy_types.go`):

```go
package v1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// PrivateEndpointScheme is the URL scheme of a private endpoint.
// +kubebuilder:validation:Enum=http;https;tls;tcp
type PrivateEndpointScheme string

const (
	PrivateEndpointSchemeHTTP  PrivateEndpointScheme = "http"
	PrivateEndpointSchemeHTTPS PrivateEndpointScheme = "https"
	PrivateEndpointSchemeTLS   PrivateEndpointScheme = "tls"
	PrivateEndpointSchemeTCP   PrivateEndpointScheme = "tcp"

	PrivateEndpointConditionReady = "Ready"
)

// PrivateEndpointSpec mirrors one private endpoint URL in the ngrok account.
// It is written by the operator; users do not author PrivateEndpoints.
type PrivateEndpointSpec struct {
	// URL of the private endpoint, e.g. tcp://bar.internal:6379.
	// +kubebuilder:validation:Required
	URL string `json:"url"`

	// +kubebuilder:validation:Required
	Scheme PrivateEndpointScheme `json:"scheme"`

	// +kubebuilder:validation:Required
	Hostname string `json:"hostname"`

	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=65535
	Port int32 `json:"port"`
}

// PrivateEndpointStatus is the in-cluster wiring for a private endpoint.
type PrivateEndpointStatus struct {
	// +optional
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`

	// ClusterIP is the address in-cluster DNS returns for spec.hostname.
	// +optional
	ClusterIP string `json:"clusterIP,omitempty"`

	// ForwarderPort is the forwarder container port serving this endpoint.
	// Unset when the endpoint is served by the shared http/https listener.
	// +optional
	ForwarderPort int32 `json:"forwarderPort,omitempty"`

	// +listType=map
	// +listMapKey=type
	// +kubebuilder:validation:MaxItems=8
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:categories=ngrok
// +kubebuilder:printcolumn:name="URL",type="string",JSONPath=".spec.url"
// +kubebuilder:printcolumn:name="Scheme",type="string",JSONPath=".spec.scheme"
// +kubebuilder:printcolumn:name="ClusterIP",type="string",JSONPath=".status.clusterIP"
// +kubebuilder:printcolumn:name="Ready",type="string",JSONPath=".status.conditions[?(@.type=='Ready')].status"
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// PrivateEndpoint is a read-only mirror of an ngrok private endpoint
// (*.internal or *.ngrok.direct) that pods in this cluster can reach by URL.
type PrivateEndpoint struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   PrivateEndpointSpec   `json:"spec,omitempty"`
	Status PrivateEndpointStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// PrivateEndpointList contains a list of PrivateEndpoint.
type PrivateEndpointList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []PrivateEndpoint `json:"items"`
}
```

In `groupversion_info.go` `addKnownTypes`, add `&PrivateEndpoint{}, &PrivateEndpointList{},` after the TrafficPolicy entries.

- [ ] **Step 2: Generate**

Run: `make generate manifests`
Expected: `zz_generated.deepcopy.go` gains PrivateEndpoint methods; `helm/ngrok-crds/templates/ngrok.com_privateendpoints.yaml` exists.

- [ ] **Step 3: Write failing naming tests**

`internal/privateendpoints/naming_test.go`:

```go
package privateendpoints

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	ngrokv1 "github.com/ngrok/ngrok-operator/api/ngrok/v1"
)

func TestParseURL(t *testing.T) {
	tests := []struct {
		name    string
		raw     string
		want    ngrokv1.PrivateEndpointSpec
		wantErr bool
	}{
		{"http default port", "http://foo.internal", ngrokv1.PrivateEndpointSpec{URL: "http://foo.internal", Scheme: "http", Hostname: "foo.internal", Port: 80}, false},
		{"https default port", "https://foo.ngrok.direct", ngrokv1.PrivateEndpointSpec{URL: "https://foo.ngrok.direct", Scheme: "https", Hostname: "foo.ngrok.direct", Port: 443}, false},
		{"tls default port", "tls://foo.internal", ngrokv1.PrivateEndpointSpec{URL: "tls://foo.internal", Scheme: "tls", Hostname: "foo.internal", Port: 443}, false},
		{"tcp explicit port", "tcp://bar.internal:6379", ngrokv1.PrivateEndpointSpec{URL: "tcp://bar.internal:6379", Scheme: "tcp", Hostname: "bar.internal", Port: 6379}, false},
		{"uppercase host normalized", "http://FOO.Internal:8080", ngrokv1.PrivateEndpointSpec{URL: "http://FOO.Internal:8080", Scheme: "http", Hostname: "foo.internal", Port: 8080}, false},
		{"tcp without port", "tcp://bar.internal", ngrokv1.PrivateEndpointSpec{}, true},
		{"unknown scheme", "udp://bar.internal:53", ngrokv1.PrivateEndpointSpec{}, true},
		{"port out of range", "tcp://bar.internal:70000", ngrokv1.PrivateEndpointSpec{}, true},
		{"garbage", "://", ngrokv1.PrivateEndpointSpec{}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseURL(tt.raw)
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestIsPrivateHostname(t *testing.T) {
	tests := []struct {
		host string
		want bool
	}{
		{"foo.internal", true},
		{"a.b.internal", true},
		{"foo.ngrok.direct", true},
		{"internal", false},
		{"foo.ngrok.app", false},
		{"notinternal", false},
		{"foo.internal.example.com", false},
	}
	for _, tt := range tests {
		t.Run(tt.host, func(t *testing.T) {
			assert.Equal(t, tt.want, IsPrivateHostname(tt.host))
		})
	}
}

func TestSharedEligible(t *testing.T) {
	tests := []struct {
		name   string
		scheme ngrokv1.PrivateEndpointScheme
		port   int32
		want   bool
	}{
		{"http 80", "http", 80, true},
		{"https 443", "https", 443, true},
		{"tls 443", "tls", 443, true},
		{"http 8080", "http", 8080, false},
		{"https 8443", "https", 8443, false},
		{"tcp 443", "tcp", 443, false},
		{"tcp 80", "tcp", 80, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, SharedEligible(ngrokv1.PrivateEndpointSpec{Scheme: tt.scheme, Port: tt.port}))
		})
	}
}

func TestNames(t *testing.T) {
	assert.Equal(t, CRName("http://foo.internal"), CRName("http://foo.internal"))
	assert.NotEqual(t, CRName("http://foo.internal"), CRName("https://foo.internal"))
	assert.Regexp(t, `^pe-[0-9a-f]{16}$`, CRName("http://foo.internal"))
	assert.Equal(t, HostKey("FOO.internal"), HostKey("foo.internal."))
	assert.Regexp(t, `^[0-9a-f]{16}$`, HostKey("foo.internal"))
	assert.Equal(t, "pe-host-"+HostKey("foo.internal"), HostServiceName(HostKey("foo.internal")))
}
```

- [ ] **Step 4: Run to verify failure**

Run: `go test ./internal/privateendpoints/`
Expected: FAIL, `undefined: ParseURL` etc.

- [ ] **Step 5: Implement**

`internal/privateendpoints/naming.go`:

```go
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
```

- [ ] **Step 6: Run tests**

Run: `go test ./internal/privateendpoints/`
Expected: PASS

- [ ] **Step 7: Record deviations in the spec**

Append the "Deviations from the spec" section of this plan verbatim to `specs/private-endpoints/design.md` under a new `## Implementation deviations` heading.

- [ ] **Step 8: Commit**

```bash
git add api/ngrok/v1 helm/ngrok-crds/templates/ngrok.com_privateendpoints.yaml internal/privateendpoints specs/private-endpoints/design.md
git commit -m "Add PrivateEndpoint CRD and shared naming rules

PrivateEndpoints mirror the account's private endpoints in the operator
namespace so the forwarder can answer DNS and route without calling the
ngrok API itself. Naming lives in one package so poller, controller and
forwarder agree on CR, label and Service names.

Co-Authored-By: Claude Opus 5.5 (1M context) <noreply@anthropic.com>"
```

---

### Task 3: Poller (ngrok API → PrivateEndpoint CRs)

**Files:**
- Create: `internal/controller/privateendpoints/poller.go`
- Test: `internal/controller/privateendpoints/poller_test.go`

**Interfaces:**
- Consumes: Task 2 `ParseURL`, `IsPrivateHostname`, `CRName`, `HostKey`, labels.
- Produces: `type EndpointLister interface { List(*ngrok.Paging) ngrok.Iter[*ngrok.Endpoint] }`; `type Poller struct { client.Client; Log logr.Logger; Namespace string; Endpoints EndpointLister; Interval time.Duration }` with `Start(ctx) error` (manager.Runnable) and `Sync(ctx) error`.

- [ ] **Step 1: Write failing tests**

`internal/controller/privateendpoints/poller_test.go`:

```go
package privateendpoints

import (
	"context"
	"errors"
	"testing"

	"github.com/go-logr/logr"
	"github.com/ngrok/ngrok-api-go/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	ngrokv1 "github.com/ngrok/ngrok-operator/api/ngrok/v1"
	"github.com/ngrok/ngrok-operator/internal/mocks/nmockapi"
	pe "github.com/ngrok/ngrok-operator/internal/privateendpoints"
)

const testNS = "ngrok-operator"

func newScheme(t *testing.T) *runtime.Scheme {
	t.Helper()
	s := runtime.NewScheme()
	require.NoError(t, ngrokv1.AddToScheme(s))
	return s
}

func existingCR(url string, managed bool) *ngrokv1.PrivateEndpoint {
	spec, _ := pe.ParseURL(url)
	labels := map[string]string{pe.HostLabel: pe.HostKey(spec.Hostname)}
	if managed {
		labels[pe.ManagedByLabel] = pe.ManagedByValue
	}
	return &ngrokv1.PrivateEndpoint{
		ObjectMeta: metav1.ObjectMeta{Name: pe.CRName(url), Namespace: testNS, Labels: labels},
		Spec:       spec,
	}
}

func crURLs(t *testing.T, c client.Client) []string {
	t.Helper()
	var list ngrokv1.PrivateEndpointList
	require.NoError(t, c.List(context.Background(), &list, client.InNamespace(testNS)))
	var urls []string
	for _, item := range list.Items {
		urls = append(urls, item.Spec.URL)
	}
	return urls
}

func TestPollerSync(t *testing.T) {
	tests := []struct {
		name      string
		endpoints []ngrok.EndpointCreate
		existing  []*ngrokv1.PrivateEndpoint
		listErr   error
		wantURLs  []string
		wantErr   bool
	}{
		{
			name: "creates private endpoints and skips public and kubernetes-bound",
			endpoints: []ngrok.EndpointCreate{
				{URL: "http://foo.internal", Bindings: []string{"internal"}},
				{URL: "https://foo.ngrok.direct"},
				{URL: "tcp://bar.internal:6379", Bindings: []string{"internal"}},
				{URL: "https://public.ngrok.app", Bindings: []string{"public"}},
				{URL: "http://svc.ns", Bindings: []string{"kubernetes"}},
				{URL: "http://legacy.internal", Bindings: []string{"kubernetes"}},
				{URL: "tcp://noport.internal"},
			},
			wantURLs: []string{"http://foo.internal", "https://foo.ngrok.direct", "tcp://bar.internal:6379"},
		},
		{
			name:      "deletes managed CRs that are gone, keeps unmanaged ones",
			endpoints: []ngrok.EndpointCreate{{URL: "http://foo.internal"}},
			existing: []*ngrokv1.PrivateEndpoint{
				existingCR("http://foo.internal", true),
				existingCR("http://gone.internal", true),
				existingCR("http://handmade.internal", false),
			},
			wantURLs: []string{"http://foo.internal", "http://handmade.internal"},
		},
		{
			name:     "list error keeps CRs",
			listErr:  errors.New("api down"),
			existing: []*ngrokv1.PrivateEndpoint{existingCR("http://foo.internal", true)},
			wantURLs: []string{"http://foo.internal"},
			wantErr:  true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			eps := nmockapi.NewEndpointsClient()
			for _, e := range tt.endpoints {
				_, err := eps.Create(ctx, &e)
				require.NoError(t, err)
			}
			if tt.listErr != nil {
				eps.SetListError(tt.listErr)
			}
			b := fake.NewClientBuilder().WithScheme(newScheme(t))
			for _, cr := range tt.existing {
				b = b.WithObjects(cr)
			}
			c := b.Build()
			p := &Poller{Client: c, Log: logr.Discard(), Namespace: testNS, Endpoints: eps}

			err := p.Sync(ctx)
			if tt.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
			assert.ElementsMatch(t, tt.wantURLs, crURLs(t, c))
		})
	}
}

func TestPollerSyncLabelsAndDedupe(t *testing.T) {
	ctx := context.Background()
	eps := nmockapi.NewEndpointsClient()
	// Pooled endpoints share a URL. The mock rejects duplicate URLs on Create,
	// so feed desiredSpecs directly.
	pooled := []*ngrok.Endpoint{
		{ID: "ep_1", URL: "tcp://bar.internal:6379"},
		{ID: "ep_2", URL: "tcp://bar.internal:6379"},
	}
	desired := desiredSpecs(logr.Discard(), pooled)
	require.Len(t, desired, 1)

	_, err := eps.Create(ctx, &ngrok.EndpointCreate{URL: "tcp://bar.internal:6379"})
	require.NoError(t, err)
	c := fake.NewClientBuilder().WithScheme(newScheme(t)).Build()
	require.NoError(t, (&Poller{Client: c, Log: logr.Discard(), Namespace: testNS, Endpoints: eps}).Sync(ctx))

	var got ngrokv1.PrivateEndpoint
	require.NoError(t, c.Get(ctx, client.ObjectKey{Namespace: testNS, Name: pe.CRName("tcp://bar.internal:6379")}, &got))
	assert.Equal(t, pe.ManagedByValue, got.Labels[pe.ManagedByLabel])
	assert.Equal(t, pe.HostKey("bar.internal"), got.Labels[pe.HostLabel])
	assert.Equal(t, int32(6379), got.Spec.Port)
}
```

- [ ] **Step 2: Run to verify failure**

Run: `go test ./internal/controller/privateendpoints/ -run TestPoller`
Expected: FAIL, `undefined: Poller`.

- [ ] **Step 3: Implement**

`internal/controller/privateendpoints/poller.go`:

```go
// Package privateendpoints reconciles PrivateEndpoint CRs from the ngrok API
// and wires them to in-cluster Services.
package privateendpoints

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"time"

	"github.com/go-logr/logr"
	"github.com/ngrok/ngrok-api-go/v9"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	ngrokv1 "github.com/ngrok/ngrok-operator/api/ngrok/v1"
	pe "github.com/ngrok/ngrok-operator/internal/privateendpoints"
)

type EndpointLister interface {
	List(*ngrok.Paging) ngrok.Iter[*ngrok.Endpoint]
}

// Poller mirrors the account's private endpoints into PrivateEndpoint CRs.
type Poller struct {
	client.Client
	Log       logr.Logger
	Namespace string
	Endpoints EndpointLister
	Interval  time.Duration
}

func (p *Poller) Start(ctx context.Context) error {
	t := time.NewTicker(p.Interval)
	defer t.Stop()
	for {
		if err := p.Sync(ctx); err != nil {
			p.Log.Error(err, "private endpoint sync failed, keeping current PrivateEndpoints")
		}
		select {
		case <-ctx.Done():
			return nil
		case <-t.C:
		}
	}
}

// Sync converges PrivateEndpoint CRs to the API's private endpoints. On a
// list error it changes nothing, so an API outage never deletes CRs.
func (p *Poller) Sync(ctx context.Context) error {
	var eps []*ngrok.Endpoint
	iter := p.Endpoints.List(nil)
	for iter.Next(ctx) {
		eps = append(eps, iter.Item())
	}
	if err := iter.Err(); err != nil {
		return fmt.Errorf("listing endpoints: %w", err)
	}
	desired := desiredSpecs(p.Log, eps)

	var existing ngrokv1.PrivateEndpointList
	if err := p.List(ctx, &existing, client.InNamespace(p.Namespace), client.MatchingLabels{pe.ManagedByLabel: pe.ManagedByValue}); err != nil {
		return fmt.Errorf("listing PrivateEndpoints: %w", err)
	}

	var errs []error
	for i := range existing.Items {
		cr := &existing.Items[i]
		if _, ok := desired[cr.Name]; ok {
			delete(desired, cr.Name)
			continue
		}
		if err := p.Delete(ctx, cr); err != nil && !apierrors.IsNotFound(err) {
			errs = append(errs, fmt.Errorf("deleting PrivateEndpoint %s: %w", cr.Name, err))
		}
	}
	for name, spec := range desired {
		cr := &ngrokv1.PrivateEndpoint{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: p.Namespace,
				Labels: map[string]string{
					pe.ManagedByLabel: pe.ManagedByValue,
					pe.HostLabel:      pe.HostKey(spec.Hostname),
				},
			},
			Spec: spec,
		}
		if err := p.Create(ctx, cr); err != nil && !apierrors.IsAlreadyExists(err) {
			errs = append(errs, fmt.Errorf("creating PrivateEndpoint for %s: %w", spec.URL, err))
		}
	}
	return errors.Join(errs...)
}

// desiredSpecs keeps private, non-kubernetes-bound endpoints, keyed by CR
// name. Pooled endpoints share a URL and collapse into one entry.
func desiredSpecs(log logr.Logger, eps []*ngrok.Endpoint) map[string]ngrokv1.PrivateEndpointSpec {
	out := map[string]ngrokv1.PrivateEndpointSpec{}
	for _, ep := range eps {
		if slices.Contains(ep.Bindings, "kubernetes") {
			continue
		}
		spec, err := pe.ParseURL(ep.URL)
		if err != nil {
			log.V(1).Info("skipping endpoint", "id", ep.ID, "url", ep.URL, "reason", err.Error())
			continue
		}
		if !pe.IsPrivateHostname(spec.Hostname) {
			continue
		}
		out[pe.CRName(ep.URL)] = spec
	}
	return out
}
```

Note: If Task 1 found `.ngrok.direct` endpoints carry a different binding value (or `Bindings` is empty) that doesn't change this filter; if it found `kubernetes`-bound endpoints use a different marker, update the `slices.Contains` check and the test case accordingly.

- [ ] **Step 4: Run tests**

Run: `go test ./internal/controller/privateendpoints/ -run TestPoller -v`
Expected: PASS (all subtests)

- [ ] **Step 5: Commit**

```bash
git add internal/controller/privateendpoints/poller.go internal/controller/privateendpoints/poller_test.go
git commit -m "Poll ngrok API for private endpoints into PrivateEndpoint CRs

Mirrors every .internal/.ngrok.direct endpoint so in-cluster DNS and
routing can be driven from the cluster. A failed list changes nothing so
an API outage can't blackhole every private endpoint.

Co-Authored-By: Claude Opus 5.5 (1M context) <noreply@anthropic.com>"
```

---

### Task 4: Host-keyed controller (CRs → Services + status)

**Files:**
- Create: `internal/controller/privateendpoints/controller.go`
- Test: `internal/controller/privateendpoints/controller_test.go`

**Interfaces:**
- Consumes: Task 2 labels, `SharedEligible`, `HostServiceName`.
- Produces: `type Reconciler struct { client.Client; Log logr.Logger; Namespace string; SharedServiceName string; ForwarderSelector map[string]string; PortMin, PortMax int32 }` with `Reconcile(ctx, ctrl.Request) (ctrl.Result, error)` where `req.Name` is a host key, and `SetupWithManager(mgr ctrl.Manager) error`.

- [ ] **Step 1: Write failing envtest**

`internal/controller/privateendpoints/controller_test.go`:

```go
package privateendpoints

import (
	"context"
	"os"
	"testing"

	"github.com/go-logr/logr"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"

	ngrokv1 "github.com/ngrok/ngrok-operator/api/ngrok/v1"
	pe "github.com/ngrok/ngrok-operator/internal/privateendpoints"
	"github.com/ngrok/ngrok-operator/internal/testutils"
)

var envClient client.Client

func TestMain(m *testing.M) {
	env := &envtest.Environment{
		CRDDirectoryPaths:     []string{testutils.OperatorCRDPath("..", "..", "..")},
		ErrorIfCRDPathMissing: true,
	}
	cfg, err := env.Start()
	if err != nil {
		panic(err)
	}
	s := runtime.NewScheme()
	_ = clientgoscheme.AddToScheme(s)
	_ = ngrokv1.AddToScheme(s)
	envClient, err = client.New(cfg, client.Options{Scheme: s})
	if err != nil {
		panic(err)
	}
	code := m.Run()
	_ = env.Stop()
	os.Exit(code)
}

func setupNS(t *testing.T, withShared bool) (string, *Reconciler) {
	t.Helper()
	ctx := context.Background()
	ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{GenerateName: "pe-test-"}}
	require.NoError(t, envClient.Create(ctx, ns))
	if withShared {
		require.NoError(t, envClient.Create(ctx, &corev1.Service{
			ObjectMeta: metav1.ObjectMeta{Name: "shared", Namespace: ns.Name},
			Spec: corev1.ServiceSpec{
				Selector: map[string]string{"app": "fwd"},
				Ports:    []corev1.ServicePort{{Name: "http", Port: 80}, {Name: "https", Port: 443}},
			},
		}))
	}
	return ns.Name, &Reconciler{
		Client: envClient, Log: logr.Discard(), Namespace: ns.Name,
		SharedServiceName: "shared", ForwarderSelector: map[string]string{"app": "fwd"},
		PortMin: 20000, PortMax: 20999,
	}
}

func createCR(t *testing.T, ns, url string) {
	t.Helper()
	spec, err := pe.ParseURL(url)
	require.NoError(t, err)
	require.NoError(t, envClient.Create(context.Background(), &ngrokv1.PrivateEndpoint{
		ObjectMeta: metav1.ObjectMeta{
			Name: pe.CRName(url), Namespace: ns,
			Labels: map[string]string{pe.ManagedByLabel: pe.ManagedByValue, pe.HostLabel: pe.HostKey(spec.Hostname)},
		},
		Spec: spec,
	}))
}

func getCR(t *testing.T, ns, url string) ngrokv1.PrivateEndpoint {
	t.Helper()
	var cr ngrokv1.PrivateEndpoint
	require.NoError(t, envClient.Get(context.Background(), types.NamespacedName{Namespace: ns, Name: pe.CRName(url)}, &cr))
	return cr
}

func reconcileHost(t *testing.T, r *Reconciler, host string) ctrl.Result {
	t.Helper()
	res, err := r.Reconcile(context.Background(), ctrl.Request{NamespacedName: types.NamespacedName{Namespace: r.Namespace, Name: pe.HostKey(host)}})
	require.NoError(t, err)
	return res
}

func sharedIP(t *testing.T, ns string) string {
	t.Helper()
	var svc corev1.Service
	require.NoError(t, envClient.Get(context.Background(), types.NamespacedName{Namespace: ns, Name: "shared"}, &svc))
	return svc.Spec.ClusterIP
}

func hostService(t *testing.T, ns, host string) (*corev1.Service, bool) {
	t.Helper()
	var svc corev1.Service
	err := envClient.Get(context.Background(), types.NamespacedName{Namespace: ns, Name: pe.HostServiceName(pe.HostKey(host))}, &svc)
	if apierrors.IsNotFound(err) {
		return nil, false
	}
	require.NoError(t, err)
	return &svc, true
}

func TestReconcile(t *testing.T) {
	t.Run("shared host uses shared service ip", func(t *testing.T) {
		ns, r := setupNS(t, true)
		createCR(t, ns, "http://foo.internal")
		createCR(t, ns, "https://foo.internal")
		reconcileHost(t, r, "foo.internal")

		for _, u := range []string{"http://foo.internal", "https://foo.internal"} {
			cr := getCR(t, ns, u)
			assert.Equal(t, sharedIP(t, ns), cr.Status.ClusterIP)
			assert.Zero(t, cr.Status.ForwarderPort)
			assert.True(t, meta.IsStatusConditionTrue(cr.Status.Conditions, ngrokv1.PrivateEndpointConditionReady))
		}
		_, ok := hostService(t, ns, "foo.internal")
		assert.False(t, ok)
	})

	t.Run("missing shared service is not ready and requeues", func(t *testing.T) {
		ns, r := setupNS(t, false)
		createCR(t, ns, "http://foo.internal")
		res := reconcileHost(t, r, "foo.internal")
		assert.NotZero(t, res.RequeueAfter)
		cr := getCR(t, ns, "http://foo.internal")
		assert.Empty(t, cr.Status.ClusterIP)
		assert.False(t, meta.IsStatusConditionTrue(cr.Status.Conditions, ngrokv1.PrivateEndpointConditionReady))
	})

	t.Run("tcp host gets dedicated service", func(t *testing.T) {
		ns, r := setupNS(t, true)
		createCR(t, ns, "tcp://bar.internal:6379")
		reconcileHost(t, r, "bar.internal")

		svc, ok := hostService(t, ns, "bar.internal")
		require.True(t, ok)
		require.Len(t, svc.Spec.Ports, 1)
		cr := getCR(t, ns, "tcp://bar.internal:6379")
		assert.Equal(t, int32(6379), svc.Spec.Ports[0].Port)
		assert.Equal(t, cr.Status.ForwarderPort, svc.Spec.Ports[0].TargetPort.IntVal)
		assert.Equal(t, svc.Spec.ClusterIP, cr.Status.ClusterIP)
		assert.Equal(t, map[string]string{"app": "fwd"}, svc.Spec.Selector)
		assert.GreaterOrEqual(t, cr.Status.ForwarderPort, int32(20000))
	})

	t.Run("mixed host goes dedicated", func(t *testing.T) {
		ns, r := setupNS(t, true)
		createCR(t, ns, "http://mix.internal")
		createCR(t, ns, "tcp://mix.internal:6379")
		reconcileHost(t, r, "mix.internal")

		svc, ok := hostService(t, ns, "mix.internal")
		require.True(t, ok)
		assert.Len(t, svc.Spec.Ports, 2)
		httpCR, tcpCR := getCR(t, ns, "http://mix.internal"), getCR(t, ns, "tcp://mix.internal:6379")
		assert.Equal(t, svc.Spec.ClusterIP, httpCR.Status.ClusterIP)
		assert.Equal(t, svc.Spec.ClusterIP, tcpCR.Status.ClusterIP)
		assert.NotZero(t, httpCR.Status.ForwarderPort)
		assert.NotEqual(t, httpCR.Status.ForwarderPort, tcpCR.Status.ForwarderPort)
	})

	t.Run("ports are unique across hosts and stable across reconciles", func(t *testing.T) {
		ns, r := setupNS(t, true)
		createCR(t, ns, "tcp://a.internal:5432")
		createCR(t, ns, "tcp://b.internal:5432")
		reconcileHost(t, r, "a.internal")
		reconcileHost(t, r, "b.internal")
		a, b := getCR(t, ns, "tcp://a.internal:5432"), getCR(t, ns, "tcp://b.internal:5432")
		assert.NotEqual(t, a.Status.ForwarderPort, b.Status.ForwarderPort)

		reconcileHost(t, r, "a.internal")
		assert.Equal(t, a.Status.ForwarderPort, getCR(t, ns, "tcp://a.internal:5432").Status.ForwarderPort)
	})

	t.Run("last CR gone deletes host service", func(t *testing.T) {
		ns, r := setupNS(t, true)
		createCR(t, ns, "tcp://gone.internal:6379")
		reconcileHost(t, r, "gone.internal")
		_, ok := hostService(t, ns, "gone.internal")
		require.True(t, ok)

		cr := getCR(t, ns, "tcp://gone.internal:6379")
		require.NoError(t, envClient.Delete(context.Background(), &cr))
		reconcileHost(t, r, "gone.internal")
		_, ok = hostService(t, ns, "gone.internal")
		assert.False(t, ok)
	})

	t.Run("host switching to shared deletes host service", func(t *testing.T) {
		ns, r := setupNS(t, true)
		createCR(t, ns, "http://sw.internal")
		createCR(t, ns, "tcp://sw.internal:6379")
		reconcileHost(t, r, "sw.internal")
		tcp := getCR(t, ns, "tcp://sw.internal:6379")
		require.NoError(t, envClient.Delete(context.Background(), &tcp))
		reconcileHost(t, r, "sw.internal")

		_, ok := hostService(t, ns, "sw.internal")
		assert.False(t, ok)
		cr := getCR(t, ns, "http://sw.internal")
		assert.Equal(t, sharedIP(t, ns), cr.Status.ClusterIP)
		assert.Zero(t, cr.Status.ForwarderPort)
	})
}
```

- [ ] **Step 2: Run to verify failure**

Run: `setup-envtest use $ENVTEST_K8S_VERSION && go test ./internal/controller/privateendpoints/ -run TestReconcile`
Expected: FAIL, `undefined: Reconciler`.

- [ ] **Step 3: Implement**

`internal/controller/privateendpoints/controller.go`:

```go
package privateendpoints

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"time"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	ngrokv1 "github.com/ngrok/ngrok-operator/api/ngrok/v1"
	pe "github.com/ngrok/ngrok-operator/internal/privateendpoints"
)

// +kubebuilder:rbac:groups=ngrok.com,resources=privateendpoints,verbs=get;list;watch;create;delete
// +kubebuilder:rbac:groups=ngrok.com,resources=privateendpoints/status,verbs=get;update;patch
// +kubebuilder:rbac:groups="",resources=services,verbs=get;list;watch;create;update;patch;delete

// Reconciler is keyed by hostname (req.Name is a host key), because DNS
// returns one IP per name: every endpoint on a hostname must share it.
type Reconciler struct {
	client.Client
	Log               logr.Logger
	Namespace         string
	SharedServiceName string
	ForwarderSelector map[string]string
	PortMin, PortMax  int32
}

func (r *Reconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	hostKey := req.Name
	var all ngrokv1.PrivateEndpointList
	if err := r.List(ctx, &all, client.InNamespace(r.Namespace)); err != nil {
		return ctrl.Result{}, fmt.Errorf("listing PrivateEndpoints: %w", err)
	}
	var mine []*ngrokv1.PrivateEndpoint
	used := map[int32]bool{}
	for i := range all.Items {
		cr := &all.Items[i]
		if cr.DeletionTimestamp != nil {
			continue
		}
		if cr.Labels[pe.HostLabel] == hostKey {
			mine = append(mine, cr)
		}
		if cr.Status.ForwarderPort != 0 {
			used[cr.Status.ForwarderPort] = true
		}
	}
	svcName := pe.HostServiceName(hostKey)
	if len(mine) == 0 {
		return ctrl.Result{}, r.deleteService(ctx, svcName)
	}
	for _, cr := range mine {
		if !pe.SharedEligible(cr.Spec) {
			return r.reconcileDedicated(ctx, hostKey, svcName, mine, used)
		}
	}
	return r.reconcileShared(ctx, svcName, mine)
}

func (r *Reconciler) reconcileShared(ctx context.Context, svcName string, crs []*ngrokv1.PrivateEndpoint) (ctrl.Result, error) {
	if err := r.deleteService(ctx, svcName); err != nil {
		return ctrl.Result{}, err
	}
	var shared corev1.Service
	err := r.Get(ctx, types.NamespacedName{Namespace: r.Namespace, Name: r.SharedServiceName}, &shared)
	if err != nil && !apierrors.IsNotFound(err) {
		return ctrl.Result{}, fmt.Errorf("getting shared Service: %w", err)
	}
	ip := shared.Spec.ClusterIP
	var errs []error
	for _, cr := range crs {
		errs = append(errs, r.setStatus(ctx, cr, ip, 0, "SharedServiceNotReady", "waiting for Service "+r.SharedServiceName))
	}
	if ip == "" {
		return ctrl.Result{RequeueAfter: 5 * time.Second}, errors.Join(errs...)
	}
	return ctrl.Result{}, errors.Join(errs...)
}

func (r *Reconciler) reconcileDedicated(ctx context.Context, hostKey, svcName string, crs []*ngrokv1.PrivateEndpoint, used map[int32]bool) (ctrl.Result, error) {
	sort.Slice(crs, func(i, j int) bool {
		if crs[i].Spec.Port != crs[j].Spec.Port {
			return crs[i].Spec.Port < crs[j].Spec.Port
		}
		return crs[i].Name < crs[j].Name
	})
	fwdPorts := map[string]int32{}
	byPort := map[int32]string{}
	var ports []corev1.ServicePort
	var conflicts []*ngrokv1.PrivateEndpoint
	for _, cr := range crs {
		if _, taken := byPort[cr.Spec.Port]; taken {
			conflicts = append(conflicts, cr)
			continue
		}
		byPort[cr.Spec.Port] = cr.Name
		fp := cr.Status.ForwarderPort
		if fp < r.PortMin || fp > r.PortMax {
			var err error
			if fp, err = r.allocate(used); err != nil {
				return ctrl.Result{}, err
			}
			used[fp] = true
		}
		fwdPorts[cr.Name] = fp
		ports = append(ports, corev1.ServicePort{
			Name:       fmt.Sprintf("port-%d", cr.Spec.Port),
			Protocol:   corev1.ProtocolTCP,
			Port:       cr.Spec.Port,
			TargetPort: intstr.FromInt32(fp),
		})
	}

	svc := &corev1.Service{ObjectMeta: metav1.ObjectMeta{Name: svcName, Namespace: r.Namespace}}
	if _, err := controllerutil.CreateOrUpdate(ctx, r.Client, svc, func() error {
		svc.Labels = map[string]string{pe.ManagedByLabel: pe.ManagedByValue, pe.HostLabel: hostKey}
		svc.Spec.Type = corev1.ServiceTypeClusterIP
		svc.Spec.Selector = r.ForwarderSelector
		svc.Spec.Ports = ports
		return nil
	}); err != nil {
		return ctrl.Result{}, fmt.Errorf("applying Service %s: %w", svcName, err)
	}

	var errs []error
	for _, cr := range crs {
		if fp, ok := fwdPorts[cr.Name]; ok {
			errs = append(errs, r.setStatus(ctx, cr, svc.Spec.ClusterIP, fp, "ServiceNotReady", ""))
		}
	}
	for _, cr := range conflicts {
		errs = append(errs, r.setStatus(ctx, cr, "", 0, "PortConflict",
			fmt.Sprintf("PrivateEndpoint %s already serves port %d on this hostname", byPort[cr.Spec.Port], cr.Spec.Port)))
	}
	if svc.Spec.ClusterIP == "" {
		return ctrl.Result{RequeueAfter: 2 * time.Second}, errors.Join(errs...)
	}
	return ctrl.Result{}, errors.Join(errs...)
}

func (r *Reconciler) allocate(used map[int32]bool) (int32, error) {
	for p := r.PortMin; p <= r.PortMax; p++ {
		if !used[p] {
			return p, nil
		}
	}
	return 0, fmt.Errorf("no free forwarder ports in %d-%d", r.PortMin, r.PortMax)
}

// setStatus writes ip/port and the Ready condition. An empty ip means not
// ready, with notReadyReason and msg explaining why.
func (r *Reconciler) setStatus(ctx context.Context, cr *ngrokv1.PrivateEndpoint, ip string, port int32, notReadyReason, msg string) error {
	orig := cr.Status.DeepCopy()
	cr.Status.ClusterIP = ip
	cr.Status.ForwarderPort = port
	cr.Status.ObservedGeneration = cr.Generation
	cond := metav1.Condition{Type: ngrokv1.PrivateEndpointConditionReady, Status: metav1.ConditionTrue, Reason: "Ready", ObservedGeneration: cr.Generation}
	if ip == "" {
		cond.Status, cond.Reason, cond.Message = metav1.ConditionFalse, notReadyReason, msg
	}
	meta.SetStatusCondition(&cr.Status.Conditions, cond)
	if equality.Semantic.DeepEqual(orig, &cr.Status) {
		return nil
	}
	if err := r.Status().Update(ctx, cr); err != nil {
		return fmt.Errorf("updating PrivateEndpoint %s status: %w", cr.Name, err)
	}
	return nil
}

func (r *Reconciler) deleteService(ctx context.Context, name string) error {
	err := r.Delete(ctx, &corev1.Service{ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: r.Namespace}})
	if err != nil && !apierrors.IsNotFound(err) {
		return fmt.Errorf("deleting Service %s: %w", name, err)
	}
	return nil
}

func (r *Reconciler) hostRequest(key string) reconcile.Request {
	return reconcile.Request{NamespacedName: types.NamespacedName{Namespace: r.Namespace, Name: key}}
}

func (r *Reconciler) SetupWithManager(mgr ctrl.Manager) error {
	byHost := handler.EnqueueRequestsFromMapFunc(func(_ context.Context, o client.Object) []reconcile.Request {
		if o.GetNamespace() != r.Namespace || o.GetLabels()[pe.HostLabel] == "" {
			return nil
		}
		return []reconcile.Request{r.hostRequest(o.GetLabels()[pe.HostLabel])}
	})
	services := handler.EnqueueRequestsFromMapFunc(func(ctx context.Context, o client.Object) []reconcile.Request {
		if o.GetNamespace() != r.Namespace {
			return nil
		}
		if o.GetName() != r.SharedServiceName {
			if key := o.GetLabels()[pe.HostLabel]; key != "" {
				return []reconcile.Request{r.hostRequest(key)}
			}
			return nil
		}
		var list ngrokv1.PrivateEndpointList
		if err := r.List(ctx, &list, client.InNamespace(r.Namespace)); err != nil {
			r.Log.Error(err, "listing PrivateEndpoints for shared Service change")
			return nil
		}
		seen := map[string]bool{}
		var reqs []reconcile.Request
		for _, cr := range list.Items {
			if key := cr.Labels[pe.HostLabel]; key != "" && !seen[key] {
				seen[key] = true
				reqs = append(reqs, r.hostRequest(key))
			}
		}
		return reqs
	})
	return ctrl.NewControllerManagedBy(mgr).
		Named("privateendpoint").
		Watches(&ngrokv1.PrivateEndpoint{}, byHost, builder.WithPredicates(predicate.GenerationChangedPredicate{})).
		Watches(&corev1.Service{}, services).
		WithOptions(controller.Options{MaxConcurrentReconciles: 1}). // port allocation assumes one worker
		Complete(r)
}
```

- [ ] **Step 4: Run tests**

Run: `go test ./internal/controller/privateendpoints/ -v`
Expected: PASS (TestPoller*, TestReconcile/* all subtests)

- [ ] **Step 5: Regenerate RBAC and commit**

Run: `make manifests` (RBAC markers; Helm RBAC is hand-written and added in Task 9).

```bash
git add internal/controller/privateendpoints/controller.go internal/controller/privateendpoints/controller_test.go
git commit -m "Give each private endpoint hostname a reachable ClusterIP

DNS can only answer one address per name, so wiring is keyed by hostname:
hostnames served entirely on http:80 / https:443 / tls:443 share the
forwarder Service and get demuxed by Host/SNI; anything else gets one
Service per hostname with a forwarder port per endpoint.

Co-Authored-By: Claude Opus 5.5 (1M context) <noreply@anthropic.com>"
```

---

### Task 5: api-manager wiring

**Files:**
- Modify: `cmd/api-manager.go`

**Interfaces:**
- Consumes: Task 3 `Poller`, Task 4 `Reconciler`, `pe.ForwarderComponent`.
- Produces: flags `--enable-feature-private-endpoints` (bool, default false) and `--private-endpoints-shared-service` (string, default `ngrok-operator-private-endpoints`), consumed by Helm in Task 9.

- [ ] **Step 1: Add opts fields and flags**

In `apiManagerOpts` next to `enableFeatureBindings`:

```go
	enableFeaturePrivateEndpoints bool
	privateEndpointsSharedService string
```

In `apiCmd()` after the `--enable-feature-bindings` flag:

```go
	c.Flags().BoolVar(&opts.enableFeaturePrivateEndpoints, "enable-feature-private-endpoints", false, "Mirrors ngrok private endpoints into PrivateEndpoint resources for in-cluster access")
	c.Flags().StringVar(&opts.privateEndpointsSharedService, "private-endpoints-shared-service", "ngrok-operator-private-endpoints", "Name of the Service in the operator namespace fronting the private endpoint forwarder's shared http/https listeners")
```

- [ ] **Step 2: Pin PrivateEndpoint cache to the operator namespace**

In `loadManager`, add to `Cache.ByObject`:

```go
				&ngrokv1.PrivateEndpoint{}: {
					Namespaces: map[string]cache.Config{
						opts.namespace: {},
					},
				},
```

(Services stay on the default cache scope. With `watchNamespace` set to a different namespace the controller can't see operator-namespace Services — accepted POC limitation, note it in findings.md.)

- [ ] **Step 3: Register feature set**

In `runNormalMode`, after the bindings block:

```go
	if opts.enableFeaturePrivateEndpoints {
		setupLog.Info("Private Endpoints feature set enabled")
		if err := enablePrivateEndpointsFeatureSet(opts, mgr, ngrokClientset); err != nil {
			return fmt.Errorf("unable to enable Private Endpoints feature set: %w", err)
		}
	}
```

New function after `enableBindingsFeatureSet` (import `privateendpointscontroller "github.com/ngrok/ngrok-operator/internal/controller/privateendpoints"` and `pe "github.com/ngrok/ngrok-operator/internal/privateendpoints"`):

```go
// enablePrivateEndpointsFeatureSet mirrors private endpoints into the cluster
// for the private-endpoint-forwarder.
func enablePrivateEndpointsFeatureSet(opts apiManagerOpts, mgr ctrl.Manager, ngrokClientset ngrokapi.Clientset) error {
	if err := (&privateendpointscontroller.Reconciler{
		Client:            mgr.GetClient(),
		Log:               ctrl.Log.WithName("controllers").WithName("PrivateEndpoint"),
		Namespace:         opts.namespace,
		SharedServiceName: opts.privateEndpointsSharedService,
		ForwarderSelector: map[string]string{"app.kubernetes.io/component": pe.ForwarderComponent},
		PortMin:           20000,
		PortMax:           20999,
	}).SetupWithManager(mgr); err != nil {
		return fmt.Errorf("setting up PrivateEndpoint controller: %w", err)
	}
	return mgr.Add(&privateendpointscontroller.Poller{
		Client:    mgr.GetClient(),
		Log:       ctrl.Log.WithName("controllers").WithName("PrivateEndpointPoller"),
		Namespace: opts.namespace,
		Endpoints: ngrokClientset.Endpoints(),
		Interval:  10 * time.Second,
	})
}
```

The Poller doesn't implement `LeaderElectionRunnable`, so the manager runs it only on the leader — matches the spec.

- [ ] **Step 4: Build and vet**

Run: `go build ./... && go vet ./cmd/...`
Expected: no output.

- [ ] **Step 5: Commit**

```bash
git add cmd/api-manager.go
git commit -m "Wire private endpoints into api-manager behind a feature flag

Off by default so existing installs are untouched while this is a POC.

Co-Authored-By: Claude Opus 5.5 (1M context) <noreply@anthropic.com>"
```

---

### Task 6: Forwarder routing table and DNS

**Files:**
- Create: `internal/privateendpoints/forwarder/table.go`, `internal/privateendpoints/forwarder/dns.go`
- Test: `internal/privateendpoints/forwarder/table_test.go`, `internal/privateendpoints/forwarder/dns_test.go`
- Modify: `go.mod`, `go.sum` (`github.com/miekg/dns`)

**Interfaces:**
- Consumes: `pe.NormalizeHost`.
- Produces: `type Entry struct { Hostname string; Port int32; ClusterIP string; ForwarderPort int32 }`; `NewTable() *Table`; `(*Table).Replace([]Entry)`, `Synced() bool`, `IP(name string) (string, bool)`, `SharedTarget(host string, port int32) (string, bool)`, `PortTarget(port int32) (string, bool)`; `type ExchangeFunc func(ctx context.Context, m *dns.Msg, addr string) (*dns.Msg, error)`; `type DNSHandler struct { Table *Table; Upstream string; TTL uint32; Exchange ExchangeFunc }` with `Answer(ctx, *dns.Msg) *dns.Msg` and `ServeDNS(dns.ResponseWriter, *dns.Msg)`; `UDPExchange` (an `ExchangeFunc`).

- [ ] **Step 1: Add dependency**

```bash
go get github.com/miekg/dns@latest
```

- [ ] **Step 2: Write failing table test**

`internal/privateendpoints/forwarder/table_test.go`:

```go
package forwarder

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestTable(t *testing.T) {
	tbl := NewTable()
	assert.False(t, tbl.Synced())

	tbl.Replace([]Entry{
		{Hostname: "foo.internal", Port: 80, ClusterIP: "10.0.0.1"},
		{Hostname: "foo.internal", Port: 443, ClusterIP: "10.0.0.1"},
		{Hostname: "bar.internal", Port: 6379, ClusterIP: "10.0.0.2", ForwarderPort: 20000},
		{Hostname: "pending.internal", Port: 80},
	})
	assert.True(t, tbl.Synced())

	tests := []struct {
		name   string
		lookup func() (string, bool)
		want   string
		ok     bool
	}{
		{"ip known", func() (string, bool) { return tbl.IP("foo.internal") }, "10.0.0.1", true},
		{"ip case and trailing dot", func() (string, bool) { return tbl.IP("FOO.Internal.") }, "10.0.0.1", true},
		{"ip pending skipped", func() (string, bool) { return tbl.IP("pending.internal") }, "", false},
		{"shared http", func() (string, bool) { return tbl.SharedTarget("foo.internal", 80) }, "foo.internal:80", true},
		{"shared https", func() (string, bool) { return tbl.SharedTarget("foo.internal", 443) }, "foo.internal:443", true},
		{"shared not for dedicated host", func() (string, bool) { return tbl.SharedTarget("bar.internal", 6379) }, "", false},
		{"port target", func() (string, bool) { return tbl.PortTarget(20000) }, "bar.internal:6379", true},
		{"port unknown", func() (string, bool) { return tbl.PortTarget(20001) }, "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := tt.lookup()
			assert.Equal(t, tt.ok, ok)
			assert.Equal(t, tt.want, got)
		})
	}

	tbl.Replace(nil)
	_, ok := tbl.IP("foo.internal")
	assert.False(t, ok, "Replace drops entries not in the new set")
	assert.True(t, tbl.Synced())
}
```

- [ ] **Step 3: Write failing DNS test**

`internal/privateendpoints/forwarder/dns_test.go`:

```go
package forwarder

import (
	"context"
	"errors"
	"testing"

	"github.com/miekg/dns"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDNSAnswer(t *testing.T) {
	synced := NewTable()
	synced.Replace([]Entry{
		{Hostname: "foo.internal", Port: 80, ClusterIP: "10.0.0.1"},
		{Hostname: "foo.ngrok.direct", Port: 443, ClusterIP: "10.0.0.1"},
		{Hostname: "bar.internal", Port: 6379, ClusterIP: "10.0.0.2", ForwarderPort: 20000},
	})

	upstreamOK := func(_ context.Context, m *dns.Msg, _ string) (*dns.Msg, error) {
		r := new(dns.Msg)
		r.SetReply(m)
		r.Answer = []dns.RR{&dns.A{Hdr: dns.RR_Header{Name: m.Question[0].Name, Rrtype: dns.TypeA, Class: dns.ClassINET, Ttl: 60}, A: []byte{169, 254, 169, 254}}}
		r.Id = 9999 // handler must restore the client's id
		return r, nil
	}
	upstreamFail := func(context.Context, *dns.Msg, string) (*dns.Msg, error) { return nil, errors.New("timeout") }

	tests := []struct {
		name     string
		table    *Table
		exchange ExchangeFunc
		qname    string
		qtype    uint16
		rcode    int
		wantA    string
	}{
		{"not synced", NewTable(), upstreamOK, "foo.internal.", dns.TypeA, dns.RcodeServerFailure, ""},
		{"known http host", synced, upstreamOK, "foo.internal.", dns.TypeA, dns.RcodeSuccess, "10.0.0.1"},
		{"known tcp host", synced, upstreamOK, "bar.internal.", dns.TypeA, dns.RcodeSuccess, "10.0.0.2"},
		{"known ngrok.direct", synced, upstreamOK, "foo.ngrok.direct.", dns.TypeA, dns.RcodeSuccess, "10.0.0.1"},
		{"mixed case", synced, upstreamOK, "FOO.Internal.", dns.TypeA, dns.RcodeSuccess, "10.0.0.1"},
		{"known host AAAA is empty", synced, upstreamOK, "foo.internal.", dns.TypeAAAA, dns.RcodeSuccess, ""},
		{"unknown ngrok.direct NXDOMAIN", synced, upstreamOK, "nope.ngrok.direct.", dns.TypeA, dns.RcodeNameError, ""},
		{"unknown internal forwarded", synced, upstreamOK, "metadata.google.internal.", dns.TypeA, dns.RcodeSuccess, "169.254.169.254"},
		{"unknown internal upstream fails", synced, upstreamFail, "metadata.google.internal.", dns.TypeA, dns.RcodeServerFailure, ""},
		{"other zone refused", synced, upstreamOK, "example.com.", dns.TypeA, dns.RcodeRefused, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := &DNSHandler{Table: tt.table, Upstream: "192.0.2.53:53", TTL: 5, Exchange: tt.exchange}
			q := new(dns.Msg)
			q.SetQuestion(tt.qname, tt.qtype)
			q.Id = 42

			resp := h.Answer(context.Background(), q)
			require.NotNil(t, resp)
			assert.Equal(t, uint16(42), resp.Id)
			assert.Equal(t, tt.rcode, resp.Rcode, dns.RcodeToString[resp.Rcode])
			if tt.wantA == "" {
				assert.Empty(t, resp.Answer)
				return
			}
			require.Len(t, resp.Answer, 1)
			a, ok := resp.Answer[0].(*dns.A)
			require.True(t, ok)
			assert.Equal(t, tt.wantA, a.A.String())
			if tt.table == synced && tt.wantA != "169.254.169.254" {
				assert.Equal(t, uint32(5), a.Hdr.Ttl)
				assert.True(t, resp.Authoritative)
			}
		})
	}
}
```

- [ ] **Step 4: Run to verify failure**

Run: `go test ./internal/privateendpoints/forwarder/`
Expected: FAIL, `undefined: NewTable`, `undefined: DNSHandler`.

- [ ] **Step 5: Implement table**

`internal/privateendpoints/forwarder/table.go`:

```go
// Package forwarder serves in-cluster DNS and connection forwarding for
// ngrok private endpoints.
package forwarder

import (
	"net"
	"strconv"
	"sync"

	pe "github.com/ngrok/ngrok-operator/internal/privateendpoints"
)

// Entry is the forwarder's view of one Ready PrivateEndpoint.
type Entry struct {
	Hostname      string
	Port          int32
	ClusterIP     string
	ForwarderPort int32 // 0 when served by the shared listener
}

type hostPort struct {
	host string
	port int32
}

// Table maps names and ports to dial targets. Replace swaps it wholesale.
type Table struct {
	mu     sync.RWMutex
	synced bool
	ips    map[string]string
	shared map[hostPort]string
	ports  map[int32]string
}

func NewTable() *Table { return &Table{} }

func (t *Table) Replace(entries []Entry) {
	ips := map[string]string{}
	shared := map[hostPort]string{}
	ports := map[int32]string{}
	for _, e := range entries {
		if e.ClusterIP == "" {
			continue
		}
		host := pe.NormalizeHost(e.Hostname)
		target := net.JoinHostPort(host, strconv.Itoa(int(e.Port)))
		if _, ok := ips[host]; !ok {
			ips[host] = e.ClusterIP
		}
		if e.ForwarderPort != 0 {
			ports[e.ForwarderPort] = target
		} else {
			shared[hostPort{host, e.Port}] = target
		}
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	t.ips, t.shared, t.ports, t.synced = ips, shared, ports, true
}

func (t *Table) Synced() bool {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.synced
}

func (t *Table) IP(name string) (string, bool) {
	t.mu.RLock()
	defer t.mu.RUnlock()
	ip, ok := t.ips[pe.NormalizeHost(name)]
	return ip, ok
}

func (t *Table) SharedTarget(host string, port int32) (string, bool) {
	t.mu.RLock()
	defer t.mu.RUnlock()
	target, ok := t.shared[hostPort{pe.NormalizeHost(host), port}]
	return target, ok
}

func (t *Table) PortTarget(port int32) (string, bool) {
	t.mu.RLock()
	defer t.mu.RUnlock()
	target, ok := t.ports[port]
	return target, ok
}
```

- [ ] **Step 6: Implement DNS**

`internal/privateendpoints/forwarder/dns.go`:

```go
package forwarder

import (
	"context"
	"net"
	"strings"
	"time"

	"github.com/miekg/dns"

	pe "github.com/ngrok/ngrok-operator/internal/privateendpoints"
)

type ExchangeFunc func(ctx context.Context, m *dns.Msg, addr string) (*dns.Msg, error)

// UDPExchange sends m to addr over UDP.
func UDPExchange(ctx context.Context, m *dns.Msg, addr string) (*dns.Msg, error) {
	c := &dns.Client{Net: "udp", Timeout: 3 * time.Second}
	resp, _, err := c.ExchangeContext(ctx, m, addr)
	return resp, err
}

// DNSHandler is authoritative for known private endpoint names. Unknown
// ngrok.direct names are NXDOMAIN; unknown .internal names go upstream so
// provider names like metadata.google.internal keep resolving.
type DNSHandler struct {
	Table    *Table
	Upstream string
	TTL      uint32
	Exchange ExchangeFunc
}

func (h *DNSHandler) ServeDNS(w dns.ResponseWriter, r *dns.Msg) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = w.WriteMsg(h.Answer(ctx, r))
}

func (h *DNSHandler) Answer(ctx context.Context, r *dns.Msg) *dns.Msg {
	m := new(dns.Msg)
	m.SetReply(r)
	if len(r.Question) != 1 {
		m.Rcode = dns.RcodeFormatError
		return m
	}
	if !h.Table.Synced() {
		m.Rcode = dns.RcodeServerFailure
		return m
	}
	q := r.Question[0]
	name := pe.NormalizeHost(q.Name)

	if ip, ok := h.Table.IP(name); ok {
		m.Authoritative = true
		hdr := dns.RR_Header{Name: q.Name, Class: dns.ClassINET, Ttl: h.TTL}
		parsed := net.ParseIP(ip)
		switch {
		case q.Qtype == dns.TypeA && parsed.To4() != nil:
			hdr.Rrtype = dns.TypeA
			m.Answer = append(m.Answer, &dns.A{Hdr: hdr, A: parsed.To4()})
		case q.Qtype == dns.TypeAAAA && parsed.To4() == nil:
			hdr.Rrtype = dns.TypeAAAA
			m.Answer = append(m.Answer, &dns.AAAA{Hdr: hdr, AAAA: parsed})
		}
		return m
	}

	switch {
	case inZone(name, "ngrok.direct"):
		m.Authoritative = true
		m.Rcode = dns.RcodeNameError
	case inZone(name, "internal"):
		resp, err := h.Exchange(ctx, r, h.Upstream)
		if err != nil || resp == nil {
			m.Rcode = dns.RcodeServerFailure
			return m
		}
		resp.Id = r.Id
		return resp
	default:
		m.Rcode = dns.RcodeRefused
	}
	return m
}

func inZone(name, zone string) bool {
	return name == zone || strings.HasSuffix(name, "."+zone)
}
```

- [ ] **Step 7: Run tests**

Run: `go test ./internal/privateendpoints/forwarder/ -v`
Expected: PASS

- [ ] **Step 8: Commit**

```bash
git add go.mod go.sum internal/privateendpoints/forwarder/table.go internal/privateendpoints/forwarder/table_test.go internal/privateendpoints/forwarder/dns.go internal/privateendpoints/forwarder/dns_test.go
git commit -m "Answer DNS for private endpoint names in the forwarder

Pods resolve *.internal and *.ngrok.direct to the ClusterIP that routes
to the forwarder. Unknown .internal names go upstream because clouds like
GKE use that TLD for their own names.

Co-Authored-By: Claude Opus 5.5 (1M context) <noreply@anthropic.com>"
```

---

### Task 7: Host/SNI peek and proxying

**Files:**
- Create: `internal/privateendpoints/forwarder/peek.go`, `internal/privateendpoints/forwarder/proxy.go`
- Test: `internal/privateendpoints/forwarder/peek_test.go`, `internal/privateendpoints/forwarder/proxy_test.go`

**Interfaces:**
- Consumes: Task 6 `Table`.
- Produces: `PeekHTTPHost(*bufio.Reader) (string, error)`, `PeekSNI(*bufio.Reader) (string, error)`, `PeekBufferSize = 17 << 10`; `type DialFunc func(ctx context.Context, address string) (net.Conn, error)`; `type Proxy struct { Table *Table; Dial DialFunc; Log logr.Logger; PeekTimeout time.Duration }` with `ServeShared(ctx, net.Listener, port int32, peek func(*bufio.Reader) (string, error)) error` and `ServePort(ctx, net.Listener, fwdPort int32) error`; `type PortListeners struct { Proxy *Proxy; Log logr.Logger; BindHost string }` with `Sync(ctx, []int32) error`, `Ports() []int32`, `Close()`.

- [ ] **Step 1: Write failing peek tests**

`internal/privateendpoints/forwarder/peek_test.go`:

```go
package forwarder

import (
	"bufio"
	"bytes"
	"crypto/tls"
	"io"
	"net"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPeekHTTPHost(t *testing.T) {
	tests := []struct {
		name    string
		req     string
		want    string
		wantErr bool
	}{
		{"simple", "GET / HTTP/1.1\r\nHost: foo.internal\r\n\r\n", "foo.internal", false},
		{"with port and case", "GET / HTTP/1.1\r\nHost: FOO.internal:80\r\nUser-Agent: x\r\n\r\nbody", "foo.internal", false},
		{"no host header", "GET / HTTP/1.0\r\n\r\n", "", true},
		{"not http", "\x16\x03\x01garbage\r\n\r\n", "", true},
		{"truncated", "GET / HTTP/1.1\r\nHost: foo", "", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			br := bufio.NewReaderSize(strings.NewReader(tt.req), PeekBufferSize)
			got, err := PeekHTTPHost(br)
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
			replay, _ := io.ReadAll(br)
			assert.Equal(t, tt.req, string(replay), "peek must not consume bytes")
		})
	}
}

// clientHello captures the bytes a real TLS client sends for serverName.
func clientHello(t *testing.T, serverName string) []byte {
	t.Helper()
	c, s := net.Pipe()
	go func() {
		_ = tls.Client(c, &tls.Config{ServerName: serverName, InsecureSkipVerify: true}).Handshake()
	}()
	buf := make([]byte, PeekBufferSize)
	n, err := io.ReadAtLeast(s, buf, 5)
	require.NoError(t, err)
	recLen := 5 + (int(buf[3])<<8 | int(buf[4]))
	for n < recLen {
		m, err := s.Read(buf[n:])
		require.NoError(t, err)
		n += m
	}
	_ = s.Close()
	_ = c.Close()
	return buf[:n]
}

func TestPeekSNI(t *testing.T) {
	hello := clientHello(t, "Foo.ngrok.direct")
	tests := []struct {
		name    string
		data    []byte
		want    string
		wantErr bool
	}{
		{"client hello", hello, "foo.ngrok.direct", false},
		{"not tls", []byte("GET / HTTP/1.1\r\n\r\n"), "", true},
		{"no sni", clientHello(t, ""), "", true},
		{"truncated", hello[:20], "", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			br := bufio.NewReaderSize(bytes.NewReader(tt.data), PeekBufferSize)
			got, err := PeekSNI(br)
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
			replay, _ := io.ReadAll(br)
			assert.Equal(t, tt.data, replay, "peek must not consume bytes")
		})
	}
}
```

Note: Go's TLS client omits SNI when `ServerName` is empty (and for IP literals), which is what the `no sni` case relies on.

- [ ] **Step 2: Write failing proxy tests**

`internal/privateendpoints/forwarder/proxy_test.go`:

```go
package forwarder

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"net"
	"sync"
	"testing"
	"time"

	"github.com/go-logr/logr"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeUpstream records the target each dial asked for and echoes bytes back
// prefixed with that target, standing in for private dial.
type fakeUpstream struct {
	mu      sync.Mutex
	targets []string
}

func (f *fakeUpstream) dial(_ context.Context, target string) (net.Conn, error) {
	f.mu.Lock()
	f.targets = append(f.targets, target)
	f.mu.Unlock()
	a, b := net.Pipe()
	go func() {
		defer b.Close()
		fmt.Fprintf(b, "[%s]", target)
		_, _ = io.Copy(b, b)
	}()
	return a, nil
}

func (f *fakeUpstream) last() string {
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.targets) == 0 {
		return ""
	}
	return f.targets[len(f.targets)-1]
}

func listen(t *testing.T) net.Listener {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	return l
}

func newProxy(tbl *Table, up *fakeUpstream) *Proxy {
	return &Proxy{Table: tbl, Dial: up.dial, Log: logr.Discard(), PeekTimeout: 200 * time.Millisecond}
}

func readPrefix(t *testing.T, c net.Conn, n int) string {
	t.Helper()
	_ = c.SetReadDeadline(time.Now().Add(2 * time.Second))
	buf := make([]byte, n)
	_, err := io.ReadFull(c, buf)
	require.NoError(t, err)
	return string(buf)
}

func TestServeSharedHTTP(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	tbl := NewTable()
	tbl.Replace([]Entry{{Hostname: "foo.internal", Port: 80, ClusterIP: "10.0.0.1"}})
	up := &fakeUpstream{}
	l := listen(t)
	go func() { _ = newProxy(tbl, up).ServeShared(ctx, l, 80, PeekHTTPHost) }()

	t.Run("known host is dialed and bytes replayed", func(t *testing.T) {
		c, err := net.Dial("tcp", l.Addr().String())
		require.NoError(t, err)
		defer c.Close()
		req := "GET / HTTP/1.1\r\nHost: foo.internal\r\n\r\n"
		_, err = io.WriteString(c, req)
		require.NoError(t, err)
		want := "[foo.internal:80]" + req
		assert.Equal(t, want, readPrefix(t, c, len(want)))
	})

	t.Run("unknown host is closed", func(t *testing.T) {
		c, err := net.Dial("tcp", l.Addr().String())
		require.NoError(t, err)
		defer c.Close()
		_, _ = io.WriteString(c, "GET / HTTP/1.1\r\nHost: nope.internal\r\n\r\n")
		_ = c.SetReadDeadline(time.Now().Add(2 * time.Second))
		_, err = bufio.NewReader(c).ReadByte()
		assert.ErrorIs(t, err, io.EOF)
	})

	t.Run("idle client is closed", func(t *testing.T) {
		c, err := net.Dial("tcp", l.Addr().String())
		require.NoError(t, err)
		defer c.Close()
		_ = c.SetReadDeadline(time.Now().Add(2 * time.Second))
		_, err = bufio.NewReader(c).ReadByte()
		assert.ErrorIs(t, err, io.EOF, "server should hang up after PeekTimeout")
	})
}

func TestServeSharedTLS(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	tbl := NewTable()
	tbl.Replace([]Entry{{Hostname: "foo.ngrok.direct", Port: 443, ClusterIP: "10.0.0.1"}})
	up := &fakeUpstream{}
	l := listen(t)
	go func() { _ = newProxy(tbl, up).ServeShared(ctx, l, 443, PeekSNI) }()

	hello := clientHello(t, "foo.ngrok.direct")
	c, err := net.Dial("tcp", l.Addr().String())
	require.NoError(t, err)
	defer c.Close()
	_, err = c.Write(hello)
	require.NoError(t, err)
	want := "[foo.ngrok.direct:443]" + string(hello)
	assert.Equal(t, want, readPrefix(t, c, len(want)))
}

func TestPortListenersFollowSync(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	tbl := NewTable()
	up := &fakeUpstream{}
	pl := &PortListeners{Proxy: newProxy(tbl, up), Log: logr.Discard(), BindHost: "127.0.0.1"}
	defer pl.Close()

	port := freePort(t)
	tbl.Replace([]Entry{{Hostname: "old.internal", Port: 6379, ClusterIP: "10.0.0.2", ForwarderPort: port}})
	require.NoError(t, pl.Sync(ctx, []int32{port}))
	assert.Equal(t, "[old.internal:6379]", dialAndRead(t, port, len("[old.internal:6379]")))

	require.NoError(t, pl.Sync(ctx, nil))
	assert.Empty(t, pl.Ports())
	_, err := net.DialTimeout("tcp", fmt.Sprintf("127.0.0.1:%d", port), time.Second)
	assert.Error(t, err, "listener should be closed")

	tbl.Replace([]Entry{{Hostname: "new.internal", Port: 5432, ClusterIP: "10.0.0.3", ForwarderPort: port}})
	require.NoError(t, pl.Sync(ctx, []int32{port}))
	assert.Equal(t, "[new.internal:5432]", dialAndRead(t, port, len("[new.internal:5432]")))
}

func freePort(t *testing.T) int32 {
	t.Helper()
	l := listen(t)
	defer l.Close()
	return int32(l.Addr().(*net.TCPAddr).Port)
}

func dialAndRead(t *testing.T, port int32, n int) string {
	t.Helper()
	c, err := net.Dial("tcp", fmt.Sprintf("127.0.0.1:%d", port))
	require.NoError(t, err)
	defer c.Close()
	return readPrefix(t, c, n)
}
```

- [ ] **Step 3: Run to verify failure**

Run: `go test ./internal/privateendpoints/forwarder/ -run 'TestPeek|TestServe|TestPortListeners'`
Expected: FAIL, undefined symbols.

- [ ] **Step 4: Implement peeking**

`internal/privateendpoints/forwarder/peek.go`:

```go
package forwarder

import (
	"bufio"
	"bytes"
	"crypto/tls"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"time"
)

// PeekBufferSize fits a full TLS record (16KiB + header) or an HTTP head.
const PeekBufferSize = 17 << 10

// PeekHTTPHost returns the Host of the HTTP/1.x request at the head of br
// without consuming any bytes.
func PeekHTTPHost(br *bufio.Reader) (string, error) {
	n := 1
	for {
		b, err := br.Peek(n)
		if i := bytes.Index(b, []byte("\r\n\r\n")); i >= 0 {
			req, perr := http.ReadRequest(bufio.NewReader(bytes.NewReader(b[:i+4])))
			if perr != nil {
				return "", fmt.Errorf("parsing http request head: %w", perr)
			}
			host := req.Host
			if h, _, serr := net.SplitHostPort(host); serr == nil {
				host = h
			}
			if host == "" {
				return "", errors.New("http request has no Host")
			}
			return strings.ToLower(host), nil
		}
		if err != nil {
			return "", fmt.Errorf("reading http request head: %w", err)
		}
		if n = br.Buffered(); n == len(b) {
			n++
		}
	}
}

var errStopHandshake = errors.New("stop after ClientHello")

// PeekSNI returns the SNI of the TLS ClientHello at the head of br without
// consuming any bytes. It never completes a handshake.
func PeekSNI(br *bufio.Reader) (string, error) {
	hdr, err := br.Peek(5)
	if err != nil {
		return "", fmt.Errorf("reading tls record header: %w", err)
	}
	if hdr[0] != 0x16 {
		return "", errors.New("not a tls handshake")
	}
	rec, err := br.Peek(5 + int(binary.BigEndian.Uint16(hdr[3:5])))
	if err != nil {
		return "", fmt.Errorf("reading tls ClientHello: %w", err)
	}
	var sni string
	herr := tls.Server(readOnlyConn{r: bytes.NewReader(rec)}, &tls.Config{
		GetConfigForClient: func(h *tls.ClientHelloInfo) (*tls.Config, error) {
			sni = h.ServerName
			return nil, errStopHandshake
		},
	}).Handshake()
	if sni == "" {
		return "", fmt.Errorf("no SNI in ClientHello: %w", herr)
	}
	return strings.ToLower(sni), nil
}

// readOnlyConn feeds recorded bytes to crypto/tls and discards its writes.
type readOnlyConn struct {
	r io.Reader
}

func (c readOnlyConn) Read(p []byte) (int, error)       { return c.r.Read(p) }
func (readOnlyConn) Write(p []byte) (int, error)        { return len(p), nil }
func (readOnlyConn) Close() error                       { return nil }
func (readOnlyConn) LocalAddr() net.Addr                { return nil }
func (readOnlyConn) RemoteAddr() net.Addr               { return nil }
func (readOnlyConn) SetDeadline(time.Time) error        { return nil }
func (readOnlyConn) SetReadDeadline(time.Time) error    { return nil }
func (readOnlyConn) SetWriteDeadline(time.Time) error   { return nil }
```

- [ ] **Step 5: Implement proxy and port listeners**

`internal/privateendpoints/forwarder/proxy.go`:

```go
package forwarder

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"slices"
	"strconv"
	"sync"
	"time"

	"github.com/go-logr/logr"
)

type DialFunc func(ctx context.Context, address string) (net.Conn, error)

// Proxy forwards accepted connections to private endpoints via Dial.
type Proxy struct {
	Table       *Table
	Dial        DialFunc
	Log         logr.Logger
	PeekTimeout time.Duration
}

// ServeShared routes by the name peek extracts (Host header or SNI) for
// endpoints on the given logical port (80 or 443).
func (p *Proxy) ServeShared(ctx context.Context, l net.Listener, port int32, peek func(*bufio.Reader) (string, error)) error {
	return serve(ctx, l, func(c net.Conn) {
		br := bufio.NewReaderSize(c, PeekBufferSize)
		_ = c.SetReadDeadline(time.Now().Add(p.PeekTimeout))
		host, err := peek(br)
		_ = c.SetReadDeadline(time.Time{})
		if err != nil {
			p.Log.V(1).Info("closing connection without a routable name", "port", port, "remote", c.RemoteAddr().String(), "reason", err.Error())
			_ = c.Close()
			return
		}
		target, ok := p.Table.SharedTarget(host, port)
		if !ok {
			p.Log.V(1).Info("closing connection for unknown private endpoint", "host", host, "port", port)
			_ = c.Close()
			return
		}
		p.forward(ctx, c, br, target)
	})
}

// ServePort forwards every connection on a per-endpoint forwarder port.
func (p *Proxy) ServePort(ctx context.Context, l net.Listener, fwdPort int32) error {
	return serve(ctx, l, func(c net.Conn) {
		target, ok := p.Table.PortTarget(fwdPort)
		if !ok {
			_ = c.Close()
			return
		}
		p.forward(ctx, c, c, target)
	})
}

func (p *Proxy) forward(ctx context.Context, client net.Conn, clientR io.Reader, target string) {
	upstream, err := p.Dial(ctx, target)
	if err != nil {
		p.Log.Error(err, "private dial failed", "target", target)
		_ = client.Close()
		return
	}
	pipe(client, clientR, upstream)
}

func serve(ctx context.Context, l net.Listener, handle func(net.Conn)) error {
	stop := context.AfterFunc(ctx, func() { _ = l.Close() })
	defer stop()
	for {
		c, err := l.Accept()
		if err != nil {
			if ctx.Err() != nil || errors.Is(err, net.ErrClosed) {
				return nil
			}
			return err
		}
		go handle(c)
	}
}

// pipe copies both ways. clientR replays any bytes already peeked from client.
func pipe(client net.Conn, clientR io.Reader, upstream net.Conn) {
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		_, _ = io.Copy(upstream, clientR)
		closeWrite(upstream)
	}()
	go func() {
		defer wg.Done()
		_, _ = io.Copy(client, upstream)
		closeWrite(client)
	}()
	wg.Wait()
	_ = client.Close()
	_ = upstream.Close()
}

func closeWrite(c net.Conn) {
	if cw, ok := c.(interface{ CloseWrite() error }); ok {
		_ = cw.CloseWrite()
		return
	}
	_ = c.Close()
}

// PortListeners keeps one listener open per forwarder port in the last Sync.
type PortListeners struct {
	Proxy    *Proxy
	Log      logr.Logger
	BindHost string

	mu sync.Mutex
	ls map[int32]net.Listener
}

// Sync opens listeners for new ports and closes ones no longer wanted. ctx
// bounds the listeners' lifetime and must outlive this call.
func (pl *PortListeners) Sync(ctx context.Context, ports []int32) error {
	pl.mu.Lock()
	defer pl.mu.Unlock()
	if pl.ls == nil {
		pl.ls = map[int32]net.Listener{}
	}
	want := map[int32]bool{}
	var errs []error
	for _, port := range ports {
		want[port] = true
		if _, ok := pl.ls[port]; ok {
			continue
		}
		l, err := net.Listen("tcp", net.JoinHostPort(pl.BindHost, strconv.Itoa(int(port))))
		if err != nil {
			errs = append(errs, fmt.Errorf("listening on forwarder port %d: %w", port, err))
			continue
		}
		pl.ls[port] = l
		go func() {
			if err := pl.Proxy.ServePort(ctx, l, port); err != nil {
				pl.Log.Error(err, "forwarder port listener stopped", "port", port)
			}
		}()
	}
	for port, l := range pl.ls {
		if !want[port] {
			_ = l.Close()
			delete(pl.ls, port)
		}
	}
	return errors.Join(errs...)
}

func (pl *PortListeners) Ports() []int32 {
	pl.mu.Lock()
	defer pl.mu.Unlock()
	var out []int32
	for p := range pl.ls {
		out = append(out, p)
	}
	slices.Sort(out)
	return out
}

func (pl *PortListeners) Close() {
	pl.mu.Lock()
	defer pl.mu.Unlock()
	for port, l := range pl.ls {
		_ = l.Close()
		delete(pl.ls, port)
	}
}
```

- [ ] **Step 6: Run tests**

Run: `go test ./internal/privateendpoints/forwarder/ -race -v`
Expected: PASS, no races.

- [ ] **Step 7: Commit**

```bash
git add internal/privateendpoints/forwarder/peek.go internal/privateendpoints/forwarder/peek_test.go internal/privateendpoints/forwarder/proxy.go internal/privateendpoints/forwarder/proxy_test.go
git commit -m "Route forwarder connections by Host, SNI, or port

http/https/tls endpoints share one IP, so the forwarder reads the Host
header or TLS SNI without terminating anything and replays the bytes to
the endpoint. Other endpoints get their own forwarder port. Idle clients
are dropped after a peek timeout so probes can't pile up goroutines.

Co-Authored-By: Claude Opus 5.5 (1M context) <noreply@anthropic.com>"
```

---

### Task 8: private-endpoint-forwarder subcommand

**Files:**
- Create: `internal/privateendpoints/forwarder/syncer.go`, `internal/privateendpoints/forwarder/syncer_test.go`
- Create: `cmd/private-endpoint-forwarder.go`
- Modify: `go.mod`, `go.sum` (ensure privatedial is a direct dependency)

**Interfaces:**
- Consumes: Tasks 6–7, `ngrokv1.PrivateEndpoint`.
- Produces: `type Syncer struct { client.Client; Namespace string; Table *Table; Ports *PortListeners }` with `Sync(ctx) error`, `Reconcile`, `SetupWithManager(mgr) error`; subcommand `private-endpoint-forwarder` with flags `--dns-bind-address` (`:5353`), `--http-bind-address` (`:8000`), `--https-bind-address` (`:8443`), `--dns-upstream` (default: first nameserver in `/etc/resolv.conf`), `--private-dial-server` (`quic.connect-endpoint.ngrok.com:443`), `--metrics-bind-address` (`:8080`), `--health-probe-bind-address` (`:8081`); env `POD_NAMESPACE`, `NGROK_ACCESS_TOKEN`. Used by Helm in Task 9.

- [ ] **Step 1: Write failing syncer test**

`internal/privateendpoints/forwarder/syncer_test.go`:

```go
package forwarder

import (
	"context"
	"testing"

	"github.com/go-logr/logr"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	ngrokv1 "github.com/ngrok/ngrok-operator/api/ngrok/v1"
)

func TestSyncerSync(t *testing.T) {
	s := runtime.NewScheme()
	require.NoError(t, ngrokv1.AddToScheme(s))
	port := freePort(t)
	cr := func(name, ns, host string, p int32, ip string, fwd int32) *ngrokv1.PrivateEndpoint {
		return &ngrokv1.PrivateEndpoint{
			ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: ns},
			Spec:       ngrokv1.PrivateEndpointSpec{Hostname: host, Port: p},
			Status:     ngrokv1.PrivateEndpointStatus{ClusterIP: ip, ForwarderPort: fwd},
		}
	}
	c := fake.NewClientBuilder().WithScheme(s).WithObjects(
		cr("a", "op", "foo.internal", 80, "10.0.0.1", 0),
		cr("b", "op", "bar.internal", 6379, "10.0.0.2", port),
		cr("c", "op", "pending.internal", 6379, "", 0),
		cr("d", "other", "elsewhere.internal", 80, "10.0.0.9", 0),
	).Build()

	tbl := NewTable()
	pl := &PortListeners{Proxy: &Proxy{Table: tbl, Log: logr.Discard()}, Log: logr.Discard(), BindHost: "127.0.0.1"}
	defer pl.Close()
	sy := &Syncer{Client: c, Namespace: "op", Table: tbl, Ports: pl}

	require.NoError(t, sy.Sync(context.Background()))
	assert.True(t, tbl.Synced())
	ip, ok := tbl.IP("foo.internal")
	assert.True(t, ok)
	assert.Equal(t, "10.0.0.1", ip)
	_, ok = tbl.IP("pending.internal")
	assert.False(t, ok)
	_, ok = tbl.IP("elsewhere.internal")
	assert.False(t, ok, "other namespaces are ignored")
	assert.Equal(t, []int32{port}, pl.Ports())
}
```

- [ ] **Step 2: Run to verify failure**

Run: `go test ./internal/privateendpoints/forwarder/ -run TestSyncer`
Expected: FAIL, `undefined: Syncer`.

- [ ] **Step 3: Implement syncer**

`internal/privateendpoints/forwarder/syncer.go`:

```go
package forwarder

import (
	"context"
	"errors"
	"fmt"

	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/manager"

	ngrokv1 "github.com/ngrok/ngrok-operator/api/ngrok/v1"
)

// +kubebuilder:rbac:groups=ngrok.com,resources=privateendpoints,verbs=get;list;watch

// Syncer rebuilds the routing table and port listeners from all
// PrivateEndpoints on every change.
type Syncer struct {
	client.Client
	Namespace string
	Table     *Table
	Ports     *PortListeners
}

func (s *Syncer) Reconcile(ctx context.Context, _ ctrl.Request) (ctrl.Result, error) {
	return ctrl.Result{}, s.Sync(ctx)
}

func (s *Syncer) Sync(ctx context.Context) error {
	var list ngrokv1.PrivateEndpointList
	if err := s.List(ctx, &list, client.InNamespace(s.Namespace)); err != nil {
		return fmt.Errorf("listing PrivateEndpoints: %w", err)
	}
	entries := make([]Entry, 0, len(list.Items))
	var ports []int32
	for _, cr := range list.Items {
		entries = append(entries, Entry{
			Hostname:      cr.Spec.Hostname,
			Port:          cr.Spec.Port,
			ClusterIP:     cr.Status.ClusterIP,
			ForwarderPort: cr.Status.ForwarderPort,
		})
		if cr.Status.ClusterIP != "" && cr.Status.ForwarderPort != 0 {
			ports = append(ports, cr.Status.ForwarderPort)
		}
	}
	s.Table.Replace(entries)
	return s.Ports.Sync(ctx, ports)
}

// SetupWithManager syncs once after the cache fills (so the table goes
// Synced even with zero PrivateEndpoints), then on every change.
func (s *Syncer) SetupWithManager(mgr ctrl.Manager) error {
	if err := mgr.Add(manager.RunnableFunc(func(ctx context.Context) error {
		if !mgr.GetCache().WaitForCacheSync(ctx) {
			return errors.New("PrivateEndpoint cache did not sync")
		}
		return s.Sync(ctx)
	})); err != nil {
		return err
	}
	return ctrl.NewControllerManagedBy(mgr).
		Named("private-endpoint-forwarder").
		For(&ngrokv1.PrivateEndpoint{}).
		Complete(s)
}
```

Note: listeners opened from `Reconcile` live on the ctx controller-runtime passes to reconcilers, which is the controller's run ctx (canceled at manager shutdown), not a per-request ctx. Don't set `ReconciliationTimeout` on this controller.

- [ ] **Step 4: Run tests**

Run: `go test ./internal/privateendpoints/forwarder/ -race`
Expected: PASS

- [ ] **Step 5: Write subcommand**

`cmd/private-endpoint-forwarder.go` (copy MIT header from another cmd file if present):

```go
package cmd

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"net"
	"net/http"
	"os"
	"time"

	"github.com/miekg/dns"
	"github.com/spf13/cobra"
	"golang.ngrok.com/ngrok/privatedial"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/metrics/server"

	"github.com/ngrok/ngrok-operator/internal/privateendpoints/forwarder"
	"github.com/ngrok/ngrok-operator/internal/version"
)

func init() {
	rootCmd.AddCommand(privateEndpointForwarderCmd())
}

type privateEndpointForwarderOpts struct {
	metricsAddr       string
	probeAddr         string
	dnsAddr           string
	httpAddr          string
	httpsAddr         string
	dnsUpstream       string
	privateDialServer string
	zapOpts           *zap.Options
}

func privateEndpointForwarderCmd() *cobra.Command {
	var opts privateEndpointForwarderOpts
	c := &cobra.Command{
		Use: "private-endpoint-forwarder",
		RunE: func(c *cobra.Command, _ []string) error {
			return runPrivateEndpointForwarder(opts)
		},
	}
	c.Flags().StringVar(&opts.metricsAddr, "metrics-bind-address", ":8080", "The address the metric endpoint binds to")
	c.Flags().StringVar(&opts.probeAddr, "health-probe-bind-address", ":8081", "The address the probe endpoint binds to")
	c.Flags().StringVar(&opts.dnsAddr, "dns-bind-address", ":5353", "UDP and TCP address the DNS server binds to")
	c.Flags().StringVar(&opts.httpAddr, "http-bind-address", ":8000", "Address of the shared listener for http endpoints on port 80")
	c.Flags().StringVar(&opts.httpsAddr, "https-bind-address", ":8443", "Address of the shared listener for https/tls endpoints on port 443")
	c.Flags().StringVar(&opts.dnsUpstream, "dns-upstream", "", "Resolver (host:port) for .internal names that aren't private endpoints. Defaults to the first nameserver in /etc/resolv.conf")
	c.Flags().StringVar(&opts.privateDialServer, "private-dial-server", "quic.connect-endpoint.ngrok.com:443",
		"Private dial gateway (host:port). QUIC is forced: privatedial's HTTP/2 transport panics on Go 1.27, so UDP/443 egress is required.")

	opts.zapOpts = &zap.Options{}
	goFlagSet := flag.NewFlagSet("manager", flag.ContinueOnError)
	opts.zapOpts.BindFlags(goFlagSet)
	c.Flags().AddGoFlagSet(goFlagSet)
	return c
}

func runPrivateEndpointForwarder(opts privateEndpointForwarderOpts) error {
	ctrl.SetLogger(zap.New(zap.UseFlagOptions(opts.zapOpts)))
	log := ctrl.Log.WithName("private-endpoint-forwarder")
	buildInfo := version.Get()
	log.Info("starting private-endpoint-forwarder", "version", buildInfo.Version, "commit", buildInfo.GitCommit)

	namespace, ok := os.LookupEnv("POD_NAMESPACE")
	if !ok {
		return errors.New("POD_NAMESPACE environment variable should be set, but was not")
	}
	token, ok := os.LookupEnv("NGROK_ACCESS_TOKEN")
	if !ok || token == "" {
		return errors.New("NGROK_ACCESS_TOKEN environment variable should be set, but was not")
	}
	upstream := opts.dnsUpstream
	if upstream == "" {
		rc, err := dns.ClientConfigFromFile("/etc/resolv.conf")
		if err != nil || len(rc.Servers) == 0 {
			return fmt.Errorf("finding upstream resolver in /etc/resolv.conf (set --dns-upstream): %w", err)
		}
		upstream = net.JoinHostPort(rc.Servers[0], rc.Port)
	}

	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{
		Scheme:                 scheme, // shared package-level scheme; ngrokv1 is registered by bindings-forwarder-manager's init
		Cache:                  cache.Options{DefaultNamespaces: map[string]cache.Config{namespace: {}}},
		Metrics:                server.Options{BindAddress: opts.metricsAddr},
		HealthProbeBindAddress: opts.probeAddr,
		LeaderElection:         false,
	})
	if err != nil {
		return fmt.Errorf("unable to start private-endpoint-forwarder: %w", err)
	}

	dialer := privatedial.New(privatedial.Config{
		QUICServerAddr: opts.privateDialServer,
		ForceProtocol:  privatedial.ProtocolQUIC,
		AuthToken:      token,
	})
	table := forwarder.NewTable()
	proxy := &forwarder.Proxy{
		Table:       table,
		Log:         log.WithName("proxy"),
		PeekTimeout: 10 * time.Second,
		Dial: func(ctx context.Context, address string) (net.Conn, error) {
			return dialer.DialContext(ctx, "tcp", address)
		},
	}
	ports := &forwarder.PortListeners{Proxy: proxy, Log: log.WithName("ports")}

	if err := (&forwarder.Syncer{Client: mgr.GetClient(), Namespace: namespace, Table: table, Ports: ports}).SetupWithManager(mgr); err != nil {
		return fmt.Errorf("setting up PrivateEndpoint syncer: %w", err)
	}

	dnsHandler := &forwarder.DNSHandler{Table: table, Upstream: upstream, TTL: 5, Exchange: forwarder.UDPExchange}
	for _, network := range []string{"udp", "tcp"} {
		srv := &dns.Server{Addr: opts.dnsAddr, Net: network, Handler: dnsHandler}
		if err := mgr.Add(manager.RunnableFunc(func(ctx context.Context) error {
			errCh := make(chan error, 1)
			go func() { errCh <- srv.ListenAndServe() }()
			select {
			case <-ctx.Done():
				return srv.ShutdownContext(context.Background())
			case err := <-errCh:
				return fmt.Errorf("dns %s server: %w", network, err)
			}
		})); err != nil {
			return err
		}
	}

	for _, l := range []struct {
		addr string
		port int32
		peek func(*bufio.Reader) (string, error)
	}{
		{opts.httpAddr, 80, forwarder.PeekHTTPHost},
		{opts.httpsAddr, 443, forwarder.PeekSNI},
	} {
		if err := mgr.Add(manager.RunnableFunc(func(ctx context.Context) error {
			ln, err := net.Listen("tcp", l.addr)
			if err != nil {
				return fmt.Errorf("listening on %s: %w", l.addr, err)
			}
			return proxy.ServeShared(ctx, ln, l.port, l.peek)
		})); err != nil {
			return err
		}
	}

	if err := mgr.AddReadyzCheck("routing-table", func(*http.Request) error {
		if !table.Synced() {
			return errors.New("routing table not synced")
		}
		return nil
	}); err != nil {
		return fmt.Errorf("error setting up readyz check: %w", err)
	}
	if err := mgr.AddHealthzCheck("healthz", func(*http.Request) error { return nil }); err != nil {
		return fmt.Errorf("error setting up health check: %w", err)
	}

	defer ports.Close()
	log.Info("serving", "dns", opts.dnsAddr, "dnsUpstream", upstream, "http", opts.httpAddr, "https", opts.httpsAddr, "privateDialServer", opts.privateDialServer)
	return mgr.Start(ctrl.SetupSignalHandler())
}
```

Add `"bufio"` to the imports. Confirm `scheme` is the package-level var in `cmd/root.go` and that `ngrokv1.AddToScheme(scheme)` runs in some `init` in package `cmd` (it does in `bindings-forwarder-manager.go`); if not, add `utilruntime.Must(ngrokv1.AddToScheme(scheme))` to this file's `init`.

- [ ] **Step 6: Build and smoke-run**

```bash
go mod tidy
go build ./... && go vet ./cmd/... ./internal/privateendpoints/...
go run . private-endpoint-forwarder --help
```

Expected: build clean; help lists the flags above.

- [ ] **Step 7: Commit**

```bash
git add go.mod go.sum cmd/private-endpoint-forwarder.go internal/privateendpoints/forwarder/syncer.go internal/privateendpoints/forwarder/syncer_test.go
git commit -m "Add private-endpoint-forwarder subcommand

A separate process from the bindings forwarder so both can run side by
side while bindings are deprecated. It needs only read access to
PrivateEndpoints and an ngrok token; all egress is private dial over QUIC.

Co-Authored-By: Claude Opus 5.5 (1M context) <noreply@anthropic.com>"
```

---

### Task 9: Helm chart

**Files:**
- Modify: `helm/ngrok-operator/values.yaml`, `helm/ngrok-operator/templates/_helpers.tpl`, `helm/ngrok-operator/templates/credentials-secret.yaml`
- Create: `helm/ngrok-operator/templates/private-endpoints/deployment.yaml`, `.../rbac.yaml`, `.../services.yaml`
- Create: `helm/ngrok-operator/tests/private-endpoints/private-endpoints_test.yaml`
- Update: snapshots under `helm/ngrok-operator/tests/**/__snapshot__/`

**Interfaces:**
- Consumes: Task 5 flags, Task 8 subcommand/flags/ports, `pe.ForwarderComponent` label value `private-endpoint-forwarder`.
- Produces: Services `<fullname>-private-dns` and `<fullname>-private-endpoints` in the release namespace (Task 10 script reads the former).

- [ ] **Step 1: Write failing helm tests**

`helm/ngrok-operator/tests/private-endpoints/private-endpoints_test.yaml`:

```yaml
suite: test private endpoints
templates:
- private-endpoints/deployment.yaml
- private-endpoints/rbac.yaml
- private-endpoints/services.yaml
- api-manager/deployment.yaml
- credentials-secret.yaml
set:
  credentials.accessToken: tok
tests:
- it: renders nothing when disabled
  templates:
  - private-endpoints/deployment.yaml
  - private-endpoints/rbac.yaml
  - private-endpoints/services.yaml
  asserts:
  - hasDocuments:
      count: 0
- it: forwarder deployment uses node DNS and the private endpoints token
  set:
    privateEndpoints.enabled: true
  template: private-endpoints/deployment.yaml
  asserts:
  - equal:
      path: metadata.name
      value: RELEASE-NAME-ngrok-operator-private-endpoint-forwarder
  - equal:
      path: spec.template.spec.dnsPolicy
      value: Default
  - equal:
      path: spec.template.metadata.labels["app.kubernetes.io/component"]
      value: private-endpoint-forwarder
  - contains:
      path: spec.template.spec.containers[0].args
      content: private-endpoint-forwarder
  - contains:
      path: spec.template.spec.containers[0].env
      content:
        name: NGROK_ACCESS_TOKEN
        valueFrom:
          secretKeyRef:
            key: PRIVATE_ENDPOINTS_ACCESS_TOKEN
            name: RELEASE-NAME-ngrok-operator-credentials
  - matchSnapshot: {}
- it: services map standard ports to forwarder container ports
  set:
    privateEndpoints.enabled: true
  template: private-endpoints/services.yaml
  asserts:
  - hasDocuments:
      count: 2
  - equal:
      path: metadata.name
      value: RELEASE-NAME-ngrok-operator-private-dns
    documentIndex: 0
  - contains:
      path: spec.ports
      content: {name: dns-udp, port: 53, protocol: UDP, targetPort: dns-udp}
    documentIndex: 0
  - equal:
      path: metadata.name
      value: RELEASE-NAME-ngrok-operator-private-endpoints
    documentIndex: 1
  - contains:
      path: spec.ports
      content: {name: https, port: 443, protocol: TCP, targetPort: https}
    documentIndex: 1
- it: api-manager gets the feature flag and shared service name
  set:
    privateEndpoints.enabled: true
  template: api-manager/deployment.yaml
  asserts:
  - contains:
      path: spec.template.spec.containers[0].args
      content: --enable-feature-private-endpoints=true
  - contains:
      path: spec.template.spec.containers[0].args
      content: --private-endpoints-shared-service=RELEASE-NAME-ngrok-operator-private-endpoints
- it: credentials secret carries the private endpoints token, preferring the override
  set:
    privateEndpoints.enabled: true
    credentials.privateEndpoints.accessToken: pe-tok
  template: credentials-secret.yaml
  asserts:
  - equal:
      path: data.PRIVATE_ENDPOINTS_ACCESS_TOKEN
      value: cGUtdG9r
- it: credentials secret falls back to the shared token
  set:
    privateEndpoints.enabled: true
  template: credentials-secret.yaml
  asserts:
  - equal:
      path: data.PRIVATE_ENDPOINTS_ACCESS_TOKEN
      value: dG9r
```

- [ ] **Step 2: Run to verify failure**

Run: `make -C helm/ngrok-operator test`
Expected: FAIL (templates missing).

- [ ] **Step 3: values.yaml**

Under `credentials:` after `apiManager:` add:

```yaml
  privateEndpoints:
    accessToken: ""
```

After the `bindings:` section add:

```yaml
##
## @section Private Endpoints (POC)
##
## @param privateEndpoints.enabled Make ngrok private endpoints (*.internal, *.ngrok.direct) reachable from pods by URL. Requires CoreDNS to forward internal. and ngrok.direct. to the <fullname>-private-dns Service, and UDP/443 egress.
## @param privateEndpoints.forwarder.replicaCount Number of private endpoint forwarder pods
## @param privateEndpoints.forwarder.resources Forwarder container resources
## @param privateEndpoints.forwarder.serviceAccount.create Create a ServiceAccount for the forwarder
## @param privateEndpoints.forwarder.serviceAccount.name ServiceAccount name (generated when empty)
## @param privateEndpoints.forwarder.serviceAccount.annotations ServiceAccount annotations
privateEndpoints:
  enabled: false
  forwarder:
    replicaCount: 1
    resources:
      limits: {}
      requests: {}
    serviceAccount:
      create: true
      name: ""
      annotations: {}
```

- [ ] **Step 4: _helpers.tpl**

In `accessTokenSecretKey`, before the `else` fail branch:

```
{{- else if eq . "privateEndpoints" -}}
PRIVATE_ENDPOINTS_ACCESS_TOKEN
```

In `cliFeatureFlags`, after the bindings block:

```
{{- if .Values.privateEndpoints.enabled }}
- --enable-feature-private-endpoints=true
- --private-endpoints-shared-service={{ include "ngrok-operator.fullname" . }}-private-endpoints
{{- end }}
```

After the bindings forwarder serviceAccountName helper, add:

```
{{/*
Create the name of the private endpoint forwarder service account to use
*/}}
{{- define "ngrok-operator.privateEndpoints.forwarder.serviceAccountName" -}}
{{- if .Values.privateEndpoints.forwarder.serviceAccount.create -}}
{{ default (printf "%s-private-endpoint-forwarder" (include "ngrok-operator.fullname" .)) .Values.privateEndpoints.forwarder.serviceAccount.name }}
{{- else -}}
{{ default "default" .Values.privateEndpoints.forwarder.serviceAccount.name }}
{{- end -}}
{{- end -}}
```

- [ ] **Step 5: credentials-secret.yaml**

After `$apiManagerToken` is computed add:

```
{{- $privateEndpointsToken := include "ngrok-operator.accessTokenFor" (dict "root" $ "component" "privateEndpoints") }}
```

Inside `data:`, after the apiManager key:

```
  {{- if and .Values.privateEndpoints.enabled $privateEndpointsToken }}
  {{ include "ngrok-operator.accessTokenSecretKey" "privateEndpoints" }}: {{ $privateEndpointsToken | b64enc }}
  {{- end }}
```

- [ ] **Step 6: templates/private-endpoints/deployment.yaml**

```yaml
{{- if .Values.privateEndpoints.enabled }}
{{- $component := "private-endpoint-forwarder" }}
{{- $forwarder := .Values.privateEndpoints.forwarder }}
apiVersion: apps/v1
kind: Deployment
metadata:
  labels:
    {{- include "ngrok-operator.labels" . | nindent 4 }}
    app.kubernetes.io/component: {{ $component }}
  name: {{ include "ngrok-operator.fullname" . }}-{{ $component }}
  namespace: {{ .Release.Namespace }}
spec:
  replicas: {{ $forwarder.replicaCount }}
  selector:
    matchLabels:
      {{- include "ngrok-operator.selectorLabels" . | nindent 6 }}
      app.kubernetes.io/component: {{ $component }}
  template:
    metadata:
      annotations:
        {{- if .Values.podAnnotations }}
          {{- toYaml .Values.podAnnotations | nindent 8 }}
        {{- end }}
        prometheus.io/path: /metrics
        prometheus.io/port: '8080'
        prometheus.io/scrape: 'true'
        checksum/credentials: {{ include (print $.Template.BasePath "/credentials-secret.yaml") . | sha256sum }}
      labels:
        {{- include "ngrok-operator.selectorLabels" . | nindent 8 }}
        {{- if .Values.podLabels }}
          {{- toYaml .Values.podLabels | nindent 8 }}
        {{- end }}
        app.kubernetes.io/component: {{ $component }}
    spec:
      # The forwarder is CoreDNS's upstream for internal. and ngrok.direct.;
      # resolving through CoreDNS itself would loop.
      dnsPolicy: Default
      serviceAccountName: {{ include "ngrok-operator.privateEndpoints.forwarder.serviceAccountName" . }}
      {{- if .Values.image.pullSecrets }}
      imagePullSecrets:
        {{- toYaml .Values.image.pullSecrets | nindent 8 }}
      {{- end }}
      containers:
      - name: forwarder
        image: {{ include "ngrok-operator.image" . }}
        imagePullPolicy: {{ .Values.image.pullPolicy }}
        command:
        - /ngrok-operator
        args:
        - private-endpoint-forwarder
        - --zap-log-level={{ .Values.log.level }}
        - --zap-stacktrace-level={{ .Values.log.stacktraceLevel }}
        - --zap-encoder={{ .Values.log.format }}
        - --health-probe-bind-address=:8081
        - --metrics-bind-address=:8080
        - --dns-bind-address=:5353
        - --http-bind-address=:8000
        - --https-bind-address=:8443
        securityContext:
          allowPrivilegeEscalation: false
        ports:
        - {name: dns-udp, containerPort: 5353, protocol: UDP}
        - {name: dns-tcp, containerPort: 5353, protocol: TCP}
        - {name: http, containerPort: 8000, protocol: TCP}
        - {name: https, containerPort: 8443, protocol: TCP}
        env:
        - name: POD_NAMESPACE
          valueFrom:
            fieldRef:
              fieldPath: metadata.namespace
        - name: NGROK_ACCESS_TOKEN
          valueFrom:
            secretKeyRef:
              key: {{ include "ngrok-operator.accessTokenSecretKey" "privateEndpoints" }}
              name: {{ include "ngrok-operator.credentialsSecretName" . }}
        {{- range $key, $value := .Values.extraEnv }}
        - name: {{ $key }}
          value: {{- toYaml $value | nindent 12 }}
        {{- end }}
        livenessProbe:
          httpGet:
            path: /healthz
            port: 8081
          initialDelaySeconds: 15
          periodSeconds: 20
        readinessProbe:
          httpGet:
            path: /readyz
            port: 8081
          initialDelaySeconds: 5
          periodSeconds: 10
        resources:
          {{- toYaml $forwarder.resources | nindent 10 }}
{{- end }}
```

- [ ] **Step 7: templates/private-endpoints/services.yaml**

```yaml
{{- if .Values.privateEndpoints.enabled }}
apiVersion: v1
kind: Service
metadata:
  name: {{ include "ngrok-operator.fullname" . }}-private-dns
  namespace: {{ .Release.Namespace }}
  labels:
    {{- include "ngrok-operator.labels" . | nindent 4 }}
    app.kubernetes.io/component: private-endpoint-forwarder
spec:
  type: ClusterIP
  selector:
    {{- include "ngrok-operator.selectorLabels" . | nindent 4 }}
    app.kubernetes.io/component: private-endpoint-forwarder
  ports:
  - {name: dns-udp, port: 53, protocol: UDP, targetPort: dns-udp}
  - {name: dns-tcp, port: 53, protocol: TCP, targetPort: dns-tcp}
---
apiVersion: v1
kind: Service
metadata:
  name: {{ include "ngrok-operator.fullname" . }}-private-endpoints
  namespace: {{ .Release.Namespace }}
  labels:
    {{- include "ngrok-operator.labels" . | nindent 4 }}
    app.kubernetes.io/component: private-endpoint-forwarder
spec:
  type: ClusterIP
  selector:
    {{- include "ngrok-operator.selectorLabels" . | nindent 4 }}
    app.kubernetes.io/component: private-endpoint-forwarder
  ports:
  - {name: http, port: 80, protocol: TCP, targetPort: http}
  - {name: https, port: 443, protocol: TCP, targetPort: https}
{{- end }}
```

- [ ] **Step 8: templates/private-endpoints/rbac.yaml**

```yaml
{{- if .Values.privateEndpoints.enabled }}
{{- if .Values.privateEndpoints.forwarder.serviceAccount.create }}
apiVersion: v1
kind: ServiceAccount
metadata:
  name: {{ include "ngrok-operator.privateEndpoints.forwarder.serviceAccountName" . }}
  namespace: {{ .Release.Namespace }}
  labels:
    {{- include "ngrok-operator.labels" . | nindent 4 }}
    app.kubernetes.io/component: private-endpoint-forwarder
  {{- with .Values.privateEndpoints.forwarder.serviceAccount.annotations }}
  annotations:
    {{- toYaml . | nindent 4 }}
  {{- end }}
---
{{- end }}
apiVersion: rbac.authorization.k8s.io/v1
kind: Role
metadata:
  name: {{ include "ngrok-operator.fullname" . }}-private-endpoint-forwarder
  namespace: {{ .Release.Namespace }}
rules:
- apiGroups: [ngrok.com]
  resources: [privateendpoints]
  verbs: [get, list, watch]
---
apiVersion: rbac.authorization.k8s.io/v1
kind: RoleBinding
metadata:
  name: {{ include "ngrok-operator.fullname" . }}-private-endpoint-forwarder
  namespace: {{ .Release.Namespace }}
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: Role
  name: {{ include "ngrok-operator.fullname" . }}-private-endpoint-forwarder
subjects:
- kind: ServiceAccount
  name: {{ include "ngrok-operator.privateEndpoints.forwarder.serviceAccountName" . }}
  namespace: {{ .Release.Namespace }}
---
# api-manager: poller writes PrivateEndpoints, controller writes their status
# and the per-hostname Services. Always the release namespace.
apiVersion: rbac.authorization.k8s.io/v1
kind: Role
metadata:
  name: {{ include "ngrok-operator.fullname" . }}-private-endpoints-manager
  namespace: {{ .Release.Namespace }}
rules:
- apiGroups: [ngrok.com]
  resources: [privateendpoints]
  verbs: [get, list, watch, create, delete]
- apiGroups: [ngrok.com]
  resources: [privateendpoints/status]
  verbs: [get, update, patch]
- apiGroups: [""]
  resources: [services]
  verbs: [get, list, watch, create, update, patch, delete]
---
apiVersion: rbac.authorization.k8s.io/v1
kind: RoleBinding
metadata:
  name: {{ include "ngrok-operator.fullname" . }}-private-endpoints-manager
  namespace: {{ .Release.Namespace }}
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: Role
  name: {{ include "ngrok-operator.fullname" . }}-private-endpoints-manager
subjects:
- kind: ServiceAccount
  name: {{ include "ngrok-operator.serviceAccountName" . }}
  namespace: {{ .Release.Namespace }}
{{- end }}
```

- [ ] **Step 9: Run helm tests and refresh snapshots**

```bash
make -C helm/ngrok-operator test
make helm-update-snapshots
git diff --stat helm/ngrok-operator/tests
```

Expected: new suite PASSes; existing snapshots unchanged (the feature is off by default) except the new one.

- [ ] **Step 10: Commit**

```bash
git add helm/ngrok-operator
git commit -m "Deploy the private endpoint forwarder from the chart

Off by default. The forwarder gets its own token key so it can be scoped
separately, and dnsPolicy Default so CoreDNS forwarding to it can't loop.

Co-Authored-By: Claude Opus 5.5 (1M context) <noreply@anthropic.com>"
```

---

### Task 10: kind CoreDNS setup and live e2e

**Files:**
- Create: `scripts/kind-private-endpoints-dns.sh`
- Modify: `tools/make/deploy.mk` (target `deploy_with_private_endpoints`), `tools/make/kind.mk` (target `kind-private-endpoints-dns`)
- Modify: `specs/private-endpoints/findings.md` (e2e results)

**Interfaces:**
- Consumes: Task 9 Service `<fullname>-private-dns`, Task 1 agent commands.

- [ ] **Step 1: Script**

`scripts/kind-private-endpoints-dns.sh`:

```bash
#!/usr/bin/env bash
# Points CoreDNS at the private endpoint forwarder for internal. and
# ngrok.direct. Kind only: real clusters manage CoreDNS differently.
set -euo pipefail

NS="${KUBE_NAMESPACE:-ngrok-operator}"
SVC="${PRIVATE_DNS_SERVICE:-ngrok-operator-private-dns}"

ip="$(kubectl -n "$NS" get svc "$SVC" -o jsonpath='{.spec.clusterIP}')"
if [[ -z "$ip" ]]; then
  echo "Service $NS/$SVC has no ClusterIP; is privateEndpoints.enabled=true deployed?" >&2
  exit 1
fi

corefile="$(kubectl -n kube-system get configmap coredns -o jsonpath='{.data.Corefile}')"
corefile="$(printf '%s\n' "$corefile" | awk '
  /^# ngrok-private-endpoints begin/ {skip=1}
  !skip {print}
  /^# ngrok-private-endpoints end/ {skip=0}
')"
corefile+="
# ngrok-private-endpoints begin
internal:53 {
    forward . ${ip}
}
ngrok.direct:53 {
    forward . ${ip}
}
# ngrok-private-endpoints end
"

kubectl -n kube-system create configmap coredns --from-literal=Corefile="$corefile" \
  --dry-run=client -o yaml | kubectl apply -f -
kubectl -n kube-system rollout restart deployment coredns
kubectl -n kube-system rollout status deployment coredns --timeout=120s
echo "CoreDNS now forwards internal. and ngrok.direct. to $ip"
```

`chmod +x scripts/kind-private-endpoints-dns.sh`

- [ ] **Step 2: Make targets**

In `tools/make/kind.mk`:

```make
.PHONY: kind-private-endpoints-dns
kind-private-endpoints-dns: ## Point kind's CoreDNS at the private endpoint forwarder.
	KUBE_NAMESPACE=$(KUBE_NAMESPACE) PRIVATE_DNS_SERVICE=$(HELM_RELEASE_NAME)-private-dns ./scripts/kind-private-endpoints-dns.sh
```

In `tools/make/deploy.mk`, after `deploy_with_bindings`, a copy of it named `deploy_with_private_endpoints` with `--set bindings.enabled=true` replaced by `--set privateEndpoints.enabled=true`, help text `## Deploy with private endpoints enabled to the K8s cluster specified in ~/.kube/config.`

Check that `$(HELM_RELEASE_NAME)` yields fullname `$(HELM_RELEASE_NAME)` (true when the release name contains the chart name, e.g. `ngrok-operator`); if not, pass the Service name explicitly.

- [ ] **Step 3: Idempotency check of the script**

```bash
make kind-create   # if no cluster
NGROK_ACCESS_TOKEN=<PAT> make deploy_with_private_endpoints
make kind-private-endpoints-dns
make kind-private-endpoints-dns
kubectl -n kube-system get configmap coredns -o jsonpath='{.data.Corefile}' | grep -c 'ngrok-private-endpoints begin'
```

Expected: `1` (running twice leaves one block). Forwarder pod Ready; `kubectl -n ngrok-operator get privateendpoints` works.

- [ ] **Step 4: Start endpoints outside the cluster** (use Task 1's working syntax; `$U` unique)

```bash
python3 -m http.server 8080
ngrok http 8080 --url http://pe-web-$U.internal
ngrok http 8080 --url https://pe-web-$U.ngrok.direct
docker run --rm -p 6379:6379 redis:7
ngrok tcp 6379 --url tcp://pe-redis-$U.internal:6379
openssl req -x509 -newkey rsa:2048 -nodes -keyout /tmp/k.pem -out /tmp/c.pem -subj /CN=pe-tls-$U.internal -days 1
openssl s_server -accept 9443 -cert /tmp/c.pem -key /tmp/k.pem -www
ngrok tls 9443 --url tls://pe-tls-$U.internal
```

Wait ≤10s, then: `kubectl -n ngrok-operator get privateendpoints` — expected 4 rows, all Ready=True with ClusterIPs.

- [ ] **Step 5: Run checks from a client pod**

```bash
kubectl run pe-client --rm -it --image=nicolaka/netshoot --restart=Never -- bash
# inside:
curl -sS -o /dev/null -w '%{http_code}\n' http://pe-web-$U.internal/                  # 200
curl -sS -o /dev/null -w '%{http_code}\n' https://pe-web-$U.ngrok.direct/             # 200, cert verified
openssl s_client -connect pe-tls-$U.internal:443 -servername pe-tls-$U.internal </dev/null 2>/dev/null | grep 'subject='   # CN=pe-tls-$U.internal (end-to-end TLS)
printf 'PING\r\n' | nc -w2 pe-redis-$U.internal 6379                                 # +PONG
dig +short nope-$U.ngrok.direct; dig nope-$U.ngrok.direct | grep status              # NXDOMAIN
dig +short metadata.google.internal; dig metadata.google.internal | grep status      # NXDOMAIN from upstream (kind), not SERVFAIL
dig +short kubernetes.default.svc.cluster.local                                       # still resolves
```

- [ ] **Step 6: Cleanup check**

Stop the `ngrok tcp` agent. Within ~15s (one poll + reconcile): `kubectl -n ngrok-operator get privateendpoints` no longer lists the redis URL, `kubectl -n ngrok-operator get svc -l ngrok.com/managed-by=private-endpoint-poller` no longer lists its host Service, and `dig +short pe-redis-$U.internal` from the client pod returns nothing.

- [ ] **Step 7: Record and commit**

Append `## E2E (Task 10)` to `specs/private-endpoints/findings.md`: kind/k8s versions, each command with actual output, any failures with the shortest decisive log line from `kubectl -n ngrok-operator logs deploy/ngrok-operator-private-endpoint-forwarder`, and the `watchNamespace` limitation from Task 5.

```bash
git add scripts/kind-private-endpoints-dns.sh tools/make/kind.mk tools/make/deploy.mk specs/private-endpoints/findings.md
git commit -m "Add kind CoreDNS setup and record private endpoints e2e

CoreDNS config differs per provider, so v1 wires it by hand on kind only;
the e2e results show which schemes work end to end over private dial.

Co-Authored-By: Claude Opus 5.5 (1M context) <noreply@anthropic.com>"
```

---

### Task 11: Full verification

- [ ] **Step 1: Generators clean**

```bash
make manifests generate
git status --porcelain
```

Expected: empty (everything generated is committed).

- [ ] **Step 2: Tests, lint, helm**

```bash
make test
make lint
make -C helm/ngrok-operator test
```

Expected: all PASS, including existing bindings tests. Fix and commit anything that fails; do not skip.

- [ ] **Step 3: Report**

Summarize for the user: tasks done, findings.md highlights (https-over-private-dial result, bindings values seen, any e2e failures), deviations from design.md, and the follow-ups still open (GAT-475 pod identity spike, provider DNS spike).
