# Kubernetes Gateway API Feature

## Overview

The ngrok-operator supports the Kubernetes Gateway API, providing a role-oriented and extensible alternative to Ingress for managing external access to services.

> **Prerequisites**: The Gateway API CRDs are **not** installed by the ngrok-operator. You must install them separately before enabling this feature. See the [Gateway API installation guide](https://gateway-api.sigs.k8s.io/guides/#installing-gateway-api). The operator supports Gateway API `v1` (stable) resources and `v1alpha2` for TCPRoute and TLSRoute.

## Configuration

| Helm Value                         | Description                                          | Default |
|------------------------------------|------------------------------------------------------|---------|
| `features.gateway.enabled`                  | Enable Gateway API support (if CRDs detected)        | `true`  |
| `features.gateway.disableReferenceGrants`   | Disable ReferenceGrant requirement                   | `false` |

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

1. A user creates a `GatewayClass` resource with `spec.controllerName` matching the operator's controller name (`ngrok.com/gateway-controller`).
2. `Gateway` resources referencing that GatewayClass are reconciled by the operator.
3. Route resources (`HTTPRoute`, `TCPRoute`, `TLSRoute`) referencing a managed Gateway are materialized as ngrok endpoints.
4. `ReferenceGrant` resources enable cross-namespace references (e.g., a route in namespace A referencing a service in namespace B).

## Driver Pattern

Gateway API controllers use a Driver pattern:

1. Controllers update an internal store with the desired state from Gateway API resources.
2. `Driver.Sync()` is called to materialize the combined state as ngrok endpoint resources.
3. This allows the driver to consider the full picture (Gateway + Routes + Services) when generating endpoints.

## Annotations

The following annotations influence behavior. They are supported on `Gateway` resources only — per-route annotation overrides are not supported.

- `ngrok.com/mapping-strategy` — Controls endpoint creation strategy
- `ngrok.com/traffic-policy` — References a TrafficPolicy
- `ngrok.com/pooling-enabled` — Enables endpoint pooling
- `ngrok.com/description` — Sets endpoint description
- `ngrok.com/metadata` — Sets endpoint metadata

See [annotations.md](../annotations.md) for details.

## Listener TLS Options

Keys in a Gateway listener's `spec.listeners[].tls.options` map with the `ngrok.com/terminate-tls.` prefix configure the `terminate-tls` traffic-policy action on endpoints generated for that listener; the suffix after the prefix becomes the action option name.

```yaml
listeners:
- name: https
  port: 443
  protocol: HTTPS
  tls:
    options:
      ngrok.com/terminate-tls.min_version: "1.3"
```

| Detail          | Value                                                  |
|-----------------|--------------------------------------------------------|
| Key form        | `ngrok.com/terminate-tls.<option>`                     |
| Reserved suffixes (rejected) | `server_private_key`, `server_certificate`, `mutual_tls_certificate_authorities` — managed via `certificateRefs`, not options |

Keys without the prefix are ignored. Supplying a reserved suffix fails translation for the Gateway.

During the `k8s.ngrok.com/` → `ngrok.com/` migration window the operator also reads the legacy `k8s.ngrok.com/terminate-tls.` prefix; when both prefixes define the same option suffix, the `ngrok.com/` key wins. Legacy support is removed in 1.0 — see [`docs/v1-migration-guide.md`](../../docs/v1-migration-guide.md).

## ReferenceGrants

By default, cross-namespace references require a `ReferenceGrant` in the target namespace. This can be disabled via `gateway.disableReferenceGrants: true`, which allows cross-namespace references without explicit grants.

## When Disabled

When `gateway.enabled: false` or Gateway API CRDs are not installed:
- Gateway API resources are not watched or reconciled
- The feature is excluded from the KubernetesOperator's `enabledFeatures`
