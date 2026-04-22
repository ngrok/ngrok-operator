# Helm Chart — Agent Deployment

## Overview

The agent (agent-manager) runs as a separate deployment responsible for managing ngrok agent tunnels for AgentEndpoint resources.

## K8s Deployment Settings

All settings below override global defaults. See [common.md](common.md) for override semantics.

| Parameter                                    | Description                                     | Default         |
|----------------------------------------------|-------------------------------------------------|-----------------|
| `agent.replicaCount`                         | Number of agent replicas                        | `1`             |
| `agent.podAnnotations`                       | Pod annotations (merged with global)            | `{}`            |
| `agent.podLabels`                            | Pod labels (merged with global)                 | `{}`            |
| `agent.nodeSelector`                         | Node labels for pod assignment                  | `{}`            |
| `agent.tolerations`                          | Pod tolerations                                 | `[]`            |
| `agent.affinity`                             | Affinity rules                                  | `{}`            |
| `agent.topologySpreadConstraints`            | Topology spread constraints                     | `[]`            |
| `agent.priorityClassName`                    | Pod priority class                              | `""`            |
| `agent.resources`                            | Container resource requests/limits              | `{}`            |
| `agent.extraVolumes`                         | Additional volumes                              | `[]`            |
| `agent.extraVolumeMounts`                    | Additional volume mounts                        | `[]`            |
| `agent.extraEnv`                             | Additional environment variables                | `{}`            |
| `agent.lifecycle`                            | Container lifecycle hooks                       | `{}`            |
| `agent.terminationGracePeriodSeconds`        | Graceful shutdown time                          | `30`            |
| `agent.updateStrategy.type`                  | Update strategy type                            | `RollingUpdate` |
| `agent.podDisruptionBudget.create`           | Enable PDB creation                             | `false`         |
| `agent.podDisruptionBudget.maxUnavailable`   | Max unavailable pods                            | `"1"`           |
| `agent.podDisruptionBudget.minAvailable`     | Min available pods                              | (unset)         |
| `agent.serviceAccount.create`                | Create a ServiceAccount                         | `true`          |
| `agent.serviceAccount.name`                  | ServiceAccount name (auto-generated if empty)   | `""`            |
| `agent.serviceAccount.annotations`           | ServiceAccount annotations                      | `{}`            |

## App Config

Component-specific app config rendered into the agent ConfigMap. Overrides values from the common ConfigMap (`ngrok.*`).

No agent-specific config keys at this time. The agent reads all shared config from `ngrok.*` and feature flags from `features.*`.
