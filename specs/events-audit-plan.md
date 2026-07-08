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

### Spam risks (both issues)

| # | Finding | Location |
|---|---------|----------|
| S0 | **Worst offender:** `BaseController.Reconcile` emits `Normal "Updating"` + `Normal "Updated"` on **every** steady-state upsert reconcile — 2 events per reconcile across all 7 BaseController users (CloudEndpoint, KubernetesOperator, IPPolicy, Domain, AgentEndpoint, BoundEndpoint ×2 modes). | base_controller.go:107, 112 |
| S1 | Emits `Normal` `"Status"` / "Successfully reconciled status" at the end of **every** `ReconcileStatus()`, ungated on change. Fires for 6 CRD types every reconcile. | base_controller.go:191 |
| S2 | Deprecation `Warning`s fire on every reconcile when a legacy policy is present. | ngroktrafficpolicy_controller.go:81, 85 |
| S3 | `"Reconciled"` `Normal` on every successful reconcile. | service/controller.go:327 |
| S4 | `DomainNotReady` `Warning` fires on every Ingress sync while a domain is unready, ungated on transition; also interpolates the domain's Ready-condition message into the note. | pkg/managerdriver/driver.go:793 |
| S5 | ippolicy emits its own `Updating`/`Updated` per remote rule-diff pass on top of BaseController's (S0) — up to 4 `Normal`s + `"Status"` for one IPPolicy reconcile. | ippolicy_controller.go:140, 150 |

Note: with the `events.k8s.io` recorder there is **no** client-side rate limiter — the legacy 25-burst token bucket doesn't apply. Stable-tuple spam collapses into a Series (bad but bounded); varying reason/action spam creates unbounded Event objects. See [events.md](events.md) "Emission cadence".

### Coverage gaps (K8SOP-18)

- **Truly zero events:** the Gateway API controllers (Gateway, HTTPRoute, TCPRoute, TLSRoute, GatewayClass, Namespace, ReferenceGrant) — no direct emits and no BaseController use. Accepted/Programmed/ResolvedRefs transitions are exactly what users debug and are all silent. This is the only real absence gap.
- **Covered via BaseController, but low quality:** domain, kubernetesoperator, forwarder, cloudendpoint, agent_endpoint, and boundendpoint all route through `BaseController.Reconcile`, so `kubectl describe` already shows Creating/Created/Updating/Updated/Deleting/Deleted/`Status` events (KubernetesOperator additionally gets `DrainStarted`/`DrainFailed`/`DrainCompleted` from internal/drain/orchestrator.go). Their problem is event **quality** — S0/S1 spam and generic reasons — not absence.
- **Error-only:** ingress_controller (3 warnings plus the driver's `DomainNotReady`; nothing on successful sync).

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
7. **Gate the spam (S0–S5)**: BaseController emits Creating/Created only on actual create, Updating/Updated only when the upsert changed something, and the Status event only on a condition/status transition (S0, S1); otherwise `log.V(1)`. Same treatment for S2 (once per resource generation), S3, S4 (on transition to/from unready), S5 (drop the duplicate layer). Remove the dead Recorder in domain/manager.go (C9).

### Phase 2 — Coverage (K8SOP-18)

Higher effort; needs per-controller judgment on which transitions matter. Emit on transition, pair with existing condition writes (do not spam steady state).

8. **Gateway API controllers** (the only true absence gap): `Warning` on Accepted/Programmed/ResolvedRefs failures; `Normal` on the first successful Programmed transition. Surface child failures on the parent via `related`. Strictly transition-gated — this already exceeds what other Gateway implementations emit.
9. **Ingress**: `Normal` on successful sync transition; keep existing failure warnings (now using shared constants).
10. **BaseController users** (domain, kubernetesoperator, forwarder, cloudendpoint, agent_endpoint, boundendpoint): already covered by BaseController lifecycle events — the Phase 1 gating/vocabulary work *is* the fix. Add per-controller events only where a meaningful transition isn't visible through BaseController (e.g. forwarder port bind/unbind); drain events already exist via internal/drain.

## Verification

- **Unit tests** per changed reconciler using the `k8s.io/client-go/tools/events` fake recorder (not the legacy `record.NewFakeRecorder` — wrong API), asserting the `(eventtype, reason, action)` tuple emitted on each branch (success, each failure). Follow existing table-driven test patterns.
- A catalog test over the `Reason*`/`Action*` constants: UpperCamelCase, ≤128 chars, reason/action never equal — server-side validation rejects violations silently at runtime, so catch them in CI.
- `make test` green.
- `make manifests` clean (no RBAC drift — events RBAC already present, confirm unchanged).
- **Manual**: in a kind cluster, create each resource type (Ingress, Service LB, Gateway+HTTPRoute, CRDs) and confirm `kubectl describe` shows meaningful events on both success and induced-failure paths, and that steady-state reconciles do **not** spam.
