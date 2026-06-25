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

## The default pattern: three-release

This is the default for any migration that touches state on K8s objects
(labels, annotations, spec/status fields, CR fields). Three releases are
required because two constraints interact: rollback safety needs the new
operator to leave the legacy key readable for the prior version, and the
delete-on-reconcile migration of existing objects needs an in-flight
release where the operator can still *see* legacy-only objects.

- **R1 (migration release):** read both prefixes; **dual-write** both
  prefixes; never delete the legacy key. The legacy key stays present on
  every object the operator writes.
- **R2 (write-side cleanup):** write the new prefix only; delete the
  legacy key from objects on next reconcile; **keep** dual-read so that
  objects which have not yet been reconciled remain visible.
- **R3 (read-side cleanup):** drop dual-read and every `Legacy*` symbol.
  Safe because R2 had a full release window to delete-on-reconcile every
  reachable object.

The roles map to releases as follows. Only R1 is firm; the later numbers
may still change, so the code and the rest of this guide refer to the
roles by name rather than by version:

| Role | What it does                                                     | Release          |
| ---- | ---------------------------------------------------------------- | ---------------- |
| R1   | read both, write both, never delete legacy                       | 0.24             |
| R2   | write new only, delete legacy on reconcile, keep dual-read       | 1.0 (planned)    |
| R3   | drop dual-read and all `Legacy*` symbols                         | 1.1 (planned)    |

Rollback from R1 to the prior release works because the legacy key is
still on every object the operator wrote. Rollback from R2 to R1 works
because R1 reads both prefixes *and* dual-writes, so any R2-stamped
object gets re-stamped with the legacy key on next reconcile. Rollback
from R3 to R2 works because R2 still reads both prefixes.

Used for: operator-written controller labels on `AgentEndpoint` /
`CloudEndpoint`, the `ngrok.com/computed-url` annotation on Services, and
the bindings labels on Services owned by `BoundEndpoint`.

## When two releases is enough

R3 exists to migrate legacy-only objects safely. You can skip it —
collapsing R2 and R3 into a single cleanup release — when *either* of the
following conditions holds:

- **The migration touches no K8s object state.** Function signatures,
  in-memory data, internal RPCs, CRD storage-version conversions with a
  webhook. No reconcile churn, no external watchers reading the legacy
  shape. The `endpointURI` → `endpointURL` rename in #779 is close to
  this: the dual-read lives in `BoundEndpoint.Spec.GetEndpointURL()`, so
  once every stored object has been migrated through the API, the helper
  can collapse without a separate read-side release.

- **An operator-driven backfill guarantees 100% coverage before R2
  ships.** An init job or startup pass that lists every affected object
  and rewrites it under the new prefix. We don't have this pattern in
  the repo today; adopting it trades "one more release in the pipeline"
  for "a backfill that has to be defensive against every shape of
  legacy state, including objects that appear during the backfill."

For the migrations currently in flight, neither applies. Stick with the
three-release default.

## Three-release pattern (finalizer-style cases)

A different three-release shape is required when the legacy key gates
object lifecycle. Finalizers are the canonical case: Kubernetes will not
let an object delete until *every* finalizer is removed, and an older
operator only knows how to remove the finalizer key it knew about.
Dual-writing both finalizers is **worse** than single-writing — it just
guarantees the old operator can't drive a deletion to completion. So
unlike the default pattern above, R1 here single-writes the *legacy* key.

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

## The `LEGACY-PREFIX-MIGRATION` sentinel

Every code site that exists *only* to support the legacy prefix during a
migration window carries the marker `LEGACY-PREFIX-MIGRATION`. Two forms:

```go
// LEGACY-PREFIX-MIGRATION: BEGIN
// ... block to delete ...
// LEGACY-PREFIX-MIGRATION: END

someLegacyCall(...) // LEGACY-PREFIX-MIGRATION (read-side cleanup): drop the legacy read
```

In the cleanup releases for each migration, run:

```sh
git grep 'LEGACY-PREFIX-MIGRATION'
```

For each hit, delete the block between `BEGIN` / `END` or delete the
marked line.

Markers say what *kind* of cleanup they are: a `(write-side cleanup)`
marker stops dual-writing the legacy key, a `(read-side cleanup)` marker
stops reading it. That distinction is the load-bearing part — write-side
cleanup must ship a release before read-side cleanup, or a rollback to the
previous release can no longer find legacy-stamped objects. This guide
prefers the cleanup-kind label over a release number in the marker text,
since the target version may still change; a few earlier markers (the
finalizer and IngressClass shims) instead embed a specific release like
`drop ... in 1.0`. Either form is a valid `git grep` target. The sentinel
exists so each cleanup release is a single, auditable sweep rather than
archaeology.

## Per-shim catalog: `k8s.ngrok.com/` → `ngrok.com/` migration

Each entry below describes one passivity shim, which release does what,
and the precise code touched at each step.

### Controller labels on AgentEndpoint / CloudEndpoint (operator-written)

- **R1 — migration release:** `internal/controller/labels/controller.go`:
  - `ControllerLabels(...)` returns a map with **both** the new and legacy
    label pairs.
  - `EnsureControllerLabels(...)` writes both pairs and **does not**
    delete `LegacyControllerName` / `LegacyControllerNamespace`.
  - `HasControllerLabels(...)` matches either pair.
  - `ControllerLabelSelectors(...)` returns both selectors so List queries
    find legacy-labeled objects.
  - `internal/domain/manager.go::ensureControllerLabels` short-circuits
    only when the operator would not change any label (probed via a
    clone-and-`EnsureLabels` no-op check), so the legacy pair gets
    backfilled on every object during the migration window.
- **R2 — write-side cleanup:**
  - `ControllerLabels(...)`: drop the legacy entries from the returned
    map; collapses back to the new pair only.
  - `EnsureControllerLabels(...)`: replace the legacy ensure-set with
    `delete(l, LegacyControllerNamespace)` / `delete(l, LegacyControllerName)`
    so existing objects shed the legacy pair on next reconcile.
  - `domain.ensureControllerLabels`: no change needed — because it probes
    by running `EnsureLabels` on a clone, it automatically tracks whatever
    `EnsureLabels` writes once the legacy ensure-set is dropped.
  - **Keep** `HasControllerLabels` dual-match and
    `ControllerLabelSelectors` dual-selectors so R2 can still find and
    migrate legacy-only objects.
- **R3 — read-side cleanup:** drop the legacy const block, the
  legacy match branch in `HasControllerLabels`,
  `LegacyControllerLabelSelector`, and `ControllerLabelSelectors`.
  Delete `pkg/managerdriver/controller_label_list.go` and re-inline a
  single `c.List(ctx, &out, d.controllerLabels.Selector())` call in
  `driver.go::Sync` and `endpoints.go::SyncEndpoints`.

### `ngrok.com/computed-url` annotation on Services (operator-written)

- **R1 — migration release:** `internal/controller/service/controller.go`:
  - `setComputedURLAnnotation` writes both keys and does **not** delete
    the legacy key.
  - `clearComputedURLAnnotation` deletes both keys (aggressive deletes
    are fine; only writes need dual-write).
  - Reader `ExtractComputedURL` (in
    `internal/annotations/annotations.go`) prefers the new key and falls
    back to legacy.
  - **Known interleaving (self-healing, no outage path):** R1's
    `clearComputedURLAnnotation` deletes *both* keys, but a pre-migration
    operator's clear deletes only the legacy `k8s.ngrok.com/computed-url`.
    A narrow rollback sequence — downgrade to the old operator, the user
    switches a Service away from `tcp://`, the old operator clears only the
    legacy key, then roll forward — leaves a stale new-key
    `ngrok.com/computed-url` behind. This is **not** the finalizer-class
    hazard: leader election (on by default, `api-manager.go`) means there is
    never a second concurrent writer on the same Service, and the TCP branch
    is gated on the *listener URL* derived from the user's `url`/`domain`
    annotation (`controller.go`, `if listenerEndpointURL == "tcp://"`), not on
    the stored annotation. A Service that is no longer TCP never reaches the
    `ExtractComputedURL` read; it falls to the non-TCP branch that re-stamps
    both keys via `setComputedURLAnnotation`, so the stale value is overwritten
    on the next reconcile. No code change —
    documented because it is the one place R1's "never delete legacy on the
    write path" property does not extend across a version boundary.
- **R2 — write-side cleanup:**
  - `setComputedURLAnnotation`: drop the legacy write and the legacy
    comparison; add
    `delete(a, annotations.LegacyComputedURLAnnotation)` so existing
    Services shed the legacy key on next reconcile.
  - **Keep** the legacy fallback read in `ExtractComputedURL`.
  - **Keep** the legacy delete in `clearComputedURLAnnotation` (no harm).
- **R3 — read-side cleanup:** drop the
  `LegacyComputedURLAnnotation` const, the legacy fallback branch in
  `ExtractComputedURL`, and the legacy delete in
  `clearComputedURLAnnotation`.

### Bindings labels on Services owned by BoundEndpoint (operator-written)

- **R1 — migration release:** `internal/controller/bindings/boundendpoint_controller.go`:
  - `LegacyLabelEndpointURL` declared alongside the existing
    `LegacyLabelBoundEndpoint{Name,Namespace}` consts.
  - `convertBoundEndpointToServices` dual-writes both label pairs in
    `thisBindingLabels` and the legacy `endpoint-url` annotation in
    `upstreamAnnotations`.
  - `boundEndpointLabelsFor` reads either prefix.
- **R2 — write-side cleanup:**
  - `convertBoundEndpointToServices`: drop the legacy label entries from
    `thisBindingLabels` and the legacy `endpoint-url` from
    `upstreamAnnotations`. The Service update overwrites labels, so
    existing legacy keys disappear automatically on next reconcile.
  - **Keep** the legacy branch in `boundEndpointLabelsFor` so the
    BoundEndpoint owner index can still find Services that haven't been
    reconciled yet.
- **R3 — read-side cleanup:** drop the three `Legacy*` consts and
  the legacy lookup in `boundEndpointLabelsFor`.

### Operator finalizer (operator-written, lifecycle-gating)

- **Pattern:** Three-release dance (finalizer-style; see above).
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

#### Why we don't shortcut the finalizer

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
