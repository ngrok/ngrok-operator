# Kubernetes Ingress Feature

## Overview

The ngrok-operator can function as a Kubernetes Ingress controller, watching Ingress resources and materializing them as ngrok endpoints.

## Configuration

| Helm Value                     | Description                                      | Default                            |
|--------------------------------|--------------------------------------------------|------------------------------------|
| `ingress.enabled`              | Enable the Ingress controller                    | `true`                             |
| `ingress.controllerName`       | Controller name for IngressClass matching        | `k8s.ngrok.com/ingress-controller` |
| `ingress.watchNamespace`       | Namespace to watch (empty = all)                 | `""`                               |
| `ingress.ingressClass.name`    | IngressClass resource name                       | `ngrok`                            |
| `ingress.ingressClass.create`  | Create the IngressClass resource                 | `true`                             |
| `ingress.ingressClass.default` | Set as the default IngressClass                  | `false`                            |

## Behavior

When enabled, the operator:

1. Creates an `IngressClass` resource (if `ingress.ingressClass.create` is true) with the configured controller name.
2. Watches Ingress resources that reference the operator's IngressClass.
3. For each matching Ingress, creates `AgentEndpoint` and/or `CloudEndpoint` resources based on the mapping strategy.
4. Manages Domain resources for the hostnames specified in the Ingress rules.
5. Updates the Ingress status with endpoint information.

## Annotations

The following annotations on Ingress resources influence behavior:

- `k8s.ngrok.com/mapping-strategy` — Controls endpoint creation strategy
- `k8s.ngrok.com/traffic-policy` — References an NgrokTrafficPolicy
- `k8s.ngrok.com/pooling-enabled` — Enables endpoint pooling
- `k8s.ngrok.com/description` — Sets endpoint description
- `k8s.ngrok.com/metadata` — Sets endpoint metadata

See [annotations.md](../annotations.md) for details.

## When Disabled

When `ingress.enabled: false`:
- No IngressClass resource is created
- Ingress resources are not watched or reconciled
- The feature is excluded from the KubernetesOperator's `enabledFeatures`
