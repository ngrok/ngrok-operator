# TCPRoute Controller

## Executive Summary

The TCPRoute controller reconciles `TCPRoute` resources (API version `gateway.networking.k8s.io/v1alpha2`) that reference an ngrok-managed Gateway. It follows the same pattern as the HTTPRoute controller.

## Watches

| Resource              | Relation   | Predicate                                        |
|-----------------------|------------|--------------------------------------------------|
| `TCPRoute`            | Primary    | `routeReferencesNgrokGateway` filter             |
| `GatewayClass`        | Secondary  | GenerationChanged                                |
| `Service`             | Secondary  | GenerationChanged                                |
| `Domain`              | Secondary  | GenerationChanged                                |
| Driver stored resources | Secondary | All events                                      |

## Reconciliation Flow

Same as [HTTPRoute](httproute.md) but for `TCPRoute` resources.

## Finalizer Behavior

Same conditional finalizer behavior as HTTPRoute.

## Created Resources

- `AgentEndpoint` and/or `CloudEndpoint` CRs (via Driver.Sync)
- `Domain` CRs (via Driver.Sync)
