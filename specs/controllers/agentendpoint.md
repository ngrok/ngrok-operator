# AgentEndpoint Controller

## Summary

The AgentEndpoint controller reconciles `AgentEndpoint` resources by creating and managing ngrok agent endpoints. It ensures associated domains exist, resolves traffic policies, fetches client certificates, and keeps endpoint status up to date.

## Watches

| Resource              | Relation   | Predicate                                    |
|-----------------------|------------|----------------------------------------------|
| `AgentEndpoint`       | Primary    | AnnotationChanged or GenerationChanged       |
| `TrafficPolicy`  | Secondary  | Indexed by `spec.trafficPolicyName`; DELETE events filtered |
| `Secret`              | Secondary  | Indexed by client certificate refs; DELETE events filtered |
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
| `caBundleRef` | `K8sObjectRef` | Reference to a Secret whose `ca.crt` key holds a PEM-encoded CA bundle trusted to sign client certificates. |
| `mode`     | string         | `"require"` (client cert required) or `"request"` (client cert requested but optional). |

The controller watches referenced Secrets (via secondary watch indexed by `spec.clientCertificateRefs`) and re-reconciles when they change so that certificate rotations are picked up automatically.

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
