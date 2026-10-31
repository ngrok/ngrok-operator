# CRD Group Migration

Internal plan for moving every operator CRD from the `v1alpha1` groups
(`ngrok.k8s.ngrok.com`, `ingress.k8s.ngrok.com`) to `ngrok.com/v1` without
downtime and without manual pre-upgrade steps. The user-facing upgrade guide
is [migration-v1.md](migration-v1.md).

## Why this needs a plan

- **No conversion webhook.** A webhook converts versions within one CRD. It
  cannot cross groups, so a legacy object and its `ngrok.com/v1` replacement
  are two objects with separate storage.
- **User manifests live in user repos.** Nothing the operator does removes the
  need for users to re-stamp `apiVersion` once. The plan keeps that the only
  required user action, done on the user's schedule.
- **Legacy finalizers block CRD removal.** Every legacy finalizer must be gone
  before the chart drops a legacy CRD.
- **Kinds own remote state.** Deleting a legacy object must not delete an ngrok
  resource that its v1 replacement now serves.

## Release plan

All kinds move together in each release, TrafficPolicy included.

| Release | Version | Behavior |
|---------|---------|----------|
| R1 | 0.25 | Serve both kinds. v1 adopts the legacy object's ID and wins. Legacy finalizers stay. New v1 CRDs carry `helm.sh/resource-policy: keep`. |
| R2 | 1.0 | Legacy kinds are unsupported. Legacy work left: strip ngrok finalizers from every legacy object, and one-way ID adoption for late re-stamps. Legacy CRDs stay installed (opt-in removal). The keep annotation is removed. |
| R3 | 1.1 | Legacy CRDs, the opt-in value, the finalizer sweeper, and late adoption are deleted. |

### Required stops

**0.25 and 1.0 are both required stops.** Skipping either breaks things:

| Skip | Result |
|------|--------|
| 0.24 → 1.0 | No adoption pass ran while both kinds were served. Second KubernetesOperator registration, new IPPolicy IDs (breaking traffic policies that reference the old ones), duplicate endpoints. |
| 0.25 → 1.1 | The same race the three-release plan avoids: 0.25 pods still running process the CRD-driven deletes and remove the ngrok resources behind unmigrated objects. Finalizers on everything else hold the legacy CRDs `Terminating`. |

The chart enforces both stops with a `fail` when the installed version is
outside the allowed range. The template reads the installed chart version with
`lookup` on the existing api-manager Deployment's `helm.sh/chart` label.
`lookup` returns nothing during `helm template` and dry runs, so the guard is
skipped there. The exact mechanism is up to the implementing PR. Release notes
for 1.0 and 1.1 also name the required stop.

### Why three releases and not two

Merging R2 and R3 (1.0 drops the legacy CRDs) is possible, but during the
rollout the old 0.25 pods also see the CRD-driven deletes. For a legacy object
the user never migrated, 0.25 runs its normal delete and removes the ngrok
resource. With three releases the same object ends up orphaned and still
serving. R3 is a chart-only change, so the extra release costs almost nothing.

Merging R1 and R2 is not possible: zero downtime with no manual step needs one
release that serves both kinds while users re-stamp manifests.

## Kinds in scope

| Kind | Legacy group | Created by | Remote state | Notes |
|------|--------------|------------|--------------|-------|
| TrafficPolicy (was NgrokTrafficPolicy) | `ngrok.k8s.ngrok.com` | User | None | Done (#875). Different plural, so no twin key collision. |
| KubernetesOperator | `ngrok.k8s.ngrok.com` | Operator | KO registration | Operator-only. No user action. |
| IPPolicy | `ingress.k8s.ngrok.com` | User | IP policy + rules | Traffic policies reference the `ipp_...` ID directly. The ID must survive through 1.0; never auto-create a replacement. |
| Domain | `ingress.k8s.ngrok.com` | User and operator (`ensureDomains`) | Reserved domain | Re-reserving finds the existing domain, so duplicates are harmless. |
| CloudEndpoint | `ngrok.k8s.ngrok.com` | User and operator (Ingress, Gateway, Service) | Cloud endpoint | Pooling allows several objects on one URL. Each object is billed. |
| AgentEndpoint | `ngrok.k8s.ngrok.com` | User and operator | Agent tunnel | Same pooling and billing concern. |

**Out of scope:** BoundEndpoint and BindingConfiguration. Private dial
replaces bindings, so they are dropped rather than moved.

## Twin rules (R1)

A legacy object and a v1 object of the same kind with the same namespace/name
are **twins**. Every kind's R1 controller follows these rules. Divergence
between kinds must be listed under [Exceptions](#exceptions).

1. **Twin key is namespace/name.** Never the URL: pooling lets distinct objects
   share a URL legitimately, and each of those must keep its own endpoint.
2. **One reconciler per kind** watches both kinds and enqueues the same
   namespace/name key. A single workqueue key means controller-runtime never
   reconciles both halves of a pair concurrently. Two controllers would each
   read the twin from their own cache and race on irreversible steps.
3. **v1 adopts the legacy ID first.** If v1 has no ID and legacy has one, v1
   takes it and persists status before any other step.
4. **One writer per pair.** When both exist, only v1 calls the ngrok API or
   the agent driver. Legacy is passive: no API calls, a `SupersededByV1`
   reason, and a `DeprecatedAPIGroup` warning event.
5. **Delete remote state only when no live twin holds it, and fail closed.**
   Before an irreversible delete, re-read the twin through the uncached
   `APIReader`. The two kinds use separate informers, so the cache can be
   stale. Interpret the read strictly:
   - NotFound, or the twin's kind is not served (NoMatch): no twin. Proceed.
   - Forbidden: treat the twin as present. Skip the delete. On rollback, RBAC
     reverts before the CRDs do, so this happens in practice.
   - Any other error: requeue.
6. **Deleting v1 hands control back to legacy.** Legacy becomes the writer
   again on the next pass.
7. **R1 never deletes legacy objects**, including operator-created ones. After
   `helm rollback` to 0.24, the old operator needs the legacy object with its
   ID intact. Without it, 0.24 creates a second endpoint: a duplicate in the
   pool and on the bill.
8. **Legacy finalizers stay in R1.** R2 strips them all. This keeps R1 simpler:
   no re-stamp logic when v1 is deleted.
9. **Deleting a twinned legacy object releases its finalizer without an ngrok
   call.** The user migration path is "apply v1, then delete legacy". When a
   live v1 twin holds the ID (rule 5), the R1 controller removes the legacy
   finalizer and leaves the remote resource alone. Otherwise the delete hangs.
10. **Drain deletes legacy before v1.** For a pair that shares an ID, the
    legacy delete skips the remote resource (rules 5 and 9) and the v1 delete
    removes it once.

### Exceptions

- **KubernetesOperator, rule 6.** The operator creates its KubernetesOperator
  only at startup, so nothing recreates or hands control back mid-run.
  Deleting the v1 object while a legacy twin exists skips the drain and the
  ngrok delete, releases the v1 finalizer, and emits `LegacyTwinSkippedDrain`.
- **KubernetesOperator, rule 10.** Deleting the KubernetesOperator is what
  starts the drain, so the general drain ordering does not apply to it.

### Per-kind application

- **IPPolicy:** Jack's stack (#913-#919) implements the rules. It keeps
  registering legacy finalizers on passive objects, consistent with rule 8.
  Check rule 5 handles Forbidden as "twin present".
- **Domain:** `ensureDomains` creates a v1 Domain only when no legacy Domain
  with that name exists, or when the existing legacy Domain is controlled by
  the operator. A user-created legacy Domain may carry custom fields
  (description, `resolvesTo`), so the user re-stamps it. If the user deletes
  the legacy Domain without creating a v1 one, or if R2 leaves it inert,
  `ensureDomains` then creates the default v1 Domain.
- **CloudEndpoint / AgentEndpoint, operator-generated:** The generator writes
  v1 objects with the same name. Rules 3-4 hand ownership over. The legacy
  object stays (rule 7) until R2 makes it inert. Users do nothing.
- **AgentEndpoint handoff:** The agent driver keys tunnels by `namespace/name`
  (`statusID`, `internal/controller/agent/agent_endpoint_controller.go`).
  `CreateAgentEndpoint` is a no-op when the stored config equals the new one
  (`pkg/agent/driver.go`). A twin with an identical spec therefore reuses the
  live tunnel. A differing spec rebinds make-before-break (pooled bind first,
  then close). Rule 4 must hold, or two objects with different specs flap the
  same slot. Rule 5 must hold, or deleting legacy closes the tunnel v1 serves
  through. The v1 spec must reach the driver as the type the driver takes.
- **KubernetesOperator:** Branch
  `alex/k8sop-318-move-kubernetesoperator-from-v1alpha-to-v1-and-migrate-api`.
  The ID is kept by `findExisting` matching. The uninstall hook deletes both
  kinds by fully qualified name. Per rule 8 the branch keeps the legacy
  finalizer and drops the `ngrok.com/superseded-by` annotation. See
  [Exceptions](#exceptions) for rules 6 and 10.

## R2 (1.0)

- **Pair reconcilers are deleted.** v1 controllers go back to plain
  single-kind controllers on `BaseController`.
- **Late ID adoption (IPPolicy, CloudEndpoint).** A user who re-stamps after
  upgrading to 1.0 must keep the ID, or traffic policies that reference an
  `ipp_...` ID break and endpoints duplicate. When a v1 object has no ID, its
  controller reads the same-named legacy object as `unstructured` (no Go types
  needed) and adopts `status.id` if set. One direction only: no writer
  handoff, no delete logic, no finalizer work. This also recovers resources
  orphaned by "Unmigrated objects" below. Domain needs no adoption (re-reserve
  is idempotent), and AgentEndpoint has no remote ID.
- **Legacy finalizer sweeper.** One kind-agnostic controller in api-manager:
  - Watches each legacy GVK as `PartialObjectMetadata` (metadata only), so no
    legacy Go types or scheme registration is needed. The `v1alpha1` API
    packages can be deleted in 1.0.
  - Removes ngrok finalizers under both prefixes. Leaves other finalizers
    alone.
  - Skips a GVK that is not installed (NoMatch) instead of failing.
  - RBAC: list, watch and patch on the legacy groups, plus get on the
    legacy IPPolicy and CloudEndpoint for late adoption.
  - Runs in the binary rather than as a Helm hook. During the rollout, 0.25
    pods can re-add finalizers, and a watch catches that where a one-shot job
    would not. It also covers `installCRDs=false` installs.
- **Unmigrated objects.** A legacy object with no v1 twin becomes inert. Its
  ngrok resource keeps running unmanaged (orphaned) until the user re-stamps
  it, at which point late adoption takes it back. The 1.0 upgrade guide says
  so, includes a command that lists legacy objects with no v1 twin, and says
  to migrate before 1.1: a re-stamp after 1.1 creates a new ID.
- **Opt-in legacy CRD removal.** The `ngrok-crds` subchart gets a value,
  set from the operator chart as `ngrok-crds.legacy.install` (default
  `true`). Once 1.0 is running and no legacy object carries an ngrok
  finalizer, users can set it to `false` and upgrade. Helm then deletes the
  legacy CRDs and they stay deleted. A manual `kubectl delete crd` is not
  durable for Helm-managed CRDs, so the guide points chart users at the value.
  `installCRDs=false` users delete the CRDs directly. Opting in also ends late
  adoption for that install.

### Idea, not committed: unmigrated-object guard

A pre-upgrade check that refuses to upgrade to 1.1 while stateful legacy
objects (IPPolicy, user Domain, user endpoints) have no v1 twin. Loud failure
instead of losing the ID. Late adoption in 1.0 makes this less urgent;
documenting it is the current plan.

## R3 (1.1)

- Delete the legacy CRD templates, the `legacy.install` value, the finalizer
  sweeper, late adoption, and the legacy RBAC.
- Any legacy object still stored is removed with its CRD. Because R2 stripped
  the finalizers, deletion completes and no ngrok API call is made.
- The chart refuses upgrades from versions older than 1.0 (see
  [Required stops](#required-stops)).

## kubectl plural ambiguity

Both CRDs of a moved kind share a plural (`domains`, `ippolicies`, ...).
`kubectl get <plural>` resolves to one group by discovery order. Verified on a
1.35 kind cluster with kubectl 1.37, using throwaway CRDs in the real groups:

| Groups defining the plural | `kubectl get <plural>` resolves to |
|----------------------------|------------------------------------|
| `ngrok.com` and `ngrok.k8s.ngrok.com` | `ngrok.com` (v1) |
| `ngrok.com` and `ingress.k8s.ngrok.com` | `ingress.k8s.ngrok.com` (legacy) |

So **Domain and IPPolicy resolve to legacy** while both CRDs exist.
KubernetesOperator, CloudEndpoint and AgentEndpoint resolve to v1.

- Docs, examples, hooks and support runbooks use fully qualified names
  (`domains.ngrok.com`, `ippolicies.ngrok.com`) from R1 until R3.
- In 1.0 the legacy list is inert but still shown by `kubectl get domains`.
  The opt-in CRD removal fixes this early for users who care.
- Older releases are affected too: the 0.24.2 cleanup hook runs
  `kubectl delete kubernetesoperator <release>` with the bare name, which
  resolves to `ngrok.com` once the v1 CRD exists. See
  [Helm rollback](#helm-rollback).

## Helm rollback

Accepted as rare. The plan does not add code for these cases, but the upgrade
guide documents them.

- **R1 → 0.24:** The v1 CRDs carry the keep annotation, so they and their
  objects survive. 0.24 serves the legacy objects, which still hold their IDs
  (rule 7).
- **v1-only objects after R1 → 0.24.** Objects created after the upgrade have
  no legacy twin, and 0.24 ignores them:
  - Their AgentEndpoint tunnels go down. Their CloudEndpoints keep running
    unmanaged.
  - Deleting one while on 0.24 hangs on its finalizer.
  - An Ingress, Gateway or Service created after the upgrade only had a v1
    CloudEndpoint. 0.24 regenerates a legacy one with no ID, which creates a
    second endpoint next to the unmanaged v1 one: a duplicate in the pool and
    on the bill.
- **Rollback, then uninstall.** Worse than one stuck object:
  - The 0.24.2 hook deletes `kubernetesoperator <release>` by bare name, which
    resolves to `ngrok.com`. The 0.24 hook role cannot touch that group, gets
    Forbidden, and nothing drains.
  - Drain is what removes finalizers from managed objects. Without it, every
    legacy object that still has a finalizer is stuck `Terminating` when Helm
    deletes the legacy CRDs. The v1 CRDs and objects stay (keep annotation).
  - With the default `drainPolicy: Retain`, ngrok resources are kept anyway.
    With `Delete`, they are orphaned.
  - **Avoid it:** before uninstalling a rolled-back release, run
    `kubectl delete kubernetesoperators.ngrok.k8s.ngrok.com <release> -n <ns>`.
    0.24 then drains normally.
  - **Clean up after it:** patch `metadata.finalizers` to `null` on every
    stuck legacy object. Then do the same for every v1 object and delete the
    leftover v1 CRDs, which uninstall leaves behind because of the keep
    annotation.
- **`ngrok-crds` installed as its own Helm release.** Uninstalling that
  release in R1 leaves the `ngrok.com` CRDs behind, because of the keep
  annotation. Delete them by hand.
- **R2 → R1:** Legacy CRDs are still installed. R1 resumes dual-read, and v1
  objects win.

## Verification per kind PR

- Envtest or unit coverage for each twin state: legacy only, v1 only, both
  with the same ID, both with different IDs, and deletes of each half.
- Rule 5 coverage: a twin read that returns Forbidden skips the delete; any
  other error requeues.
- Live upgrade in kind: 0.24 → R1 with real ngrok resources. Confirm remote
  IDs are unchanged, requests continuously succeed through migration (apply
  v1, delete legacy), and no duplicate endpoint appears in the ngrok API.
- Leader election ID is identical in 0.24 and R1 (`<fullname>-leader` today),
  so the two api-managers never both write during the rolling update.
- Rollback R1 → 0.24: legacy objects serve with the same IDs, and no duplicate
  remote resources appear.
- Uninstall drain removes every remote resource exactly once.
- R2 late adoption: a legacy IPPolicy re-stamped after upgrading to 1.0 keeps
  its `ipp_...` ID.
