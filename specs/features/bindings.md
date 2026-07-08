# Endpoint Bindings Feature

## Overview

The endpoint bindings feature allows ngrok endpoints to be "bound" into a Kubernetes cluster, projecting external ngrok endpoints as local Kubernetes services. This enables traffic from ngrok to flow directly to services inside the cluster.

Bindings is opt-in (`features.bindings.enabled: false` by default).

## Configuration

| Helm Value                              | Description                                           | Default                                   |
|-----------------------------------------|-------------------------------------------------------|-------------------------------------------|
| `features.bindings.enabled`             | Enable the bindings feature                           | `false`                                   |
| `features.bindings.endpointSelectors`   | CEL expressions filtering which endpoints to project  | `["true"]`                                |
| `features.bindings.serviceAnnotations`  | Annotations applied to projected services             | `{}`                                      |
| `features.bindings.serviceLabels`       | Labels applied to projected services                  | `{}`                                      |
| `features.bindings.ingressEndpoint`     | Hostname of the bindings ingress endpoint             | `kubernetes-binding-ingress.ngrok.io:443` |

## Components

| Component            | Description                                              |
|----------------------|----------------------------------------------------------|
| BoundEndpoint CRD    | Represents an endpoint bound from ngrok to a service     |
| BoundEndpoint controller | Creates target and upstream services for each binding |
| Bindings Forwarder   | Bridges connections via mTLS to the ngrok ingress        |

## Flow

1. The operator registers with the ngrok API as a Kubernetes operator with bindings enabled.
2. The operator polls the ngrok API for bound endpoint updates.
3. The operator creates `BoundEndpoint` CRs for matching endpoints (filtered by `endpointSelectors`).
4. The BoundEndpoint controller creates two Services per bound endpoint:
   - **Target Service**: An `ExternalName` service in the target namespace, providing a local DNS name for the endpoint.
   - **Upstream Service**: A `ClusterIP` service in the operator namespace, pointing to the bindings forwarder pods.
5. The bindings forwarder listens on allocated ports and bridges incoming connections via mTLS to the ngrok ingress endpoint.

## mTLS

The bindings forwarder uses mutual TLS for secure communication with ngrok's ingress endpoint:

- The operator generates a self-signed TLS certificate and submits a CSR to the ngrok API.
- The certificate is stored in a Kubernetes Secret in the operator's namespace. The name is set via the `bindings.tlsSecretName` Helm value, which plumbs through to the `--bindings-tls-secret-name` flag and onto the `KubernetesOperator` CR's `binding.tlsSecretName` field. When the Helm value is empty it defaults to `<release-fullname>-default-tls` (e.g. `ngrok-operator-default-tls`), prefixed per-release to avoid collisions between operators sharing a namespace.
- The forwarder uses this certificate to authenticate with the ingress endpoint.

## Related Specs

- [BoundEndpoint CRD](../crds/boundendpoint.md)
- [BoundEndpoint controller](../controllers/boundendpoint.md)
- [Bindings Forwarder Helm values](../helm/bindings-forwarder.md)
- [RBAC](../rbac/README.md)

## When Disabled

When `bindings.enabled: false`:
- The bindings forwarder deployment is not created
- BoundEndpoint resources are not managed
- The bindings feature is excluded from the KubernetesOperator's `enabledFeatures`
