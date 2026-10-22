# Helm Chart — Private Endpoint Forwarder Deployment

## Overview

The private endpoint forwarder runs as a separate deployment that forwards in-cluster connections to ngrok private endpoints over private dial. It is only deployed when `ngrok.features.privateEndpoints.enabled: true`, and is skipped when `ngrok.features.oneClickDemoMode.enabled: true`.

## Pod Settings

Any key from `components.common` (see [common.md](common.md)) can also be set here to override it for this component alone.

| Parameter | Description | Default |
|-----------|-------------|---------|
| `components.privateEndpointForwarder.replicaCount` | Number of replicas | `1` |
| `components.privateEndpointForwarder.resources` | Container resource requests/limits | `{}` |
| `components.privateEndpointForwarder.lifecycle` | Overrides `components.common.lifecycle` | (inherits) |
| `components.privateEndpointForwarder.extraVolumes` | Overrides `components.common.extraVolumes` | (inherits) |
| `components.privateEndpointForwarder.extraVolumeMounts` | Overrides `components.common.extraVolumeMounts` | (inherits) |
| `components.privateEndpointForwarder.serviceAccount.create` | Create a ServiceAccount | `true` |
| `components.privateEndpointForwarder.serviceAccount.name` | ServiceAccount name (auto-generated if empty) | `""` |
| `components.privateEndpointForwarder.serviceAccount.annotations` | ServiceAccount annotations | `{}` |

Pods carry `app.kubernetes.io/component: private-endpoint-forwarder`, which the per-hostname Services select. Probes: liveness `/healthz` and readiness `/readyz` on port 8081; metrics on port 8080.

## Credentials

The forwarder mounts the `PRIVATE_ENDPOINTS_ACCESS_TOKEN` key of the credentials Secret as `NGROK_ACCESS_TOKEN`.

| Parameter | Description | Default |
|-----------|-------------|---------|
| `ngrok.credentials.privateEndpointForwarder.accessToken` | Access token for the forwarder only; falls back to `ngrok.credentials.accessToken` | `""` |

With the chart-generated Secret, the key is written only when the feature is enabled, and rendering fails if neither token is set. A Secret named by `ngrok.credentials.secret.name` cannot be checked at render time; it must hold a `PRIVATE_ENDPOINTS_ACCESS_TOKEN` key, or the forwarder does not start.

## Operator Configuration

`components.privateEndpointForwarder.log` overrides `ngrok.log` for the private-endpoint-forwarder alone.

## Network

The cluster must allow outbound UDP/443 from the forwarder pods: private dial runs over QUIC only.
