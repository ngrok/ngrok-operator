# Kubernetes Gateway API Feature

## Overview

The ngrok-operator supports the Kubernetes Gateway API, providing a role-oriented and extensible alternative to Ingress for managing external access to services.

## Configuration

| Helm Value                         | Description                                          | Default |
|------------------------------------|------------------------------------------------------|---------|
| `gateway.enabled`                  | Enable Gateway API support (if CRDs detected)        | `true`  |
| `gateway.disableReferenceGrants`   | Disable ReferenceGrant requirement                   | `false` |

## Supported Resources

| Resource         | API Version                          | Description                        |
|------------------|--------------------------------------|------------------------------------|
| `GatewayClass`   | `gateway.networking.k8s.io/v1`       | Defines the controller type        |
| `Gateway`        | `gateway.networking.k8s.io/v1`       | Configures listeners and addresses |
| `HTTPRoute`      | `gateway.networking.k8s.io/v1`       | HTTP routing rules                 |
| `TCPRoute`       | `gateway.networking.k8s.io/v1alpha2` | TCP routing rules                  |
| `TLSRoute`       | `gateway.networking.k8s.io/v1alpha2` | TLS routing rules                  |
| `ReferenceGrant` | `gateway.networking.k8s.io/v1beta1`  | Cross-namespace reference grants   |

## Behavior

When enabled and Gateway API CRDs are detected:

1. The operator registers a `GatewayClass` with its controller name.
2. `Gateway` resources referencing the operator's GatewayClass are reconciled.
3. Route resources (`HTTPRoute`, `TCPRoute`, `TLSRoute`) referencing a managed Gateway are materialized as ngrok endpoints.
4. `ReferenceGrant` resources enable cross-namespace references (e.g., a route in namespace A referencing a service in namespace B).

## Driver Pattern

Gateway API controllers use a Driver pattern:

1. Controllers update an internal store with the desired state from Gateway API resources.
2. `Driver.Sync()` is called to materialize the combined state as ngrok endpoint resources.
3. This allows the driver to consider the full picture (Gateway + Routes + Services) when generating endpoints.

## Annotations

The following annotations on Gateway API resources influence behavior:

- `k8s.ngrok.com/mapping-strategy` — Controls endpoint creation strategy
- `k8s.ngrok.com/traffic-policy` — References an NgrokTrafficPolicy
- `k8s.ngrok.com/pooling-enabled` — Enables endpoint pooling
- `k8s.ngrok.com/description` — Sets endpoint description
- `k8s.ngrok.com/metadata` — Sets endpoint metadata

## ReferenceGrants

By default, cross-namespace references require a `ReferenceGrant` in the target namespace. This can be disabled via `gateway.disableReferenceGrants: true`, which allows cross-namespace references without explicit grants.

## When Disabled

When `gateway.enabled: false` or Gateway API CRDs are not installed:
- Gateway API resources are not watched or reconciled
- The feature is excluded from the KubernetesOperator's `enabledFeatures`
