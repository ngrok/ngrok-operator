# RBAC Overhaul

**Date**: 2026-04-07
**Status**: Proposed
**Spec**: [specs/rbac.md](../rbac.md)

## Goal

1. Support namespace-scoped RBAC (Roles instead of ClusterRoles) when `watchNamespace` is set
2. Clean up inconsistent RBAC across all three operator components
3. Hand-manage all RBAC in Helm templates (remove controller-gen RBAC generation)
4. Fix all known RBAC bugs

## Problems being fixed

1. **controller-gen produces a monolithic ClusterRole** merging all markers from all controllers into one role, but only api-manager binds to it
2. **Inconsistent management**: api-manager role is generated, agent and forwarder are hand-managed
3. **`controller-rbac.yaml` is a kitchen sink** — leader-election, dead proxy-role, manager binding, secret-manager all in one file
4. **4 stale CRD access roles** referencing non-existent CRDs (bindingconfiguration, operatorconfiguration)
5. **Wrong API group** on boundendpoint editor/viewer roles (`ngrok.k8s.ngrok.com` should be `bindings.k8s.ngrok.com`)
6. **Stale tunnel rules** in agent ClusterRole (Tunnel CRD was removed)
7. **Dead proxy-role** — leftover kube-rbac-proxy scaffolding
8. **Duplicate events rule** in bindings-forwarder
9. **Missing CRD access roles** for AgentEndpoint, NgrokTrafficPolicy, KubernetesOperator
10. **`clusterRole.annotations`** applied to operator-internal roles instead of only CRD access roles
11. **No namespace-scoped support** for agent-manager

## Why hand-manage?

Every major multi-deployment K8s operator (cert-manager, Kyverno, ArgoCD, Crossplane, Istio) hand-manages RBAC. controller-gen RBAC is designed for single-binary, single-SA operators and cannot:
- Split roles per deployment within a shared Go package
- Generate ServiceAccounts or Bindings
- Handle the bindings package (2 controllers in 1 package, running in different pods)

## Changes

### Helm template reorganization

Move from scattered RBAC files to per-component directories:

**New structure:**
```
templates/
  api-manager/       deployment, sa, configmap, pdb, role, rolebinding, leader-election
  agent/             deployment, sa, role, rolebinding
  bindings-forwarder/ deployment, sa, role
  rbac/crd-access/   editor/viewer ClusterRoles
```

**Files deleted:**
- `controller-rbac.yaml` (split into api-manager/ files)
- `rbac/role.yaml` (replaced by api-manager/role.yaml)
- `agent/rbac.yaml` (split into role/rolebinding files)
- `bindings-forwarder/rbac.yaml` (replaced)
- 4 stale CRD access roles

**Files renamed:**
- `controller-deployment.yaml` -> `api-manager/deployment.yaml`
- `controller-serviceaccount.yaml` -> `api-manager/serviceaccount.yaml`
- `controller-cm.yaml` -> `api-manager/configmap.yaml`
- `controller-pdb.yaml` -> `api-manager/pdb.yaml`
- `agent/service-account.yaml` -> `agent/serviceaccount.yaml`
- `bindings-forwarder/service-account.yaml` -> `bindings-forwarder/serviceaccount.yaml`

### Go code changes

- Remove all ~74 `+kubebuilder:rbac` markers from ~18 Go files
- Remove `output:rbac:artifacts:config=...` from controller-gen in `tools/make/generate.mk` (keep CRD + webhook)

### values.yaml changes

- Rename `clusterRole.annotations` to `crdAccessRoles.annotations`
- Apply only to CRD access role templates

### Bug fixes rolled in

1. Remove dead proxy-role ClusterRole + ClusterRoleBinding
2. Fold secret-manager-role into api-manager role (add secret create/update/patch)
3. Remove stale tunnel rules from agent role
4. Remove duplicate events rule from bindings-forwarder
5. Fix boundendpoint CRD access roles API group
6. Delete 4 stale CRD access roles
7. Add 6 missing CRD access roles

### Namespace-scoping (new feature)

New `isNamespaced` helper in `_helpers.tpl`. Each component gets `role.yaml` (ClusterRole, default) and `role-namespaced.yaml` (Role, watchNamespace) with simple `if` guards. api-manager's cluster-scoped resources (namespaces, ingressclasses, gatewayclasses) always stay in a ClusterRole.

## Verification

### Baseline (captured)

RBAC baseline from a kind cluster deployment of main, stored in `specs/rbac/baseline/`:
- Effective permissions per SA via `kubectl auth can-i --list`
- Individual role definitions as YAML

### After refactor

1. Deploy refactored chart to kind
2. Run `kubectl auth can-i --list` per SA, diff against baseline
3. Expected intentional diffs:
   - **Removed**: proxy-role (tokenreviews, subjectaccessreviews), tunnel rules from agent
   - **Added**: secret create/update/patch on api-manager (folded from secret-manager-role)
4. Deploy with `--set watchNamespace=<ns>`, confirm Roles instead of ClusterRoles, same effective permissions
5. `make build && make test && make helm-update-snapshots && make manifest-bundle`
