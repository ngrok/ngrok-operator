# KubernetesOperator: `ngrok.k8s.ngrok.com/v1alpha1` → `ngrok.com/v1`

Ticket: K8SOP-318. Part of the 1.0 CRD group move. The release plan and the
twin rules every kind follows are in `specs/crd-group-migration.md`; this doc
covers what is specific to KubernetesOperator and conforms to that spec,
including its listed KubernetesOperator exceptions (rules 6 and 10).

## Intent

Every operator CRD moves to `ngrok.com/v1` before 1.0. KubernetesOperator (KO)
is the first kind that is **written only by the operator** and **backs an ngrok
API resource**. Users never author it, so the TrafficPolicy recipe ("users
re-stamp manifests, then delete the old object") does not apply. The operator
migrates its own object.

At the same time, the v1 type drops the two shape shims the v1alpha1 type
carries:

- `spec.metadata`: v1alpha1 is a schemaless `json.RawMessage` accepting a JSON
  string or a map (`LEGACY-metadata-format`). v1 is a strict
  `map[string]string`.
- `status.enabledFeatures`: v1alpha1 writes a comma-separated string via
  `MarshalJSON` for rollback safety (`LEGACY-enabledfeatures-format`). v1 is a
  strict `[]string`.

The enabledFeatures shim stays on the v1alpha1 type and dies with it in the
cleanup release (this absorbs K8SOP-321). The `spec.metadata` helpers
(`MetadataAPIString` and friends, tag `LEGACY-metadata-format`) are shared with
Domain, IPPolicy, CloudEndpoint and AgentEndpoint. They stay; only the KO's use
of them goes away.

## Requirements (from Alex)

- **No user action.** Upgrading the chart is the whole migration.
- **No downtime.** No drain, no endpoint churn, no bindings cert rotation, and
  the ngrok-side KO keeps its ID.
- **A realistic rollback path.** `helm rollback` to the previous release must
  work, with at most a documented manual step for edge cases.
- **No ngrok-side leftovers.** The legacy KO object stays passive in R1:
  finalizer and ID intact, never reconciled, never deleted, marked
  superseded. R2 (1.0) strips its finalizer; R3 (1.1) removes the legacy CRD
  and the object with it.
- **Three releases**, shared with every kind (`specs/crd-group-migration.md`).
  R1 (0.25, this work) ships the v1 kind and the pair logic. R2 removes the
  pair logic and the legacy type. R3 removes the legacy CRD.

## Non-goals

- Migrating any other kind. Domain, IPPolicy and the endpoints get their own
  designs.
- A generic multi-kind framework. KO is a singleton per install. Build what KO
  needs.
- Changing KO behavior (drain, registration, bindings cert renewal) beyond the
  type swap.

## Why "skip the delete" and not "delete and recreate"

Deleting the legacy KO object normally runs its finalizer, and the finalizer
runs a **full drain**: it deletes or retains every AgentEndpoint, CloudEndpoint,
Domain, IPPolicy and BoundEndpoint, then deletes the ngrok KO. Recreating the
ngrok KO would also mint a new ID and force a new bindings certificate. Both
cause downtime.

Adoption already exists. `findExisting` matches the ngrok KO by deployment
name, deployment namespace and the `namespace.uid` key the operator merges into
the KO metadata. A new canonical object with the same name in the same
namespace therefore attaches to the **same ngrok KO** on its first reconcile.
The legacy object only needs to stop being able to run its finalizer.

## Why not delete the legacy object in R1

`createKubernetesOperator` runs at startup, before leader election, and it is
the only place the old operator creates its KO. During `helm rollback` the
old-version pods start, recreate the legacy object, and wait for the lease.
The new-version leader is still running at that moment. If R1 deleted legacy
objects, it would delete this one. The old operator would then run with no KO:
its BoundEndpoint poller hangs, the forwarder errors, and its uninstall hook
deletes nothing. So R1 never deletes the legacy object (rule 7), and it keeps
the legacy finalizer (rule 8), so a rollback finds the object exactly as 0.24
left it.

## Design (R1)

### API

- New `api/ngrok/v1/kubernetesoperator_types.go`.
  - Same spec, status, print columns and condition vocabulary as v1alpha1.
  - `spec.metadata`: `map[string]string` with an object default of
    `{"owned-by":"ngrok-operator"}`. No `Schemaless` or `PreserveUnknownFields`.
  - `status.enabledFeatures`: `[]string` with no custom marshaller.
  - `categories=ngrok`, matching TrafficPolicy.
  - Condition, reason, feature and `DrainPolicy` constants are defined in the v1
    package. Operator code switches to them.
- Register the type in `api/ngrok/v1/groupversion_info.go`. Regenerate deepcopy,
  the CRD (`helm/ngrok-crds/templates/ngrok.com_kubernetesoperators.yaml`) and
  `manifest-bundle.yaml`.
- Legacy `api/ngrok/v1alpha1/kubernetesoperator_types.go`:
  - add `+kubebuilder:deprecatedversion:warning=...`;
  - add a Go `Deprecated:` doc comment;
  - add an SA1019 exclusion in `.golangci.yml`;
  - tag everything `LEGACY-kubernetesoperator-kind`;
  - keep its constants for the code that still touches it.
- Shared helper for the API metadata string: a small function that marshals the
  v1 map (plus the `namespace.uid` merge) to the string the ngrok API takes.
  `mergeMetadata` already works on strings, so marshal the map and reuse it.

### Writer: api-manager

- `createKubernetesOperator` (`cmd/api-manager.go`) writes
  `ngrokv1.KubernetesOperator` and stops writing the legacy kind entirely. It
  runs before leader election in every replica, as it does today.
- `KubernetesOperatorReconciler` and `drain.Orchestrator` are retyped to v1.
  There is no generic reconciler and no legacy reconciler: the operator never
  reconciles the legacy kind in R1.

### Legacy object (passive)

`reconcileLegacy`, in the leader-elected `KubernetesOperatorReconciler`
(`kubernetesoperator_legacy.go`, tagged). The reconciler adds
`Watches(&ngrokv1alpha1.KubernetesOperator{})`, filtered to its own
namespace/name and mapped onto the same key, so one reconcile handles both
halves of the pair (rule 2). It runs after the v1 object has adopted its ID
(rule 3) and never while the v1 object is being deleted:

- **Live legacy object:** keep its finalizer. Set `Ready=False`, reason
  `SupersededByV1`, and emit one `DeprecatedAPIGroup` warning event naming the
  v1 object, only when the condition changes (rule 4). No ngrok call.
- **Deleted legacy object, v1 holds the ID:** release its finalizer (both
  prefixes) with no ngrok call (rule 9). "Delete the legacy object" never
  hangs and never drains.
- **Deleted legacy object, v1 not yet registered:** leave it; the next
  reconcile after registration releases it.

### Drain guard (rule 5, fail closed)

At the top of the v1 `delete()`, `shouldSkipDrain` reads the same-named legacy
object through the uncached API reader:

| Condition | Result |
|---|---|
| A drain already in progress (`Draining=True`) | Always continues |
| Legacy twin exists | Skip the drain and the ngrok delete; return success so BaseController releases the finalizer; Warning event `LegacyTwinSkippedDrain` |
| Legacy read Forbidden | Treat the twin as present: skip |
| Legacy NotFound, or legacy kind not served | Drain as before |
| Any other error | Return it and requeue |

This is the KubernetesOperator exception to rule 6: nothing hands control
back to the legacy object mid-run, because the operator only creates its KO
at startup. The uninstall hook deletes the legacy object first, so a real
uninstall finds no twin and drains (the exception to rule 10).

With `keep` on the CRDs (below), rollback no longer deletes the v1 object, so
the guard is a safety net: for a hand deletion of the v1 object, or a rollback
path that removes the CRD some other way.

### CRD retention

Both `ngrok.com` CRDs carry `helm.sh/resource-policy: keep` via
`+kubebuilder:metadata:annotations` (tagged). Helm reads the policy from the
release it moves away from, so `helm rollback` from R1 to 0.24 leaves the
CRDs and their objects in place: no stuck finalizer, no `Terminating` CRD, no
manual step. Because keep also stops `helm uninstall`, the uninstall hook
deletes both CRDs once the v1 KO is gone, when `installCRDs` is set, through a
hook-only ClusterRole limited to the two CRD names. R2 removes the keep
markers and the hook block: rolling back from R2 to R1 deletes nothing,
because both ship the CRDs.

### Readers

Two other places read the KO:

| Reader | Code |
|---|---|
| Drain state checker (api-manager, agent-manager) | `internal/drain/state.go` |
| Bindings forwarder | `internal/controller/bindings/forwarder_controller.go` |

Each reads the canonical object first and falls back to the legacy object when
the canonical object is NotFound **or not yet registered** (empty `status.id`).
api-manager creates the canonical object at startup with an empty status, so
NotFound alone would never cover the window. When both are unusable, return
the canonical result. The fallback is tagged `LEGACY-kubernetesoperator-kind`.

The fallback covers the window between a new agent or forwarder pod starting
and api-manager creating the canonical object. Without it the forwarder errors
during that window and the drain checker reports "not draining". The second is
safe today but brittle.

The BoundEndpoint poller (`boundendpoint_poller.go:getKubernetesOperatorId`)
runs in api-manager and polls until the KO is registered. It switches to the
canonical kind **only**, with no fallback. The same process creates the
canonical object at startup, and waiting for the canonical registration means
the poller sees the ID the operator now owns. Both objects carry the same ID
anyway.

Every manager that reads the KO adds the canonical kind to its namespace-pinned
cache config next to the legacy one:

- `cmd/api-manager.go`
- `cmd/agent-manager.go`
- `cmd/bindings-forwarder-manager.go` (it has a `ByObject` cache config)

### Helm

RBAC for the canonical `kubernetesoperators` (`ngrok.com`) resource, plus
`/status` and `/finalizers`:

| Component | Template | Canonical rules |
|---|---|---|
| api-manager | `api-manager/release-namespace-role.yaml` | Same verbs as today's legacy rules |
| agent | `agent/release-namespace-role.yaml` | Read |
| bindings-forwarder | `bindings-forwarder/role.yaml` | Read |
| cleanup hook | `cleanup-hook/rbac.yaml` | get/list/watch/delete |

Legacy rules:

- Keep them, tagged with BEGIN/END sentinels.
- api-manager's legacy rules already have patch and status update, which
  `reconcileLegacy` needs. No new legacy verbs.

Aggregated user roles: the existing `crd-access/kubernetesoperator-{editor,viewer}`
roles keep their file and ClusterRole names, so user bindings keep working.
They gain rules for the `ngrok.com` group, and the legacy-group rules are
tagged for removal. TrafficPolicy got new files only because its kind name
changed. Update `tests/rbac/crd-access_test.yaml` and the snapshots.

Cleanup hook job (`cleanup-hook/job.yaml`), all names fully qualified because
both kinds share the plural:

1. Patch the legacy object's finalizers to null (in case the operator is not
   running), then delete it.
2. Delete `kubernetesoperators.ngrok.com/<release>`. This is the drain-bearing
   delete and keeps the existing `--wait`, `--timeout` and `|| true`.
3. With `installCRDs`, delete the two `ngrok.com` CRDs once the v1 KO is gone.

Chainsaw: about 35 asserts in `tests/chainsaw-uninstall/*` use
`apiVersion: ngrok.k8s.ngrok.com/v1alpha1` for the KO, and
`tests/chainsaw*/operator-registration` asserts on it too. All move to
`ngrok.com/v1`. Bare `kubectl get/delete kubernetesoperator` calls in chainsaw
and `.github/workflows/ci.yaml` get fully qualified.

### Rollback story

**`helm rollback` (or `helm upgrade --version 0.24.x`) to 0.24.** Verified in
kind with the keep annotation:

1. Helm rolls back RBAC and the Deployments. The `ngrok.com` CRDs are kept.
2. The v1 KO stays, with its finalizer. Nothing reconciles it, and nothing
   deletes it, so nothing drains.
3. The 0.24 operator reconciles the legacy object it left, with its finalizer
   and ID intact, and `findExisting` re-adopts the same ngrok KO.
4. Re-upgrading to R1 resumes the v1 KO with the same ID.

No manual step. Separate CRD installs behave the same.

**Uninstall after a rollback.** Accepted as rare
(`specs/crd-group-migration.md`, "Helm rollback"). The 0.24.2 hook deletes the
bare `kubernetesoperator`, which resolves to `ngrok.com`, where its role is
Forbidden, so nothing drains and old objects stick in `Terminating`. The
upgrade guide documents the avoidance (delete the legacy KO by its qualified
name first) and the cleanup.

### Upgrade requirements

- **`installCRDs=false`:** upgrade `ngrok-crds` first, or the operator fails
  cache sync on the canonical kind. Stated once for all new CRDs in the
  release's upgrade guide.
- **0.25 and 1.0 are required stops** (`specs/crd-group-migration.md`,
  "Required stops").

## Later releases (not this branch)

- **R2 (1.0):** run `git grep 'LEGACY-kubernetesoperator-kind'`. Delete the
  legacy type and its compat decoder (`LEGACY-enabledfeatures-format`,
  K8SOP-306/321), `reconcileLegacy`, the legacy watch, the drain guard, the
  reader fallback, the legacy cache entries, the keep markers, the hook's CRD
  block and ClusterRole, the legacy RBAC and hook lines, the SA1019 exclusion
  and the PROJECT entry. The shared finalizer sweeper strips the legacy
  finalizer. The legacy CRD stays in the chart.
- **R3 (1.1):** the legacy CRD leaves the chart. Helm deletes it, and the
  finalizer-free legacy object goes with it.

## Verification

- **Unit and envtest** (`internal/controller/ngrok` suite):
  - a legacy object with a finalizer exists → the v1 object adopts the same
    mock ngrok KO, the legacy object keeps its finalizer and gets
    `SupersededByV1`, one `DeprecatedAPIGroup` event, no drain;
  - the legacy object is deleted → its finalizer is released, the ngrok KO
    is untouched, the v1 object does not drain;
  - a terminating legacy object is released once the v1 object holds the ID,
    and not before;
  - the canonical object is terminating → the legacy object is untouched;
  - `shouldSkipDrain`: twin present, NotFound, NoMatch, Forbidden, drain in
    progress, other error (requeue).
- **Drain state checker and forwarder:** table tests for canonical-only,
  legacy-only, both, and neither.
- **Type tests:** v1 metadata round-trip and default. v1 `enabledFeatures`
  always serializes as an array.
- **Generated files and tooling:** `make generate manifests manifest-bundle`
  produces a clean diff. `make lint`, `make go-fix` (clean), `make test` and
  `make helm-test` all pass.
- **kind cluster, by hand:**
  1. Install 0.24.2 with bindings enabled and a few endpoints.
  2. Record the KO ID, the endpoint IDs and the TLS secret `tls.crt` hash.
  3. Upgrade to the branch. Check that the legacy KO keeps its finalizer and
     shows `SupersededByV1`, the v1 KO has the same ID, the endpoint IDs and
     cert hash are unchanged, and the logs show no drain.
  4. `helm rollback`. Check that both `ngrok.com` CRDs remain, there is no
     drain, and 0.24 serves the legacy KO with the same ID.
  5. Re-upgrade. Same ID, no drain.
  6. Uninstall. Check that the drain runs once and every ngrok CRD is gone.

## Left to the builder

- File layout for the v1 helpers, and whether condition setters move or are
  retyped in place.
- Event reason wording and log fields.
- Test placement within the existing suites.
