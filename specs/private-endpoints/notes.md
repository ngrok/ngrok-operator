# Private Endpoints in Kubernetes — notes and research brief

Knowledge base for this POC: what we're solving, what we built, what we tried and dropped,
what's still open, and a brief for researching how other projects solve the same problem.

- Current design: [design.md](design.md)
- Test results (private dial probe, live kind runs): [findings.md](findings.md)
- [plan.md](plan.md) is the original implementation plan for revision 1 (own DNS server +
  Host/SNI routing). It is history, superseded by revision 2.

---

## 1. The problem

ngrok **private endpoints** have exactly one URL, under `*.internal` or `*.ngrok.direct`, and no
public ingress. They're reachable only through ngrok: from other ngrok endpoints, or from a
client using the **private dial** SDK with account credentials.

Goal: a pod in any cluster running the operator can use those URLs directly, with no
per-endpoint Kubernetes config, no sidecar, and no app changes:

```
curl http://foo.internal
curl https://foo.ngrok.direct          # real cert, verification on
redis-cli -h bar.internal -p 6379
```

This replaces **Kubernetes bindings** (`BoundEndpoint`), the older mechanism:
- an endpoint had to be explicitly bound to the cluster;
- the operator projected a Service per endpoint using a `service.namespace` URL convention
  (not the endpoint's real hostname);
- the operator's forwarder connected to an ngrok ingress over mTLS and attached pod identity.

Requirements:
- Real hostnames, so TLS for `*.ngrok.direct` verifies.
- All endpoint schemes: http, https, tls, tcp.
- New CRD so bindings can be deprecated while both run side by side.

Explicitly out of scope for the POC: access control (which pods may reach which endpoints),
per-pod identity on the dial, DNS setup for real cloud providers (only kind), scale beyond
low hundreds of endpoints.

## 2. Building blocks and facts we rely on

**Private dial** (`golang.ngrok.com/ngrok/privatedial`):
- `Dialer.DialContext(ctx, "tcp", "host:port")` returns a byte stream to the endpoint. ngrok
  matches `(host, port)` to the endpoint server-side.
- Transport is HTTP/3 over QUIC with an HTTP/2 fallback, to `connect-endpoint.ngrok.com`.
  One lazily connected session multiplexes many dials. Each dial has a 5s budget.
- Auth: an ngrok access token (PAT).
- Newer versions add `GetHost(host)`, a "does an endpoint exist for this hostname" check meant
  for answering DNS without dialing.
- Client metadata can be sent per session, but isn't exposed to endpoint Traffic Policy yet,
  so there's no per-connection identity (see open items).
- Not on ngrok-go `main` yet; it lives on a feature branch. We pin `v0.0.0-20260616091145-e2b70148b6de`.
- We force QUIC because the HTTP/2 transport path failed on Go 1.27 in our earlier testing.
  Consequence: the cluster needs UDP/443 egress.

**What this means for the design:** the client only has to know *which hostname and port* a
connection is for. It never needs to parse HTTP or TLS. One IP per hostname plus the
destination port passed through is enough. ngrok's upcoming agent v4 uses the same model on
workstations (a virtual network interface, a local DNS resolver, one synthetic IP per
hostname, port passed through).

**TLS:**
- `https://…` endpoints: ngrok terminates TLS. Verified: a private dial to an `https://…ngrok.direct`
  endpoint on 443 presents a publicly trusted wildcard cert for `*.ngrok.direct`.
- `tls://…` endpoints: TLS is end to end to the origin.
- `.internal` is reserved by ICANN for private use and will never be delegated publicly, so no
  public cert is possible; https clients for `.internal` must skip verification. That's why
  `.ngrok.direct` exists.

**ngrok API quirks** (ngrok-api-go v9):
- `Endpoints.List` takes `*ngrok.Paging`, which has no CEL filter, so we filter client-side.
- `Scheme`, `Host`, `Port` come back empty in list results; parse everything from `URL`.
- Both TLDs currently report `bindings: ["internal"]`. The binding value is expected to be
  renamed (to `private`); our filter uses the hostname suffix, so it's unaffected.
- A `tls://` endpoint's URL comes back with an explicit `:443`.
- Pooled endpoints share a URL; we collapse them into one resource.

## 3. Current design (revision 2) in one picture

```
ngrok API ──poll every 10s──▶ PrivateEndpoint CRs (operator namespace, one per URL)
                                     │
             ┌───────────────────────┴────────────────────────┐
             ▼                                                ▼
 controller (api-manager):                       forwarder Deployment:
 one Service per hostname (foo-internal),        one listener per forwarderPort,
 one port per endpoint → forwarderPort           forwarderPort → private dial host:port

 pod ── DNS foo.internal ──▶ CoreDNS rewrite ──▶ foo-internal.<ns>.svc.cluster.local ──▶ ClusterIP
 pod ── ClusterIP:6379 ──▶ Service ──▶ forwarder:forwarderPort ──▶ DialContext("foo.internal:6379")
```

Kubernetes Services do the "one IP per hostname" job that agent v4's IP allocator does, and
CoreDNS does the name → IP job that agent v4's DNS resolver does.

## 4. DNS: making the names resolve (the "hijacking" problem)

Pods use the cluster's DNS (almost always CoreDNS, sometimes behind NodeLocal DNSCache).
Something has to make `foo.internal` / `foo.ngrok.direct` answer with the right ClusterIP.
Cluster DNS is owned by the cluster admin or the cloud provider, so this always needs a
privileged, provider-specific config change. **This is the biggest open question.**

### What we do now (revision 2): CoreDNS rewrite onto Service names

Added to CoreDNS's main `.:53` block (kind only, via `make kind-private-endpoints-dns`):

```
rewrite stop name regex ^([a-z][a-z0-9-]*)\.internal\.$ {1}-internal.<ns>.svc.cluster.local. answer auto
rewrite stop name regex ^([a-z][a-z0-9-]*)\.ngrok\.direct\.$ {1}-ngrok-direct.<ns>.svc.cluster.local. answer auto
```

- Only single-label names are rewritten, so `metadata.google.internal` (GKE's metadata server)
  and other multi-label `.internal` names keep resolving upstream. Verified in kind.
- `answer auto` makes replies carry the name the client asked for.
- Unknown single-label names get NXDOMAIN.
- The `-internal` / `-ngrok-direct` suffix keeps `foo.internal` and `foo.ngrok.direct` (different
  endpoints) on different Services.

Caveats (accepted for now):
- **Single-label hostnames only.** `api.foo.internal` can't map to a Service name (no dots allowed).
- **Service-name rules:** the label must be a DNS-1035 label (lowercase letters, digits, `-`,
  starting with a letter), max 54 chars before `.internal`, 50 before `.ngrok.direct`.
- **We take over every single-label `.internal` name** in the cluster. Clusters that already
  use such names collide. Long term: document it, steer those users to `.ngrok.direct`.
- One Service per hostname.

### What we tried first (revision 1, built then removed): our own DNS server

- The forwarder ran an authoritative DNS server for `internal.` and `ngrok.direct.`.
- CoreDNS got stub-zone `forward` blocks for those two zones pointing at it.
- Known names → shared Service IP (http/https/tls, routed by Host header / SNI) or a per-host
  Service IP (tcp). Unknown `.ngrok.direct` → NXDOMAIN; unknown `.internal` → forwarded to the
  node's upstream resolver. The forwarder pod used `dnsPolicy: Default` to avoid a loop.
- Removed because Kubernetes Services already give us IPs and names; it was a lot of our own
  low-level networking code (DNS, HTTP/TLS sniffing) for the team to support.
- What it handled that rev 2 doesn't: multi-label names, arbitrary labels, fewer Services.
- It remains the fallback if the rev 2 caveats become blockers (it could answer from
  `privatedial.Dialer.GetHost` instead of polling the API).

### Other options considered, not built

- **Mutating webhook that sets pod `dnsConfig`** (nameservers/search): avoids touching
  kube-system, but per-pod opt-in, invasive, and not "works everywhere automatically".
- **NodeLocal DNSCache customization:** still a cluster-wide, provider-specific edit.
- **Transparent capture** (TUN device or eBPF connect hooks per node, synthetic IP range):
  privileged, per-node, CNI-sensitive.
- **Old bindings model:** Kubernetes-native names (`svc.namespace`) need no DNS change at all,
  but aren't the endpoint's real hostname, so TLS/hostnames don't line up.

## 5. Data path

- Each endpoint gets a forwarder port (20000–20999). The hostname's Service maps
  `port: <endpoint port>` → `targetPort: <forwarder port>`.
- The forwarder accepts on that port and calls `DialContext("<hostname>:<port>")`, then copies
  bytes both ways: half-close at EOF, and once either direction ends the other gets 5s before
  both are closed. A client half-close starts that timer too, so a response slower than 5s
  after it is cut (R8, research/07 PR-2).
- No HTTP/TLS parsing, no DNS, no per-pod identity.
- Listeners reuse the operator's existing `pkg/bindingsdriver`. Known quirks of it: binds
  `0.0.0.0`; on an accept error it logs and retries immediately (could spin).
- The dial-and-join code is isolated in
  [`internal/privateendpoints/forwarder/privatedial_forward.go`](../../internal/privateendpoints/forwarder/privatedial_forward.go).
  The same "accept, dial, copy both ways" loop exists, unexported, in several ngrok codebases
  (the agent, ngrok-go's forwarder, the old bindings forwarder). Proposal: export
  `privatedial.Join(a, b, drain)` and `(*Dialer).Forward(ctx, listener, target)` upstream and
  delete this file.

## 6. Control plane

- **Poller** (api-manager, leader only): lists all endpoints every 10s; keeps those whose
  hostname ends in `.ngrok.direct` (plus `.internal` only with
  `--features-private-endpoints-include-internal`, D8) and that aren't `kubernetes`-bound; creates and
  deletes `PrivateEndpoint` resources (only ones carrying its label). Any list error → change
  nothing that tick.
- **Controller** (api-manager): keyed by hostname, because every endpoint on a hostname must
  share the one IP DNS returns. Creates/updates the hostname's Service, allocates forwarder
  ports (reading uncached so back-to-back reconciles can't hand out the same port; a port
  claimed by two hostnames is reassigned to the higher-named one), writes status, marks
  hostnames that can't be a Service name `UnsupportedHostname`, deletes the Service when the
  hostname's last endpoint goes away.
- **Forwarder:** watches `PrivateEndpoint` resources; opens/closes listeners to match; ready
  once the first sync completes.
- Helm: off by default (`ngrok.features.privateEndpoints.enabled`); the forwarder reads its token from the
  credentials secret key `PRIVATE_ENDPOINTS_ACCESS_TOKEN` (falls back to `ngrok.credentials.accessToken`).

## 7. Decision log

| Decision | Why |
|---|---|
| Real work, but backend gaps (pod identity) can be hand-waved for the POC | Get the shape right first |
| Support both `.ngrok.direct` (primary) and `.internal` (*superseded by D8*) | `.ngrok.direct` gets real certs; `.internal` is the private-use TLD |
| D8: mirror `.ngrok.direct` only by default; `.internal` opt-in (Helm `ngrok.features.privateEndpoints.includeInternal`, kind `PE_INCLUDE_INTERNAL=true`) | PM (2026-10-12): `.ngrok.direct` is the prescribed pattern; `.internal` stays supported but secondary. Avoids `.internal` collisions (R4, R15, R18) by default |
| Drop endpoint selectors; mirror every private endpoint in the account | Simplicity for the POC; access control comes back later |
| New `PrivateEndpoint` CRD in `ngrok.com/v1` | Deprecate bindings independently; all CRDs are moving to this group |
| kind + manual CoreDNS edit for the POC | Provider DNS is a separate research spike |
| Separate forwarder Deployment | Runs alongside the bindings forwarder |
| Rev 1 → rev 2: drop own DNS server and Host/SNI routing; CoreDNS rewrite + Service per hostname | Less low-level code we own; matches private dial's (host, port) model and agent v4 |
| Hostname-to-Service naming with TLD suffix | Same label under both TLDs must not collide |
| Accept single-label-only, DNS-1035 labels, owning single-label `.internal` | Revisit with own DNS server if needed |
| Isolate dial/copy code with an upstream proposal | Push common logic into the SDK |

## 8. Review status

Fixed after review (each with a test that failed first):
- api-manager crashed on installs without the new CRD even with the feature off (cache entry
  was unconditional). With the feature **on** and the CRD missing it also failed to start
  (research/07 PR-1); now a discovery check warns and disables the feature. Fixed in the
  working tree, unit-tested (`TestPrivateEndpointCRDInstalled`), not run live.
- Back-to-back reconciles could give two hostnames the same forwarder port (stale cache),
  silently cross-routing tcp traffic.
- With `watchNamespace` set to another namespace, Services in the operator namespace weren't
  cached, so nothing got wired.
- Agent-manager crashlooped when the feature was enabled (shared Helm flag helper).

Fixed since (R6): Helm rendering now fails when the forwarder has no token source; a custom
credentials secret can't be checked, so its `PRIVATE_ENDPOINTS_ACCESS_TOKEN` key is documented
in `values.yaml`.

Still open (minor):
- A pre-existing duplicate forwarder port heals only on the losing hostname's next reconcile.
- A connection to a port a hostname no longer has times out (no Service port) instead of
  being refused.
- No idle timeout on connections where neither side has finished.
- `bindingsdriver` quirks (above).

## 9. Open items and follow-ups

- **Provider DNS** (EKS, AKS, GKE, OpenShift, k3s, IaC-managed CoreDNS, NodeLocal DNSCache):
  see the research brief below.
- **Pod identity:** private dial metadata is per session and not exposed to endpoint policy;
  per-connection metadata exists in the protocol but isn't plumbed. Needs backend work.
- **Upstream SDK:** `Join` / `Forward` helpers in `privatedial`; move off the feature-branch pin
  once it lands on ngrok-go main; re-test HTTP/2 on Go 1.27.
- **Multi-label hostnames** → own DNS server (revision 1 approach, backed by `GetHost`).
- **Access control:** which pods/namespaces may reach which endpoints.
- **Scale:** one Service per hostname, 1000 forwarder ports.
- **Release:** bump the ngrok-crds chart version (new CRD).

## 10. Running it locally

```
kind create cluster --name pe-poc
NGROK_ACCESS_TOKEN=<PAT> KIND_CLUSTER_NAME=pe-poc make deploy_with_private_endpoints
make kind-private-endpoints-dns
# endpoints outside the cluster, e.g.
NGROK_AUTHTOKEN=<PAT> ngrok http 8080 --url https://<name>.ngrok.direct
NGROK_AUTHTOKEN=<PAT> ngrok tcp 6379 --url tcp://<name>.internal:6379
kubectl -n ngrok-operator get privateendpoints
kubectl run c --rm -it --image=nicolaka/netshoot -- curl https://<name>.ngrok.direct
```

Gotchas:
- If `make manifests` rewrites every CRD's `controller-gen` version annotation, the devshell
  is stale: `direnv reload`.
- `go test -race` needs `CGO_ENABLED=1` in the devshell.
- quic-go warns about the UDP receive buffer size; harmless for testing.

---

## 11. Research brief (landscape / prior art)

**Context to give a research agent:** We're building a Kubernetes operator feature that makes
externally hosted private services (ngrok private endpoints, hostnames like `foo.internal` and
`foo.ngrok.direct`) reachable from any pod by their real hostname, with no app changes. Traffic
leaves the cluster through an in-cluster forwarder that tunnels each TCP connection to the
service by `(hostname, port)`. The hard part is DNS: making cluster DNS answer those names with
in-cluster IPs on every major Kubernetes platform, safely and with minimal privileges. Today we
add CoreDNS `rewrite` rules mapping `<label>.internal` to a per-hostname Service
(`<label>-internal.<ns>.svc.cluster.local`), and we've also built (and parked) a variant where
our own DNS server is a CoreDNS stub domain.

**Questions:**

1. **Platform DNS customization.** For EKS (CoreDNS add-on), AKS, GKE (kube-dns and Cloud DNS
   for GKE), OpenShift (DNS Operator), k3s/RKE2, and other common platforms:
   - What is the supported way to add a stub domain, conditional forwarder, or rewrite rule?
   - Does the provider overwrite manual CoreDNS changes on upgrade or add-on update?
   - What permissions does it take, and can an operator do it automatically?
   - How does NodeLocal DNSCache change the picture?
   - Does any platform already reserve or use `.internal` (e.g. GKE's `metadata.google.internal`),
     and what breaks if we intercept it?
2. **Prior art.** How do comparable products make private or external names resolvable inside a
   cluster, and how do they get traffic out? For example: Tailscale's Kubernetes operator
   (egress, MagicDNS), Cloudflare (Tunnel / WARP in clusters), Twingate, Netbird, ZeroTier,
   Teleport, HashiCorp Consul (DNS forwarding for `.consul`), Istio (DNS proxying,
   ServiceEntry), Linkerd multicluster, Cilium ClusterMesh, Submariner Lighthouse and the
   Multi-Cluster Services API (`clusterset.local`), AWS VPC Lattice / PrivateLink, GCP Private
   Service Connect. For each: which DNS mechanism (CoreDNS edit, own DNS server, webhook-injected
   pod `dnsConfig`, sidecar DNS proxy, CNI/eBPF), how IPs are allocated (Service per name vs
   synthetic range), and why.
3. **Patterns and trade-offs.** Rewrite vs stub domain vs own DNS server vs pod `dnsConfig`
   webhook vs sidecar/CNI interception: install UX, privileges, upgrade safety, multi-label
   names, conflicts with existing names, scale, uninstall cleanup.
4. **Identity.** How do these products convey the calling workload's identity (pod, service
   account, namespace) to the remote service or its policy engine?
5. **TLS for private names.** How do others handle certificates for private hostnames
   (reserved TLDs like `.internal` vs vendor-owned domains)?

**Deliverable:** a comparison table (product × DNS mechanism × IP allocation × privileges ×
platform support × identity), a short write-up per platform on the supported DNS
customization path, and a recommendation for our case.
