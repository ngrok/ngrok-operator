# Helm Chart — Features

Features are configured under `ngrok.features:`, as part of the operator configuration (see [common.md](common.md#operator-configuration)). Each setting is set once for the whole install; the chart passes it to whichever components need it, and it cannot be overridden per component.

Defaults below are the operator's built-in defaults. An empty value in `values.yaml` means the default applies.

## Ingress

| Parameter                              | Description                                      | Default                          |
|----------------------------------------|--------------------------------------------------|----------------------------------|
| `ngrok.features.ingress.enabled`             | Enable the Kubernetes Ingress controller         | `true`                           |
| `ngrok.features.ingress.controllerName`      | Controller name for IngressClass matching        | `k8s.ngrok.com/ingress-controller` |
| `ngrok.features.ingress.watchNamespace`      | Namespace to watch (empty = all namespaces)      | `""`                             |
| `ngrok.features.ingress.ingressClass.name`   | IngressClass resource name                       | `ngrok`                          |
| `ngrok.features.ingress.ingressClass.create` | Create the IngressClass resource                 | `true`                           |
| `ngrok.features.ingress.ingressClass.default`| Set as the default IngressClass                  | `false`                          |

When disabled, no IngressClass is created and Ingress resources are not watched.

See [features/ingress.md](../features/ingress.md) for behavior details.

## Gateway API

| Parameter                                      | Description                                                          | Default |
|------------------------------------------------|----------------------------------------------------------------------|---------|
| `ngrok.features.gateway.enabled`                     | Enable Gateway API support (if CRDs detected)                        | `true`  |
| `ngrok.features.gateway.disableReferenceGrants`      | Disable ReferenceGrant requirement for cross-namespace references    | `false` |

When disabled, Gateway API resources are not watched regardless of whether CRDs are installed.

See [features/gateway-api.md](../features/gateway-api.md) for behavior details.

## Bindings

| Parameter                                | Description                                           | Default                                   |
|------------------------------------------|-------------------------------------------------------|-------------------------------------------|
| `ngrok.features.bindings.enabled`              | Enable the Endpoint Bindings feature                  | `false`                                   |
| `ngrok.features.bindings.endpointSelectors`    | CEL expressions filtering which endpoints to project  | `["true"]`                                |
| `ngrok.features.bindings.serviceAnnotations`   | Annotations applied to projected services             | `{}`                                      |
| `ngrok.features.bindings.serviceLabels`        | Labels applied to projected services                  | `{}`                                      |
| `ngrok.features.bindings.ingressEndpoint`      | Hostname of the bindings ingress endpoint             | `kubernetes-binding-ingress.ngrok.io:443` |

When `ngrok.features.bindings.enabled` is `true`, the bindings forwarder deployment is created (controlled by `bindingsForwarder`) and the operator starts managing BoundEndpoint resources.

See [features/bindings.md](../features/bindings.md) for behavior details.

## Domains

| Parameter                                  | Description                                                           | Default    |
|--------------------------------------------|-----------------------------------------------------------------------|------------|
| `ngrok.features.domains.defaultReclaimPolicy`    | Reclaim policy given to the Domains the operator creates: `"Delete"` or `"Retain"` | `"Delete"` |

## One-Click Demo Mode

| Parameter                            | Description                                                      | Default |
|--------------------------------------|------------------------------------------------------------------|---------|
| `ngrok.features.oneClickDemoMode.enabled`  | Start without credentials and become Ready without reconciling. Also skips rendering the agent and bindings-forwarder | `false` |

## Cleanup

On uninstall, a pre-delete hook deletes the KubernetesOperator resource, which makes the operator drain the resources it manages before it is removed. The drain policy decides what happens to the ngrok API resources. It is unrelated to `ngrok.features.domains.defaultReclaimPolicy`, which applies to a Domain deleted while the operator runs.

| Parameter                        | Description                                                        | Default    |
|----------------------------------|--------------------------------------------------------------------|------------|
| `ngrok.features.cleanup.enabled`       | Run the pre-delete hook                                            | `true`     |
| `ngrok.features.cleanup.timeout`       | Seconds the hook waits for the drain                               | `300`      |
| `ngrok.features.cleanup.drainPolicy`   | `"Delete"` or `"Retain"` the ngrok API resources on drain          | `"Retain"` |

The hook's own pod settings live under `components.cleanupHook`:

| Parameter                              | Description                                  | Default              |
|----------------------------------------|----------------------------------------------|----------------------|
| `components.cleanupHook.image.repository`         | kubectl image repository                     | `bitnami/kubectl`    |
| `components.cleanupHook.image.tag`               | kubectl image tag                            | `latest`             |
| `components.cleanupHook.image.pullPolicy`         | Image pull policy                            | `IfNotPresent`       |
| `components.cleanupHook.resources`               | Resource requests/limits for the hook        | See below            |

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
