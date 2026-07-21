# Operator (api-manager) RBAC

Runs most controllers plus drain logic and needs the broadest access. Permissions
are bound to the `ngrok-operator` ServiceAccount across several roles, split by
where the underlying resources live and how they must be scoped. See
[README.md](README.md#namespace-scoping) for the cross-component scoping strategy
and the cache/RBAC caveats.

Roles bound to this ServiceAccount:

- a watchNamespace-following Role/ClusterRole (user workloads)
- an always-cluster-scoped ClusterRole (cluster-scoped K8s resources)
- a release-namespace leader-election Role
- a release-namespace operator-state Role
- an unconditional cluster-wide bindings ClusterRole

## Cluster-scoped resources (always ClusterRole)

### Core API (`""`)

| Resource | Verbs | Used by |
|---|---|---|
| `namespaces` | get, list, watch | HTTPRoute (cross-ns refs), Namespace controller |

### Networking (`networking.k8s.io`)

| Resource | Verbs | Used by |
|---|---|---|
| `ingressclasses` | get, list, watch | Ingress controller (class filtering) |

### Gateway API (`gateway.networking.k8s.io`)

| Resource | Verbs | Used by |
|---|---|---|
| `gatewayclasses` | get, list, patch, update, watch | GatewayClass controller |
| `gatewayclasses/status` | get, list, patch, update, watch | GatewayClass controller |
| `gatewayclasses/finalizers` | patch, update | GatewayClass controller |

## Namespace-scoped resources (Role when watchNamespace set, else ClusterRole)

### Core API (`""`)

| Resource | Verbs | Used by |
|---|---|---|
| `configmaps` | get, list, watch | Gateway `frontendValidation` CA-bundle reads; driver resource store |
| `events` | create, patch | Event recording across all controllers |
| `secrets` | get, list, watch | Ingress/Gateway (TLS reads). Write access (`create, patch, update`) is granted separately by the release-namespace-only `operator-state-role` so the api-manager cannot mutate Secrets outside its release namespace. |
| `services` | get, list, patch, update, watch | Service controller (status/annotations), Ingress/Gateway backend resolution. Service create/delete is the bindings poller (bindings ClusterRole), not here. |
| `services/finalizers` | patch, update | Service controller |
| `services/status` | get, list, patch, update, watch | Service controller |

### Events (`events.k8s.io`)

| Resource | Verbs | Used by |
|---|---|---|
| `events` | create, patch, update | Event recording across all controllers |

### Networking (`networking.k8s.io`)

| Resource | Verbs | Used by |
|---|---|---|
| `ingresses` | get, list, patch, update, watch | Ingress controller |
| `ingresses/finalizers` | patch, update | Ingress controller |
| `ingresses/status` | get, list, update, watch | Ingress controller |

### Gateway API (`gateway.networking.k8s.io`)

| Resource | Verbs | Used by |
|---|---|---|
| `gateways` | get, list, patch, update, watch | Gateway controller |
| `gateways/finalizers` | patch, update | Gateway controller |
| `gateways/status` | get, list, update, watch | Gateway controller |
| `httproutes` | get, list, patch, update, watch | HTTPRoute controller |
| `httproutes/finalizers` | patch, update | HTTPRoute controller |
| `httproutes/status` | get, list, update, watch | HTTPRoute controller |
| `tcproutes` | get, list, patch, update, watch | TCPRoute controller |
| `tcproutes/finalizers` | patch, update | TCPRoute controller |
| `tcproutes/status` | get, list, update, watch | TCPRoute controller |
| `tlsroutes` | get, list, patch, update, watch | TLSRoute controller |
| `tlsroutes/finalizers` | patch, update | TLSRoute controller |
| `tlsroutes/status` | get, list, update, watch | TLSRoute controller |
| `referencegrants` | get, list, watch | ReferenceGrant controller |

### ngrok API (`ngrok.com`)

| Resource | Verbs | Used by |
|---|---|---|
| `domains` | create, delete, get, list, patch, update, watch | Domain controller, Drain |
| `domains/finalizers` | patch, update | Domain controller |
| `domains/status` | get, patch, update | Domain controller |
| `ippolicies` | delete, get, list, patch, update, watch | IPPolicy controller, Drain. No `create` — the operator never creates the k8s IPPolicy CR (cloud-API only). |
| `ippolicies/finalizers` | patch, update | IPPolicy controller |
| `ippolicies/status` | get, patch, update | IPPolicy controller |
| `agentendpoints` | create, delete, get, list, patch, update, watch | Drain (cleanup), driver (creates from ingress/gateway) |
| `agentendpoints/finalizers` | patch, update | AgentEndpoint lifecycle |
| `agentendpoints/status` | get, patch, update | AgentEndpoint lifecycle |
| `cloudendpoints` | create, delete, get, list, patch, update, watch | CloudEndpoint controller, Drain |
| `cloudendpoints/finalizers` | patch, update | CloudEndpoint controller |
| `cloudendpoints/status` | get, patch, update | CloudEndpoint controller |
| `trafficpolicies` | get, list, watch | TrafficPolicy controller — no spec writes or finalizer (resolves policy refs) |
| `trafficpolicies/status` | get, patch, update | TrafficPolicy controller — writes `Ready`/`Valid` validation conditions |

## Leader election (always Role in release namespace)

### Core API (`""`)

| Resource | Verbs | Used by |
|---|---|---|
| `configmaps` | create, delete, get, list, patch, update, watch | controller-runtime leader election |
| `events` | create, patch | Leader election event recording |

### Coordination (`coordination.k8s.io`)

| Resource | Verbs | Used by |
|---|---|---|
| `leases` | create, delete, get, list, patch, update, watch | controller-runtime leader election |

## Operator state (always Role in release namespace)

These resources live in the release namespace regardless of `watchNamespace` because they are owned by the operator itself, not by the user. Confining writes here also prevents the api-manager from mutating arbitrary Secrets cluster-wide. There is no `delete` verb on secrets — the operator never deletes them.

### Core API (`""`)

| Resource | Verbs | Used by |
|---|---|---|
| `secrets` | create, get, list, patch, update, watch | KubernetesOperator TLS cert creation/rotation in `findOrCreateTLSSecret` (writes to `r.K8sOpNamespace` = release ns). Reads are granted here so `CreateOrUpdate` can `Get` the existing secret before deciding whether to create, even when `watchNamespace ≠ release ns`. |

### ngrok API (`ngrok.com`)

| Resource | Verbs | Used by |
|---|---|---|
| `kubernetesoperators` | create, get, list, patch, update, watch | KubernetesOperator controller — singleton CR for the operator's own state. No `delete` (the cleanup hook owns deletion, via its own role). |
| `kubernetesoperators/finalizers` | patch, update | KubernetesOperator controller |
| `kubernetesoperators/status` | get, patch, update | KubernetesOperator controller |

## Bindings (always cluster-wide ClusterRole, unconditional)

The BoundEndpoint controller (binding poller) reconciles BoundEndpoint CRs and creates Kubernetes Services in any namespace based on the BoundEndpoint's top-level domain. Both are inherently cluster-wide and are not constrained by `watchNamespace`.

These rules are **not** gated on `bindings.enabled`. The BoundEndpoint CRD is always installed (it ships in the unconditional `ngrok-crds` subchart), and the drain orchestrator in `internal/drain/drain.go` unconditionally lists BoundEndpoints during operator shutdown. Without these grants, drain would block on a forbidden cache list and the KubernetesOperator finalizer would never be released. The cross-namespace Service write rules are inert when `bindings.enabled=false` (the BoundEndpoint poller doesn't run, so nothing creates Services), but kept here for symmetry and to match `main`'s behavior.

### ngrok API (`ngrok.com`)

| Resource | Verbs | Used by |
|---|---|---|
| `boundendpoints` | create, delete, get, list, patch, update, watch | BoundEndpoint controller, Drain |
| `boundendpoints/finalizers` | patch, update | BoundEndpoint controller |
| `boundendpoints/status` | get, patch, update | BoundEndpoint controller |

### Core API (`""`)

| Resource | Verbs | Used by |
|---|---|---|
| `services` | create, delete, get, list, patch, update, watch | Binding poller creates Services for bound endpoints in any namespace |
| `services/finalizers` | patch, update | Binding poller |
| `services/status` | get, list, patch, update, watch | Binding poller |
