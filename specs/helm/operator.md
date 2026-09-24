# Helm Chart — API Manager Deployment

## Overview

The api-manager is the primary operator component responsible for reconciling CRDs, managing ngrok API resources, and handling Ingress/Gateway API integration.

## K8s Deployment Settings

All settings below override the shared pod settings in `defaults`. See [common.md](common.md) for override semantics.

| Parameter                                      | Description                                          | Default         |
|------------------------------------------------|--------------------------------------------------------|-----------------|
| `apiManager.replicaCount`                      | Number of api-manager replicas                       | `1`             |
| `apiManager.podAnnotations`                    | Pod annotations (merged with `defaults`)              | `{}`            |
| `apiManager.podLabels`                         | Pod labels (merged with `defaults`)                   | `{}`            |
| `apiManager.nodeSelector`                      | Node labels for pod assignment                       | `{}`            |
| `apiManager.tolerations`                       | Pod tolerations                                      | `[]`            |
| `apiManager.affinity`                          | Affinity rules                                       | `{}`            |
| `apiManager.podAffinityPreset`                 | Pod affinity preset                                  | `""`            |
| `apiManager.podAntiAffinityPreset`             | Pod anti-affinity preset                             | `soft`          |
| `apiManager.nodeAffinityPreset`                | Node affinity preset                                 | `{}`            |
| `apiManager.topologySpreadConstraints`         | Topology spread constraints                          | `[]`            |
| `apiManager.priorityClassName`                 | Pod priority class                                   | `""`            |
| `apiManager.resources`                         | Container resource requests/limits                   | `{}`            |
| `apiManager.extraVolumes`                      | Additional volumes                                   | `[]`            |
| `apiManager.extraVolumeMounts`                 | Additional volume mounts                             | `[]`            |
| `apiManager.extraEnv`                          | Additional environment variables                     | `{}`            |
| `apiManager.lifecycle`                         | Container lifecycle hooks                            | `{}`            |
| `apiManager.terminationGracePeriodSeconds`     | Graceful shutdown time                               | `30`            |
| `apiManager.updateStrategy.type`               | Update strategy type                                 | `RollingUpdate` |
| `apiManager.podDisruptionBudget.create`        | Enable PDB creation                                  | `false`         |
| `apiManager.podDisruptionBudget.maxUnavailable`| Max unavailable pods                                 | `"1"`           |
| `apiManager.podDisruptionBudget.minAvailable`  | Min available pods                                   | (unset)         |
| `apiManager.serviceAccount.create`             | Create a ServiceAccount                              | `true`          |
| `apiManager.serviceAccount.name`               | ServiceAccount name (auto-generated if empty)        | `""`            |
| `apiManager.serviceAccount.annotations`        | ServiceAccount annotations                           | `{}`            |

## App Config

App config keys sit directly under `apiManager`, alongside the pod settings above, and are rendered into the api-manager's own ConfigMap (see [common.md](common.md#config-delivery)). Everything the api-manager needs beyond this comes from the shared `ngrok.*`, `log.*`, and `features.*` sections.

| Parameter                                  | Description                                                    | Default  |
|----------------------------------------------|------------------------------------------------------------------|----------|
| `apiManager.oneClickDemoMode`              | Start without credentials for demo purposes                     | `false`  |
| `apiManager.log.level`                     | Overrides the shared `log.level` for the api-manager only       | (unset)  |
| `apiManager.log.format`                    | Overrides the shared `log.format` for the api-manager only      | (unset)  |
| `apiManager.log.stacktraceLevel`           | Overrides the shared `log.stacktraceLevel` for the api-manager only | (unset) |

`oneClickDemoMode` reaches beyond the api-manager despite living under
`apiManager`: when it is true the agent and bindings-forwarder
Deployments do not render at all. Those pods have no authtoken on a
credential-less install and would only crashloop.

<!-- TODO(alex): audit which other shared app config keys should be overridable per component. -->
