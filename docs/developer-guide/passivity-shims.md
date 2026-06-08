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
