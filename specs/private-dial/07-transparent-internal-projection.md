# Transparent `*.internal` projection into the cluster (the "no namespace URIs" end-state)

**Question being answered:** what would it *actually* take to make a real `foo.internal`
private endpoint directly addressable from any pod in the cluster — over **TCP, not just
HTTP** — with **no `service.namespace` URL convention** and **no fake `internal` namespace**?

This is [option E](04-design-options.md) taken seriously as the *target end-state*, not as
the migration path. The prize, if it works: the operator stops minting per-namespace name
conventions entirely. `foo.internal` resolves the same on a laptop, in an agent, and in a
pod. Endpoint identity for policy comes from **Traffic Policy pod variables**
(`conn.k8s.pod.*`, GAT-475), not from which namespace a Service happened to live in. If
product blesses dropping the namespace model, this is the cleanest thing we could ship.

The reason to write it down before pitching it: the honest cost is **cross-provider DNS
integration** plus a choice about **how to demux L4**. This doc separates those two halves,
because they have very different difficulty, and corrects an over-pessimistic claim in
[05](05-dns-tun-feasibility.md) ("it's TUN or nothing"). It isn't.

---

## The problem decomposes into two independent halves

Making `foo.internal` work in-cluster is two separate mechanisms that people conflate:

1. **Resolution** — a pod does `getaddrinfo("foo.internal")` and gets *an* IP back.
   This is a DNS problem. It is the same for every approach below.
2. **Demux** — bytes land on that IP and the forwarder must know they belong to `foo`,
   not `bar`, with no hostname in the packet (raw TCP). This is a routing problem, and it
   is where the real fork is.

You must solve **both**. Solving only resolution gives you an HTTP/TLS-only toy (Host/SNI
carry the name in-band). Raw TCP is non-negotiable here, so demux is mandatory.

---

## Half 1 — Resolution: the cross-provider DNS reality

Every pod's `/etc/resolv.conf` points at the cluster DNS Service (CoreDNS on most,
kube-dns on default GKE). To answer `*.internal` we insert a **stub zone / forward rule**
that sends `*.internal` queries to a DNS server the operator runs. That operator-DNS
answers live from the endpoint set (endpoint exists → A record; else NXDOMAIN), so no DNS
reloads as endpoints churn. The resolver logic is trivial and identical everywhere.

**The hard part is inserting that rule, and it is genuinely N provider integrations.**
This is the piece to be honest with product about. Matrix:

| Platform | Cluster DNS | Insertion mechanism | Survives upgrades? | Notes |
|---|---|---|---|---|
| **AKS** | CoreDNS | sanctioned `coredns-custom` ConfigMap (`import ready custom/*.override`) | **Yes** — designed for this | Cleanest. First-class extension point. |
| **kubeadm / kOps / k3s / RKE / bare** | CoreDNS | edit the `coredns` ConfigMap directly (add a server block) | Yes (we own it) | Fine. Standard. |
| **EKS** | CoreDNS (managed addon) | edit `coredns` ConfigMap; or addon "advanced configuration" | **Risky** — addon updates can revert manual ConfigMap edits | Must use the addon config API or re-apply after upgrades. |
| **GKE (default)** | **kube-dns**, not CoreDNS | `stubDomains` JSON in the `kube-dns` ConfigMap | Yes | Different mechanism entirely. And see the `.internal` collision below. |
| **GKE (Cloud DNS mode)** | Cloud DNS | GCP-managed; no in-cluster ConfigMap | Yes | Needs GCP API calls, not kubectl. A third code path. |

Two things fall out of this table:

- **It needs cluster-admin.** The operator is namespaced today; editing a `kube-system`
  ConfigMap (or calling a cloud DNS API) is a privilege escalation and an install-time RBAC
  ask. Non-trivial for security-conscious users.
- **It is not one integration.** "Edit CoreDNS" is really *four* code paths (AKS-custom,
  raw-CoreDNS, EKS-addon, GKE-kube-dns) plus a GCP-Cloud-DNS path. Each is individually
  documented and tractable — none is research-grade — but the surface is real and it is
  ongoing maintenance as providers change their DNS story.

### The `.internal` collision (a correctness wart, GCP-specific)

GCE/GKE nodes already use `*.internal` for instance DNS
(`hostname.zone.c.project-id.internal`). A cluster-wide `internal:53` forward to us would
**shadow** those names. ICANN reserving `.internal` in 2024 was partly to stop exactly this
collision, but GCP's usage predates it and is live in every GKE cluster today. Mitigation:
our operator-DNS must **fall through** — answer only for hosts that are known ngrok
endpoints, and forward everything else back to the platform resolver. CoreDNS `forward`
with a fallthrough is expressible, but ordering/precedence is fiddly and it's one more thing
that behaves differently on GKE. Flag it explicitly in any product pitch.

### Can we avoid touching cluster DNS at all?

Two escape hatches, both worse:

- **Mutate workload pods' `dnsConfig`** (nameservers + search) via an admitting webhook, so
  only opted-in pods resolve `.internal` through us. Avoids cluster-admin on kube-system,
  but is *invasive* (rewrites user pods), *not zero-touch* (per-pod opt-in), and adds a
  webhook. Fails the "appears everywhere automatically" product requirement.
- **NodeLocal DNSCache** customization — still a cluster-wide, provider-specific edit; no win.

**Conclusion for Half 1:** resolution is *doable everywhere* but the DNS insertion is the
single biggest cross-provider tax, it demands cluster-admin, and it is shared by **every**
demux option below. If this half is a dealbreaker for a customer's security team, the whole
transparent-mode idea is off the table for them regardless of how we demux — which is
exactly why per-namespace Services (which need none of this) stay the safe default.

---

## Half 2 — Demux: there are TWO L4-correct mechanisms, not one

[05](05-dns-tun-feasibility.md) framed this as "unique IP per name = a Service, therefore
it's Services-or-TUN and TUN is the only no-Service option." The unique-IP principle is
right; the "only TUN" gloss undersold the practical middle. Precisely:

> A unique routable destination IP can be realized **either** by a Kubernetes Service
> (kube-proxy programs the node to route its ClusterIP to a backend) **or** by a synthetic
> IP captured by a kernel datapath you own (TUN, or eBPF). Those are the two families.

That gives two concrete mechanisms, with very different build cost.

### Mechanism B — per-endpoint ClusterIP Service + CoreDNS alias  ← the cheap one that still delivers the dream

Keep minting a real `Service` per endpoint (we already create Services today), but change
**only the name the user types**:

- Operator creates a ClusterIP Service (in the *operator's own* namespace — invisible
  bookkeeping, the user never references it) whose endpoints are the forwarder pods.
- Operator-DNS answers `foo.internal` → that Service's ClusterIP (or CNAME to
  `foo.ngrok-system.svc.cluster.local`).
- Bytes hit the ClusterIP → kube-proxy delivers to the forwarder → forwarder knows the
  endpoint because **it's the ClusterIP/port they landed on** (same demux we rely on today).
- Forwarder private-dials `foo.internal` upstream. Done.

What this buys, measured against the user's actual goal:

- ✅ **The namespace URI is gone.** Clients address `foo.internal`. No `foo.myns`. The
  Service still lives in *a* namespace (Services must), but it's the operator's namespace
  and **the client never names it** — which is the whole objection to the "fake internal
  namespace" hack. This is categorically different from making users type `foo.internal-ns`.
- ✅ **No TUN. No privileged/`NET_ADMIN` DaemonSet.** The only elevated thing is the DNS
  edit (Half 1), which *every* option needs anyway.
- ✅ **NetworkPolicy still works** and `kubectl get svc` still shows something. Keeps the
  real Service object that some users explicitly want.
- ✅ **Reuses today's forwarder datapath** almost verbatim — this is close to what we
  already build, minus the per-namespace naming.
- ❌ **Doesn't scale to huge endpoint counts.** One ClusterIP + kube-proxy ruleset per
  endpoint. iptables-mode kube-proxy is O(n) in rules; thousands of endpoints bloat every
  node's iptables and slow reprogramming. IPVS mode softens it. But this is a real ceiling
  (and it's *already* our ceiling today — we make **two** Services per endpoint now).

**Mechanism B is the headline finding of this doc.** It delivers "address `foo.internal`,
no namespace convention" *without* a TUN DaemonSet and *without* losing NetworkPolicy. The
earlier "you need a privileged TUN" conclusion only holds if you also insist on **zero
Service objects**. If "no *namespace URI*" is the real requirement (it is) rather than "no
Service *object*", B is the pragmatic answer.

### Mechanism A — TUN DaemonSet (full agent parity)

Rebuild what the agent does (see the M2 DNS+TUN datapath): per node, a privileged DaemonSet
with a TUN device. Operator-DNS hands out **synthetic** IPs from a CGNAT-style pool
(e.g. `100.64.0.0/10`); the TUN captures packets to those IPs, maps dest-IP → endpoint name
from an in-memory table, and relays the TCP stream over private dial (userspace netstack or
splice). No Services at all.

- ✅ **No Service objects, no kube-proxy** — synthetic IPs sidestep the O(n) rule bloat.
  Scales to very large endpoint counts far better than B.
- ✅ **Exact agent parity** — reuses the datapath the agent team is already building, so
  behavior is identical laptop/agent/pod. If that code is liftable (see the lift assessment
  below), the marginal build cost drops.
- ❌ **Privileged, per-node DaemonSet** (`NET_ADMIN`, `/dev/net/tun`). Much bigger security
  ask than B; many clusters forbid it via PodSecurity/OPA.
- ❌ **Loses the real Service object** — no NetworkPolicy targeting, no `kubectl get svc`.
- ❌ **Most code to own** — TUN lifecycle, IP pool GC, userspace TCP, per-node health.

### Mechanism C — eBPF redirect (footnote, not recommended)

Instead of a TUN, an eBPF program (sockops/`connect` hook or TC) rewrites/redirects
connections to synthetic IPs into the forwarder. Still privileged, still per-node, more
CNI-version-sensitive than TUN, and no NetworkPolicy win. Strictly harder than A for the
same downsides. Mentioned only so it's on record as considered.

---

## Scale is the real axis between A and B

The A-vs-B decision isn't taste, it's **endpoint cardinality**:

- **Tens–hundreds of endpoints per cluster:** Mechanism **B** wins outright. Cheap, safe,
  keeps NetworkPolicy, no privileged pods. The kube-proxy cost is irrelevant at that size.
- **Thousands+ endpoints per cluster:** kube-proxy rule bloat makes B painful; **A**'s
  synthetic IPs are the scalable datapath. But you pay privilege + lose Services.

So the honest framing for product: *B is the default transparent mode; A is the escape hatch
for scale we don't have evidence anyone hits yet.* Which means the expensive, scary part
(privileged TUN across providers) is **not** on the critical path for shipping the
"`foo.internal` in-cluster" experience — B ships it. That materially de-risks the pitch.

---

## Pod identity: the piece that makes "ditch namespaces" actually safe

Today, some of the endpoint's meaning is *carried by the namespace* the Service lives in.
Drop the namespace convention and that signal has to come from somewhere. It does:
**Traffic Policy pod variables**. The forwarder attaches the client pod's identity to the
dial, and TP expressions read `conn.k8s.pod.namespace` / `.name` / `.labels.*` server-side.

- This is **GAT-475** — plumb pod identity through `DialReq.metadata` and expose it as
  `conn.k8s.pod.*` in TP CEL. It's already a hard dependency of the datapath swap
  (K8SOP-292); [06](06-poc-and-findings.md) notes those policies silently regress without it.
- It is **orthogonal to A vs B** — both mechanisms have a forwarder that can stamp pod
  identity onto the dial. So the "rely on Traffic Policy with pod variables instead of
  namespaces" bet the user wants to make is enabled by GAT-475 regardless of demux choice.
- **This is the actual gating question for product**, more than the DNS plumbing: *are we
  comfortable that per-namespace Services are replaceable by TP pod-variable policies for
  identity/authz?* If yes, the namespace model can go. If no, transparent mode can only ever
  be *additive* (a nicer name on top of Services), never a replacement.

---

## What breaks / what you keep

| Property | Per-namespace Services (today) | **B: Service + DNS alias** | **A: TUN DaemonSet** |
|---|---|---|---|
| Client address | `foo.myns` | `foo.internal` | `foo.internal` |
| Namespace URI convention | required | **gone** | **gone** |
| Privileged per-node pod | no | **no** | **yes** (`NET_ADMIN`) |
| Cluster-admin DNS edit | no | yes (Half 1) | yes (Half 1) |
| Real Service object | yes (2 per ep) | yes (1 per ep) | **no** |
| NetworkPolicy targeting | yes | yes | **no** |
| `kubectl get svc` | yes | yes | no |
| Scales to 1000s of endpoints | poor (2 svc/ep) | poor (1 svc/ep) | **good** |
| New code to own | — | small (naming + DNS) | large (TUN/netstack/pool) |
| Raw TCP correct | yes | yes | yes |

---

## Lift assessment — how much of the agent's DNS+TUN is reusable (Mechanism A)

*Surveyed `~/code/ngrok` (server mux tree) for the M2 client datapath. Verdict: the pieces
split cleanly into "already portable" and "not in these repos, must be assessed against the
agent tree."*

**Confirmed portable (good for A, B, and the POC):**

- **The private-dial relay is already a standalone dialer package**, not welded to anything.
  It's `golang.ngrok.com/ngrok/privatedial`, imported as `ngrokdial` at
  `local/ngrok/go/svc/ship/core/privatedial/privatedial.go:14`, and exposes a plain
  `Dialer.DialContext(ctx, "tcp", "host:port")`. So lift-question 3 ("is the relay factored
  for reuse?") is a clean **yes** — a DaemonSet (A), the forwarder (B), or the POC can all
  call `DialContext` per flow without dragging in agent runtime. This is the same package
  the minimal POC in [06](06-poc-and-findings.md) uses.
- **The server already exposes the live-resolution endpoints the operator-DNS needs.**
  `/get-host` (`go/svc/mux/privatedial/handler.go:918-957`) answers "does this host exist?"
  (name only, no port — exactly a DNS A/NXDOMAIN decision) and `/get-hostport`
  (`:963-1024`) probes a specific `host:port` before dial. So Half 1's "operator-DNS answers
  live from the mux, no CoreDNS reloads" is backed by real endpoints, not hand-waving. This
  is independent of A vs B.

**Not in these repos — the actual blocker on finishing the A lift estimate:**

- The agent's **client-side DNS server + synthetic-IP allocator + TUN datapath** (the M2
  work) is **not** in `~/code/ngrok` (that tree is the *server* mux) nor in the operator.
  It lives in the agent/ngrokd tree and is described in the product spec
  [`ngrok-agent-private-dial.md`](https://github.com/ngrok-private/product/blob/main/specs/private-dial/ngrok-agent-private-dial.md).
  So the two questions that actually decide A's cost are **still open** and need that tree:
  1. Is the resolver + IP-pool allocator a **portable Go package**, or welded to agent
     session/config and desktop OS assumptions (`/etc/resolv.conf`, macOS/Windows utun)?
  2. Is the TUN datapath **gvisor/netstack userspace TCP** (container-friendly, no host
     routing games) or does it lean on host routes/iptables that would fight the CNI?

**Net:** the *relay* (the hard-looking part) is already reusable, and the *server-side
resolution API* exists — so **B and the POC are fully unblocked by portable code today**.
Only **A's TUN/resolver lift** is unresolved, and finishing that estimate requires reading
the agent/ngrokd tree, not these repos. That's another reason to ship B first and treat A as
a later, separately-scoped effort: B needs nothing we don't already have in hand.

---

## Recommendation / how to de-risk before pitching product

1. **Pitch Mechanism B, not TUN.** It gives product the visible win — `foo.internal`
   addressable in-cluster, namespace URIs gone — with no privileged DaemonSet and while
   *keeping* NetworkPolicy. That's a far easier "yes" than "we need a privileged per-node
   agent in your cluster."
2. **Make the DNS integration cost explicit and bounded.** Present the provider matrix
   above as the true cost. Land AKS + raw-CoreDNS first (cleanest), then EKS-addon, then
   GKE. Call out the `.internal`/GCP collision as a known wart with a fallthrough mitigation.
3. **Get the product decision that actually gates this**, framed as: *"Can Traffic Policy
   pod variables (GAT-475) fully replace per-namespace Services for identity/policy?"* If
   yes → transparent mode can replace the namespace model. If no → it's additive only, and
   we keep Services as the substrate (which B does anyway).
4. **Hold TUN (A) as the scale escape hatch**, justified only when a customer demonstrably
   blows past kube-proxy's per-endpoint ceiling. Don't put it on the shipping critical path.
5. **Cheap validations to do before committing** (each is a half-day):
   - Stand up a CoreDNS stub zone → dummy operator-DNS in kind; confirm a pod resolves
     `foo.internal` and NXDOMAIN fallthrough works.
   - Prove B end-to-end in kind: operator-namespace ClusterIP Service + CoreDNS alias +
     forwarder → private-dial upstream, over **raw TCP** (e.g. redis/postgres), confirming
     L4 demux by landing-IP works with the `.internal` name.
   - On EKS specifically, edit CoreDNS via the addon config path and confirm it survives a
     forced addon update (validates the riskiest cell in the matrix).

**Bottom line:** the transparent `foo.internal` experience is *not* "incredibly hard across
providers" **if** we accept Mechanism B (keep an invisible per-endpoint Service, alias it
via DNS). The genuinely hard/uncertain parts — privileged TUN, dropping the Service object —
are only required for the pure/no-Service/high-scale variant (A), which we can defer. The
one unavoidable cross-provider tax is the DNS stub-zone insertion, and it's a bounded,
known-mechanism-per-provider job, not research. The real *product* gate is pod-identity via
Traffic Policy, not the plumbing.
