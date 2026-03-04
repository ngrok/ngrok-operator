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
| `bindingconfiguration_*` | BindingConfiguration | ❌ **CRD doesn't exist — DELETE** |
| `operatorconfiguration_*` | OperatorConfiguration | ❌ **CRD doesn't exist — DELETE** |
| `boundendpoint_*` | BoundEndpoint | ⚠️ **Wrong API group** (`ngrok.k8s.ngrok.com` → should be `bindings.k8s.ngrok.com`) |
| `cloudendpoint_*` | CloudEndpoint | ✅ Correct |
| `domain_*` | Domain | ✅ Correct |
| `ippolicy_*` | IPPolicy | ✅ Correct |

**Missing editor/viewer roles (CRDs with no roles):**
- `AgentEndpoint` — **Missing** (users may want to create/view these)
- `KubernetesOperator` — **Missing** (probably intentional — this is operator-internal)
- `NgrokTrafficPolicy` — **Missing** (users definitely want to create/view these)

**Recommendation**: Add `agentendpoint_editor/viewer_role.yaml` and `ngroktrafficpolicy_editor/viewer_role.yaml`. Skip KubernetesOperator (it's an internal singleton). Whether this pattern is worth keeping at all is debatable — many operators don't bother with these roles — but since you already have them for some CRDs, being consistent is better than the current half-state.

---

## Recommendations

### Phase 1: Quick Wins ✅ DONE

1. ✅ Deleted 4 stale editor/viewer roles (bindingconfiguration_*, operatorconfiguration_*)
2. ✅ Fixed boundendpoint_* API group to `bindings.k8s.ngrok.com`
3. ✅ Removed dead `tunnels` rules from `agent/rbac.yaml`
4. ✅ Removed duplicate `events` rule from `bindings-forwarder/rbac.yaml`
5. ✅ Added missing editor/viewer roles for AgentEndpoint, NgrokTrafficPolicy, KubernetesOperator

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

### Phase 3: Per-Deployment controller-gen RBAC ✅ DONE

Separated `controller-gen` into per-deployment invocations:
- **api-manager**: paths include ingress, ngrok, gateway, service, bindings controllers + drain
- **agent-manager**: paths include only `internal/controller/agent/`
- **bindings-forwarder**: stays hand-managed (ForwarderReconciler has no kubebuilder:rbac markers)

Generated data files: `files/rbac/api-manager-role.yaml` and `files/rbac/agent-manager-role.yaml`

Added missing markers to `agent_endpoint_controller.go` for `events` and `kubernetesoperators` (needed for event recording and DrainState).

### Phase 4: Remove accidental `clusterRole.annotations` from operator roles

The `clusterRole.annotations` Helm value was added in [PR #738](https://github.com/ngrok/ngrok-operator/pull/738) specifically for RBAC aggregation on the CRD access roles (editor/viewer roles). It was accidentally also applied to:
- `templates/agent/rbac.yaml` (the agent-manager Role/ClusterRole)
- `templates/controller-rbac.yaml` (the proxy ClusterRole)

These are operator-internal infrastructure roles — aggregation annotations don't belong on them. Remove the `{{- with .Values.clusterRole.annotations }}` blocks from both files.

### Phase 5: Move CRD Access Roles to the CRDs Chart

#### What are CRD access roles?

These are ClusterRoles that grant **end-users** (not the operator) the ability to read or modify CRD instances. They exist as a kubebuilder convention and are commonly used with [Kubernetes RBAC aggregation](https://kubernetes.io/docs/reference/access-authn-authz/rbac/#aggregated-clusterroles). By adding annotations like `rbac.authorization.k8s.io/aggregate-to-admin: "true"`, these roles automatically merge into the built-in `admin`/`edit`/`view` ClusterRoles. This means users with existing `admin` RoleBindings in a namespace automatically get CRD permissions without needing new bindings.

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

Helm has built-in subchart value passing. Any values nested under the subchart's name in the parent chart's `values.yaml` are automatically forwarded to the subchart. So:

```yaml
# helm/ngrok-operator/values.yaml
ngrok-crds:
  crdAccessRoles:
    annotations: {}
```

When a user installs the operator chart and sets:
```
--set ngrok-crds.crdAccessRoles.annotations."rbac\.authorization\.k8s\.io/aggregate-to-admin"=true
```
Helm passes `crdAccessRoles.annotations` to the CRDs chart automatically. No custom mapping code needed — this is standard Helm behavior.

When someone installs the CRDs chart standalone (without the operator), they set the value directly:
```
--set crdAccessRoles.annotations."rbac\.authorization\.k8s\.io/aggregate-to-admin"=true
```

#### Implementation steps

1. **Add `values.yaml` to the CRDs chart:**
   ```yaml
   # helm/ngrok-crds/values.yaml
   crdAccessRoles:
     annotations: {}
   ```

2. **Add minimal `_helpers.tpl` to the CRDs chart** (it currently has none):
   Only needs a helper for the annotation injection — no fullname/labels helpers needed.

3. **Move the 14 editor/viewer role files** from `helm/ngrok-operator/templates/rbac/` to `helm/ngrok-crds/templates/crd-access-roles/`:
   - Drop the `{{ include "ngrok-operator.labels" . }}` block (pod selector labels don't belong on these)
   - Drop the `{{ include "ngrok-operator.fullname" . }}` prefix — use a fixed `ngrok-` prefix (e.g., `ngrok-cloudendpoint-editor-role`). These are cluster singletons tied to the CRDs, not per-release resources.
   - Change `{{- with .Values.clusterRole.annotations }}` to `{{- with .Values.crdAccessRoles.annotations }}`

4. **Update `helm/ngrok-operator/values.yaml`:**
   - Remove `clusterRole.annotations` (no longer used by anything in the operator chart after Phase 4)
   - Add the pass-through value:
     ```yaml
     ngrok-crds:
       crdAccessRoles:
         annotations: {}
     ```

5. **After the move, `templates/rbac/` in the operator chart** will contain only `role.yaml` (the api-manager role). Consider renaming the directory or moving `role.yaml` elsewhere to avoid confusion.

---

## Summary of Current File Layout (After Phases 1, 3, 4, 5)

```
helm/ngrok-crds/
  templates/
    crd-access-roles/                      # NEW: moved from operator chart
      agentendpoint_editor_role.yaml
      agentendpoint_viewer_role.yaml
      boundendpoint_editor_role.yaml
      boundendpoint_viewer_role.yaml
      cloudendpoint_editor_role.yaml
      cloudendpoint_viewer_role.yaml
      domain_editor_role.yaml
      domain_viewer_role.yaml
      ippolicy_editor_role.yaml
      ippolicy_viewer_role.yaml
      kubernetesoperator_editor_role.yaml
      kubernetesoperator_viewer_role.yaml
      ngroktrafficpolicy_editor_role.yaml
      ngroktrafficpolicy_viewer_role.yaml
    <CRD yaml files>                       # Existing
  values.yaml                              # NEW: crdAccessRoles.annotations
  _helpers.tpl                             # NEW: minimal, for annotation injection only

helm/ngrok-operator/
  files/rbac/
    api-manager-role.yaml                  # Auto-generated by controller-gen
    agent-manager-role.yaml                # Auto-generated by controller-gen

  templates/
    rbac/
      role.yaml                            # api-manager Role/ClusterRole (reads from files/rbac/api-manager-role.yaml)

    agent/
      deployment.yaml
      rbac.yaml                            # Agent Role/ClusterRole (reads from files/rbac/agent-manager-role.yaml)
      service-account.yaml

    bindings-forwarder/
      deployment.yaml
      rbac.yaml                            # Hand-managed (no kubebuilder markers)
      service-account.yaml

    cleanup-hook/
      job.yaml
      rbac.yaml

    controller-rbac.yaml                   # leader-election, proxy, manager bindings, secret-manager
    controller-deployment.yaml
    controller-serviceaccount.yaml
    controller-cm.yaml
    ...
```
