# Helm Chart — Agent Deployment

## Overview

The agent (agent-manager) runs as a separate deployment responsible for managing ngrok agent tunnels for AgentEndpoint resources.

## K8s Deployment Settings

All settings below override the shared pod settings in `defaults`. See [common.md](common.md) for override semantics.

| Parameter                                    | Description                                     | Default         |
|------------------------------------------------|----------------------------------------------------|-----------------|
| `agent.replicaCount`                         | Number of agent replicas                        | `1`             |
| `agent.podAnnotations`                       | Pod annotations (merged with `defaults`)         | `{}`            |
| `agent.podLabels`                            | Pod labels (merged with `defaults`)              | `{}`            |
| `agent.nodeSelector`                         | Node labels for pod assignment                  | `{}`            |
| `agent.tolerations`                          | Pod tolerations                                 | `[]`            |
| `agent.affinity`                             | Affinity rules                                  | `{}`            |
| `agent.podAffinityPreset`                    | Pod affinity preset                             | `""`            |
| `agent.podAntiAffinityPreset`                | Pod anti-affinity preset                        | `soft`          |
| `agent.nodeAffinityPreset`                   | Node affinity preset                            | `{}`            |
| `agent.topologySpreadConstraints`            | Topology spread constraints                     | `[]`            |
| `agent.priorityClassName`                    | Pod priority class                              | `""`            |
| `agent.resources`                            | Container resource requests/limits              | `{}`            |
| `agent.extraVolumes`                         | Additional volumes                              | `[]`            |
| `agent.extraVolumeMounts`                    | Additional volume mounts                        | `[]`            |
| `agent.extraEnv`                             | Additional environment variables                | `{}`            |
| `agent.lifecycle`                            | Container lifecycle hooks                       | `{}`            |
| `agent.terminationGracePeriodSeconds`        | Graceful shutdown time                          | `30`            |
| `agent.updateStrategy.type`                  | Update strategy type                            | `RollingUpdate` |
| `agent.serviceAccount.create`                | Create a ServiceAccount                         | `true`          |
| `agent.serviceAccount.name`                  | ServiceAccount name (auto-generated if empty)   | `""`            |
| `agent.serviceAccount.annotations`           | ServiceAccount annotations                      | `{}`            |

## App Config

App config keys sit directly under `agent`, alongside the pod settings above, and are rendered into the agent's own ConfigMap (see [common.md](common.md#config-delivery)). Everything not listed here comes from the shared `ngrok.*`, `log.*`, and `features.*` sections.

| Parameter                     | Description                                                                                         | Default |
|---------------------------------|---------------------------------------------------------------------------------------------------------|---------|
| `agent.watchNamespace`        | Namespace the agent watches for AgentEndpoint resources (empty = all namespaces). Distinct from `features.ingress.watchNamespace`, which scopes the api-manager's Ingress watch. | `""`    |
| `agent.log.level`             | Overrides the shared `log.level` for the agent only                                                  | (unset) |
| `agent.log.format`            | Overrides the shared `log.format` for the agent only                                                 | (unset) |
| `agent.log.stacktraceLevel`   | Overrides the shared `log.stacktraceLevel` for the agent only                                        | (unset) |

<!-- TODO(alex): audit which other shared app config keys should be overridable per component. Starting with log.* only; the merge logic supports any of them. -->
