# The gap

The product docs (Private Endpoints 1-Pager, Private Dial RFC, agent spec) cover the binding consolidation and the dial protocol. **None of them address how the operator materializes an in-cluster Kubernetes `Service` for a private endpoint.** That omission is the whole problem.

## Where the target lives today vs under `private`

Today the kubernetes endpoint URL *is* the target:

```
https://my-service.my-namespace   →  Service "my-service" in namespace "my-namespace"
       └── service ──┘└ namespace ┘
```

The operator parses `service` and `namespace` out of the host and builds the Service. It's the only source of that information — there is no structured field (see [01](01-current-state.md)).

A private endpoint URL is:

```
https://foo.internal
        └ one account-global name, no namespace segment ┘
```

There is nothing to parse a target service or namespace from. `foo.internal` is one name for the whole account, not a per-namespace address. So the operator has no idea what Service to create, named what, in which namespace.

## The "double-duty binding" insight

The `kubernetes` binding is doing **two independent jobs**, and consolidation deletes both:

1. **Projection discriminator (poll side).** `GetBoundEndpoints(koId)` returns endpoints that are kubernetes-bound and match the operator's selectors. The binding value is how the server and operator know "this endpoint is meant to become a Kubernetes Service."

2. **Mux resolution scope (dial side).** Endpoints are indexed in the muxmap by `(host, port, binding)`. The kubernetes mТLS path resolves against the **kubernetes-operator** binding key; private dial resolves against the **internal** binding key. They are different index entries even for the same host:port (verified — see [06](06-poc-and-findings.md)).

Remove the `kubernetes` binding and you lose the "project me into k8s" signal (job 1) **and** the endpoint stops being resolvable by the kubernetes path — it has to live in the internal/private index instead (job 2). The datapath swap and the binding consolidation are therefore **coupled**, not independent as they first appear.

## Two things need re-homing

When `kubernetes` → `private`:

- **(a) intent** — "project this private endpoint as a Kubernetes Service." Was: `bindings == kubernetes`.
- **(b) target** — service name + namespace. Was: the URL host.

And one thing survives:

- **(c) scoping** — which cluster(s) project it. Still `endpoint_selectors` (CEL), applied at poll time.

Every design option in [04](04-design-options.md) is really an answer to "how do we supply (a) and (b)."

## The consolidation-vs-namespace tension (the reason this is hard)

Stated plainly (from the Slack thread):

> A private endpoint you want projected into k8s has platform-specific requirements (a namespace, an L4-addressable Service) that make a *default* private endpoint unsuitable for k8s — unless we completely change how k8s bindings work so they no longer use namespaces.

The consolidation goal says "one `private` binding, no platform specifics." But Kubernetes L4 addressing **requires a per-name destination** (a Service in a namespace), because a raw TCP connection carries no hostname to demux on ([05](05-dns-tun-feasibility.md)). So a private endpoint that's genuinely useful in k8s inherently carries platform-specific shape. The only way to honor the consolidation literally is to make k8s bindings stop using namespaces — the DNS/TUN route — which is privileged, provider-specific, and loses NetworkPolicy.

This is a genuine product contradiction. It doesn't resolve by being clever; someone has to decide which side bends:
- **Keep namespaced Services** → private endpoints destined for k8s need *some* platform-specific signal (a field, a URL convention, or a mapping resource). Consolidation isn't fully "clean."
- **Drop namespaced Services** → transparent `foo.internal` in-cluster via DNS/TUN. Clean consolidation, but a heavier, non-passive operator and no NetworkPolicy targeting.

## Constraints we're operating under

- **Real Services are required.** Client pods must reach services with plain HTTP/TCP and cannot be forced onto an SDK. NetworkPolicy targeting matters to some users.
- **Zero-touch is required.** "Make an endpoint once, it appears in every cluster with the operator installed." We were explicitly told the "author a CR in each cluster yourself" model is not viable. This kills the otherwise-most-k8s-native option (see [04](04-design-options.md), option D).
- **`endpointSelectors: ["true"]` becomes dangerous.** Today it means "project all *kubernetes* endpoints." Post-consolidation there is no kubernetes binding, so `true` would try to project *every private endpoint in the account* as a Service in *every* cluster. So some discriminator for intent (a) is mandatory — "do nothing / it's all magic" is not on the table.
