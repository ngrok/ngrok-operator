# Resurrect Research: `k8s.ngrok.com/` → `ngrok.com/` prefix migration

## Your Context

You started with one giant branch that changed **everything** from the `k8s.ngrok.com`
prefix to `ngrok.com`. It was too big to ship as one PR, so it was sliced into smaller
efforts. You've already landed/opened a PR for the **operator-written labels & annotations**
(your current branch). You believed you also had branches in flight for the **finalizer**
rename and the **ingress class controller name**, and thought the one remaining area was
**user-supplied annotations**.

This doc maps the combined WIP branch against the broken-out branches, tells you what's
already extracted, what's still only local, and what (if anything) is genuinely left.

## Branch Summary

| Branch | Pushed? | Ahead / Behind main | Files | Status |
|---|---|---|---|---|
| `alex/migrate-ngrok-com-prefix-wip` | ✅ origin | 1 / 0¹ | 114 | **The combined WIP.** Single squashed commit. Has everything. |
| `alex/migrate-finalizer-r1` | ✅ origin | 3 / 0 | 6 | **Clean & ready.** Finalizer rename (3-release pattern). |
| `alex/migrate-ingress-class-r1` | ✅ origin | 2 / 0 | 7 | **Clean & ready.** Ingress class controller name (deferred-helm). |
| `alex/migrate-operator-written-labels-annotations` (HEAD) | ✅ origin | 1 / 0 | 78 | **Your open-PR branch.** Operator-written labels/annotations/computed-url/bindings. |
| `alex/migrate-user-facing-keys-r1` | ❌ **local only** | 2 / 0 | 19 | **Real, complete R1 — but never pushed.** This *is* the user-supplied-annotations work. |
| `alex/migrate-operator-written-keys-r1` | ❌ local only | 0 / 0 | 0 | **EMPTY placeholder** — identical to main. Delete. |
| `alex/migrate-ngrok-com-prefix` | ❌ local only | 0 / 1 | 0 | **Stale**, points at an old main commit, no migration work. Delete. |

¹ The WIP's merge-base is one commit behind current main; the commit it lacks is your
operator-written branch's tip. Bucket-wise the WIP overlaps heavily with work already
broken out, so it is effectively stale and would conflict if merged as-is.

**Worktrees:** none dedicated per branch. Only the main checkout at `/workspaces/ngrok-operator`.
(The WIP branch does carry four leftover `.claude/worktrees/*` submodule gitlinks named
`finalizer-r1`, `ingress-class-r1`, `operator-written-keys-r1`, `user-facing-keys-r1` —
artifacts of the agent worktrees used to do the split. They should not go into any PR.)

## The headline answer

The breakout is **essentially done**. All four intended slices exist as branches:

1. **Finalizer** → `alex/migrate-finalizer-r1` (pushed, clean)
2. **Ingress class** → `alex/migrate-ingress-class-r1` (pushed, clean)
3. **Operator-written labels/annotations** → your current HEAD branch (pushed, open PR)
4. **User-supplied annotations** → `alex/migrate-user-facing-keys-r1` (**exists with full code, but only local — never pushed**)

So "user supplied annotations" is **not** unwritten work — it's a finished local branch you
forgot to push. The genuinely-remaining area is the **CRD API-group rename**
(`ingress.k8s.ngrok.com`, `ngrok.k8s.ngrok.com`, `bindings.k8s.ngrok.com` → `ngrok.com/v1`),
which was **deliberately deferred** to a separate conversion-webhook workstream and is not in
any of these branches.

---

## The migration strategy (from the design docs)

All branches implement a staged "passivity shim" migration: introduce the canonical
`ngrok.com/` prefix with read-side fallback to legacy `k8s.ngrok.com/`, every legacy-only
code path tagged with a `LEGACY-PREFIX-MIGRATION` sentinel for a single auditable cleanup
sweep later. Two patterns:

- **Two-release** (read-side or dual-write, non-lifecycle keys): R1 read both + dual-write
  both / read-both-write-unchanged; R2 cleanup drops legacy.
- **Three-release** (finalizers — lifecycle-gating): R1 read both, write **legacy only**;
  R2 write **new** + strip legacy; R3 drop legacy entirely. Dual-writing finalizers is
  *worse* than single-write (breaks rollback), so only the *identity* of the single written
  key changes between releases.
- **Deferral** (ingress class): operator binary dual-matches in R1, but the helm-rendered
  manifest stays legacy until R2 to avoid a mid-upgrade rollout race.

<details>
<summary>docs/developer-guide/passivity-shims.md (canonical / most-complete version, verbatim)</summary>

> # Passivity shims and migration strategy
>
> This document is for ngrok-operator maintainers. It describes how we stage backwards-incompatible changes across multiple releases using **passivity shims** — small pieces of read-side and/or write-side compatibility code that let an older operator coexist with a newer one during rolling upgrades and (where possible) survive a `helm rollback`. The user-facing counterpart that lists what each release means for users is [`docs/v1-migration-guide.md`](../v1-migration-guide.md).
>
> ## Why we need shims
>
> A `helm upgrade` (and a rolling `kubectl apply`) does not atomically swap the operator. For a window of seconds to minutes:
>
> 1. The new manifest (with a new IngressClass `spec.controller`, new label selectors expected on AEPs/CEPs, etc.) has been applied.
> 2. The **old** operator pod is still running, watching, and reconciling.
> 3. The new operator pod is starting up.
>
> During that window the old operator can interpret newly-written objects in ways that destroy resources or stall finalizers, unless we constrain *what the new operator writes* during the migration release.
>
> Rollbacks are worse: a `helm rollback` returns the cluster to the prior operator image but leaves objects in whatever state the newer operator stamped them. Anything the newer operator wrote that the older release doesn't understand becomes a hazard.
>
> ## The two patterns
>
> ### Two-release pattern (most sites)
>
> Suitable when the legacy key is non-load-bearing for object lifecycle — i.e. the absence of the legacy key doesn't *block* something like a deletion or trigger a destructive sync.
>
> - **R1 (migration release):** read both prefixes; **dual-write** both prefixes; never delete the legacy key. The legacy key stays present on every object the operator writes.
> - **R2 (cleanup release, immediately before 1.0):** drop legacy-read code; write the new prefix only; delete legacy keys from objects on next reconcile.
>
> Rollback from R1 to the prior release works because the legacy key is still on every object. Rollback from R2 to R1 works because R1 reads the new prefix the rolled-back code wrote.
>
> Used for: operator-written controller labels on `AgentEndpoint` / `CloudEndpoint`, the `ngrok.com/computed-url` annotation on Services, and the bindings labels on Services owned by `BoundEndpoint`.
>
> ### Three-release pattern (finalizer-style cases)
>
> Required when the legacy key gates object lifecycle. Finalizers are the canonical case: Kubernetes will not let an object delete until *every* finalizer is removed, and an older operator only knows how to remove the finalizer key it knew about. Dual-writing both finalizers is **worse** than single-writing — it just guarantees the old operator can't drive a deletion to completion.
>
> - **R1 (migration release):** read both prefixes; `Add` writes the **legacy** key only (no write-side change from the prior release); the `Remove` path removes both keys. R1 is rollback-safe to the prior release (no new-prefix keys exist yet) and forward-safe to R2 (R2 finds objects already carrying the legacy key it knows how to remove).
> - **R2 (next release):** read both prefixes; `Add` writes the **new** key and removes the legacy. `Remove` removes both. Rollback to R1 is safe because R1 knows how to remove the new key.
> - **R3 (cleanup release):** read and write the new key only.
>
> Used for: the operator finalizer (`ngrok.com/finalizer`).
>
> ### Deferral for rollout races
>
> Some changes are safe to ship in the operator binary but unsafe to ship in the rendered helm chart at the same time, because the rendered manifest takes effect mid-upgrade while the old operator pod is still running. The IngressClass `spec.controller` flip is the only example so far. The operator binary gains dual-match in R1; the rendered manifest stays on the legacy value until R2.
>
> ## The `LEGACY-PREFIX-MIGRATION` sentinel
>
> Every code site that exists *only* to support the legacy prefix during a migration window carries the marker `LEGACY-PREFIX-MIGRATION`. Two forms:
>
> ```go
> // LEGACY-PREFIX-MIGRATION: BEGIN
> // ... block to delete ...
> // LEGACY-PREFIX-MIGRATION: END
>
> someLegacyCall(...) // LEGACY-PREFIX-MIGRATION: drop in 1.0
> ```
>
> In the cleanup release for each migration, run:
>
> ```sh
> git grep 'LEGACY-PREFIX-MIGRATION'
> ```
>
> For each hit, delete the block between `BEGIN` / `END` or delete the marked line. The sentinel exists so cleanup is a single, auditable sweep rather than archaeology.
>
> ## Per-shim catalog: `k8s.ngrok.com/` → `ngrok.com/` migration
>
> ### User-facing annotations (read-side compatibility)
> - **Pattern:** Two-release. Read-side only — these are user-set keys, so no operator writes are involved.
> - **R1 (0.24):** `internal/annotations/parser/parser.go` exposes `Get*AnnotationWithFallback` helpers; `internal/annotations/annotations.go` `Extract*` functions read both prefixes; controller reconcile paths call `deprecation.ScanAnnotations` to emit Warning events.
> - **R2 cleanup:** delete the `*WithFallback` family, the `internal/deprecation` package, and the `ScanAnnotations` call sites.
>
> ### Gateway TLS option keys (read-side compatibility)
> - **Pattern:** Two-release. Read-side only.
> - **R1 (0.24):** `pkg/managerdriver/translate_gatewayapi.go` reads `ngrok.com/terminate-tls.*` first, falls back to `k8s.ngrok.com/terminate-tls.*`. Gateway controller emits a single `LegacyAnnotation` event per reconcile when legacy keys are present.
> - **R2 cleanup:** delete the legacy fallback branch and the `warnIfLegacyTLSOptions` helper.
>
> ### Controller labels on AgentEndpoint / CloudEndpoint (operator-written)
> - **Pattern:** Two-release dual-write.
> - **R1 (0.24):** `internal/controller/labels/controller.go`: `ControllerLabels(...)` returns both new+legacy pairs; `EnsureControllerLabels(...)` writes both and does not delete legacy; `HasControllerLabels(...)` matches either; `ControllerLabelSelectors(...)` returns both selectors.
> - **R2 cleanup:** drop the legacy const block, legacy match branch, `LegacyControllerLabelSelector`, legacy entries in `ControllerLabels`; delete `pkg/managerdriver/controller_label_list.go` and re-inline single-selector List in `driver.go::Sync` and `endpoints.go::SyncEndpoints`.
>
> ### `ngrok.com/computed-url` annotation on Services (operator-written)
> - **Pattern:** Two-release dual-write.
> - **R1 (0.24):** `internal/controller/service/controller.go`: `setComputedURLAnnotation` writes both keys, does not delete legacy; `clearComputedURLAnnotation` deletes both; reader `ExtractComputedURL` prefers new, falls back to legacy.
> - **R2 cleanup:** drop legacy write, `Legacy*` const, fallback read branch, legacy-delete in clear.
>
> ### Bindings labels on Services owned by BoundEndpoint (operator-written)
> - **Pattern:** Two-release dual-write.
> - **R1 (0.24):** `internal/controller/bindings/boundendpoint_controller.go`: declare `LegacyLabelEndpointURL` alongside existing legacy consts; include legacy keys in `thisBindingLabels`/`upstreamAnnotations`; merge labels non-destructively in the update branches; readers already dual-read.
> - **R2 cleanup:** drop `Legacy*` consts, the legacy fallback in `boundEndpointLabelsFor`, and legacy writes.
>
> ### Operator finalizer (operator-written, lifecycle-gating)
> - **Pattern:** Three-release dance.
> - **R1 (0.24):** `internal/util/k8s.go`: `HasFinalizer` checks both; `AddFinalizer` adds `LegacyFinalizerName` only; `RemoveFinalizer` removes both.
> - **R2 (0.25):** `AddFinalizer` switches to adding `FinalizerName` and removing legacy.
> - **R3 cleanup (0.26):** delete `LegacyFinalizerName`, legacy branches in `HasFinalizer`, legacy `RemoveFinalizer` call.
>
> ### IngressClass `spec.controller` (rollout-race deferral)
> - **Pattern:** Helm-rendered manifest deferred to cleanup release.
> - **R1 (0.24):** operator binary `ListNgrokIngressClassesV1` dual-matches; CLI flag default flips to new prefix; helm chart stays on legacy.
> - **R2 (0.25):** flip the helm-rendered IngressClass to the new prefix.
> - **R3 cleanup:** drop the dual-match branch in `store.go`.
>
> ## Why we don't shortcut the finalizer
> - **Keep the legacy finalizer forever.** Rejected — prefix unification is being done specifically to make all ngrok-owned keys consistent ahead of 1.0.
> - **Two-release dual-write of both finalizers.** Forward-safe but rollback-broken.
> - **Skip R1 entirely (flip writes in 0.24).** Equivalent to the first cut, and the reason this strategy exists.

</details>

<details>
<summary>docs/v1-migration-guide.md (combined/user-key version from the WIP, verbatim)</summary>

> # ngrok-operator v1 migration guide
>
> This guide tracks the backwards-incompatible changes the ngrok-operator is making on the path to 1.0. Each migration is staged across multiple releases so existing manifests and running deployments keep working during the transition window.
>
> ## Migrations
>
> ### Annotation, label, finalizer, and IngressClass prefix: `k8s.ngrok.com/` → `ngrok.com/`
>
> Status: in progress across 0.24 → 0.25 → 0.26.
>
> #### What changes for you
>
> **User-set annotations:**
>
> | Legacy | New | Applies to |
> | --- | --- | --- |
> | `k8s.ngrok.com/url` | `ngrok.com/url` | Service (LoadBalancer) |
> | `k8s.ngrok.com/mapping-strategy` | `ngrok.com/mapping-strategy` | Service, Ingress, Gateway |
> | `k8s.ngrok.com/traffic-policy` | `ngrok.com/traffic-policy` | Service, Ingress, Gateway |
> | `k8s.ngrok.com/pooling-enabled` | `ngrok.com/pooling-enabled` | Service, Ingress, Gateway |
> | `k8s.ngrok.com/bindings` | `ngrok.com/bindings` | Service, Ingress, Gateway |
> | `k8s.ngrok.com/metadata` | `ngrok.com/metadata` | Ingress, Gateway |
> | `k8s.ngrok.com/description` | `ngrok.com/description` | Ingress, Gateway |
> | `k8s.ngrok.com/app-protocols` | `ngrok.com/app-protocols` | Service (LoadBalancer) |
>
> The legacy Service `appProtocol` field value `k8s.ngrok.com/http2` continues to be recognized; `ngrok.com/http2` is the new preferred value.
>
> **Gateway TLS option keys:** `k8s.ngrok.com/terminate-tls.<option>` → `ngrok.com/terminate-tls.<option>`
>
> Each legacy-key hit on a user-owned resource emits a Warning event with reason `LegacyAnnotation`:
> ```sh
> kubectl get events -A --field-selector reason=LegacyAnnotation
> ```
>
> **IngressClass `spec.controller`:** the operator dual-matches both prefixes when `controllerName` is the new default. The helm chart renders the legacy value through 0.24; flips in 0.25.
>
> **Operator-written labels/keys (update external selectors before cleanup release):**
>
> | Legacy | New |
> | --- | --- |
> | `k8s.ngrok.com/finalizer` | `ngrok.com/finalizer` |
> | `k8s.ngrok.com/computed-url` | `ngrok.com/computed-url` |
> | `k8s.ngrok.com/controller-name` | `ngrok.com/controller-name` |
> | `k8s.ngrok.com/controller-namespace` | `ngrok.com/controller-namespace` |
> | `bindings.k8s.ngrok.com/endpoint-binding-name` | `ngrok.com/endpoint-binding-name` |
> | `bindings.k8s.ngrok.com/endpoint-binding-namespace` | `ngrok.com/endpoint-binding-namespace` |
> | `bindings.k8s.ngrok.com/endpoint-url` | `ngrok.com/endpoint-url` |
>
> #### Action required, by release
>
> | Release | Reads | Operator writes | What you do |
> | --- | --- | --- | --- |
> | 0.24 (this) | Both prefixes | Legacy keys on its own objects; both prefixes on labels / computed-url; legacy finalizer only | Nothing required. Optionally start updating your YAML; both work. |
> | 0.25 | Both prefixes | New keys; legacy removed on next reconcile; new finalizer | Finish updating YAML and external selectors. |
> | 0.26 | New prefix only | New prefix only | Confirm no legacy keys remain. |
>
> #### Supported upgrade path
> `previous-stable → 0.24 → 0.25 → 0.26`.
>
> ## What did *not* change in this set of migrations
> The CRD API groups (`ingress.k8s.ngrok.com/v1alpha1`, `ngrok.k8s.ngrok.com/v1alpha1`, `bindings.k8s.ngrok.com/v1alpha1`) are **unchanged**. A separate 1.0 workstream will consolidate these into `ngrok.com/v1` with a conversion webhook.

</details>

---

## Branch: `alex/migrate-finalizer-r1` (pushed, clean, R1 ready)

- Merge-base `038e7b2e` (= main tip); 3 ahead / 0 behind; 6 files; 2026-05-28→29.
- Commits: `9963283` feat(util): R1 finalizer rename — write legacy, dual-read · `3466f12` make fix · `2861846` fix tests.
- **Old:** `k8s.ngrok.com/finalizer` → **New:** `ngrok.com/finalizer`. Consts in `internal/util/k8s.go:13` (`FinalizerName`) and `:19` (`LegacyFinalizerName`).
- R1 semantics: `HasFinalizer` matches either; `AddFinalizer` writes **legacy only**; `RemoveFinalizer` strips both. Sentinel-tagged. Tests assert R1 behavior.
- No WIP/TODO markers. Local == origin. **Ready to PR.**

## Branch: `alex/migrate-ingress-class-r1` (pushed, clean, R1 ready)

- Merge-base `038e7b2e`; 2 ahead / 0 behind; 7 files; 2026-05-28.
- Commits: `0d53fc1` feat(ingress): R1 IngressClass controller name flip with deferred helm · `88bc838` fix so the store respects overrides a bit better.
- **Old:** `k8s.ngrok.com/ingress-controller` → **New:** `ngrok.com/ingress-controller`. CLI default flip in `cmd/api-manager.go`; consts `defaultIngressControllerName` / `legacyDefaultIngressControllerName` in `internal/store/store.go`.
- `ListNgrokIngressClassesV1` dual-matches **only** when running on either stock default; custom controller names get strict exact-match (multi-instance isolation). Helm chart deliberately untouched.
- No new WIP markers (the 3 TODOs in touched files are pre-existing/unrelated). Local == origin. **Ready to PR.**
- Minor doc nit: developer guide vs user guide disagree on whether ingress-class cleanup is 0.25/R2 or 0.26/R3.

## Branch: `alex/migrate-operator-written-labels-annotations` (HEAD, your open PR)

- Merge-base `038e7b2e`; 1 ahead / 0 behind; 78 files (66 are `pkg/managerdriver/testdata/*.yaml` golden updates); 2026-06-02. Single squash commit `0ce82eb`.
- Three operator-written sets, all dual-read + **dual-write** (R1):
  1. **Controller labels** → `ngrok.com/controller-name|namespace` (`internal/controller/labels/{common,controller}.go`); new dual-List helper `pkg/managerdriver/controller_label_list.go`; four-key backfill in `internal/domain/manager.go`.
  2. **computed-url** → `ngrok.com/computed-url` (`internal/annotations/annotations.go`, `internal/controller/service/controller.go`); re-stamps legacy-only Services.
  3. **Bindings labels** → `ngrok.com/endpoint-binding-name|namespace`, `ngrok.com/endpoint-url` (`internal/controller/bindings/boundendpoint_controller.go`).
- Complete for R1. No deprecation/event machinery (correct — these keys migrate silently).

## Branch: `alex/migrate-user-facing-keys-r1` (LOCAL ONLY — needs push)

- Merge-base `038e7b2e`; 2 ahead / 0 behind; 19 files; 2026-05-28.
- Commits: `8183158` R1 read-side dual-prefix for user-facing keys · `6ec0795` docs: R1 user-facing keys migration guide.
- **This is the "user-supplied annotations" work — already written, just never pushed.** Migrates all user-set annotation consts in `internal/annotations/annotations.go` (`url`, `mapping-strategy`, `traffic-policy`, `pooling-enabled`, `bindings`, `metadata`, `description`), plus `AppProtocolsAnnotation` and the `appProtocol` http2 value (`pkg/managerdriver/utils.go`), plus Gateway `terminate-tls.*` keys.
- Back-compat is the most sophisticated of the set:
  - `internal/annotations/parser/parser.go`: new `Get*AnnotationWithFallback` family + `LegacyHitFunc` + `keysForFallback`. (Note: `parser.AnnotationsPrefix` still defaults to **legacy** during the window.)
  - **New package `internal/deprecation/deprecation.go`**: `Annotation()`/`Label()`/`ScanAnnotations()` emit a structured log + Warning Event (reason `LegacyAnnotation`); `userFacingAnnotationSuffixes` lists the 8 user-facing suffixes.
  - Ingress + Gateway reconcilers call `deprecation.ScanAnnotations`; Gateway separately warns on legacy `terminate-tls.*`; Service reconciler + translator thread `(log, recorder, obj)` through `Extract*`.
- R1 only (R2/R3 documented, not implemented).
- **Known gotchas to fix before PR:** dangling references to a non-existent `docs/migration-v1-prefix.md` in `internal/annotations/annotations.go:100`, `internal/annotations/parser/parser.go:227`, `internal/deprecation/deprecation.go:26`, and `AGENTS.md:14` — the doc actually shipped as `docs/v1-migration-guide.md`.

## Branch: `alex/migrate-operator-written-keys-r1` (DEAD — delete)

Empty. 0 commits ahead of main, identical to `038e7b2e`. A placeholder name; the real
operator-written work landed in `alex/migrate-operator-written-labels-annotations`.

## Branch: `alex/migrate-ngrok-com-prefix` (DEAD — delete)

Stale. Points at old main commit `07f944c4`, 0 ahead / 1 behind, no migration work. Reflog
shows only the branch-create entry.

---

## Synthesis

### Timeline & Progression
The combined WIP (`migrate-ngrok-com-prefix-wip`, 2026-05-28, 114 files, one squash) was
sliced the **same day** into per-bucket R1 branches (finalizer, ingress-class, user-facing
all dated 2026-05-28). The operator-written slice was finished/polished later (2026-06-02)
and became the open-PR HEAD branch. Two leftover names (`operator-written-keys-r1`,
`migrate-ngrok-com-prefix`) are dead.

### What Overlaps / Where They Diverge
- The two slices that touch `internal/annotations/annotations.go` (user-facing and
  operator-written) edit the **same const block** from different angles — user-facing flips
  the user keys and leaves `computed-url` legacy; operator-written flips `computed-url`.
  Independent merges will textually conflict there and in `internal/controller/service/controller.go`.
- Every branch shipped its **own copy** of the two shared docs
  (`docs/v1-migration-guide.md`, `docs/developer-guide/passivity-shims.md`), each tailored to
  its slice. The HEAD branch's `passivity-shims.md` and the WIP's `v1-migration-guide.md` are
  the most complete. Landing multiple slices means reconciling these docs into one catalog,
  not taking one side.

### Mapping to Your Context
- *"branches for the finalizer and the ingress class controller name"* → **confirmed**, both
  exist, pushed, clean: `migrate-finalizer-r1`, `migrate-ingress-class-r1`.
- *"1 more area is user supplied annotations"* → **already done** as
  `migrate-user-facing-keys-r1`; it's just **not pushed**. Not unwritten work.
- *"old wip branch that had everything combined"* → `migrate-ngrok-com-prefix-wip`. Still the
  full reference, but now mostly redundant with the broken-out branches and effectively stale.

### Discrepancies between WIP code and the documented R1 design (decide when finalizing)
The WIP code went further than R1 in places the design doc says should still be R1:
- WIP `internal/util/k8s.go` writes the **new** finalizer (R2 behavior); the finalizer
  branch correctly walked this back to legacy-only (R1). The finalizer branch is the correct one.
- WIP flipped the **helm-rendered** IngressClass to new; the ingress-class branch correctly
  keeps helm on legacy (R1 deferral). The ingress-class branch is the correct one.
- Conclusion: the **broken-out branches are more correct than the WIP**. The WIP is a
  reference, not something to merge.

## Carry-Forward Candidates

1. **Push `alex/migrate-user-facing-keys-r1`** and open its PR — fix the four dangling
   `docs/migration-v1-prefix.md` references first.
2. **Open PRs for `migrate-finalizer-r1` and `migrate-ingress-class-r1`** — both are clean,
   rebased on main, and ready.
3. **CRD API-group rename** (`*.k8s.ngrok.com` → `ngrok.com/v1`) — the one genuinely
   remaining area, intentionally deferred to a separate conversion-webhook workstream. Two
   `TODO(v1-group-rename)` markers track it. Not started anywhere.
4. **Doc reconciliation** — when more than one slice lands, merge the per-branch copies of
   `v1-migration-guide.md` / `passivity-shims.md` into one canonical catalog; resolve the
   R2/R3 (0.25/0.26) labeling mismatch on the ingress-class branch.
5. **Delete dead branches** `migrate-operator-written-keys-r1` and `migrate-ngrok-com-prefix`
   (local). Consider deleting the WIP branch once you're confident the slices cover it.
6. **Strip WIP-only noise** if you ever cherry-pick from the WIP: release-skill/agent
   deletions, `CLAUDE.md`/`AGENTS.md` edits, and the four `.claude/worktrees/*` gitlinks.
