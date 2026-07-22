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

## `LEGACY-*` sentinels

Every code site that exists *only* to support a legacy form during a
migration window carries a `LEGACY-<short-tag>` marker. The tag identifies
the migration so each cleanup is an independent sweep. Forms:

```go
// LEGACY-PREFIX-MIGRATION: BEGIN
// ... block to delete ...
// LEGACY-PREFIX-MIGRATION: END

someLegacyCall(...) // LEGACY-PREFIX-MIGRATION (read-side cleanup): drop the legacy read
```

In the cleanup releases for each migration, run:

```sh
git grep '// LEGACY-'
```

then narrow by tag (e.g. `git grep 'LEGACY-trafficpolicy-name'`). For each
hit, delete the block between `BEGIN` / `END` or delete the marked line.

Markers say what *kind* of cleanup they are: a `(write-side cleanup)`
marker stops dual-writing the legacy key, a `(read-side cleanup)` marker
stops reading it. That distinction is the load-bearing part — write-side
cleanup must ship a release before read-side cleanup, or a rollback to the
previous release can no longer find legacy-stamped objects. This guide
prefers the cleanup-kind label over a release number in the marker text,
since the target version may still change; a few earlier markers (the
finalizer and IngressClass shims) instead embed a specific release like
`drop ... in 1.0`, and the `LEGACY-trafficpolicy-*` tags embed `drop in
cleanup release`. Either form is a valid `git grep` target. The sentinel
exists so each cleanup release is a single, auditable sweep rather than
archaeology.

## The `LEGACY-FIELD-MIGRATION` sentinel

CRD **field renames** (a `json:` tag changing name) are a different
migration from the prefix unification, so they carry their own marker,
`LEGACY-FIELD-MIGRATION`, using the same `BEGIN` / `END` block and
`(read-side cleanup)` / `(write-side cleanup)` line forms as above. Keeping
the two families separate means the prefix-migration cleanup sweep
(`git grep 'LEGACY-PREFIX-MIGRATION'`) and the field-rename cleanup sweep
(`git grep 'LEGACY-FIELD-MIGRATION'`) stay independent — they ship on
different release cadences.

One CRD-specific gotcha: the doc comment immediately above a struct field
becomes that field's description in the generated CRD (`kubectl explain`).
A `LEGACY-FIELD-MIGRATION` marker placed there would leak implementation
detail into user-facing API docs, so separate the marker from the field's
doc comment with a blank line — controller-gen only reads the contiguous
comment block directly above the field.

Current sentinel tags:

- `LEGACY-PREFIX-MIGRATION` — `k8s.ngrok.com/` → `ngrok.com/` prefix renames.
- `LEGACY-FIELD-MIGRATION` — CRD field (`json:` tag) renames, e.g.
  `Domain.spec.resolves_to` → `resolvesTo`.
- `LEGACY-trafficpolicy-name` — `CloudEndpoint.spec.trafficPolicyName` → `spec.trafficPolicy.targetRef.name`.
- `LEGACY-trafficpolicy-policy` — `CloudEndpoint.spec.trafficPolicy.policy` → `spec.trafficPolicy.inline`.

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

## Per-shim catalog: CloudEndpoint traffic policy field renames

Each entry below describes one passivity shim, which release does what,
and the precise code touched at each step.

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

#### Why we don't shortcut the CloudEndpoint trafficpolicy migration

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
  The worst rollback consequence here is a detached policy: a canonical-only
  object that is rolled back has its `inline`/`targetRef` pruned by the API
  server, so the prior-release controller sees no policy. In practice this
  surfaces as a persistent failing reconcile (`CloudEndpointCreationFailed`,
  since a CloudEndpoint with no terminal traffic-policy action is rejected
  by the ngrok API) rather than a silent blip — but it is fully recoverable
  by re-adding the legacy field, and because the failing call is an *update*
  of an already-created endpoint, ngrok keeps the last-good policy live on
  the data plane. With finalizers, by contrast, the worst consequence is an
  object stuck in `Terminating` forever. A two-release pattern is sufficient.
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

## Per-shim catalog: user-facing key compatibility (read-side only)

### User-facing annotations (read-side compatibility)

- **Pattern:** Two-release. These are user-written keys — the operator never
  writes them, so there is no write side and no delete-on-reconcile
  migration. The dual-read *is* the user contract, which places its removal
  at the 1.0 major-version boundary rather than the R3 read-side sweep:
  dropping it in a post-1.0 minor would be a user-visible breaking change.
- **R1 (0.24):** `internal/annotations/parser/parser.go` resolves each key
  via `annotationKeyFor` — canonical `ngrok.com/<suffix>` wins on presence,
  legacy `k8s.ngrok.com/<suffix>` is the fallback. All `Extract*` helpers in
  `internal/annotations/annotations.go` inherit this through the parser with
  no signature changes. The Ingress, Gateway, and Service controllers call
  `deprecation.ScanAnnotations` once per reconcile to emit `LegacyAnnotation`
  Warning events per legacy key present.
- **Cleanup (1.0, read-side):** delete the fallback in `annotationKeyFor`
  and the `LegacyAnnotationsPrefix` const, the entire `internal/deprecation`
  package, and the `ScanAnnotations` call sites.

### Gateway TLS option keys (read-side compatibility)

- **Pattern:** Two-release, read-side only (same rationale as user-facing
  annotations; removal at 1.0).
- **R1 (0.24):** `pkg/managerdriver/translate_gatewayapi.go` reads both
  `ngrok.com/terminate-tls.*` and `k8s.ngrok.com/terminate-tls.*`; when both
  prefixes define the same option suffix the canonical key wins,
  deterministically (canonical suffixes are collected before the merge loop
  so precedence never depends on map iteration order). The Gateway controller
  emits a single `LegacyAnnotation` Warning event per reconcile via
  `warnIfLegacyTLSOptions` when any listener uses legacy keys.
- **Cleanup (1.0, read-side):** delete `LegacyTLSOptionKeyPrefix`, the legacy
  case in the options loop, the legacy reserved-key entries, and
  `warnIfLegacyTLSOptions`.

### Service `app-protocols` annotation and `http2` appProtocol value (read-side compatibility)

- **Pattern:** Two-release, read-side only (removal at 1.0).
- **R1 (0.24):** `pkg/managerdriver/utils.go::getProtoForServicePort` reads
  `ngrok.com/app-protocols` (presence-based) and falls back to
  `k8s.ngrok.com/app-protocols`; `knownApplicationProtocols` accepts both
  `ngrok.com/http2` and `k8s.ngrok.com/http2` port `appProtocol` values,
  with a deprecation log on the legacy value in `getPortAppProtocol`. Both
  are read only from backend Services of Ingress/Gateway routes, in
  translator hot paths with no event recorder — legacy hits are log-only
  (`legacy annotation key in use` / `legacy appProtocol value in use`).
- **Cleanup (1.0, read-side):** delete `LegacyAppProtocolsAnnotation`, the
  fallback read, the legacy-value log, and the legacy `k8s.ngrok.com/http2`
  map entry.

### Bindings-forwarder pod identity prefix filter (read-side compatibility)

- **Pattern:** Two-release, read-side only (removal at 1.0).
- **R1 (0.24):** `internal/controller/bindings/forwarder_controller.go::podIdentityFromPod`
  forwards pod annotations under either prefix. Keys are forwarded verbatim,
  so upstream traffic-policy expressions that match on annotation key names
  migrate on the pod owner's schedule, not the operator's.
- **Cleanup (1.0, read-side):** drop the legacy prefix match.

## Per-shim catalog: CRD field renames (`LEGACY-FIELD-MIGRATION`)

CRD field renames are two-release cases (see "When two releases is enough"):
the shim touches no K8s object state beyond the CR itself, and the dual-read
lives in a `Get*` helper that collapses once every stored object has been
rewritten under the new field name. R1 adds the new field and keeps the
legacy field readable; the cleanup release drops the legacy field and the
fallback read. No write-side step is needed for **spec** fields — the
operator only reads spec, so there is nothing to dual-write.

### `Domain.spec.resolves_to` → `resolvesTo` (user-written spec field)

- **R1 — migration release (0.24):** `api/ingress/v1alpha1/domain_types.go`:
  - `ResolvesTo` carries the new `json:"resolvesTo"` tag.
  - `ResolvesToLegacy` (`json:"resolves_to"`) is added, marked
    `Deprecated`, and wrapped in a `LEGACY-FIELD-MIGRATION: BEGIN/END`
    block (separated from its doc comment by a blank line so the marker
    does not leak into the CRD description).
  - `DomainSpec.GetResolvesTo()` prefers `ResolvesTo`, falling back to
    `ResolvesToLegacy`; the fallback return carries a
    `LEGACY-FIELD-MIGRATION (read-side cleanup)` marker.
  - `internal/controller/ingress/domain_controller.go` reads via
    `domain.Spec.GetResolvesTo()` at both call sites, never the fields
    directly.
- **Cleanup release:** delete the `ResolvesToLegacy` field and the fallback
  in `GetResolvesTo` (collapse to `return s.ResolvesTo`). `resolves_to` is
  brand-new in 0.24, so the break window is small; still noted in the
  user-facing migration guide.

### Note: `BoundEndpoint.spec.endpointURI` → `endpointURL` (removed)

This earlier field rename (#779) predated the `LEGACY-FIELD-MIGRATION`
marker. It followed the same shape — a `Deprecated` `EndpointURI` field plus
a `GetEndpointURL()` dual-read helper — but was not marked with a sentinel.
The cleanup landed via K8SOP-276: `EndpointURI` is deleted, `GetEndpointURL`
is collapsed to direct `EndpointURL` reads, and `endpointURL` is now
required.
