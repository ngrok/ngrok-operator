# Private Endpoints in Kubernetes — requirements

Status: POC · Owner: Alex Bezek · Updated: 2026-10-12

What users get, what the POC covers, what we accept, and what must close before preview and GA.
Decisions (D1–D8) and risks (R1–R19) live in the [tracker](README.md); design in
[design.md](design.md); rationale in [strategy.md](strategy.md).

IDs: `R-n` is a product requirement (this doc). `Rn` without the hyphen is a risk in the
tracker. `GAP-n` is a to-do to close a gap.

Verification state:
- **Verified** — an automated test or a recorded live run (cited) shows it.
- **Partial** — part is covered; the rest is named.
- **Unverified** — claimed by design or code reading only.

Paths are relative to the repo root. "Working tree" means uncommitted on
`alex/private-endpoints-poc` as of this date (D8 is landing).

---

## 1. Product requirements

### Reach

| ID | Requirement | Verified by | State |
|---|---|---|---|
| R-1 | Pods reach a private endpoint by its real URL. `https://*.ngrok.direct` verifies with ngrok's public cert, no client config | `tests/chainsaw-private-endpoints/http` (expects `200 ssl_verify=0`); [findings.md](findings.md) rev 2 e2e; demo run 2026-10-12 (tracker milestone 3) | Verified (kind) |
| R-2 | All endpoint schemes work: http, https, tls (end-to-end, origin cert), tcp | chainsaw `http` / `tls` (subject) / `tcp` (redis `+PONG`) tests; `internal/privateendpoints/naming_test.go` `TestParseURL` (schemes, default ports); findings.md rev 2. Recorded runs used `.internal` for tls/tcp; the D8 demo moves them to `.ngrok.direct` and hasn't run yet | Verified (kind) |
| R-3 | No per-endpoint Kubernetes config, no sidecar, no app change. One admin DNS step per cluster | Demo client is stock `nicolaka/netshoot`; demo origins are plain AgentEndpoints (`scripts/private-endpoints-demo/demo.yaml`); nothing else is created per endpoint | Verified (kind demo, 2026-10-12) |
| R-8 | Only `*.ngrok.direct` is mirrored. `*.internal` is not supported (D9): no Helm value, flag, or kind DNS rule | `internal/controller/privateendpoints/privateendpoint_poller_test.go` `TestPollerSync` "creates only ngrok.direct endpoints, skips internal, public and kubernetes-bound" and "deletes managed CRs that are gone" (a managed `.internal` CR is deleted); `naming_test.go` (`.internal` is not a private hostname, gets no Service name); chainsaw `dns` test (a `.internal` AgentEndpoint gets no PrivateEndpoint) | Verified (unit); chainsaw not yet run live |
| R-9 | Endpoint lifecycle converges within about one poll (10s): new endpoints appear, stopped ones disappear, pooled endpoints collapse into one resource | findings.md (rev 2 cleanup: CR and Service gone ~2s after agent stop); `privateendpoint_poller_test.go` `TestPollerSyncLabelsAndDedupe`; `internal/controller/privateendpoints/privateendpoint_controller_test.go` "removing one endpoint drops its port, last one deletes the service" | Verified |
| R-17 | Normal cluster DNS keeps working. Multi-label names (`metadata.google.internal`) are not captured | chainsaw `dns` test (`kubernetes.default.svc.cluster.local` NOERROR); findings.md rev 2 multi-label fall-through (temporary CoreDNS `hosts` entry) | Verified (kind) |

### Safety and correctness

| ID | Requirement | Verified by | State |
|---|---|---|---|
| R-4 | Separate CRD that coexists with bindings. `kubernetes`-bound endpoints are never mirrored | `privateendpoint_poller_test.go` `TestPollerSync` (`http://svc.ns` and `http://legacy.ngrok.direct` with `kubernetes` binding skipped). Running both features at once: no test or recorded run | Partial (GAP-12) |
| R-5 | Forwarder is bytes-only: no DNS server, no Host/SNI parsing, no TLS | Code: `internal/controller/privateendpoints/forwarder_controller.go:102` looks up `host:port` by forwarder port, `privatedial_forward.go` only copies. Helm test asserts no `dnsPolicy` override on the forwarder | Verified (by construction) |
| R-18 | A client that half-closes its write side still gets the full response | Not met for slow responses: the 5s drain starts when **either** direction ends, so a response that takes >5s after the client's `shutdown(SHUT_WR)` is cut (`privatedial_forward.go` `join`; research/07 PR-2, confirmed in a scratch test). Works for fast responses. No test on real TCP (`TestForwardConnJoinsBothWays` uses `net.Pipe`) | Not met (GAP-16, R8) |
| R-6 | An ngrok API error never deletes mirrored resources | `privateendpoint_poller_test.go` "list error keeps CRs"; `privateendpoint_poller.go:65` returns before any write | Verified (unit) |
| R-7 | Feature is off by default | `values.yaml` `ngrok.features.privateEndpoints.enabled: false`; Helm test "renders nothing when disabled"; `cmd/api-manager_test.go` `TestManagerCacheOptions` "private endpoints disabled pins nothing for them". The CRD ships in the ngrok-crds chart with no toggle, so "on without the CRD" is a broken install and is not guarded (D10) | Verified (unit, Helm) |
| R-10 | The operator never takes over objects it didn't create: a same-named Service is left alone (R1); an unlabeled same-named `PrivateEndpoint` is left alone | `privateendpoint_controller_test.go` "unmanaged service with the same name is left alone" (`ServiceConflict`, Service unchanged); `privateendpoint_poller_test.go` "leaves CR without managed-by label alone", "deletes managed CRs that are gone, keeps unmanaged ones" | Verified (unit) |
| R-11 | The mirror self-heals: edits to a managed resource's spec or poller labels are reverted on the next poll (R3), with RBAC that allows it (R19) | `privateendpoint_poller_test.go` `TestPollerSyncHealsDrift`. RBAC (`update` on `privateendpoints`, `helm/ngrok-operator/templates/private-endpoints/rbac.yaml:51`) has no test: envtest runs as admin and no Helm test asserts Role verbs. Live recheck after R19 fix not done | Partial (GAP-2) |
| R-12 | A connection for one endpoint never reaches another: forwarder ports are unique across hostnames and a freed port isn't reused right away | `privateendpoint_controller_test.go`: "ports are unique across hosts and stable across reconciles", "port allocation ignores cache lag", "duplicate forwarder port across hosts is reallocated", "a freed port is not reused while higher ports are free", "allocation wraps to freed low ports once the top is used"; `privateendpoints/forwarder_controller_test.go` `TestForwarderReconcilerFollowsChanges` (accept-time lookup). A lagging replica can still misroute a reused port (R2, D5); restart can reuse a top port at once (R12) | Partial (GAP-3) |
| R-13 | Every way a resource can't be wired shows as `Ready=False` with a reason: `UnsupportedHostname`, `PortConflict`, `PortsExhausted`, `ServiceConflict`, `ServiceNotReady` | `privateendpoint_controller_test.go` covers `UnsupportedHostname`, `ServiceConflict`, `PortsExhausted`. `PortConflict` (`privateendpoint_controller.go:153`) and `ServiceNotReady` (`privateendpoint_controller.go:197`) untested | Partial (GAP-5) |
| R-14 | Forwarder readiness reflects whether it can serve: table synced and no listener bind errors (R5). A bind failure at startup doesn't crashloop. Liveness never depends on ngrok | `privateendpoints/forwarder_controller_test.go` `TestForwarderReconcilerReportsBindFailures`, `TestForwarderReconcilerBindFailureClearsWhenNoLongerWanted` (`BindErr`). The readyz wiring (`cmd/private-endpoint-forwarder-manager.go:120`) and the startup no-exit path (`privateendpoints/forwarder_controller.go:150`) have no test | Partial (GAP-6) |
| R-15 | Missing forwarder token fails at `helm install`, not at pod start (R6) | Helm tests "fails when only the agent and api-manager tokens are set", "fails when no credentials are set at all". A custom `ngrok.credentials.secret.name` can't be checked; documented in `values.yaml` | Verified (helm-unittest) |
| R-16 | The feature needs only namespace-scoped RBAC in the release namespace | `helm/ngrok-operator/templates/private-endpoints/rbac.yaml` has Roles only. No test asserts it | Unverified (inspection) |

---

## 2. POC scope

In:
- `PrivateEndpoint` CRD (`ngrok.com/v1`), poller, per-hostname Service controller, forwarder.
- Helm wiring behind `ngrok.features.privateEndpoints.enabled`. `.ngrok.direct` only; `.internal` is out of scope (D9).
- kind CoreDNS helper (`make kind-private-endpoints-dns`) and canned demo (`make pe-demo-up` / `pe-demo-test`).
- Runbooks for kind, EKS, k3s (Alex runs them; not yet run on EKS/k3s).

Non-goals (deferred, not rejected):

| Non-goal | Where it's tracked |
|---|---|
| Access control (which pods/namespaces reach which endpoints) | GAP-20 |
| Per-pod identity on the dial | GAP-21, GAT-475 |
| Operator editing cluster DNS on any platform | D4 (decided: docs only) |
| DNS on platforms other than kind / EKS / k3s | D1, D7, GAP-19 |
| Multi-label hostnames (`api.foo.ngrok.direct`) | D1(b), D2 |
| Multiple operator installs or accounts per cluster | D6, GAP-25 |
| IPv6 / dual-stack | GAP-24 |
| More than ~1000 endpoint-ports per cluster | GAP-23 |
| HTTP/2 fallback (non-UDP egress) | GAP-22 |
| Operator's Ingress/Gateway emitting `.ngrok.direct` internal endpoints | Future, post-1.0 (GAP-27) |
| Removing bindings | GAP-26 |

---

## 3. Accepted limitations (POC)

Deliberate and documented. Each needs a decision before GA (section 4).

| Limitation | Ref |
|---|---|
| Every private endpoint in the account is reachable from every pod. DNS opt-in is not authorization. On flat-network CNIs (EKS VPC CNI) forwarder pod IPs may also be reachable from the VPC and peered networks, which widens this (research/07 PR-4, plausible) | strategy.md §6, GAP-29 |
| Controller and forwarder act on any `PrivateEndpoint` in the operator namespace, not only poller-made ones, and trust its host label. A hand-made CR can claim a hostname's port and redirect it (research/07 PR-7) | GAP-31 |
| No pod identity on the dial; Traffic Policy can't match the caller | GAT-475 |
| DNS setup is manual. Tested only in kind | D4, D7 |
| Single-label hostnames only. Label is DNS-1035, ≤50 chars before `.ngrok.direct` (`internal/privateendpoints/naming.go` `ServiceName`) | design.md |
| QUIC only: needs UDP/443 egress. `privatedial` pinned to feature-branch commit `e2b70148` | design.md |
| ~1000 endpoint-ports per cluster (20000–20999, hard-coded in `cmd/api-manager.go`). Replicas add throughput, not capacity | design.md |
| `Ready` means configured, not end-to-end reachable | strategy.md §4.2 |
| Connection to a port a hostname no longer serves times out instead of being refused | R7 |
| Fixed 5s drain after either direction ends, including a client half-close, so slow responses after a half-close are cut; no idle timeout | R8, PR-2 |
| Removing the managed-by label from a resource stops healing it; log repeats each poll | R11 |
| Port cursor is in memory; after restart a top-of-range freed port can be reused at once | R12 |
| A stale bind error can linger on readiness if the next list fails | R14 |

---

## 4. Gaps

Owner is a placeholder until assigned. "Verify" is the exit check.

### Before preview

- [x] **GAP-1** ~~Land D8~~ Superseded by D9 (2026-10-21): `.internal` removed instead of opt-in. Verify: `go test ./internal/controller/privateendpoints/... ./internal/privateendpoints/...`, `make helm-test`, chainsaw `dns` test.
- [ ] **GAP-2** Lock in R19 (R-11). Add a Helm test that the api-manager Role has `update` on `privateendpoints`; redeploy kind and confirm drift heal live. Owner: _TBD_. Verify: helm-unittest case; edit a CR's spec in kind, see it restored within one poll (tracker milestone 9).
- [ ] **GAP-3** Run the port-reuse experiment (T2) and pick a D5 direction (R-12). Owner: _TBD_. Verify: result recorded in findings.md (two forwarders, paused watch, delete A, reuse port for B); if a fix lands, a test that fails without it.
- [ ] **GAP-4** Uninstall and feature-disable cleanup (R16, PR-3). Expected from code: with the default `installCRDs=true` the CRD is in the same release, so `helm uninstall` deletes it and its CRs, and only the per-host Services are orphaned (no ownerRef; they select forwarder pods that are gone). With `installCRDs=false`, or when only `ngrok.features.privateEndpoints.enabled` is turned off, both CRs and Services stay and the Role that could clean them up is removed. Fix (ownerRef on Services, or label cleanup in the drain hook) or document cleanup. Owner: _TBD_. Verify: `runbooks/kind-e2e.md` §5 F, recorded in findings.md.
- [ ] **GAP-5** Test the untested Ready reasons `PortConflict` and `ServiceNotReady` (R-13). Owner: _TBD_. Verify: new cases in `privateendpoint_controller_test.go`.
- [ ] **GAP-6** Test forwarder readiness wiring and the startup bind-failure path (R-14). Owner: _TBD_. Verify: unit test that a held port makes readyz fail and the manager keeps running; or a kind check with the port occupied.
- [ ] **GAP-7** Rerun full kind e2e after all fixes and D8 (T10), plus DNS behavior checks (T6): search domains and ndots on glibc, musl and Go images; negative-cache delay on create. Owner: _TBD_. Verify: dated entry in findings.md.
- [ ] **GAP-8** Validate on EKS (managed add-on `corefile`, survives add-on update), then k3s (`coredns-custom`) (T7). Owner: Alex. Verify: results recorded per `runbooks/eks.md` and `runbooks/k3s.md`.
- [ ] **GAP-9** User-facing setup docs and preview contract (T9): kind/EKS/k3s guides, `.ngrok.direct`-first DNS snippets; contract covers account-wide exposure, no identity, Ready semantics, UDP/443, ~1000 ports, one DNS owner per suffix, cleanup. Owner: _TBD_. Verify: someone else follows the guide on a fresh cluster.
- [ ] **GAP-10** Bump the ngrok-crds chart for the new CRD (`helm/ngrok-crds/Chart.yaml` is still `0.4.0`, same as `main`). Owner: _TBD_. Verify: version bumped; `helm template helm/ngrok-crds` includes `privateendpoints.ngrok.com`.
- [ ] **GAP-11** Land the R17 fix: `ForwarderPort` comment in `api/ngrok/v1/privateendpoint_types.go` no longer says "shared http/https listener" (working tree, with regenerated `helm/ngrok-crds/templates/ngrok.com_privateendpoints.yaml`). Owner: _TBD_. Verify: `make manifests` leaves no diff; `grep -i shared` on the generated CRD finds nothing.
- [ ] **GAP-12** Run with bindings enabled at the same time (R-4). Owner: _TBD_. Verify: kind run with `ngrok.features.bindings.enabled=true` and `ngrok.features.privateEndpoints.enabled=true`; a `kubernetes`-bound endpoint gets a `BoundEndpoint` and no `PrivateEndpoint`; demo checks still pass.
- [ ] **GAP-13** Forwarder ignores `status.forwarderPort` outside the configured range (R13). Status write in the operator namespace is otherwise routing control. Owner: _TBD_. Verify: `privateendpoints/forwarder_controller_test.go` case with an out-of-range port opens no listener.
- [x] **GAP-28** ~~api-manager survives the feature being on without the CRD~~ Won't do (D10, 2026-10-21): the CRD ships in the ngrok-crds chart with no toggle; the discovery check was removed.
- [ ] **GAP-29** Forwarder exposure on flat-network CNIs (PR-4): optional NetworkPolicy in the chart allowing ingress to 20000–20999 only from cluster pods/namespaces; call it out in the EKS runbook and preview docs. EKS is next in D7. Owner: _TBD_. Verify: on EKS, a VPC host outside the cluster can't connect to `<forwarder-pod-ip>:2000x` with the policy on; in-cluster pods still can.
- [ ] **GAP-31** Only wire poller-made `PrivateEndpoint`s (PR-7): controller and ForwarderReconciler filter on the managed-by label and skip CRs whose host label ≠ `HostKey(spec.hostname)`. Wrong-destination risk, and it's what "read-only mirror" promises. Owner: _TBD_. Verify: envtest with a hand-made CR claiming another host's label and a lower name: it gets no port and no Service; syncer test that an unlabeled CR opens no listener.

### Before GA

- [ ] **GAP-14** Decide `Ready` semantics: keep "configured", or surface forwarder bind state and/or a DNS self-check (research M1). Owner: _TBD_. Verify: decision recorded in tracker; tests for whatever is surfaced.
- [ ] **GAP-15** Connection to a removed port is refused, not timed out (R7). Owner: _TBD_. Verify: e2e check that connect fails fast.
- [ ] **GAP-16** Fix the half-close cut-off (R-18, R8, PR-2): start the drain only after upstream→client EOF, or replace the fixed drain with an idle timeout, ideally as the SDK contract. Owner: _TBD_ (with SDK owner). Verify: real-TCP test where the client half-closes and the upstream replies after more than the drain, and still gets the full response. Before preview if a preview user runs half-closing clients (`nc -N`, piped `redis-cli`).
- [ ] **GAP-17** `bindingsdriver` IPv4-only bind and accept-error spin (R9). Owner: _TBD_. Verify: unit test that accept errors back off.
- [ ] **GAP-18** Persist or reseed the port cursor so restart doesn't reuse a just-freed port (R12), or subsume under the D5 fix. Owner: _TBD_. Verify: envtest restarting the reconciler.
- [ ] **GAP-19** Portable DNS: spike D1(b) DNS server behind a conditional forward (T8); if it holds, hash Service names (D2) and lift single-label limits. Proves GKE and OpenShift. Owner: _TBD_. Verify: kind, then GKE Standard `stubDomains`, then OpenShift DNS Operator, each recorded.
- [ ] **GAP-20** Access-control model (account side, cluster side, Traffic Policy, or a mix). Owner: _TBD_ (product). Verify: decision in tracker; e2e that a denied pod can't connect.
- [ ] **GAP-21** Per-connection pod identity once the backend exposes it (GAT-475). Owner: _TBD_ (backend + operator). Verify: Traffic Policy on `conn.k8s.pod.*`-style fields matches in e2e.
- [ ] **GAP-22** Upstream `privatedial.Join` / `Dialer.Forward`, move off the `e2b70148` pin, retest HTTP/2 on Go 1.27 to drop the UDP/443 requirement. Owner: _TBD_ ([sdk-upstream.md](sdk-upstream.md)). Verify: `internal/privateendpoints/forwarder/privatedial_forward.go` deleted; e2e over HTTP/2 with UDP blocked.
- [ ] **GAP-23** Scale: benchmark at 100, 300, 1000 endpoint-ports; decide range or allocation changes. Owner: _TBD_. Verify: numbers recorded in findings.md.
- [ ] **GAP-24** IPv6 / dual-stack. Owner: _TBD_. Verify: Service, listener, DNS and private dial pass on a dual-stack kind cluster.
- [ ] **GAP-25** Multiple installs per cluster (D6). Preview constraint: one DNS owner per suffix. GA path: per-cluster base domain (`foo.mycluster.ngrok.direct`) once ngrok ships agent `baseDomain`; needs multi-label names (GAP-19). Owner: _TBD_. Verify: two installs in one cluster each resolve their own zone.
- [ ] **GAP-26** Bindings deprecation criteria and migration guide. Owner: _TBD_ ([migration.md](migration.md)). Verify: guide followed end to end on kind.
- [ ] **GAP-30** Forwarder availability (PR-5): graceful shutdown (preStop delay, stop accepting, wait for joins up to the grace period), and a PDB. The scheduling knobs (`nodeSelector`, `tolerations`, `affinity`, `priorityClassName`, `terminationGracePeriodSeconds`) now come from `components.common` / `components.privateEndpointForwarder` since the values refactor (#899). Default `replicaCount: 1` makes a node drain a full outage; preview docs state the limitation. Owner: _TBD_. Verify: rolling restart with a long-lived tcp connection open and two replicas: no connect failures, in-flight connection survives to the grace period; Helm tests for the new values.

### Post-1.0

- [ ] **GAP-27** Operator's Ingress/Gateway/Service translation emits internal AgentEndpoints on `*.ngrok.direct` instead of `*.internal` (create new, switch CLEP policy, delete old). Needed before `.ngrok.direct` mirroring would pick up operator-made endpoints (R18). Owner: _TBD_. Verify: e2e upgrade with no traffic gap.
