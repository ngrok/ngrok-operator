# Resurrect Research: ClusterRole → Role RBAC Refactor

## Your Context
You were working on changing the K8s operator ClusterRoles to flex to regular Roles when watching a single namespace. This expanded into a broader refactoring and simplification of the RBAC setup across all three operator components (api-manager, agent-manager, bindings-forwarder), which had accumulated inconsistencies.

## Branch Summary

| Branch | Merge Base | Date | Commits | Files Changed | Lines | Status |
|---|---|---|---|---|---|---|
| `alex/cluster-role-and-rbac-cleanup` | `aac95968` | Mar 4-5, 2026 | 1 | 52 | +2750/-1201 | Partial: Phases 1 & 3 done, 2/4/5 deferred |
| `alex/cluster-role-and-rbac-refactor` | `aac95968` | Mar 5, 2026 | 1 | 82 | +2265/-1669 | More complete: all phases attempted in one commit |

Both branches fork from the same merge base (`aac95968` — "Make the driver seed function respect the watch namespace filter (#771)") and are **19 commits behind `origin/main`**.

---

## Branch: alex/cluster-role-and-rbac-cleanup

### Overview
This appears to be the **first attempt**. It takes a more incremental approach — completing phases 1 (quick wins) and 3 (per-deployment controller-gen RBAC), while explicitly deferring phases 2, 4, and 5 for later PRs.

Key architectural decisions:
- **Keeps controller-gen** for RBAC generation but splits it into per-deployment invocations (api-manager, agent-manager separate)
- **Moves forwarder controller** into a `forwarder` subpackage (`internal/controller/bindings/forwarder/`)
- **Adds kubebuilder:rbac markers** to controllers that were missing them (agent endpoint, forwarder, kubernetes operator)
- **Introduces data files** (`files/rbac/`) for controller-gen output, read by templates
- **Reorganizes RBAC templates** under `templates/rbac/` with per-component files (agent.yaml, api-manager.yaml, bindings-forwarder.yaml)
- CRD access roles moved to `templates/rbac/crd-access/` subdirectory (not yet to CRDs chart)

### Plan Documents

<details>
<summary>plan.md (from alex/cluster-role-and-rbac-cleanup)</summary>

# RBAC Cleanup Plan (Completed)

## Current State Analysis

### The 3 Deployments and Their ServiceAccounts

| Deployment | ServiceAccount | Helm SA Template | Manager Command |
|---|---|---|---|
| `*-manager` (api-manager) | `ngrok-operator.serviceAccountName` | `controller-serviceaccount.yaml` | `api-manager` |
| `*-agent` (agent-manager) | `ngrok-operator.agent.serviceAccountName` | `agent/service-account.yaml` | `agent-manager` |
| `*-bindings-forwarder` | `ngrok-operator.bindings.forwarder.serviceAccountName` | `bindings-forwarder/service-account.yaml` | `bindings-forwarder-manager` |

### RBAC File Inventory

| File | Manages | Source | Bound To |
|---|---|---|---|
| `files/rbac/role.yaml` | Data file: all `manager-role` rules | **Auto-generated** by `controller-gen` via `make manifests` | Read by `templates/rbac/role.yaml` |
| `templates/rbac/role.yaml` | `manager-role` ClusterRole (or Role+ClusterRole when namespaced) | **Template** (reads from data file) | api-manager SA |
| `templates/controller-rbac.yaml` | leader-election Role+RoleBinding, proxy ClusterRole+ClusterRoleBinding, manager RoleBinding/ClusterRoleBinding, bindings secret-manager Role+RoleBinding | **Hand-managed** | api-manager SA |
| `templates/agent/rbac.yaml` | agent ClusterRole (or Role) + ClusterRoleBinding (or RoleBinding) | **Hand-managed** | agent-manager SA |
| `templates/bindings-forwarder/rbac.yaml` | bindings-forwarder Role + RoleBinding | **Hand-managed** | bindings-forwarder SA |
| `templates/cleanup-hook/rbac.yaml` | cleanup SA + Role + RoleBinding (helm pre-delete hook) | **Hand-managed** | cleanup SA |
| `templates/rbac/*_editor_role.yaml` (x6) | End-user ClusterRoles for editing CRDs | **Originally controller-gen, now hand-managed** | Not bound to any SA (for end-users to bind) |
| `templates/rbac/*_viewer_role.yaml` (x6) | End-user ClusterRoles for viewing CRDs | **Originally controller-gen, now hand-managed** | Not bound to any SA (for end-users to bind) |

### Go kubebuilder:rbac Annotations → Generated `files/rbac/role.yaml`

All `+kubebuilder:rbac` markers across these files get merged into a **single** `manager-role` ClusterRole:

| Go File | Controller | Resources |
|---|---|---|
| `internal/controller/ingress/ingress_controller.go` | Ingress | events, configmaps, secrets, services, ingresses, ingresses/status, ingressclasses, ngroktrafficpolicies |
| `internal/controller/ingress/domain_controller.go` | Domain | domains, domains/status, domains/finalizers |
| `internal/controller/ingress/ippolicy_controller.go` | IPPolicy | ippolicies, ippolicies/status, ippolicies/finalizers |
| `internal/controller/ngrok/cloudendpoint_controller.go` | CloudEndpoint | cloudendpoints, cloudendpoints/status, cloudendpoints/finalizers |
| `internal/controller/ngrok/ngroktrafficpolicy_controller.go` | NgrokTrafficPolicy | ngroktrafficpolicies, ngroktrafficpolicies/status, ngroktrafficpolicies/finalizers |
| `internal/controller/ngrok/kubernetesoperator_controller.go` | KubernetesOperator | kubernetesoperators, kubernetesoperators/status, kubernetesoperators/finalizers |
| `internal/controller/agent/agent_endpoint_controller.go` | AgentEndpoint | agentendpoints, agentendpoints/status, agentendpoints/finalizers, ngroktrafficpolicies, domains, secrets |
| `internal/controller/service/controller.go` | Service | events, configmaps, secrets, services, services/status, ngroktrafficpolicies |
| `internal/controller/gateway/gatewayclass_controller.go` | GatewayClass | gatewayclasses, gatewayclasses/status, gatewayclasses/finalizers, gateways |
| `internal/controller/gateway/gateway_controller.go` | Gateway | events, configmaps, secrets, services, gateways, gateways/status, gatewayclasses, gatewayclasses/status |
| `internal/controller/gateway/httproute_controller.go` | HTTPRoute | httproutes, httproutes/status, namespaces |
| `internal/controller/gateway/tcproute_controller.go` | TCPRoute | tcproutes, tcproutes/status |
| `internal/controller/gateway/tlsroute_controller.go` | TLSRoute | tlsroutes, tlsroutes/status |
| `internal/controller/gateway/namespace_controller.go` | Namespace | namespaces |
| `internal/controller/gateway/referencegrant_controller.go` | ReferenceGrant | referencegrants |
| `internal/controller/bindings/boundendpoint_controller.go` | BoundEndpoint | boundendpoints, boundendpoints/status, boundendpoints/finalizers |
| `internal/drain/drain.go` | Drainer | cloudendpoints, agentendpoints, domains, ippolicies, boundendpoints, ingresses, ingressclasses, services, gateways, gatewayclasses, httproutes, tcproutes, tlsroutes |

**Key insight**: controller-gen merges ALL markers into one role. It does not distinguish which deployment needs which permissions. The generated `manager-role` is a superset of all controllers' needs, but only the api-manager SA gets bound to it.

### What Each Deployment Actually Needs

**api-manager** (runs most controllers):
- All controllers in `ingress/`, `ngrok/`, `gateway/`, `service/`, `bindings/` (BoundEndpoint only)
- Drain logic
- Leader election (configmaps, leases)
- → Needs: everything in the generated `manager-role` + leader-election permissions

**agent-manager** (runs only AgentEndpointReconciler):
- agentendpoints + status + finalizers
- ngroktrafficpolicies (read)
- domains (CRUD for auto-creating domains)
- secrets (read, for TLS certs)
- events (create/patch)
- kubernetesoperators (read, for drain state checking)
- → Currently hand-managed in `templates/agent/rbac.yaml`

**bindings-forwarder** (runs only ForwarderReconciler):
- boundendpoints (read + update)
- events (create/patch)
- kubernetesoperators (read, for drain state checking)
- secrets (read, for TLS certs)
- pods (read, for watching pods)
- → Currently hand-managed in `templates/bindings-forwarder/rbac.yaml`

---

## Issues Found

### 1. Stale/Dead Editor+Viewer Roles (files that reference non-existent CRDs)

- **`bindingconfiguration_editor_role.yaml`** and **`bindingconfiguration_viewer_role.yaml`**: Reference `ngrok.k8s.ngrok.com/bindingconfigurations` — this CRD does NOT exist anywhere in the `api/` directory. **DELETE these files.**
- **`operatorconfiguration_editor_role.yaml`** and **`operatorconfiguration_viewer_role.yaml`**: Reference `ngrok.k8s.ngrok.com/operatorconfigurations` — this CRD does NOT exist. **DELETE these files.**
- **`boundendpoint_editor_role.yaml`** and **`boundendpoint_viewer_role.yaml`**: Reference `ngrok.k8s.ngrok.com/boundendpoints` but the actual CRD uses `bindings.k8s.ngrok.com/boundendpoints`. **Wrong API group — fix or delete.**

### 2. Stale Permissions in `agent/rbac.yaml`

- References `ingress.k8s.ngrok.com/tunnels` — the Tunnel CRD no longer exists (`api/ingress/` has no Tunnel type, no controller references it). **These rules are dead and should be removed.**

### 3. Duplicate Event Rules in `bindings-forwarder/rbac.yaml`

- The `events` create/patch rule appears twice (lines 21-27 and lines 44-50). **Remove the duplicate.**

### 4. Generated `manager-role` Is a Superset — But Mostly Justified

The generated `files/rbac/role.yaml` contains permissions for ALL controllers, but it's only bound to the api-manager ServiceAccount. The api-manager legitimately needs most of these because:
- The Driver aggregates Ingress/Gateway/Service resources and creates downstream CRDs (CloudEndpoints, AgentEndpoints, Domains)
- The Drain logic needs broad read/write access to clean up resources
- The BoundEndpoint controller runs inside the api-manager

Some overlap with agent/bindings-forwarder permissions is expected and fine.

### 5. `controller-rbac.yaml` Is Overloaded and Hard to Read

This file contains 4+ unrelated concerns:
- Leader election Role + RoleBinding
- Proxy ClusterRole + ClusterRoleBinding
- Manager role bindings (with namespaced vs cluster-wide branching)
- Secret manager Role + RoleBinding (bindings-only)

### 6. Editor/Viewer Roles: Missing CRDs and Stale CRDs

These are kubebuilder-scaffolded ClusterRoles intended for **end-users** (not the operator itself). They let cluster admins grant users read or write access to specific CRDs via RoleBindings. This is a common kubebuilder pattern — they're not bound to any ServiceAccount by default.

**Actual CRDs that exist:**
- `AgentEndpoint` (ngrok.k8s.ngrok.com)
- `CloudEndpoint` (ngrok.k8s.ngrok.com)
- `KubernetesOperator` (ngrok.k8s.ngrok.com)
- `NgrokTrafficPolicy` (ngrok.k8s.ngrok.com)
- `Domain` (ingress.k8s.ngrok.com)
- `IPPolicy` (ingress.k8s.ngrok.com)
- `BoundEndpoint` (bindings.k8s.ngrok.com)

**Editor/viewer roles that exist:**
| Role file | CRD | Status |
|---|---|---|
| `bindingconfiguration_*` | BindingConfiguration | CRD doesn't exist — DELETE |
| `operatorconfiguration_*` | OperatorConfiguration | CRD doesn't exist — DELETE |
| `boundendpoint_*` | BoundEndpoint | Wrong API group (`ngrok.k8s.ngrok.com` → should be `bindings.k8s.ngrok.com`) |
| `cloudendpoint_*` | CloudEndpoint | Correct |
| `domain_*` | Domain | Correct |
| `ippolicy_*` | IPPolicy | Correct |

**Missing editor/viewer roles (CRDs with no roles):**
- `AgentEndpoint` — **Missing** (users may want to create/view these)
- `KubernetesOperator` — **Missing** (probably intentional — this is operator-internal)
- `NgrokTrafficPolicy` — **Missing** (users definitely want to create/view these)

**Recommendation**: Add `agentendpoint_editor/viewer_role.yaml` and `ngroktrafficpolicy_editor/viewer_role.yaml`. Skip KubernetesOperator (it's an internal singleton). Whether this pattern is worth keeping at all is debatable — many operators don't bother with these roles — but since you already have them for some CRDs, being consistent is better than the current half-state.

---

## Recommendations

### Phase 1: Quick Wins — DONE

1. Deleted 4 stale editor/viewer roles (bindingconfiguration_*, operatorconfiguration_*)
2. Fixed boundendpoint_* API group to `bindings.k8s.ngrok.com`
3. Removed dead `tunnels` rules from `agent/rbac.yaml`
4. Removed duplicate `events` rule from `bindings-forwarder/rbac.yaml`
5. Added missing editor/viewer roles for AgentEndpoint, NgrokTrafficPolicy, KubernetesOperator

### Phase 2: Split `controller-rbac.yaml` (Future — Do as Part of Namespace-Scoped RBAC PR)

Split `controller-rbac.yaml` into separate files organized by concern:

```
templates/controller/
  leader-election-rbac.yaml     # leader-election Role + RoleBinding (always namespaced to release NS)
  proxy-rbac.yaml               # proxy ClusterRole + ClusterRoleBinding (always cluster-scoped)
  manager-rolebinding.yaml      # manager ClusterRoleBinding (cluster-wide mode)
  manager-rolebinding-ns.yaml   # manager Role+ClusterRole bindings (namespace-scoped mode)
  secret-manager-rbac.yaml      # bindings secret-manager Role + RoleBinding (conditional on bindings.enabled)
```

Each file has a simple guard at the top (`{{- if $isNamespaced }}` or `{{- if not $isNamespaced }}`), making the logic flat and readable. No deep if/else nesting.

### Phase 3: Per-Deployment controller-gen RBAC — DONE

Separated `controller-gen` into per-deployment invocations:
- **api-manager**: paths include ingress, ngrok, gateway, service, bindings controllers + drain
- **agent-manager**: paths include only `internal/controller/agent/`
- **bindings-forwarder**: stays hand-managed (ForwarderReconciler has no kubebuilder:rbac markers)

Generated data files: `files/rbac/api-manager-role.yaml` and `files/rbac/agent-manager-role.yaml`

Added missing markers to `agent_endpoint_controller.go` for `events` and `kubernetesoperators` (needed for event recording and DrainState).

### Phase 4: Remove accidental `clusterRole.annotations` from operator roles

The `clusterRole.annotations` Helm value was added in PR #738 specifically for RBAC aggregation on the CRD access roles (editor/viewer roles). It was accidentally also applied to:
- `templates/agent/rbac.yaml` (the agent-manager Role/ClusterRole)
- `templates/controller-rbac.yaml` (the proxy ClusterRole)

These are operator-internal infrastructure roles — aggregation annotations don't belong on them. Remove the `{{- with .Values.clusterRole.annotations }}` blocks from both files.

### Phase 5: Move CRD Access Roles to the CRDs Chart

#### What are CRD access roles?

These are ClusterRoles that grant **end-users** (not the operator) the ability to read or modify CRD instances. They exist as a kubebuilder convention and are commonly used with Kubernetes RBAC aggregation. By adding annotations like `rbac.authorization.k8s.io/aggregate-to-admin: "true"`, these roles automatically merge into the built-in `admin`/`edit`/`view` ClusterRoles. This means users with existing `admin` RoleBindings in a namespace automatically get CRD permissions without needing new bindings.

#### Why move to the CRDs chart?

These roles are conceptually tied to the CRDs, not the operator. A user who installs only the CRDs chart (without the operator) should still be able to grant users permission to view/edit the CRDs. Currently they can't because the access roles live in the operator chart.

#### How Helm subchart value passing works

The operator chart already depends on the CRDs chart:
```yaml
# helm/ngrok-operator/Chart.yaml
dependencies:
- name: ngrok-crds
  repository: file://../ngrok-crds
```

Helm has built-in subchart value passing. Any values nested under the subchart's name in the parent chart's `values.yaml` are automatically forwarded to the subchart.

#### Implementation steps

1. Add `values.yaml` to the CRDs chart with `crdAccessRoles.annotations: {}`
2. Add minimal `_helpers.tpl` to the CRDs chart
3. Move the 14 editor/viewer role files from `helm/ngrok-operator/templates/rbac/` to `helm/ngrok-crds/templates/crd-access-roles/`
4. Update `helm/ngrok-operator/values.yaml` — remove `clusterRole.annotations`, add pass-through value
5. Clean up `templates/rbac/` in the operator chart

</details>

### Key Changes
- **Deleted**: `agent/rbac.yaml`, `bindings-forwarder/rbac.yaml` (replaced by new structure)
- **New data files**: `files/rbac/agent/role.yaml`, `files/rbac/bindings-forwarder/role.yaml` (controller-gen output)
- **New templates**: `templates/rbac/agent.yaml`, `templates/rbac/api-manager.yaml`, `templates/rbac/bindings-forwarder.yaml`
- **New CRD access roles**: agentendpoint, ngroktrafficpolicy editor/viewer
- **Go refactor**: Moved forwarder controller to `internal/controller/bindings/forwarder/` subpackage
- **Added kubebuilder:rbac markers** to agent endpoint controller (events, kubernetesoperators) and forwarder controller
- **Build changes**: Per-deployment controller-gen targets in `generate.mk` and `_common.mk`

### Dead Ends / Abandoned Work
- **Phases 2, 4, 5 not implemented** — explicitly deferred for future PRs
- CRD access roles moved to `crd-access/` subdirectory but NOT yet to the `ngrok-crds` chart (Phase 5)
- Plan mentions moving CRD access roles to CRDs chart for standalone install support — not done

---

## Branch: alex/cluster-role-and-rbac-refactor

### Overview
This appears to be the **second, more ambitious attempt**. It takes the opposite approach from the cleanup branch — going all-in on a single commit that completes all phases. Key architectural differences:

- **Removes controller-gen entirely** for RBAC — hand-manages all roles in Helm templates
- **Removes ALL kubebuilder:rbac markers** from Go code (73 markers across 17 files)
- **Renames controller files** to `api-manager/` directory (full restructure)
- **Does NOT move forwarder** to a subpackage (keeps existing Go structure)
- **Adds watchNamespace helper** to `_helpers.tpl`
- **Renames `clusterRole` → `crdAccessRoles`** in values.yaml

### Plan Documents

<details>
<summary>new-plan.md (from alex/cluster-role-and-rbac-refactor)</summary>

# RBAC Overhaul Plan

## Goal

Support namespace-scoped RBAC (Roles instead of ClusterRoles) when `watchNamespace` is set, and clean up years of accumulated RBAC debt along the way.

---

## Current State (on `main`)

### The 3 Deployments

| Deployment | ServiceAccount | Helm Template Dir |
|---|---|---|
| `*-manager` (api-manager) | `ngrok-operator.serviceAccountName` | `templates/controller-*.yaml` + `templates/rbac/` |
| `*-agent` (agent-manager) | `ngrok-operator.agent.serviceAccountName` | `templates/agent/` |
| `*-bindings-forwarder` | `ngrok-operator.bindings.forwarder.serviceAccountName` | `templates/bindings-forwarder/` |

### Current RBAC File Layout

```
templates/
  controller-rbac.yaml              # KITCHEN SINK: leader-election Role+RoleBinding,
                                    #   proxy-role ClusterRole+ClusterRoleBinding (DEAD),
                                    #   manager ClusterRoleBinding,
                                    #   secret-manager Role+RoleBinding (conditional)

  rbac/
    role.yaml                       # api-manager ClusterRole (auto-generated by controller-gen)
    api-manager-namespaced.yaml     # api-manager Role+ClusterRole for watchNamespace mode
    bindingconfiguration_editor_role.yaml  # STALE: CRD doesn't exist
    bindingconfiguration_viewer_role.yaml  # STALE: CRD doesn't exist
    operatorconfiguration_editor_role.yaml # STALE: CRD doesn't exist
    operatorconfiguration_viewer_role.yaml # STALE: CRD doesn't exist
    boundendpoint_editor_role.yaml  # WRONG API GROUP: says ngrok.k8s.ngrok.com, should be bindings.k8s.ngrok.com
    boundendpoint_viewer_role.yaml  # WRONG API GROUP: same
    cloudendpoint_editor_role.yaml  # OK
    cloudendpoint_viewer_role.yaml  # OK
    domain_editor_role.yaml         # OK
    domain_viewer_role.yaml         # OK
    ippolicy_editor_role.yaml       # OK
    ippolicy_viewer_role.yaml       # OK
    (missing: agentendpoint, ngroktrafficpolicy, kubernetesoperator)

  agent/
    rbac.yaml                       # agent ClusterRole + ClusterRoleBinding
                                    #   contains STALE tunnel rules
                                    #   has clusterRole.annotations (shouldn't)

  bindings-forwarder/
    rbac.yaml                       # bindings-forwarder Role + RoleBinding
                                    #   has DUPLICATE events rule
```

### Problems

1. **`controller-rbac.yaml` is a kitchen sink** — 4 unrelated concerns in one file (leader-election, proxy, manager binding, secret-manager). Hard to read, hard to reason about.

2. **`proxy-role` is dead code** — leftover kubebuilder scaffolding for kube-rbac-proxy. No proxy sidecar exists. The ClusterRole and its ClusterRoleBinding should be deleted.

3. **`secret-manager-role` should be folded into the api-manager role** — the KubernetesOperatorReconciler creates/updates secrets for TLS certs but the api-manager role only has `get;list;watch` for secrets. Add `create;update;patch` to the api-manager role and remove this separate Role+RoleBinding.

4. **`role.yaml` is auto-generated by controller-gen** — it's a single monolithic ClusterRole that merges ALL controllers' kubebuilder:rbac markers. This is the wrong approach for a multi-component operator. We should hand-manage per-component roles and remove the kubebuilder:rbac markers entirely.

5. **`api-manager-namespaced.yaml` already exists on main** but `role.yaml` doesn't have the corresponding non-namespaced guard (`if not $isNamespaced`). They'll both render in default mode, creating duplicate/conflicting roles.

6. **`agent/rbac.yaml` has stale tunnel rules** — the Tunnel CRD was removed long ago. Also has `clusterRole.annotations` which is meant for CRD access roles, not operator-internal roles.

7. **`bindings-forwarder/rbac.yaml` has duplicate events rule** — lines 21-27 and 44-50 are identical.

8. **4 stale editor/viewer roles** — `bindingconfiguration_*` and `operatorconfiguration_*` reference CRDs that don't exist.

9. **2 wrong-API-group editor/viewer roles** — `boundendpoint_*` uses `ngrok.k8s.ngrok.com` but the CRD is in `bindings.k8s.ngrok.com`.

10. **3 missing editor/viewer roles** — no roles for `AgentEndpoint`, `NgrokTrafficPolicy`, `KubernetesOperator`.

11. **`clusterRole.annotations` applied to wrong roles** — it's on `agent/rbac.yaml` (operator-internal) and `controller-rbac.yaml` (proxy-role). Should only be on CRD access editor/viewer roles.

12. **Inconsistent template organization** — api-manager RBAC is scattered across `controller-rbac.yaml`, `rbac/role.yaml`, and `rbac/api-manager-namespaced.yaml`. Agent and bindings-forwarder have their RBAC inside their component directories. No consistent pattern.

13. **No namespace-scoped support for agent-manager** — agent/rbac.yaml always creates a ClusterRole. Should support Role when `watchNamespace` is set.

---

## Design

### Principles

- **One file per Kubernetes resource (or tightly-coupled pair)**. A Role and its RoleBinding can share a file. But leader-election, proxy, and manager bindings should not be in the same file.
- **Plain YAML**. No Helm `dict`/`list` variable tricks. If namespace-scoping requires different resources, use separate files with simple `if`/`if not` guards at the top.
- **Per-component RBAC in one place**. All RBAC for a component lives under its directory. No scattering across `controller-rbac.yaml` and `rbac/role.yaml` and `rbac/api-manager-namespaced.yaml`.
- **Hand-managed roles, no kubebuilder:rbac markers**. Remove all `// +kubebuilder:rbac` annotations from Go code. Remove `controller-gen rbac` from the Makefile. The Helm templates are the single source of truth for RBAC.
- **CRD access roles (editor/viewer)** stay in the operator chart under `templates/rbac/crd-access/`, separate from controller RBAC.

### Target File Layout

```
templates/
  api-manager/
    deployment.yaml                     # renamed from controller-deployment.yaml
    serviceaccount.yaml                 # renamed from controller-serviceaccount.yaml
    configmap.yaml                      # renamed from controller-cm.yaml
    pdb.yaml                            # renamed from controller-pdb.yaml
    role.yaml                           # api-manager ClusterRole (default mode)
    role-namespaced.yaml                # api-manager Role + ClusterRole (watchNamespace mode)
    rolebinding.yaml                    # api-manager ClusterRoleBinding (default mode)
    rolebinding-namespaced.yaml         # api-manager RoleBinding + ClusterRoleBinding (watchNamespace mode)
    leader-election-role.yaml           # leader-election Role + RoleBinding (always namespaced to release NS)

  agent/
    deployment.yaml                     # unchanged
    serviceaccount.yaml                 # renamed from service-account.yaml
    role.yaml                           # agent ClusterRole (default mode)
    role-namespaced.yaml                # agent Role (watchNamespace mode)
    rolebinding.yaml                    # agent ClusterRoleBinding (default mode)
    rolebinding-namespaced.yaml         # agent RoleBinding (watchNamespace mode)

  bindings-forwarder/
    deployment.yaml                     # unchanged
    serviceaccount.yaml                 # renamed from service-account.yaml
    role.yaml                           # bindings-forwarder Role + RoleBinding (always namespaced)

  cleanup-hook/
    job.yaml                            # unchanged
    rbac.yaml                           # unchanged (cleanup SA + Role + RoleBinding)

  rbac/
    crd-access/
      agentendpoint-editor.yaml         # NEW
      agentendpoint-viewer.yaml         # NEW
      boundendpoint-editor.yaml         # FIXED api group
      boundendpoint-viewer.yaml         # FIXED api group
      cloudendpoint-editor.yaml         # existing (reformatted)
      cloudendpoint-viewer.yaml         # existing (reformatted)
      domain-editor.yaml                # existing (reformatted)
      domain-viewer.yaml                # existing (reformatted)
      ippolicy-editor.yaml              # existing (reformatted)
      ippolicy-viewer.yaml              # existing (reformatted)
      kubernetesoperator-editor.yaml    # NEW
      kubernetesoperator-viewer.yaml    # NEW
      ngroktrafficpolicy-editor.yaml    # NEW
      ngroktrafficpolicy-viewer.yaml    # NEW

  _helpers.tpl
  credentials-secret.yaml
  ingress-class.yaml
  NOTES.txt
```

### What Gets Deleted

- `templates/controller-rbac.yaml` — split into individual files under `api-manager/`
- `templates/rbac/role.yaml` — replaced by `api-manager/role.yaml`
- `templates/rbac/api-manager-namespaced.yaml` — replaced by `api-manager/role-namespaced.yaml`
- `templates/rbac/bindingconfiguration_editor_role.yaml` — CRD doesn't exist
- `templates/rbac/bindingconfiguration_viewer_role.yaml` — CRD doesn't exist
- `templates/rbac/operatorconfiguration_editor_role.yaml` — CRD doesn't exist
- `templates/rbac/operatorconfiguration_viewer_role.yaml` — CRD doesn't exist
- `templates/agent/rbac.yaml` — split into individual files under `agent/`
- `templates/bindings-forwarder/rbac.yaml` — replaced by `bindings-forwarder/role.yaml`

### What Gets Removed from Go Code

- All `// +kubebuilder:rbac` markers from 17 Go files (73 marker lines total)
- The `controller-gen rbac:roleName=...` invocations from `tools/make/generate.mk`
- The `API_MANAGER_RBAC_PATHS`, `AGENT_MANAGER_RBAC_PATHS` variables from `tools/make/_common.mk` (these don't exist on main yet, but noting for completeness)

### What Gets Removed from values.yaml

- `clusterRole.annotations` — this value was intended for CRD access roles but got accidentally applied to operator-internal roles too. Replace with a more specific `crdAccessRoles.annotations` value that is only used by the CRD access role templates.

---

## Implementation Phases

All phases are in a single PR. They're ordered to minimize broken intermediate states.

### Phase 1: Delete stale files and fix bugs

No new files, just cleanup:

1. Delete `bindingconfiguration_editor_role.yaml` and `bindingconfiguration_viewer_role.yaml`
2. Delete `operatorconfiguration_editor_role.yaml` and `operatorconfiguration_viewer_role.yaml`
3. Fix `boundendpoint_editor_role.yaml` and `boundendpoint_viewer_role.yaml` — change API group from `ngrok.k8s.ngrok.com` to `bindings.k8s.ngrok.com`
4. Remove stale `tunnels` rules from `agent/rbac.yaml`
5. Remove duplicate `events` rule from `bindings-forwarder/rbac.yaml`
6. Remove `clusterRole.annotations` from `agent/rbac.yaml` and `controller-rbac.yaml` (proxy-role) — these are operator-internal roles
7. Remove `proxy-role` ClusterRole and `proxy-rolebinding` ClusterRoleBinding from `controller-rbac.yaml` — dead kube-rbac-proxy scaffolding
8. Remove `secret-manager-role` Role and RoleBinding from `controller-rbac.yaml` — will fold secret create/update/patch into the api-manager role

### Phase 2: Remove kubebuilder:rbac markers and controller-gen rbac

1. Remove all `// +kubebuilder:rbac` markers from all Go files
2. Remove `controller-gen rbac:roleName=...` invocations from `tools/make/generate.mk` — keep only `controller-gen crd webhook`
3. Remove `API_MANAGER_RBAC_PATHS` and related variables from `tools/make/_common.mk` (if present)

### Phase 3: Reorganize api-manager templates

Move and split the api-manager templates into `templates/api-manager/`:

1. Rename `controller-deployment.yaml` → `api-manager/deployment.yaml`
2. Rename `controller-serviceaccount.yaml` → `api-manager/serviceaccount.yaml`
3. Rename `controller-cm.yaml` → `api-manager/configmap.yaml`
4. Rename `controller-pdb.yaml` → `api-manager/pdb.yaml`
5. Replace `rbac/role.yaml` with `api-manager/role.yaml` — plain YAML ClusterRole with all api-manager rules, guarded by `if not $isNamespaced`. Add secret `create;update;patch` verbs (previously in secret-manager-role).
6. Replace `rbac/api-manager-namespaced.yaml` with `api-manager/role-namespaced.yaml` — Role (namespace-scoped resources) + ClusterRole (cluster-scoped resources), guarded by `if $isNamespaced`
7. Extract manager rolebinding from `controller-rbac.yaml` → `api-manager/rolebinding.yaml` (ClusterRoleBinding, guarded by `if not $isNamespaced`)
8. Create `api-manager/rolebinding-namespaced.yaml` — RoleBinding + ClusterRoleBinding for watchNamespace mode
9. Extract leader-election Role + RoleBinding from `controller-rbac.yaml` → `api-manager/leader-election-role.yaml`
10. Delete `controller-rbac.yaml` (now empty)
11. Update all helm test files and snapshots — test paths, document indices, template references

### Phase 4: Reorganize agent templates

1. Rename `agent/service-account.yaml` → `agent/serviceaccount.yaml`
2. Split `agent/rbac.yaml` into:
   - `agent/role.yaml` — ClusterRole, guarded by `if not $isNamespaced`
   - `agent/role-namespaced.yaml` — Role, guarded by `if $isNamespaced`
   - `agent/rolebinding.yaml` — ClusterRoleBinding, guarded by `if not $isNamespaced`
   - `agent/rolebinding-namespaced.yaml` — RoleBinding, guarded by `if $isNamespaced`
3. Delete `agent/rbac.yaml`
4. Update helm tests

### Phase 5: Reorganize bindings-forwarder templates

1. Rename `bindings-forwarder/service-account.yaml` → `bindings-forwarder/serviceaccount.yaml`
2. Replace `bindings-forwarder/rbac.yaml` with `bindings-forwarder/role.yaml` — keep as namespaced Role + RoleBinding (bindings-forwarder always runs in the release namespace)
3. Update helm tests

### Phase 6: Add missing CRD access roles and reorganize

1. Create `templates/rbac/crd-access/` directory
2. Move and rename existing editor/viewer roles:
   - `boundendpoint_editor_role.yaml` → `crd-access/boundendpoint-editor.yaml` (already fixed in Phase 1)
   - `cloudendpoint_editor_role.yaml` → `crd-access/cloudendpoint-editor.yaml`
   - etc.
3. Add missing roles:
   - `crd-access/agentendpoint-editor.yaml`
   - `crd-access/agentendpoint-viewer.yaml`
   - `crd-access/ngroktrafficpolicy-editor.yaml`
   - `crd-access/ngroktrafficpolicy-viewer.yaml`
   - `crd-access/kubernetesoperator-editor.yaml`
   - `crd-access/kubernetesoperator-viewer.yaml`
4. Rename `clusterRole.annotations` → `crdAccessRoles.annotations` in `values.yaml` and all CRD access role templates
5. Drop `{{ include "ngrok-operator.labels" . }}` from CRD access roles — pod selector labels don't belong on these
6. Update helm tests

### Phase 7: Update tests directory structure

Reorganize `tests/` to mirror the new `templates/` structure:

```
tests/
  api-manager/
    deployment_test.yaml            # renamed from controller-deployment_test.yaml
    serviceaccount_test.yaml        # renamed from controller-serviceaccount_test.yaml
    configmap_test.yaml             # renamed from controller-cm_test.yaml
    pdb_test.yaml                   # renamed from controller-pdb_test.yaml
    role_test.yaml                  # tests for role.yaml + role-namespaced.yaml
    rolebinding_test.yaml           # tests for rolebinding.yaml + rolebinding-namespaced.yaml
    leader-election_test.yaml       # tests for leader-election-role.yaml
  agent/
    deployment_test.yaml            # existing
    rbac_test.yaml → split into role_test.yaml, rolebinding_test.yaml
    service-account_test.yaml → serviceaccount_test.yaml
  bindings-forwarder/
    deployment_test.yaml            # existing
    rbac_test.yaml → role_test.yaml
    service-account_test.yaml → serviceaccount_test.yaml
  rbac/
    crd-access_test.yaml            # tests for all CRD access roles
  credentials-secret_test.yaml
  ingress-class_test.yaml
```

---

## RBAC Rules Reference

For each component, the exact rules that should be in the hand-managed templates.

### api-manager

Runs: Ingress, Domain, IPPolicy, CloudEndpoint, NgrokTrafficPolicy, KubernetesOperator, Gateway, HTTPRoute, TCPRoute, TLSRoute, GatewayClass, Namespace, ReferenceGrant, Service, BoundEndpoint controllers + Drain logic.

**Cluster-scoped resources** (go in ClusterRole even in watchNamespace mode):
- `namespaces` — get, list, update, watch
- `ingressclasses` — get, list, watch
- `gatewayclasses` — get, list, patch, update, watch
- `gatewayclasses/status` — get, list, patch, update, watch
- `gatewayclasses/finalizers` — patch, update

**Namespace-scoped resources** (go in Role in watchNamespace mode, ClusterRole in default mode):
- `configmaps` — create, delete, get, list, update, watch
- `events` — create, patch
- `secrets` — create, get, list, patch, update, watch
- `services`, `services/status` — get, list, patch, update, watch
- `boundendpoints` (bindings.k8s.ngrok.com) — create, delete, get, list, patch, update, watch
- `boundendpoints/finalizers` — update
- `boundendpoints/status` — get, patch, update
- `gateways`, `httproutes`, `tcproutes`, `tlsroutes` — get, list, patch, update, watch
- `gateways/status`, `httproutes/status`, `tcproutes/status`, `tlsroutes/status` — get, list, update, watch
- `referencegrants` — get, list, watch
- `domains`, `ippolicies` (ingress.k8s.ngrok.com) — create, delete, get, list, patch, update, watch
- `domains/finalizers`, `ippolicies/finalizers` — update
- `domains/status`, `ippolicies/status` — get, patch, update
- `ingresses` — get, list, patch, update, watch
- `ingresses/status` — get, list, update, watch
- `agentendpoints` — delete, get, list, patch, update, watch
- `cloudendpoints`, `kubernetesoperators`, `ngroktrafficpolicies` — create, delete, get, list, patch, update, watch
- `cloudendpoints/finalizers`, `kubernetesoperators/finalizers`, `ngroktrafficpolicies/finalizers` — update
- `cloudendpoints/status`, `kubernetesoperators/status`, `ngroktrafficpolicies/status` — get, patch, update

### agent-manager

Runs: AgentEndpointReconciler only.

All namespace-scoped (goes in Role in watchNamespace mode):
- `events` — create, patch
- `secrets` — get, list, watch
- `domains` (ingress.k8s.ngrok.com) — create, delete, get, list, patch, update, watch
- `agentendpoints` — create, delete, get, list, patch, update, watch
- `agentendpoints/finalizers` — update
- `agentendpoints/status` — get, patch, update
- `kubernetesoperators`, `ngroktrafficpolicies` — get, list, watch

### bindings-forwarder

Runs: ForwarderReconciler only. Always namespace-scoped (Role in release namespace).

- `events` — create, patch
- `pods` — get, list, watch
- `secrets` — get, list, watch
- `boundendpoints` (bindings.k8s.ngrok.com) — get, list, patch, update, watch
- `kubernetesoperators` — get, list, watch

---

## Verification

After all changes, run:
```bash
make manifests        # should only run controller-gen for CRDs/webhooks, no rbac
make build
make test             # all Go tests
make helm-update-snapshots
make manifest-bundle
```

</details>

### Key Changes
- **Deleted 9 files**: `controller-rbac.yaml`, `agent/rbac.yaml`, 4 stale CRD access roles, `rbac/role.yaml`, `rbac/api-manager-namespaced.yaml`, test/snap files
- **New 18 files**: Per-component role/rolebinding files under `api-manager/`, `agent/`, `bindings-forwarder/`; 6 new CRD access roles under `rbac/crd-access/`
- **Renamed 16 files**: `controller-*` → `api-manager/*`, `service-account.yaml` → `serviceaccount.yaml`, CRD access role renames
- **Go changes**: Pure deletion of all 73 `+kubebuilder:rbac` markers across 17 files (no logic changes)
- **Build changes**: Removed `controller-gen rbac` from Makefile; renamed `clusterRole` → `crdAccessRoles` in values.yaml/schema
- **New helper**: `ngrok-operator.watchNamespace` in `_helpers.tpl`

### Dead Ends / Abandoned Work
- **Phase 7 (test reorg) incomplete**: Plan calls for splitting agent/bindings-forwarder test files but they were modified in-place instead
- **`new-plan.md` at repo root** — working document, not committed to main
- No reverted commits, no TODOs, no commented-out code

---

## Synthesis

### Timeline & Progression
Both branches fork from the same commit (`aac95968`) and were created on the same day (Mar 4-5, 2026). The **cleanup** branch was authored first (Mar 4), the **refactor** branch second (Mar 5). The refactor branch represents a more complete evolution of the same idea.

### What Overlaps
Both branches share the same problem analysis and fix the same issues:
- Delete 4 stale CRD access roles (bindingconfiguration, operatorconfiguration)
- Fix boundendpoint API group
- Remove stale tunnel rules from agent RBAC
- Remove duplicate events rule from bindings-forwarder
- Add missing CRD access roles (agentendpoint, ngroktrafficpolicy, kubernetesoperator)
- Move CRD access roles to `rbac/crd-access/` subdirectory
- Support namespace-scoped Roles when `watchNamespace` is set

### Where They Diverge

| Aspect | cleanup | refactor |
|---|---|---|
| **controller-gen** | Keeps it, splits into per-deployment invocations | Removes entirely, hand-manages all roles |
| **kubebuilder:rbac markers** | Keeps and adds missing ones | Removes all 73 markers |
| **RBAC source of truth** | data files (`files/rbac/`) + templates | Helm templates only |
| **Go refactoring** | Moves forwarder to subpackage | No Go structural changes |
| **Template organization** | Centralized under `templates/rbac/` (agent.yaml, api-manager.yaml, etc.) | Per-component dirs (api-manager/, agent/, bindings-forwarder/) |
| **api-manager files** | Keeps `controller-*` names | Renames to `api-manager/*` |
| **Scope** | Incremental (phases 1+3 done, rest deferred) | All-in-one (phases 1-7 attempted) |
| **Dead code removal** | Doesn't remove proxy-role or secret-manager-role | Removes both |
| **values.yaml** | No rename | Renames `clusterRole` → `crdAccessRoles` |

### Mapping to Your Context

You mentioned **"changing cluster roles to flex to regular roles if watching 1 namespace"** — both branches implement this with `$isNamespaced` guards. The refactor branch is more complete, with separate `role.yaml`/`role-namespaced.yaml` files per component and explicit RBAC rules references for each deployment.

You mentioned **"refactoring and simplifying RBAC setup across various components as they are inconsistent"** — the refactor branch addresses this more thoroughly by:
1. Establishing a consistent per-component directory structure
2. Removing controller-gen entirely (single source of truth in Helm)
3. Deleting all dead code (proxy-role, secret-manager-role)
4. Providing a complete RBAC rules reference for each deployment

## Carry-Forward Candidates

1. **Problem analysis** (both branches): The 13-item problem list in the refactor plan is thorough and still relevant. Use it as the starting point for any re-implementation.

2. **RBAC rules reference** (refactor branch, `new-plan.md`): The per-component breakdown of exactly which K8s resources each deployment needs — this is the most valuable artifact and should be verified against current main.

3. **Design principles** (refactor branch): One file per resource, plain YAML, per-component RBAC, hand-managed roles — these decisions are well-reasoned.

4. **Target file layout** (refactor branch): The `api-manager/`, `agent/`, `bindings-forwarder/` directory structure with role/rolebinding splits.

5. **CRD access role fixes** (both branches): Stale deletions, API group fix, missing roles — these are quick wins that could land independently.

6. **`watchNamespace` helper** (refactor branch, `_helpers.tpl`): Utility for the `$isNamespaced` pattern.

7. **Forwarder subpackage move** (cleanup branch only): Moving the forwarder controller to its own package for per-deployment controller-gen — only relevant if you decide to keep controller-gen.

## High Conflict Risk Commits on Main

Both branches need to rebase over 19 commits. The highest-risk ones:
- `8e7a76e6` — **Pod Identity bug fixes** to `bindings-forwarder/rbac.yaml` (both branches delete this file)
- `769efe70` — **Gateway API changes** to `_helpers.tpl` and `values.yaml` (both branches modify these)
- `aa1781d3` — **Dependency updates** touching many controller files (refactor branch removes markers from these)
- `b0d701c0` — **URI vs URL fix** to forwarder controller (cleanup branch moves this file)
