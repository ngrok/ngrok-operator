# Endpoint Bindings Feature

## Overview

The endpoint bindings feature allows ngrok endpoints to be "bound" into a Kubernetes cluster, projecting external ngrok endpoints as local Kubernetes services. This enables traffic from ngrok to flow directly to services inside the cluster.

**Status:** In development (`bindings.enabled: false` by default).

## Configuration

| Helm Value                     | Description                                           | Default                                   |
|--------------------------------|-------------------------------------------------------|-------------------------------------------|
| `bindings.enabled`             | Enable the bindings feature                           | `false`                                   |
| `bindings.endpointSelectors`   | CEL expressions filtering which endpoints to project  | `["true"]`                                |
| `bindings.serviceAnnotations`  | Annotations applied to projected services             | `{}`                                      |
| `bindings.serviceLabels`       | Labels applied to projected services                  | `{}`                                      |
| `bindings.ingressEndpoint`     | Hostname of the bindings ingress endpoint             | `kubernetes-binding-ingress.ngrok.io:443` |

## Components

| Component            | Description                                              |
|----------------------|----------------------------------------------------------|
| BoundEndpoint CRD    | Represents an endpoint bound from ngrok to a service     |
| BoundEndpoint controller | Creates target and upstream services for each binding |
| Bindings Forwarder   | Bridges connections via mTLS to the ngrok ingress        |

## Flow

1. The operator registers with the ngrok API as a Kubernetes operator with bindings enabled.
2. The ngrok API pushes bound endpoint information to the operator via a poller.
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

## When Disabled

When `bindings.enabled: false`:
- The bindings forwarder deployment is not created
- BoundEndpoint resources are not managed
- The bindings feature is excluded from the KubernetesOperator's `enabledFeatures`
