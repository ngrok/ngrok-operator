# KubernetesOperator → ngrok.com/v1 (R1) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make `ngrok.com/v1 KubernetesOperator` the kind the operator writes and reconciles. The legacy `ngrok.k8s.ngrok.com/v1alpha1` object becomes inert, with no user action, no drain and no ngrok-side churn.

**Architecture:**
- New v1 type with strict `spec.metadata` map and strict `status.enabledFeatures` array.
- The operator code is retyped to v1.
- The KO reconciler continuously strips the legacy object's finalizer and never deletes it.
- Readers fall back to the legacy object until the canonical object registers.
- A permanent guard skips the drain when the KO CRD itself is being deleted, which is the `helm rollback` case.

**Tech Stack:** Go, controller-runtime, kubebuilder markers, Ginkgo/envtest, helm-unittest, chainsaw.

**Spec:** `specs/design-docs/2026-10-26-kubernetesoperator-v1-migration.md`

> **Superseded in part (2026-10-27):** Task 5's CRD-read guard, its ClusterRole
> and the uninstall-hook annotation were replaced by a legacy-twin check. The
> spec's "Rollback must not drain" section is authoritative.

## Global Constraints

- Sentinel tag for everything that goes at cleanup: `LEGACY-kubernetesoperator-kind`. Use `// LEGACY-kubernetesoperator-kind: BEGIN` / `END` blocks or line markers. Never wrap surviving lines. Never put a marker in a CRD field doc comment.
- The superseded annotation key is exactly `ngrok.com/superseded-by`, with value `kubernetesoperators.ngrok.com/<name>`.
- Event reasons are exactly `LegacyKubernetesOperatorSuperseded` (Normal) and `CRDDeletionSkippedDrain` (Warning).
- The CRD name the guard reads is exactly `kubernetesoperators.ngrok.com`.
- The v1 `spec.metadata` default is exactly `{"owned-by":"ngrok-operator"}`.
- The operator never deletes the legacy KO object.
- `make lint` and `make go-fix` must leave a clean diff. Tests included.
- Helm settings and values are untouched. This change adds no config.

## Review Focus

1. **Old pod re-adds the legacy finalizer mid-rollout.** The new leader must strip it again: a watch on the legacy kind, not a one-shot.
2. **Canonical object exists but is unregistered** (empty `status.id`). Readers must use the legacy object, not report "no bindings endpoint" or "not draining" from the empty canonical object.
3. **The CRD Get returns Forbidden** (custom RBAC). The uninstall drain must still happen. NotFound must count as "deleting".
4. **The legacy object is terminating when the new operator starts.** The finalizer is stripped and no drain runs.
5. **The canonical object is terminating.** The reconciler must not touch the legacy object during an uninstall drain.

Each is pinned by a test in the owning task.

---

### Task 1: v1 KubernetesOperator API type and CRD

**Files:**
- Create: `api/ngrok/v1/kubernetesoperator_types.go`, `api/ngrok/v1/kubernetesoperator_types_test.go`
- Modify:
  - `api/ngrok/v1/groupversion_info.go`
  - `api/ngrok/v1alpha1/kubernetesoperator_types.go` (deprecation marker and `Deprecated:` comment, tagged)
  - `.golangci.yml` (SA1019 exclusion for `KubernetesOperator is deprecated`, tagged)
  - `PROJECT`
- Generated:
  - `api/ngrok/v1/zz_generated.deepcopy.go`
  - `helm/ngrok-crds/templates/ngrok.com_kubernetesoperators.yaml`
  - legacy CRD (now with `deprecated: true`)
  - `manifest-bundle.yaml`

**Interfaces:**
- Produces:
  - Package `ngrokv1` types: `KubernetesOperator`, `KubernetesOperatorList`, `KubernetesOperatorSpec`, `KubernetesOperatorStatus`, `KubernetesOperatorDeployment`, `KubernetesOperatorBinding`, `KubernetesOperatorDrainStatus`, `DrainConfig`, `DrainPolicy`.
  - The same constant names as v1alpha1: `KubernetesOperatorCondition*`, `KubernetesOperatorReason*`, `KubernetesOperatorFeature*`, `DrainPolicyDelete` / `DrainPolicyRetain`.
  - The same methods as v1alpha1: `GetDrainPolicy() DrainPolicy`, `IsDrainComplete() bool`, `SetObservedGeneration(int64)`.
  - `Spec.Metadata map[string]string`.
  - `Status.EnabledFeatures []string`.

- [ ] **Step 1: Write failing tests** in `kubernetesoperator_types_test.go`:
  - `TestKubernetesOperatorStatusEnabledFeaturesMarshalsAsArray` marshals a status with `[]string{"ingress","bindings"}` and asserts the JSON contains `"enabledFeatures":["ingress","bindings"]`.
  - Port the `GetDrainPolicy` and `IsDrainComplete` tables from `api/ngrok/v1alpha1/kubernetesoperator_types_test.go`, retargeted to v1.
- [ ] **Step 2: Run** `go test ./api/ngrok/v1/...`. Expect a compile failure.
- [ ] **Step 3: Implement the type.**
  - Copy the v1alpha1 type file, then change two fields:
    - `Metadata map[string]string` with `+kubebuilder:default:={"owned-by":"ngrok-operator"}`, and no Schemaless or PreserveUnknownFields.
    - `EnabledFeatures []string` in status, with no shim markers.
  - Add `+kubebuilder:resource:categories=ngrok`.
  - Register `&KubernetesOperator{}` and `&KubernetesOperatorList{}` in `addKnownTypes`.
  - Legacy type:
    - `+kubebuilder:deprecatedversion:warning="ngrok.k8s.ngrok.com/v1alpha1 KubernetesOperator is deprecated and managed automatically; the operator now uses ngrok.com/v1 KubernetesOperator"`
    - a `Deprecated:` doc line;
    - a sentinel line.
- [ ] **Step 4: Run** `make generate manifests manifest-bundle && go test ./api/...`. Expect PASS, a new CRD file, and the legacy CRD showing `deprecated: true`.
- [ ] **Step 5: Commit**: "Add ngrok.com/v1 KubernetesOperator type".

### Task 2: Retype the operator to v1

**Files:**
- Modify:
  - `internal/controller/ngrok/kubernetesoperator_controller.go`
  - `internal/controller/ngrok/kubernetesoperator_conditions.go`
  - `internal/drain/orchestrator.go`
  - `internal/drain/state.go` (type only; fallback comes in Task 3)
  - `internal/controller/bindings/boundendpoint_poller.go` (v1 only, no fallback)
  - `internal/controller/bindings/forwarder_controller.go` (type only)
  - `cmd/api-manager.go` (`createKubernetesOperator` writes v1; add v1 KO to the namespace-pinned `ByObject` cache, keeping the legacy entry tagged)
  - `cmd/agent-manager.go` and `cmd/bindings-forwarder-manager.go` (add the v1 KO cache entry, legacy tagged)
- Tests (all fixtures retargeted to v1):
  - `internal/controller/ngrok/kubernetesoperator_controller_test.go`
  - `internal/controller/ngrok/kubernetesoperator_controller_unit_test.go`
  - `internal/drain/*_test.go`
  - `internal/controller/bindings/suite_test.go`

**Interfaces:**
- Consumes: the Task 1 types.
- Produces:
  - `drain.Orchestrator.HandleDrain(ctx, *ngrokv1.KubernetesOperator)`.
  - `metadataForAPI(m map[string]string) string`, in the controller file. It marshals the map to a JSON string, returns `""` for an empty map, and replaces `commonv1alpha1.MetadataAPIString(ko.Spec.Metadata)` in `tryMergeMetadata`. `mergeMetadata` is unchanged.

- [ ] **Step 1:** Swap `ngrokv1alpha1.KubernetesOperator*` and the KO constant references to `ngrokv1.` in the files above. Test fixtures that built metadata with `commonv1alpha1.MetadataFromLegacyString(...)` become plain maps.
- [ ] **Step 2: Add a unit test** `TestMetadataForAPI`:
  - `{"owned-by":"ngrok-operator"}` → `{"owned-by":"ngrok-operator"}`;
  - nil → `""`;
  - output feeds `mergeMetadata` without error.
- [ ] **Step 3: Run** `go build ./... && go test ./internal/controller/ngrok/... ./internal/drain/... ./internal/controller/bindings/... ./cmd/...`. Expect PASS. Existing registration, drain and renewal specs now run against v1.
- [ ] **Step 4: Commit**: "Reconcile KubernetesOperator as ngrok.com/v1".

### Task 3: Reader fallback to the legacy KO

**Files:**
- Create: `internal/kubernetesoperator/lookup.go`, `internal/kubernetesoperator/lookup_test.go`
- Modify: `internal/drain/state.go`, `internal/controller/bindings/forwarder_controller.go`
- Test: `internal/drain/state_test.go`, plus forwarder tests if the existing suite covers this function; otherwise `lookup_test.go` is enough.

**Interfaces:**
- Produces `kubernetesoperator.Lookup(ctx context.Context, c client.Reader, key client.ObjectKey) (*ngrokv1.KubernetesOperator, error)`:
  - Return the canonical object when it exists with a non-empty `Status.ID`.
  - Otherwise, if the legacy object exists, return it converted to v1. Copy only the fields readers use: ObjectMeta, `Spec.Binding`, `Status.ID`, `Status.Conditions` and `Status.BindingsIngressEndpoint`.
  - Otherwise return the canonical result: the object, or its NotFound error.
  - The legacy branch is a BEGIN/END sentinel block.
  - A legacy NoMatch error is treated like NotFound.

- [ ] **Step 1: Write the failing table test** `TestLookup` using a fake client with both schemes:

  | Case | Expect |
  |---|---|
  | canonical registered only | canonical |
  | canonical unregistered + legacy registered | legacy values (ID, BindingsIngressEndpoint) |
  | legacy only | legacy |
  | neither | `apierrors.IsNotFound` |
  | canonical unregistered, no legacy | canonical with empty ID |
- [ ] **Step 2: Run** `go test ./internal/kubernetesoperator/...`. Expect FAIL.
- [ ] **Step 3: Implement `Lookup`.** Switch `StateChecker.IsDraining` and the forwarder's KO Get to it.
- [ ] **Step 4: Extend `state_test.go`:**
  - `legacy Draining=True, canonical unregistered` → `IsDraining` returns true;
  - `canonical Draining=True` → true.
- [ ] **Step 5: Run** the package tests. Expect PASS. Then commit: "Fall back to the legacy KubernetesOperator until the v1 object registers".

### Task 4: Neutralize the legacy KO object

**Files:**
- Create: `internal/controller/ngrok/kubernetesoperator_legacy.go`, the whole file a sentinel block
- Modify: `internal/controller/ngrok/kubernetesoperator_controller.go`. In `SetupWithManager`, add `Watches(&ngrokv1alpha1.KubernetesOperator{}, handler.EnqueueRequestsFromMapFunc(...))` mapping same namespace/name → canonical request, as a tagged line. `Reconcile` calls the neutralizer.
- Test: `internal/controller/ngrok/kubernetesoperator_legacy_test.go` (envtest)

**Interfaces:**
- Produces:
  - `(r *KubernetesOperatorReconciler) neutralizeLegacy(ctx context.Context, canonical *ngrokv1.KubernetesOperator) error`. It is a no-op if `canonical.DeletionTimestamp` is set.
  - `const AnnotationSupersededBy = "ngrok.com/superseded-by"`.
  - `const ReasonLegacyKubernetesOperatorSuperseded = "LegacyKubernetesOperatorSuperseded"`.
- Behavior:
  - Get the legacy object at the same key. NotFound or NoMatch → nil.
  - Compute the desired state:
    - remove both `util.FinalizerName` and the legacy-prefix finalizer (use the existing helper in `internal/util/k8s.go` that removes both);
    - set the annotation.
  - If nothing changed, return nil.
  - Otherwise send one `client.MergeFrom` patch.
  - If a finalizer was removed, emit a Normal event on the canonical object.
- The watch's predicate must fire on finalizer changes. Legacy events bypass the canonical `kubernetesOperatorReconcilePredicate`, so put an own-name predicate on the `Watches` itself, not the global `WithEventFilter`. Refactor the global filter to a `For` predicate if it would drop legacy events.
- `Reconcile` must call `neutralizeLegacy` even when the canonical reconcile returned a requeue.

- [ ] **Step 1: Write failing envtest specs** in an `Ordered` Describe:
  - **"strips the finalizer from a pre-existing legacy KO without draining"**
    - Setup: create a legacy KO with `util.FinalizerName` and the same name, and give the mock ngrok clientset an existing KO matching deployment name, namespace and namespace UID. Then create the canonical KO.
    - Expect, eventually:
      - the legacy object exists with no finalizers and the annotation `kubernetesoperators.ngrok.com/test-ko`;
      - the canonical `Status.ID` equals the mock KO ID;
      - the mock's KO count is unchanged and there were no delete calls.
  - **"strips a re-added legacy finalizer"**: after the first spec, add the finalizer back to the legacy object and expect it gone again.
  - **"leaves the legacy KO alone while the canonical KO is terminating"**:
    - Use a canonical KO with an extra test finalizer so it lingers, delete it, then add a finalizer to the legacy object.
    - Expect it to still be present after `Consistently` 3s.
    - Cleanup: force-remove both.
  - **"strips the finalizer from a terminating legacy KO"**:
    - Setup: a legacy KO with the finalizer and a `deletionTimestamp` (delete it while it carries the finalizer), then create the canonical KO.
    - Expect: the legacy object is fully gone and the mock shows no KO delete.
- [ ] **Step 2: Run** `go test ./internal/controller/ngrok/... -run 'Ngrok Controller Suite' -ginkgo.focus=legacy`. Expect FAIL.
- [ ] **Step 3: Implement** `neutralizeLegacy`, the watch and the call.
- [ ] **Step 4: Run** the full `./internal/controller/ngrok/...` suite. Expect PASS.
- [ ] **Step 5: Commit**: "Neutralize the legacy KubernetesOperator instead of deleting it".

### Task 5: Skip the drain when the KO CRD is being deleted

**Files:**
- Modify:
  - `internal/controller/ngrok/kubernetesoperator_controller.go`: in `delete()`, before Step 1, add an `APIReader client.Reader` field and a `crdDeletionInProgress(ctx) (bool, error)` method.
  - `cmd/api-manager.go`: `apiextensionsv1.AddToScheme`; pass `mgr.GetAPIReader()`.
  - `internal/controller/ngrok/suite_test.go`: pass `k8sManager.GetAPIReader()`; add apiextensions to the scheme.
- Create: `helm/ngrok-operator/templates/api-manager/crd-read-cluster-role.yaml`, a ClusterRole plus ClusterRoleBinding (`get` on `customresourcedefinitions`, `resourceNames: [kubernetesoperators.ngrok.com]`). Model names and labels on `bindings-cluster-role.yaml`. It always renders, including in namespace-scoped mode.
- Test:
  - `kubernetesoperator_controller_unit_test.go`, for `crdDeletionInProgress` against a fake client;
  - `helm/ngrok-operator/tests/api-manager/` for a new crd-read ClusterRole test file following existing patterns;
  - snapshots.

**Interfaces:**
- Produces:
  - `const KubernetesOperatorCRDName = "kubernetesoperators.ngrok.com"`.
  - `const ReasonCRDDeletionSkippedDrain = "CRDDeletionSkippedDrain"`.
  - `crdDeletionInProgress` returns:

    | CRD state | Result |
    |---|---|
    | NotFound | (true, nil) |
    | has deletionTimestamp | (true, nil) |
    | present, not deleting | (false, nil) |
    | Forbidden | (false, nil) after a logged warning |
    | other error | (false, err) |
  - When it reports true, `delete()` emits the Warning event and returns nil without calling `HandleDrain` or `KubernetesOperators().Delete`. This guard is permanent and NOT sentinel-tagged.

- [ ] **Step 1: Write the failing unit test** `TestCRDDeletionInProgress`, one row per outcome in the table above. Use `interceptor.Funcs` on the fake client to return Forbidden and a generic error.
- [ ] **Step 2: Write the failing unit test** `TestDeleteSkipsDrainWhenCRDDeleting`:
  - Set up a reconciler with a fake client whose CRD has a deletionTimestamp (set finalizers on the fake CRD so it can carry the timestamp), a mock clientset holding the KO, and a `DrainOrchestrator` that records calls. Use the existing unit-test reconciler builder pattern in `kubernetesoperator_controller_unit_test.go`.
  - Expect `delete()` to return nil, the mock KO to still exist, and the drain to never be called.
  - Also write the counterpart, `TestDeleteDrainsWhenCRDPresent`: the existing drain path runs.
- [ ] **Step 3: Run.** Expect FAIL.
- [ ] **Step 4: Implement** the guard, the field, the scheme, and the Helm ClusterRole.
- [ ] **Step 5: Run** `go test ./internal/controller/ngrok/... && make helm-test` (snapshot update via `make helm-update-snapshots` if needed). Expect PASS.
- [ ] **Step 6: Commit**: "Do not drain when the KubernetesOperator CRD itself is being deleted".

### Task 6: Helm RBAC, cleanup hook and crd-access roles

**Files:**
- Modify, adding canonical `ngrok.com` rules and keeping legacy rules inside `# LEGACY-kubernetesoperator-kind: BEGIN/END`:
  - `helm/ngrok-operator/templates/api-manager/release-namespace-role.yaml`: kubernetesoperators create/get/list/patch/update/watch, `/finalizers` patch/update, `/status` get/patch/update.
  - `helm/ngrok-operator/templates/agent/release-namespace-role.yaml`: read.
  - `helm/ngrok-operator/templates/bindings-forwarder/role.yaml`: read.
  - `helm/ngrok-operator/templates/cleanup-hook/rbac.yaml`: get/list/watch/delete.
  - `helm/ngrok-operator/templates/rbac/crd-access/kubernetesoperator-{editor,viewer}.yaml`: same file and role names, add a `ngrok.com` rule, tag the legacy rule.
  - `helm/ngrok-operator/templates/cleanup-hook/job.yaml`:
    1. `kubectl delete kubernetesoperators.ngrok.com "<release>" ... --wait=true --timeout=... || true`
    2. a tagged `kubectl delete kubernetesoperators.ngrok.k8s.ngrok.com "<release>" -n ... --ignore-not-found --wait=false || true`
- Test: `helm/ngrok-operator/tests/rbac/crd-access_test.yaml` (assert both groups present), the cleanup-hook tests if present, and snapshots.

- [ ] **Step 1: Add the helm-unittest asserts:**
  - the editor role contains a rule with `apiGroups: [ngrok.com]` and `resources: [kubernetesoperators]`;
  - the job command contains `kubernetesoperators.ngrok.com`.
- [ ] **Step 2: Run** `make helm-test`. Expect FAIL.
- [ ] **Step 3: Edit the templates.**
- [ ] **Step 4: Run** `make helm-update-snapshots && make helm-test && make manifest-bundle`. Expect PASS.
- [ ] **Step 5: Commit**: "Grant RBAC for ngrok.com KubernetesOperator and qualify the cleanup hook".

### Task 7: Chainsaw and CI references

**Files:**
- Modify:
  - `tests/chainsaw-uninstall/*/chainsaw-test.yaml`: every KO `apiVersion: ngrok.k8s.ngrok.com/v1alpha1` becomes `ngrok.com/v1`, and every bare `kubectl get/delete kubernetesoperator` becomes `kubernetesoperators.ngrok.com`.
  - `tests/chainsaw/operator-registration/*` and `tests/chainsaw-multi-ns/operator-registration/*`.
  - `.github/workflows/ci.yaml`: the bare delete near line 239.

- [ ] **Step 1: Edit** the files above. Then run `git grep -nE 'kubectl (get|delete|wait)[^|]*kubernetesoperator[^s.]|kind: KubernetesOperator' tests .github`, check every remaining hit, and confirm each one is qualified or paired with `apiVersion: ngrok.com/v1`.
- [ ] **Step 2: Run** `make lint`, which includes yamllint. Expect PASS.
- [ ] **Step 3: Commit**: "Point chainsaw and CI at ngrok.com/v1 KubernetesOperator".

### Task 8: Docs

**Files:**
- Modify:
  - `docs/developer-guide/passivity-shims.md`:
    - add `LEGACY-kubernetesoperator-kind` to the sentinel list;
    - add a new catalog section "KubernetesOperator kind + group (operator-written)" covering the R1 mechanics, why no delete, the CRD-deletion guard (permanent), and the R-cleanup sweep list from the spec;
    - note that `LEGACY-metadata-format` must stay.
  - `docs/upgrading-to-0.25.md`:
    - the "Upgrade the CRDs before the operator" section mentions `kubernetesoperators.ngrok.com`;
    - "Custom RBAC" lists the new rules and the new CRD-read ClusterRole;
    - a new section, "KubernetesOperator moves to `ngrok.com/v1`", saying no action is needed, what you'll see (the legacy object annotated and inert), and how to query (`kubectl get kubernetesoperators.ngrok.com -A`);
    - the "Rolling back" section covers the KO behavior and the `installCRDs=false` uninstall-after-rollback manual step from the spec;
    - the "Prepare for later cleanup releases" table gets a row.
  - `specs/migration-v1.md`:
    - fix the group table (CloudEndpoint and AgentEndpoint are in `ngrok.k8s.ngrok.com`; remove `BindingConfiguration`);
    - add a short KO note: migrated automatically.
  - `specs/crds/kubernetesoperator.md`: the group, version and shape changes.

- [ ] **Step 1: Write** the doc edits.
- [ ] **Step 2: Commit**: "Document the KubernetesOperator move to ngrok.com/v1".

### Task 9: Whole-branch verification

- [ ] **Step 1:** Run `make generate manifests manifest-bundle`, then check that `git status` is clean.
- [ ] **Step 2:** Run `make lint && make go-fix`, then check that `git status` is clean.
- [ ] **Step 3:** Run `make test && make helm-test`. Expect PASS.
- [ ] **Step 4: kind upgrade, rollback and uninstall walk** per the spec's Verification section, if a cluster and credentials are available. Record:
  - the KO ID before, after the upgrade, and after the rollback;
  - the endpoint IDs;
  - the `tls.crt` hash;
  - drain log lines.

  If they are not available, say so explicitly in the handoff.
- [ ] **Step 5:** Fresh-reviewer pass over the whole branch diff against the spec.
