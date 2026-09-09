# Approach: a `kubernetes` object field on the Endpoint API resource

**This is the chosen direction.** Add a new, extensible **object** field to the ngrok Endpoint
API resource that carries a **list of structured Kubernetes projection targets**. The endpoint
stays an ordinary `.internal`/private endpoint; the new field is metadata the operator consumes
to decide *where in the cluster* to project it. Then build the operator around it.

Decisions locked (2026-09-09):
- **Structured sub-fields**, not a URI string — server-validatable, no parsing, typo-proof.
- **A list of targets**, so one endpoint can project into several namespaces/clusters at once.
- Object wrapper so we can add fields later (protocol, TLS, metadata, per-target policy)
  without another API break.

```yaml
# ngrok Endpoint API resource
url: tcp://foo.internal:80          # a real private endpoint (internal-bound)
bindings: [private]                 # NOT the kubernetes binding
kubernetes:                         # NEW object field
  targets:
    - service: myservice
      namespace: team-a
      port: 80
    - service: myservice
      namespace: team-b
      port: 80
    # future per-target fields: protocol, tls, metadata, ...
```

---

## Why this is the clean one: no datapath/backend `/dial` change

The endpoint is `private`/internal-bound, so **private dial already resolves it today** — the
hardcoded-internal-binding blocker ([06](06-poc-and-findings.md)) never applies, and unlike
[09](09-keep-k8s-binding-approach.md) there is **no `/dial` change**. The backend work here is
an **API field addition** (schema + storage + list), not a datapath change. The operator dials
the endpoint's real `.internal` host and projects Services from the field.

It also fixes the discriminator problem cleanly: **carrying `kubernetes.targets` is itself the
projection signal.** Only endpoints with the field get projected, so the dangerous
"`endpoint_selectors` default `["true"]` projects everything" concern from [03](03-the-gap.md)
is defused — selectors now only narrow *which clusters* among field-carrying endpoints, not
*whether to project at all*.

Where this sits vs the other options ([04](04-design-options.md)): it's **option A1**, done as
an extensible object with a target list. Explicit and validatable (unlike the URL-convention
option C), no new top-level resource (unlike option B), and no privileged TUN / DNS
(unlike option E).

---

## Backend build (mono repo)

### 1. Proto / Endpoint model
Add the field to the Endpoint model (`proto/rpx/model_endpoint.proto` — the `rpx.Endpoint`
message that today carries `binding` / `resolved_binding` ~L146-147). New messages:

```proto
message KubernetesProjection {
  repeated KubernetesTarget targets = 1;
}
message KubernetesTarget {
  string service   = 1;
  string namespace = 2;
  int32  port      = 3;
  // reserved / future: protocol, tls, metadata
}
// on Endpoint:
KubernetesProjection kubernetes = <next-free-tag>;
```

### 2. API surface (create / update / get / list)
- Accept + persist + return `kubernetes` on Endpoint **create** and **update**.
- **Validate** server-side: `service`/`namespace` are DNS-1123 labels, `port` in range, targets
  non-empty when the object is present, and (recommended) reject duplicate `(service,namespace)`
  within one endpoint.
- **List/poll:** the operator needs to enumerate the account's endpoints that carry the field.
  Two options — pick per effort:
  - *POC:* return `kubernetes` on the existing endpoints list; operator filters client-side.
  - *Productization:* **server-side filter** for "has kubernetes projection" + evaluate the
    operator's `endpoint_selectors`, returning only what this operator should project. Mirror
    the recently-added reserved-domains server-side filter (operator commit `e50f4c76`) — same
    pattern, far less data over the wire.

### 3. Storage
Persist `kubernetes` with the endpoint. No new datapath tables; this is descriptive metadata.

### 4. ngrok-api-go (Go client — third repo)
The operator imports `github.com/ngrok/ngrok-api-go/v9`. Add `Kubernetes *EndpointKubernetes`
(with `Targets []EndpointKubernetesTarget{Service, Namespace, Port}`) to the client's `Endpoint`
type and the create/update payloads. For the POC you can `go.mod replace` a local ngrok-api-go
checkout; productization = a real ngrok-api-go release bump in the operator.

**No mux / `/dial` / muxmap / hostport changes.** That's the point.

---

## Operator build

The operator already cleanly separates **dial identity** (`Spec.EndpointURL`) from **projection
target** (`Spec.Target{Service,Namespace,Port}`) on the `BoundEndpoint` CRD — today both happen
to derive from the same `svc.namespace` URL. This approach simply sources them from two places:
`EndpointURL` from the endpoint's `.internal` `url`, `Target` from a `kubernetes.targets[]`
entry. Most of the controller is unchanged.

### 1. Poller + aggregator
Two files:
- `internal/controller/bindings/boundendpoint_poller.go:214` — replace the k8s-binding poll
  `GetBoundEndpoints(r.koId, …)` (clientset iface `internal/ngrokapi/clientset.go:123`) with a
  **list of the account's endpoints** carrying `kubernetes.targets`, scoped by the operator's
  `endpoint_selectors` (server-side if built per above; else list-all + client-side filter on
  field presence + selector eval).
- `internal/ngrokapi/bindingendpoint_aggregator.go:36-63` — **this is the key seam.** Today
  `AggregateBindingEndpoints` parses `service`/`namespace`/`Target` out of the endpoint **URL**
  (the `EndpointTarget{}` build at ~L63). Re-source it: build `Spec.Target` from
  `endpoint.Kubernetes.Targets[i]` instead of the URL, and set `Spec.EndpointURL` to the
  endpoint's `.internal` `url` (the dial identity). Update the aggregator's table test
  (`bindingendpoint_aggregator_test.go`) accordingly.
- **Fan out over targets:** for each endpoint × each `kubernetes.targets[i]`, emit one desired
  `BoundEndpoint`:
  - `Spec.EndpointURL` = the endpoint's `.internal` url (the **dial** target, e.g.
    `tcp://foo.internal:80`) — shared across that endpoint's targets.
  - `Spec.Target` = `{Service, Namespace, Port}` from `targets[i]` (the **projection** target).
  - Name/`HashedName` = hash of `(endpoint.id + service + namespace)` so multiple targets of one
    endpoint get distinct BoundEndpoints and distinct allocated `Spec.Port`s (poller builds the
    `EndpointTarget` at `boundendpoint_poller.go:404`). Today `HashedName` hashes
    service+namespace; extend to include the endpoint id.
- Keep the poller as the source of zero-touch auto-discovery (unchanged product behavior).
- **`ngrok.Endpoint` needs the new field:** the aggregator consumes `ngrok.Endpoint`
  (ngrok-api-go v9), so the client type must carry `Kubernetes.Targets` — see the ngrok-api-go
  step above; without it the aggregator can't read the targets.

### 2. BoundEndpoint CRD — `api/bindings/v1alpha1/boundendpoint_types.go`
- **Relax the `EndpointURL` validation pattern** (L44). Today it enforces 2-label
  `service.namespace`; the dial host is now a `.internal` endpoint host (`foo.internal`, possibly
  multi-label `foo.bar.internal`). Widen the pattern to accept `.internal` hosts (or validate
  scheme+host+port more loosely). `foo.internal` already passes, but don't rely on the
  coincidence — make `.internal` first-class.
- No structural change needed for multi-target (we fan out to one BoundEndpoint per target). If
  you'd rather hold targets inline, you *could* add `Spec.Targets []EndpointTarget`, but that
  forces the forwarder's per-port listener to demux multiple targets on one port — avoid;
  one-BoundEndpoint-per-target reuses the existing 1 port ↔ 1 endpoint model.

### 3. Controller — `internal/controller/bindings/boundendpoint_controller.go`
- **Largely unchanged.** `convertBoundEndpointToServices` (L396) already builds the Target
  ExternalName Service from `Spec.Target` and wires the Upstream ClusterIP Service via the
  internal `<name>.<ns>.<ClusterDomain>` FQDN — none of that references the ngrok URL, so it
  keeps working. The Target Service is still `service.namespace`, so **clients address exactly
  what they do today**. No CoreDNS, no new names.

### 4. Forwarder — `internal/controller/bindings/forwarder_controller.go`
- **Egress swap** (L223-268), identical to [08 §Change 1/1b](08-poc-build-guide.md): replace the
  mTLS cert dial + `mux.UpgradeToBindingConnection` with a private-dial
  `DialContext(ctx,"tcp", host+":"+port)` where `host` is the `.internal` dial host parsed from
  `Spec.EndpointURL`. Keep `joinConnections`. Gate behind `USE_PRIVATE_DIAL`. Build the dialer
  once in `cmd/bindings-forwarder-manager.go` from a PAT + connect-URL flag.
- No `/dial` dependency — the `.internal` endpoint resolves over private dial as-is.

### 5. Auth & selectors
- Dial auth = account PAT (`ngrok_pat_*`); the endpoint is account-scoped internal-bound, so the
  PAT resolves it. No operator cert needed for the dial.
- `endpoint_selectors` on the `KubernetesOperator` resource still scope *which clusters* project
  a given endpoint. With the field as the discriminator, revisit the `["true"]` default
  (`cmd/api-manager.go:171`): it's now "project every field-carrying endpoint into this cluster,"
  which is defensible, but make it a conscious default.

### Pod identity — the one regression (GAT-475)
Same as the other private-dial approaches: `DialReq.metadata` (proto field 3) is unread by
`handleDial`, so `conn.k8s.pod.*` Traffic Policy silently regresses on a naive swap. POC-minimal
skips it (note loudly). Full parity = operator writes `PodIdentity` into `DialReq.metadata` +
backend reads it. This is the only place this approach touches the mux, and it's optional for the
POC.

---

## End-to-end test plan (both repos on the devbox)

1. **Backend:** add the `kubernetes` field (proto + API + storage + list) and the ngrok-api-go
   client field. Create a `.internal` endpoint via the API **with** `kubernetes.targets` set to
   two targets (`myservice.team-a:80`, `myservice.team-b:80`), backed by an HTTP echo AND a
   redis (for raw TCP). Confirm the field round-trips through create/get/list.
2. **Operator:** deploy with the new poller (filter on field), `USE_PRIVATE_DIAL=true`, PAT, and
   connect-URL → dial ingress.
3. **Projection:** confirm the operator created a Target ExternalName Service in **both** `team-a`
   and `team-b` (`myservice.team-a`, `myservice.team-b`), plus the Upstream Services + forwarder
   listeners.
4. **Reachability** from a client pod in each namespace:
   - `curl http://myservice.team-a/` and `.../team-b` → the echo backend (L7).
   - `redis-cli -h myservice.team-a ping` → `PONG` — **raw-TCP proof.**
   - Both namespaces reach the **same** underlying `.internal` endpoint via private dial.
5. **A/B:** `USE_PRIVATE_DIAL` off → falls back to mTLS (if you keep a k8s-bound endpoint around
   for comparison) or is simply disabled. Proves the swap is isolated.
6. **Field edits:** add/remove a target on the endpoint → operator adds/removes the corresponding
   Service (zero-touch). Delete the endpoint → all its Services removed.

### Success criteria
- A `.internal` endpoint with a **list** of `kubernetes.targets` projects into **each** target
  namespace as a `service.namespace` Service, reachable over **private dial** for HTTP and raw
  TCP, with **no `/dial`/mux change**, no CoreDNS, and clients addressing `service.namespace`
  exactly as today.
- Adding/removing targets or the endpoint reconciles Services correctly.
- Field round-trips through create/update/get/list and validates server-side.

### Out of scope / not blockers
Pod identity (GAT-475, stretch), server-side selector filtering (POC can filter client-side),
region pinning, throughput, the `.internal` in-cluster DNS mode ([07](07-transparent-internal-projection.md)).

---

## How this maps to tickets & the other options

- Still implements **K8SOP-292**'s datapath swap (private dial egress) and lets **K8SOP-293**
  (retire the kubernetes binding) proceed *properly* — the binding genuinely goes away here,
  replaced by real `private` endpoints + the projection field, which is the consolidation
  direction product wants.
- Chosen over [09](09-keep-k8s-binding-approach.md) (keeps the binding + needs a `/dial` change)
  and over the URL-convention (implicit, unvalidated). It's the explicit, extensible middle: a
  validatable server-side field that the dashboard/API/Terraform can drive, and that the operator
  turns into Services with the existing projection pipeline.
- Reuses the operator egress swap from [08](08-poc-build-guide.md) verbatim; the new work is the
  API field + the poller sourcing `Target` from the field instead of the URL.
