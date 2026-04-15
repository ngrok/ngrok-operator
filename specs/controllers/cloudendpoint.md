# CloudEndpoint Controller

## Executive Summary

The CloudEndpoint controller reconciles `CloudEndpoint` resources by creating and managing ngrok cloud endpoints via the ngrok API. It ensures associated domains exist, resolves traffic policies, and keeps the endpoint synchronized with the desired state.

## Watches

| Resource              | Relation   | Predicate                                    |
|-----------------------|------------|----------------------------------------------|
| `CloudEndpoint`       | Primary    | AnnotationChanged or GenerationChanged       |
| `TrafficPolicy`  | Secondary  | Indexed by `spec.trafficPolicyName`; DELETE events filtered |
| `Domain`              | Owned      | All events                                   |

## Reconciliation Flow

1. Ensure the associated Domain exists via `DomainManager.EnsureDomainExists()`.
2. Fetch the traffic policy (inline or by name).
3. Create or update the cloud endpoint via the ngrok API.
4. Update status with the endpoint ID, domain reference, and conditions.
5. Call `ReconcileStatus()`.

## Created Resources

- ngrok cloud endpoint (via ngrok API)
- `Domain` CR (via DomainManager)

## Status

| Field        | Description                            |
|--------------|----------------------------------------|
| `id`         | The ngrok API resource ID              |
| `domainRef`  | Reference to the associated Domain CR  |

## Conditions

| Type    | Description                                    |
|---------|------------------------------------------------|
| `Ready` | Overall readiness of the cloud endpoint        |

## Error Handling

| Error                          | Behavior                                       |
|--------------------------------|------------------------------------------------|
| Codes 18016, 18017             | Retryable (endpoint pooling state conflicts)   |
| `ErrDomainNotReady`            | Requeue after 10s                              |
| `ErrInvalidTrafficPolicyConfig`| No requeue                                     |
| Default                        | Via `CtrlResultForErr`                         |
