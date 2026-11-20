# Private Endpoint Forwarder RBAC

Runs only the Private Endpoint Forwarder controller. Watches PrivateEndpoints in
its own release namespace only (`cache.Options.DefaultNamespaces` in
`cmd/private-endpoint-forwarder-manager.go`), and Pods in every namespace
(`cache.Options.ByObject`) to find the Pod behind each connection. So it has a
namespaced Role and a ClusterRole for Pods. Bound to the
`ngrok-operator-private-endpoint-forwarder` ServiceAccount; only deploys when
`ngrok.features.privateEndpoints.enabled: true`.
See [README.md](README.md#namespace-scoping) for the cross-component scoping strategy.

## Namespace-scoped resources (Role in release namespace)

### ngrok API (`ngrok.com`)

| Resource | Verbs | Used by |
|---|---|---|
| `privateendpoints` | get, list, watch | Forwarder reconciler (routing table from status) |

## Cluster-scoped resources (ClusterRole)

### Core API (`""`)

| Resource | Verbs | Used by |
|---|---|---|
| `pods` | get, list, watch | Finds the calling Pod by source IP to send its [pod identity](../features/private-endpoints.md#pod-identity); not affected by `watchNamespace` |

## Notes

- The forwarder never writes to the Kubernetes API: it does not record Events, write status, or use leader election.
- The access token is mounted from the credentials Secret as an environment variable, so no Secret read access is needed.
- Pod access is read-only, the same as the bindings forwarder's. Any pod in the cluster can be a caller, so the ClusterRole is rendered whatever `watchNamespace` is.
- The Role, RoleBinding, ClusterRole and ClusterRoleBinding are all named `<fullname>-private-endpoint-forwarder` and live in `templates/private-endpoints/rbac.yaml`.
