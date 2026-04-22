# AgentEndpoint Controller

## Executive Summary

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

## Status

| Field                    | Description                              |
|--------------------------|------------------------------------------|
| `assignedURL`            | The URL assigned by ngrok                |
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
