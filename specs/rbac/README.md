# RBAC

## Overview

The ngrok-operator uses Kubernetes Role-Based Access Control (RBAC) to authorize its components to interact with the Kubernetes API. Three separate components each have their own ServiceAccount and RBAC configuration:

| Component              | Scope               | Purpose                                            |
|------------------------|---------------------|----------------------------------------------------|
| Operator (api-manager) | Cluster + Namespace | Main controller managing all CRDs and resources    |
| Agent                  | Cluster             | Agent tunnel management                            |
| Bindings Forwarder     | Cluster + Namespace | Endpoint binding forwarding                        |

## Architecture

| Deployment          | ServiceAccount                          | Controllers                                                                                                                                                            | Conditional?          |
|---------------------|-----------------------------------------|------------------------------------------------------------------------------------------------------------------------------------------------------------------------|-----------------------|
| api-manager         | `ngrok-operator`                        | Ingress, Domain, IPPolicy, CloudEndpoint, TrafficPolicy, KubernetesOperator, BoundEndpoint, Gateway, HTTPRoute, TCPRoute, TLSRoute, GatewayClass, Namespace, ReferenceGrant, Service + Drain | No |
| agent-manager       | `ngrok-operator-agent`                  | AgentEndpoint                                                                                                                                                          | Yes (`ingress.enabled`) |
| bindings-forwarder  | `ngrok-operator-bindings-forwarder`     | Forwarder                                                                                                                                                              | Yes (`bindings.enabled`) |

### Management approach

All RBAC is hand-managed in Helm templates. `controller-gen` is used only for CRDs and webhooks, not for RBAC generation. This follows the pattern used by cert-manager, Kyverno, ArgoCD, and other multi-deployment operators.

### Namespace-scoping

The api-manager's permissions split into three categories based on where the underlying resources actually live, not just on `watchNamespace`:

- **User workloads** (Ingress, Gateway routes, AgentEndpoint, CloudEndpoint, Domain, IPPolicy, TrafficPolicy, Service, etc.) — follow `watchNamespace`. Role in the watched namespace, or ClusterRole when watchNamespace is unset.
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
> **KubernetesOperator cache scope**: the `KubernetesOperator` CR is a singleton owned by the operator and always lives in the release namespace, regardless of `watchNamespace`. Both `cmd/api-manager.go` and `cmd/agent-manager.go` use a per-resource cache scope (`cache.Options.ByObject`) to pin this resource to the release namespace, so the cache list/watch matches the release-namespace-only RBAC grant.
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

## Design Principles

- **Least privilege**: Each component only has the permissions it needs.
- **Cluster-scoped for CRDs**: CRDs and cross-namespace resources require ClusterRoles.
- **Namespace-scoped for internal state**: Leader election and secret management use namespaced Roles.
- **Conditional permissions**: Some permissions (e.g., secret management for bindings) are only granted when the corresponding feature is enabled.

## Component permissions

Each component's full permission tables — with per-rule `Used by` attribution — live in its own file, matching the one-ServiceAccount-per-component split above. No role is shared between components; each ServiceAccount is bound only to its own component's roles.

- **api-manager** — [operator.md](operator.md): the broadest set. A watchNamespace-following Role/ClusterRole, an always-cluster-scoped ClusterRole, plus release-namespace leader-election and operator-state Roles and an unconditional cluster-wide bindings ClusterRole.
- **agent-manager** — [agent.md](agent.md): AgentEndpoint controller. A watchNamespace-following Role/ClusterRole plus a release-namespace operator-state Role.
- **bindings-forwarder** — [bindings-forwarder.md](bindings-forwarder.md): Forwarder controller. A namespaced Role plus a cluster-wide Pods ClusterRole.

## Cleanup hook

Helm pre-delete hook that cleans up KubernetesOperator resources. Uses a dedicated ServiceAccount with a Role scoped to the release namespace.

| Resource | API Group | Verbs | Used by |
|---|---|---|---|
| kubernetesoperators | ngrok.com | delete, get, list, watch | Pre-delete cleanup job |

## CRD access roles

End-user ClusterRoles for granting read/write access to operator CRDs via RBAC aggregation. These are not bound to any operator ServiceAccount. Users bind them to their own subjects.

Each CRD gets an editor role (full CRUD + status read) and a viewer role (read-only + status read). See [aggregation.md](aggregation.md) for the full role definitions.

| CRD | API Group | Editor | Viewer |
|---|---|---|---|
| AgentEndpoint | ngrok.com | Yes | Yes |
| CloudEndpoint | ngrok.com | Yes | Yes |
| KubernetesOperator | ngrok.com | Yes | Yes |
| TrafficPolicy | ngrok.com | Yes | Yes |
| Domain | ngrok.com | Yes | Yes |
| IPPolicy | ngrok.com | Yes | Yes |
| BoundEndpoint | ngrok.com | Yes | Yes |

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
