# Helm Chart — Features

Features are configured at the top level under `features:`. This is the **single source of truth** for what is enabled and how each feature is configured. Components do not duplicate feature flags.

## Ingress

| Parameter                              | Description                                      | Default                          |
|----------------------------------------|--------------------------------------------------|----------------------------------|
| `features.ingress.enabled`             | Enable the Kubernetes Ingress controller         | `true`                           |
| `features.ingress.controllerName`      | Controller name for IngressClass matching        | `ngrok.com/ingress-controller`   |
| `features.ingress.watchNamespace`      | Namespace to watch (empty = all namespaces)      | `""`                             |
| `features.ingress.ingressClass.name`   | IngressClass resource name                       | `ngrok`                          |
| `features.ingress.ingressClass.create` | Create the IngressClass resource                 | `true`                           |
| `features.ingress.ingressClass.default`| Set as the default IngressClass                  | `false`                          |

When disabled, no IngressClass is created and Ingress resources are not watched.

See [features/ingress.md](../features/ingress.md) for behavior details.

## Gateway API

| Parameter                                      | Description                                                          | Default |
|------------------------------------------------|----------------------------------------------------------------------|---------|
| `features.gateway.enabled`                     | Enable Gateway API support (if CRDs detected)                        | `true`  |
| `features.gateway.disableReferenceGrants`      | Disable ReferenceGrant requirement for cross-namespace references    | `false` |

When disabled, Gateway API resources are not watched regardless of whether CRDs are installed.

See [features/gateway-api.md](../features/gateway-api.md) for behavior details.

## Bindings

| Parameter                                | Description                                           | Default                                   |
|------------------------------------------|-------------------------------------------------------|-------------------------------------------|
| `features.bindings.enabled`              | Enable the Endpoint Bindings feature                  | `false`                                   |
| `features.bindings.endpointSelectors`    | CEL expressions filtering which endpoints to project  | `["true"]`                                |
| `features.bindings.serviceAnnotations`   | Annotations applied to projected services             | `{}`                                      |
| `features.bindings.serviceLabels`        | Labels applied to projected services                  | `{}`                                      |
| `features.bindings.ingressEndpoint`      | Hostname of the bindings ingress endpoint             | `kubernetes-binding-ingress.ngrok.io:443` |

When `features.bindings.enabled` is `true`, the bindings forwarder deployment is created (controlled by `bindingsForwarder`) and the operator starts managing BoundEndpoint resources.

See [features/bindings.md](../features/bindings.md) for behavior details.

## Drain and Domain Policies

| Parameter                                | Description                                                      | Default    |
|------------------------------------------|------------------------------------------------------------------|------------|
| `features.drainPolicy`                   | Drain policy on uninstall: `"Delete"` or `"Retain"`             | `"Retain"` |
| `features.defaultDomainReclaimPolicy`    | Default reclaim policy for Domains: `"Delete"` or `"Retain"`   | `"Delete"` |

## Cleanup Hook

| Parameter                              | Description                                  | Default              |
|----------------------------------------|----------------------------------------------|----------------------|
| `cleanupHook.enabled`                  | Enable the pre-delete cleanup hook           | `true`               |
| `cleanupHook.timeout`                  | Cleanup timeout in seconds                   | `300`                |
| `cleanupHook.image.repository`         | kubectl image repository                     | `bitnami/kubectl`    |
| `cleanupHook.image.tag`               | kubectl image tag                            | `latest`             |
| `cleanupHook.image.pullPolicy`         | Image pull policy                            | `IfNotPresent`       |
| `cleanupHook.resources`               | Resource requests/limits for the hook        | See below            |

Default cleanup hook resources:
```yaml
resources:
  limits:
    cpu: 250m
    memory: 256Mi
  requests:
    cpu: 250m
    memory: 256Mi
```

See [features/draining.md](../features/draining.md) for cleanup behavior details.
