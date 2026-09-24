# Helm Chart — Bindings Forwarder Deployment

## Overview

The bindings forwarder runs as a separate deployment responsible for forwarding traffic for bound endpoints. It is only deployed when `features.bindings.enabled: true`.

## K8s Deployment Settings

All settings below override the shared pod settings in `defaults`. See [common.md](common.md) for override semantics.

| Parameter                                                | Description                                     | Default         |
|-------------------------------------------------------------|------------------------------------------------------|-----------------|
| `bindingsForwarder.replicaCount`                         | Number of forwarder replicas                    | `1`             |
| `bindingsForwarder.podAnnotations`                       | Pod annotations (merged with `defaults`)         | `{}`            |
| `bindingsForwarder.podLabels`                            | Pod labels (merged with `defaults`)              | `{}`            |
| `bindingsForwarder.nodeSelector`                         | Node labels for pod assignment                  | `{}`            |
| `bindingsForwarder.tolerations`                          | Pod tolerations                                 | `[]`            |
| `bindingsForwarder.affinity`                             | Affinity rules                                  | `{}`            |
| `bindingsForwarder.podAffinityPreset`                    | Pod affinity preset                             | `""`            |
| `bindingsForwarder.podAntiAffinityPreset`                | Pod anti-affinity preset                        | `soft`          |
| `bindingsForwarder.nodeAffinityPreset`                   | Node affinity preset                            | `{}`            |
| `bindingsForwarder.topologySpreadConstraints`            | Topology spread constraints                     | `[]`            |
| `bindingsForwarder.priorityClassName`                    | Pod priority class                              | `""`            |
| `bindingsForwarder.resources`                            | Container resource requests/limits              | `{}`            |
| `bindingsForwarder.extraVolumes`                         | Additional volumes                              | `[]`            |
| `bindingsForwarder.extraVolumeMounts`                    | Additional volume mounts                        | `[]`            |
| `bindingsForwarder.extraEnv`                             | Additional environment variables                | `{}`            |
| `bindingsForwarder.lifecycle`                            | Container lifecycle hooks                       | `{}`            |
| `bindingsForwarder.terminationGracePeriodSeconds`        | Graceful shutdown time                          | `30`            |
| `bindingsForwarder.updateStrategy.type`                  | Update strategy type                            | `RollingUpdate` |
| `bindingsForwarder.serviceAccount.create`                | Create a ServiceAccount                         | `true`          |
| `bindingsForwarder.serviceAccount.name`                  | ServiceAccount name (auto-generated if empty)   | `""`            |
| `bindingsForwarder.serviceAccount.annotations`           | ServiceAccount annotations                      | `{}`            |

There is no `bindingsForwarder.enabled` toggle. `features.bindings.enabled` alone controls both the Endpoint Bindings feature and whether this Deployment renders — see [features.md](features.md).

## App Config

The forwarder has no app config keys of its own. It reads shared config from `ngrok.*`, `log.*`, and `features.*`, and supports the same per-component logging overrides as the other components.

| Parameter                                 | Description                                                              | Default |
|---------------------------------------------|----------------------------------------------------------------------------|---------|
| `bindingsForwarder.log.level`             | Overrides the shared `log.level` for the forwarder only                   | (unset) |
| `bindingsForwarder.log.format`            | Overrides the shared `log.format` for the forwarder only                  | (unset) |
| `bindingsForwarder.log.stacktraceLevel`   | Overrides the shared `log.stacktraceLevel` for the forwarder only         | (unset) |

<!-- TODO(alex): audit which other shared app config keys should be overridable per component. -->
