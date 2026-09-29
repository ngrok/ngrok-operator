# Private Endpoints in Kubernetes — design

Status: draft (POC), revision 2 · Owner: Alex Bezek · Date: 2026-09-29

Revision 2 replaced the forwarder's own DNS server and Host/SNI routing with CoreDNS
rewrites onto one Service per hostname. See "Why no DNS server" below.

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
- Controller in api-manager that gives every private endpoint hostname a Service.
- New `private-endpoint-forwarder` subcommand + Deployment: one listener per endpoint,
  all egress over private dial.
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
  Either way the forwarder never decrypts; it only copies bytes.
- Private dial matches `(host, port)` to an endpoint server-side, so the client only needs to
  know which hostname a connection is for. One IP per hostname plus the destination port is
  enough; nothing has to read the Host header or SNI. Agent v4 uses the same model (a
  synthetic IP per hostname on a TUN device, port passed through).

## Architecture

```
ngrok API ──poll(Endpoints.List)──▶ PrivateEndpoint CRs (operator ns)
                                           │
              ┌────────────────────────────┴───────────────────────────┐
              ▼                                                        ▼
 controller (api-manager):                                forwarder pods (informer on CRs):
 Service foo-internal per hostname,                       listener per forwarderPort,
 port per endpoint → forwarderPort                        forwarderPort → host:port

 pod ─DNS foo.internal─▶ CoreDNS rewrite ─▶ foo-internal.<ns>.svc.cluster.local ─▶ ClusterIP
 pod ─conn ClusterIP:6379─▶ Service ─▶ forwarder :forwarderPort ─▶ privatedial.DialContext(foo.internal:6379)
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

- Keyed by hostname, since every endpoint on a hostname must share the one IP DNS returns.
- Each hostname gets a ClusterIP Service in the operator namespace named after it:
  `foo.internal` → `foo-internal`, `foo.ngrok.direct` → `foo-ngrok-direct`
  (`ServiceName` in `internal/privateendpoints/naming.go`). The suffix keeps the same label
  under the two TLDs from colliding.
- One Service port per endpoint: `port: spec.port` → `targetPort: forwarderPort`, selecting
  the forwarder pods. Forwarder ports are allocated from 20000–20999, reading PrivateEndpoints
  uncached so back-to-back reconciles can't hand out the same port.
- Writes `status.clusterIP` / `status.forwarderPort`, marks Ready.
- Hostnames that can't be a Service name → `Ready=False`, reason `UnsupportedHostname`.
- When the last endpoint for a hostname goes away its Service is deleted (found by label; no
  finalizer).

### 4. `private-endpoint-forwarder` (new subcommand + Deployment)

- Watches `PrivateEndpoint` CRs (operator namespace only) and keeps one TCP listener open per
  Ready CR's `status.forwarderPort`.
- Each accepted connection → `privatedial.DialContext(ctx, "tcp", hostname:port)` → copy both
  ways with half-close; once one direction finishes, the other gets 5s before both close
  (same as agent v4).
- One `privatedial.Dialer` per process, PAT from the credentials secret (key
  `PRIVATE_ENDPOINTS_ACCESS_TOKEN`, falling back to `credentials.accessToken`),
  `ProtocolQUIC` forced. The library bounds each dial at 5s.
- No DNS server, no Host/SNI parsing, no TLS handling: bytes only.

### 5. Helm

- `privateEndpoints.enabled` (default false).
- Forwarder Deployment, ServiceAccount, RBAC (read `privateendpoints`).
- api-manager gets `--enable-feature-private-endpoints` and a Role in the release namespace for
  `privateendpoints` (+status) and Services.
- CRD in the CRDs chart.

### 6. kind CoreDNS setup

`make kind-private-endpoints-dns` (`scripts/kind-private-endpoints-dns.sh`) adds to the main
`.:53` server block:

```
rewrite stop name regex ^([a-z][a-z0-9-]*)\.internal\.$ {1}-internal.<ns>.svc.cluster.local. answer auto
rewrite stop name regex ^([a-z][a-z0-9-]*)\.ngrok\.direct\.$ {1}-ngrok-direct.<ns>.svc.cluster.local. answer auto
```

and restarts CoreDNS. `answer auto` makes replies carry the queried name. Only single-label
names match, so `metadata.google.internal` and other multi-label `.internal` names keep
resolving upstream. Unknown single-label names get NXDOMAIN. Documented as the manual v1 step;
provider-specific setup is a follow-up spike.

## Why no DNS server

Revision 1 ran a DNS server in the forwarder (authoritative for `internal.` / `ngrok.direct.`),
shared :80/:443 listeners that routed by Host header / SNI, and per-host Services only for
non-http endpoints. Kubernetes already gives every Service an IP and a DNS name, so a CoreDNS
rewrite onto a Service per hostname does the same job with far less of our own networking code,
and matches how agent v4 and private dial are meant to be used. Our own DNS server (answering
from `privatedial.Dialer.GetHost`) stays the fallback if the caveats below become blockers.

## Coexistence with bindings

Separate CRD, controllers, Deployment, and listeners. Both can be enabled at once. An
endpoint bound as `kubernetes` is not an internal-binding endpoint, so it is never picked up
here and vice versa.

## Caveats / open items

- **Single-label hostnames only.** `api.foo.internal` can't be mapped to a Service name, so it
  is marked `UnsupportedHostname`. Lifting this needs our own DNS server.
- **Hostname label rules.** The label must be a valid DNS-1035 label (lowercase letters,
  digits, `-`, starting with a letter), and at most 50 characters for `.ngrok.direct` / 54 for
  `.internal`, so the Service name stays within 63.
- **One Service per hostname** and one forwarder port per endpoint (1000 ports). Fine for low
  hundreds of endpoints.
- **We take over every single-label `.internal` name** in the cluster. Clusters already using
  `foo.internal` names for something else collide; long term, document this and steer those
  users to `.ngrok.direct`.
- **Connections to a removed port** on a still-existing hostname time out rather than being
  refused (the Service no longer has that port).
- **QUIC-only egress** requires UDP/443 out of the cluster.
- **Pod identity** (GAT-475): `privatedial.Config.Metadata` is per session, and the server
  doesn't expose it to Traffic Policy yet, so `conn.k8s.pod.*`-style policies don't match.
- **`privatedial` isn't on ngrok-go main.** We pin `e2b70148` (same as the monorepo); agent v4
  pins `cc6f75f` on the `privatedial` branch.
- **Binding rename.** The API will report `bindings: ["private"]` instead of `internal`. The
  poller filters on hostname suffix, so it's unaffected.

## Verification

- Unit (table-driven): hostname → Service name mapping, poller diff (dedupe pooled
  endpoints, create/delete, error → no deletes), forwarder table, port listeners, drain.
- envtest: controller creates/updates/deletes per-hostname Services, allocates unique ports
  (including under cache lag), rejects unsupported hostnames.
- Live e2e in kind with endpoints started outside the cluster:
  - `curl http://foo.internal`
  - `curl https://foo.ngrok.direct` with cert verification on
  - a `tls://` endpoint via `openssl s_client -servername`
  - `redis-cli -h bar.internal -p 6379 PING` → `PONG`
  - unknown single-label name → NXDOMAIN; multi-label `.internal` name falls through
  - stop an endpoint → DNS record and Service gone within one poll cycle
- `make manifests generate test` clean; existing bindings tests unaffected.

## Implementation deviations

- `ngrok-api-go` v9 `endpoints.Client.List` takes `*ngrok.Paging`, which has no `Filter`.
  Filtering is client-side (hostname suffix + not `kubernetes`-bound).
- Controller is keyed by hostname, not by CR. No finalizer: a hostname's Service is deleted
  when its last CR disappears.
