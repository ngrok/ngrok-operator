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

The api-manager's permissions split into three categories based on where the underlying resources actually live, not just on `watchNamespace`:

- **User workloads** (Ingress, Gateway routes, AgentEndpoint, CloudEndpoint, Domain, IPPolicy, NgrokTrafficPolicy, Service, etc.) — follow `watchNamespace`. Role in the watched namespace, or ClusterRole when watchNamespace is unset.
- **Operator state** (KubernetesOperator CR, the operator's own TLS Secret writes) — always in the release namespace. The KubernetesOperator CR is a singleton owned by the operator and the TLS Secret is created in `r.K8sOpNamespace` (= release namespace), so these resources never live in a user-chosen `watchNamespace`.
- **Bindings** (BoundEndpoint CR, cross-namespace Service writes by the binding poller) — always cluster-wide. The poller creates Services in any namespace based on the BoundEndpoint's top-level domain. Even when `bindings.enabled=false`, the BoundEndpoint CRD is still installed (it ships in the unconditional `ngrok-crds` subchart) and the drain orchestrator unconditionally lists BoundEndpoints during shutdown, so the api-manager always needs these grants.

Cluster-scoped K8s resources (namespaces, ingressclasses, gatewayclasses) always require a ClusterRole regardless.

| Component | Default mode | watchNamespace mode |
|---|---|---|
| api-manager: user workloads | ClusterRole + ClusterRoleBinding | Role + RoleBinding (in watchNamespace) **plus** ClusterRole + ClusterRoleBinding (cluster-scoped K8s resources only) |
| api-manager: operator state | Role + RoleBinding (always release ns) | No change |
| api-manager: bindings (BoundEndpoint + cross-ns Services) | ClusterRole + ClusterRoleBinding | No change — cluster-wide by design |
| agent-manager: user workloads | ClusterRole + ClusterRoleBinding | Role + RoleBinding (in watchNamespace) |
| agent-manager: operator state (KubernetesOperator drain reads) | Role + RoleBinding (always release ns) | No change |
| bindings-forwarder | Role + RoleBinding (namespaced) **plus** ClusterRole + ClusterRoleBinding (pods only) | No change — Pod watch is cluster-wide by design |
| leader-election | Role + RoleBinding (always release ns) | No change |

> **Note**: the `bindings-forwarder` ClusterRole and the api-manager `bindings-cluster-role` both exist because the binding feature is inherently cluster-wide on both sides — the forwarder watches Pods anywhere that consumes the binding Service, and the poller creates the corresponding Service anywhere the BoundEndpoint targets. `watchNamespace` does not constrain either of these.
>
> **KubernetesOperator cache scope**: the `KubernetesOperator` CR is a singleton owned by the operator and always lives in the release namespace, regardless of `watchNamespace`. Both `cmd/api-manager.go` and `cmd/agent-manager.go` use a per-resource cache scope (`cache.Options.ByObject`) to pin this resource to the release namespace, so the cache list/watch matches the release-namespace-only RBAC grant. This makes both managers tolerate `watchNamespace ≠ release ns` for the KubernetesOperator reconciliation and drain-state read paths.
>
> **Remaining caveat**: the api-manager's `findOrCreateTLSSecret` writes the operator's TLS Secret to the release namespace. While the RBAC correctly grants those writes only in the release namespace, the controller-runtime cache for Secrets is still scoped to `watchNamespace` (via `DefaultNamespaces`). With `watchNamespace ≠ release ns`, the `Get` that precedes the create would miss the cache. As long as `watchNamespace = release ns` (the documented setup) this is fine. Pinning Secrets through `cache.Options.ByObject` like we do for KubernetesOperator would close this gap; that fix is out of scope for this RBAC restructure.

### Helm template organization

RBAC lives in per-component directories under `helm/ngrok-operator/templates/`:

```
api-manager/        role.yaml (watchNamespace-following Role/ClusterRole)
                    rolebinding.yaml
                    leader-election-role.yaml (always release ns — controller-runtime infra)
                    release-namespace-role.yaml (always release ns — KubernetesOperator CR + secret writes)
                    bindings-cluster-role.yaml (always cluster-wide and unconditional — BoundEndpoint + cross-ns Services; required even when bindings.enabled=false because drain always lists BoundEndpoints)
agent/              role.yaml (watchNamespace-following Role/ClusterRole)
                    rolebinding.yaml
                    release-namespace-role.yaml (always release ns — KubernetesOperator drain reads)
bindings-forwarder/ role.yaml (Role + RoleBinding + ClusterRole + ClusterRoleBinding)
rbac/crd-access/    editor/viewer ClusterRoles for end-users
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
| secrets | core | get, list, watch | Ingress/Gateway (TLS reads). Write access (`create, patch, update`) is granted separately by the release-namespace-only `secret-manager-role` so the api-manager cannot mutate Secrets outside its release namespace. |
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
| agentendpoints | ngrok.k8s.ngrok.com | create, delete, get, list, patch, update, watch | Drain (cleanup), driver (creates from ingress/gateway) |
| agentendpoints/finalizers | ngrok.k8s.ngrok.com | patch, update | AgentEndpoint lifecycle |
| agentendpoints/status | ngrok.k8s.ngrok.com | get, patch, update | AgentEndpoint lifecycle |
| cloudendpoints | ngrok.k8s.ngrok.com | create, delete, get, list, patch, update, watch | CloudEndpoint controller, Drain |
| cloudendpoints/finalizers | ngrok.k8s.ngrok.com | patch, update | CloudEndpoint controller |
| cloudendpoints/status | ngrok.k8s.ngrok.com | get, patch, update | CloudEndpoint controller |
| ngroktrafficpolicies | ngrok.k8s.ngrok.com | create, delete, get, list, patch, update, watch | NgrokTrafficPolicy controller |
| ngroktrafficpolicies/finalizers | ngrok.k8s.ngrok.com | patch, update | NgrokTrafficPolicy controller |
| ngroktrafficpolicies/status | ngrok.k8s.ngrok.com | get, patch, update | NgrokTrafficPolicy controller |

### Leader election (always namespaced Role in release NS)

| Resource | API Group | Verbs | Used by |
|---|---|---|---|
| configmaps | core | create, delete, get, list, patch, update, watch | controller-runtime leader election |
| leases | coordination.k8s.io | create, delete, get, list, patch, update, watch | controller-runtime leader election |
| events | core | create, patch | Leader election event recording |

### Operator state (always Role in release NS)

These resources live in the release namespace regardless of `watchNamespace` because they are owned by the operator itself, not by the user. Confining writes here also prevents the api-manager from mutating arbitrary Secrets cluster-wide.

| Resource | API Group | Verbs | Used by |
|---|---|---|---|
| secrets | core | create, get, list, patch, update, watch | KubernetesOperator TLS cert creation/rotation in `findOrCreateTLSSecret` (writes to `r.K8sOpNamespace` = release ns). Reads are granted here so `CreateOrUpdate` can `Get` the existing secret before deciding whether to create, even when `watchNamespace ≠ release ns`. Cluster-wide / watchNamespace-following secret reads are still granted by the api-manager Role for user-referenced TLS material. |
| kubernetesoperators | ngrok.k8s.ngrok.com | create, delete, get, list, patch, update, watch | KubernetesOperator controller — singleton CR for the operator's own state |
| kubernetesoperators/finalizers | ngrok.k8s.ngrok.com | patch, update | KubernetesOperator controller |
| kubernetesoperators/status | ngrok.k8s.ngrok.com | get, patch, update | KubernetesOperator controller |

### Bindings (always cluster-wide ClusterRole, unconditional)

The BoundEndpoint controller (binding poller) reconciles BoundEndpoint CRs and creates Kubernetes Services in any namespace based on the BoundEndpoint's top-level domain. Both are inherently cluster-wide and are not constrained by `watchNamespace`.

These rules are **not** gated on `bindings.enabled`. The BoundEndpoint CRD is always installed (it ships in the unconditional `ngrok-crds` subchart), and the drain orchestrator in `internal/drain/drain.go` unconditionally lists BoundEndpoints during operator shutdown. Without these grants, drain would block on a forbidden cache list and the KubernetesOperator finalizer would never be released. The cross-namespace Service write rules are inert when `bindings.enabled=false` (the BoundEndpoint poller doesn't run, so nothing creates Services), but kept here for symmetry and to match `main`'s behavior.

| Resource | API Group | Verbs | Used by |
|---|---|---|---|
| boundendpoints | bindings.k8s.ngrok.com | create, delete, get, list, patch, update, watch | BoundEndpoint controller, Drain |
| boundendpoints/finalizers | bindings.k8s.ngrok.com | patch, update | BoundEndpoint controller |
| boundendpoints/status | bindings.k8s.ngrok.com | get, patch, update | BoundEndpoint controller |
| services | core | create, delete, get, list, patch, update, watch | Binding poller creates Services for bound endpoints in any namespace |
| services/finalizers | core | patch, update | Binding poller |
| services/status | core | get, list, patch, update, watch | Binding poller |

## agent-manager permissions

Runs only the AgentEndpoint controller. Most resources are namespace-scoped and follow `watchNamespace`; `kubernetesoperators` is pinned to the release namespace because the singleton CR always lives there.

### Namespace-scoped resources (Role when watchNamespace set)

| Resource | API Group | Verbs | Used by |
|---|---|---|---|
| events | core | create, patch | Event recording |
| secrets | core | get, list, watch | TLS certificate reads for AgentEndpoints |
| domains | ingress.k8s.ngrok.com | create, delete, get, list, patch, update, watch | Auto-creates Domain resources for AgentEndpoints |
| agentendpoints | ngrok.k8s.ngrok.com | get, list, watch, patch, update | AgentEndpoint reconciler |
| agentendpoints/finalizers | ngrok.k8s.ngrok.com | patch, update | AgentEndpoint finalizer |
| agentendpoints/status | ngrok.k8s.ngrok.com | get, patch, update | AgentEndpoint status updates |
| ngroktrafficpolicies | ngrok.k8s.ngrok.com | get, list, watch | Resolves traffic policy refs |

### Operator state (always Role in release NS)

The `KubernetesOperator` CR is the api-manager's singleton state object and always lives in the release namespace. The agent reads it for drain state via a release-namespace-pinned cache scope (`cache.Options.ByObject` in `cmd/agent-manager.go`), so RBAC is granted only in the release namespace.

| Resource | API Group | Verbs | Used by |
|---|---|---|---|
| kubernetesoperators | ngrok.k8s.ngrok.com | get, list, watch | Reads drain state via `drain.StateChecker` |

## bindings-forwarder permissions

Runs only the Forwarder controller. Watches its own release namespace for most resources and watches Pods cluster-wide (`cache.AllNamespaces` in `cmd/bindings-forwarder-manager.go`) so it can reconcile bindings against consumer Pods in any namespace. As a result the bindings-forwarder always renders a small ClusterRole for Pods in addition to its namespaced Role.

### Namespace-scoped resources (Role in release namespace)

| Resource | API Group | Verbs | Used by |
|---|---|---|---|
| events | core | create, patch | Event recording |
| secrets | core | get, list, watch | TLS certificate reads |
| boundendpoints | bindings.k8s.ngrok.com | get, list, patch, update, watch | Forwarder reconciler |
| kubernetesoperators | ngrok.k8s.ngrok.com | get, list, watch | Reads drain state |

### Cluster-scoped resources (always ClusterRole)

| Resource | API Group | Verbs | Used by |
|---|---|---|---|
| pods | core | get, list, watch | Discovers consumer pods cluster-wide so the forwarder can reconcile bindings against pods in any namespace; not affected by `watchNamespace` |

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
