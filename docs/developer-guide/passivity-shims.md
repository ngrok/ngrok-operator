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

## Deferral for rollout races

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

### IngressClass `spec.controller` (rollout-race deferral)

- **Pattern:** Helm-rendered manifest deferred to cleanup release.
- **R1 (0.24):**
  - Operator binary: `internal/store/store.go::ListNgrokIngressClassesV1`
    dual-matches when `controllerName` equals the new default.
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
