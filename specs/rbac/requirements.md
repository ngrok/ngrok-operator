# RBAC Requirements

The operator has 3 deployments, each with its own ServiceAccount and least-privilege Role/ClusterRole.

## Architecture

| Deployment | ServiceAccount | Controllers | Conditional? |
|---|---|---|---|
| api-manager | `ngrok-operator` | Ingress, Domain, IPPolicy, CloudEndpoint, NgrokTrafficPolicy, KubernetesOperator, BoundEndpoint, Gateway, HTTPRoute, TCPRoute, TLSRoute, GatewayClass, Namespace, ReferenceGrant, Service + Drain | No |
| agent-manager | `ngrok-operator-agent` | AgentEndpoint | No |
| bindings-forwarder | `ngrok-operator-bindings-forwarder` | Forwarder | Yes (`bindings.enabled`) |

### Management approach

All RBAC is hand-managed in Helm templates. `controller-gen` is used only for CRDs and webhooks, not for RBAC generation. This follows the pattern used by cert-manager, Kyverno, ArgoCD, and other multi-deployment operators.

controller-gen RBAC is designed for single-binary, single-SA operators and cannot split roles per deployment within a shared Go package, generate ServiceAccounts or Bindings, or handle the bindings package (2 controllers in 1 package, running in different pods).

### Namespace-scoping

When `watchNamespace` is set, namespace-scoped resources use a **Role** (scoped to that namespace) instead of a **ClusterRole**. Cluster-scoped K8s resources always require a ClusterRole regardless.

| Component | Default mode | watchNamespace mode |
|---|---|---|
| api-manager | ClusterRole + ClusterRoleBinding | Role + ClusterRole (cluster-scoped only) + RoleBinding + ClusterRoleBinding |
| agent-manager | ClusterRole + ClusterRoleBinding | Role + RoleBinding |
| bindings-forwarder | Role + RoleBinding (always namespaced) | No change |
| leader-election | Role + RoleBinding (always namespaced) | No change |

### Helm template organization

RBAC lives in per-component directories under `helm/ngrok-operator/templates/`:

```
api-manager/       role.yaml, role-namespaced.yaml, rolebinding.yaml, rolebinding-namespaced.yaml, leader-election-role.yaml
agent/             role.yaml, role-namespaced.yaml, rolebinding.yaml, rolebinding-namespaced.yaml
bindings-forwarder/ role.yaml (always namespaced)
rbac/crd-access/   editor/viewer ClusterRoles for end-users
```

## CRD access roles

End-user ClusterRoles for granting read/write access to operator CRDs via RBAC aggregation. These are not bound to any operator ServiceAccount. Users bind them to their own subjects.

Each CRD gets an editor role (full CRUD + status read) and a viewer role (read-only + status read). Annotations for RBAC aggregation (e.g., `rbac.authorization.k8s.io/aggregate-to-admin`) are configurable via `crdAccessRoles.annotations` in values.yaml.

## Verification

To verify RBAC after changes, deploy to a kind cluster and compare effective permissions:

```bash
kubectl auth can-i --list \
  --as=system:serviceaccount:<ns>:<sa-name> \
  -n <ns>
```

The Helm templates in `helm/ngrok-operator/templates/` are the source of truth for exact permissions per component.
