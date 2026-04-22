# Helm Chart — Bindings Forwarder Deployment

## Overview

The bindings forwarder runs as a separate deployment responsible for forwarding traffic for bound endpoints. It is only deployed when `features.bindings.enabled: true`.

## K8s Deployment Settings

All settings below override global defaults. See [common.md](common.md) for override semantics.

| Parameter                                                | Description                                     | Default         |
|----------------------------------------------------------|-------------------------------------------------|-----------------|
| `bindingsForwarder.replicaCount`                         | Number of forwarder replicas                    | `1`             |
| `bindingsForwarder.podAnnotations`                       | Pod annotations (merged with global)            | `{}`            |
| `bindingsForwarder.podLabels`                            | Pod labels (merged with global)                 | `{}`            |
| `bindingsForwarder.nodeSelector`                         | Node labels for pod assignment                  | `{}`            |
| `bindingsForwarder.tolerations`                          | Pod tolerations                                 | `[]`            |
| `bindingsForwarder.affinity`                             | Affinity rules                                  | `{}`            |
| `bindingsForwarder.topologySpreadConstraints`            | Topology spread constraints                     | `[]`            |
| `bindingsForwarder.priorityClassName`                    | Pod priority class                              | `""`            |
| `bindingsForwarder.resources`                            | Container resource requests/limits              | `{}`            |
| `bindingsForwarder.extraVolumes`                         | Additional volumes                              | `[]`            |
| `bindingsForwarder.extraVolumeMounts`                    | Additional volume mounts                        | `[]`            |
| `bindingsForwarder.extraEnv`                             | Additional environment variables                | `{}`            |
| `bindingsForwarder.lifecycle`                            | Container lifecycle hooks                       | `{}`            |
| `bindingsForwarder.terminationGracePeriodSeconds`        | Graceful shutdown time                          | `30`            |
| `bindingsForwarder.updateStrategy.type`                  | Update strategy type                            | `RollingUpdate` |
| `bindingsForwarder.podDisruptionBudget.create`           | Enable PDB creation                             | `false`         |
| `bindingsForwarder.podDisruptionBudget.maxUnavailable`   | Max unavailable pods                            | `"1"`           |
| `bindingsForwarder.podDisruptionBudget.minAvailable`     | Min available pods                              | (unset)         |
| `bindingsForwarder.serviceAccount.create`                | Create a ServiceAccount                         | `true`          |
| `bindingsForwarder.serviceAccount.name`                  | ServiceAccount name (auto-generated if empty)   | `""`            |
| `bindingsForwarder.serviceAccount.annotations`           | ServiceAccount annotations                      | `{}`            |

## App Config

No bindings-forwarder-specific config keys at this time. The forwarder reads all shared config from `ngrok.*` and feature flags from `features.*`.
