# Approach: keep the `kubernetes` binding, keep the `svc.namespace` URL, just swap the datapath

This is the approach where we **change as little as possible**: endpoints stay
`kubernetes`-bound, the endpoint URL stays the k8s-specific `svc.namespace` form (**not**
`.internal`), the operator's poller / `BoundEndpoint` / Service projection all stay exactly as
today. The *only* things that change:

1. **Backend (mono repo):** teach private dial's `/dial` to resolve the **kubernetes-operator
   binding** for the authenticated account, not just the internal binding.
2. **Operator:** swap the forwarder's egress leg from mTLS + `UpgradeToBindingConnection` to a
   private-dial `DialContext(svc.namespace:port)`.

Nothing in the cluster's addressing changes — clients still reach `svc.namespace` through the
same two Services. No CoreDNS, no new names, no CRD change. That's the appeal: it's the
smallest possible step to get onto the private-dial datapath.

> **Honest framing:** this is *not* the direction product wants (they want to consolidate the
> `kubernetes` binding away into `private`). It keeps two binding types alive. We're proving it
> out because it's a clean, low-risk way to (a) validate the private-dial datapath against
> *real k8s-bound endpoints* end-to-end, and (b) decouple the datapath swap (K8SOP-292) from
> the consolidation/projection redesign (K8SOP-293). If it works, it's a shippable interim
> datapath and a fallback if consolidation slips.

This is exactly **unblock option 1** from [06](06-poc-and-findings.md#unblock-options). Contrast
with the [08](08-poc-build-guide.md) POC, which avoided the backend change by using `.internal`
endpoints; here we make the backend change so we can keep the k8s URL and the whole existing
projection.

---

## The one backend change: teach `/dial` the kubernetes-operator binding

### The blocker (verified against current mono repo — lines confirmed)

`/dial` (`handleDial`, `go/svc/mux/privatedial/handler.go:790-885`) hardcodes the **internal**
binding when it resolves a host. Current shape at **L826-837**:

```go
hpb := hostport.NewHostPortBinding(dreq.Host, dreq.Port, hostport.NewInternalBinding(sess.accountID))
eps, err := h.endpointMap.Get(hpb.Host, hpb.Port, hpb.Binding.ToResolvedBinding(sess.accountID))
switch {
case err == nil:
    // fall through
case errors.Is(err, muxmap.ErrNotFound):
    h.Count("mux_privatedial_dial_total", 1, obs.Tag("result", "not_found"))
    return &protocolError{ee.MuxNotFoundCode, "endpoint not found"}
default:
    h.ReportFault(r.Context(), errs.Newf("private-dial endpoint lookup: %w", err))
    return err
}
```

`sess.accountID` comes from PAT introspection (`sessionForRequest` at L816 → `authenticate`
L1072-1124). The muxmap keys endpoints by `(host, port, binding)` and **supports multi-binding
queries** (`muxmap/map.go:90` `Get`, binding is part of the key; `Key` at :438-440) — the map
holds internal- and kubernetes-bound entries side by side. So an internal-binding lookup for
`svc.namespace:port` simply never matches the **kubernetes-operator**-bound entry; there's no
fallback today.

The **exact pattern to mirror** already exists in the mTLS ingress
(`go/svc/mux/mux_k8sop.go:143-144`):

```go
hpb := hostport.NewHostPortBinding(hdr.Host, hdr.Port, hostport.NewKubernetesOperatorBinding(accID))
eps, err := h.endpointMap.Get(hpb.Host, hpb.Port, hpb.Binding.ToResolvedBinding(accID))
```

Binding constructors live in `go/lib/netx/hostport/hostportbinding.go`:
`NewInternalBinding(accountID *pb.ID)` (:254), `NewKubernetesOperatorBinding(accountID *pb.ID)`
(:261) — **both take only the account id**, and `ToResolvedBinding(resource *pb.ID)` (:411)
takes the account KSUID. Confirmed: a PAT-authed session has everything the k8s-binding query
needs.

### The change (surgical: retry in the existing ErrNotFound branch)

The cleanest insertion point is the `errors.Is(err, muxmap.ErrNotFound)` branch above: before
returning `MuxNotFoundCode`, retry the lookup with the kubernetes-operator binding.

```go
case errors.Is(err, muxmap.ErrNotFound):
    // SPIKE(k8sop-292): fall back to the kubernetes-operator binding so private
    // dial can resolve operator-projected endpoints without consolidation.
    if h.resolveKubernetesBinding {           // spike flag, default false
        kb := hostport.NewKubernetesOperatorBinding(sess.accountID)
        eps, err = h.endpointMap.Get(dreq.Host, dreq.Port, kb.ToResolvedBinding(sess.accountID))
    }
    if err != nil {  // still not found (or flag off)
        h.Count("mux_privatedial_dial_total", 1, obs.Tag("result", "not_found"))
        return &protocolError{ee.MuxNotFoundCode, "endpoint not found"}
    }
    // fall through with the k8s-bound endpoint
```

**Gate behind a spike flag** (`h.resolveKubernetesBinding`, default off) so default `/dial`
behavior is untouched. Confirm the downstream bridge still works: `handleDial` invokes the
resolved endpoint at **L871-874** via
`ep.Handler().Handle(ctx, subop.WithEndpoint(ep.Endpoint))` — a k8s-bound endpoint found in the
map has the same handler shape, so the bridge is binding-agnostic. Add a log line recording
which binding resolved, for the test evidence.

### Why auth already works (no auth change needed)

- Private dial authenticates with a **PAT**, introspected to an **account KSUID**
  (`handler.go` ~L1072-1124). `sess.accountID` is that account.
- The kubernetes-operator binding is **account-scoped** (`go/lib/bindings/computation.go`
  ~L44-48); `NewKubernetesOperatorBinding` takes the account id — the same value the mTLS
  ingress derives from the operator cert's `DNSNames[0]`.
- So a PAT-authed session has exactly what's needed to query the k8s binding index. The
  operator's *identity/cert* was never what selected the endpoint — the account was. This is
  the intended private-endpoints posture ("account boundary is the access-control layer"), so
  loosening dial resolution from per-operator-cert to per-account-PAT is a feature here, not a
  regression. (Note it explicitly when you write this up — it's a real security-scope change
  reviewers will ask about.)

### Also patch `/get-hostport` and `/get-host` (confirmed: they hardcode internal too)

Both probe handlers hardcode the internal binding the same way:
`handleGetHost` at **handler.go:948** (`hostport.NewInternalBinding(sess.accountID)`) and
`handleGetHostPort` at **:990-991**. If the private-dial **dialer** probes `/get-hostport`
before `/dial`, the probe fails before `handleDial` ever runs — so apply the same
kubernetes-binding fallback there. **Verify the dialer's behavior in PR #245**: the protocol
lets the client go straight to `/dial` (it already knows host:port), in which case you only
strictly need `handleDial`. Patch `handleGetHostPort` too if the dialer probes, or just to keep
the three paths consistent. (Note: this approach keeps the k8s Services, so we do **not** use
`/get-host` for any operator-side DNS — that's only relevant to the Mechanism-B/operator-DNS
path in [07](07-transparent-internal-projection.md).)

---

## The operator change: same egress swap, pointed at the k8s URL

Identical to [08 §Change 1/1b](08-poc-build-guide.md), with one difference: **do not** change
the URL or add DNS. The forwarder already parses `Spec.EndpointURL = tcp://svc.namespace:port`
into `host = svc.namespace`, so `DialContext(ctx,"tcp", host+":"+port)` dials the k8s URL and
the (now-patched) backend resolves it via the kubernetes-operator binding.

- `internal/controller/bindings/forwarder_controller.go:223-268` — replace the cert load +
  `tlsDialer.Dial(ingressEndpoint)` + `mux.UpgradeToBindingConnection` with the private-dial
  `DialContext`, keep `joinConnections`. Gate behind `USE_PRIVATE_DIAL`.
- Construct the dialer once in `cmd/bindings-forwarder-manager.go` from a PAT + connect-URL
  flag (dev `connect-endpoint.dev-ngrok.com`). Confirm the constructor against
  [ngrok-go PR #245](https://github.com/ngrok/ngrok-go/pull/245) via `go.mod replace`.
- **Keep the poller, `BoundEndpoint` CRDs, and both Services unchanged.** The endpoints stay
  kubernetes-bound; the projection is untouched. This is the whole point.

### Pod identity — the one real regression to decide on

Today the k8s binding carries pod identity through `UpgradeToBindingConnection`'s ConnRequest
header, surfacing as `conn.k8s.pod.*` in Traffic Policy. Private dial's `DialReq.metadata`
(`proto/lib/private_dial/private_dial.proto:74-84`, field 3) exists but is **unread** —
`handleDial` unmarshals `dreq` at handler.go:811 and never accesses `dreq.Metadata`. So a naive
swap **silently drops pod identity** and any `conn.k8s.pod.*` policy regresses.

- **POC-minimal:** skip pod identity; note the regression loudly. Fine to prove resolution +
  datapath.
- **Full parity (this is GAT-475):** have the operator stuff `PodIdentity` into
  `DialReq.metadata`, and have `handleDial` read it and attach it to the connection the same
  way the ConnRequest header path does, so `conn.k8s.pod.*` keeps working. Doing this on the
  same devbox is the natural stretch goal — it's the other half of K8SOP-292 and de-risks the
  scariest part of the migration.

---

## End-to-end test plan (both repos on one box)

1. **Backend:** build the mono repo with the `/dial` k8s-binding change, run the devenv mux/
   dial ingress locally (or the shared dev ingress). Note the connect URL the operator must use.
2. **A real kubernetes-bound endpoint:** create one the normal way — the simplest is to run the
   operator's *existing* (unmodified, mTLS) path once and let the poller project a
   `kubernetes`-bound endpoint (`svc.namespace`) so you have a genuine k8s-bound entry in the
   mux. Confirm it resolves over the OLD mTLS path first (baseline).
3. **Operator:** deploy with `USE_PRIVATE_DIAL=true`, PAT secret, connect URL → your devbox
   dial ingress. The forwarder now private-dials `svc.namespace`.
4. **Prove resolution:** from a client pod, hit `svc.namespace` (the existing Target Service).
   - **L7:** `curl http://svc.namespace/` reaches the backend.
   - **Raw TCP (the crux):** a redis/postgres-backed k8s-bound endpoint —
     `redis-cli -h svc.namespace ping` → `PONG`.
   - Backend `/dial` logs show it resolved via the **kubernetes-operator** binding (add a log
     line in the spike).
5. **A/B:** toggle `USE_PRIVATE_DIAL` off → same endpoint still works over mTLS. Proves you
   swapped the datapath without breaking projection.
6. **(Stretch) Pod identity:** apply a Traffic Policy that reads `conn.k8s.pod.namespace`;
   confirm it's empty with POC-minimal and correct once `DialReq.metadata` is plumbed.

### Success criteria
- A **kubernetes-bound** endpoint (`svc.namespace`, real k8s URL, existing projection) is
  reachable **over private dial** for both HTTP and raw TCP, with the backend resolving it via
  the kubernetes-operator binding — **no `.internal`, no DNS changes, no CRD changes.**
- mTLS path still works behind the flag.
- Clear log evidence of the new resolution path and (if attempted) pod identity via metadata.

### Out of scope / not blockers
Consolidation into the `private` binding, the projection-discriminator redesign, cross-provider
DNS, region pinning, throughput. Those belong to the other approaches / K8SOP-293.

---

## How this maps to the tickets and the other options

- Implements **K8SOP-292** (datapath swap) *without* waiting on **K8SOP-293** (retire the
  kubernetes binding) — deliberately keeps the binding.
- The backend `/dial` change is the smaller of the two [06 unblock options](06-poc-and-findings.md#unblock-options);
  the larger one (consolidate to `private`) is what the field/new-resource/URL-convention
  options in [04](04-design-options.md) are really about.
- If product later insists on consolidation, this work isn't wasted: the operator egress swap
  and the pod-identity-via-metadata plumbing are identical; only the *binding the backend
  resolves* changes.
