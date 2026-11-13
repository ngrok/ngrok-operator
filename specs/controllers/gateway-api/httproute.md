# HTTPRoute Controller

## Summary

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
5. Update the HTTPRoute in the Driver store.
6. Call `Driver.Sync()`, which also writes the route's status.

## Status

Written by the Driver at the end of every `Sync()`. The route gets one `status.parents` entry per parentRef that names a Gateway of an ngrok GatewayClass, with `group`, `kind` and `namespace` filled in and `controllerName: ngrok.com/gateway-controller`. Entries written by other controllers, including ones for the same parentRef, are kept as they are. Every condition carries the route's current `metadata.generation`.

| Type | Status | Reason | When |
|------|--------|--------|------|
| `Accepted` | `False` | `NoMatchingParent` | No listener matches the parentRef's `sectionName` and `port` |
| `Accepted` | `False` | `NotAllowedByListeners` | No selected, valid listener allows the route's kind and namespace |
| `Accepted` | `False` | `NoMatchingListenerHostname` | None of the route's hostnames intersects an allowing listener's |
| `Accepted` | `True` | `Accepted` | Otherwise |
| `ResolvedRefs` | `False` | `InvalidKind` | A backendRef isn't a core Service |
| `ResolvedRefs` | `False` | `RefNotPermitted` | A backendRef in another namespace without a ReferenceGrant |
| `ResolvedRefs` | `False` | `BackendNotFound` | A backendRef names a Service that doesn't exist |
| `ResolvedRefs` | `True` | `ResolvedRefs` | Otherwise, including rules without backendRefs |

An unresolved backend doesn't change `Accepted`: the route stays attached and the backend's share of its traffic gets a 500.

## Finalizer Behavior

The finalizer is **conditional**:
- Added only if the route references an ngrok-managed Gateway.
- Removed if the route no longer references an ngrok Gateway (e.g., parentRef changed).

## Created Resources

- `AgentEndpoint` and/or `CloudEndpoint` CRs (via Driver.Sync)
- `Domain` CRs (via Driver.Sync)

## Annotations

Endpoint-influencing annotations (`ngrok.com/mapping-strategy`, `ngrok.com/traffic-policy`, `ngrok.com/pooling-enabled`, `ngrok.com/description`, `ngrok.com/metadata`) are read from the parent **Gateway**, not from HTTPRoute resources — per-route annotation overrides are not supported. See [features/gateway-api.md](../../features/gateway-api.md).
