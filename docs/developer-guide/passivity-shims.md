# Passivity shims and migration strategy

This document is for ngrok-operator maintainers. It describes how we stage
backwards-incompatible changes across multiple releases using **passivity
shims** — small pieces of read-side and/or write-side compatibility code
that let an older operator coexist with a newer one during rolling
upgrades and (where possible) survive a `helm rollback`. The user-facing
counterpart that lists what each release means for users is
[`docs/v1-migration-guide.md`](../v1-migration-guide.md).

## Why we need shims

A `helm upgrade` (and a rolling `kubectl apply`) does not atomically swap
the operator. For a window of seconds to minutes:

1. The new manifest (with a new IngressClass `spec.controller`, new label
   selectors expected on AEPs/CEPs, etc.) has been applied.
2. The **old** operator pod is still running, watching, and reconciling.
3. The new operator pod is starting up.

During that window the old operator can interpret newly-written objects in
ways that destroy resources or stall finalizers, unless we constrain *what
the new operator writes* during the migration release.

Rollbacks are worse: a `helm rollback` returns the cluster to the prior
operator image but leaves objects in whatever state the newer operator
stamped them. Anything the newer operator wrote that the older release
doesn't understand becomes a hazard.

## Two-release pattern (deprecated-field-style cases)

Used when the legacy form does not gate lifecycle and the old operator can
keep operating on objects that still carry the legacy field. CRD field
deprecations fall here.

- **R1 (migration release):** the CRD CEL relaxes to accept both legacy
  and canonical fields together; the controller dual-reads both shapes;
  when both are set the canonical field wins and the legacy field is
  ignored with a `DeprecatedField` warning event. Rollback to the prior
  release is safe because objects carrying only the legacy field still
  resolve in the old operator.
- **R-cleanup (later release):** legacy field removed from the CRD,
  controller normalization removed, `LEGACY-*` sentinels deleted.

## Three-release pattern (finalizer-style cases)

Required when the legacy key gates object lifecycle. Finalizers are the
canonical case: Kubernetes will not let an object delete until *every*
finalizer is removed, and an older operator only knows how to remove the
finalizer key it knew about. Dual-writing both finalizers is **worse** than
single-writing — it just guarantees the old operator can't drive a
deletion to completion.

- **R1 (migration release):** read both prefixes; `Add` writes the
  **legacy** key only (no write-side change from the prior release); the
  `Remove` path removes both keys. R1 is rollback-safe to the prior
  release (no new-prefix keys exist yet) and forward-safe to R2 (R2 finds
  objects already carrying the legacy key it knows how to remove).
- **R2 (next release):** read both prefixes; `Add` writes the **new** key
  and removes the legacy. `Remove` removes both. Rollback to R1 is safe
  because R1 knows how to remove the new key.
- **R3 (cleanup release):** read and write the new key only.

Used for: the operator finalizer (`ngrok.com/finalizer`).

## Deferral for rollout races

Some changes are safe to ship in the operator binary but unsafe to ship in
the rendered helm chart at the same time, because the rendered manifest
takes effect mid-upgrade while the old operator pod is still running.
The IngressClass `spec.controller` flip is the only example so far. The
operator binary gains dual-match in R1; the rendered manifest stays on the
legacy value until R2.

## `LEGACY-*` sentinels

Every code site that exists *only* to support a legacy form during a
migration window carries a `LEGACY-<short-tag>` marker. The tag identifies
the migration so each cleanup is an independent sweep. Forms:

```go
// LEGACY-trafficpolicy-name: BEGIN
// ... block to delete ...
// LEGACY-trafficpolicy-name: END

someLegacyCall(...) // LEGACY-trafficpolicy-name: drop in cleanup release
```

In the cleanup release for each migration, run:

```sh
git grep '// LEGACY-'
```

…then narrow by tag (e.g. `git grep 'LEGACY-trafficpolicy-name'`) and
delete the marked blocks / lines for that specific migration.

Current sentinel tags:

- `LEGACY-PREFIX-MIGRATION` — `k8s.ngrok.com/` → `ngrok.com/` prefix renames.
- `LEGACY-trafficpolicy-name` — `CloudEndpoint.spec.trafficPolicyName` → `spec.trafficPolicy.targetRef.name`.
- `LEGACY-trafficpolicy-policy` — `CloudEndpoint.spec.trafficPolicy.policy` → `spec.trafficPolicy.inline`.

## Per-shim catalog

### Operator finalizer (operator-written, lifecycle-gating)

- **Pattern:** Three-release dance.
- **R1 (0.24):** `internal/util/k8s.go`:
  - `HasFinalizer` checks both (already implemented).
  - `AddFinalizer` adds `LegacyFinalizerName` only; **does not** add
    `FinalizerName` and does **not** remove `LegacyFinalizerName`.
  - `RemoveFinalizer` removes both (already implemented).
  - Update the doc comments on `AddFinalizer` and on the package to make
    clear this is R1 of the three-release pattern.
- **R2 (0.25):** `AddFinalizer` switches to adding `FinalizerName` and
  removing `LegacyFinalizerName`. `HasFinalizer` and `RemoveFinalizer`
  unchanged (still bridge both).
- **R3 cleanup (0.26):** delete `LegacyFinalizerName`, the legacy branches
  in `HasFinalizer`, and the legacy `RemoveFinalizer` call.

### `CloudEndpoint.spec.trafficPolicyName` → `spec.trafficPolicy.targetRef.name`

- **Pattern:** Two-release (deprecated field). Tag: `LEGACY-trafficpolicy-name`.
- **R1 (0.24):**
  - CRD: the CloudEndpoint schema never carried a spec-level CEL rule
    rejecting `trafficPolicyName` + `trafficPolicy`; the R1 CRD stays
    permissive, so the two can coexist at admission during a staged
    rollout or a rollback that resurrects an older manifest. What
    changes in R1 is the **controller**: the previous runtime rejection
    of the coexistence (`ErrInvalidTrafficPolicyConfig`) is relaxed so
    the fields can be set together. Note the 0.23 controller still
    rejects the coexistence at runtime, so **dual-setting top-level
    fields is not rollback-safe**. Users keep `trafficPolicyName`
    alone during the migration window; the controller normalizes
    legacy-only manifests in-memory.
  - `cloudendpoint_controller.go::normalizeLegacyTrafficPolicy`: when an
    effective `spec.trafficPolicy` is set alongside `trafficPolicyName`,
    emit a `DeprecatedField` warning event and use `spec.trafficPolicy`.
    An empty struct (`trafficPolicy: {}`) is **not** treated as
    effective, so a templating system that emits `{}` does not silently
    detach the legacy attachment. When only `trafficPolicyName` is set
    (or `trafficPolicy` is empty), normalize in-memory to
    `spec.trafficPolicy.targetRef`.
  - Deprecation events are suppressed for operator-managed
    CloudEndpoints — those carrying either a controller OwnerReference
    (Service path) or the operator's controller label
    (`k8s.ngrok.com/controller-name`, managerdriver Ingress/Gateway
    path) — because the user can't act on them and we'd otherwise
    spam events every reconcile.
  - `indexCloudEndpointTrafficPolicyRefs` falls back to the legacy
    name field only when `spec.trafficPolicy` carries no effective
    policy (no inline, no targetRef, no nested `policy`). When the
    canonical field is effective — including inline-only — the
    legacy field is not indexed, so updates to a TrafficPolicy that
    matches `trafficPolicyName` cannot stale-requeue an endpoint
    whose canonical field has already won.
- **R-cleanup:** delete the `TrafficPolicyName` field from
  `CloudEndpointSpec`, drop the legacy branch in `normalizeLegacyTrafficPolicy`
  and the legacy key emission in `indexCloudEndpointTrafficPolicyRefs`.

### `CloudEndpoint.spec.trafficPolicy.policy` → `spec.trafficPolicy.inline`

- **Pattern:** Two-release (deprecated nested field). Tag: `LEGACY-trafficpolicy-policy`.
- **R1 (0.24):**
  - CRD: union CEL on `CloudEndpointTrafficPolicyCfg` relaxed from
    "exactly one of inline/targetRef/policy" to "at most one of
    inline/targetRef" so `policy` may coexist with either canonical
    field. (`inline + targetRef` is still rejected — those are both
    canonical and ambiguous.)
  - `CloudEndpointTrafficPolicyCfg.ToTrafficPolicyCfg` folds `policy`
    into `inline` only when neither canonical field is set.
  - When `policy` is set alongside `inline` or `targetRef`, the controller
    emits a `DeprecatedField` warning event noting `policy` is ignored.
    The wording is differentiated for canonical=`inline` vs
    canonical=`targetRef` so users get the right replacement field
    in the message.
  - **Operator-generated CloudEndpoints dual-write `policy + inline`.**
    The 0.23 CRD prunes the unknown `inline` field but preserves
    `policy`, so dual-writing keeps generated objects rollback-safe.
    The new controller prefers `inline`. The deprecation event is
    suppressed for these objects because they are operator-managed
    (controller OwnerReference for the Service path, operator
    controller label for the managerdriver Ingress/Gateway path).
- **R-cleanup:** delete the `Policy` field, `HasDeprecatedPolicy`, the
  fallback branch in `ToTrafficPolicyCfg`, the deprecation event in
  the controller, and the dual-write in
  `pkg/managerdriver/translator.go` and
  `internal/controller/service/controller.go`.

### IngressClass `spec.controller` (rollout-race deferral)

- **Pattern:** Helm-rendered manifest deferred to cleanup release.
- **R1 (0.24):**
  - Operator binary: `internal/store/store.go::ListNgrokIngressClassesV1`
    dual-matches whenever `controllerName` equals either stock default
    (legacy `k8s.ngrok.com/ingress-controller` or new
    `ngrok.com/ingress-controller`). Custom controller names retain
    exact-match for multi-instance isolation. The Go code cannot
    distinguish "default" from "explicitly set to the default value",
    so both stock defaults are treated symmetrically; nobody sets the
    legacy default explicitly to mean "exact-match legacy only".
  - CLI flag default in `cmd/api-manager.go` flips to the new prefix.
  - Helm chart **stays on legacy**: `helm/ngrok-operator/values.yaml`
    `ingress.controllerName` remains `k8s.ngrok.com/ingress-controller`;
    `values.schema.json` default matches; `README.md` table matches;
    `tests/__snapshot__/ingress-class_test.yaml.snap` shows the legacy
    controller. The helm `CHANGELOG.md` notes that the *default will
    change in 0.25*, not that it does now.
- **R2 (0.25):** flip the helm-rendered IngressClass to the new prefix.
  At this point no pre-migration operator pod can observe the change.
- **R3 cleanup:** drop the dual-match branch in `store.go`.

## Why we don't shortcut the CloudEndpoint trafficpolicy migration

A few alternatives were considered and rejected for the two `CloudEndpoint`
trafficpolicy field renames:

- **Reject the legacy field at admission once the canonical shape exists**
  (the original CEL on `CloudEndpointSpec` did this). Rejected because it
  is not passive: a user who migrates their manifest to the canonical
  field and then needs to roll back hits admission rejection from the
  prior release's controller — or, worse, the legacy R0 operator reads
  no policy at all from the new-shape manifest. R1 must accept the
  legacy + canonical combination so the legacy field can stay as a
  rollback fallback.
- **Three-release dance like the finalizer migration.** Rejected because
  CloudEndpoint traffic policy attachment does not gate object lifecycle.
  The worst rollback consequence here is a missing policy attachment for
  one reconcile, which a user can recover from by re-adding the legacy
  field; with finalizers, the worst consequence is an object stuck in
  `Terminating` forever. A two-release pattern is sufficient.
- **Skip R1 entirely and remove the legacy fields in 0.24.** Rejected
  because it gives users no rollback-safe migration window and no
  deprecation signal in their reconcile events.
- **Force-normalize the legacy field by writing back the canonical
  shape to the API.** Rejected because it would mutate user manifests
  silently and leave the user's working copy diverged from the cluster
  state. We normalize in-memory only.

If you find yourself adding another `CloudEndpoint` field rename, follow
this two-release pattern: relax CEL to accept coexistence, dual-read,
emit `DeprecatedField` events, normalize in-memory only, sentinel-tag
every legacy-only code path.

## Why we don't shortcut the finalizer

A few alternatives were considered and rejected:

- **Keep the legacy finalizer forever.** Lowest-risk option; the operator
  finalizer name is internal and users don't select on it. Rejected
  because the prefix unification is being done specifically to make all
  ngrok-owned keys consistent ahead of 1.0.
- **Two-release dual-write of both finalizers.** Forward-safe but
  rollback-broken: the older operator only knows how to strip the legacy
  finalizer, so the new finalizer would block deletion of any object that
  reached `Terminating` after rollback. The finalizer **must** be
  single-written at any given time — only the *identity* of the
  single-written key changes between R1 and R2.
- **Skip R1 entirely (flip writes in 0.24).** Equivalent to the current
  PR's first cut, and the reason this strategy exists at all.

If you find yourself adding a new finalizer rename, follow the
three-release pattern above; there is no two-release shortcut that
preserves rollback safety.
