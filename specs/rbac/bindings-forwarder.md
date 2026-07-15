# Bindings Forwarder RBAC

Runs only the Forwarder controller. Watches its own release namespace for most
resources and watches Pods cluster-wide (`cache.AllNamespaces` in
`cmd/bindings-forwarder-manager.go`) so it can reconcile bindings against consumer
Pods in any namespace. As a result it always renders a small ClusterRole for Pods
in addition to its namespaced Role. Bound to the `ngrok-operator-bindings-forwarder`
ServiceAccount; only deploys when `bindings.enabled: true`. See
[README.md](README.md#namespace-scoping) for the cross-component scoping strategy.

## Namespace-scoped resources (Role in release namespace)

### Core API (`""`)

| Resource | Verbs | Used by |
|---|---|---|
| `events` | create, patch | Event recording |
| `secrets` | get, list, watch | TLS certificate reads (mTLS to the ngrok ingress endpoint) |

### Events (`events.k8s.io`)

| Resource | Verbs | Used by |
|---|---|---|
| `events` | create, patch, update | Event recording |

### ngrok API (`ngrok.com`)

| Resource | Verbs | Used by |
|---|---|---|
| `boundendpoints` | get, list, patch, update, watch | Forwarder reconciler |
| `kubernetesoperators` | get, list, watch | Reads binding configuration and the ingress endpoint address |

## Cluster-scoped resources (always ClusterRole)

### Core API (`""`)

| Resource | Verbs | Used by |
|---|---|---|
| `pods` | get, list, watch | Discovers consumer pods cluster-wide so the forwarder can reconcile bindings against pods in any namespace; not affected by `watchNamespace` |

## Notes

- Pod read access is cluster-wide because the forwarder looks up pods by IP address to identify connection sources. Pod IPs are indexed via a field indexer on `status.podIP`.
- Secret read access is for the TLS certificate used in mTLS communication with the ngrok ingress endpoint. The referenced Secret lives in the release namespace (named by the KubernetesOperator CR), so this stays a namespaced grant.
- KubernetesOperator read access is for binding configuration and the ingress endpoint address.
