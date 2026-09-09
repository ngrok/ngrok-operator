# Feasibility: making `foo.internal` directly addressable in-cluster

> **Update / correction:** this doc concludes "no-Services ⇒ privileged TUN." That's true only if you insist on **zero Service objects**. If the real requirement is just "no namespace URI" (it is), there's a cheaper L4-correct middle path — keep an *invisible* per-endpoint Service and alias it via DNS — that this doc missed. See [07](07-transparent-internal-projection.md) for the corrected, serious treatment. Keep this doc for the L4-demux principle, which stands.

The appealing idea: skip namespace URLs and per-endpoint Services entirely, and make `foo.internal` resolve + connect from any pod — the way the agent does it. This is option E in [04](04-design-options.md). It's feasible, but there's a hard wall at L4.

## The easy half — DNS hijack

Pods resolve via CoreDNS (the `kube-dns` Service; its IP is injected into every pod's `/etc/resolv.conf` by kubelet). CoreDNS config is the `coredns` ConfigMap in `kube-system`. Add a zone:

```
internal:53 {
    forward . <operator-dns-service.clusterIP>
}
```

Now every `*.internal` query cluster-wide goes to a DNS server the operator runs — same shape as the agent's resolver. It can answer live (query mux `/get-host`: exists → A record, else NXDOMAIN), so no CoreDNS reloads as endpoints churn. This half is standard and works.

## The hard half — what IP do you answer with?

DNS collapses `foo.internal` to an IP. The connection then arrives carrying **no endpoint name** unless the protocol supplies one in-band:

- HTTP → `Host` header (L7).
- TLS → SNI (L6/handshake).
- **Raw TCP → nothing.** The forwarder sees bytes on `podIP:port` and cannot tell `foo` from `bar`.

**Any solution that only works for HTTP/TLS is not a solution.** So the question is how to demux raw TCP.

## The unifying principle

**L4 has no in-band name, so demuxing N endpoints requires N unique destinations.** There is no way around it. Every approach solves it the same way — give each name its own destination IP, assigned at resolution time:

- **Kubernetes Services today** → a unique ClusterIP per endpoint. Uniqueness chain: `svc.ns.svc` (unique name) → unique ClusterIP → unique forwarder port. By the time bytes reach the forwarder pod, the endpoint identity *is* the local port they landed on. The name→endpoint binding is established by DNS + Service routing before any byte flows.
- **The agent's TUN** → a unique *synthetic* IP per name (resolver hands out `100.64.0.7` for foo, `.8` for bar from a CGNAT range; the TUN device captures the packet and maps dest-IP → name).

Both are the same trick. HTTP/TLS are the only exception, because they smuggle the name in-band and a shared IP suffices.

So "DNS → one shared forwarder IP" can **only** ever do HTTP/TLS. For raw TCP it is fundamentally impossible: one IP can't disambiguate N names at L4.

## What that means for "no Services"

To make `foo.internal` TCP-addressable in-cluster **without** per-endpoint Services, you must mint a unique IP per name yourself → a **TUN DaemonSet** (per node, `NET_ADMIN`/privileged), exactly like the agent. There is no lighter middle path, because the CNI owns IP allocation — you can't conjure a routable IP range and point it at your pod. In Kubernetes, "a stable unique cluster IP" *is* a Service. So "unique IP per endpoint" ≈ "a Service per endpoint."

The real fork for a TCP-correct transparent mode:

1. **Keep per-endpoint Services** (today's plumbing), optionally expose the name as `foo.internal` via CoreDNS instead of `foo.ns.svc`. Services stay; the user-facing name changes. Passive-ish, no privileged pods.
2. **TUN DaemonSet** — synthetic per-name IPs, no Services, full agent parity. Privileged, per-node, non-passive.

There is no option that is simultaneously (no per-endpoint Services) + (works for raw TCP) + (no privileged TUN). Pick two. Since raw TCP is non-negotiable, the choice is really **Services-with-nicer-DNS-names vs a TUN DaemonSet**.

## Operational warts (why E is non-passive)

- **Editing cluster DNS is elevated and provider-specific.** Mutating a `kube-system` ConfigMap needs cluster-scoped RBAC (the operator is namespaced today). And each provider differs: AKS has a sanctioned `coredns-custom` ConfigMap (import plugin); EKS CoreDNS is a managed addon that can **revert** edits on upgrade; GKE defaults to **kube-dns** (not CoreDNS) with a different `stubDomains` mechanism, or Cloud DNS. "Edit CoreDNS" is really N provider integrations.
- **`.internal` collides with GCP.** GCE/GKE nodes already use `*.internal` for instance DNS (`host.zone.c.project.internal`). A cluster-wide `internal` zone forwarding to us can shadow GCP's names. ICANN reserved `.internal` in 2024 partly to stop this, but GCP's usage predates it and is live.
- **Loses the real Service object.** No `kubectl get svc`, and — bigger — **NetworkPolicy can't target it** (policies match Services/pods/labels, not a synthetic DNS name). Some users specifically want the Service for policy and discovery. So E is arguably *additive*, not a replacement.

## Verdict

E is plausibly the nicest v1 *end-state* and it reuses exactly what the agent (ngrokd / agent-v4 M2) is building, but for full protocol correctness it requires a privileged TUN DaemonSet and cluster-admin DNS integration. That makes it a non-passive change, unfit as the *migration* path. Two facts decide whether it's ever worth it:

1. **How much bound-endpoint traffic is raw TCP vs HTTP/TLS?** If mostly HTTP/TLS, a shared-forwarder + SNI/Host demux gets you most of the way with no TUN. If lots of raw TCP, you're pushed to TUN.
2. **Do users need the real Service object** (NetworkPolicy, discovery), or is a resolvable name enough? If they need it, E can't fully replace Service projection — it augments it.
