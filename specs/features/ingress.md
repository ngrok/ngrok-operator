# Kubernetes Ingress Feature

## Overview

The ngrok-operator can function as a Kubernetes Ingress controller, watching Ingress resources and materializing them as ngrok endpoints.

## Configuration

| Helm Value                                    | Description                                      | Default                          |
|-----------------------------------------------|--------------------------------------------------|----------------------------------|
| `features.ingress.enabled`                    | Enable the Ingress controller                    | `true`                           |
| `features.ingress.controllerName`             | Controller name for IngressClass matching        | `ngrok.com/ingress-controller`   |
| `features.ingress.watchNamespace`             | Namespace to watch (empty = all)                 | `""`                             |
| `features.ingress.ingressClass.name`          | IngressClass resource name                       | `ngrok`                          |
| `features.ingress.ingressClass.create`        | Create the IngressClass resource                 | `true`                           |
| `features.ingress.ingressClass.default`       | Set as the default IngressClass                  | `false`                          |

## Behavior

When enabled, the operator:

1. Creates an `IngressClass` resource (if `ingress.ingressClass.create` is true) with the configured controller name.
2. Watches Ingress resources that reference the operator's IngressClass.
3. For each matching Ingress, creates `AgentEndpoint` and/or `CloudEndpoint` resources based on the mapping strategy.
4. Manages Domain resources for the hostnames specified in the Ingress rules.
5. Updates the Ingress status with endpoint information.

## Annotations

The following annotations on Ingress resources influence behavior:

- `ngrok.com/mapping-strategy` — Controls endpoint creation strategy
- `ngrok.com/traffic-policy` — References a TrafficPolicy
- `ngrok.com/pooling-enabled` — Enables endpoint pooling
- `ngrok.com/description` — Sets endpoint description
- `ngrok.com/metadata` — Sets endpoint metadata

See [annotations.md](../annotations.md) for details.

## When Disabled

When `features.ingress.enabled: false`:
- No IngressClass resource is created
- Ingress resources are not watched or reconciled
- The feature is excluded from the KubernetesOperator's `enabledFeatures`
