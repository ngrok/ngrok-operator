# Events Audit & Remediation Plan

**Status:** temporary working doc. Delete or archive once the codebase conforms to [events.md](events.md).

Tracks: [K8SOP-109 — Audit Events API Surface](https://linear.app/ngrok/issue/K8SOP-109/audit-events-api-surface) (consistency) and [K8SOP-18 — Better Events for Resources](https://linear.app/ngrok/issue/K8SOP-18/better-events-for-resources) (coverage).

This is the implementation plan that brings the code in line with the conventions in [events.md](events.md). The spec is the "what/why"; this is the "how/where", with concrete file:line references from the audit.

## Current state (audit summary)

Baseline is healthier than expected:

- ✅ Modern `events.k8s.io/v1` API via `GetEventRecorder` at all 56 emit sites (11 files). No legacy `record` API anywhere.
- ✅ RBAC already grants `create;patch` on core `events` **and** `events.k8s.io` (recent RBAC overhaul). Core events must stay — controller-runtime leader election still uses the deprecated recorder. The `events.k8s.io` grant also includes an unused `update` verb (optional trim).
- ✅ Recorder component names are consistent kebab-case.
- ✅ 53 / 56 emit sites use `corev1.EventType*` constants.

56 emit sites total. The gaps are consistency (K8SOP-109) and coverage (K8SOP-18), not the wrong API.

## Findings

### Consistency defects (K8SOP-109)

| # | Severity | Finding | Locations |
|---|----------|---------|-----------|
| C1 | **Bug** | `Warning` event with a **success** reason (`"Created"` / `"Updated"`) on failure paths — reason contradicts type + message. The correct string (`ReasonServiceCreationFailed`) already exists as the condition reason on the adjacent line; the event just doesn't use it. | boundendpoint_controller.go:234, 252; boundendpoint_poller.go:425, 510 |
| C2 | High | ~92% of reasons are inline string literals. Only `ngroktrafficpolicy_controller.go` defines constants (`EventPolicyDeprecation`, `EventTrafficPolicyParseFailed`). No shared vocabulary. | all controllers |
| C3 | High | EventType string literals `"Warning"` instead of `corev1.EventTypeWarning`. | ingress_controller.go:91, 94, 97 |
| C4 | Medium | `action` overloaded: `"Reconcile"` used for 15+ distinct failures, collapsing the reason-vs-action distinction. | service, cloudendpoint, agent, ingress, driver.go:793 |
| C5 | Medium | Incoherent reason taxonomy: lifecycle (`Creating`/`Created`), `Failed*` prefix (service), `*NotFound`, plus bindings' `"Delete"` colliding with base's `Deleting`/`Deleted`. `"Status"` reason is vague and near-duplicates `"StatusError"`. | cross-cutting |
| C6 | Medium | Raw `err.Error()` in the `note` → possible info leak, and per-occurrence detail is silently discarded by Series dedup (note isn't in the dedup key) so the variation buys nothing. ~18 sites. | base_controller.go:102, 109, 120, 186 (multiplies across all BaseController users); service/controller.go:294, 300, 456, 478, 493; agent_endpoint_controller.go:165, 454, 512, 539; cloudendpoint_controller.go:137; boundendpoint_poller.go:425, 510; ingress_controller.go:94, 97; drain/orchestrator.go:138; driver.go:793 |
| C7 | Low | `related` used inconsistently — bindings passes the `Service` on success only; everywhere else always `nil`. Adjacent: boundendpoint_controller.go:297/298 and :327/328 double-emit the same failure on both the Service and the CR instead of one event with `regarding`=CR, `related`=Service. | bindings |
| C8 | Low | No nil-guard on `Recorder` except driver.go:777. Real risk only for the driver (`WithEventRecorder` is an optional opt); theoretical for controllers (cmd wiring always sets it) — solve via construction guarantee + one guard in the shared helper. | base_controller.go + all controllers |
| C9 | Low | `internal/domain/manager.go:75-83` accepts and stores a `Recorder` that is never used — dead plumbing; remove or use it. | domain/manager.go |
| C10 | **Bug** | `findTrafficPolicyByName` emits `TrafficPolicyNotFound` on the **empty** `tp` object when the Get fails — the ObjectReference has kind/apiVersion but no name/namespace, so the Event lands orphaned in the `default` namespace where the endpoint owner never sees it. Verified live from **both** sites (same copy-pasted helper). It's also redundant — the owning endpoint separately gets a `CreateError`/`UpdateError` for the same root cause. Emit on the endpoint (`regarding`) instead. | agent_endpoint_controller.go:579; cloudendpoint_controller.go:420 |
| C11 | Medium | Duplicate warnings for one root cause: a missing secret ref emits both `SecretNotFound` (controller) and `UpdateError` (BaseController) per attempt, plus `Normal Updating` and `Normal Status "Successfully reconciled status"` — a *success* event while the resource is broken. 4 events per retry, 2 misleading. Verified live. | agent_endpoint_controller.go + base_controller.go interplay |
| C12 | Medium | Expected transient waits emit `Warning`: a brand-new AgentEndpoint/CloudEndpoint gets `Warning CreateError/UpdateError` "domain is not ready yet" during the normal domain-reservation race, and an internal endpoint-migration race surfaced `Warning` "pooling was set to disabled when another endpoint exists for this url [ERR_NGROK_18017]" 7× in 2s before self-resolving. Same class on the Ingress path: transient `DomainNotReady` warnings (with a leaked raw `HTTP 400: This domain is already reserved for your account`) fire on Ingresses whose domains then go Ready seconds later, with no resolving event. Users see scary warnings on healthy resources. Needs a transient-wait treatment (Normal reason like `WaitingForDomain`, or emit only after N failures / on transition). | domain readiness + endpoint create paths; driver.go:793 |
| C13 | High | One generic `Warning/UpdateError/Update` tuple masks every distinct root cause: bad traffic-policy action (ERR_NGROK_2201), malformed TLS cert (x509), domain-not-ready, and URL-change races all emit the identical (reason, action), differing only in the raw-error note (C6). Meanwhile the **conditions written on the adjacent lines distinguish them cleanly** (`TrafficPolicyError`, `ConfigurationError`, `NgrokAPIError`, `CloudEndpointCreationFailed`) — events are strictly less machine-usable than the conditions. The Reporter helper (Phase 1 item 1) should derive event reasons from the condition reasons. | base_controller.go ErrResult paths across AgentEndpoint/CloudEndpoint |

### Spam risks (both issues)

| # | Finding | Location |
|---|---------|----------|
| S0 | **Worst offender:** `BaseController.Reconcile` emits `Normal "Updating"` + `Normal "Updated"` on **every** steady-state upsert reconcile — 2 events per reconcile across all 7 BaseController users (CloudEndpoint, KubernetesOperator, IPPolicy, Domain, AgentEndpoint, BoundEndpoint ×2 modes). | base_controller.go:107, 112 |
| S1 | Emits `Normal` `"Status"` / "Successfully reconciled status" at the end of **every** `ReconcileStatus()`, ungated on change. Fires for 6 CRD types every reconcile. | base_controller.go:191 |
| S2 | Deprecation `Warning`s fire on every reconcile when a legacy policy is present. ~~Downgraded by testing:~~ the controller is gated by `predicate.Or(AnnotationChanged, GenerationChanged)` (ngroktrafficpolicy_controller.go:95-97), so in steady state it fires exactly once and stops after the policy is fixed (verified live, E14). It still re-fires on every annotation/spec touch and on every operator restart. | ngroktrafficpolicy_controller.go:81, 85 |
| S3 | `"Reconciled"` `Normal` on every successful reconcile. Verified live; a port update also produces only another `Reconciled` — no specific signal. | service/controller.go:327 |
| S4 | `DomainNotReady` `Warning` fires on every Ingress sync while a domain is unready, ungated on transition; also interpolates the domain's Ready-condition message into the note (verified: includes CNAME target and per-occurrence retry timestamps). Measured: **144 emit calls in 6 min for one never-ready Ingress (~24/min)** — but only 2 Event objects (series 4) are visible, because the driver never rewrites Ingress status so the RV stays stable and coalescing holds. `kubectl` understates the real emit volume ~36×. | pkg/managerdriver/driver.go:793 |
| S5 | ippolicy emits its own `Updating`/`Updated` per remote rule-diff pass on top of BaseController's (S0) — up to 4 `Normal`s + `"Status"` for one IPPolicy reconcile. (Could not be verified live: account at the 20-policy cap, create never succeeds, update path never runs.) | ippolicy_controller.go:140, 150 |
| S6 | **Chronic-failure drip:** client-go "finishes" (evicts from dedup cache) any Series not updated within ~6 min. A persistently failing resource's requeue backoff caps at 1000s (16.7 min) — past the 6-min window, **every retry mints a fresh batch of new Event objects** (the C11 quartet = 4 per retry, ~14 objects/hr per broken resource, indefinitely; observed transition precisely: 328s-gap retry still bumped the series, 655s-gap retry minted new objects). Standing count only bounded by the ~1h event TTL. | client-go events broadcaster + BaseController per-reconcile emits |

Note: with the `events.k8s.io` recorder there is **no** client-side rate limiter — the legacy 25-burst token bucket doesn't apply. **And Series coalescing is far weaker than assumed** — it only holds inside a narrow window requiring BOTH: (a) the regarding object's `resourceVersion` is stable (the dedup key embeds the full ObjectReference — any spec or status write breaks it), and (b) recurrences arrive faster than the ~6-min series-finish eviction (S6). Outside that window every emit is a new Event object. Verified end-to-end: healthy resources with zero mutations emit **nothing** (steady-state spam is failure/churn-driven, not resync-driven — the ~10h SyncPeriod never fires in practice and predicates filter no-ops), while a chronically failing resource drips ~14 new objects/hr forever. Also confirmed: `kubectl get events` counts lag real emissions by minutes (series heartbeat flush) — at one point 72 actual emit calls showed as 18. See [events.md](events.md) "Emission cadence" and the evidence tables below.

### Coverage gaps (K8SOP-18)

- **Truly zero events:** the Gateway API controllers (Gateway, HTTPRoute, TCPRoute, TLSRoute, GatewayClass, Namespace, ReferenceGrant) — no direct emits and no BaseController use. Accepted/Programmed/ResolvedRefs transitions are exactly what users debug and are all silent. Verified live — and worse than the code audit suggested: an HTTPRoute with a nonexistent `backendRef` gets `Accepted=True` and **no `ResolvedRefs` condition at all**, and an HTTPRoute whose `parentRef` names a nonexistent Gateway gets a completely **empty status**. For these two misconfigurations the user has zero signal anywhere (no event, no condition). Events here matter even more than the plan assumed because the condition coverage is also thin.
- **Ingress missing-backend is silent:** an Ingress whose rule points at a nonexistent Service emits **no event** — the failure is logged (`cache-store-driver` ERROR, observed 5× in one second) and the Ingress still gets its address published in status. The most common user typo has no kubectl-visible signal. The driver-created child AgentEndpoint/CloudEndpoint gets the (generic BaseController) events instead of the user's Ingress — exactly the "surface child failures on the parent via `related`" rule in [events.md](events.md). Break-and-fix verified silent in **both** directions: breaking the backend deletes the child endpoint but the Ingress keeps publishing its (now dead) address; fixing it recreates the endpoint — no event either way.
- **Ingress missing traffic-policy annotation is worse than missing-backend:** `k8s.ngrok.com/traffic-policy: <nonexistent>` drops the **entire Ingress** from translation (translate_ingresses.go:47-51, resolver :392-399) — no endpoint is ever created — yet the Domain still gets reserved and the address is still published on the Ingress. Zero events, not even an orphan; ERROR logged every sync. A "working-looking" address that routes to nothing.
- **Data-plane failures are 100% invisible:** an AgentEndpoint whose upstream is unreachable provisions to `Ready=True` (all conditions True), serves 502s, and the only signal anywhere is an unstructured agent stdout line (`http: proxy error: dial tcp ... no such host`) — not an event, not a condition, not even a structured log. The tunnel-driver layer emits zero events. This is the #1 user-debug scenario and it's out of scope for every emit site that exists today.
- **Never-verifying custom domain (needs CNAME):** the Domain shows a misleading `Normal Created` while `Ready=False`, emits **no Warning** about the missing DNS record, and because it polls via `RequeueAfter` + status writes, drips an `Updating`/`Status`/`Updated` triple per poll cycle indefinitely (12 objects in 3.5 min observed). The one place a user must act (create the CNAME) is the one thing events never say. Inverse of C12: a stuck state emitting success `Normal`s.
- **Domain condition transitions have no events:** Domain has `CertificateReady`/`DNSConfigured`/`Progressing` conditions but no events fire on their transitions — only generic BaseController lifecycle events. (Adjacent status bug: a failing domain's cert/DNS conditions use reason `ProvisioningError` with an "in progress" message.)
- **Silently ignored config:** an Ingress `tls` section referencing a nonexistent Secret is ignored with no warning; two Ingresses sharing a host merge into one endpoint with no merge/attribution event (low priority, but both violate "user-actionable" coverage).
- **Covered via BaseController, but low quality:** domain, kubernetesoperator, forwarder, cloudendpoint, agent_endpoint, and boundendpoint all route through `BaseController.Reconcile`, so `kubectl describe` already shows Creating/Created/Updating/Updated/Deleting/Deleted/`Status` events (KubernetesOperator additionally gets `DrainStarted`/`DrainFailed`/`DrainCompleted` from internal/drain/orchestrator.go). Their problem is event **quality** — S0/S1 spam and generic reasons — not absence.
- **Error-only:** ingress_controller (3 warnings plus the driver's `DomainNotReady`; nothing on successful sync).

## Empirical evidence (kind cluster, 2026-07-08)

Live experiments: created NgrokTrafficPolicy (valid/legacy/malformed), Domain (valid/unreservable), AgentEndpoint (valid/missing TP ref/missing secret ref), CloudEndpoint, IPPolicy, Ingress (valid/missing backend), Service LB, Gateway + HTTPRoutes (valid/bad backend/bad parent), then mutation bumps, a manager restart, and deletions.

| # | Observation | Implication |
|---|-------------|-------------|
| E1 | 5 annotation bumps on `tp-legacy` → **6 distinct `PolicyDeprecation` Event objects**, no Series coalescing. Root cause verified in client-go v0.36.1 `tools/events/event_broadcaster.go getKey`: dedup key includes `regarding` as a full `ObjectReference` with `resourceVersion`+`uid`. | Series dedup cannot be relied on as a spam bound; transition-gating (Phase 1 item 7) is the only defense. S2 is unbounded, not Series-bounded. |
| E2 | `aep-missing-tp` produced an orphaned `TrafficPolicyNotFound` event in the **`default` namespace** with a name-less regarding ref; `kubectl describe agentendpoint` never shows it. | C10. |
| E3 | 3 no-op annotation bumps on a healthy AgentEndpoint → **9 new Event objects** (`Updating`+`Updated`+`Status` ×3). | S0/S1 live; combined with E1, every mutation-triggered reconcile mints 3 objects. |
| E4 | Manager restart: **62 Event objects in ~90s** for ~15 resources — full `Updating`/`Updated`/`Status` sweep plus re-fired deprecation warnings. | Every rollout/restart re-spams the whole cluster; scales linearly with resource count. |
| E5 | One driver-created CloudEndpoint: **24 Event objects in 2 seconds** — Creating/CreateError/Status retry loop racing domain readiness and an endpoint-pooling conflict (`ERR_NGROK_18017`), all `Warning`s, self-resolved to `Created`. | C12; also shows retry loops with status writes get zero dedup (E1 mechanism). |
| E6 | Broken AgentEndpoint (missing secret): each retry emits `Normal Updating` + `Warning SecretNotFound` + `Warning UpdateError` + `Normal Status "Successfully reconciled status"`. | C11 — duplicate warnings per cause, and success-flavored `Normal`s on a broken resource. |
| E7 | Notes contain raw ngrok API errors incl. per-occurrence **Operation IDs** (`ERR_NGROK_414`, `ERR_NGROK_1410` observed) and multi-line bodies. | C6 live; the varying Operation ID is discarded by dedup anyway when RV is stable, and defeats nothing when it isn't — pure loss. |
| E8 | Ingress with missing backend Service: no event, address still published; only a controller log line. HTTPRoute bad-backend/bad-parent: no event **and** no meaningful condition. | Coverage gaps above; Phase 2 items 8–9. |
| E9 | Deletions look correct: single `Deleting`/`Deleted` pairs per resource, transition-gated by nature. | No action needed. |

### Round 2 (parallel subagent sweeps: CRD edge cases, Ingress/driver depth, endpoint failure modes, 25-min steady-state observation)

| # | Observation | Implication |
|---|-------------|-------------|
| E10 | Second orphan site live: CloudEndpoint with missing `trafficPolicyName` → `TrafficPolicyNotFound` orphan in `default` (n=13 — the empty ref's zero uid/RV is stable, so the bug accidentally coalesces). | C10 covers both sites. |
| E11 | Missing traffic-policy **annotation** on Ingress: whole Ingress dropped from translation, address still published, zero events. Distinct code path from C10's spec-ref case. | Coverage gap above; Phase 2 item 12. |
| E12 | Unreachable upstream: AgentEndpoint `Ready=True`, 502s at runtime, only signal is agent stdout. | Coverage gap above; Phase 2 item 15. |
| E13 | S4 measured: 144 `DomainNotReady` emits / 6 min for one Ingress → 2 visible objects (stable Ingress RV keeps coalescing alive); note interpolates CNAME target + retry timestamps. | S4 fix = gate on transition; kubectl counts hide the real volume. |
| E14 | Legacy policy fixed (`inbound` → `on_http_request`): deprecation warning correctly stops (generation-gated) — but no resolution signal; the stale Warning just ages out. | S2 downgraded; acceptable per spec. |
| E15 | Steady state (25 min + 67 min, zero mutations): healthy Domain/CloudEndpoint/TrafficPolicy = **0 events/hr** (no resync reconciles at all). Failing AgentEndpoint = ~14 new objects/hr forever once backoff (1000s cap) exceeds the 6-min series window. | S6. Spam is churn- and failure-driven only; transition-gating fixes both. |
| E16 | `kubectl get events` lag quantified: 72 actual emit calls visible as 18 at t0, converging minutes later via heartbeat flush. | Don't use event counts for real-time debugging; another reason notes must be stable (C6). |
| E17 | Two AgentEndpoints sharing one URL: both `Ready=True`, zero conflict events — agent endpoints default `poolingEnabled:true`; the ERR_NGROK_18017 conflict class applies to CloudEndpoints only. | Refines C12/E5 scope. |
| E18 | Distinct failure causes (bad TP action ERR_NGROK_2201, malformed x509 cert, domain-not-ready, URL-change race) all emit the identical `Warning/UpdateError/Update`; the adjacent conditions distinguish all of them. Garbage cert PEM leaked no secret bytes into the note. | C13. |
| E19 | IPPolicy at account limit: terminal 400, no requeue → exactly 2 coalesced events, no growth. CloudEndpoint recovery after creating a missing TP: clean `Normal Created` on transition. | Bounded/correct paths — the codebase already demonstrates the target behavior in places. |

### Adjacent non-events bugs found while testing (file separately, out of scope here)

1. **CloudEndpoint stuck after `spec.url` change** — transient `ERR_NGROK_18006` (domain not yet visible) is a 400 not in the retryable list (cloudendpoint_controller.go:118-128 has only 18016/18017), so `CtrlResultForErr` returns `{}, nil` — no requeue; the racing Domain hits terminal Ready so its watch never re-fires; `GenerationChangedPredicate` filters annotation kicks. Result: `Ready=False` indefinitely with a stale Warning as the last event; a generation bump recovers instantly. Violates the repo rule "requeue on transient ngrok API errors".
2. **Auto-created Domain CRs are orphaned** — endpoint URL changes and deletions leave the auto-reserved Domain CRs behind (no `ownerReferences`, never GC'd; 8 Domains observed for 2 live endpoints). No teardown event either.
3. **HTTPRoute condition gaps** — bad backendRef: `Accepted=True`, no `ResolvedRefs` condition; bad parentRef: empty status (also listed under coverage above).
4. **Ingress publishes/keeps a stale address** when translation drops it (missing backend or TP annotation).

### Not yet tested live

- Bindings controllers (BoundEndpoint/BindingConfiguration) — forwarder not deployed; **C1 is code-verified only**.
- Drain events (`DrainStarted`/`DrainCompleted`) — requires KubernetesOperator deletion/uninstall.
- S5 IPPolicy rule-diff double-emit — account at 20-policy cap, update path unreachable.
- Invalid ngrok credentials (cluster-wide failure storm), TCPRoute/TLSRoute (experimental-channel CRDs not installed).

## Plan

### Phase 1 — Consistency & anti-spam (K8SOP-109)

Low risk, mostly mechanical. Do first; it establishes the vocabulary Phase 2 builds on.

1. **Shared vocabulary.** Add `internal/controller/events/events.go` (package `events`) defining:
   - `Reason*` constants (success + failure pairs: `ReasonCreated`/`ReasonCreateFailed`, `ReasonUpdated`/`ReasonUpdateFailed`, `ReasonDeleted`/`ReasonDeleteFailed`, `ReasonSynced`/`ReasonSyncFailed`, `ReasonValidationFailed`, `ReasonRefNotFound`, etc.).
   - `Action*` constants (`ActionCreate`, `ActionUpdate`, `ActionDelete`, `ActionSync`, `ActionValidate`, `ActionResolve`, `ActionReserve`, `ActionDrain`).
   - A cert-manager-`Reporter`-style helper that couples a condition write with the paired event and **only emits when the condition actually transitions**. This is the mechanism for gating S0/S1 (change detection has to exist anyway), and it centralizes the nil-recorder guard (C8). Not optional — it's the enforcement point for most of [events.md](events.md).
2. **Fix C1** — bindings failure events: use `Warning` + `ReasonCreateFailed`/`ReasonUpdateFailed`. (boundendpoint_controller.go:234, 252; poller:425, 510)
3. **Fix C3** — ingress_controller: `"Warning"` → `corev1.EventTypeWarning`.
4. **Migrate all literals** to the shared constants (C2), collapsing the taxonomy in C5 to the canonical set. Includes the sites outside `internal/controller/`: internal/drain/orchestrator.go (3 sites) and pkg/managerdriver/driver.go:793.
5. **Assign real actions** (C4) from the `Action*` vocabulary; stop using `"Reconcile"` as a catch-all.
6. **Strip raw errors from notes** (C6): short stable note, error goes to `log`. The base_controller.go sites fix all BaseController users at once.
7. **Gate the spam (S0–S5)**: BaseController emits Creating/Created only on actual create, Updating/Updated only when the upsert changed something, and the Status event only on a condition/status transition (S0, S1); otherwise `log.V(1)`. Same treatment for S2 (once per resource generation), S3, S4 (on transition to/from unready), S5 (drop the duplicate layer). Remove the dead Recorder in domain/manager.go (C9). Per E1, gating must be at the emit site — do not count on Series dedup to bound anything.
8. **Fix C10** — emit `TrafficPolicyNotFound` on the owning endpoint (`regarding`), not the missing TP. Both sites: agent_endpoint_controller.go:579 and cloudendpoint_controller.go:420 (deduplicate the copy-pasted helper while there).
9. **De-duplicate failure emits (C11) and align reasons with conditions (C13)** — when a controller emits a specific failure event (e.g. `SecretNotFound`), BaseController must not add a generic `UpdateError` for the same reconcile, and `Updating`/`Status` success events must not fire on a failed pass. The Reporter helper should derive the event reason from the condition reason being written (`TrafficPolicyError`, `ConfigurationError`, `NgrokAPIError`, ...) instead of the generic `UpdateError`.
10. **Transient-wait treatment (C12)** — "domain not ready yet" and similar expected self-resolving races: `Normal` reason (e.g. `WaitingForDomain`) emitted once on entering the wait, `Warning` only if still unresolved after a threshold; internal migration/pooling races shouldn't be user-visible warnings at all.

### Phase 2 — Coverage (K8SOP-18)

Higher effort; needs per-controller judgment on which transitions matter. Emit on transition, pair with existing condition writes (do not spam steady state).

11. **Gateway API controllers** (the only true absence gap): `Warning` on Accepted/Programmed/ResolvedRefs failures; `Normal` on the first successful Programmed transition. Surface child failures on the parent via `related`. Strictly transition-gated — this already exceeds what other Gateway implementations emit. Per E8, also fix the missing conditions themselves (`ResolvedRefs=False` for bad backendRefs; a status entry for unresolvable parentRefs) — events alone can't carry this.
12. **Ingress**: `Warning` on backend-Service resolution failure AND on missing traffic-policy annotation (both currently log-only, E8/E11 — the second one silently drops the whole Ingress); `Normal` on successful sync transition; keep existing failure warnings (now using shared constants). Don't publish/keep the address when translation dropped the Ingress (adjacent bug 4).
13. **BaseController users** (domain, kubernetesoperator, forwarder, cloudendpoint, agent_endpoint, boundendpoint): already covered by BaseController lifecycle events — the Phase 1 gating/vocabulary work *is* the fix. Add per-controller events only where a meaningful transition isn't visible through BaseController (e.g. forwarder port bind/unbind); drain events already exist via internal/drain.
14. **Domain DNS/cert visibility**: pair the `CertificateReady`/`DNSConfigured` condition transitions with events; for a custom domain awaiting its CNAME, emit one `Warning` (e.g. `DNSRecordRequired`, with the expected CNAME target) on entering the wait — it's the single most user-actionable state the operator has, and today it emits misleading `Normal`s instead (coverage gap above).
15. **Data-plane failures (stretch, needs design)**: upstream-unreachable is invisible end to end (E12). Events alone can't fix this — the agent would need to observe dial/proxy errors and surface them as a condition + transition event on the AgentEndpoint. Scope separately; capture here so it isn't lost.

## Verification

- **Unit tests** per changed reconciler using the `k8s.io/client-go/tools/events` fake recorder (not the legacy `record.NewFakeRecorder` — wrong API), asserting the `(eventtype, reason, action)` tuple emitted on each branch (success, each failure). Follow existing table-driven test patterns.
- A catalog test over the `Reason*`/`Action*` constants: UpperCamelCase, ≤128 chars, reason/action never equal — server-side validation rejects violations silently at runtime, so catch them in CI.
- `make test` green.
- `make manifests` clean (no RBAC drift — events RBAC already present, confirm unchanged).
- **Manual**: in a kind cluster, create each resource type (Ingress, Service LB, Gateway+HTTPRoute, CRDs) and confirm `kubectl describe` shows meaningful events on both success and induced-failure paths, and that steady-state reconciles do **not** spam.
