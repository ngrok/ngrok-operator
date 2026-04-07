# RBAC Overhaul Design

## Goal

1. Support namespace-scoped RBAC (Roles instead of ClusterRoles) when `watchNamespace` is set
2. Clean up inconsistent RBAC across all three operator components
3. Hand-manage all RBAC in Helm templates (remove controller-gen RBAC generation)
4. Fix all known RBAC bugs (stale rules, wrong API groups, dead code)

## Background

The operator has 3 deployments with different service accounts:

| Deployment | ServiceAccount | Controllers |
|---|---|---|
| api-manager | `ngrok-operator` | Ingress, Domain, IPPolicy, CloudEndpoint, NgrokTrafficPolicy, KubernetesOperator, BoundEndpoint, Gateway, HTTPRoute, TCPRoute, TLSRoute, GatewayClass, Namespace, ReferenceGrant, Service + Drain |
| agent-manager | `ngrok-operator-agent` | AgentEndpoint only |
| bindings-forwarder | `ngrok-operator-bindings-forwarder` | Forwarder only (conditional on `bindings.enabled`) |

### Current problems

1. **controller-gen produces a monolithic ClusterRole** merging all markers from all controllers. Only api-manager binds to it, but it includes agent-manager's markers too.
2. **Inconsistent management**: api-manager role is generated, agent and forwarder roles are hand-managed.
3. **`controller-rbac.yaml` is a kitchen sink** — leader-election, proxy-role (dead), manager binding, secret-manager all in one file.
4. **Stale CRD access roles**: `bindingconfiguration_*`, `operatorconfiguration_*` reference non-existent CRDs.
5. **Wrong API group**: `boundendpoint_*` roles use `ngrok.k8s.ngrok.com` instead of `bindings.k8s.ngrok.com`.
6. **Stale agent rules**: `tunnels` CRD was removed but agent ClusterRole still grants access.
7. **Dead proxy-role**: leftover kube-rbac-proxy scaffolding, no proxy sidecar exists.
8. **Duplicate events rule** in bindings-forwarder RBAC.
9. **Missing CRD access roles**: no editor/viewer for AgentEndpoint, NgrokTrafficPolicy, KubernetesOperator.
10. **`clusterRole.annotations`** applied to operator-internal roles instead of only CRD access roles.
11. **No namespace-scoped support** for agent-manager.

### Why hand-manage (not controller-gen)?

Every major multi-deployment K8s operator hand-manages RBAC:

| Project | controller-gen for RBAC? | Approach |
|---|---|---|
| cert-manager | No | Per-concern ClusterRoles in Helm |
| Kyverno | No | Per-deployment directories in Helm |
| ArgoCD | No | Per-deployment directories in Helm |
| Crossplane | No | Hand-managed + dynamic RBAC manager |
| Istio | No | Per-chart in Helm |

controller-gen RBAC is designed for single-binary, single-SA operators. Limitations:
- Produces one monolithic role per invocation
- Cannot split roles per deployment within a shared Go package
- No ServiceAccount/Binding generation
- Broken RoleBinding generation (controller-tools#760)
- Markers end up in libraries, API types, and drain logic — not just controllers

## Design

### Principles

- **Per-component directories** in Helm (Kyverno pattern)
- **One file per K8s resource** (or tightly-coupled pair like Role+RoleBinding)
- **Plain YAML with simple `if` guards** for namespace-scoping
- **Hand-managed roles, no kubebuilder:rbac markers**
- **CRD access roles** stay in operator chart under `templates/rbac/crd-access/`

### Target file layout

```
templates/
  api-manager/
    deployment.yaml                     # renamed from controller-deployment.yaml
    serviceaccount.yaml                 # renamed from controller-serviceaccount.yaml
    configmap.yaml                      # renamed from controller-cm.yaml
    pdb.yaml                            # renamed from controller-pdb.yaml
    role.yaml                           # ClusterRole (default mode)
    role-namespaced.yaml                # Role + ClusterRole (watchNamespace mode)
    rolebinding.yaml                    # ClusterRoleBinding (default mode)
    rolebinding-namespaced.yaml         # RoleBinding + ClusterRoleBinding (watchNamespace mode)
    leader-election-role.yaml           # Role + RoleBinding (always namespaced to release NS)

  agent/
    deployment.yaml                     # unchanged
    serviceaccount.yaml                 # renamed from service-account.yaml
    role.yaml                           # ClusterRole (default mode)
    role-namespaced.yaml                # Role (watchNamespace mode)
    rolebinding.yaml                    # ClusterRoleBinding (default mode)
    rolebinding-namespaced.yaml         # RoleBinding (watchNamespace mode)

  bindings-forwarder/
    deployment.yaml                     # unchanged
    serviceaccount.yaml                 # renamed from service-account.yaml
    role.yaml                           # Role + RoleBinding (always namespaced)

  cleanup-hook/
    job.yaml                            # unchanged
    rbac.yaml                           # unchanged

  rbac/
    crd-access/
      agentendpoint-editor.yaml         # NEW
      agentendpoint-viewer.yaml         # NEW
      boundendpoint-editor.yaml         # FIXED api group
      boundendpoint-viewer.yaml         # FIXED api group
      cloudendpoint-editor.yaml
      cloudendpoint-viewer.yaml
      domain-editor.yaml
      domain-viewer.yaml
      ippolicy-editor.yaml
      ippolicy-viewer.yaml
      kubernetesoperator-editor.yaml    # NEW
      kubernetesoperator-viewer.yaml    # NEW
      ngroktrafficpolicy-editor.yaml    # NEW
      ngroktrafficpolicy-viewer.yaml    # NEW
```

### Deleted files

- `controller-rbac.yaml` — split into api-manager/ files
- `rbac/role.yaml` — replaced by api-manager/role.yaml
- `agent/rbac.yaml` — replaced by agent/role.yaml + rolebinding.yaml etc.
- `bindings-forwarder/rbac.yaml` — replaced by bindings-forwarder/role.yaml
- `rbac/bindingconfiguration_editor_role.yaml` — CRD doesn't exist
- `rbac/bindingconfiguration_viewer_role.yaml` — CRD doesn't exist
- `rbac/operatorconfiguration_editor_role.yaml` — CRD doesn't exist
- `rbac/operatorconfiguration_viewer_role.yaml` — CRD doesn't exist

### Namespace-scoping

A new helper in `_helpers.tpl`:

```yaml
{{- define "ngrok-operator.isNamespaced" -}}
{{- if or .Values.watchNamespace .Values.ingress.watchNamespace -}}true{{- end -}}
{{- end -}}
```

Per-component behavior:

| Component | Default mode | watchNamespace mode |
|---|---|---|
| api-manager | 1 ClusterRole + ClusterRoleBinding | 1 Role (ns-scoped) + 1 ClusterRole (cluster-scoped) + RoleBinding + ClusterRoleBinding |
| agent-manager | 1 ClusterRole + ClusterRoleBinding | 1 Role + RoleBinding |
| bindings-forwarder | 1 Role + RoleBinding (always) | No change |
| leader-election | 1 Role + RoleBinding (always) | No change |

**api-manager cluster-scoped resources** (always in ClusterRole, even in watchNamespace mode):
- `namespaces` — get, list, update, watch
- `ingressclasses` — get, list, watch
- `gatewayclasses` + `/status` + `/finalizers` — get, list, patch, update, watch

Everything else is namespace-scoped and goes in a Role when `isNamespaced`.

### Go code changes

- Remove all ~74 `+kubebuilder:rbac` markers from ~18 Go files
- Remove the 1 marker in `api/bindings/v1alpha1/boundendpoint_types.go`
- Remove `output:rbac:artifacts:config=...` from controller-gen invocation in `tools/make/generate.mk` (keep CRD + webhook generation)

### values.yaml changes

- Rename `clusterRole.annotations` to `crdAccessRoles.annotations`
- Apply only to CRD access role templates (not operator-internal roles)

### Cleanup (rolled into restructure)

1. Remove dead `proxy-role` ClusterRole + ClusterRoleBinding
2. Fold `secret-manager-role` into api-manager role (add secret create/update/patch)
3. Remove stale `tunnels` rules from agent role
4. Remove duplicate `events` rule from bindings-forwarder role
5. Fix boundendpoint CRD access roles API group
6. Delete 4 stale CRD access roles
7. Add 6 missing CRD access roles (agentendpoint, ngroktrafficpolicy, kubernetesoperator)

## RBAC rules per deployment

### api-manager

Source: baseline dump from deployed cluster (`specs/rbac/clusterrole-manager.yaml`) + secret-manager-role folded in.

**Cluster-scoped resources** (ClusterRole in both modes):
- `namespaces` — get, list, update, watch
- `ingressclasses` (networking.k8s.io) — get, list, watch
- `gatewayclasses` (gateway.networking.k8s.io) — get, list, patch, update, watch
- `gatewayclasses/status` — get, list, patch, update, watch
- `gatewayclasses/finalizers` — patch, update

**Namespace-scoped resources** (ClusterRole in default, Role in watchNamespace):
- `configmaps` — create, delete, get, list, patch, update, watch
- `events` — create, patch
- `secrets` — create, get, list, patch, update, watch
- `services` — create, delete, get, list, patch, update, watch
- `services/status` — get, list, patch, update, watch
- `ingresses` (networking.k8s.io) — get, list, patch, update, watch
- `ingresses/status` — get, list, update, watch
- `gateways` (gateway.networking.k8s.io) — get, list, patch, update, watch
- `gateways/status` — get, list, update, watch
- `httproutes` — get, list, patch, update, watch
- `httproutes/status` — get, list, update, watch
- `tcproutes` — get, list, patch, update, watch
- `tcproutes/status` — get, list, update, watch
- `tlsroutes` — get, list, patch, update, watch
- `tlsroutes/status` — get, list, update, watch
- `referencegrants` — get, list, watch
- `boundendpoints` (bindings.k8s.ngrok.com) — create, delete, get, list, patch, update, watch
- `boundendpoints/finalizers` — update
- `boundendpoints/status` — get, patch, update
- `domains` (ingress.k8s.ngrok.com) — create, delete, get, list, patch, update, watch
- `domains/finalizers` — update
- `domains/status` — get, patch, update
- `ippolicies` (ingress.k8s.ngrok.com) — create, delete, get, list, patch, update, watch
- `ippolicies/finalizers` — update
- `ippolicies/status` — get, patch, update
- `agentendpoints` (ngrok.k8s.ngrok.com) — create, delete, get, list, patch, update, watch
- `agentendpoints/finalizers` — update
- `agentendpoints/status` — get, patch, update
- `cloudendpoints` — create, delete, get, list, patch, update, watch
- `cloudendpoints/finalizers` — update
- `cloudendpoints/status` — get, patch, update
- `kubernetesoperators` — create, delete, get, list, patch, update, watch
- `kubernetesoperators/finalizers` — update
- `kubernetesoperators/status` — get, patch, update
- `ngroktrafficpolicies` — create, delete, get, list, patch, update, watch
- `ngroktrafficpolicies/finalizers` — update
- `ngroktrafficpolicies/status` — get, patch, update

**Leader election** (always namespaced Role in release NS):
- `configmaps` — get, list, watch, create, update, patch, delete
- `leases` (coordination.k8s.io) — get, list, watch, create, update, patch, delete
- `events` — create, patch

### agent-manager

All namespace-scoped (Role in watchNamespace mode):
- `events` — create, patch
- `secrets` — get, list, watch
- `domains` (ingress.k8s.ngrok.com) — create, delete, get, list, patch, update, watch
- `agentendpoints` (ngrok.k8s.ngrok.com) — get, list, watch, patch, update
- `agentendpoints/finalizers` — update
- `agentendpoints/status` — get, patch, update
- `kubernetesoperators` (ngrok.k8s.ngrok.com) — get, list, watch
- `ngroktrafficpolicies` (ngrok.k8s.ngrok.com) — get, list, watch

**Removed** (stale): tunnels, tunnels/finalizers, tunnels/status

### bindings-forwarder

Always namespaced Role in release NS:
- `events` — create, patch
- `pods` — get, list, watch
- `secrets` — get, list, watch
- `boundendpoints` (bindings.k8s.ngrok.com) — get, list, patch, update, watch
- `kubernetesoperators` (ngrok.k8s.ngrok.com) — get, list, watch

**Removed**: duplicate events rule

## Verification

### Phase 0: Baseline (done)

RBAC baseline captured from live kind cluster deployment of main branch:
- `specs/rbac/api-manager-effective.txt` — effective permissions for ngrok-operator SA
- `specs/rbac/agent-manager-effective.txt` — effective permissions for ngrok-operator-agent SA
- `specs/rbac/clusterrole-*.yaml` — individual role definitions
- `specs/rbac/role-*.yaml` — individual role definitions
- `specs/rbac/crd-access-*.yaml` — CRD access role definitions

### Phase N: After refactor

1. Deploy refactored chart to kind
2. Run same `kubectl auth can-i --list` commands per SA
3. Diff against baseline — every delta must be intentional:
   - **Removed**: proxy-role permissions (tokenreviews, subjectaccessreviews), tunnel rules from agent
   - **Added to api-manager**: secret create/update/patch (folded from secret-manager-role)
   - **Unchanged**: all other permissions
4. Verify watchNamespace mode: deploy with `--set watchNamespace=ngrok-operator`, confirm Roles created instead of ClusterRoles, same effective permissions within the namespace
5. `make build && make test && make helm-update-snapshots && make manifest-bundle`
