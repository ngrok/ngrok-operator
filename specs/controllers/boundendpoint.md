# BoundEndpoint Controller

## Executive Summary

The BoundEndpoint controller reconciles `BoundEndpoint` resources by creating two Kubernetes Services for each binding: a target ExternalName service in the destination namespace and an upstream ClusterIP service in the operator namespace. It verifies TCP connectivity through the service chain.

## Watches

| Resource         | Relation   | Predicate                              |
|------------------|------------|----------------------------------------|
| `BoundEndpoint`  | Primary    | GenerationChanged or AnnotationChanged |
| `Service`        | Secondary  | Indexed by label `bindings.k8s.ngrok.com/endpoint-binding-name` |
| `Namespace`      | Secondary  | Indexed by `spec.targetNamespace`      |

## Reconciliation Flow

1. Convert the BoundEndpoint spec into two Service definitions.
2. Create or update both Services:
   - **Target Service**: `ExternalName` type in the target namespace, providing a local DNS name.
   - **Upstream Service**: `ClusterIP` type in the operator namespace, pointing to bindings forwarder pods.
3. Test TCP connectivity through the service chain (8 retries, exponential backoff, max 10s timeout).
4. Update status with service references and conditions.
5. Requeue after a refresh interval for periodic health checks.

## Created Resources

- Target `Service` (ExternalName) in the target namespace
- Upstream `Service` (ClusterIP) in the operator namespace

## Status

| Field                | Description                                  |
|----------------------|----------------------------------------------|
| `targetServiceRef`   | Reference to the created target service      |
| `upstreamServiceRef` | Reference to the created upstream service    |

## Conditions

| Type                   | Description                                       |
|------------------------|---------------------------------------------------|
| `ServicesCreated`      | Whether both target and upstream services exist   |
| `ConnectivityVerified` | Whether TCP connectivity through services works   |
| `Ready`                | Overall readiness                                 |

## Notes

- The `endpoints`, `endpointsSummary`, and `hashedName` status fields are managed by the poller, not this controller.
- The controller re-queues periodically to verify connectivity remains healthy.
- See [features/bindings.md](../features/bindings.md) for the full feature overview.
