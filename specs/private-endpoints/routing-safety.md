# Routing safety: private endpoints vs bindings

Scope: R2, R12, R25, R26 and D5 from the [tracker](README.md). Invariant (Correctness bar): **a connection reaches its intended endpoint or fails.**
Question asked: are these failure modes new in the private endpoints (PE) design, or does bindings already have them? We want to fix only what's new or worse, and fix it as simply as we can.

## TL;DR

| Scenario | Bindings | PE today | Verdict |
|---|---|---|---|
| (a) Partial write: Service published, port not recorded, failover, port reallocated (R25) | **No.** Port is in `spec`, written in the same Create as the object, before any Service exists | **Yes**, and it's close to deterministic after a crash (see below) | **New.** Fix |
| (b) Stale forwarder replica after delete + reuse (R2) | **Yes, worse.** Target is pinned when the listener opens, the port is reused on the next poll, and a shared finalizer can leak listeners for good | Yes, bounded by that replica's watch lag, and only if the cursor wraps or the operator restarts first | **Better** than bindings. Accept, document |
| (c) Duplicate port in the forwarder (R26) | **Partly.** A cache-lag race can create duplicates. The forwarder keeps the first listener silently, and the poller then errors on every cycle | Yes. Last entry wins silently, and the winner differs from the controller's tie-break | **Same class.** Small fix (fail closed), because (a) can produce duplicates |
| (d) Restart reuses a just-freed port (R12) | **Yes, always.** The bitmap is rebuilt every 10s poll, so the lowest free port is reused on every allocation | Only after restart/failover, and only for a freed port above every other used port | **Better.** Accept. (a)'s fix also removes the dangerous part |

Recommendation: two small changes, about 30 lines of code plus tests:
1. **O1.** During allocation, treat the target ports of existing managed Services as used. Read them uncached.
2. **O2.** The forwarder fails closed on duplicate forwarder ports.

Skip reordering the writes, tombstones/quarantine, and replica ack. We accept (b) and (d), bounded as described below.

## Evidence

**PE controller** (`internal/controller/privateendpoints/privateendpoint_controller.go`)
- The allocator's used set comes only from CR `status.forwarderPort`: `:68-83` builds `others` from status, and `:108-116` builds `used`. Existing Service targetPorts are never read.
- Write order: the Service goes out first (`CreateOrUpdate` at `:170`). Status is written per CR afterwards (`:195-198`, then `Status().Update` at `:249`).
- The cursor `lastPort` lives in memory. After a restart it is seeded from the highest port **in status** (`:214-222`). A port P that was just allocated is usually the highest one, and if it reached only the Service it isn't in status. So after a crash between `:170` and `:249`, the new leader's first allocation is **exactly P**. If that allocation is for another host B, then Service A → P → B. Before B's status lands, nothing listens on P, so A fails closed (`privateendpoints/forwarder_controller.go:56-58` skips CRs without ClusterIP/port).
- Without a crash, a failed status write (for example a conflict with the poller's drift healing) is safe today. The in-memory cursor has already moved past P, and the requeue rewrites Service A to a fresh port.
- Leader election: on when `--election-id` is set (`cmd/api-manager.go:479-480`). The controller and poller run on the leader only, with `MaxConcurrentReconciles: 1` (`:284`). Range is 20000-20999 (`cmd/api-manager.go:877-878`).
- Related variant: CR A is deleted, and the leader dies before `deleteHostServices` runs. Service A then still targets P while P is absent from status, so the same reuse misroutes clients of the deleted hostname.

**PE forwarder** (`internal/privateendpoints/forwarder/`)
- `table.go:28-33`: duplicate `ForwarderPort` → the last entry wins. The order comes from a cached List, so in practice it's arbitrary. The controller instead keeps the lowest CR name (`privateendpoint_controller.go:79`), so during the window the forwarder can disagree with the Service.
- `privateendpoints/forwarder_controller.go:102-117`: the target is resolved **at accept time**. There's no finalizer: the table is rebuilt from the full list. A replica is wrong only until its informer catches up.

**Bindings**
- Port storage: `spec.port`, chosen with `SetAny` and set in the same `Create` as the object (`boundendpoint_poller.go:389-418`). Services are created only later, by the BoundEndpoint controller (`boundendpoint_controller.go:202-227`), with the upstream targetPort = `spec.Port` (`:467`). So bindings can't produce (a).
- Allocation: every 10s poll (`cmd/api-manager.go:856`) builds a new bitmap from a **cached** List (`boundendpoint_poller.go:238-256`). `SetAny(serial)` on that new bitmap means the lowest free port, so a freed port is reused on the next create. A create that hasn't reached the cache yet leaves its port marked free, which can produce a duplicate `spec.port`. The next poll's `Set` then fails, and the whole poll returns an error (`:249-251`) every cycle until someone fixes it by hand.
- Forwarder: `NeedLeaderElection: false`, so every replica runs it (`forwarder_controller.go:104`). host/port are captured in the handler closure when the listener opens (`:174-185`, `:273`). `Listen` on a port that is already open is a no-op (`pkg/bindingsdriver/driver.go:26-28`): the first listener wins, and a reused port keeps the old target.
- Shared finalizer: the api-manager BoundEndpoint controller and every forwarder replica use the same `ngrok.com/finalizer` (`internal/util/k8s.go:13`, `base_controller.go:95,128`). Whoever removes it first lets the object go. The other replicas then get NotFound, ignore it (`base_controller.go:73-75`), and never `Close` the listener (`forwarder_controller.go:318-320`). After that, a reused port routes to the old target **until the pod restarts**. This comes from reading the code; we haven't reproduced it.
- In both designs, a misroute needs the old endpoint to still accept dials. If it's gone from ngrok, the dial fails, which fails closed.

## Options (smallest first)

| | Change | Size | Closes | Leaves |
|---|---|---|---|---|
| **O1** | Before allocating, List managed Services in the namespace through `APIReader`, and add every `TargetPort` to `used`. A port stays reserved as long as any Service routes to it, so the reservation survives withdrawal | ~15 lines | (a), the deleted-CR-with-surviving-Service variant, and the unsafe part of (d) | A brief reuse of A's old P on reallocation after an existing collision (A's Service moves away within the same reconcile) |
| **O2** | `Table.Replace` drops any port claimed by more than one entry and returns those ports. The ForwarderReconciler logs them. The listener stays open and `handler` closes the connection (`privateendpoints/forwarder_controller.go:106-108`) | ~10 lines | (c) misroute: an ambiguous port fails instead of guessing | Connections fail until the controller reallocates the loser, which takes one reconcile |
| O3 | Write `status.forwarderPort` (not ready, empty ClusterIP, so the forwarder ignores it) before the Service, then mark it Ready afterwards | ~15 lines + one more status write per allocation | (a) only, which O1 already covers. It keeps A's chosen port stable across a crash | Not the surviving-Service variant. Redundant with O1 |
| O4 | Quarantine (tombstones / delay before reuse) | ~40+ lines, persisted state | (b) and (d) beyond the cursor | Adds state for a case that's better than bindings |
| O5 | Replica ack / epoch before reuse | Large (forwarder → controller feedback) | (b) fully | Over-engineering for POC/preview |

## Recommendation

Do **O1 + O2**. With O1, a port is free only when no CR status claims it **and** no Service routes to it. The Service is the object that actually routes traffic, so it's the right thing to check, and it covers both orderings without adding a new write. O2 makes any duplicate that still happens (leader overlap during lease handoff, manual edits, state left over from today's build) fail instead of misrouting.

We still accept, and should write down:
- **(b)** A stale replica can misroute a reused port for as long as its watch lags. With the cursor, that only happens if the 1000-port range wraps or the operator restarts within that lag. Bindings is worse here: the target is pinned when the listener opens, the port is reused on the next poll, and listeners can leak.
- **(d)** After a restart, a freed port at the top of the range can be handed out at once. Once O1 is in, that only happens after its Service is gone, so the only risk left is the replica lag in (b).
- **Leader overlap**: an old leader's in-flight write can land after the new leader has listed. This is generic to controller-runtime and affects bindings equally. O2 makes the resulting duplicate fail closed.

## Tests

Use envtest in `privateendpoint_controller_test.go`, same shape as "port allocation ignores cache lag" (`:188`), plus unit tests in `forwarder/`.
- **FT-1a** (R25, crash between writes): wrap `r.Client` with `interceptor.Funcs{SubResourceUpdate}` so that A's status update fails. Reconcile A and assert Service A targets P while A's status is still 0. Then build a **new** Reconciler (`lastPort` = 0, which simulates failover), create B, and reconcile B. Assert B's port ≠ P. Reconcile A, and assert A's Service targetPort equals A's status port and that no two managed Services share a targetPort.
- **FT-1b** (deleted CR, surviving Service): reconcile A to P, delete CR A without reconciling, then use a new Reconciler for B. Assert B's port ≠ P while Service A exists, and that B can get P once A's Service is deleted.
- **FT-1c** (status conflict without a crash): same as FT-1a but reuse the same Reconciler. This guards the existing safe path.
- **FT-2a** (R26, `table_test.go`): two entries share a ForwarderPort. Assert `PortTarget` returns `!ok` and the duplicate port is reported. The non-duplicate entries still resolve.
- **FT-2b** (`privateendpoints/forwarder_controller_test.go`): duplicate in the list. A connection to that port is accepted and then closed without a dial (assert the fake `Dial` isn't called).
- **FT-2c** (R2 bound, documents the acceptance): ForwarderReconciler with a stale table (P→A). Replace it with (P→B) and assert the next accepted connection dials B without reopening the listener. Together with the existing "freed port is not reused while higher ports are free" (`privateendpoint_controller_test.go:266`), this pins down the bound we're accepting.
