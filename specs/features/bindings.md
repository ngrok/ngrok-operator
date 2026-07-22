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
- The certificate is stored in a Kubernetes Secret (default name: `default-tls`).
- The forwarder uses this certificate to authenticate with the ingress endpoint.

## Pod Identity

When a workload connects through a projected bound-endpoint Service, the bindings forwarder looks up the source Pod by client IP and attaches a pod identity (UID, name, namespace, annotations) to the upstream connection. Only pod annotations under the `ngrok.com/` prefix are forwarded; all other annotations are pruned. Keys and values are forwarded verbatim, so ngrok traffic-policy expressions on the bound endpoint can match on them.

| Detail          | Value                                                  |
|-----------------|--------------------------------------------------------|
| Applies to      | `Pod` annotations on workloads connecting through bound-endpoint Services |
| Key form        | `ngrok.com/<anything>` — free-form, user-defined       |
| Consumed by     | ngrok traffic-policy expressions on the bound endpoint |

During the `k8s.ngrok.com/` → `ngrok.com/` migration window the forwarder forwards pod annotations under either prefix, verbatim — policy expressions matching `k8s.ngrok.com/*` key names keep working until the pod annotations themselves are renamed. Legacy-prefix forwarding is removed in 1.0 — see [`docs/v1-migration-guide.md`](../../docs/v1-migration-guide.md).

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
