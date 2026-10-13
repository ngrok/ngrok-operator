# Private Endpoints — SDK upstream plan

Status: draft 2026-10-12 · Owner: Alex Bezek · Tracker: [README.md](README.md) (milestone 7)

What generic code in the POC should move into ngrok SDKs, what stays in the operator, and the
API + behavior contract we propose. Builds on [strategy.md §4.3, §7.4](strategy.md) and
[design.md §4](design.md).

References to the SDK are against the pinned module
`golang.ngrok.com/ngrok/privatedial@v0.0.0-20260616091145-e2b70148b6de` (`go.mod:19`),
written `pd/client.go:N`. Everything here was read from code, not run, unless it says so.

## 1. Summary

| Component | Stays / moves | Target | Why |
|---|---|---|---|
| Bidirectional copy with half-close + drain (`join`, `closeWrite` — `internal/privateendpoints/forwarder/privatedial_forward.go:68-95`) | **Moves** | ngrok-go `privatedial` (`Join`) | Reimplemented, unexported, with different semantics in agent v4, ngrok-go v2 (`forwarder.go:152-171`, full close, no half-close), bindings (`internal/controller/bindings/forwarder_controller.go:331`, full close), and here. The contract (half-close, drain, idle) belongs next to the conn whose `CloseWrite` semantics it relies on |
| Dial-then-join (`ForwardConn`, `privatedial_forward.go:57-66`) | **Moves** | `privatedial` (`Forwarder`) | Same reason; also where dial-error → client signal (RST vs FIN) should be decided |
| Accept loop (`pkg/bindingsdriver/driver.go:98-112`) | **Moves for PE**, stays for bindings | `privatedial` (`Forwarder.Serve`) | Generic; current loop has no backoff on accept errors (`driver.go:107`, R9) and binds `0.0.0.0` only (`driver.go:32`). Bindings keeps its own copy until bindings is removed |
| Per-accept target lookup (`Syncer.handler`, `syncer.go:100-114`) | Stays (as a callback) | operator | Routing table is k8s state. SDK must accept a resolver func, not a fixed target string, or we lose "port reassigned → new target without reopening" |
| `Syncer`, `Table` (`syncer.go`, `table.go`) | Stays | operator | Driven by `PrivateEndpoint` CR status |
| `naming.go` `ServiceName`, `CRName`, `HostKey`, labels (`naming.go:18-23, 67-105`) | Stays | operator | Service/CR naming, k8s label rules (`validation.IsDNS1035Label`) |
| `naming.go` `ParseURL`, `IsPrivateHostname` (`naming.go:31-65`) | Stays for now | maybe `privatedial` later | Generic logic, but `ParseURL` returns a CRD type, and private TLDs are hard-coded (`naming.go:62-65`). Move only if a second consumer appears or the SDK exports the TLD list |
| Port allocator (`controller.go:205-234`) | Stays | operator | Allocates Service targetPorts; only meaningful with k8s Services + forwarder replicas |
| Poller list + filter (`poller.go:58-66, 138-158`) | Stays; **ask** for server-side support | ngrok-api-go / API; or `privatedial` | Lists *all* endpoints and filters client-side because v9 `Paging` has no filter (design.md:216-217). Real fix is an API filter or a dial-side "what can I reach" list/watch, not moving our filter |
| Dialer config: QUIC forced, 5s budget, no ClientVersion/Logger (`cmd/private-endpoint-forwarder.go:98-102`) | Stays; SDK fixes unblock changes | operator | Wiring. Forcing QUIC is a workaround for the HTTP/2 panic on Go 1.27 (`cmd/private-endpoint-forwarder.go:62-63`) |
| Per-connection identity | **Ask** | `privatedial` + backend | `Config.Metadata` is per session (`pd/client.go:208-211, 459-461`); `DialReq` carries only host and port (`pd/client.go:1260-1263`). GAT-475 |

## 2. What `privatedial` exposes today, and gaps

Exposed (pin `e2b70148`): `New(Config)`, `Dialer.{Connect, DialContext, Close, Done, Err,
Protocol, ServerID, RemoteAddr}`, `Conn` (`net.Conn` + `EndpointID()`), `Error` (`Code()`),
`ErrSessionClosed`, `Protocol{Auto,QUIC,H2}`. Not in this pin: `GetHost` (notes.md:51 says newer
branch commits add it).

Gaps that matter to a forwarder:

| # | Gap | Where | Operator impact / workaround |
|---|---|---|---|
| G1 | No join/forward helper | — | Local `privatedial_forward.go` |
| G2 | Deadlines are silent no-ops: `SetDeadline`/`SetReadDeadline`/`SetWriteDeadline` return nil and do nothing | `pd/client.go:1752-1754` | Can't build idle timeout on deadlines; any caller wrapping the conn in `tls.Conn`/`http.Server` gets no timeouts without knowing. We have no idle timeout (R8) |
| G3 | Dial-stage HTTP status errors are untyped (`fmt.Errorf("private-dial /dial status %d…")`) | `pd/client.go:1316-1326` | Can't tell "endpoint gone" from "server error" without string matching |
| G4 | Only `ERR_NGROK_709` (unauthorized) and `ERR_NGROK_742` (draining) map to a net sentinel; endpoint-not-found has no sentinel | `pd/client.go:1437-1458` | We can't choose RST-to-client vs retry by category. Today every dial failure → close client (`privatedial_forward.go:60-62`) |
| G5 | Dial budget fixed at 5s, field unexported | `pd/client.go:795, 1000-1005` | Caller ctx can only shorten it. Fine for now |
| G6 | Established stream ignores ctx: request ctx is `context.Background()` | `pd/client.go:1251` | Shutdown/ctx cancel doesn't end in-flight streams; only `Close` does. `Join` must own this |
| G7 | HTTP/2: when `ReserveNewRequest` fails (stream limit) the dial returns `staleConnError` and the loop retries the same session immediately until the 5s budget expires, then reports `ECONNREFUSED` | `pd/client.go:1271-1275, 932-956` | Possible busy-spin + false "refused" under load on H2. Unverified; we force QUIC today |
| G8 | HTTP/2 transport failed on Go 1.27 in our testing | notes.md:56 | QUIC forced → UDP/443 egress required |
| G9 | `stickyProtocol` is a process-global | `pd/client.go:91` | Two Dialers in one process (multi-account, tests) share the first protocol picked |
| G10 | No metrics hooks; only a debug `slog.Logger` | `pd/client.go:213-217` | No dial latency / error / bytes metrics in the forwarder |
| G11 | No per-dial metadata | `pd/client.go:1260-1263` | No pod identity (GAT-475) |
| G12 | `LocalAddr()` is an empty `dialAddr` | `pd/client.go:1750` | Cosmetic; logs only |

Operator-side things the SDK already supports but we don't use (fix locally, no SDK change):
`Connect` for a startup auth check (`pd/client.go:730`), `Done`/`Err` for readiness on fatal
auth (`pd/client.go:834-850`), `ClientVersion` and `Logger` (`pd/client.go:205-217`).

## 3. Proposed ngrok-go API

Package `golang.ngrok.com/ngrok/privatedial`. `Join` and `Forwarder` take any
`ContextDialer`, so they work with `*Dialer`, `net.Dialer` and test fakes, and ngrok-go v2's
`endpointForwarder` could reuse `Join` later (that would change its close semantics, see Asks).

```go
// ContextDialer is satisfied by *Dialer and *net.Dialer.
type ContextDialer interface {
	DialContext(ctx context.Context, network, addr string) (net.Conn, error)
}

type JoinOptions struct {
	// DrainTimeout bounds how long the second direction may keep copying
	// after the first reaches EOF. 0 means DefaultDrainTimeout (5s, agent v4).
	// Negative means no limit.
	DrainTimeout time.Duration
	// IdleTimeout force-closes both conns when neither direction has moved
	// a byte for this long. 0 disables it.
	IdleTimeout time.Duration
}

type CloseReason int

const (
	ClosedEOF      CloseReason = iota // both directions reached EOF
	ClosedDrain                       // drain timeout after the first EOF
	ClosedIdle                        // idle timeout
	ClosedContext                     // ctx done
	ClosedError                       // a read or write failed
)

type JoinResult struct {
	AToB, BToA int64       // bytes copied
	Reason     CloseReason
	// Err is the first non-EOF error from either direction, after
	// filtering errors caused by Join's own force-close. Mid-stream
	// server errors arrive here as a *net.OpError wrapping a privatedial.Error.
	Err error
}

// Join copies a<->b until both directions finish, then closes both.
func Join(ctx context.Context, a, b net.Conn, opts JoinOptions) JoinResult

// ConnInfo is passed to Forwarder.OnConn once per accepted conn, after it ends.
type ConnInfo struct {
	Client     net.Addr
	Target     string        // "" if Target returned !ok
	EndpointID string        // from privatedial.Conn, if dialed
	DialTime   time.Duration
	DialErr    error
	Join       JoinResult
}

type Forwarder struct {
	Dialer ContextDialer
	// Target picks host:port for an accepted conn at accept time.
	// ok=false closes the conn (RST) without dialing.
	Target func(c net.Conn) (addr string, ok bool)
	Join   JoinOptions
	// OnConn, if set, is called after each conn ends. Metrics/logging hook.
	OnConn func(ConnInfo)
}

// Serve accepts on l and forwards each conn. It returns when l is closed
// (nil) or ctx is done (ctx.Err()), and only after every in-flight conn has
// ended. Closing l = graceful (in-flight conns finish); cancelling ctx = hard
// stop (in-flight conns are force-closed).
func (f *Forwarder) Serve(ctx context.Context, l net.Listener) error

// Forward is Serve with a fixed target.
func (d *Dialer) Forward(ctx context.Context, l net.Listener, target string, opts JoinOptions) error
```

This replaces the strategy.md sketch `Join(a, b, drain) error` / `Forward(ctx, l, target)`:
a fixed `target` can't express our accept-time lookup (`syncer.go:100-114`), and `drain`
alone can't carry idle timeout or ctx cancellation.

### Behavior contract

- **Half-close.** When a direction hits EOF, `Join` calls `CloseWrite()` on its destination if
  it has one (`*net.TCPConn`, `*tls.Conn`, `privatedial.Conn`), else closes it fully.
  `privatedial.Conn.CloseWrite` ends the request body (`pd/client.go:1735-1737`); the SDK must
  document what the endpoint's upstream sees (TCP FIN? nothing?). See Ask A2.
- **Drain.** Only when the **upstream** finishes first does the client get `DrainTimeout` to
  finish sending, then both are closed. A client half-close starts no timer: the upstream may
  take as long as it needs to reply (starting the drain on *either* EOF was R21 and cut slow
  replies). Default 5s. Matches the operator as of 2026-10-13.
- **Idle timeout.** Implemented by `Join` with an activity timer reset on every copied chunk,
  not with deadlines (G2). A trickle of bytes keeps the conn alive. Default off.
- **Deadlines.** `Join` doesn't set or need deadlines. Separately, `privatedial.Conn` should
  either implement them or return an error (Ask A3); silent no-op is the worst option.
- **Cancellation.** `ctx` done → both conns closed immediately, `Reason=ClosedContext`. The dial
  in `Forwarder` uses the same ctx; the dial budget (5s) still applies on top.
- **Close.** `Join` always closes both conns before returning, exactly once. Safe if the caller
  closes them concurrently.
- **Abort on error.** A copy error other than EOF in either direction aborts **both** sides
  (TCP: `SetLinger(0)` then Close → RST) so the peer never sees an orderly end of a truncated
  stream (R27). `privatedial.Conn` needs a real abort for this (Ask A11).
- **Errors.** `io.EOF`, `net.ErrClosed` and errors caused by `Join`'s own force-close are not
  errors. A `privatedial.Error` from `Read` (mid-stream trailer, `pd/client.go:1718-1729`) is
  surfaced in `JoinResult.Err`. `Join` never logs.
- **Dial failure in `Forwarder`.** Close the client with RST where possible
  (`(*net.TCPConn).SetLinger(0)`) so clients see "connection refused/reset" quickly, not a
  clean EOF that looks like an empty response. Report through `OnConn`.
- **Accept errors.** Temporary errors back off 5ms → 1s (net/http style). `net.ErrClosed` →
  return nil. Other errors → return them.
- **Concurrency.** One goroutine pair per conn; no global limit in v1. A `MaxConns` option can
  come later if needed.
- **HTTP/2 / ALPN.** `Join` is bytes-only, so app-level h2, h2c, gRPC and end-to-end TLS ALPN
  (`tls://`) pass through untouched. For `https://` endpoints ngrok terminates TLS, so ALPN is
  negotiated with ngrok's edge, not the origin (Ask A6). Separately, the *transport* to
  `connect-endpoint` is h3 or h2 (`pd/client.go:42-54`); on h2 all dials share one TCP conn
  (head-of-line blocking, stream limit G7). Neither affects `Join`'s contract.

### Tests to upstream with it

Port the three existing tests (`privatedial_forward_test.go`: joins both ways, dial failure
closes client, drains after upstream EOF), then add:

- [ ] half-close client→upstream: client `CloseWrite`s after the request, still gets the full
  response (TCP pair and `privatedial` integration server, h3 + h2)
- [ ] half-close upstream→client: upstream sends then `CloseWrite`s, client sees EOF, client
  can still send within drain
- [ ] drain: after **upstream** EOF, a client stream longer than `DrainTimeout` is cut; negative drain = unlimited
- [ ] client half-close starts no drain: reply delayed past `DrainTimeout` still arrives in full
- [ ] upstream reset mid-response → client sees reset, not EOF (and the reverse direction)
  (operator has the TCP-loopback versions: `TestForwardConnOverTCP`)
- [ ] idle: closed after `IdleTimeout` of silence; kept open by a trickle below it
- [ ] ctx cancel force-closes both, `Reason=ClosedContext`
- [ ] conn without `CloseWrite` falls back to `Close`
- [ ] error classification: clean EOF → `Err==nil`; mid-stream `privatedial.Error` →
  `errors.As` works on `JoinResult.Err` (reuse `TestDialMidStreamTrailerError` harness)
- [ ] `Serve`: `Target` ok=false → RST, no dial; dial error → RST + `OnConn` with `DialErr`;
  `l.Close()` returns nil after in-flight conns finish; temporary accept error backs off
  (fake listener)
- [ ] `-race`, and `goleak` on every test
- [ ] H2 transport on the current Go release (G8 regression), and stream-limit behavior (G7)

The SDK already has an in-process server harness (`pd/client_integration_test.go:206`,
`TestDialEcho`) to run these over both transports.

## 4. Sequencing

**Now, before the SDK lands (operator, no SDK change):**

1. Reshape `privatedial_forward.go` to the exact signatures above (`Join`, `JoinOptions`,
   `JoinResult`, `Forwarder`) in its own package, e.g. `internal/privateendpoints/pdforward`,
   importing only stdlib. The swap is then an import change plus file deletion.
2. Move the PE forwarder off `bindingsdriver` onto `net.Listen` + `Forwarder.Serve`, keeping
   accept-time lookup via `Target: func(c) { return table.PortTarget(localPort(c)) }`. Fixes
   R9's accept spin for PE; listen on `:port` (dual-stack where the node allows it) or keep
   `0.0.0.0` until IPv6 is in scope.
3. Use what the SDK already has: `Connect` at startup (fail fast on a bad token), `Done`/`Err` in
   readiness, `ClientVersion` = operator version, `Logger` via logr→slog bridge.
4. Add forwarder metrics from `OnConn` (dials, dial errors by `Error.Code()`, dial latency,
   bytes, close reason). These names then become the hook's reference consumer.

**SDK lands:**

5. PR to ngrok-go `privatedial` with the API + tests above. Agent v4 is the second consumer;
   ask them to review the contract.
6. Bump the operator to that commit, delete the local package.

**Moving off the pin:** we pin `e2b70148`, agent v4 pins `cc6f75f` on the `privatedial` branch
(design.md:194-195). Target: `privatedial` merged to ngrok-go `main` and tagged
(`privatedial/v0.x`), operator and agent on the same tag.

**Blocks GA (operator private endpoints):**

- A tagged `privatedial` release (no pseudo-version off a feature branch).
- H2 fixed on the Go version in `flake.nix` (`go_1_27`), so `ProtocolAuto` can replace forced
  QUIC and UDP/443 stops being a hard requirement (G8).
- A decided half-close / drain / idle contract (R8) — whether it lives in the SDK or stays
  local, it has to be written down and tested.
- Typed endpoint-not-found error (G3/G4), so the forwarder can RST instead of masking it.

Not GA-blocking: identity (G11), metrics hooks (G10, `OnConn` covers it), `GetHost`, dial-side
endpoint listing.

## 5. Asks for the SDK / backend teams

Owners: `privatedial` lives on an ngrok-go feature branch shared with agent v4; who reviews it
is not recorded in our docs (Alex to fill in).

- **A1 (ngrok-go)** Will you take `Join`/`Forwarder` in `privatedial`, or prefer a separate
  package also used by ngrok-go v2's `endpointForwarder`? Moving v2 onto `Join` would change it
  from full close to half-close + drain.
- **A2 (backend)** What does the origin see when a dial client half-closes (request body ends)?
  Does the server propagate FIN to the agent's upstream, and does the response stay open?
  Same question for HTTP/2 and HTTP/3 transports.
- **A3 (ngrok-go)** Implement deadlines on `privatedial.Conn`, or make them return an error?
  Silent no-ops (`pd/client.go:1752-1754`) break callers that rely on them.
- **A4 (ngrok-go / backend)** What error code does "no endpoint for host:port" return, and can it
  map to a sentinel (`ECONNREFUSED` or a new `ErrEndpointNotFound`)? Same for non-200 `/dial`
  status (G3).
- **A5 (ngrok-go)** Is the Go 1.27 HTTP/2 failure known/fixed on a newer branch commit? Is G7
  (H2 stream-limit spin) real? What are the per-session stream limits for h2 and h3, and does
  the Dialer ever open a second session when one is full?
- **A6 (backend)** For `https://` private endpoints, which ALPN protocols does ngrok offer to
  the client (h2? http/1.1 only?).
- **A7 (ngrok-go)** Can `stickyProtocol` be per-Dialer (G9)? Can the dial budget be a `Config`
  field (G5)?
- **A8 (backend, GAT-475)** Plan for per-dial metadata (`DialReq.Metadata`) visible to Traffic
  Policy? The operator would send pod name/namespace/IP per connection.
- **A9 (API / ngrok-go)** Server-side filter for `Endpoints.List` (by binding or hostname
  suffix), or a dial-session `ListHosts`/`Watch` returning exactly what this token can reach?
  Either would replace the poller's full-account list + client filter. When does `GetHost`
  land?
- **A10 (backend)** Binding rename `internal` → `private` (design.md:196): date, and will the
  SDK export the private TLD list so `IsPrivateHostname` isn't hard-coded?
- **A11 (ngrok-go)** Real abort on `privatedial.Conn` (`Abort()`, or `reqBody.CloseWithError`
  instead of `Close` on the abort path) so the endpoint sees a stream reset, never a clean end
  of request, when the client side fails.
- **A12 (ngrok-go)** Initial `Connect` auth failure (401/403) is not terminal and the
  `authFatalError` type is unexported, so the forwarder can't tell a bad token from an outage
  at startup. Export it (or a sentinel), or make initial auth failures terminal.

## 6. TODOs

- [ ] SDK-1 Reshape `privatedial_forward.go` into a stdlib-only package with the §3 signatures (operator, now)
- [ ] SDK-2 Move PE forwarder from `bindingsdriver` to `net.Listen` + `Forwarder.Serve`, keep accept-time target lookup (operator, now)
- [ ] SDK-3 Use `Connect`, `Done`/`Err`, `ClientVersion`, `Logger` in the forwarder (operator, now)
- [ ] SDK-4 Forwarder metrics via `OnConn` (operator, now)
- [ ] SDK-5 Send A1–A12 to the ngrok-go / private dial owners; record answers here
- [ ] SDK-6 Write upstream tests (§3 list), run them locally against the pinned harness first
- [ ] SDK-7 Open ngrok-go PR: `Join`, `Forwarder`, `Dialer.Forward` + tests
- [ ] SDK-8 Repro the Go 1.27 HTTP/2 failure with a minimal program; attach to A5
- [ ] SDK-9 Check G7 (H2 stream-limit spin) with a test against the in-process server
- [ ] SDK-10 Move operator to a tagged `privatedial` release; delete the local helper
- [ ] SDK-11 Switch to `ProtocolAuto` once H2 is fixed; drop the UDP/443 requirement from docs and Helm help text
- [ ] SDK-12 Revisit poller once A9 is answered (API filter or dial-side list/watch)
- [ ] SDK-13 Per-connection identity once A8 lands (GAT-475)
