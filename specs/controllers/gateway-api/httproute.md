# HTTPRoute Controller

## Executive Summary

The HTTPRoute controller reconciles `HTTPRoute` resources that reference an ngrok-managed Gateway. It updates the Driver store and triggers synchronization to materialize the routes as ngrok endpoints.

## Watches

| Resource              | Relation   | Predicate                                        |
|-----------------------|------------|--------------------------------------------------|
| `HTTPRoute`           | Primary    | `routeReferencesNgrokGateway` filter             |
| `GatewayClass`        | Secondary  | GenerationChanged                                |
| `Service`             | Secondary  | GenerationChanged                                |
| `Domain`              | Secondary  | GenerationChanged                                |
| Driver stored resources | Secondary | All events                                      |

## Reconciliation Flow

1. If deleted or no longer references ngrok Gateway: remove finalizer, delete from Driver store.
2. Verify the route references an ngrok-managed Gateway.
3. If draining: skip.
4. Add finalizer.
5. Validate the HTTPRoute.
6. Update the HTTPRoute in the Driver store.
7. Call `Driver.Sync()`.

## Finalizer Behavior

The finalizer is **conditional**:
- Added only if the route references an ngrok-managed Gateway.
- Removed if the route no longer references an ngrok Gateway (e.g., parentRef changed).

## Created Resources

- `AgentEndpoint` and/or `CloudEndpoint` CRs (via Driver.Sync)
- `Domain` CRs (via Driver.Sync)

## Annotations

The following annotations on HTTPRoute resources influence behavior:

- `k8s.ngrok.com/mapping-strategy`
- `k8s.ngrok.com/traffic-policy`
- `k8s.ngrok.com/pooling-enabled`
- `k8s.ngrok.com/description`
- `k8s.ngrok.com/metadata`
