# Helm Chart — API Manager Deployment

## Overview

The api-manager is the primary operator component responsible for reconciling CRDs, managing ngrok API resources, and handling Ingress/Gateway API integration.

## Pod Settings

Any key from `defaults` (see [common.md](common.md)) can also be set here to override it for this component alone.

| Parameter | Description | Default |
|-----------|-------------|---------|
| `apiManager.replicaCount` | Number of replicas | `1` |
| `apiManager.resources` | Container resource requests/limits | `{}` |
| `apiManager.lifecycle` | Container lifecycle hooks | `{}` |
| `apiManager.terminationGracePeriodSeconds` | Graceful shutdown time | `30` |
| `apiManager.extraVolumes` | Additional volumes | `[]` |
| `apiManager.extraVolumeMounts` | Additional volume mounts | `[]` |
| `apiManager.updateStrategy.type` | Update strategy type | `RollingUpdate` |
| `apiManager.podDisruptionBudget.create` | Enable PDB creation | `false` |
| `apiManager.podDisruptionBudget.maxUnavailable` | Max unavailable pods | `"1"` |
| `apiManager.podDisruptionBudget.minAvailable` | Min available pods; set instead of `maxUnavailable` | (unset) |
| `apiManager.serviceAccount.create` | Create a ServiceAccount | `true` |
| `apiManager.serviceAccount.name` | ServiceAccount name (auto-generated if empty) | `""` |
| `apiManager.serviceAccount.annotations` | ServiceAccount annotations | `{}` |

## Operator Configuration

`apiManager.config` overrides the shared operator configuration for the api-manager alone. It also holds settings only the api-manager reads:

| Parameter | Description | Default |
|-----------|-------------|---------|
| `apiManager.config.oneClickDemoMode` | Start without credentials and become Ready without reconciling. Also skips rendering the agent and bindings-forwarder | `false` |
