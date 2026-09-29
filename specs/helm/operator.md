# Helm Chart — API Manager Deployment

## Overview

The api-manager is the primary operator component responsible for reconciling CRDs, managing ngrok API resources, and handling Ingress/Gateway API integration.

## Pod Settings

Any key from `components.common` (see [common.md](common.md)) can also be set here to override it for this component alone.

| Parameter | Description | Default |
|-----------|-------------|---------|
| `components.apiManager.replicaCount` | Number of replicas | `1` |
| `components.apiManager.resources` | Container resource requests/limits | `{}` |
| `components.apiManager.lifecycle` | Container lifecycle hooks | `{}` |
| `components.apiManager.terminationGracePeriodSeconds` | Graceful shutdown time | `30` |
| `components.apiManager.extraVolumes` | Additional volumes | `[]` |
| `components.apiManager.extraVolumeMounts` | Additional volume mounts | `[]` |
| `components.apiManager.updateStrategy.type` | Update strategy type | `RollingUpdate` |
| `components.apiManager.podDisruptionBudget.create` | Enable PDB creation | `false` |
| `components.apiManager.podDisruptionBudget.maxUnavailable` | Max unavailable pods | `"1"` |
| `components.apiManager.podDisruptionBudget.minAvailable` | Min available pods; set instead of `maxUnavailable` | (unset) |
| `components.apiManager.serviceAccount.create` | Create a ServiceAccount | `true` |
| `components.apiManager.serviceAccount.name` | ServiceAccount name (auto-generated if empty) | `""` |
| `components.apiManager.serviceAccount.annotations` | ServiceAccount annotations | `{}` |

## Operator Configuration

`components.apiManager.log` overrides `ngrok.log` for the api-manager alone.
