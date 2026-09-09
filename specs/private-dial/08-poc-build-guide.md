# POC build guide — full transparent-mode datapath on private dial

**Goal of this POC:** prove the *entire* transparent-mode datapath end-to-end — a pod
resolves `foo.internal`, opens a **raw TCP** connection, and it reaches a real ngrok
`.internal` endpoint's upstream — using **only existing private-dial code and existing
`.internal` endpoints**, with **zero mono-repo/backend changes** and **no `kubernetes`
binding anywhere**.

This is the POC that Mechanism B ([07](07-transparent-internal-projection.md)) unlocks. Read
[06](06-poc-and-findings.md) first for the backend finding that makes it possible.

---

## Why this POC has none of the k8s-bound POC's limitations

The k8s-bound "full POC" was blocked: private dial's `/dial` hardcodes the **internal**
binding (`handler.go:826`), and kubernetes-bound endpoints live under a different muxmap
index, so private dial can't resolve them without a backend change (teach `/dial` the k8s
binding, or consolidate).

**This POC sidesteps that entirely by not using k8s-bound endpoints at all.** It projects an
ordinary `.internal` (private) endpoint — which already lives in the internal binding index,
which is exactly what `/dial` already resolves. The blocker only ever applied to the "keep
the kubernetes binding" premise, and B drops that premise.

| | k8s-bound "full POC" | **This POC (Mechanism B)** |
|---|---|---|
| Endpoint type projected | `kubernetes`-bound (`svc.ns`) | ordinary `.internal` private endpoint |
| Private dial can resolve it today? | **No** (hardcoded internal binding) | **Yes** |
| Mono-repo/backend change needed? | **Yes** (teach `/dial` or consolidate) | **No** |
| Proves raw-TCP demux in-cluster? | yes | yes |
| Proves `foo.internal` addressability (no ns URI)? | no | **yes** |
| New in-cluster code | forwarder datapath | forwarder datapath **+ DNS glue** |

The one honest caveat: this proves the **transparent-mode** architecture (the thing we'd
pitch product), not a drop-in swap of today's shipping datapath. It's evidence *for* the
"drop namespace URIs" decision, not code that assumes it's been made.

---

## What already exists and is reused unchanged

Today's datapath already implements Mechanism B's demux. Trace it:

- **Per-endpoint ClusterIP Service (the "Upstream Service")** — `boundendpoint_controller.go:455`
  (`convertBoundEndpointToServices`). ClusterIP in the operator namespace, named after the
  BoundEndpoint, selector = forwarder pods, `Port = Target.Port` → `TargetPort = Spec.Port`.
- **Per-endpoint forwarder listener** — `bindingsdriver/driver.go:23` (`Listen(port, handler)`)
  opens `0.0.0.0:<Spec.Port>`. **The endpoint identity IS the local port the bytes land on.**
  That's the L4 demux, and it's already correct for raw TCP.
- **The forwarder handler** — `forwarder_controller.go:143-274` (`update`): parses
  `Spec.EndpointURL` → `host`,`port`, then for each accepted connection does the egress leg.

So the connection path *inside* the cluster — client → ClusterIP:port → forwarder pod:Spec.Port
— is untouched. We change exactly two things: the **egress leg** (mTLS → private dial) and
the **DNS name** clients use to reach the ClusterIP (`svc.ns` → `foo.internal`).

---

## The two code changes

### Change 1 — egress leg: mTLS + UpgradeToBindingConnection → private dial

**File:** `internal/controller/bindings/forwarder_controller.go`
**Replace:** lines **223-268** (load TLS cert → `tlsDialer.Dial` → `mux.UpgradeToBindingConnection`
→ `joinConnections`).
**With:** a private-dial `DialContext` to the same `host:port`, then the existing
`joinConnections`.

Sketch (confirm exact dialer constructor against [ngrok-go PR #245](https://github.com/ngrok/ngrok-go/pull/245)
/ the `golang.ngrok.com/ngrok/privatedial` package — used in the mono repo at
`local/ngrok/go/svc/ship/core/privatedial/privatedial.go:14` as `ngrokdial`):

```go
// r.PrivateDialer constructed once at manager setup (see Change 1b), reused per conn.
ngrokConn, err := r.PrivateDialer.DialContext(ctx, "tcp", net.JoinHostPort(host, strconv.Itoa(port)))
if err != nil {
    log.Error(err, "private dial failed", "target", host)
    return err
}
log.Info("private-dialed endpoint", "target", host)
return joinConnections(log, conn, ngrokConn)
```

Gate it behind a flag/env (`--use-private-dial` / `USE_PRIVATE_DIAL=true`) so the mTLS path
stays intact and toggleable. In the private-dial branch you can **skip** the cert load
(223-227), the ingress-endpoint plumbing (`ingressEndpoint`, 180-183, 242-246), and the pod
identity gather (201-221) — pod identity is GAT-475 and out of POC scope.

### Change 1b — construct the dialer once, wire in the PAT

- Add `PrivateDialer *privatedial.Dialer` (or the ngrok-go dialer type) to `ForwarderReconciler`.
- Build it in `cmd/bindings-forwarder-manager.go` from:
  - **auth**: a PAT or authtoken (`ngrok_pat_*` — private dial is PAT-only, `handler.go:1072`),
    mounted from a Secret / env. Reuse whatever the manager already reads for credentials.
  - **connect ingress**: the private-dial connect URL. Prod `connect.ngrok-agent.com`
    (or `{h2,quic}.connect-endpoint.ngrok.com`); dev `connect-endpoint.dev-ngrok.com`.
    Confirm the default/override knob in PR #245. Make it a flag.
- One `/session` is established by the dialer; each `DialContext` opens a stream. Per-conn
  dial is fine for the POC (throughput is K8SOP-261's problem, not this POC's).

### Change 2 — make `foo.internal` resolve in-cluster

The Upstream ClusterIP Service is named after the BoundEndpoint and lives in the operator
namespace, so it already has a cluster DNS name: `<name>.<opns>.svc.cluster.local`. Name the
BoundEndpoint `foo` and it's `foo.ngrok-operator.svc.cluster.local`.

**POC shortcut (no custom DNS server): a CoreDNS `rewrite`.** Add to the CoreDNS ConfigMap
(`kube-system/coredns`), inside the `.:53` server block:

```
rewrite name regex (.*)\.internal {1}.ngrok-operator.svc.cluster.local
```

Now `foo.internal` → `foo.ngrok-operator.svc.cluster.local` → the Upstream ClusterIP, and
the client's port (`foo.internal:80`) is preserved through DNS. Raw TCP demuxes on the
landing port exactly as today. `kubectl -n kube-system rollout restart deploy/coredns` after
editing.

> **Productization note, not for the POC:** the real Mechanism B replaces this coarse regex
> rewrite with a small **operator-DNS** pod that answers `*.internal` live from the mux
> (`/get-host` at `handler.go:918`; NXDOMAIN otherwise) and a per-provider stub-zone install
> (the matrix in [07](07-transparent-internal-projection.md)). The rewrite is purely to prove
> the datapath without building a DNS server first.

**No CRD change needed.** `BoundEndpointSpec.EndpointURL`'s validation pattern
(`boundendpoint_types.go:44`) accepts `tcp://foo.internal:80` — it parses as service=`foo`,
namespace=`internal`. (3-label `svc.ns.internal` would *not* match the 2-label pattern; keep
POC names 2-label.)

---

## Stand up the POC — step by step

### Prereqs
- A kind cluster (or any cluster where you can edit the `kube-system/coredns` ConfigMap).
- ngrok account creds available to the codespace: an **authtoken** and a **PAT** (`ngrok_pat_*`).
- The ngrok-go private-dial branch (PR #245) pulled via `go.mod replace`.
- Network egress from the cluster to the connect ingress
  (`connect.ngrok-agent.com` / dev `connect-endpoint.dev-ngrok.com`). **Verify this early** —
  it's the most likely environmental snag (see gotchas).

### Step 1 — create the test `.internal` endpoint + upstream (existing product, no code)
Stand up an ordinary internal endpoint bound to a trivial backend, using existing tooling:
- Run a backend in-cluster: an HTTP echo (`hashicorp/http-echo` / `ealen/echo-server`) **and**
  a raw-TCP service (`redis`) so you can prove both L7 and L4.
- Expose each as an internal endpoint via an **ngrok agent** (or agent endpoint / cloud
  endpoint with a `forward-internal` Traffic Policy) at `foo.internal` (HTTP) and
  `bar.internal` (redis/TCP). Confirm from a laptop with the released dialer or the dashboard
  that `foo.internal` resolves and reaches the backend **before** touching the operator.

### Step 2 — build the operator with the datapath swap
- Apply Change 1 / 1b. `go.mod replace` the ngrok-go branch. `make build`.
- Load the image into kind, deploy the operator with `USE_PRIVATE_DIAL=true`, the PAT secret,
  and the connect-URL flag set to your environment.

### Step 3 — hand-wire the projection (skip the poller)
Apply a BoundEndpoint CR per test endpoint — no poller changes for the POC:

```yaml
apiVersion: bindings.k8s.ngrok.com/v1alpha1
kind: BoundEndpoint
metadata:
  name: foo                       # → Upstream Service foo.<opns>.svc.cluster.local
  namespace: ngrok-operator
spec:
  endpointURL: tcp://foo.internal:80   # host the forwarder private-dials
  scheme: tcp
  port: 10080                     # per-endpoint forwarder listener port (Spec.Port)
  target:
    service: foo
    namespace: default            # target ExternalName svc (optional for the POC)
    protocol: TCP
    port: 80                       # client-facing port; Upstream Service Port
```

The BoundEndpoint controller mints the Upstream ClusterIP Service; the forwarder controller
opens `:10080` and, on each conn, private-dials `foo.internal:80`.

### Step 4 — DNS
Apply Change 2 (CoreDNS rewrite), restart CoreDNS.

### Step 5 — test from a pod
```
kubectl run test --rm -it --image=nicolaka/netshoot -- bash
# L7:
curl -v http://foo.internal/
# L4 (the one that matters — proves demux with no in-band name):
redis-cli -h bar.internal ping        # expect PONG
nc -vz bar.internal 6379
# resolution sanity:
getent hosts foo.internal
```

---

## Test matrix / success criteria

| Check | Proves |
|---|---|
| `curl http://foo.internal` returns the echo backend | full path incl. DNS + private dial (L7) |
| `redis-cli -h bar.internal ping` → `PONG` | **raw-TCP demux works in-cluster** (the crux) |
| Two endpoints (`foo`,`bar`) both reachable, no crosstalk | per-port L4 demux is correct |
| forwarder logs show `private-dialed endpoint` and no mTLS/cert path | egress leg actually swapped |
| kill the backend → connection fails cleanly; restore → works | it's really traversing the endpoint, not cached |
| `getent hosts foo.internal` resolves to the Upstream ClusterIP | DNS rewrite + Service wiring |

Explicitly **out of POC scope** (design-gated, not blockers): pod identity `conn.k8s.pod.*`
(GAT-475), the projection discriminator (which endpoints project into which cluster — hand-scoped
here), cross-provider DNS install (POC uses one CoreDNS), throughput (K8SOP-261), region
pinning (GAT-675).

---

## Gotchas

- **Connect-ingress reachability is the #1 risk.** Confirm the cluster can reach the connect
  URL (and that the branch defaults to the right one for your account's environment) before
  debugging anything else. A silent dial hang is almost always this.
- **PAT, not authtoken, for the dial itself.** `handler.go:1072-1124` rejects non-`ngrok_pat_*`
  tokens. The agent that *serves* the test endpoint uses an authtoken; the forwarder that
  *dials* uses a PAT. Don't cross them.
- **Port preservation.** DNS only maps the name; the client's port must equal the Upstream
  Service `Port` (= `Target.Port`). Keep them equal (`80`/`80`, `6379`/`6379`).
- **2-label names only.** `foo.internal` passes the CRD pattern; `svc.ns.internal` doesn't.
  Fine for the POC; it's a real constraint the productized path (operator-DNS, relaxed/removed
  pattern) removes.
- **Don't reintroduce the internal-binding blocker.** If you point the forwarder at a
  *kubernetes-bound* endpoint instead of a `.internal` one, `/dial` won't resolve it — that's
  the [06](06-poc-and-findings.md) finding, and it's the whole reason this POC uses `.internal`.
- **Leave the mTLS path intact behind the flag** so you can A/B and so you haven't broken the
  shipping datapath while spiking.

---

## What a green POC lets us say to product

> "Transparent `foo.internal` in-cluster addressing over raw TCP works today on the existing
> private-dial datapath, no backend change, no `kubernetes` binding — and it kept the real
> Service (so NetworkPolicy still applies) with no privileged TUN. The remaining asks are
> pod-identity in Traffic Policy (GAT-475) and the per-provider DNS install, both scoped and
> known. The open *product* question is whether TP pod variables can replace per-namespace
> Services for authz — if yes, we can retire namespace URIs."
