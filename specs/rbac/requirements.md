# RBAC Requirements

The operator has 3 deployments, each with its own ServiceAccount and least-privilege Role/ClusterRole.

## Architecture

| Deployment | ServiceAccount | Controllers | Conditional? |
|---|---|---|---|
| api-manager | `ngrok-operator` | Ingress, Domain, IPPolicy, CloudEndpoint, NgrokTrafficPolicy, KubernetesOperator, BoundEndpoint, Gateway, HTTPRoute, TCPRoute, TLSRoute, GatewayClass, Namespace, ReferenceGrant, Service + Drain | No |
| agent-manager | `ngrok-operator-agent` | AgentEndpoint | Yes (`ingress.enabled`) |
| bindings-forwarder | `ngrok-operator-bindings-forwarder` | Forwarder | Yes (`bindings.enabled`) |

### Management approach

All RBAC is hand-managed in Helm templates. `controller-gen` is used only for CRDs and webhooks, not for RBAC generation. This follows the pattern used by cert-manager, Kyverno, ArgoCD, and other multi-deployment operators.

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

## api-manager permissions

Runs most controllers plus drain logic. Needs broad access.

### Cluster-scoped resources (always ClusterRole)

| Resource | API Group | Verbs | Used by |
|---|---|---|---|
| namespaces | core | get, list, update, watch | HTTPRoute (cross-ns refs), Namespace controller |
| ingressclasses | networking.k8s.io | get, list, watch | Ingress controller (class filtering) |
| gatewayclasses | gateway.networking.k8s.io | get, list, patch, update, watch | GatewayClass controller |
| gatewayclasses/status | gateway.networking.k8s.io | get, list, patch, update, watch | GatewayClass controller |
| gatewayclasses/finalizers | gateway.networking.k8s.io | patch, update | GatewayClass controller |

### Namespace-scoped resources (Role when watchNamespace set)

| Resource | API Group | Verbs | Used by |
|---|---|---|---|
| configmaps | core | create, delete, get, list, patch, update, watch | Leader election, driver config |
| events | core | create, patch | Event recording across all controllers |
| secrets | core | create, get, list, patch, update, watch | KubernetesOperator (TLS cert creation), Ingress/Gateway (TLS reads) |
| services | core | create, delete, get, list, patch, update, watch | Service controller, Ingress/Gateway (backend resolution) |
| services/finalizers | core | patch, update | Service controller |
| services/status | core | get, list, patch, update, watch | Service controller |
| ingresses | networking.k8s.io | get, list, patch, update, watch | Ingress controller |
| ingresses/finalizers | networking.k8s.io | patch, update | Ingress controller |
| ingresses/status | networking.k8s.io | get, list, update, watch | Ingress controller |
| gateways | gateway.networking.k8s.io | get, list, patch, update, watch | Gateway controller |
| gateways/finalizers | gateway.networking.k8s.io | patch, update | Gateway controller |
| gateways/status | gateway.networking.k8s.io | get, list, update, watch | Gateway controller |
| httproutes | gateway.networking.k8s.io | get, list, patch, update, watch | HTTPRoute controller |
| httproutes/finalizers | gateway.networking.k8s.io | patch, update | HTTPRoute controller |
| httproutes/status | gateway.networking.k8s.io | get, list, update, watch | HTTPRoute controller |
| tcproutes | gateway.networking.k8s.io | get, list, patch, update, watch | TCPRoute controller |
| tcproutes/finalizers | gateway.networking.k8s.io | patch, update | TCPRoute controller |
| tcproutes/status | gateway.networking.k8s.io | get, list, update, watch | TCPRoute controller |
| tlsroutes | gateway.networking.k8s.io | get, list, patch, update, watch | TLSRoute controller |
| tlsroutes/finalizers | gateway.networking.k8s.io | patch, update | TLSRoute controller |
| tlsroutes/status | gateway.networking.k8s.io | get, list, update, watch | TLSRoute controller |
| referencegrants | gateway.networking.k8s.io | get, list, watch | ReferenceGrant controller |
| domains | ingress.k8s.ngrok.com | create, delete, get, list, patch, update, watch | Domain controller, Drain |
| domains/finalizers | ingress.k8s.ngrok.com | patch, update | Domain controller |
| domains/status | ingress.k8s.ngrok.com | get, patch, update | Domain controller |
| ippolicies | ingress.k8s.ngrok.com | create, delete, get, list, patch, update, watch | IPPolicy controller, Drain |
| ippolicies/finalizers | ingress.k8s.ngrok.com | patch, update | IPPolicy controller |
| ippolicies/status | ingress.k8s.ngrok.com | get, patch, update | IPPolicy controller |
| boundendpoints | bindings.k8s.ngrok.com | create, delete, get, list, patch, update, watch | BoundEndpoint controller, Drain |
| boundendpoints/finalizers | bindings.k8s.ngrok.com | patch, update | BoundEndpoint controller |
| boundendpoints/status | bindings.k8s.ngrok.com | get, patch, update | BoundEndpoint controller |
| agentendpoints | ngrok.k8s.ngrok.com | create, delete, get, list, patch, update, watch | Drain (cleanup), driver (creates from ingress/gateway) |
| agentendpoints/finalizers | ngrok.k8s.ngrok.com | patch, update | AgentEndpoint lifecycle |
| agentendpoints/status | ngrok.k8s.ngrok.com | get, patch, update | AgentEndpoint lifecycle |
| cloudendpoints | ngrok.k8s.ngrok.com | create, delete, get, list, patch, update, watch | CloudEndpoint controller, Drain |
| cloudendpoints/finalizers | ngrok.k8s.ngrok.com | patch, update | CloudEndpoint controller |
| cloudendpoints/status | ngrok.k8s.ngrok.com | get, patch, update | CloudEndpoint controller |
| kubernetesoperators | ngrok.k8s.ngrok.com | create, delete, get, list, patch, update, watch | KubernetesOperator controller |
| kubernetesoperators/finalizers | ngrok.k8s.ngrok.com | patch, update | KubernetesOperator controller |
| kubernetesoperators/status | ngrok.k8s.ngrok.com | get, patch, update | KubernetesOperator controller |
| ngroktrafficpolicies | ngrok.k8s.ngrok.com | create, delete, get, list, patch, update, watch | NgrokTrafficPolicy controller |
| ngroktrafficpolicies/finalizers | ngrok.k8s.ngrok.com | patch, update | NgrokTrafficPolicy controller |
| ngroktrafficpolicies/status | ngrok.k8s.ngrok.com | get, patch, update | NgrokTrafficPolicy controller |

### Leader election (always namespaced Role in release NS)

| Resource | API Group | Verbs | Used by |
|---|---|---|---|
| configmaps | core | create, delete, get, list, patch, update, watch | controller-runtime leader election |
| leases | coordination.k8s.io | create, delete, get, list, patch, update, watch | controller-runtime leader election |
| events | core | create, patch | Leader election event recording |

## agent-manager permissions

Runs only the AgentEndpoint controller. All resources are namespace-scoped.

| Resource | API Group | Verbs | Used by |
|---|---|---|---|
| events | core | create, patch | Event recording |
| secrets | core | get, list, watch | TLS certificate reads for AgentEndpoints |
| domains | ingress.k8s.ngrok.com | create, delete, get, list, patch, update, watch | Auto-creates Domain resources for AgentEndpoints |
| agentendpoints | ngrok.k8s.ngrok.com | get, list, watch, patch, update | AgentEndpoint reconciler |
| agentendpoints/finalizers | ngrok.k8s.ngrok.com | patch, update | AgentEndpoint finalizer |
| agentendpoints/status | ngrok.k8s.ngrok.com | get, patch, update | AgentEndpoint status updates |
| kubernetesoperators | ngrok.k8s.ngrok.com | get, list, watch | Reads drain state |
| ngroktrafficpolicies | ngrok.k8s.ngrok.com | get, list, watch | Resolves traffic policy refs |

## bindings-forwarder permissions

Runs only the Forwarder controller. Always namespace-scoped (Role in release namespace).

| Resource | API Group | Verbs | Used by |
|---|---|---|---|
| events | core | create, patch | Event recording |
| pods | core | get, list, watch | Watches forwarding pods |
| secrets | core | get, list, watch | TLS certificate reads |
| boundendpoints | bindings.k8s.ngrok.com | get, list, patch, update, watch | Forwarder reconciler |
| kubernetesoperators | ngrok.k8s.ngrok.com | get, list, watch | Reads drain state |

## Cleanup hook

Helm pre-delete hook that cleans up KubernetesOperator resources. Uses a dedicated ServiceAccount with a Role scoped to the release namespace.

| Resource | API Group | Verbs | Used by |
|---|---|---|---|
| kubernetesoperators | ngrok.k8s.ngrok.com | delete, get, list, watch | Pre-delete cleanup job |

## CRD access roles

End-user ClusterRoles for granting read/write access to operator CRDs via RBAC aggregation. These are not bound to any operator ServiceAccount. Users bind them to their own subjects.

Each CRD gets an editor role (full CRUD + status read) and a viewer role (read-only + status read).

| CRD | API Group | Editor | Viewer |
|---|---|---|---|
| AgentEndpoint | ngrok.k8s.ngrok.com | Yes | Yes |
| CloudEndpoint | ngrok.k8s.ngrok.com | Yes | Yes |
| KubernetesOperator | ngrok.k8s.ngrok.com | Yes | Yes |
| NgrokTrafficPolicy | ngrok.k8s.ngrok.com | Yes | Yes |
| Domain | ingress.k8s.ngrok.com | Yes | Yes |
| IPPolicy | ingress.k8s.ngrok.com | Yes | Yes |
| BoundEndpoint | bindings.k8s.ngrok.com | Yes | Yes |

Annotations for RBAC aggregation (e.g., `rbac.authorization.k8s.io/aggregate-to-admin`) are configurable via `crdAccessRoles.annotations` in values.yaml.

## Verification

The Helm templates are the single source of truth for RBAC. Three layers of verification exist:

1. **Helm unit tests** — 134 tests with snapshot assertions covering every Role, ClusterRole, RoleBinding, and ClusterRoleBinding across both default and namespace-scoped modes. Run with `make helm-test`.
2. **Manifest bundle** — `manifest-bundle.yaml` is regenerated by `make manifest-bundle` and diffed in CI. Any RBAC change shows up as a diff.
3. **Go tests** — controller integration tests verify that controllers can perform the operations they need.

After RBAC changes, run:
```bash
make build && make test && make helm-update-snapshots && make manifest-bundle
```

For live cluster verification:
```bash
kubectl auth can-i --list \
  --as=system:serviceaccount:<ns>:<sa-name> \
  -n <ns>
```
