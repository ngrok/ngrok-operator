# Private Endpoints in Kubernetes — design

Status: draft (POC) · Owner: Alex Bezek · Date: 2026-09-29

## Goal

Make ngrok private endpoints (`*.ngrok.direct`, and `*.internal` where possible) directly
addressable from any pod in a cluster running the operator, by their real URL, over
HTTP, HTTPS, TLS, and raw TCP. No `service.namespace` URL convention, no `kubernetes`
binding, no per-endpoint Kubernetes configuration.

This replaces Kubernetes bindings (`BoundEndpoint`) as the in-cluster private access story.
The new path runs alongside the old one so users can migrate; bindings are deprecated, not
removed, by this work.

Direction set in the 2026-09-17 Private Endpoints discussion (Notion) — DNS-based private
dial, HTTP demuxed by host, TCP via per-endpoint Services, kubernetes bindings not carried
forward. Background and earlier options: `specs/private-dial/` on branch
`alex/private-dial-poc` (esp. 07, 09).

## Scope

In:
- `PrivateEndpoint` CRD (`ngrok.com/v1`) mirroring the account's private endpoints.
- Poller in api-manager that lists internal-binding endpoints and reconciles CRs.
- Controller in api-manager that gives tcp endpoints a ClusterIP Service.
- New `private-endpoint-forwarder` subcommand + Deployment: DNS server, host/SNI-demuxing
  shared listener, per-port tcp listeners, all egress over private dial.
- Helm wiring behind `privateEndpoints.enabled`.
- kind-only CoreDNS setup (script / make target).

Out (deferred, hand-waved in the POC):
- Endpoint selectors / access control. Every private endpoint in the account is reachable
  from the cluster. Known, accepted temporary regression vs bindings.
- Pod identity on the dial (`conn.k8s.pod.*`, GAT-475). Revisit at the end of the POC.
- DNS integration for real providers (EKS, AKS, GKE, IaC-managed CoreDNS). Follow-up spike.
- Fixing the `privatedial` H2 transport panic on Go 1.27. We force QUIC.
- Scale beyond low hundreds of tcp endpoints per cluster (one Service per tcp endpoint).

## Background facts this relies on

- A private endpoint has exactly one URL, under `.internal` or `.ngrok.direct`.
  `.ngrok.direct` exists because browsers reject the cert for `.internal`; ngrok owns it
  and serves real certs. `.internal` https clients will need to skip verification.
- Private dial (`golang.ngrok.com/ngrok/privatedial`) gives a raw byte stream to an
  endpoint given `host:port`, authenticated with a PAT. The operator is already PAT-only.
- For `https://` endpoints ngrok terminates TLS (Traffic Policy runs at L7), so the client's
  TLS session ends at ngrok with the real cert. For `tls://` endpoints TLS is end-to-end.
  Either way the forwarder never decrypts; it peeks SNI and copies bytes.
- `http://` carries the name in the `Host` header. Raw `tcp://` carries no name, so each
  tcp endpoint needs its own destination IP — a ClusterIP Service.
- ngrok-api-go v9 `Endpoints.List` takes a CEL `Filter`.

## Architecture

```
ngrok API ──poll(Endpoints.List, internal binding)──▶ PrivateEndpoint CRs (operator ns)
                                                                 │
               ┌─────────────────────────────────────────────────┤
               ▼                                                 ▼
 controller (api-manager): tcp CR → port + ClusterIP Svc   forwarder pods (informer on CRs)
                                                             ├─ DNS :53  (internal., ngrok.direct.)
 pod ─DNS─▶ CoreDNS ─forward─▶ ngrok-private-dns Svc ────────┘   http/https/tls → shared Svc IP
 pod ─conn─▶ shared Svc :80/:443 ─▶ Host/SNI peek ─┐              tcp → CR.status.clusterIP
 pod ─conn─▶ per-tcp Svc :port ──▶ port→endpoint ──┴─▶ privatedial.DialContext(host:port) ─▶ ngrok
```

## Components

### 1. `PrivateEndpoint` CRD — `ngrok.com/v1`, namespaced (operator namespace)

One CR per unique endpoint URL. Pooled endpoints share a URL and collapse into one CR.

```yaml
apiVersion: ngrok.com/v1
kind: PrivateEndpoint
metadata:
  name: pe-<hash(url)>
  namespace: ngrok-operator
  labels:
    ngrok.com/managed-by: private-endpoint-poller
spec:
  url: tcp://bar.internal:6379
  scheme: tcp            # http | https | tls | tcp
  hostname: bar.internal
  port: 6379
status:
  clusterIP: 10.96.12.34 # tcp only
  forwarderPort: 20001   # tcp only
  conditions: [...]      # Ready
```

Printer columns: URL, Scheme, ClusterIP, Ready, Age. Users don't author these; they're a
read-only mirror of the account plus the in-cluster wiring.

### 2. Poller (api-manager)

- On an interval (default 10s), `Endpoints.List` with a filter for the internal binding
  (exact CEL confirmed during implementation), paginated.
- Normalize to unique URLs, compute desired CR set, create/update/delete to converge.
- Only touches CRs carrying the managed-by label. Never deletes anything else.
- API errors: log, keep the current CR set, retry next tick. No mass deletion on a failed
  or empty-by-error list.
- Runs under leader election alongside other api-manager controllers.

### 3. PrivateEndpoint controller (api-manager)

- `http`/`https`/`tls`: nothing to create; mark Ready.
- `tcp`: allocate a forwarder port (reuse the `port_allocator` approach, rebuilt from CR
  status on startup), create a ClusterIP Service in the operator namespace
  (`port: spec.port` → `targetPort: forwarderPort`, selector = forwarder pods), write
  `status.clusterIP` / `status.forwarderPort`, mark Ready.
- Finalizer deletes the Service and releases the port.
- Uses `BaseController` helpers and status subresource per repo rules.

### 4. `private-endpoint-forwarder` (new subcommand + Deployment)

Watches `PrivateEndpoint` CRs (informer, operator namespace only) and serves:

- **DNS server** (UDP+TCP :53, `miekg/dns`) authoritative for `internal.` and
  `ngrok.direct.`:
  - Known http/https/tls host → A = shared forwarder Service ClusterIP.
  - Known tcp host → A = `status.clusterIP`.
  - Unknown `ngrok.direct.` name → NXDOMAIN.
  - Unknown `internal.` name → forward to the pod's upstream resolver (GKE's own
    `*.internal` names keep working).
  - AAAA → empty NOERROR for known names. TTL 5s.
  - Before the informer syncs → SERVFAIL.
- **Shared listener** (:80, :443, behind Service `ngrok-private-endpoints`):
  - :80 — read the HTTP request head to get `Host`, look up the endpoint, dial, replay the
    buffered bytes, then copy both ways.
  - :443 — peek the TLS ClientHello for SNI, look up (https or tls endpoint on 443), dial,
    replay, copy. No termination.
  - Unknown host / no SNI → close.
- **tcp listeners**: one per `forwarderPort` in CR status; accepted conn → dial
  `hostname:port`, copy.
- **Egress**: one `privatedial.Dialer` per process, PAT from the existing credentials secret
  (new per-component key, falling back to `credentials.accessToken`), `ProtocolQUIC` forced.
- Dial failure → close the client conn, log with endpoint URL.

### 5. Helm

- `privateEndpoints.enabled` (default false).
- Deployment, ServiceAccount, RBAC (read `privateendpoints`), Services
  `ngrok-private-dns` (53/udp, 53/tcp) and `ngrok-private-endpoints` (80, 443).
- api-manager gets RBAC for `privateendpoints` (+status, finalizers) and Services in its
  namespace; poller/controller enabled by a flag wired from the same value.
- CRD in the CRDs chart.

### 6. kind CoreDNS setup

Script (`scripts/kind-private-endpoints-dns.sh`, exposed via make) appends to the Corefile:

```
internal:53 {
    forward . <ngrok-private-dns ClusterIP>
}
ngrok.direct:53 {
    forward . <ngrok-private-dns ClusterIP>
}
```

and restarts CoreDNS. Documented as the manual v1 step; automatic CoreDNS detection is v2.

## Coexistence with bindings

Separate CRD, controllers, Deployment, and listeners. Both can be enabled at once. An
endpoint bound as `kubernetes` is not an internal-binding endpoint, so it is never picked up
here and vice versa.

## Risks / open items

- Exact Endpoints.List filter expression for the internal binding.
- Whether private dial to `host:443` for an https endpoint behaves as assumed (TLS
  terminated by ngrok with the endpoint's cert). Validated first in the e2e.
- QUIC-only egress requires UDP/443 out of the cluster.
- Pod identity regression (GAT-475) — policies on `conn.k8s.pod.*` won't match.
- Shared :80/:443 means an http and a tls endpoint can't both claim port 80/443 on the same
  hostname ambiguously; URL uniqueness should prevent this, confirm.

## Verification

- Unit (table-driven):
  - DNS answer logic (known/unknown per zone, scheme→IP, pre-sync SERVFAIL).
  - Host parsing and SNI peek, including replay of peeked bytes.
  - Poller diff: dedupe pooled endpoints, create/update/delete, error → no deletes.
  - Port allocation and rebuild from status.
- envtest: controller creates/deletes Service, status populated, finalizer cleanup.
- Live e2e in kind with endpoints started outside the cluster:
  - `curl http://foo.internal`
  - `curl https://foo.ngrok.direct` with cert verification on
  - a `tls://` endpoint via `openssl s_client -servername`
  - `redis-cli -h bar.internal -p 6379 PING` → `PONG`
  - unknown `x.ngrok.direct` → NXDOMAIN; non-ngrok `.internal` name falls through
  - stop an endpoint → DNS record and Service gone within one poll cycle
- `make manifests generate test` clean; existing bindings tests unaffected.

## Implementation deviations

- `ngrok-api-go` v9 `endpoints.Client.List` takes `*ngrok.Paging`, which has no `Filter`. Filtering is client-side (hostname suffix + not `kubernetes`-bound).
- Controller is keyed by hostname, not by CR, and owns one Service per hostname that needs one. Reason: DNS can return one IP per name, so every endpoint on a hostname must share an IP. A hostname goes "dedicated" if any of its endpoints is not http:80 / https:443 / tls:443; then all its endpoints (including http:80) go through per-port forwarder listeners. No finalizer: Services are deleted when the last CR for the hostname disappears.
- `status.clusterIP` is set for every Ready CR (shared Service IP or the hostname Service IP); `status.forwarderPort` only for dedicated ones. The forwarder needs no Service RBAC.
