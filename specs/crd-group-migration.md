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

All kinds move together in each release.

| Release | Version | Behavior |
|---------|---------|----------|
| R1 | 0.25 | Serve both kinds. v1 adopts the legacy object's ID and wins. Legacy finalizers stay. New v1 CRDs carry `helm.sh/resource-policy: keep`. |
| R2 | 1.0 | Legacy kinds are unsupported. The only legacy work left is stripping ngrok finalizers from every legacy object. Legacy CRDs stay installed (opt-in removal). The keep annotation is removed. |
| R3 | 1.1 | Legacy CRDs, the opt-in value, and the finalizer sweeper are deleted. |

Users cannot skip R2: going from R1 straight to R3 leaves legacy CRDs stuck
`Terminating` on finalizers. 1.0 is already a natural required stop, and 1.1
release notes say "upgrade through 1.0".

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
| IPPolicy | `ingress.k8s.ngrok.com` | User | IP policy + rules | Traffic policies reference the `ipp_...` ID directly. The ID must survive; never auto-create a replacement. |
| Domain | `ingress.k8s.ngrok.com` | User and operator (`ensureDomains`) | Reserved domain | Re-reserving finds the existing domain, so duplicates are harmless. |
| CloudEndpoint | `ngrok.k8s.ngrok.com` | User and operator (Ingress, Gateway, Service) | Cloud endpoint | Pooling allows several objects on one URL. Each object is billed. |
| AgentEndpoint | `ngrok.k8s.ngrok.com` | User and operator | Agent tunnel | Same pooling and billing concern. |

**Out of scope:** BoundEndpoint and BindingConfiguration. Private dial
replaces bindings, so they are dropped rather than moved.

## Twin rules (R1)

A legacy object and a v1 object of the same kind with the same namespace/name
are **twins**. Every kind's R1 controller follows these rules. Divergence
between kinds should be a deliberate, documented exception.

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
5. **Delete remote state only when no live twin holds it.** Before an
   irreversible delete, re-read the twin through the uncached `APIReader`. The
   two kinds use separate informers, so the cache can be stale.
6. **Deleting v1 hands control back to legacy.** Legacy becomes the writer
   again on the next pass.
7. **R1 never deletes legacy objects**, including operator-created ones. After
   `helm rollback` to 0.24, the old operator needs the legacy object with its
   ID intact. Without it, 0.24 creates a second endpoint: a duplicate in the
   pool and on the bill.
8. **Legacy finalizers stay in R1.** R2 strips them all. This keeps R1 simpler:
   no re-stamp logic when v1 is deleted.
9. **Drain deletes legacy before v1.** For a pair that shares an ID, the
   legacy delete skips the remote resource (rule 5) and the v1 delete removes
   it once.

### Per-kind application

- **IPPolicy:** Jack's stack (#913-#919) implements rules 1-9. It keeps
  registering legacy finalizers on passive objects, consistent with rule 8.
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
- **KubernetesOperator:** Done on branch
  `alex/k8sop-318-move-kubernetesoperator-from-v1alpha-to-v1-and-migrate-api`.
  The ID is kept by `findExisting` matching, the drain is skipped while a
  legacy twin exists, and the uninstall hook deletes both kinds by fully
  qualified name. The branch currently strips the legacy finalizer in R1; it
  must conform to rule 8.

## R2 (1.0)

- **Pair reconcilers are deleted.** v1 controllers go back to plain
  single-kind controllers on `BaseController`.
- **Legacy finalizer sweeper.** One kind-agnostic controller in api-manager:
  - Watches each legacy GVK as `PartialObjectMetadata` (metadata only), so no
    legacy Go types or scheme registration is needed. The `v1alpha1` API
    packages can be deleted in 1.0.
  - Removes ngrok finalizers under both prefixes. Leaves other finalizers
    alone.
  - Skips a GVK that is not installed (NoMatch) instead of failing.
  - RBAC: list, watch and patch on the legacy groups only.
  - Runs in the binary rather than as a Helm hook. During the rollout, 0.25
    pods can re-add finalizers, and a watch catches that where a one-shot job
    would not. It also covers `installCRDs=false` installs.
- **Unmigrated objects.** A legacy object with no v1 twin becomes inert. Its
  ngrok resource keeps running unmanaged (orphaned). The 1.0 upgrade guide
  says so and includes a command that lists legacy objects with no v1 twin.
- **Opt-in legacy CRD removal.** The `ngrok-crds` chart gets a value
  (e.g. `legacy.install`, default `true`). Once 1.0 is running and no legacy
  object carries an ngrok finalizer, users can set it to `false` and upgrade.
  Helm then deletes the legacy CRDs and they stay deleted. A manual
  `kubectl delete crd` is not durable for Helm-managed CRDs, so the guide
  points chart users at the value. `installCRDs=false` users delete the CRDs
  directly.

### Idea, not committed: R2 upgrade guard

A pre-upgrade check that refuses to upgrade to 1.0 while stateful legacy
objects (IPPolicy, user Domain, user endpoints) have no v1 twin. Loud failure
instead of a silent orphan. Documenting the orphan behavior is the current
plan.

## R3 (1.1)

- Delete the legacy CRD templates, the `legacy.install` value, the finalizer
  sweeper, and the legacy RBAC.
- Any legacy object still stored is removed with its CRD. Because R2 stripped
  the finalizers, deletion completes and no ngrok API call is made.
- Optional: a chart `fail` when upgrading from a version older than 1.0.

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

## Helm rollback

- **R1 → 0.24:** The v1 CRDs carry the keep annotation, so they and their
  objects survive. 0.24 serves the legacy objects, which still hold their IDs
  (rule 7).
- **Rollback then uninstall:** May leave a v1 CRD and a `Terminating` object
  that needs a manual finalizer patch. Accepted: rare and easy to clean up.
- **R2 → R1:** Legacy CRDs are still installed. R1 resumes dual-read, and v1
  objects win.

## Verification per kind PR

- Envtest or unit coverage for each twin state: legacy only, v1 only, both
  with the same ID, both with different IDs, and deletes of each half.
- Live upgrade in kind: 0.24 → R1 with real ngrok resources. Confirm remote
  IDs are unchanged, requests continuously succeed through migration (apply
  v1, delete legacy), and no duplicate endpoint appears in the ngrok API.
- Rollback R1 → 0.24: legacy objects serve with the same IDs, and no duplicate
  remote resources appear.
- Uninstall drain removes every remote resource exactly once.
