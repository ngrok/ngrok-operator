# Helm Chart — Feature Flags

## Ingress

| Parameter                       | Description                                      | Default                               |
|---------------------------------|--------------------------------------------------|---------------------------------------|
| `ingress.enabled`               | Enable the Kubernetes Ingress controller         | `true`                                |
| `ingress.controllerName`        | Controller name for IngressClass matching        | `k8s.ngrok.com/ingress-controller`    |
| `ingress.watchNamespace`        | Namespace to watch (empty = all namespaces)      | `""`                                  |
| `ingress.ingressClass.name`     | IngressClass resource name                       | `ngrok`                               |
| `ingress.ingressClass.create`   | Create the IngressClass resource                 | `true`                                |
| `ingress.ingressClass.default`  | Set as the default IngressClass                  | `false`                               |

When disabled, no IngressClass is created and Ingress resources are not watched.

See [features/ingress.md](../features/ingress.md) for behavior details.

## Gateway API

| Parameter                         | Description                                          | Default |
|-----------------------------------|------------------------------------------------------|---------|
| `gateway.enabled`                 | Enable Gateway API support (if CRDs detected)        | `true`  |
| `gateway.disableReferenceGrants`  | Disable ReferenceGrant requirement for cross-namespace references | `false` |

When disabled, Gateway API resources are not watched regardless of whether CRDs are installed.

See [features/gateway-api.md](../features/gateway-api.md) for behavior details.

## Bindings

| Parameter                        | Description                                           | Default                                     |
|----------------------------------|-------------------------------------------------------|---------------------------------------------|
| `bindings.enabled`               | Enable the Endpoint Bindings feature                  | `false`                                     |
| `bindings.endpointSelectors`     | CEL expressions filtering which endpoints to project  | `["true"]`                                  |
| `bindings.serviceAnnotations`    | Annotations applied to projected services             | `{}`                                        |
| `bindings.serviceLabels`         | Labels applied to projected services                  | `{}`                                        |
| `bindings.ingressEndpoint`       | Hostname of the bindings ingress endpoint             | `kubernetes-binding-ingress.ngrok.io:443`   |

When enabled, the bindings forwarder deployment is created and the operator starts managing BoundEndpoint resources.

See [features/bindings.md](../features/bindings.md) for behavior details.

## Cleanup Hook

| Parameter                              | Description                                  | Default              |
|----------------------------------------|----------------------------------------------|----------------------|
| `cleanupHook.enabled`                  | Enable the pre-delete cleanup hook           | `true`               |
| `cleanupHook.timeout`                  | Cleanup timeout in seconds                   | `300`                |
| `cleanupHook.image.repository`         | kubectl image repository                     | `bitnami/kubectl`    |
| `cleanupHook.image.tag`               | kubectl image tag                            | `latest`             |
| `cleanupHook.image.pullPolicy`         | Image pull policy                            | `IfNotPresent`       |
| `cleanupHook.resources.limits.cpu`     | CPU limit                                    | `100m`               |
| `cleanupHook.resources.limits.memory`  | Memory limit                                 | `128Mi`              |
| `cleanupHook.resources.requests.cpu`   | CPU request                                  | `50m`                |
| `cleanupHook.resources.requests.memory`| Memory request                               | `64Mi`               |

See [features/draining.md](../features/draining.md) for cleanup behavior details.
