# POC results — what actually changed, and which concern it belongs to

Companion to [08-poc-build-guide.md](08-poc-build-guide.md) (the plan) — this is what actually
happened when it was built, on branch `alex/private-dial-poc`. Docs 01-08 are planning/design
notes written *before* any code existed; this doc is the only one that describes the real diff.

**The one thing to keep straight:** this POC bundles two *independent* mechanisms that don't
depend on each other. If you're trying to understand "what makes private dial work" vs "what
makes `foo.internal` resolve in a pod," they're entirely different files and entirely different
kinds of change. Conflating them is the confusion this doc exists to prevent.

## Concern A — the private-dial egress leg (the actual K8SOP-292 datapath swap)

**Doesn't know or care about DNS.** This code takes a `host:port` string (wherever it came
from) and dials it over private dial instead of mTLS. It would work identically if the host
string came from a hardcoded value, a ConfigMap, or a different DNS mechanism entirely.

| File | What changed |
|---|---|
| `internal/controller/bindings/forwarder_controller.go` | Split the single `cnxnHandler` closure in `update()` into two named methods: `mtlsCnxnHandler` (today's shipping path, byte-for-byte the same logic as before, just extracted into its own function) and `privateDialCnxnHandler` (new). `ForwarderReconciler` gained `UsePrivateDial bool` and `PrivateDialer *privatedial.Dialer`; `update()` picks a handler based on the flag. |
| `cmd/bindings-forwarder-manager.go` | `--use-private-dial` / `USE_PRIVATE_DIAL` flag, `--private-dial-server` flag, `NGROK_PRIVATE_DIAL_PAT` env var. Constructs one `privatedial.Dialer` at startup and hands it to the reconciler. |
| `go.mod` | Added `golang.ngrok.com/ngrok/privatedial` (the real, standalone module the mono repo uses — see below). |

**Nothing here touches Services, CoreDNS, or the `BoundEndpoint` CRD/controller.** The
per-endpoint `Upstream` Service and the forwarder's per-port listener are pre-existing, untouched
code (`boundendpoint_controller.go`, `bindingsdriver/driver.go`) — this POC reuses them exactly
as the build guide describes.

### The library detour (read this before reusing this code)

[08](08-poc-build-guide.md) pointed at [ngrok-go PR #245](https://github.com/ngrok/ngrok-go/pull/245)
as "the dialer." That PR is an abandoned, unmerged experiment (title: "vibe: private dial
stub") — probing the real gateway against it live surfaced two protocol bugs in *that specific
branch* (wrong frame length-prefix width, and a leading server frame it doesn't know to skip).
**Don't use it.** The real, working client the mono repo actually depends on is a separate,
already-published module:

```
golang.ngrok.com/ngrok/privatedial v0.0.0-20260616091145-e2b70148b6de
```

`go get` it directly — no `go.mod replace`, no forked branch, no hand-rolled protocol code.
That module has its own bug, but a much smaller one: `Dialer`'s HTTP/2 transport
(`newH2Transport`) panics (nil-pointer dereference) when built with **Go 1.27**, because Go
1.27's native `net/http` HTTP/2 support changed how `golang.org/x/net/http2.Transport` needs to
be initialized, and this library's H2 path was never updated for it. This operator is pinned to
Go 1.27 (`flake.nix`), so:

- `cmd/bindings-forwarder-manager.go` forces `privatedial.ProtocolQUIC` (the HTTP/3 transport,
  which has no such bug) and never selects `ProtocolH2` or `ProtocolAuto` — either of those
  would crash the forwarder pod on the first dial. See the `--private-dial-server` flag's help
  text for the exact repro.
- This means the forwarder's outbound path needs **UDP/443** egress to the connect ingress
  (`quic.connect-endpoint.ngrok.com`), not TCP/443. Confirm that's actually open before assuming
  a "connect-ingress unreachable" failure is DNS or auth — it might just be UDP being blocked
  where TCP isn't.
- Fixing the H2 path upstream (so `ProtocolAuto`'s happy-eyeballs race is safe again) is real
  work someone should pick up before this goes anywhere near production; it's out of scope here.

## Concern B — making `foo.internal` resolve inside the cluster (pure DNS, zero Go code)

**One CoreDNS ConfigMap edit, nothing else.** This is the *entire* mechanism, and it has nothing
to do with private dial, the forwarder, or any code in this repo:

```
kubectl -n kube-system edit configmap coredns
# inside the `.:53` server block, add:
rewrite name regex (.*)\.internal {1}.ngrok-operator.svc.cluster.local
kubectl -n kube-system rollout restart deploy/coredns
```

That's it. It works because the (pre-existing, unmodified) `BoundEndpoint` controller already
creates an `Upstream` Service in the operator's namespace named after the `BoundEndpoint`
(`foo` → `foo.ngrok-operator.svc.cluster.local`, a real in-cluster DNS name every cluster gets
for free). The rewrite just aliases `foo.internal` onto that existing name. No new Service type,
no new controller, no code in this repo — see [08](08-poc-build-guide.md#change-2--make-foointernal-resolve-in-cluster)
and [07](07-transparent-internal-projection.md) for why this is enough and what it doesn't scale
to.

**This is also why Concern A doesn't need Concern B to be testable.** You can point the
forwarder's `--use-private-dial` path at any reachable `host:port` without touching CoreDNS at
all — DNS only matters once you want to reach it *by the `foo.internal` name from inside a pod*,
which is the specific thing this POC is proving is possible.

## Concern C — a real gotcha the build guide didn't anticipate (not a code change)

[08](08-poc-build-guide.md) says to hand-wire a `BoundEndpoint` CR and "skip the poller." What
actually happens: the account poller (`BoundEndpointPoller`, `boundendpoint_poller.go`, 10s
interval, runs inside `ngrok-operator-manager` alongside the `BoundEndpoint` reconciler that
mints the Services) treats any `BoundEndpoint` CR that isn't in its own idea of "endpoints this
operator's ngrok account actually has bound to it" as orphaned garbage and **deletes it** every
cycle. A hand-wired `.internal` `BoundEndpoint` is never in that set (it's not a real
`kubernetes`-bound API endpoint), so it gets reaped within ~10-20 seconds of creation —
independent of, and in addition to, the `BoundEndpoint` reconciler's own connectivity-check
condition that [08](08-poc-build-guide.md#gotchas) does call out.

There's no flag to disable the poller. The workaround used for this POC's testing (not part of
the diff, not a real fix): apply the `BoundEndpoint` CRs, then immediately
`kubectl scale deployment ngrok-operator-manager --replicas=0` to freeze both the reconciler and
the poller before the next reap cycle, run the test, then scale back to `1`. A real fix would
need the poller to leave alone `BoundEndpoint`s it didn't create itself (e.g. an annotation
opt-out) — out of scope for this POC.

## What was actually tested (live, against real ngrok endpoints)

- Two live `.internal` endpoints: `foo.internal` (HTTP, `ngrok http --url http://foo.internal`,
  backend = `ealen/echo-server`) and `bar.internal` (TCP, `ngrok tcp --url tcp://bar.internal:6379`,
  backend = real `redis:7-alpine`), served by ngrok agents running outside the cluster.
- `curl http://foo.internal` from a pod in the kind cluster → the echo backend's real response.
- Raw redis `PING` over `bar.internal:6379` from a pod → `+PONG` — **the raw-TCP proof.**
- Both endpoints reachable with no crosstalk (per-port L4 demux, unchanged pre-existing code).
- Forwarder logs show `private-dialed endpoint`, never the mTLS/cert path.
- Killing the redis backend made the response go empty; restarting it brought `+PONG` back —
  confirms it's really traversing the endpoint, not cached.
- `getent hosts foo.internal` from a pod resolves to the `Upstream` Service's ClusterIP.
- Existing `internal/controller/bindings` test suite passes unmodified — the mTLS path (behind
  `UsePrivateDial=false`, the default) is untouched.

## How to reproduce

1. Get a PAT (`ngrok_pat_*`) from `dashboard.ngrok.com/api/personal-access-tokens` — there's no
   API/CLI way to mint one. Private dial is PAT-only; the account's authtoken/API key won't
   authenticate against the gateway.
2. Stand up the two test endpoints (Concern-B-independent, see above).
3. Apply Concern B (CoreDNS rewrite).
4. `make deploy_with_bindings`, then patch the forwarder Deployment to add
   `--use-private-dial=true`, `--private-dial-server=quic.connect-endpoint.ngrok.com:443`, and
   `NGROK_PRIVATE_DIAL_PAT` from a Secret (not part of the Helm chart yet — this is a POC flag).
5. Hand-wire the two `BoundEndpoint` CRs (Concern A + B meet here: the CR's `endpointURL` is what
   the forwarder private-dials, and its `name` is what the DNS rewrite resolves).
6. Immediately scale `ngrok-operator-manager` to 0 replicas (Concern C workaround), then test.
