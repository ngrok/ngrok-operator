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

## Notes

- The Gateway controller works in concert with the route controllers (HTTPRoute, TCPRoute, TLSRoute). The Driver considers both Gateway listeners and route rules when generating endpoints.
- Status is managed by the Driver during Sync.
