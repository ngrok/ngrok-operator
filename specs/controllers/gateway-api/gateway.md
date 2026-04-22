# Gateway Controller

## Executive Summary

The Gateway controller reconciles `Gateway` resources that reference an ngrok-managed GatewayClass. It validates the Gateway, updates the Driver store, and triggers synchronization to materialize endpoints.

## Watches

| Resource  | Relation | Predicate |
|-----------|----------|-----------|
| `Gateway` | Primary  | None      |

## Reconciliation Flow

1. If deleted: remove from Driver store, call `Driver.Sync()`, remove finalizer.
2. Verify the referenced GatewayClass exists and is managed by ngrok.
3. If draining: skip.
4. Add finalizer.
5. Validate the Gateway.
6. Update the Gateway in the Driver store.
7. Call `Driver.Sync()` to materialize endpoints.

## Created Resources

- `AgentEndpoint` and/or `CloudEndpoint` CRs (via Driver.Sync)
- `Domain` CRs (via Driver.Sync)

## Status

Status is managed by the Driver during `Sync()`.

### Addresses

`status.addresses` is populated with hostname addresses resolved from Domain CRs associated with the Gateway's listeners:

- Address type: `Hostname`
- Value: the Domain's `status.cnameTarget` if present (custom domains), otherwise `status.domain` (ngrok-managed domains). Wildcard prefixes are trimmed.

### Conditions

#### Gateway-level

| Type         | Status | Reason       | Description                          |
|--------------|--------|--------------|--------------------------------------|
| `Programmed` | `True` | `Programmed` | Set when the Gateway has been synced |

#### Per-listener (`status.listeners[].conditions`)

| Type         | Status | Reason       | Description                                              |
|--------------|--------|--------------|----------------------------------------------------------|
| `Programmed` | `True` | `Programmed` | Set for each listener whose `Accepted` condition is not False |

## Error Handling

| Error               | Behavior                                                    |
|---------------------|-------------------------------------------------------------|
| Status update conflict | Retried via `retry.RetryOnConflict` with concurrency limit of 4 |
| Gateway not found   | Silently skipped (Gateway was deleted during sync)          |

## Notes

- The Gateway controller works in concert with the route controllers (HTTPRoute, TCPRoute, TLSRoute). The Driver considers both Gateway listeners and route rules when generating endpoints.
- HTTPRoute status updates are not yet implemented in the Driver.
