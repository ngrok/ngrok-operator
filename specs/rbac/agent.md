# Agent RBAC

Runs only the AgentEndpoint controller. Most resources are namespace-scoped and
follow `watchNamespace` (a Role when set, a ClusterRole in the default
all-namespace mode); `kubernetesoperators` is pinned to the release namespace
because the singleton CR always lives there. Bound to the `ngrok-operator-agent`
ServiceAccount. See [README.md](README.md#namespace-scoping) for the
cross-component scoping strategy.

## Namespace-scoped resources (Role when watchNamespace set, else ClusterRole)

### Core API (`""`)

| Resource | Verbs | Used by |
|---|---|---|
| `events` | create, patch | Event recording |
| `secrets` | get, list, watch | TLS certificate reads for AgentEndpoints |

### Events (`events.k8s.io`)

| Resource | Verbs | Used by |
|---|---|---|
| `events` | create, patch, update | Event recording |

### ngrok API (`ngrok.com`)

| Resource | Verbs | Used by |
|---|---|---|
| `domains` | create, delete, get, list, patch, update, watch | Auto-creates Domain resources for AgentEndpoints |
| `agentendpoints` | get, list, watch, patch, update | AgentEndpoint reconciler |
| `agentendpoints/finalizers` | patch, update | AgentEndpoint finalizer |
| `agentendpoints/status` | get, patch, update | AgentEndpoint status updates |
| `trafficpolicies` | get, list, watch | Resolves traffic policy refs |

## Operator state (always Role in release namespace)

The `KubernetesOperator` CR is the api-manager's singleton state object and always lives in the release namespace. The agent reads it for drain state via a release-namespace-pinned cache scope (`cache.Options.ByObject` in `cmd/agent-manager.go`), so RBAC is granted only in the release namespace.

### ngrok API (`ngrok.com`)

| Resource | Verbs | Used by |
|---|---|---|
| `kubernetesoperators` | get, list, watch | Reads drain state via `drain.StateChecker` |

## Notes

- The agent does not need write access to most CRDs — it only updates the AgentEndpoints it manages and the Domains they reference.
- TrafficPolicies and KubernetesOperators are read-only — the agent only reads their configuration.
- Secret read access is needed for client certificate references on AgentEndpoints.
