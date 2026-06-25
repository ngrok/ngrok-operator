# Agent RBAC

## ClusterRole

The agent (agent-manager) has a separate ClusterRole with permissions for tunnel management.

### ngrok API (`ngrok.com`)

| Resource                          | Verbs                                          |
|-----------------------------------|------------------------------------------------|
| `agentendpoints`                  | get, list, watch, patch, update                |
| `agentendpoints/finalizers`       | patch, update                                  |
| `agentendpoints/status`           | get, patch, update                             |
| `trafficpolicies`            | get, list, watch                               |
| `kubernetesoperators`             | get, list, watch                               |
| `domains`                         | create, delete, get, list, patch, update, watch |
| `domains/finalizers`              | patch, update                                  |
| `domains/status`                  | get, patch, update                             |

### Core API (`""`)

| Resource  | Verbs              |
|-----------|--------------------|
| `events`  | create, patch      |
| `secrets` | get, list, watch   |

## Notes

- The agent does not need write access to most CRDs — it only needs to update the AgentEndpoints it manages and the Domains they reference.
- TrafficPolicies and KubernetesOperators are read-only because the agent only reads their configuration.
- Secret read access is needed for client certificate references on AgentEndpoints.
