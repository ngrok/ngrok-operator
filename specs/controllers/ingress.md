# Ingress Controller

## Executive Summary

The Ingress controller reconciles Kubernetes `Ingress` resources that reference the operator's IngressClass. It uses a Driver pattern to collect Ingress state and materialize it as ngrok endpoint resources.

## Watches

| Resource              | Relation   | Predicate                              |
|-----------------------|------------|----------------------------------------|
| `Ingress`             | Primary    | Filters by IngressClass match          |
| `IngressClass`        | Secondary  | All events                             |
| `Service`             | Secondary  | All events                             |
| `Domain`              | Secondary  | All events                             |
| `NgrokTrafficPolicy`  | Secondary  | All events                             |

## Reconciliation Flow

1. Validate the Ingress references the operator's IngressClass.
2. If draining: delete from driver store without adding finalizer.
3. If deleted or no longer matching: remove finalizer and delete from store.
4. Add finalizer.
5. Update the Ingress in the Driver store.
6. Call `Driver.Sync()` to materialize endpoints.

## Driver Pattern

The Ingress controller uses a Driver that collects state from multiple Ingress resources and produces the desired set of ngrok endpoints:

1. Each Ingress is stored in the driver's internal state.
2. `Driver.Sync()` considers all stored Ingresses, Services, and Domains to generate the correct set of `AgentEndpoint` and/or `CloudEndpoint` resources.
3. This allows the driver to handle cross-resource concerns like shared domains.

## Created Resources

- `AgentEndpoint` and/or `CloudEndpoint` CRs (via Driver)
- `Domain` CRs (via Driver)

## Annotations

The following annotations on Ingress resources influence behavior:

- `k8s.ngrok.com/mapping-strategy`
- `k8s.ngrok.com/traffic-policy`
- `k8s.ngrok.com/pooling-enabled`
- `k8s.ngrok.com/description`
- `k8s.ngrok.com/metadata`

See [annotations.md](../annotations.md) for details.
