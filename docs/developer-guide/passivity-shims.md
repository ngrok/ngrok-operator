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

## The two patterns

### Two-release pattern (most sites)

Suitable when the legacy key is non-load-bearing for object lifecycle — i.e.
the absence of the legacy key doesn't *block* something like a deletion or
trigger a destructive sync.

- **R1 (migration release):** read both prefixes; **dual-write** both
  prefixes; never delete the legacy key. The legacy key stays present on
  every object the operator writes.
- **R2 (cleanup release, immediately before 1.0):** drop legacy-read code;
  write the new prefix only; delete legacy keys from objects on next
  reconcile.

Rollback from R1 to the prior release works because the legacy key is
still on every object. Rollback from R2 to R1 works because R1 reads the
new prefix the rolled-back code wrote.

Used for: operator-written controller labels on `AgentEndpoint` /
`CloudEndpoint`, the `ngrok.com/computed-url` annotation on Services, and
the bindings labels on Services owned by `BoundEndpoint`.

### Three-release pattern (finalizer-style cases)

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

### Deferral for rollout races

Some changes are safe to ship in the operator binary but unsafe to ship in
the rendered helm chart at the same time, because the rendered manifest
takes effect mid-upgrade while the old operator pod is still running.
The IngressClass `spec.controller` flip is the only example so far. The
operator binary gains dual-match in R1; the rendered manifest stays on the
legacy value until R2.

## The `LEGACY-PREFIX-MIGRATION` sentinel

Every code site that exists *only* to support the legacy prefix during a
migration window carries the marker `LEGACY-PREFIX-MIGRATION`. Two forms:

```go
// LEGACY-PREFIX-MIGRATION: BEGIN
// ... block to delete ...
// LEGACY-PREFIX-MIGRATION: END

someLegacyCall(...) // LEGACY-PREFIX-MIGRATION: drop in 1.0
```

In the cleanup release for each migration, run:

```sh
git grep 'LEGACY-PREFIX-MIGRATION'
```

For each hit, delete the block between `BEGIN` / `END` or delete the
marked line. The sentinel exists so cleanup is a single, auditable sweep
rather than archaeology.

## Per-shim catalog: `k8s.ngrok.com/` → `ngrok.com/` migration

Each entry below describes one passivity shim, which pattern it follows,
the code involved, and the cleanup step.

### User-facing annotations (read-side compatibility)

- **Pattern:** Two-release. Read-side only — these are user-set keys, so
  no operator writes are involved.
- **R1 (0.24):** `internal/annotations/parser/parser.go` exposes
  `Get*AnnotationWithFallback` helpers; `internal/annotations/annotations.go`
  `Extract*` functions read both prefixes; controller reconcile paths call
  `deprecation.ScanAnnotations` to emit Warning events.
- **R2 cleanup:** delete the `*WithFallback` family, the
  `internal/deprecation` package, and the `ScanAnnotations` call sites.

### Gateway TLS option keys (read-side compatibility)

- **Pattern:** Two-release. Read-side only.
- **R1 (0.24):** `pkg/managerdriver/translate_gatewayapi.go` reads
  `ngrok.com/terminate-tls.*` first, falls back to
  `k8s.ngrok.com/terminate-tls.*`. Gateway controller emits a single
  `LegacyAnnotation` event per reconcile when legacy keys are present.
- **R2 cleanup:** delete the legacy fallback branch and the
  `warnIfLegacyTLSOptions` helper.

### Controller labels on AgentEndpoint / CloudEndpoint (operator-written)

- **Pattern:** Two-release dual-write.
- **R1 (0.24):** `internal/controller/labels/controller.go`:
  - `ControllerLabels(...)` returns a map with **both** the new and legacy
    label pairs.
  - `EnsureControllerLabels(...)` writes both pairs and **does not**
    delete `LegacyControllerName` / `LegacyControllerNamespace`.
  - `HasControllerLabels(...)` matches either pair (already implemented).
  - `ControllerLabelSelectors(...)` returns both selectors so List queries
    find legacy-labeled objects (already implemented).
- **R2 cleanup:** in `controller.go`, drop the legacy const block, the
  legacy match branch in `HasControllerLabels`, `LegacyControllerLabelSelector`,
  and the legacy entries in `ControllerLabels`. Delete
  `pkg/managerdriver/controller_label_list.go` and re-inline a single
  `c.List(ctx, &out, d.controllerLabels.Selector())` call in
  `driver.go::Sync` and `endpoints.go::SyncEndpoints`.

### `ngrok.com/computed-url` annotation on Services (operator-written)

- **Pattern:** Two-release dual-write.
- **R1 (0.24):** `internal/controller/service/controller.go`:
  - `setComputedURLAnnotation` writes both keys; does **not** delete the
    legacy key (drop the `delete(a, annotations.LegacyComputedURLAnnotation)`
    line).
  - `clearComputedURLAnnotation` keeps deleting both keys (aggressive
    deletes are fine; only writes need dual-write).
  - Reader `ExtractComputedURL` prefers new and falls back to legacy
    (already implemented).
- **R2 cleanup:** drop the legacy write, the `Legacy*` const, the
  fallback read branch, and the legacy-delete in `clearComputedURLAnnotation`.

### Bindings labels on Services owned by BoundEndpoint (operator-written)

- **Pattern:** Two-release dual-write.
- **R1 (0.24):** `internal/controller/bindings/boundendpoint_controller.go`:
  - Declare `LegacyLabelEndpointURL` alongside the existing
    `LegacyLabelBoundEndpoint{Name,Namespace}` consts.
  - In `convertBoundEndpointToServices`, include legacy keys in
    `thisBindingLabels` and `upstreamAnnotations`.
  - In the update branches at `:287-289` and `:317-319`, merge labels
    instead of replacing the label map outright — or include the legacy
    keys in the desired set so the replace is non-destructive.
  - Readers (`boundEndpointLabelsFor`) already dual-read.
- **R2 cleanup:** drop the `Legacy*` consts, the legacy-fallback branch in
  `boundEndpointLabelsFor`, and the legacy writes.

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

### IngressClass `spec.controller` (rollout-race deferral)

- **Pattern:** Helm-rendered manifest deferred to cleanup release.
- **R1 (0.24):**
  - Operator binary: `internal/store/store.go::ListNgrokIngressClassesV1`
    dual-matches when `controllerName` equals the new default (already
    implemented).
  - CLI flag default in `cmd/api-manager.go` flips to the new prefix
    (already implemented).
  - Helm chart **stays on legacy**: `helm/ngrok-operator/values.yaml`
    `ingress.controllerName` remains `k8s.ngrok.com/ingress-controller`;
    `values.schema.json` default matches; `README.md` table matches;
    `tests/__snapshot__/ingress-class_test.yaml.snap` shows the legacy
    controller. The helm `CHANGELOG.md` notes that the *default will
    change in 0.25*, not that it does now.
- **R2 (0.25):** flip the helm-rendered IngressClass to the new prefix.
  At this point no pre-migration operator pod can observe the change.
- **R3 cleanup:** drop the dual-match branch in `store.go`.

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
