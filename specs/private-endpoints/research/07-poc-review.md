# 07 — Independent POC review (adversarial)

Date: 2026-10-12 · Reviewed: committed `HEAD` (`1f4bd38d`) of `alex/private-endpoints-poc`, read
with `git show HEAD:<path>`. In-flight D8 edits (poller filter, api-manager flag, demo) were
not reviewed.

Legend: **CONFIRMED** = shown by reading the code or by a throwaway test (scratch copy of HEAD,
nothing written to the repo). **PLAUSIBLE** = follows from the code, but not run end to end.

## Test results (HEAD)

| Suite | Result |
|---|---|
| `CGO_ENABLED=1 go test -race ./internal/controller/privateendpoints/... ./internal/privateendpoints/... ./cmd/... ./pkg/bindingsdriver/...` | All pass. No races reported. Note that `-race` needs `CGO_ENABLED=1` in the devShell, and plain `go test -race` errors out |
| `helm unittest -f 'tests/**/*_test.yaml'` (ngrok-operator chart) | 16 suites, 150 tests, 55 snapshots, all pass |
| Scratch: feature on, CRD missing (envtest without CRDs) | `ctrl.NewManager` fails: `failed to determine if *v1.PrivateEndpoint is namespaced ... no matches for ngrok.com/v1` (see PR-1) |
| Scratch: client half-close, upstream replies after 300ms (drain 5s) | Full response delivered |
| Scratch: client half-close, upstream replies after 700ms (drain 200ms) | Client gets **0 bytes**. The response is cut (see PR-2) |

RBAC was checked call by call and matches. Every client call (APIReader list PE; Status().Update;
CreateOrUpdate Service = get/create/update; Service list/delete; watches on PE and Service;
poller list/create/update/delete PE; forwarder list/watch PE) has a matching verb in both the
kubebuilder markers and the Helm Roles. No R19-class gap remains, but no test guards it either
(see test gaps).

## Findings

| ID | Sev | Where | Issue | Failure scenario | Suggested fix | Dup? | Status |
|---|---|---|---|---|---|---|---|
| PR-1 | Med | `cmd/api-manager.go:491` (`managerCacheOptions`), `:405` | Turning the feature on without the CRD makes the **whole api-manager** fail to start. The `ByObject` pin on `PrivateEndpoint` needs a REST mapping at `NewManager`. The Gateway path does a discovery check and disables itself. Private endpoints don't | User has `installCRDs=false` and an ngrok-crds chart that predates the CRD (the tracker says the CRD chart bump hasn't shipped). They set `privateEndpoints.enabled=true`. api-manager crashloops, which takes down Ingress, Gateway and bindings too | Check discovery for `ngrok.com/v1 privateendpoints` as Gateway does. Either disable the feature with a loud log, or fail with a clear message. Add a test | Contradicts R-7 "no-op without the CRD" | CONFIRMED (scratch envtest) |
| PR-2 | Med | `internal/privateendpoints/forwarder/privatedial_forward.go:69-85` | The drain timer starts when **either** direction ends, including a client half-close. Any response that takes longer than 5s after the client's `shutdown(SHUT_WR)` is cut off | `nc -N`, `printf ... \| redis-cli`, HTTP/1.0-style or RPC clients that half-close after the request, then the server takes more than 5s (slow query, large download). The client gets a truncated or empty response | Start the drain only after upstream→client EOF, or apply it only when the upstream finishes first. Better: an idle timeout instead of a fixed drain (SDK contract) | R8 (now confirmed; the strategy.md wording overclaims, see docs list) | CONFIRMED (scratch test) |
| PR-3 | Med | `helm/ngrok-operator/Chart.yaml` (ngrok-crds subchart, `condition: installCRDs`), `internal/drain/drain.go:99-106`, `controller.go:154` | R16 is more specific than the tracker says. With the default `installCRDs=true`, the CRD is a **templated resource in the same release**, with no `resource-policy: keep`. `helm uninstall` deletes it, and the API server then deletes every PrivateEndpoint. The per-host Services have no ownerRef, so they **stay**, selecting forwarder pods that no longer exist. The pre-delete drain hook doesn't know about PrivateEndpoints or their Services. Turning the feature off (`enabled=false`) leaves both CRs and Services, and removes the Role that could clean them up | After uninstall or disable, `foo.ngrok.direct` still resolves (if the admin's CoreDNS rewrite stays) to a ClusterIP with no endpoints. That gives refused connections instead of NXDOMAIN, plus leftover objects in the namespace | Put an ownerRef on each Service (owner: a CR on that host, or one anchor object), or add a label-based cleanup to the drain hook. Document the disable path | R16 (extends it; "CRD in separate chart" is wrong for the default install) | CONFIRMED (code); runtime PLAUSIBLE |
| PR-4 | Med | `pkg/bindingsdriver/driver.go:33` (binds `0.0.0.0`), no NetworkPolicy in chart | The forwarder listens on the pod IP on 20000–20999 with the account's token behind it. On flat-network CNIs (EKS VPC CNI, which is next in D7), pod IPs can be routed from the VPC and peered networks, not just from the cluster | Any host in the VPC can connect to `<forwarder-pod-ip>:2000x` and reach that private endpoint. The exposure is wider than "every pod in the cluster" | Ship an optional NetworkPolicy (ingress from cluster pod CIDRs or namespaces only), and say so in the EKS runbook | New (widens the accepted "account-wide exposure") | PLAUSIBLE |
| PR-5 | Low–Med | `cmd/private-endpoint-forwarder.go:131`, `helm/.../private-endpoints/deployment.yaml` | There is no graceful shutdown. On SIGTERM, `mgr.Start` returns and the process exits, cutting every in-flight connection. There is no preStop delay for endpoint removal to propagate. Default `replicaCount: 1`, no PDB, and none of the bindings-forwarder knobs (`nodeSelector`, `tolerations`, `affinity`, `priorityClassName`, `terminationGracePeriodSeconds`) | Rolling the image, draining a node, or evicting the pod drops every Redis or DB connection, and new connects during the propagation window are refused. With 1 replica, a node drain is a full outage | Add a preStop sleep, then stop accepting and wait up to the grace period for joins to finish. Add a PDB and the scheduling knobs, mirroring `bindings-forwarder/deployment.yaml` | New | CONFIRMED (code) |
| PR-6 | Low | `cmd/private-endpoint-forwarder.go:120-125`, `syncer.go:84` | The R5 fix makes readiness fail on **any** bind error. Readiness is per pod, so one unbindable port takes **every** endpoint on that replica out of every Service | One CR with a bad `status.forwarderPort` (R13: no range check, e.g. 8080/8081, which the same process already holds) drops all private endpoint traffic on a 1-replica install | Range-check ports in the Syncer (R13) and skip out-of-range ones. Use readiness only for "synced", and surface per-port bind errors as metrics, logs or a CR condition | Amplifies R13, R5 | CONFIRMED (code) |
| PR-7 | Low | `controller.go:74` (`cr.Labels[pe.HostLabel] == hostKey`), `:87` (`mine[0].Spec.Hostname`), `syncer.go:57-64` | The controller and the forwarder act on **any** PrivateEndpoint in the namespace, not only `managed-by` ones. The controller trusts the host label and never checks it against `HostKey(spec.Hostname)`. The forwarder dials each CR's own `spec.hostname` | Someone who can create PrivateEndpoints and their status in the operator namespace makes a CR with victim's host label, `spec.hostname: other.ngrok.direct`, port 443, and a lower name. It wins port 443 on the victim's Service, and the forwarder dials `other.ngrok.direct:443`: **wrong destination**. A hand-made CR also gets a Service and listener, which contradicts "read-only mirror" | Filter to `managed-by` in the controller and the Syncer (or make it an explicit feature). Skip CRs where label ≠ `HostKey(spec.Hostname)` | New | CONFIRMED (code) |
| PR-8 | Low | `syncer.go:48-64` | `Sync` lists and calls `Table.Replace` **before** taking `s.mu`. The startup `RunnableFunc` Sync and controller reconciles run at the same time, so an older list can replace a newer table | A CR changes between the two lists, and the stale Replace lands last. The table, and so routing, is stale until the next PrivateEndpoint event | Take `s.mu` for the whole of `Sync` (list → Replace → listeners) | New | PLAUSIBLE (tiny window) |
| PR-9 | Low | `controller.go:282` (`GenerationChangedPredicate`) | Label-only changes don't change generation, so a host-label edit (or the poller restoring it, R3) enqueues neither the old host nor the new one | The label is edited and another CR on the same host triggers a reconcile, so the port is dropped. The poller restores the label, but no event follows, so the port stays missing until something else touches that host | Add `predicate.LabelChangedPredicate` (OR with generation) to the PrivateEndpoint watch | Related to R11 | CONFIRMED (code) |
| PR-10 | Low | `poller.go:55` | Every 10s the poller pages through **all** endpoints in the account, public included. There's no server-side filter (v9 `Paging` has none) and it shares the token's rate limit with the rest of the operator | Large account (thousands of endpoints): dozens of API calls per tick, which can hit rate limits and slow down Ingress/Gateway reconciles | Back off on 429. Add a configurable interval. Ask for a server-side filter by binding | New (design deviation is documented, but its cost isn't) | PLAUSIBLE |
| PR-11 | Low | `poller.go:53-82` | R-6 only covers list **errors**. A successful empty, or wrongly scoped, list deletes every CR, and with them every Service | Token rotated to another account, or an API key with narrower scope. Every in-cluster private endpoint vanishes on the next tick | Optional guard: refuse to delete more than N% in one tick without a second confirming poll | Edge of R-6 | PLAUSIBLE |
| PR-12 | Low (latent) | `syncer.go:102-114`, `:144` | Listeners opened from `Reconcile` capture the reconcile ctx and use it for every later dial. That's only safe because `ReconciliationTimeout` defaults to 0 in controller-runtime v0.24.1. If anyone sets it (manager-wide `config.Controller` or per controller), the ctx is cancelled when the reconcile returns, and every dial on those ports fails | Someone sets a manager-wide reconciliation timeout. All forwarding breaks with `context canceled` | Store a run ctx from a `RunnableFunc` (or the manager ctx) on the Syncer, and use that for handlers | New | CONFIRMED (code + c-r source) |

Checked and fine (not findings): the poller pages correctly, because ngrok-api-go's iterator
follows `next_page_uri` and a mid-page error leaves `iter.Err()` set, so nothing is deleted.
The poller runs leader-only (`mgr.Add` of a plain Runnable defaults to the leader-election
group). Service names can't collide across TLDs (the suffixes are disjoint). Service port names
stay within 15 chars. Orphaned Services after an api-manager outage self-heal through the
Service watch. Port allocation can't double-assign under cache lag (uncached list, 1 worker).
No goroutine or listener leak in `bindingsdriver` Stop/Close beyond R9's spin.

## Test gaps

1. **R19 has no regression test.** No Helm unittest asserts the verbs on
   `*-private-endpoints-manager` / forwarder Roles, and nothing checks markers against the Helm
   Role. Envtest runs as admin, so the next missing verb will also only show up live. Cheapest
   fix: a Helm unittest on the Role rules. Better: an envtest that impersonates the
   ServiceAccount, bound to the rendered Role.
2. **Watch wiring is untested.** Envtest calls `Reconcile` directly. Nothing covers
   `SetupWithManager` (the `byHost` mapping, `GenerationChangedPredicate`, the Service watch):
   CR delete → Service port removed, someone edits a managed Service → restored, label edit
   (PR-9).
3. **R5's startup path is untested.** The `RunnableFunc` "bind failure doesn't stop the
   manager" branch (`syncer.go:144-154`) has no test, and neither does the readiness check
   wiring in `cmd`.
4. **The half-close and drain contract is untested on real TCP.** `TestForwardConnJoinsBothWays`
   uses `net.Pipe` (no `CloseWrite`). There's no test for "client half-closes, still gets the
   response", and none pinning the R8/PR-2 cut-off.
5. **Feature on + CRD missing (R-7)** has no test (PR-1).
6. **Pagination and mid-page errors.** The `nmockapi` mock never pages, so a regression to a
   single page (e.g. passing `Limit` without iterating) wouldn't be caught.
7. **Forwarder table with duplicate `forwarderPort`.** Which CR wins is unspecified and
   untested (PR-7/PR-8).
8. **R2 lagging replica (T2)** is an argument plus a cursor unit test. Nothing two-replica is
   automated.
9. **Uninstall/disable (R16/PR-3)** has no e2e or scripted check.
10. `cmd/api-manager_test.go` only covers cache options. Nothing asserts the poller is added
    as a leader-election runnable.

## Doc claims not backed by code

1. **README R-7 ✅ / strategy.md §R-7 "Met": "no-op without the CRD installed".** True only
   when the feature is **off**. With it on and the CRD missing, api-manager fails at
   `NewManager` (PR-1, confirmed).
2. **README R16 / strategy.md "Uninstall orphans": "the CRD is in a separate chart … likely
   leaves PrivateEndpoint resources".** With the default `installCRDs=true`, the CRD is in the
   same release and is deleted on uninstall, taking the CRs with it. Only the Services are
   orphaned. With `installCRDs=false`, both are orphaned (PR-3).
3. **strategy.md §4.3 / `privatedial_forward.go` header: "a client that shuts down its write
   side still gets the full response".** Only if the response finishes within 5s of the
   half-close. Confirmed cut-off (PR-2).
4. **design.md §1 / strategy: "Users don't author these; read-only mirror".** The controller
   and forwarder wire any PrivateEndpoint in the namespace, including hand-made ones, and
   trust the host label (PR-7).
5. **README "Accepted limitation: every private endpoint is reachable from the cluster".**
   This understates exposure on flat-network CNIs (PR-4, plausible).
6. **README milestone 1: "fixed with tests (R1, R2, R3, R5, R6, R10)".** R5's startup branch
   is untested. R19 (also fixed) has no test. R2 is only narrowed (the tracker says so too).
7. **README "What's built" table: Controller "Known issues R1, R2".** Stale: R1 is fixed.
8. **design.md status example (`clusterIP`/`forwarderPort` "tcp only") and the
   `ForwarderPort` type comment ("shared http/https listener").** Still stale at HEAD
   (= R17).
