# Passivity shims and migration strategy

This document is for ngrok-operator maintainers. It describes how we stage
backwards-incompatible changes across multiple releases using **passivity
shims** — small pieces of read-side and/or write-side compatibility code
that let an older operator coexist with a newer one during rolling
upgrades and (where possible) survive a `helm rollback`. User-facing
instructions belong in release-specific upgrade guides under `docs/`, such as
[`docs/upgrading-to-0.24.md`](../upgrading-to-0.24.md).

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
- `LEGACY-trafficpolicy-kind` — CRD **kind and group** rename:
  `ngrok.k8s.ngrok.com/v1alpha1 NgrokTrafficPolicy` →
  `ngrok.com/v1 TrafficPolicy`. Covers both the kind change and the group
  move, since we fold them into a single dual-CRD migration rather than
  paying two dual-read cycles.
- `LEGACY-metadata-format` — CRD `spec.metadata` raw JSON **string** → `map[string]string` (a field *type* change, not a rename; same `json:` tag).
- `LEGACY-enabledfeatures-format` — `KubernetesOperator.status.enabledFeatures` comma-separated **string** → `[]string` (a field *type* change on an operator-only, operator-written status field; same `json:` tag).

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

## Per-shim catalog: TrafficPolicy CRD kind + group rename

This is the pattern for renaming a CRD **kind** or moving it between API
**groups**. Kubernetes has no primitive that lets one storage back two
names: a CRD is keyed by `plural.group`, and conversion webhooks convert
*versions* within one CRD — they can't cross group or kind boundaries.
So the only passive option is to serve **two CRDs** for one policy
concept during the migration window and let the controller resolve either.

### `ngrok.k8s.ngrok.com/v1alpha1 NgrokTrafficPolicy` → `ngrok.com/v1 TrafficPolicy`

- **Pattern:** Two-release (deprecated kind + group, both change together
  in one dual-CRD migration; see the analysis in
  [`docs/superpowers/plans/2026-08-12-trafficpolicy-kind-migration-analysis.md`](../superpowers/plans/2026-08-12-trafficpolicy-kind-migration-analysis.md)
  §"Interaction with the `ngrok.com/v1` group move" for why kind + group
  are folded into one sentinel rather than staged).
  Tag: `LEGACY-trafficpolicy-kind`.
- **R1 — migration release:**
  - Two CRDs ship: canonical `trafficpolicies.ngrok.com` (`api/ngrok/v1`)
    and deprecated `ngroktrafficpolicies.ngrok.k8s.ngrok.com`
    (`api/ngrok/v1alpha1`). Spec is byte-compatible so a user migration
    is a manifest re-stamp (`kind` + `apiVersion`) plus re-apply.
  - The legacy kind carries `+kubebuilder:deprecatedversion:warning=...`,
    which generates `deprecated: true` + `deprecationWarning` on the CRD's
    only version, so `kubectl apply` on a legacy manifest prints the
    migration instruction server-side. That covers users who never reach
    the operator's `DeprecatedAPIGroup` event because they don't have an
    endpoint referencing the policy yet. The Go type also carries a
    `Deprecated:` doc comment for downstream importers of
    `api/ngrok/v1alpha1`; since the operator's own dual-read code must keep
    referencing it, `.golangci.yml` excludes the resulting `SA1019` (the
    exclusion is sentinel-tagged and deletes at cleanup).
  - Two reconcilers run:
    - `TrafficPolicyReconciler` (`internal/controller/ngrok/trafficpolicy_controller.go`)
      for the canonical kind.
    - `NgrokTrafficPolicyReconciler` (`internal/controller/ngrok/ngroktrafficpolicy_controller.go`)
      for the deprecated kind — tagged as legacy at the file level, deletes at cleanup.
    - Both share the `Condition*`/`Reason*` vocabulary. The constants live
      in `trafficpolicy_conditions.go` (the canonical file) so the
      cleanup deletion of `ngroktrafficpolicy_conditions.go` doesn't
      strand them.
  - `internal/store`:
    - `CacheStores.TrafficPolicyV1` is the canonical cache;
      `CacheStores.NgrokTrafficPolicyV1` is the legacy cache (tagged).
    - `Store.ResolveTrafficPolicy(name, ns)` is canonical-first: try the
      v1 cache, fall back to the v1alpha1 cache on miss. The returned
      `TrafficPolicyLookup` carries a `LegacyKind` bool so callers know
      to emit a deprecation event.
    - `Store.GetNgrokTrafficPolicyV1` and the `LegacyKind` field are both
      sentinel-tagged and both go away at cleanup.
  - `internal/trafficpolicy/manager.go::resolveRef` does canonical-first
    resolve directly against the API (endpoint reconcilers use client
    Gets, not the store). On legacy fallback it emits a
    `DeprecatedAPIGroup` warning event on the referring endpoint. When
    both kinds exist under the same name the canonical wins silently —
    no warning, to match the natural collision case during a staged
    migration.
  - `pkg/managerdriver`:
    - `handleExtensionRef` accepts both `TrafficPolicy` and
      `NgrokTrafficPolicy` `ExtensionRef.Kind` values so a HTTPRoute
      authored against the legacy kind still resolves once the user
      re-stamps the policy without needing to edit the route.
    - Ingress and Gateway API translators accept either group in the
      resource reference / extensionRef `Group` field and route through
      `Store.ResolveTrafficPolicy`.
  - Endpoint controllers `Watches(&ngrokv1alpha1.NgrokTrafficPolicy{}, ...)`
    alongside the canonical watch so legacy-kind updates re-enqueue
    referring endpoints. The `Watches` block is `LEGACY-trafficpolicy-kind`-tagged;
    the mapper is kind-agnostic (accepts either type via a type switch)
    and its switch narrows at cleanup.
  - RBAC: `helm/ngrok-operator/templates/{api-manager,agent}/role.yaml`
    keep the `ngroktrafficpolicies` rule blocks tagged
    `LEGACY-trafficpolicy-kind`; the canonical `trafficpolicies` rules
    are on the same file and stay after cleanup. The
    `ngroktrafficpolicy-editor.yaml` / `ngroktrafficpolicy-viewer.yaml`
    aggregation templates are tagged for whole-file deletion; the
    `trafficpolicy-editor.yaml` / `trafficpolicy-viewer.yaml` siblings
    replace them.
- **R-cleanup:** `git grep 'LEGACY-trafficpolicy-kind'` and sweep:
  - Delete `api/ngrok/v1alpha1/ngroktrafficpolicy_types.go` (regen
    `zz_generated.deepcopy.go` drops the `NgrokTrafficPolicy*` method
    set), the two entries in `groupversion_info.go::addKnownTypes`.
  - Delete `internal/controller/ngrok/ngroktrafficpolicy_controller.go`,
    `ngroktrafficpolicy_conditions.go`, `ngroktrafficpolicy_conditions_test.go`,
    and the `NgrokTrafficPolicyReconciler` setup block in `cmd/api-manager.go`.
  - `internal/store/cachestores.go`: drop the `NgrokTrafficPolicyV1`
    field, its initializer, and the `*ngrokv1alpha1.NgrokTrafficPolicy`
    dispatch cases in Get/Add/Delete.
  - `internal/store/store.go`: drop `GetNgrokTrafficPolicyV1`, drop the
    `LegacyKind` field on `TrafficPolicyLookup`, and collapse
    `ResolveTrafficPolicy` to a bare `GetTrafficPolicyV1` call.
  - `internal/trafficpolicy/manager.go`: delete the fallback block in
    `resolveRef`, `warnDeprecatedAPIGroup`, `EventDeprecatedAPIGroup`,
    and inline `marshalTrafficPolicy` back into the canonical branch.
  - `pkg/managerdriver`: drop the `NgrokTrafficPolicy` case in
    `handleExtensionRef` and the LegacyKind log branch; drop the
    `NgrokTrafficPolicy` case in `listObjectsForType` and the Seed
    entry; narrow the kind/group acceptance in
    `translate_ingresses.go` and `translate_gatewayapi.go`; collapse
    the `owningKind` LegacyKind branch.
  - Endpoint controllers: delete the legacy `Watches` calls; narrow the
    mapper type switches to `*ngrokv1.TrafficPolicy` only.
  - Helm: delete
    `helm/ngrok-crds/templates/ngrok.k8s.ngrok.com_ngroktrafficpolicies.yaml`,
    the two `ngroktrafficpolicy-{editor,viewer}.yaml` templates, and the
    tagged rule blocks in `api-manager/role.yaml` and `agent/role.yaml`.
    Drop the two `ngroktrafficpolicy-*` entries and the deprecated-kind case
    from `tests/rbac/crd-access_test.yaml`, then
    `make helm-update-snapshots` and regenerate `manifest-bundle.yaml`.
    Note the CRD manifest is controller-gen output, so it carries no
    sentinel of its own — deleting `ngroktrafficpolicy_types.go` is what
    stops it being emitted.
  - Tests: delete `internal/testutils::NewTestNgrokTrafficPolicy` and the
    legacy-kind fixtures
    `pkg/managerdriver/testdata/translator/ingress-trafficpolicy-mixed-kind.yaml`
    and `gwapi-trafficpolicy-annotation.yaml`; narrow
    `TranslatorTestCase.Input.TrafficPolicies` back to
    `[]*ngrokv1.TrafficPolicy` and drop the type switch in
    `loadTranslatorTestCase`; drop the legacy specs in
    `driver_test.go`'s "dual-kind extensionRef resolution" context.
  - `.golangci.yml`: delete the `SA1019 ... NgrokTrafficPolicy is
    deprecated` exclusion rule.
  - `PROJECT`: delete the legacy scaffold entry.

#### Why we don't shortcut the TrafficPolicy kind + group migration

A few alternatives were considered and rejected:

- **Conversion webhook.** A CRD conversion webhook can retype a live key
  but is registered on a *single* `plural.group`. It cannot cross group
  boundaries, and cannot fold two different kinds onto the same storage.
  So it can't serve `NgrokTrafficPolicy@ngrok.k8s.ngrok.com` →
  `TrafficPolicy@ngrok.com` at all. The user-facing docs used to imply
  the group move would use a conversion webhook; that's wrong per
  Kubernetes semantics and is fixed alongside this shim.
- **Ship the kind rename at v1alpha1 first, then the group move
  separately.** Would cost two dual-CRD dual-read migrations — every
  user does two manifest re-stamps and the operator implements the same
  fallback infrastructure twice on the same conceptual resource. The
  intermediate state also has three shapes (`TP@old + NTP@old + TP@new`)
  in the release where the second migration overlaps the first cleanup,
  which is a materially harder debugging surface. See the analysis doc
  for the sequencing argument.
- **Operator-driven backfill copies legacy CRs into canonical CRs at
  R1.** Rejected because it creates duplicate storage rows that diverge
  on status (each CRD has its own status subresource) and mutates user
  manifests silently. R1 is discover-and-warn only; users re-stamp
  manifests on their own schedule.

If you find yourself adding another CRD kind or group rename, follow
this pattern: two coexisting CRDs, canonical-first dual-read helper on
the store, `DeprecatedAPIGroup` warning event on legacy fallback,
sentinel-tag every legacy-only code path, no operator-driven backfill.

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

## Per-shim catalog: CRD `spec.metadata` type change (`LEGACY-metadata-format`)

This is a different animal from the field renames above. The other CRD
migrations parked the new shape on a **new** `json:` key and let the old key
age out. Here the shape we want (a `map[string]string`) has to live on the key
that is already occupied by string data in the wild (`metadata`). You cannot
passively change the type a live key accepts — existing stored objects hold a
string under `metadata`, and re-typing the key to an object would make them
unreadable by the typed operator and un-appliable. So instead of a rename, the
field becomes **schemaless** for the migration window: the API server accepts
either a JSON string or a JSON object under the same key, and the operator
normalizes both.

- **Pattern:** Two-release (deprecated form, same key). Tag: `LEGACY-metadata-format`.
- **Type:** the field is a bare `json.RawMessage` with
  `+kubebuilder:validation:Schemaless` and `+kubebuilder:pruning:PreserveUnknownFields`
  markers (the same shape as `CloudEndpoint.spec.trafficPolicy.inline`), so
  `encoding/json` and controller-gen handle (un)marshaling and deepcopy — no wrapper
  type. `common.MetadataAPIString` (in `api/common/v1alpha1/metadata_types.go`) turns
  a raw value into the string the ngrok API expects: a legacy string passes through
  verbatim; a flat string→string map is re-marshaled with sorted keys (no spurious
  API diffs); any other JSON (nested, null, or non-string values) is passed through
  unchanged.
- **Affected fields:** `Domain.spec.metadata`, `IPPolicy.spec.metadata` and
  `IPPolicy.spec.rules[].metadata`, `KubernetesOperator.spec.metadata`,
  `CloudEndpoint.spec.metadata`, `AgentEndpoint.spec.metadata`.
- **R1 (0.24):**
  - CRD: the field is schemaless, so both string and object shapes admit. The
    `+kubebuilder:default` stays a **JSON string** (`{"owned-by":"ngrok-operator"}`)
    so defaulted objects remain rollback-safe to a prior release (see below).
  - Controllers read via `common.MetadataAPIString(spec.Metadata)` at every ngrok
    API call site and drift comparison. There is deliberately **no** runtime deprecation event
    for the string form: the only reliable signal that would need it (an object
    that isn't operator-managed) requires ownership-suppression machinery not worth
    its weight, and the strict schema at cleanup rejects the string form at
    admission — a hard error that supersedes any transient event. The deprecation
    lives in the docs (this guide, the migration guide, and the CRD field
    description) only.
  - Operator-generated objects (`pkg/managerdriver/translator.go`,
    `pkg/managerdriver/domains.go`) keep writing the **string** form via
    `commonv1alpha1.MetadataFromLegacyString` for rollback safety.
- **R-cleanup:** drop the legacy string branch in `MetadataAPIString` and delete
  `MetadataFromLegacyString`; change the fields from `json.RawMessage` to
  `map[string]string`; flip the CRD default to the object form; switch the
  operator-generated write paths to the map form; make the CRD schema a real
  `additionalProperties: {type: string}` object. Sweep with
  `git grep 'LEGACY-metadata-format'`.

### Why not a rename or a conversion webhook

- **Rename to a new key** (`metadataMap`, etc.) would force users through *two*
  migrations — first onto the interim key, then back onto `metadata` once the
  string field is removed — and leave an awkward field name in the API for the
  whole 0.2x line. Rejected.
- **A storage-version conversion webhook** cleanly retypes a live key, but the
  operator does not otherwise use a conversion webhook, and the planned
  `ngrok.com/v1` group move is itself expected to be a dual-read migration rather
  than a webhook. Not adopting one just for this field.

The cost of the schemaless approach is weaker server-side validation: the API
server no longer enforces string values on the map. The operator normalizes the
supported forms (legacy string, flat string map) and passes any other JSON
through to ngrok unchanged rather than rejecting it — plus the rollback caveat
for object-form adopters documented in the user-facing guide.

## Per-shim catalog: `KubernetesOperator.status.enabledFeatures` type change

Same family as the `spec.metadata` type change above (a live key can't
passively change the shape it accepts), but simpler: `status.enabledFeatures`
is entirely operator-written — no user ever sets it — so there's no
user-authored-legacy-object case to dual-read, and the operator fully
recomputes the field from the ngrok API on every reconcile rather than
merging into existing state, so it self-heals to the current wire format on
the object's next reconcile with no backfill needed.

Landed via #846 with only a read-side shim (`UnmarshalJSON` accepting either
shape), which covers upgrades but not rollback: the *old* operator binary
predates the shim entirely, so it can never be taught to read the array this
release would otherwise write. Caught testing the 0.24 RC: rolling the
operator back to the prior release while the array-shaped status was already
live crashed api-manager (`unable to create KubernetesOperator: json: cannot
unmarshal array into Go struct field ... of type string`) and spammed
agent-manager's reflector with the same decode failure on every `List`. Both
call sites — `controllerutil.CreateOrUpdate`'s `Get` in
`cmd/api-manager.go::createKubernetesOperator`, and the informer's `List` —
decode the *entire* stored object with the old binary's plain-`string`
`EnabledFeatures` field, so the crash is unconditional and has no
workaround short of `kubectl edit --subresource=status` on every existing
object before starting the old binary.

- **Pattern:** Two-release (deprecated form, same key), operator-written
  variant — no coexistence window is needed because there is exactly one
  writer. Tag: `LEGACY-enabledfeatures-format`.
- **Type:** `api/ngrok/v1alpha1/kubernetesoperator_types.go`'s
  `KubernetesOperatorStatus.EnabledFeatures` keeps its final-state type
  (`KubernetesOperatorEnabledFeatures`, an eventual `[]string`) and doc
  comment — only a `+kubebuilder:validation:Schemaless` /
  `+kubebuilder:pruning:PreserveUnknownFields` marker pair is added so the
  CRD accepts whichever shape is actually written this release
  (`+kubebuilder:validation:Type=string` was tried first and rejected by
  controller-gen: `conflicting types in allOf branches in schema: array vs
  string`, because the field's underlying Go type is a slice).
- **R1 (0.24):**
  - `api/ngrok/v1alpha1/kubernetesoperator_status_compat.go`:
    `UnmarshalJSON` (already landed in #846) keeps reading either shape.
    A new `MarshalJSON` writes the legacy comma-separated string — the
    write-side half #846 was missing — so a rollback to the prior release
    can decode status this release wrote.
  - CRD: `status.enabledFeatures` becomes schemaless (no `type:`,
    `x-kubernetes-preserve-unknown-fields: true`) instead of the strict
    `type: array` #846 generated, matching what's actually on the wire.
  - No controller change: `kubernetesoperator_controller.go:308`
    (`ko.Status.EnabledFeatures = ngrokKo.EnabledFeatures`) is unaware of
    the wire format; the shim type's `MarshalJSON`/`UnmarshalJSON` handle
    it transparently.
- **R-cleanup:** delete `MarshalJSON` (write-side cleanup) so the field
  marshals as a plain array again; drop the `Schemaless` /
  `PreserveUnknownFields` markers and regenerate the CRD back to strict
  `type: array`. Keep `UnmarshalJSON` one release longer — an object last
  reconciled under R1 still carries the legacy string until its next
  reconcile — then delete it too (read-side cleanup) along with the
  `KubernetesOperatorEnabledFeatures` type, switching the field to plain
  `[]string`. Sweep with `git grep 'LEGACY-enabledfeatures-format'`.

### Why not a rename or a conversion webhook

Same reasoning as `spec.metadata`: a new field name (e.g.
`enabledFeatureList`) would need its own eventual deprecation once
`enabledFeatures` was free to take the array shape, trading one migration
for two; a conversion webhook is a pattern this operator has deliberately
not adopted anywhere. Unlike `spec.metadata`, there's no user-authored form
to ever reconcile away — once every operator's next reconcile has run
against a release with `MarshalJSON` removed, no object anywhere is left
carrying the string form.
