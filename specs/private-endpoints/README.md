# Private Endpoints in Kubernetes — tracker

One-stop status for the POC on `alex/private-endpoints-poc`: what's built, what we promise,
what's in flux, what's next. Detail lives in the linked docs; this file links, it doesn't copy.

Last updated: 2026-10-12 · Owner: Alex Bezek · Phase: **finishing the bounded POC**

## Doc map

| Doc | What it is | Status |
|---|---|---|
| [design.md](design.md) | Revision 2 design (CoreDNS rewrite + Service per hostname) | Current |
| [notes.md](notes.md) | Knowledge base, decision log, review status, research brief | Current |
| [findings.md](findings.md) | Private dial probe + kind e2e results (rev 1 and rev 2) | Current, 2026-09-29 |
| [plan.md](plan.md) | Implementation plan for revision 1 (own DNS server + Host/SNI) | **History**, superseded |
| [requirements.md](requirements.md) | Product requirements (R-n) with how each is verified, POC scope, accepted limitations, gaps to close (GAP-n) before preview / GA | Current, 2026-10-12 |
| [strategy.md](strategy.md) | In-depth strategy for operator devs + product: how it works, vs bindings, shortcomings, challenges, open questions | Reviewed 2026-10-12; D8/D6 folded in |
| [migration.md](migration.md) | Bindings → private endpoints (parity, phases M0–M4, shim), and operator `.internal` → `.ngrok.direct` (post-1.0) | Draft 2026-10-12 |
| [sdk-upstream.md](sdk-upstream.md) | What moves to ngrok-go `privatedial` (Join, Forwarder.Serve), proposed API + contract, asks for SDK team | Draft 2026-10-12 |
| [runbooks/](runbooks/) | Manual test runbooks: kind-e2e, eks, k3s (Alex runs; agents never touch real clusters) | Draft 2026-10-12 |
| [demo.md](demo.md) | Canned kind demo: `make pe-demo-up` / `pe-demo-test`, in-cluster origins via AgentEndpoint, live demo script | First run passed 2026-10-12; D8 version not yet run |
| [research/](research/00-summary.md) | Deep-research bundle (review, prior art, platform playbooks, recommendation, doc drafts, experiments) | Input, 2026-10-12. Reconstructed, not rerun; claims need checking |
| [routing-safety.md](routing-safety.md) | Wrong-destination scenarios vs bindings (R25 new; R2/R12 better than bindings; R26 same class), minimal fix O1+O2 | Draft 2026-10-13, for review |
| [research/07-poc-review.md](research/07-poc-review.md) | Independent adversarial review of the POC code: PR-1..PR-12, test gaps, doc claims not backed | 2026-10-12 |

## Goal (one line)

Any pod reaches any ngrok private endpoint by its real URL (`foo.ngrok.direct`; `foo.internal` opt-in)
over http/https/tls/tcp, no per-endpoint config, no app changes. Replaces Kubernetes bindings
(`BoundEndpoint`), which are deprecated alongside, not removed.

## What's built (rev 2)

```
ngrok API ──poll 10s──▶ PrivateEndpoint CRs ──▶ controller: Service per hostname, port per endpoint → forwarderPort
                                           └──▶ forwarder: listener per forwarderPort → privatedial(host:port)
pod ─ foo.ngrok.direct ─▶ CoreDNS rewrite ─▶ foo-ngrok-direct.<ns>.svc ─▶ ClusterIP ─▶ forwarder ─▶ ngrok
(.internal only with ngrok.features.privateEndpoints.includeInternal, D8)
```

| Piece | Where | State |
|---|---|---|
| `PrivateEndpoint` CRD (`ngrok.com/v1`) | `api/ngrok/v1/privateendpoint_types.go` | Done |
| Hostname → Service naming rules | `internal/privateendpoints/naming.go` | Done, tested |
| Poller (API → CRs, client-side filter, drift heal, `.internal` opt-in) | `internal/controller/privateendpoints/poller.go` | Done, tested |
| Controller (Services, cursor port alloc 20000–20999, conflict/exhaustion conditions) | `internal/controller/privateendpoints/controller.go` | Done, envtest |
| api-manager wiring, feature flag, missing-CRD guard (PR-1) | `cmd/api-manager.go` | Done |
| `private-endpoint-forwarder` subcommand | `cmd/private-endpoint-forwarder.go`, `internal/privateendpoints/forwarder/` | Done, reuses `pkg/bindingsdriver` listeners |
| Dial/copy helper (to upstream) | `internal/privateendpoints/forwarder/privatedial_forward.go` | Isolated, awaiting SDK |
| Helm (`ngrok.features.privateEndpoints.enabled`, off by default) | `helm/ngrok-operator/templates/private-endpoints/` | Done, snapshot tests |
| kind CoreDNS setup | `scripts/kind-private-endpoints-dns.sh`, `make kind-private-endpoints-dns` (`PE_INCLUDE_INTERNAL=true` for `.internal`) | Done, kind only |
| Kind demo | `scripts/private-endpoints-demo/`, `make pe-demo-*` | Done; 10/10 checks 2026-10-12 |
| Live e2e | [findings.md](findings.md) | Passed 2026-09-29: http, https (verify on), tls passthrough, tcp/redis, 2 ports on one host, both TLDs, cleanup within a poll |

## Requirements, limitations, gaps

Live in [requirements.md](requirements.md); not copied here. Snapshot 2026-10-12:
R-1..R-18, each with how it's verified (most Verified in kind, several Partial, R-18 half-close Not met).
Gaps: GAP-1..13, 28, 29, 31 before preview; GAP-14..26, 30 before GA; GAP-27 post-1.0.

Headline limitations: every `.ngrok.direct` private endpoint in the account is reachable from every pod;
no pod identity; manual DNS (kind scripted); single-label hostnames; UDP/443 egress; ~1000 endpoint-ports;
`Ready` = configured, not proven end to end.

## Risks and bugs (open)

Research findings I checked against the code are marked **confirmed**.

| ID | Issue | Severity | Status |
|---|---|---|---|
| R1 | Controller `CreateOrUpdate` overwrites a pre-existing Service with the same name, no ownership check (`controller.go:142`) | High | **Fixed** (`a7040ea6`): unowned Service left alone, CRs get `ServiceConflict`, 30s recheck |
| R2 | Port reuse can cross-route tcp: `allocate` hands out lowest free port, so a freed port is reused immediately; a stale forwarder replica may still map it to the old host | High (wrong destination) | **Narrowed** (`a7040ea6`): cursor allocation, freed port reused only after full range cycles. Not closed: lagging replica can still misroute a reused port (see D5) |
| R3 | Poller doesn't heal spec drift (treats CR as present by name; no Update) | Medium | **Fixed** (`058b945b`, RBAC in `a878d1c1`): spec + poller labels restored each poll; unlabeled same-name CR left alone. Live recheck pending |
| R4 | `.internal` capture collides with existing names; GKE also has single-label `metadata.internal` | High for `.internal` on GKE | **Mitigated by D8** once landed (`.internal` off by default). Remains for opt-in users |
| R5 | Forwarder bind failures hidden after initial sync; readiness only checks first sync | Medium | **Fixed** (`c083a3a9`): bind errors logged + fail readiness; startup bind failure no longer crashloops |
| R6 | Forwarder fails to start if token key `PRIVATE_ENDPOINTS_ACCESS_TOKEN` missing; should fail at Helm render | Low | **Fixed** (`c083a3a9`): Helm `fail` when no token source; custom secret documented |
| R7 | Connection to a removed port times out instead of refusing | Low | Accepted for now |
| R8 | No idle timeout; fixed 5s half-close drain may cut legit one-way streams | Low | Belongs in SDK contract |
| R9 | `bindingsdriver` binds `0.0.0.0` (no IPv6), accept-error spin | Low | Open |
| R10 | Port exhaustion: error + requeue, no clear condition | Low | **Fixed** (`a7040ea6`): `PortsExhausted` condition, 30s recheck |
| R11 | Removing managed-by label from a CR stops healing; log repeats every poll | Low | Accepted for POC |
| R12 | Port cursor is in memory; after restart/failover a top-of-range freed port can be reused at once | Low | Accepted for POC |
| R13 | Forwarder opens listeners for any status port, no range check | Low | Open |
| R14 | Stale bind error lingers on readiness if next PrivateEndpoint list fails | Low | Accepted for POC |
| R15 | Rewrite catches single-label apexes `ec2.internal.` / `compute.internal.` (EKS) and `google.internal.` (GCE search domain) → NXDOMAIN. Node names (`ip-x.ec2.internal`) unaffected | Low–Medium | **Mitigated by D8** (no `.internal` rewrite by default). Remains for opt-in users; measured in eks.md / k3s.md runbooks |
| R17 | Stale rev-1 wording: `ForwarderPort` comment in `api/ngrok/v1/privateendpoint_types.go` ("shared http/https listener"); design.md says clusterIP/forwarderPort "tcp only" | Low | **Fixed, working tree** (type comment + regenerated CRD; design.md rewritten) |
| R18 | Operator's own Ingress/Gateway translation creates `*.internal` AgentEndpoints (CLEP + internal AEP default). The poller mirrors those too, so in-cluster traffic to them hairpins out through private dial and back in, and they add noise/ports. Confirmed naming: Ingress/Gateway internal URLs are single-label (`pkg/managerdriver/utils.go:111`, dashes, `.internal`) so get mirrored if ≤54 chars; LoadBalancer Service URLs are `tcp://<uid>.<name>.<ns>.internal` (`internal/controller/service/controller.go:584`), multi-label, so become `UnsupportedHostname` CRs | Medium | **Mitigated by D8** (operator-generated endpoints are all `.internal`, not mirrored by default). Deferred for opt-in users; real fix is the Ingress/Gateway → `.ngrok.direct` move (Future). Do NOT filter user-authored AgentEndpoints |
| R19 | Poller drift healing (R3 fix) was denied by RBAC: api-manager Role lacked `update` on `privateendpoints`; envtest runs as admin so tests missed it. Found live in kind demo | High (R3 fix inert) | **Fixed** (`a878d1c1`). Helm Role-verb regression test added (working tree); live recheck pending |
| R16 | Cleanup on uninstall / disable. Per review (PR-3): default `installCRDs=true` puts the CRD in the same release, so `helm uninstall` deletes CRD + CRs and orphans only per-host Services; `installCRDs=false` or disabling the feature orphans both | Medium | Open (GAP-4). Live check: kind-e2e §5 F |
| R20 | Feature on + CRD missing crashed the whole api-manager (Ingress/Gateway/bindings too) (PR-1) | Medium | **Fixed, working tree**: discovery check like Gateway; warn and disable feature |
| R21 | Forwarder half-close: 5s drain starts when *either* side ends, so slow replies after client half-close are cut (PR-2; sharper R8) | Medium | **Fixed, working tree**: client half-close starts no timer; drain only after upstream EOF. Bindings is worse (full close on first EOF). sdk-upstream contract updated. No idle timeout yet |
| R22 | Forwarder on flat-network CNIs (EKS VPC CNI): ports reachable from VPC, no NetworkPolicy (PR-4) | Medium | Plausible; GAP-29, check on EKS |
| R23 | Controller/forwarder trust any CR in the namespace; a hand-made CR can claim a host's port (PR-7) | Medium | Open (GAP-31) |
| R24 | Forwarder: no graceful shutdown/preStop/PDB, 1 replica default, no scheduling knobs (PR-5) | Low–Med | Open (GAP-30) |

| R25 | **Wrong destination via partial write.** Controller publishes the Service (targetPort P) *before* writing P to CR status (`controller.go:170` then `:249`), and allocation reserves only ports found in CR status (`controller.go:68-82,108-116`), not existing Service targetPorts. Status write fails or leader dies → new leader doesn't see P → another host gets P → Service A routes to B. No stale forwarder needed | **High, preview blocker** | Confirmed. **New vs bindings** (bindings writes port in BoundEndpoint spec at create, before any Service). **Fixed, working tree** (O1, [routing-safety.md](routing-safety.md)): allocation also treats other hosts' managed Services' targetPorts as used (one extra uncached Service list per reconcile). Tests FT-1a/b/c |
| R26 | Forwarder ambiguous mapping: duplicate forwarderPort across CRs → last entry wins silently (`table.go:32`). Should fail closed | High (with R25) | Confirmed. Same class in bindings (keeps first). **Fixed, working tree** (O2): duplicated port resolves to nothing, connections closed without dial, logged once per change. Tests FT-2a/b/c |
| R27 | Copy errors discarded (`privatedial_forward.go:71,76`): mid-stream upstream failure becomes orderly EOF to client, so truncated data looks complete. Should reset, not FIN | High (silent truncation) | **Fixed, working tree**: error aborts both sides (TCP RST), returned + logged. Bindings has it too (`forwarder_controller.go:331`). privatedial-side reset needs SDK (A11) |
| R28 | Forwarder stays Ready while unable to serve: SDK `Done()/Err()` (terminal auth failure, e.g. revoked token) not observed; liveness always OK; informer staleness undetected | Medium | **Mostly fixed, working tree**: not Ready until first Connect; exits on terminal session end (restart re-reads token). Bad token *at startup* stays not-Ready (SDK A12). Informer staleness still open. Bindings: same blind spot, never stuck (fresh TLS per conn) |
| R29 | Endpoint presence coupled to address lifetime: one successful list without the endpoint deletes CR → Service → ClusterIP. Ephemeral origin restart across a poll churns the IP apps may cache, and feeds port reuse | Medium | Confirmed live (findings 2026-10-13): new ClusterIP + port after a 25s origin outage; caching clients never recover; re-resolving clients wait ~30s extra (CoreDNS negative cache). Bindings deletes on first missed poll too. Open: retention period for Service + allocation |
| R30 | DNS trust boundary: rewrite resolves by Service *existence*, not validated ownership. Refusing to overwrite a foreign `foo-ngrok-direct` Service (R1) doesn't stop `foo.ngrok.direct` resolving to it; plain http/tcp has no TLS backstop. Service-write in the operator namespace = authority over cluster-wide names | Medium | Open. Document as trust boundary now; D1(b) DNS responder narrows it. Relates to R23/GAP-31 |

Other review findings (PR-6, 8–12) are low and tracked in [research/07](research/07-poc-review.md).
R25–R30 come from a second independent review (4 subagents: control-plane convergence, forwarding/HA, DNS/deployment, security/migration), 2026-10-12. It also sharpened: bindings replacement changes **authorization** (bindings scopes to a KubernetesOperator and carries client-cert identity; prod + staging in one account becomes cross-reachable on migration — see migration.md P1/P2), and forwarder token scope/rotation needs an explicit backend contract (unproven that it's admin, unproven that it isn't).

## In flux (decisions not made)

| ID | Question | Options | Leaning |
|---|---|---|---|
| D1 | DNS integration for real clusters | (a) CoreDNS rewrite per platform, (b) small ngrok DNS server + admin-configured conditional forward (Tailscale model), (c) pod `dnsConfig` webhook, (d) TUN/eBPF | Keep (a) for POC/compatible clusters; prototype (b) for portability. (c)/(d) parked |
| D2 | If (b): Service naming | Keep hostname-derived names vs hash names (DNS owns mapping, lifts single-label + length limits) | Hash, if (b) lands |
| D5 | Port-reuse guarantee (R2) | Cursor alloc (done) / plus reuse quarantine / replica ack (epoch) | Cursor is better than bindings (bindings rebuilds its bitmap every poll, so it effectively reuses lowest free port; its forwarder also pins target at listen time). Stronger guarantee post-POC if wanted |
| D6 | Multiple installs / accounts per cluster | (a) one DNS owner per suffix; users hand-carve per-name rules. (b) per-cluster base domain (`foo.mycluster.ngrok.direct`), cluster DNS forwards each zone to its operator. (c) operator DNS server that cluster DNS delegates to | (a) for POC. Slack 2026-10-12 (Eric): a configurable agent-level `baseDomain` is easy to add on the ngrok side but doesn't exist yet. (b) needs multi-label hostnames, so it rides on D1(b)/D2. Open Q: can endpoint URLs be `foo.<sub>.ngrok.direct` today? |

## Decided

| ID | Decision | Date | Why |
|---|---|---|---|
| D3 | *(Partly superseded by D8)* Support both TLDs out of the box. `.ngrok.direct` is primary. `.internal` is secondary: hold it (or mark unsupported) on any platform where it collides (e.g. GKE `metadata.internal`) | 2026-10-12 | Private endpoints are trending to `.ngrok.direct` for browsers, but `.internal`-bound endpoints already exist and are used outside browsers; unclear if they go away |
| D4 | Operator does not edit cluster DNS. Docs first; automation later only to prove things out. kind script stays as a dev helper | 2026-10-12 | Matches research rec and prior direction; DNS is admin-owned |
| D8 | `.ngrok.direct` only by default. `.internal` projection is opt-in (poller skips `.internal` hosts unless enabled; kind DNS rewrite follows). Supersedes the "both out of the box" half of D3 | 2026-10-12 | PM (Slack): `.ngrok.direct` is the prescribed pattern going forward; `.internal` stays supported but secondary. Also defuses R4, R15, R18 by default |
| D7 | Platform order: kind (local dev) → EKS (ngrok dogfoods) → k3s (ngrok devboxes). AKS, GKE, OpenShift, RKE2: basic "how this works" docs only, no tested support claim | 2026-10-12 | Platforms we run ourselves first |

## Future (post-POC, not started)

- Access control: which pods/namespaces may reach which endpoints.
- Pod identity per connection → Traffic Policy (needs backend; compare old bindings pod-IP lookup).
- Upstream `privatedial.Join` / `Dialer.Forward` to ngrok-go; delete local helper; move off pin; retest HTTP/2 on Go 1.27.
- Multi-label hostnames (needs D1(b)).
- IPv6 / dual-stack.
- Scale beyond ~1000 endpoint-ports.
- Bindings deprecation + migration guide.
- Operator's Ingress/Gateway/Service translation emits internal AgentEndpoints on `*.ngrok.direct` instead of `*.internal`. Migration: create new AEPs, then switch the CLEP traffic policy to forward to them, then delete old ones. Post-1.0. PM agrees it's mostly technical (Slack 2026-10-12).
- Configurable per-cluster base domain (D6b), once ngrok ships agent `baseDomain`.
- Release: bump ngrok-crds chart for the new CRD.

## Correctness bar (from 2nd review)

Invariant: **a connection reaches its intended endpoint or fails.** Not "ports are usually unique".
Wrong-destination routing (R2, R25, R26) and silent truncation (R21, R27) are not acceptable POC limitations.
Single install, IPv4, QUIC-only, bounded scale stay explicit POC constraints.

Fault tests required before calling the POC correctness-confirmed (none exist yet):
Kind scenarios for FT-3/5/6: `make pe-demo-faults FAULT=origin-restart|rolling-restart|uninstall|revoke` ([demo.md](demo.md)). Expected to FAIL on today's code for R29/R24/R16; bindings shares the R24 and R29 failures.

- [x] FT-1 Leader death / status-write failure between Service publish and status publish (R25) — envtest `TestRoutingSafety` (fails on old code)
- [ ] FT-2 Stale forwarder + endpoint delete + port reallocation (R2) — duplicate/retarget parts covered (`TestSyncerAmbiguousPort`, `TestSyncerRetargetsOpenListener`); lagging-replica window itself accepted (better than bindings, routing-safety.md)
- [x] FT-3 Origin restart across a poll — run live 2026-10-13, FAILS as expected (R29), with a client that caches DNS (R29)
- [x] FT-4 Half-close then delayed response; upstream reset mid-response (R21, R27) — loopback TCP (`TestForwardConnOverTCP`); not yet over real privatedial
- [ ] FT-5 Credential revocation / reconnect; rolling restart with held connections (R28, R24) — rolling restart run live 2026-10-13, FAILS as expected (held conn cut, 1/10 new conns failed); revoke not run (manual)
- [x] FT-6 Disable / uninstall / re-enable — disable/re-enable run live 2026-10-13 (orphans as expected; re-enable recovers same IPs/ports); `helm uninstall` path not run incl. DNS removal and orphans (R16)

## Next up (ordered)

Full list with verification steps: [requirements.md §4](requirements.md). T1–T10 from the old to-do list are folded in.

1. Alex: live kind run on the D8 working tree (GAP-1, GAP-2 live, GAP-4 via kind-e2e §5 F). Confirms tls/tcp on `.ngrok.direct`.
2. Agents: GAP-31 (only wire poller-made CRs), GAP-13 (forwarder port range check). Small, code-only, before preview.
3. Agents: GAP-16 half-close fix shaped as the SDK `Join` contract (sdk-upstream SDK-1).
4. Ask SDK/backend owners: sdk-upstream asks A1–A10; migration open questions (esp. `.ngrok.direct` hostname scope, multi-label).
5. Later: GAP-3 port-reuse experiment, GAP-29 NetworkPolicy with EKS run, GAP-30 forwarder availability.

## POC milestones (in order, no dates)

1. ✅ Confirmed correctness bugs fixed with tests (R1, R2, R3, R5, R6, R10). Committed.
2. ✅ Strategy doc for operator devs + product ([strategy.md](strategy.md)). Committed.
3. ✅ Canned kind demo. First run 2026-10-12: 10/10 checks. Found R19 (fixed, `a878d1c1`).
4. 🔄 D8: `.ngrok.direct` default, `.internal` opt-in (poller, flag, Helm, kind DNS, demo, runbooks). Working tree, unit + Helm tests pass; needs Alex review + live run.
5. ✅ Review hardening: PR-1 fixed; tests for GAP-2 (Helm Role verbs, exact match), GAP-5 (PortConflict, ServiceNotReady), GAP-6 (readyz, startup bind failure). Working tree. Note: kubebuilder RBAC markers are docs only (Helm Roles hand-written), so the Helm test is the real guard.
6. ✅ Docs: requirements.md, design.md rewrite, strategy/notes/runbooks updated for D8 and review. Working tree.
7. ✅ Post-POC plans: [sdk-upstream.md](sdk-upstream.md), [migration.md](migration.md). Working tree.
8. Live: R19 drift-heal recheck, D8 demo both modes, R16 uninstall check (Alex, kind).
9. Before-preview code gaps: GAP-31, GAP-13, then GAP-16.

Later (not POC): EKS/k3s runs, user-facing docs, DNS server spike (GAP-19), identity, access control, SDK upstream, IPv6.

## Changelog

- 2026-10-13: Rebased onto main after the Helm values refactor (#899) and regrouped history into 9 reviewable commits. Chart: `ngrok.features.privateEndpoints.{enabled,includeInternal}` (env `NGROK_OPERATOR__FEATURES__PRIVATE_ENDPOINTS__*`, defined in `internal/flags`), forwarder pod settings under `components.privateEndpointForwarder` (inherits `components.common`, so GAP-30 scheduling knobs come free), token `ngrok.credentials.privateEndpointForwarder.accessToken` (secret key unchanged: `PRIVATE_ENDPOINTS_ACCESS_TOKEN`). plan.md/research/findings.md keep the old names. Re-run live on kind after the rebase: demo 11/11 pass, settings reach pods as env vars.
- 2026-10-13: Answers (Alex): `.ngrok.direct` private hostnames are NOT globally unique or reserved — private, so many accounts can each have `foo.ngrok.direct` (no cross-account collision/leak; migration.md Q6 closed). Multi-label `.ngrok.direct` not supported today; Eric wants agent `baseDomain` subdomains later (D6b). Abort (A11) and startup bad-token detection (A12): backend/SDK-owned, Alex can inquire; A12 low value (pod is not-Ready either way).
- 2026-10-13: Live kind run (findings.md): demo 11/11 incl. tls/tcp on `.ngrok.direct`; R19 heal, PR-1 missing CRD confirmed live; faults: origin-restart, rolling-restart, disable/re-enable fail as expected (R29, R24, R16). New: CoreDNS negative cache adds ~30s after an origin outage.
- 2026-10-13: Routing fixes O1 (R25) + O2 (R26) in working tree; only the new-vs-bindings bug fixed, rest accepted per routing-safety.md.
- 2026-10-13: Iteration 2 (working tree): R21/R27 copy fix, R28 forwarder health, kind fault scenarios, routing-safety.md (R25 is the only new-vs-bindings routing bug). Every fix compared against bindings first.
- 2026-10-12: Second independent review: R25–R30, correctness bar + fault tests FT-1..6. Routing identity (R25) is now the top blocker. Holding commits while the POC iterates.
- 2026-10-12: Agent round: D8 implemented; PR-1 fixed; requirements.md, sdk-upstream.md, migration.md, research/07 review written; design/strategy/notes/runbooks/demo updated for D8. Risks R20–R24 added from review. Tracker now points to requirements.md for requirements, limitations, gaps.
- 2026-10-12: Slack answers folded in. PM: `.ngrok.direct` is the prescribed pattern, `.internal` secondary → D8 (`.internal` opt-in), mitigates R4/R15/R18. Eric: agent `baseDomain` would enable per-cluster subdomains (D6b). Ingress → `.ngrok.direct` move filed as post-1.0 future work. R19 fix + demo committed.
- 2026-10-12: Fixes, strategy.md, runbooks committed. Slack: raised multi-operator (D6) and asked PM about `.internal` vs `.ngrok.direct` (D3b). Added R18 (mirroring operator-created `.internal` AEPs).
- 2026-10-12: Fixes for R1/R2/R3/R5/R6/R10 landed on worktree branches; merged on `alex/private-endpoints-poc-integration` for verification, pending review.
- 2026-10-12: Decided D3 (both TLDs, `.ngrok.direct` primary), D4 (docs-only DNS), D7 (kind → EKS → k3s).
- 2026-10-12: Tracker created. Research bundle moved to `research/`. R1–R3 confirmed in code.
- 2026-10-12: Deep-research bundle added (reconstructed; claims unverified unless marked).
- 2026-09-29: Rev 2 (CoreDNS rewrite) built and e2e-tested in kind. Rev 1 removed.
- 2026-09-17: Direction set (DNS-based private dial, no `kubernetes` binding carried forward).
