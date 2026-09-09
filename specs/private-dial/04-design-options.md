# Design options

All framed as: **how does a `private` endpoint declare intent (a) "project me into k8s" and target (b) "as Service X in namespace Y"?** (See [03](03-the-gap.md) for why that's the question.)

Scoping (c) — which clusters — stays as `endpoint_selectors` (CEL, poll time) in every option.

---

## A. Field on the Endpoint API resource

Add a field to the endpoint that designates the k8s target. Two sub-variants:

### A1. Kubernetes-specific field

A structured `kubernetes: {service, namespace}` (or a list, for multi-namespace/multi-cluster). The poller filters endpoints that carry it and reads service/namespace from it instead of the URL.

- **Pro:** simplest backend; smallest operator change; explicit, opt-in, validatable; unambiguous.
- **Con:** a platform-specific field on the otherwise platform-agnostic endpoint. Feels like a one-off that can't be removed passively. Fights the consolidation's "no platform specifics" intent (though as *optional operator-only metadata*, not a binding type).

### A2. Non-specific "alternative names" / SAN field

A generic list of alternative names (think TLS SANs) that also serves the agent world. The operator projects any entry shaped `part1.part2` where `part2` matches a namespace in its cluster.

- **Pro:** not k8s-specific on the API surface; reusable for the agent; one endpoint can be both a normal `foo.internal` *and* k8s-projected (SAN `foo.myns`) without a second endpoint.
- **Con:** magic — the "only names matching a namespace get projected" behavior is confusing; dashboard has to stitch it into a friendly UX; no validation of typos.

---

## B. New API resource mapping k8s URL → endpoint

A separate resource (e.g. `EndpointProjection` / `KubernetesBinding`) that references an endpoint (by URL/ID, or via a selector) and carries the k8s URL / service / namespace. Operator polls that.

- **Pro:** keeps the endpoint resource clean; natural home for k8s specifics; can iterate independently; naturally supports many-namespace / many-cluster; dashboard can auto-create + wire it to an endpoint.
- **Con:** new API surface and a creation/UX story (dashboard/API/terraform). More moving parts than a field.

---

## C. URL convention on the private endpoint (`service.namespace.internal`)

Don't map anything. Let the private endpoint's URL *be* the k8s address, in a 3-label form under `.internal`:

- `foo.internal` (2-label) → plain account-global private endpoint (laptop/agent/forward-internal). **Not projected.**
- `svc.ns.internal` (3-label) → project as Service `svc` in namespace `ns`.

The operator projects any `*.internal` endpoint whose host parses as `service.namespace.internal` where the namespace exists in the cluster.

- **Pro:** **zero new API surface**; the backend stays fully platform-agnostic (it just sees a private endpoint hostname; only the operator ascribes k8s meaning); **symmetric with a convention already in this repo** — `endpoints-verbose` mapping already projects cluster Services *outbound* as `svc.ns.internal` (`specs/mapping-strategy.md:56`).
- **Con:** internal endpoints legally allow multiple subdomains, so `foo.bar.internal` could be a legit *non-k8s* endpoint the parser wrongly grabs. The "namespace must exist" guard mitigates but makes intent **implicit/environmental** with no server-side typo validation (`svc.prd.internal` silently wrong). Can't take an existing `foo.internal` and *also* project it — you'd make a second endpoint that `forward-internal`s to the first (product already blesses this, but it's two names to manage).

This is the strongest low-effort baseline. A1/A2/B differ from it mainly on dual-use (one endpoint being both agent-global and k8s-projected) and on explicit-vs-implicit intent.

---

## D. User authors the `BoundEndpoint` CR in-cluster — **RULED OUT**

The most k8s-native option: the user creates a `BoundEndpoint` (or new CR) in the cluster saying "take this ngrok endpoint and expose it here as this Service." Explicit, local, no server-side k8s concept.

**Dead** because it inverts the required product model. We were explicitly told during the original bindings implementation that it must auto-bind into *all* clusters just by creating an endpoint; making the CR in each cluster is not viable. Recorded here so it isn't re-proposed.

---

## E. Transparent in-cluster DNS (operator behaves like the agent) — **maybe a later v1, not a migration**

Make `foo.internal` directly addressable from any pod by hijacking cluster DNS, no namespace URL. Deep dive in [05](05-dns-tun-feasibility.md); serious end-state treatment (and the correction that this does **not** require dropping Services / a privileged TUN) in [07](07-transparent-internal-projection.md).

- **Pro:** cleanest end-state; `foo.internal` works identically on pod, agent, laptop; rides the exact infra the agent team is already building.
- **Con:** requires cluster-admin (edit CoreDNS/kube-dns — provider-specific), a privileged per-node TUN DaemonSet to support raw TCP, collides with GCP's `.internal` DNS, and **loses the real Service object** (no NetworkPolicy targeting, no `kubectl get svc`). Non-passive change to how the product works. Best treated as an additive future mode, not the migration path.

---

## Cross-cutting: the intent discriminator

| Option | How intent (a) is signaled | How target (b) is carried | New API surface | Server stays k8s-agnostic |
|---|---|---|---|---|
| A1 field | presence of field | structured field | small | no |
| A2 SAN | name matches a namespace | SAN list entry | small | mostly |
| B resource | resource exists | the resource | medium | yes |
| C convention | 3-label `.internal` + ns exists | the URL | **none** | **yes** |
| D in-cluster CR | the CR | the CR | none (server) | yes — but **ruled out** |
| E DNS/TUN | n/a (everything reachable) | n/a (no Service) | none | yes |

Reminder: whatever the choice, `endpointSelectors` default of `["true"]` must change or narrow, because post-consolidation it would otherwise project every private endpoint into every cluster ([03](03-the-gap.md)).
