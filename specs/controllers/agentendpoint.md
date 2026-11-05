# AgentEndpoint Controller

## Summary

The AgentEndpoint controller reconciles `AgentEndpoint` resources by creating and managing ngrok agent endpoints. It ensures associated domains exist, resolves traffic policies, fetches client certificates, and keeps endpoint status up to date.

During the migration window it reconciles both the canonical `ngrok.com/v1` `AgentEndpoint` and the deprecated `ngrok.k8s.ngrok.com/v1alpha1` `AgentEndpoint`, as a pair keyed by namespace/name. See [Migration from `ngrok.k8s.ngrok.com`](#migration-from-ngrokk8sngrokcom).

## Watches

| Resource              | Relation   | Predicate                                    |
|-----------------------|------------|----------------------------------------------|
| `AgentEndpoint` (`ngrok.com/v1`) | Primary    | AnnotationChanged or GenerationChanged       |
| `AgentEndpoint` (`ngrok.k8s.ngrok.com/v1alpha1`, deprecated) | Same namespace/name key as its v1 twin | AnnotationChanged or GenerationChanged |
| `TrafficPolicy`  | Secondary  | Indexed by `spec.trafficPolicy.targetRef`; DELETE events filtered |
| `Secret`              | Secondary  | Secrets referenced by client certs or TLS termination        |
| `Domain`              | Owned      | All events                                   |

## Reconciliation Flow

1. Ensure the associated Domain exists via `DomainManager.EnsureDomainExists()`.
2. Fetch the traffic policy (by reference or inline).
3. Fetch client certificates from referenced Secrets.
4. Create or update the ngrok agent endpoint via `AgentDriver`.
5. Update status conditions and fields.
6. Call `ReconcileStatus()`.

## Created Resources

- ngrok agent endpoint (via AgentDriver API)
- `Domain` CR (via DomainManager)

A `Domain` whose reservation was skipped because a wildcard parent already
covers it satisfies `DomainReady` like any other ready Domain, and the
`CoveredByWildcardDomain` reason propagates onto the endpoint's condition. See
[domain.md](domain.md#wildcard-domain-coverage).

## TLS Termination (Zero-Knowledge TLS)

AgentEndpoints support agent-side TLS termination via `spec.tlsTermination`. When configured, the ngrok agent decrypts TLS traffic in-cluster before forwarding to the upstream service, so ngrok never sees the plaintext — hence "zero-knowledge" TLS.

`spec.url` must use the `tls://` scheme when `tlsTermination` is set (enforced by XValidation).

### `spec.tlsTermination`

| Field                  | Type           | Description |
|------------------------|----------------|-------------|
| `serverCertificateRef` | `K8sObjectRef` | Reference to a `kubernetes.io/tls` Secret containing the server certificate and key (`tls.crt`, `tls.key`). Must be in the same namespace as the AgentEndpoint. |
| `mutualTLS`            | object         | Optional. When set, enables mTLS — the agent requires or requests client certificates during the TLS handshake. |

### `spec.tlsTermination.mutualTLS`

| Field      | Type           | Description |
|------------|----------------|-------------|
| `clientCAsRef` | `K8sObjectRef` | Reference to a Secret whose `ca.crt` key holds a PEM-encoded CA bundle trusted to sign client certificates. Must be in the same namespace as the AgentEndpoint. |
| `mode`     | string         | `"require"` (client cert required) or `"request"` (client cert requested but optional). |

The controller watches the referenced Secrets and re-reconciles when they change, so certificate rotations are picked up automatically.

## Status

| Field                    | Description                              |
|--------------------------|------------------------------------------|
| `assignedURL`            | The URL assigned by ngrok for this endpoint. For `endpoints-verbose` mapping, this is a `.internal` URL; for `endpoints` mapping, it is the public URL. |
| `attachedTrafficPolicy`  | `"none"`, `"inline"`, or policy ref name |
| `domainRef`              | Reference to the associated Domain CR    |

## Conditions

| Type               | Description                                      |
|--------------------|--------------------------------------------------|
| `EndpointCreated`  | Whether the ngrok agent endpoint was created      |
| `TrafficPolicy`    | Whether the traffic policy was applied            |
| `DomainReady`      | Whether the associated Domain is ready            |
| `Ready`            | Aggregates all conditions and domain status       |

## Migration from `ngrok.k8s.ngrok.com`

A deprecated and a canonical `AgentEndpoint` with the same namespace and name are twins. The agent driver keys its tunnel by namespace/name, so both address the same agent endpoint. Each pass loads both:

- **One writer:** while the canonical object exists and isn't being deleted, only its spec reaches the driver. The deprecated object reports `Ready=True` with reason `SupersededByV1` and its other status is left as it was. A user-created deprecated object gets a `DeprecatedAPIGroup` warning event when it becomes passive or is edited; an operator-generated one (a controller-name label, set by the api-manager, or a Service controller owner) doesn't. Once the deprecated object's status is settled it isn't rewritten.
- **Deletes:** the tunnel is closed only when no remaining twin would still use it. Just before closing, the twin is re-read without the cache, failing closed: NotFound or an unserved kind means no twin; Forbidden means the twin is treated as present and the tunnel is kept; any other error requeues. Deleting the deprecated object while its twin is alive releases its finalizer without touching the tunnel. Deleting the canonical object hands the tunnel back to its twin in the same pass. Every agent replica binds every AgentEndpoint but only one removes the finalizer, so a replica that reconciles a key with no object of either kind re-reads both kinds without the cache and closes its own tunnel if neither exists.
- **Drain:** only deletes run. Drain deletes the deprecated kind first, so the canonical delete closes the tunnel.

The driver takes `agent.EndpointSpec`, which both kinds convert to, so twins with the same spec keep the live tunnel without a rebind.

These are the twin rules in [crd-group-migration.md](../crd-group-migration.md#twin-rules-r1).

## Events

- `Creating` / `Created`
- `Updating` / `Updated`
- `Deleting` / `Deleted`
- Error variants for each operation

## Error Handling

| Error                          | Behavior               |
|--------------------------------|------------------------|
| `ErrInvalidTrafficPolicyConfig`| No requeue             |
| `ErrDomainNotReady`            | Requeue after 10s      |
| Default                        | Via `CtrlResultForErr` |
