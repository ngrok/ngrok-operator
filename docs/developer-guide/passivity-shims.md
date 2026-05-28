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
