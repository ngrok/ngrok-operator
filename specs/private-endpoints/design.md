# Private Endpoints in Kubernetes — design

Status: draft (POC), revision 2 · Owner: Alex Bezek · Updated: 2026-10-21

Revision 2 replaced the forwarder's own DNS server and Host/SNI routing with CoreDNS
rewrites onto one Service per hostname. See "Why no DNS server" below.

Related: [requirements.md](requirements.md) (what we promise, how each is verified, gaps),
[README.md](README.md) (tracker: decisions D1–D10, risks R1–R19), [strategy.md](strategy.md)
(rationale, comparison with bindings), [notes.md](notes.md) (knowledge base).

## Goal

Make ngrok private endpoints directly addressable from any pod in a cluster running the
operator, by their real URL, over HTTP, HTTPS, TLS, and raw TCP. No `service.namespace` URL
convention, no `kubernetes` binding, no per-endpoint Kubernetes configuration.

Only `*.ngrok.direct` is mirrored. `*.internal` is not supported (D9).

This replaces Kubernetes bindings (`BoundEndpoint`) as the in-cluster private access story.
The new path runs alongside the old one so users can migrate; bindings are deprecated, not
removed, by this work.

Direction set in the 2026-09-17 Private Endpoints discussion (Notion): DNS-based private
dial, HTTP demuxed by host, TCP via per-endpoint Services, kubernetes bindings not carried
forward. Background: `specs/private-dial/` on branch `alex/private-dial-poc` (esp. 07, 09).

## Scope

In:
- `PrivateEndpoint` CRD (`ngrok.com/v1`) mirroring the account's private endpoints.
- Poller in api-manager that lists endpoints and reconciles CRs.
- Controller in api-manager that gives every private endpoint hostname a Service.
- `private-endpoint-forwarder-manager` subcommand + Deployment: one listener per endpoint, all egress
  over private dial.
- Helm wiring behind `ngrok.features.privateEndpoints.enabled`.
- kind-only CoreDNS helper and a canned kind demo ([demo.md](demo.md)).

Out for the POC (full list with tracking in [requirements.md §2](requirements.md#2-poc-scope)):
access control, pod identity on the dial (GAT-475), operator-managed DNS (D4), real-platform
DNS beyond runbooks (D7), multi-label hostnames, IPv6, more than ~1000 endpoint-ports, HTTP/2
fallback.

## Background facts this relies on

- A private endpoint has exactly one URL, under `.internal` or `.ngrok.direct`.
  `.ngrok.direct` exists because browsers reject certs for `.internal`; ngrok owns it and
  serves a real `*.ngrok.direct` cert. `.internal` https clients must skip verification.
  `.ngrok.direct` is the prescribed pattern going forward (PM, Slack 2026-10-12), and the
  only one the operator mirrors (D9).
- Private dial (`golang.ngrok.com/ngrok/privatedial`) gives a raw byte stream to an endpoint
  given `host:port`, authenticated with a PAT. The operator is already PAT-only.
- For `https://` endpoints ngrok terminates TLS (Traffic Policy runs at L7). For `tls://`
  endpoints TLS is end-to-end. Either way the forwarder never decrypts.
- Private dial matches `(host, port)` to an endpoint server-side, so the client only needs to
  know which hostname a connection is for. One IP per hostname plus the destination port is
  enough; nothing reads the Host header or SNI. Agent v4 uses the same model.
- ngrok-api-go v9 `Endpoints.List` has no server-side filter, and `Scheme`/`Host`/`Port` come
  back empty, so everything is parsed from `URL` ([findings.md](findings.md)).

## Architecture

```
ngrok API ──poll(Endpoints.List)──▶ PrivateEndpoint CRs (operator ns)
                                           │
              ┌────────────────────────────┴───────────────────────────┐
              ▼                                                        ▼
 controller (api-manager):                                forwarder pods (informer on CRs):
 Service foo-ngrok-direct per hostname,                   listener per forwarderPort,
 port per endpoint → forwarderPort                        forwarderPort → host:port

 pod ─DNS foo.ngrok.direct─▶ CoreDNS rewrite ─▶ foo-ngrok-direct.<ns>.svc.cluster.local ─▶ ClusterIP
 pod ─conn ClusterIP:443─▶ Service ─▶ forwarder :forwarderPort ─▶ privatedial.DialContext(foo.ngrok.direct:443)
```

## Components

### 1. `PrivateEndpoint` CRD — `ngrok.com/v1`, namespaced (operator namespace)

`api/ngrok/v1/privateendpoint_types.go`. One CR per unique endpoint URL; pooled endpoints
share a URL and collapse into one CR. Users aren't meant to author these, but nothing enforces
it: the controller and forwarder wire any CR in the namespace and trust its host label (PR-7,
GAP-31).

```yaml
apiVersion: ngrok.com/v1
kind: PrivateEndpoint
metadata:
  name: pe-<sha256(url)[:16]>
  namespace: ngrok-operator
  labels:
    ngrok.com/managed-by: private-endpoint-poller
    ngrok.com/private-endpoint-host: <sha256(hostname)[:16]>
spec:
  url: tcp://bar.ngrok.direct:6379
  scheme: tcp            # http | https | tls | tcp
  hostname: bar.ngrok.direct
  port: 6379             # defaulted: http 80, https/tls 443; tcp needs an explicit port
status:
  clusterIP: 10.96.12.34 # the hostname's Service
  forwarderPort: 20001   # this endpoint's forwarder port
  conditions: [...]      # Ready
```

Every scheme gets its own `forwarderPort` and the hostname's `clusterIP`. The forwarder
copies bytes and never inspects them, whatever the scheme (R17 corrected the rev-1 wording).

Printer columns: URL, Scheme, ClusterIP, Ready, Age.

### 2. Poller (api-manager)

`internal/controller/privateendpoints/privateendpoint_poller.go`. A manager runnable, so leader-only.

Every 10s (fixed, `cmd/api-manager.go`):

1. `Endpoints.List`, paginated. On error, change nothing (R-6).
2. Keep endpoints that parse, are under `.ngrok.direct`, and aren't `kubernetes`-bound. Filtering is by hostname suffix, so the
   expected binding rename (`internal` → `private`) doesn't affect it.
3. Diff against CRs carrying the managed-by label:
   - create missing ones;
   - **heal drift**: restore spec and poller-owned labels if edited; leave other labels and
     status alone (`heal`);
   - delete labeled CRs whose endpoint is gone (or is no longer mirrored, e.g. a `.internal`
     CR left by an older build);
   - a same-named CR without the label is logged and left alone, never adopted.

Healing needs `update` on `privateendpoints`; the Helm Role and kubebuilder marker carry it
(R19).

### 3. PrivateEndpoint controller (api-manager)

`internal/controller/privateendpoints/privateendpoint_controller.go`. **Keyed by hostname**, since every
endpoint on a hostname must share the one IP DNS returns. The request name is
`HostKey(hostname)`; watches on CRs and labeled Services map through the host label.
`MaxConcurrentReconciles: 1` because port allocation assumes one worker.

Per hostname:

1. List all CRs **uncached** (`APIReader`) so back-to-back reconciles can't hand out the same
   forwarder port from a stale cache.
2. Service name from `ServiceName(hostname)` (`internal/privateendpoints/naming.go`):
   `foo.ngrok.direct` → `foo-ngrok-direct`. Unmappable hostnames →
   `UnsupportedHostname`, and any Service for the host is deleted.
3. One Service port per endpoint: `port: spec.port` → `targetPort: forwarderPort`, selecting
   forwarder pods. A second endpoint on the same hostname and port → `PortConflict`.
4. Forwarder ports come from 20000–20999 (hard-coded). `allocate` uses a **cursor**: next
   free port after the last one handed out, wrapping, so a freed port is reused only after
   the range cycles (R2 narrowing). The cursor is in memory and reseeded from the highest
   port in use after restart (R12). If two hostnames claim one port, the lowest CR name keeps
   it. No free port → `PortsExhausted`, recheck in 30s.
5. `CreateOrUpdate` the Service. **Ownership check**: an existing Service without this
   controller's managed-by and host labels is left untouched, its CRs get `ServiceConflict`,
   recheck in 30s (the Service watch only maps labeled Services, so nothing else would
   enqueue us).
6. Write `status.clusterIP`, `status.forwarderPort`, `Ready=True`. No ClusterIP yet →
   `ServiceNotReady`, recheck in 2s.
7. When the last CR for a hostname goes away, delete its Services by label. No finalizer, no
   ownerRef.

`Ready` reasons: `Ready`, `UnsupportedHostname`, `PortConflict`, `PortsExhausted`,
`ServiceConflict`, `ServiceNotReady`. `Ready=True` means **configured**, not that a forwarder
bound the port, DNS is set up, or the endpoint is reachable.

### 4. Forwarder (`private-endpoint-forwarder-manager` subcommand + Deployment)

`cmd/private-endpoint-forwarder-manager.go`, `internal/controller/privateendpoints/forwarder_controller.go`
(`ForwarderReconciler`), `internal/privateendpoints/forwarder/` (table, dial/copy helper).

- Watches CRs in its own namespace (namespace-scoped cache). No leader election: every
  replica serves every port.
- On each change, `ForwarderReconciler.Sync` rebuilds a `forwarderPort → host:port` table from CRs with
  both `clusterIP` and `forwarderPort` set, then opens and closes listeners to match.
  Listeners come from `pkg/bindingsdriver` (binds `0.0.0.0`).
- The target is looked up **at accept time** (`ForwarderReconciler.handler`), so a reassigned port routes
  to its new endpoint without reopening the listener. Unknown port → close.
- Each connection → `privatedial.DialContext(ctx, "tcp", host:port)` → copy both ways with
  half-close; once either direction ends the other gets 5s, then both close (same as agent v4).
  That includes a client half-close, so a response slower than 5s after it is cut (R8, PR-2).
  This code is isolated in `privatedial_forward.go` to move upstream ([sdk-upstream.md](sdk-upstream.md)).
- One `privatedial.Dialer` per process, `ProtocolQUIC` forced (HTTP/2 transport failed on Go
  1.27), so UDP/443 egress is required. PAT from `NGROK_ACCESS_TOKEN`.
- **Readiness** (`/readyz`): table synced at least once **and** the last sync had no listener
  bind errors (R5). A bind failure at startup fails readiness and is retried with backoff
  instead of exiting. Liveness never depends on ngrok.
- No DNS server, no Host/SNI parsing, no TLS: bytes only.

### 5. Helm

`helm/ngrok-operator/templates/private-endpoints/`:

- `ngrok.features.privateEndpoints.enabled` (default false) gates everything.
- It's a setting in `internal/flags` (`NGROK_OPERATOR__FEATURES__PRIVATE_ENDPOINTS__ENABLED`),
  delivered as env like every other setting; only the api-manager reads it.
- Forwarder Deployment, ServiceAccount, Role (read `privateendpoints`). Pod settings live under
  `components.privateEndpointForwarder` and inherit `components.common`.
- api-manager gets a Role in the release namespace: `privateendpoints`
  get/list/watch/create/update/delete, `privateendpoints/status` update, Services.
- Token: the forwarder reads `PRIVATE_ENDPOINTS_ACCESS_TOKEN` from the credentials secret,
  falling back to `ngrok.credentials.accessToken`. Rendering **fails** without a token source (R6).
  A custom `ngrok.credentials.secret.name` can't be checked; the key requirement is documented in
  `values.yaml`.
- CRD ships in the ngrok-crds chart (`ngrok.com_privateendpoints.yaml`), which is a subchart of
  the operator chart under `installCRDs` (default true), so by default it's in the same release.
- No missing-CRD guard (D10): the CRD can't be toggled off, so the api-manager assumes it.
- Namespace-scoped RBAC only. DNS is not in the chart (D4).

### 6. kind CoreDNS setup

`make kind-private-endpoints-dns` (`scripts/kind-private-endpoints-dns.sh`) adds to the main
`.:53` block, then restarts CoreDNS:

```
rewrite stop name regex ^([a-z][a-z0-9-]*)\.ngrok\.direct\.$ {1}-ngrok-direct.<ns>.svc.cluster.local. answer auto
```

- `answer auto` makes replies carry the queried name.
- Only single-label names match, so multi-label names (`metadata.google.internal`) resolve
  upstream as before.
- Matching is by syntax, not known endpoints: an unknown single-label name gets NXDOMAIN.
- Re-running replaces the managed block.

Dev helper only. Real clusters: runbooks for EKS and k3s ([runbooks/](runbooks/)), user docs
later (D4, D7).

## Decisions

Full table in the [tracker](README.md#decided). What they mean for this design:

| ID | Decision | Effect here |
|---|---|---|
| D3 | *(Superseded by D9)* Both TLDs supported | — |
| D4 | Operator never edits cluster DNS; docs first | No DNS in the chart; kind script is a dev helper |
| D5 | Port reuse: cursor allocation now; stronger guarantee (quarantine / replica ack) undecided | §3 step 4; pending T2 experiment |
| D6 | Multiple installs per cluster: one DNS owner per suffix for the POC. Later: per-cluster base domain (`foo.mycluster.ngrok.direct`) once ngrok adds agent `baseDomain` | Needs multi-label hostnames, so rides on D1(b)/D2 |
| D7 | Platform order kind → EKS → k3s; others docs-only | Runbooks for these three |
| D8 | *(Superseded by D9)* `.internal` opt-in | — |
| D9 | `.ngrok.direct` only; `.internal` not supported | Poller filter (§2), one rewrite rule (§6), `naming.go` knows one TLD. Closes R4, R15, R18 |
| D10 | No missing-CRD guard | §5 |

Open: D1 (DNS for real clusters: rewrite vs own DNS server behind a conditional forward), D2
(hash Service names if D1(b) lands).

## Why no DNS server

Revision 1 ran a DNS server in the forwarder (authoritative for `internal.` / `ngrok.direct.`),
shared :80/:443 listeners that routed by Host header / SNI, and per-host Services only for
non-http endpoints. Kubernetes already gives every Service an IP and a DNS name, so a CoreDNS
rewrite onto a Service per hostname does the same job with far less of our own networking code,
and matches how agent v4 and private dial are meant to be used. A DNS-only server (no Host/SNI
routing) is the leading option for portability and multi-label names (D1(b)).

## Coexistence with bindings

Separate CRD, controllers, Deployment, and listeners. Both can be enabled at once. An
endpoint bound as `kubernetes` is skipped by the poller, so the two select disjoint sets.

## Caveats

Full list of accepted limitations: [requirements.md §3](requirements.md#3-accepted-limitations-poc).
Design-relevant ones:

- **Single-label hostnames only.** `api.foo.ngrok.direct` can't be a Service name →
  `UnsupportedHostname`. Label must be DNS-1035, ≤50 chars before `.ngrok.direct`, so the Service name fits in 63.
- **No `.internal`** (D9). Supporting it would mean owning every single-label `.internal`
  name in the cluster, including platform apexes (`ec2.internal`, GKE `metadata.internal`)
  (R4, R15), and mirroring the operator's own Ingress/Gateway `*.internal` AgentEndpoints,
  which would hairpin out through private dial (R18).
- **Port reuse.** Cursor allocation delays reuse but a lagging replica can still misroute a
  reused port (R2, D5).
- **Capacity.** 1000 endpoint-ports per cluster; replicas add throughput, not capacity.
- **Removed port** on a still-existing hostname times out rather than refusing (R7).
- **QUIC-only egress** requires UDP/443.
- **Pod identity** (GAT-475): private dial metadata is per session and not visible to Traffic
  Policy.
- **`privatedial` isn't on ngrok-go main.** Pinned to `e2b70148`.
- **Cleanup** (R16, PR-3). No ownerRefs or finalizer. Default uninstall (`installCRDs=true`)
  deletes the CRD and so the CRs, but orphans the per-host Services. With `installCRDs=false`,
  or when only the feature is disabled, CRs and Services both stay. From code; not yet run.
- **Forwarder trusts status.** It opens a listener for any `forwarderPort` in status, without
  a range check (R13).
- **Forwarder reachability.** Listeners bind `0.0.0.0`; there's no NetworkPolicy, so on
  flat-network CNIs pod IPs may be reachable from outside the cluster (PR-4).
- **Forwarder availability.** No graceful shutdown, no PDB, one replica by default (PR-5).

## Verification

Per-requirement mapping lives in [requirements.md](requirements.md). Summary:

- Unit: `internal/privateendpoints/naming_test.go` (URL parsing, Service names);
  `internal/controller/privateendpoints/privateendpoint_poller_test.go` (filter incl. D8, dedupe, delete,
  list error, drift heal, unlabeled CRs); `internal/privateendpoints/forwarder/*_test.go`
  (table, listeners, accept-time lookup, bind errors, drain); `cmd/api-manager_test.go`
  (cache scoping).
- envtest: `internal/controller/privateendpoints/privateendpoint_controller_test.go` (per-host Services,
  ports unique and stable, cache lag, duplicate reallocation, cursor no-reuse and wrap,
  `UnsupportedHostname`, `ServiceConflict`, `PortsExhausted`, removal).
- Helm: `helm/ngrok-operator/tests/private-endpoints/private-endpoints_test.yaml` (disabled
  renders nothing, flags, token fallback and render-time failure).
- Live: kind e2e ([findings.md](findings.md), 2026-09-29) and canned demo
  (`make pe-demo-up`), chainsaw e2e (`tests/chainsaw-private-endpoints/`, `make pe-demo-test`).

Not yet covered: `PortConflict`/`ServiceNotReady` reasons, forwarder readyz wiring, Role
verbs (R19), port reuse with a lagging replica, uninstall. Tracked as gaps in
requirements.md.

## Implementation deviations from the original plan

- ngrok-api-go v9 `endpoints.Client.List` takes `*ngrok.Paging`, which has no `Filter`.
  Filtering is client-side (hostname suffix + not `kubernetes`-bound).
- Controller is keyed by hostname, not by CR. No finalizer: a hostname's Service is deleted
  when its last CR disappears.
