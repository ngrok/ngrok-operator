# Private Dial migration — working notes

Migrating the ngrok-operator off **kubernetes bindings** onto **private endpoints + private dial**. These docs replace the old Notion "Private Dial Notes" page; work happens here now.

**Status:** spike / design exploration. Nothing committed to an approach. No production code yet.
**Owner:** Alex
**Last updated:** 2026-09-09

---

## The one-paragraph version

Product is consolidating three endpoint bindings (`public`, `internal`, `kubernetes`) into two (`public`, `private`). The `private` binding is account-scoped and resolves under the `.internal` TLD, reachable via a new connectivity primitive called **Private Dial** (`ngrok.Dial("foo.internal:80")`). The operator's job today depends on the `kubernetes` binding in two ways that both disappear under consolidation: it uses the `service.namespace` URL to know what Kubernetes `Service` to create, and it uses the binding type to make the endpoint resolvable to the in-cluster forwarder. Neither survives a plain `foo.internal` private endpoint. This is the gap we're working through.

## Read in order

1. [01-current-state.md](01-current-state.md) — how kubernetes bindings work end-to-end today (operator + backend), with file refs.
2. [02-private-endpoints-and-dial.md](02-private-endpoints-and-dial.md) — the two initiatives, the dial protocol, and what's built vs in progress.
3. [03-the-gap.md](03-the-gap.md) — the core problem, the "double-duty binding" insight, the consolidation-vs-namespace tension.
4. [04-design-options.md](04-design-options.md) — every option we've weighed (field / resource / URL convention / in-cluster CR / DNS-hijack), with tradeoffs.
5. [05-dns-tun-feasibility.md](05-dns-tun-feasibility.md) — deep dive on the "make `foo.internal` directly addressable in-cluster" idea and why L4 demux is the wall.
6. [06-poc-and-findings.md](06-poc-and-findings.md) — the POC plan, and the verified backend finding that private dial can't resolve kubernetes-bound endpoints today.
7. [references.md](references.md) — every Linear ticket, Slack thread, RFC, doc, and code file referenced.

## Decisions made so far

- **Keep real per-namespace Kubernetes Services** as the primary in-cluster addressing. Client pods must reach services with plain HTTP and cannot be forced onto an SDK. A transparent `foo.internal` in-cluster DNS mode is at most a later, opt-in add-on.
- **Mapping stays server-side / poller keeps auto-discovering.** The zero-touch "endpoint appears in every cluster with the operator installed" behavior is a product requirement (we were explicitly told the "make the CR in each cluster yourself" model is not viable).

## The biggest open question (for Euan / Gateway)

Private dial's `/dial` handler hardcodes the **internal** binding when it resolves a host, and kubernetes-bound endpoints live under a **different** muxmap index — so private dial cannot resolve them today (see [06](06-poc-and-findings.md)). For the migration, do we:

1. **Teach `/dial` to also resolve the kubernetes-operator binding** (small handler change; keeps two binding types alive), or
2. **Make these endpoints `private`-bound** via consolidation (cleaner; but re-opens "how does the operator know which private endpoints to project" — the discriminator question in [04](04-design-options.md))?

## Next steps

- Put the question above to Euan in `#proj-...` (the [options thread](https://ngrok.slack.com/archives/C07K9EDUTTK/p1788909819619689) is the starting point).
- Build the **minimal datapath POC** (operator forwarder private-dials a plain `.internal` endpoint, no k8s projection) — unblocked, needs no backend change. See [06](06-poc-and-findings.md).
- When a backend change is unavoidable, develop against the mono repo devenv.
