# Helm Chart — Agent Deployment

## Overview

The agent (agent-manager) runs as a separate deployment responsible for managing ngrok agent tunnels for AgentEndpoint resources. It is deployed when `ngrok.features.ingress.enabled` is true and one-click demo mode is off.

## Pod Settings

Any key from `components.common` (see [common.md](common.md)) can also be set here to override it for this component alone.

| Parameter | Description | Default |
|-----------|-------------|---------|
| `components.agent.replicaCount` | Number of replicas | `1` |
| `components.agent.resources` | Container resource requests/limits | `{}` |
| `components.agent.lifecycle` | Container lifecycle hooks | `{}` |
| `components.agent.terminationGracePeriodSeconds` | Graceful shutdown time | `30` |
| `components.agent.extraVolumes` | Additional volumes | `[]` |
| `components.agent.extraVolumeMounts` | Additional volume mounts | `[]` |
| `components.agent.updateStrategy.type` | Update strategy type | `RollingUpdate` |
| `components.agent.serviceAccount.create` | Create a ServiceAccount | `true` |
| `components.agent.serviceAccount.name` | ServiceAccount name (auto-generated if empty) | `""` |
| `components.agent.serviceAccount.annotations` | ServiceAccount annotations | `{}` |

## Operator Configuration

`components.agent.log` overrides `ngrok.log` for the agent alone.
