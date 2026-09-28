# Helm Chart — Agent Deployment

## Overview

The agent (agent-manager) runs as a separate deployment responsible for managing ngrok agent tunnels for AgentEndpoint resources. It is deployed when `features.ingress.enabled` is true and one-click demo mode is off.

## Pod Settings

Any key from `defaults` (see [common.md](common.md)) can also be set here to override it for this component alone.

| Parameter | Description | Default |
|-----------|-------------|---------|
| `agent.replicaCount` | Number of replicas | `1` |
| `agent.resources` | Container resource requests/limits | `{}` |
| `agent.lifecycle` | Container lifecycle hooks | `{}` |
| `agent.terminationGracePeriodSeconds` | Graceful shutdown time | `30` |
| `agent.extraVolumes` | Additional volumes | `[]` |
| `agent.extraVolumeMounts` | Additional volume mounts | `[]` |
| `agent.updateStrategy.type` | Update strategy type | `RollingUpdate` |
| `agent.serviceAccount.create` | Create a ServiceAccount | `true` |
| `agent.serviceAccount.name` | ServiceAccount name (auto-generated if empty) | `""` |
| `agent.serviceAccount.annotations` | ServiceAccount annotations | `{}` |

## Operator Configuration

`agent.config` overrides the shared operator configuration for the agent alone. There are no agent-only settings at this time.
