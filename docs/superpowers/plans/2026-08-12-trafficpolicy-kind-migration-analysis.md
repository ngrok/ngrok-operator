# NgrokTrafficPolicy → TrafficPolicy kind rename: usage map and passive-migration plan

**Status:** Implemented — kept as the rationale record. The recommendation in
[§ Recommendation: Sequence C](#recommendation-sequence-c) is what shipped: the
v1alpha1-only kind rename was dropped and the kind rename was folded into the
`ngrok.com/v1` group move, as a single dual-CRD dual-read migration under one
`LEGACY-trafficpolicy-kind` sentinel. The as-built description — the authority
for the cleanup sweep — lives in
[`docs/developer-guide/passivity-shims.md`](../../developer-guide/passivity-shims.md)
§"Per-shim catalog: TrafficPolicy CRD kind + group rename". Where the two
disagree, that section wins; this doc is frozen at its 2026-08-12 wording.
**Date:** 2026-08-12.
**Author:** j.mcclary@ngrok.com.
**Precedes:** an R1 implementation plan (not yet written) that will convert commit `84c12924` from a hard rename into either a passive dual-kind release *or* a full revert-and-fold-into-v1, depending on the sequencing decision in [§ Interaction with the `ngrok.com/v1` group move](#interaction-with-the-ngrokcomv1-group-move).

## Why this doc exists

Commit `84c12924` ("refactor: rename NgrokTrafficPolicy to TrafficPolicy") is a **hard, atomic rename**: the old CRD `ngroktrafficpolicies.ngrok.k8s.ngrok.com` is deleted; the new CRD `trafficpolicies.ngrok.k8s.ngrok.com` replaces it; the Go type `NgrokTrafficPolicy` is deleted and `TrafficPolicy` replaces it in scheme registration, RBAC, tests, and manifests. Every user in the field with a `kind: NgrokTrafficPolicy` object loses that object on upgrade — the CRD is gone, so the storage rows are gone with it.

That is not passive. The upgrade is not rollback-safe, does not survive `helm upgrade` mid-flight (old pod reads a resource whose CRD has been deleted), and gives no user-visible deprecation window.

This doc catalogs **every code and manifest site** that has to change to convert `84c12924` into a passive migration release, and recommends the migration mechanism.

## TL;DR

- **The load-bearing decision is sequencing.** There is a parallel
  `ngrok.k8s.ngrok.com/v1alpha1` → `ngrok.com/v1` group-move workstream in
  flight. That migration also cannot use a conversion webhook (webhooks
  don't cross groups) and also lands as dual-CRD dual-read. So we have two
  passive migrations queued up on the same conceptual resource.
  **Recommend Sequence C: revert `84c12924` and fold the kind rename into
  the `ngrok.com/v1` group move.** Users do one migration, not two; the
  operator implements dual-read once, not twice; the intermediate state
  is two shapes, not three. See [§ Interaction with the `ngrok.com/v1`
  group move](#interaction-with-the-ngrokcomv1-group-move) for the full
  argument.
- **If Sequence C is rejected** and we ship the kind rename at v1alpha1
  first: **we should treat this as two CRDs coexisting in the same
  group/version, not one CRD with two names.** Kubernetes has no
  kind-aliasing primitive — a CRD is keyed by `plural.group`, and
  `ngroktrafficpolicies.ngrok.k8s.ngrok.com` and
  `trafficpolicies.ngrok.k8s.ngrok.com` are separate storage. The
  operator has to serve both in R1, with `TrafficPolicy` canonical and
  `NgrokTrafficPolicy` legacy-readable.
- **This doesn't fit any pattern in
  [`passivity-shims.md`](../../developer-guide/passivity-shims.md).**
  The existing shims cover label prefix, annotation prefix, finalizer
  name, CR field rename, and CR field-type change. Nothing covers a
  CRD *kind* rename or a CRD *group* rename. The mechanism needs to
  be added to that guide as a new pattern (`LEGACY-KIND-MIGRATION` if
  we ship the kind rename separately; `LEGACY-GROUP-MIGRATION` for
  the group move regardless). The two pattern families are the same
  shape.
- **Two-release pattern applies** (R1 migration release + R-cleanup),
  *not* the three-release default. The rationale mirrors the
  CloudEndpoint `trafficPolicyName` case: the migration doesn't gate
  object lifecycle (no finalizer), the operator doesn't write
  TrafficPolicy CRs itself (they are user-authored), and there is
  nothing to delete-on-reconcile because a legacy-only object is *the
  whole object*, not a legacy key on a canonical object. R-cleanup =
  drop dual-read of legacy shape and delete the legacy CRD.
- **Users migrate their own manifests.** No operator-driven backfill
  copies legacy objects into canonical ones — that would create
  duplicate storage rows with divergent status. Instead: R1 emits
  `DeprecatedKind` (Sequence A) or `DeprecatedAPIGroup` (Sequence C)
  warning events on the referenced-from endpoint whenever a legacy
  shape is resolved; the migration guide documents the `kubectl`
  recipe.
- **The user-facing docs are wrong about conversion webhooks.** Both
  `docs/v1-migration-guide.md:516-520` and `specs/migration-v1.md:47-56`
  say a conversion webhook handles the `ngrok.com/v1` migration. That's
  incorrect — webhooks convert versions of a single CRD; a group rename
  is two different CRDs. `docs/developer-guide/passivity-shims.md:625`
  already has the right story ("dual-read migration rather than a
  webhook"). Both user-facing docs need updating regardless of
  sequencing.

## Scope: what has to change

Every category below has to be re-doubled to support both kinds. The table gives the change class per category. **Legacy = `NgrokTrafficPolicy`; canonical = `TrafficPolicy`.** For each row, the "R1 change" column names the minimum work to make R1 dual-kind-safe.

| # | Category | File(s) / marker | R0 state (post-`84c12924`) | R1 change | Cleanup |
|---|---|---|---|---|---|
| 1 | **Go type: struct + list + deepcopy** | `api/ngrok/v1alpha1/trafficpolicy_types.go`, `zz_generated.deepcopy.go:639-759` | `TrafficPolicy` + `TrafficPolicyList` only | **Add back** `NgrokTrafficPolicy` + `NgrokTrafficPolicyList` structs (regenerated deepcopy) with **identical schema** — the two structs are byte-compatible today (verified against `84c12924~1`), so the type reintroduction is a pure alias in shape. Mark both with `// LEGACY-trafficpolicy-kind` | Delete `NgrokTrafficPolicy*` types + regenerated deepcopy |
| 2 | **Scheme registration** | `api/ngrok/v1alpha1/groupversion_info.go:47-56` | Registers `TrafficPolicy{}` + `TrafficPolicyList{}` only | Register **both** pairs (`NgrokTrafficPolicy{}`, `NgrokTrafficPolicyList{}`, `TrafficPolicy{}`, `TrafficPolicyList{}`) so the client can decode either kind from watch/list responses | Remove `NgrokTrafficPolicy{}` registrations |
| 3 | **CRD manifest** | `helm/ngrok-crds/templates/ngrok.k8s.ngrok.com_trafficpolicies.yaml`, `manifest-bundle.yaml` | One CRD (`trafficpolicies.ngrok.k8s.ngrok.com`, kind `TrafficPolicy`) | **Add back** `ngroktrafficpolicies.ngrok.k8s.ngrok.com` (kind `NgrokTrafficPolicy`) as a second CRD with an identical schema block. Add a `deprecated: true` marker + `deprecationWarning` on the legacy CRD's v1alpha1 version so `kubectl apply -f ...NgrokTrafficPolicy...yaml` prints a warning | Delete the legacy CRD manifest **and** ship a `StorageVersionMigration` job or documented `kubectl` recipe to migrate any remaining `NgrokTrafficPolicy` objects |
| 4 | **Controller reconciler** | `internal/controller/ngrok/trafficpolicy_controller.go:50-113`, `cmd/api-manager.go:622-631` | One reconciler watches `TrafficPolicy` | Either (a) **add a thin legacy reconciler** that watches `NgrokTrafficPolicy` and shares the same conditions helpers, or (b) register the same reconciler struct twice with different `For(...)` types. Pick (a) for readability + easier deletion at cleanup | Delete legacy reconciler |
| 5 | **Conditions helpers** | `internal/controller/ngrok/trafficpolicy_conditions.go` | Generic over `TrafficPolicy` | Refactor to accept a small interface (`SetObservedGeneration`, `GetConditions`, `SetConditions`) so both kinds share it, or clone the helpers behind the sentinel | Delete legacy paths |
| 6 | **Store cache** | `internal/store/cachestores.go:54,81,149,204,260`, `internal/store/store.go:55,173-174` | Single `TrafficPolicyV1 cache.Store`; single `GetTrafficPolicyV1(name, ns)` | Two options: **(a) two separate caches** (`TrafficPolicyV1`, `NgrokTrafficPolicyV1`) with a lookup helper `GetTrafficPolicyPreferCanonical(name, ns)` that tries canonical first and falls back to legacy; **(b) unified cache keyed by `(kind, ns, name)`**. Recommend (a) — matches how the existing cache dispatches on Go type in the `Get`/`Add`/`Delete` switches | Delete legacy cache field + switch cases + `Store.Get…` fallback |
| 7 | **Endpoint indexer** | `internal/trafficpolicy/indexer.go:44-69` | Indexes endpoints by their referenced TrafficPolicy name | Extend `ReferenceIndexFunc` so an update to *either* kind re-enqueues endpoints whose `targetRef` points at that name. The endpoint's targetRef carries no kind today (see #10 below), so the index key stays `namespace/name` — both stores just push into the same index | No index-shape change; drop the legacy update path |
| 8 | **`trafficpolicy.Manager.resolveRef`** | `internal/trafficpolicy/manager.go:217-231` | Calls `store.GetTrafficPolicyV1(name, ns)` | Change to **canonical-wins with legacy fallback**: if canonical hits, use it; if only legacy hits, use it and emit a `DeprecatedKind` warning event on the endpoint via the recorder; if both hit, prefer canonical + still emit the warning (users have staged the migration) | Collapse to canonical-only |
| 9 | **Gateway API ExtensionRef** | `pkg/managerdriver/driver.go:1072-1097` | Hardcoded `case "TrafficPolicy":` | Add `case "NgrokTrafficPolicy":` alongside (same fallback logic as #8) and emit a deprecation event on the parent HTTPRoute/TCPRoute/TLSRoute | Delete the legacy case |
| 10 | **Cross-CRD reference field (`K8sObjectRef`)** | `api/ngrok/v1alpha1/agentendpoint_types.go:232-245`, `cloudendpoint_types.go:166-180` | `K8sObjectRef.Name` (no kind field) | **No change** in R1 — the reference is untyped-by-name and same-namespace, so the resolver in #8 handles the kind lookup transparently. This is why we don't need CRD-schema surgery on Agent/CloudEndpoint | No change |
| 11 | **User-facing annotation** | `internal/annotations/annotations.go:45,100-116` | `ngrok.com/traffic-policy` annotation names the CR (no kind) | **No change** in R1 — the annotation value is a name; the resolver (#8) handles kind lookup. Consider adding a companion `traffic-policy-kind` annotation *only if* users need to disambiguate a same-name collision across the two kinds; parking that decision until we observe demand | No change |
| 12 | **RBAC — API-manager role** | `helm/ngrok-operator/templates/api-manager/role.yaml:330-345` | `trafficpolicies` + `trafficpolicies/status` (get/list/watch, patch/update) | **Add** `ngroktrafficpolicies` + `ngroktrafficpolicies/status` with the same verbs | Delete the legacy rules |
| 13 | **RBAC — Agent role** | `helm/ngrok-operator/templates/agent/role.yaml:79-86` | `trafficpolicies` (get/list/watch) | **Add** `ngroktrafficpolicies` (get/list/watch) | Delete the legacy rules |
| 14 | **RBAC — Editor/Viewer aggregation** | `helm/ngrok-operator/templates/rbac/crd-access/trafficpolicy-editor.yaml`, `trafficpolicy-viewer.yaml` | Editor/viewer roles cover `trafficpolicies` only | **Add back** `ngroktrafficpolicy-editor.yaml` and `ngroktrafficpolicy-viewer.yaml` (still named for the legacy plural so the aggregate labels don't shift under downstream RBAC operators), OR extend the existing files with a second rule block for the legacy plural. Recommend the file-per-kind form — cleaner deletion at cleanup | Delete legacy files |
| 15 | **RBAC — helm snapshot test** | `helm/ngrok-operator/tests/rbac/crd-access_test.yaml`, `tests/rbac/__snapshot__/crd-access_test.yaml.snap` | Test lists only `trafficpolicy-editor.yaml` / `trafficpolicy-viewer.yaml`; **snapshot still contains legacy `ngroktrafficpolicy-editor-role` name** — this is a live test failure introduced by `84c12924` | List both files in the test; regenerate the snapshot so both roles appear | Regenerate snapshot to canonical-only |
| 16 | **Kubebuilder markers** | `internal/controller/ngrok/trafficpolicy_controller.go` (rbac markers) | Markers grant on `trafficpolicies` only | Add markers for `ngroktrafficpolicies` and `ngroktrafficpolicies/status` on the legacy reconciler (#4) so `make manifests` re-emits the rules under a sentinel-tagged block | Delete legacy markers |
| 17 | **Testutils** | `internal/testutils/k8s-resources.go:312-322` | `NewTestTrafficPolicy` returns canonical kind | **Add** `NewTestNgrokTrafficPolicy` and refactor tests that need to exercise the fallback to use it | Delete legacy helper |
| 18 | **Testdata YAMLs** | `pkg/managerdriver/testdata/translator/*.yaml` (9 files listed in the exploration report) | All use `kind: TrafficPolicy` post-rename | **No change to the existing fixtures** — they exercise the canonical path. Add **new** fixtures under `testdata/translator/legacy-kind/` that use `kind: NgrokTrafficPolicy` to exercise the fallback resolver in #8/#9 | Delete legacy fixtures |
| 19 | **PROJECT file** | `PROJECT:46` | Still lists `kind: NgrokTrafficPolicy` — this is a stale reference `84c12924` missed | Add a second entry so **both** kinds are declared in the kubebuilder scaffold metadata. Order canonical first | Remove the legacy entry |
| 20 | **CHANGELOG (operator + CRDs)** | `CHANGELOG.md`, `helm/ngrok-crds/CHANGELOG.md`, `helm/ngrok-operator/CHANGELOG.md` | R0 entry says the rename shipped as-is | Rewrite the R0 entry as **"passive rename: TrafficPolicy is canonical; NgrokTrafficPolicy is deprecated and still resolved for one release"** with a link to the migration guide | Cleanup CHANGELOG note that legacy support is dropped |
| 21 | **v1 migration guide** | `docs/v1-migration-guide.md:40,137-156,432-436` | Silent on the kind rename | Add a new section: "TrafficPolicy kind rename" with the `kubectl` recipe (see §"User migration mechanism" below) | No change (guide keeps the note for one release, then trimmed at 1.0) |
| 22 | **Developer guide (passivity-shims)** | `docs/developer-guide/passivity-shims.md` | Covers labels, annotations, finalizers, CR field renames, field-type changes | Add a new subsection **"CRD kind renames (`LEGACY-KIND-MIGRATION`)"** documenting the pattern this doc lands. Register the sentinel in the "Current sentinel tags" list (line 195) | No change |
| 23 | **Architecture doc** | `docs/developer-guide/architecture.md` | Post-rename references to `TrafficPolicy` | No change in R1 — canonical name is correct | No change |
| 24 | **CI / agents metadata** | `.github/agents/test-agent.agent.md:29`, `AGENTS.md:*` | Post-rename references | No change | No change |

**Files touched in R1 (estimate):** ~30 (types + deepcopy + scheme + 2 CRD manifests + manifest-bundle + reconciler + conditions + store + indexer + manager + driver + 4 RBAC files + snapshot + testutils + PROJECT + 3 CHANGELOGs + 2 docs + a handful of new testdata fixtures + reconciler test).

**Files touched at cleanup (later release):** the same list, in delete-mode. `git grep 'LEGACY-trafficpolicy-kind'` returns every one.

## What the R0 hard rename broke that R1 has to unbreak

Beyond the migration surface, the current post-`84c12924` state has two live problems that R1 should fix regardless of the passivity work:

1. **The helm snapshot test is red.** `tests/rbac/__snapshot__/crd-access_test.yaml.snap:374,406` still contains the legacy `ngroktrafficpolicy-editor-role` / `ngroktrafficpolicy-viewer-role` names, while `crd-access_test.yaml:15-16` was renamed to point at the new files. Either the snapshot wasn't regenerated, or it was regenerated but the update landed later. Verify with `helm unittest` before touching anything else.
2. **PROJECT is stale.** Line 46 still declares `kind: NgrokTrafficPolicy`. Kubebuilder scaffolding will now emit code for a resource whose Go type no longer exists.

Both are one-line fixes and belong in the R1 branch.

## Migration mechanism

### Kubernetes-side: two coexisting CRDs

Kubernetes has three primitives one might reach for to rename a kind. Only one applies:

| Option | Applies here? | Why |
|---|---|---|
| **Storage-version conversion webhook** | No | Conversion webhooks convert *versions* within a single `group/kind`. They cannot map two different kinds onto the same storage. |
| **`spec.names.shortNames` alias** | No | A short name is a CLI-side alias for `kubectl`; it does not create a second registered kind, and the CRD it points at can hold only one plural. |
| **Two CRDs sharing one group/version, different kinds/plurals** | **Yes** | Standard, boring approach. Both CRDs coexist; the operator watches both; users migrate their manifests by re-applying under the new kind. |

So R1 ships **two CRDs** — `ngroktrafficpolicies.ngrok.k8s.ngrok.com` (deprecated) and `trafficpolicies.ngrok.k8s.ngrok.com` (canonical). Both point at the same Go v1alpha1 API group, with distinct Go types whose specs are byte-compatible. The deprecated CRD carries a `deprecated: true` version marker so `kubectl apply` prints a warning.

Rollback: users on R1 who need to fall back to `pre-84c12924` are covered because `NgrokTrafficPolicy` is still the storage kind the older operator knows. Users on R1 who need to fall back to `84c12924` (which never shipped in a tagged release, so this is only relevant if the R0 build was consumed from a dev/main channel) are covered because their canonical `TrafficPolicy` objects still work — the R0 operator just doesn't know about legacy `NgrokTrafficPolicy` objects, which was true before.

### User migration mechanism

R1 does **not** copy legacy CRs into canonical CRs. That would create duplicate storage rows and diverge on status. Instead:

1. **The operator resolves legacy CRs transparently.** Any endpoint that names a TrafficPolicy `foo` will find it whether the object under that name is a `NgrokTrafficPolicy` or a `TrafficPolicy` (canonical-wins on same-name collision).
2. **The operator emits a `DeprecatedKind` warning event** on the endpoint whenever a `NgrokTrafficPolicy` is resolved. Users see the event in `kubectl describe endpoint`.
3. **The migration guide documents the recipe** users run to re-stamp their manifests:
   ```sh
   # Discover
   kubectl get ngroktrafficpolicies -A

   # Migrate one object (per-namespace loop is trivial)
   kubectl get ngroktrafficpolicy foo -n bar -o json \
     | jq '.kind = "TrafficPolicy" | .apiVersion = "ngrok.k8s.ngrok.com/v1alpha1" | del(.metadata.resourceVersion, .metadata.uid, .metadata.creationTimestamp, .metadata.generation, .status)' \
     | kubectl apply -f -

   kubectl delete ngroktrafficpolicy foo -n bar
   ```

The recipe is safe because the schemas are identical, the endpoint reference in R1 resolves both kinds, and the `delete` only runs after the `apply` succeeds. We publish this in the migration guide and in the `DeprecatedKind` event message.

For users with GitOps: the migration is a manifest edit (change the `kind` and re-apply). The old object gets garbage-collected when the operator no longer has the legacy CRD (cleanup release).

### The cleanup release

R-cleanup deletes the legacy CRD. Any `NgrokTrafficPolicy` objects still in etcd at that point become undecodable and are removed by the API server when the CRD goes. To make this survivable:

- The R1 CHANGELOG and migration guide loudly announce the R-cleanup date.
- R1 emits a **cluster-scoped** warning if any `NgrokTrafficPolicy` objects exist at operator startup (log line + K8s `Warning` event on the operator's own leader-election lease or the `KubernetesOperator` CR).
- **Do not** run an operator-driven backfill at R-cleanup startup that mutates user manifests. Discover-and-warn only. The recipe is user-run.

If we later decide we want zero user friction, we can ship a `StorageVersionMigration`-style Job that runs the `kubectl` recipe cluster-wide during R-cleanup upgrade. This is not required for R1 and adds a new operational primitive to the project; parking it.

### New sentinel: `LEGACY-trafficpolicy-kind`

Following the convention in `passivity-shims.md:139-201`, every legacy-only site above carries a `LEGACY-trafficpolicy-kind` sentinel. The tag is distinct from the existing `LEGACY-trafficpolicy-name` (which covers the `CloudEndpoint.spec.trafficPolicyName` → `spec.trafficPolicy.targetRef.name` migration) and `LEGACY-trafficpolicy-policy` (the nested `.policy` → `.inline` migration), so the three trafficpolicy cleanup sweeps stay independent. Add the entry to the current-sentinel-tags list in `passivity-shims.md:195-201` in the same PR as the pattern subsection.

## Interaction with the `ngrok.com/v1` group move

There is a parallel workstream to consolidate the three v1alpha1 API groups
(`ngrok.k8s.ngrok.com`, `ingress.k8s.ngrok.com`, `bindings.k8s.ngrok.com`)
into a single `ngrok.com/v1` group. For TrafficPolicy the target end-state
is:

```
ngrok.k8s.ngrok.com/v1alpha1 NgrokTrafficPolicy   (legacy)
                            ↓
ngrok.com/v1 TrafficPolicy                        (canonical, at 1.0)
```

That workstream is currently referenced in three places, and they disagree
on the mechanism:

| Source | Says |
|---|---|
| `docs/v1-migration-guide.md:516-520` | "A separate 1.0 workstream will consolidate these into `ngrok.com/v1` with a **conversion webhook**; that migration will appear here when it begins." |
| `specs/migration-v1.md:47-56` | "A **conversion webhook** handles in-place conversion from the old group/version combinations to `ngrok.com/v1`." |
| `docs/developer-guide/passivity-shims.md:625` | "The planned `ngrok.com/v1` group move is itself expected to be a **dual-read migration rather than a webhook**." |

**The webhook framing in the user-facing docs is wrong per Kubernetes
semantics.** A CRD conversion webhook is registered on a *single CRD*
(identified by `plural.group`) and converts between the versions declared
on that CRD. It cannot cross group boundaries. `ngrok.k8s.ngrok.com`
CRDs and `ngrok.com` CRDs are, by definition, separate CRDs — they are
not one CRD with two versions, they are two CRDs. So a conversion
webhook cannot move `NgrokTrafficPolicy@ngrok.k8s.ngrok.com/v1alpha1`
to `TrafficPolicy@ngrok.com/v1`. The `passivity-shims.md` note is the
correct call: the group move ends up as dual-CRD dual-read, same
mechanism as the kind rename in this doc.

**This is the first thing R1 should do: resolve the doc conflict** in
favor of dual-read and update `docs/v1-migration-guide.md` and
`specs/migration-v1.md`. Otherwise both migrations end up with
contradictory user-facing guidance.

Once we agree on dual-read, the sequencing choice for these two
migrations is the load-bearing decision.

### Three legitimate sequences

Let `TP` = `TrafficPolicy`, `NTP` = `NgrokTrafficPolicy`, `old` =
`ngrok.k8s.ngrok.com/v1alpha1`, `new` = `ngrok.com/v1`. The stable
end-state everyone agrees on is: `TP@new` only.

**Sequence A — kind rename first, group move second (independent migrations, series).**

| Release | Shapes operable | User does |
|---|---|---|
| Today (main, `84c12924`) | `TP@old` | (nothing shipped) |
| Kind R1 | `TP@old` + `NTP@old` (dual-read at v1alpha1) | Migrates `NTP@old` → `TP@old` |
| Kind cleanup | `TP@old` | — |
| Group R1 | `TP@old` + `TP@new` | Migrates `TP@old` → `TP@new` |
| Group cleanup | `TP@new` | — |

Users perform **two independent manifest migrations** separated in time.
The Kind cleanup and the Group R1 could be the same release, but they
don't have to be.

**Sequence B — do both migrations concurrently in one dual-migration release.**

| Release | Shapes operable | User does |
|---|---|---|
| Today | `TP@old` | — |
| Combined R1 | `TP@old` + `NTP@old` + `TP@new` (three shapes) | Migrates to `TP@new` directly |
| Combined cleanup | `TP@new` | — |

At R1 the operator has to serve **three shapes** for one policy concept
(dual-cache, dual-resolve, dual-RBAC, tripled). Complex to implement,
brittle to reason about, worse debugging surface. Cleanup is a single
release. Reject.

**Sequence C — revert the kind rename at v1alpha1; fold it into the group move.**

| Release | Shapes operable | User does |
|---|---|---|
| Today (main, `84c12924`) | `TP@old` | (nothing shipped; `84c12924` reverted before it leaves main) |
| Post-revert (v1alpha1 stable) | `NTP@old` | — |
| Group R1 | `NTP@old` + `TP@new` | Migrates `NTP@old` → `TP@new` |
| Group cleanup | `TP@new` | — |

At most **two shapes** operable at any time. The kind rename and the
group move happen in one user-visible migration. Matches the "v1 is
the big migration event" story already in the user-facing docs.

### Recommendation: Sequence C

**Revert `84c12924` and fold the kind rename into the `ngrok.com/v1` group move.**

Reasoning:

- **`84c12924` gives users no functional benefit at v1alpha1.** The
  rename is aesthetic — same schema, same fields, same operator
  behavior, just a shorter kind name. There is no feature that ships
  with the rename that couldn't ship without it.
- **Two dual-read migrations are more expensive than one.** Both
  migrations rebuild the same infrastructure (dual-cache in
  `internal/store`, dual-resolve in `internal/trafficpolicy/manager.go`,
  dual-Gateway-ExtensionRef, dual-RBAC, dual-editor/viewer roles).
  Sequence A pays that cost twice; Sequence C pays it once.
- **Users prefer one migration to two.** Sequence A tells users
  "migrate your CRD kind now, then in six months migrate the API
  group too". Sequence C tells them "migrate to `ngrok.com/v1
  TrafficPolicy` at 1.0" — one edit per manifest.
- **The `84c12924` PR body doesn't cite a shipping constraint.** The
  commit message reads as an idiomatic cleanup, not a
  release-blocking fix. Reverting costs a rebase; reverting *after*
  the kind R1 ships costs a passive migration release.
- **The end-state is identical.** `TP@new` under both sequences. The
  only difference is how many intermediate steps the user goes
  through to get there.

**What "revert" means concretely.** Preferred form is `git revert
84c12924` on `main` before it ships in a tagged release, then rebase
the outstanding branches. If any downstream branches have already
built on `84c12924`, follow up with an explicit "un-rename" commit
that restores the pre-`84c12924` state file-by-file (regeneration of
`zz_generated.deepcopy.go`, `manifest-bundle.yaml`, RBAC role names,
snapshot). The revert must include:

- Restore `api/ngrok/v1alpha1/ngroktrafficpolicy_types.go` (Go types).
- Restore `helm/ngrok-crds/templates/ngrok.k8s.ngrok.com_ngroktrafficpolicies.yaml` (CRD manifest).
- Restore RBAC files under legacy names (`ngroktrafficpolicy-editor.yaml`, `ngroktrafficpolicy-viewer.yaml`, plus the resource name change in `api-manager/role.yaml` and `agent/role.yaml`).
- Restore `internal/controller/ngrok/ngroktrafficpolicy_controller.go` (rename back).
- Restore the `NgrokTrafficPolicy` cache/store field and lookup method.
- Restore `PROJECT` scaffold entry.
- Restore the pre-`84c12924` test fixtures and testutil helpers.
- Restore CHANGELOG/AGENTS.md wording.
- Fix the helm RBAC snapshot (`tests/rbac/__snapshot__/crd-access_test.yaml.snap`) so it matches whichever state we land on (regenerate against the reverted state).

None of this is technically hard because `84c12924` is a single commit
and the reverse diff is mechanically the inverse of the forward diff.
The risk is downstream branches: if the ngrok-operator team has feature
branches already based on `84c12924`, they need a rebase-and-fixup pass.

### If Sequence C is rejected, what changes in the R1 plan for the kind rename?

If for policy reasons (already-announced deprecation, external commitment,
or a downstream dep) we cannot revert `84c12924`, then Sequence A applies
and the R1 plan in this doc stays largely as written, with three
adjustments:

1. **Sentinel name change.** Use `LEGACY-trafficpolicy-kind-v1alpha1` (not
   the shorter `LEGACY-trafficpolicy-kind`) so the group-move cleanup sweep
   at v1 gets its own tag and doesn't collide.
2. **Coordinate the kind cleanup release with the group R1 release.**
   If the kind cleanup ships *before* group R1, users have to be all-on
   `TP@old` at the moment group R1 arrives — that's fine and matches the
   Sequence A table above. If the kind cleanup ships *after* group R1,
   the operator briefly serves `NTP@old` alongside `TP@new`, which means
   the group-move dual-read has to know about three shapes for a
   window. Avoid this by making the kind cleanup a hard prerequisite of
   the group R1 (documented in the release notes, gated in the release
   checklist).
3. **The v1 conversion path must handle both v1alpha1 kinds.** When
   group R1 dual-reads `TP@new` + `TP@old`, it also has to fall through
   to `NTP@old` for the users still lagging on the kind migration. This
   is one extra `case` branch in the resolver, not a structural change,
   but it's a code smell that Sequence C would eliminate entirely.

### Doc conflict to fix regardless of sequence

Independent of the sequencing decision, the R1 branch (whichever
migration goes first) should include a commit that:

- Removes the "conversion webhook" language from `docs/v1-migration-guide.md:516-520`.
- Removes the "conversion webhook" language from `specs/migration-v1.md:47-56`.
- Replaces both with the dual-read description already correct in `docs/developer-guide/passivity-shims.md:625`.
- Adds a note in the migration guide explaining why a conversion webhook cannot serve a group rename.

## Open questions

- **Do we need a `traffic-policy-kind` annotation companion to `ngrok.com/traffic-policy`?** Only if a user could plausibly hold two objects with the same name but different kinds and need to select one explicitly. Recommend deferring; canonical-wins covers the natural case and the collision is transient during migration.
- **`webhook conversion`?** Not needed. The two kinds don't share storage; there is nothing to convert.
- **Do we want a `deprecated` warning on the CRD itself (server-side) or only from the operator (event-side)?** Both. The CRD-level `deprecated: true` covers users who apply manifests directly; the operator event covers users who don't run `kubectl apply` on the TrafficPolicy itself (e.g. GitOps'd from a chart) but still see reconcile events.
- **Do we ship the R1 branch as a new commit *on top of* `84c12924`, or rebase-away `84c12924`?** Recommend: **new commit(s) on top**. The rename half of `84c12924` is what we want to keep canonical; R1 is the "add legacy support back" delta. Cleaner history, cleaner cleanup PR later.

## Explicit non-goals for R1

- No automatic conversion of legacy → canonical objects (the "user migration mechanism" section above is deliberately manual).
- No changes to the TrafficPolicy schema itself. Both kinds share the same byte-compatible v1alpha1 spec.
- No changes to how AgentEndpoint / CloudEndpoint reference a TrafficPolicy. `K8sObjectRef` stays name-only; the resolver does the kind sniffing.
- No conversion webhook.
- No new user-facing annotation.

## Next step

Write the R1 implementation plan in the `docs/superpowers/plans/` cadence used by `2026-07-13-user-facing-prefix-migration-r1.md`, task-by-task, with each `LEGACY-trafficpolicy-kind` site called out by file+line and paired with a failing test. Land alongside a `docs/developer-guide/passivity-shims.md` update introducing the `LEGACY-KIND-MIGRATION` family.
