# KubernetesOperator: `ngrok.k8s.ngrok.com/v1alpha1` → `ngrok.com/v1`

Ticket: K8SOP-318. Part of the 1.0 CRD group move (see
`docs/developer-guide/passivity-shims.md`, TrafficPolicy catalog entry, for
the reference pattern this deviates from).

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
- **No ngrok-side leftovers, no live legacy object.** After the upgrade the
  legacy KO object is inert: no finalizer, never reconciled, marked superseded.
  It is removed automatically when the cleanup release drops the legacy CRD.
  Alex accepted this (2026-10-26) over deleting it in R1, because deleting it
  in R1 breaks `helm rollback` (see "Why not delete the legacy object in R1").
- **Two releases.** R1 (this work) ships the canonical kind and the migration
  code. The cleanup release removes the migration code, the legacy type and
  the legacy CRD.

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
deletes nothing. There is no reliable way to tell "left over from before the
upgrade" from "recreated by a rollback", so R1 never deletes the legacy object.
It strips the finalizer, which removes the one dangerous thing about it.

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

### Legacy neutralization (the migration)

This lives in the leader-elected `KubernetesOperatorReconciler`, not in the
startup function. The old operator re-adds its finalizer whenever it
reconciles: an old-version pod mid-rollout, or a rollback followed by a
re-upgrade. Only a continuous check catches that. The reconciler adds
`Watches(&ngrokv1alpha1.KubernetesOperator{})`, filtered to its own
namespace/name and mapped onto the canonical key.

On every reconcile of the canonical object, if a legacy object with the same
namespace/name exists and the canonical object is not being deleted:

1. Patch the legacy object in one merge patch:
   - remove the operator finalizer, under both the `ngrok.com/` and
     `k8s.ngrok.com/` prefixes;
   - set the annotation `ngrok.com/superseded-by: kubernetesoperators.ngrok.com/<name>`.

   Skip the patch when both are already in that state.
2. Emit a Normal event on the canonical object (reason
   `LegacyKubernetesOperatorSuperseded`) only when the patch actually removed a
   finalizer, and log it.

Nothing is ever deleted. An old-version pod that sees the legacy object
without a finalizer re-adds it on its next reconcile, which is harmless. The
new leader strips it again, and an old pod never sees a deletion with the
finalizer present unless a human deletes the legacy object.

No registration gate is needed: stripping the finalizer doesn't remove the
legacy object, so it stays available as a reader fallback.

### CRD deletion must not drain

`installCRDs` defaults to `true`. CRDs ship as templates in the `ngrok-crds`
subchart, with no `helm.sh/resource-policy: keep`. So a default `helm rollback`
to a release without `kubernetesoperators.ngrok.com` deletes that CRD while the
new-version pods are still running. The API server then marks the canonical
KO for deletion, and today's `delete()` would run the full drain and delete the
ngrok KO.

Fix: at the top of the canonical `delete()`, before any drain work, read the
`kubernetesoperators.ngrok.com` CRD with the uncached API reader. If it has a
`deletionTimestamp`:

- skip the drain and the ngrok-side delete;
- return success so BaseController removes the finalizer;
- log it and emit a Warning event (reason `CRDDeletionSkippedDrain`).

The ngrok KO and every managed resource stay. The rolled-back old operator
re-adopts them through the legacy object.

- Error handling:
  - NotFound on the CRD: treat as deleting.
  - Forbidden: log and fall through to today's drain behavior, for custom-RBAC
    installs that lack the new rule.
  - Any other error: return it and requeue.
- RBAC: a new ClusterRole (with binding) for api-manager granting `get` on
  `customresourcedefinitions` with `resourceNames: [kubernetesoperators.ngrok.com]`.
  It must be a ClusterRole even in namespace-scoped mode, because CRDs are
  cluster-scoped.
- Scheme: register `apiextensionsv1` in api-manager. Do not put CRDs in the
  cache.
- This guard is **not** legacy-tagged. It stays after cleanup, because
  "deleting the CRD must not delete the user's ngrok resources" is a permanent
  safety property.

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
- api-manager's legacy rule already has patch, which the neutralization step
  needs. No new legacy verbs.

Aggregated user roles: the existing `crd-access/kubernetesoperator-{editor,viewer}`
roles keep their file and ClusterRole names, so user bindings keep working.
They gain rules for the `ngrok.com` group, and the legacy-group rules are
tagged for removal. TrafficPolicy got new files only because its kind name
changed. Update `tests/rbac/crd-access_test.yaml` and the snapshots.

Cleanup hook job (`cleanup-hook/job.yaml`):

- Delete `kubernetesoperators.ngrok.com/<release>`. This is the drain-bearing
  delete and keeps the existing `--wait`, `--timeout` and `|| true`.
- Then delete `kubernetesoperators.ngrok.k8s.ngrok.com/<release>` with
  `--ignore-not-found --wait=false`, tagged. It has no finalizer, so the
  delete is immediate. Qualify both names: the plural is identical in
  both groups, so bare `kubernetesoperator` is ambiguous.

Chainsaw: about 35 asserts in `tests/chainsaw-uninstall/*` use
`apiVersion: ngrok.k8s.ngrok.com/v1alpha1` for the KO, and
`tests/chainsaw*/operator-registration` asserts on it too. All move to
`ngrok.com/v1`. Bare `kubectl get/delete kubernetesoperator` calls in chainsaw
and `.github/workflows/ci.yaml` get fully qualified.

### Rollback story

**Default install (bundled CRDs), `helm rollback` to the previous release:**

1. Helm deletes the `kubernetesoperators.ngrok.com` CRD and rolls the
   Deployments back.
2. The still-running new pod sees the canonical object terminating and the CRD
   terminating. It skips the drain and removes its finalizer, and the CRD
   finishes deleting.
3. Old pods start. `createKubernetesOperator` updates the legacy object, which
   still exists. Their reconciler re-adds the finalizer and `findExisting`
   re-adopts the same ngrok KO with the same ID and the same bindings TLS
   Secret.

No drain, no churn, no manual step.

**Separate CRDs (`installCRDs=false`), operator rolled back, CRDs kept:**

- The legacy flow is the same as above.
- The canonical object stays with its finalizer and nobody reconciles it. It
  is harmless.
- On re-upgrade it resumes.
- Uninstalling from the rolled-back release is the one manual case. The old
  hook's bare `kubectl delete kubernetesoperator` is ambiguous with both CRDs
  present and may hit the new group, where it has no RBAC. The guide documents
  the manual fix:
  1. Delete `kubernetesoperators.ngrok.k8s.ngrok.com/<release>` yourself, so
     the old operator drains.
  2. Strip the finalizer from the leftover canonical object:

     ```sh
     kubectl patch kubernetesoperators.ngrok.com <release> -n <ns> --type=merge -p '{"metadata":{"finalizers":null}}'
     ```

### Upgrade requirements

- **`installCRDs=false`:** upgrade `ngrok-crds` first, or the operator fails
  cache sync on the canonical kind. This is the same rule as 0.25. State it
  once for all new CRDs in the release's upgrade guide.
- **Upgrade through R1.** Skipping R1 leaves the legacy object with its
  finalizer. Dropping the legacy CRD in the cleanup release then hangs, because
  nothing strips that finalizer. The cleanup release's guide says so.

## Cleanup release (later, not this branch)

Run `git grep 'LEGACY-kubernetesoperator-kind'` and delete:

- the legacy type and its compat decoder (`LEGACY-enabledfeatures-format`, K8SOP-306/321);
- the legacy watch and the neutralization step;
- the reader fallbacks;
- the legacy cache entries, RBAC blocks, crd-access roles and hook line;
- the SA1019 exclusion and the PROJECT entry.

The legacy CRD then stops being generated and leaves the ngrok-crds chart.
Helm deletes it on upgrade, and the inert legacy objects (no finalizer) go with
it. The CRD-deletion drain guard stays. Add a passivity-shims catalog entry in
R1 that lists exactly this.

## Verification

- **Unit and envtest** (`internal/controller/ngrok` suite):
  - a legacy object with a finalizer exists → after reconcile its finalizer is
    gone, the superseded annotation is set, the object still exists, no drain
    ran, and the canonical object adopted the same mock ngrok KO;
  - the finalizer is re-added to the legacy object → it is stripped again
    (watch path);
  - the canonical object is terminating → the legacy object is untouched;
  - canonical delete while the KO CRD is terminating → no drain, no ngrok
    delete, finalizer removed;
  - canonical delete while the CRD is healthy → drain runs as today.
- **Drain state checker and forwarder:** table tests for canonical-only,
  legacy-only, both, and neither.
- **Type tests:** v1 metadata round-trip and default. v1 `enabledFeatures`
  always serializes as an array.
- **Generated files and tooling:** `make generate manifests manifest-bundle`
  produces a clean diff. `make lint`, `make go-fix` (clean), `make test` and
  `make helm-test` all pass.
- **kind cluster, by hand:**
  1. Install the current release with bindings enabled and a few endpoints.
  2. Record the KO ID, the endpoint IDs and the TLS secret `tls.crt` hash.
  3. Upgrade to the branch. Check that the legacy KO has no finalizer and is
     annotated, the canonical KO has the same ID, the endpoint IDs and cert
     hash are unchanged, and the logs show no drain.
  4. `helm rollback`. Check that the canonical CRD is gone, there is no drain,
     the legacy KO has its finalizer back with the same ID, and nothing
     churned.
  5. Re-upgrade. Check that the legacy finalizer is stripped again.
  6. Uninstall. Check that the drain runs and both CRDs are removed.

## Left to the builder

- File layout for the v1 helpers, and whether condition setters move or are
  retyped in place.
- Event reason wording and log fields.
- Test placement within the existing suites.
