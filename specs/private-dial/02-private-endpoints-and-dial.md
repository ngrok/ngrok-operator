# Private Endpoints and Private Dial — the two initiatives

These are **two separate workstreams** that get talked about in the same breath. Keeping them apart is essential.

## Private Endpoints (the binding-model consolidation)

Owner: product (Niji / Brian Melton-Grace). [1-Pager](https://linear.app/ngrok/document/private-endpoints-product-1-pager-20654e9d0ce9).

Today ngrok has **three** bindings:

| Binding | Availability | Platform requirement |
|---|---|---|
| `public` | Internet-accessible | none |
| `internal` | Account-scoped, reachable only via `forward-internal` | none |
| `kubernetes` | Account-scoped, reachable only inside a cluster running the operator | Kubernetes |

Private Endpoints collapse `internal` + `kubernetes` into a single **`private`** binding, leaving **two**: `public` and `private`.

Key properties of `private`:
- Account-scoped, not internet-reachable, not discoverable outside the account.
- **Resolves under the `.internal` TLD** (e.g. `https://api.internal`). The `.private` TLD floated early was dropped — ICANN reserves `.internal` for private use; squatting `.private` runs against ICANN guidance.
- Namespaced per account (two accounts can both have `api.internal`, no conflict, no reservation).
- No platform dependency — works on Linux/macOS/Windows/containers, not just Kubernetes.
- `forward-internal` keeps working, both as a target and a forwarding layer (hub-and-spoke topologies unaffected).
- Auth is optional (account boundary is the default access-control layer); Traffic Policy still applies.

The 1-Pager is explicit that it covers **only the binding-model consolidation**. It says nothing about how the operator materializes an in-cluster Service — that's the gap ([03](03-the-gap.md)).

## Private Dial (the connectivity primitive)

Owners: Gateway (Euan) + Brian Melton-Grace. [Initiative](https://linear.app/ngrok/initiative/private-dial-4773dcadb830/overview) · [RFC](https://github.com/ngrok-private/wiki/blob/main/rfcs/2026-04-29-private-dial.md).

Private Dial is how anything in the account reaches a private endpoint **directly by URL** — no public URL in front, no cluster, no `forward-internal` hop. It productizes the [ngrokd](https://github.com/ngrok-oss/ngrokd-go) flow.

### SDK shape

```go
dialer := ngrok.NewPrivateDialer(ctx, ngrok.WithAuthtokenFromEnv())
conn, err := dialer.DialContext(ctx, "tcp", "foo.internal:80") // net.Conn
```

### Protocol

- Ingress hostnames: `quic.connect-endpoint.ngrok.com` (HTTP/3 over QUIC) and `h2.connect-endpoint.ngrok.com` (HTTP/2 over TCP/443), with regional variants `…<region>.ngrok.com`. Dev: `connect-endpoint.dev-ngrok.com`.
- Transport chosen via a Happy-Eyeballs-style race (prefer QUIC, fall back to H2), sticky per process.
- Length-prefixed protobuf framing (`libmux.ReadProxyMessage` / `WriteProxyMessage`).
- Four "endpoints":
  - `/session` — long-lived control stream: client/server metadata, Ping/Pong liveness, `PleaseDrain`, `SessionError`.
  - `/dial` — per-target stream: `DialReq{host, port, metadata}` → `DialResp{endpoint_id, metadata}`, then the stream becomes raw TCP. Errors after the response come via a response header/trailer, not in-band.
  - `/get-host` — does this host have any endpoint? (lets the agent answer a DNS query with A vs NXDOMAIN without a port).
  - `/get-hostport` — metadata for a specific host:port without dialing.
- **Auth = PAT (or authtoken) as `Authorization: Bearer`, per request**, validated via IAM introspect. No control-plane round-trip to mint a cert, no mTLS. Explicitly designed so a short-lived `ngrok curl foo.internal` is fast and never touches the control plane in the dial path.
- Explicit goals: kill TCP-over-TCP; make a one-shot dial as fast as a public `forward-internal` request.

### Consumers

1. **The agent** — `ngrok curl` / `ngrok bind` (daemon-mediated), and later Milestone 2: transparent reachability where the agent resolves `*.internal` and routes packets via a **TUN device**. [Agent spec](https://github.com/ngrok-private/product/blob/main/specs/private-dial/ngrok-agent-private-dial.md).
2. **The operator** — this migration.

## What's built vs in progress (as of 2026-09)

| Piece | Status |
|---|---|
| Gateway dial ingress | Up in dev (`connect-endpoint.dev-ngrok.com`, quic + h2). |
| ngrok-go private dialer | **Not released.** Benjamin's branch ([ngrok-go PR #245](https://github.com/ngrok/ngrok-go/pull/245)) — "builds, barely tested." Proper release is GAT-361. |
| `DialReq.metadata` in Traffic Policy CEL | **Not done** (GAT-475). `/dial` never reads `DialReq.metadata` today. Blocks pod-identity (`conn.k8s.pod.*`) parity. |
| Operator PAT issuance | ADMIN-675 (PAT template), ADMIN-685 (IAM client config). |
| Region pinning for private dial | **Unsolved** (GAT-675). Popsink deal needs it; INF-2153 is a stopgap on the old datapath. |
| Operator migration | Nothing merged. K8SOP-292 (datapath swap) blocked on the SDK; K8SOP-293 (retire kubernetes binding) is where the gap lives. |

## The agent's transparent model (relevant to option 5)

The agent does **not** poll a list of endpoints. Its daemon hijacks `*.internal` DNS and, per lookup, asks mux via `/get-host` whether the endpoint exists (A record vs NXDOMAIN). For actually carrying arbitrary TCP it uses a **TUN device** (Milestone 2): each name gets a synthetic IP, the TUN captures packets, and maps dest-IP → name. This matters for the "make `foo.internal` addressable in-cluster" idea — see [05](05-dns-tun-feasibility.md).
