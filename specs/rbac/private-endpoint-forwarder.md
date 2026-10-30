# Private Endpoint Forwarder RBAC

Runs only the Private Endpoint Forwarder controller. Watches PrivateEndpoints in
its own release namespace only (`cache.Options.DefaultNamespaces` in
`cmd/private-endpoint-forwarder-manager.go`), so it needs a single namespaced
Role and no ClusterRole. Bound to the `ngrok-operator-private-endpoint-forwarder`
ServiceAccount; only deploys when `ngrok.features.privateEndpoints.enabled: true`.
See [README.md](README.md#namespace-scoping) for the cross-component scoping strategy.

## Namespace-scoped resources (Role in release namespace)

### ngrok API (`ngrok.com`)

| Resource | Verbs | Used by |
|---|---|---|
| `privateendpoints` | get, list, watch | Forwarder reconciler (routing table from status) |

## Notes

- The forwarder never writes to the Kubernetes API: it does not record Events, write status, or use leader election.
- The access token is mounted from the credentials Secret as an environment variable, so no Secret read access is needed.
- Unlike the bindings forwarder, it does not look up Pods, so there is no Pod access.
- The Role and RoleBinding are named `<fullname>-private-endpoint-forwarder` and live in `templates/private-endpoints/rbac.yaml`.
