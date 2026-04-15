# Helm Chart — API Manager Deployment

## Overview

The api-manager is the primary operator component responsible for reconciling CRDs, managing ngrok API resources, and handling Ingress/Gateway API integration.

## K8s Deployment Settings

All settings below override global defaults. See [common.md](common.md) for override semantics.

| Parameter                                      | Description                                          | Default         |
|------------------------------------------------|------------------------------------------------------|-----------------|
| `apiManager.replicaCount`                      | Number of api-manager replicas                       | `1`             |
| `apiManager.podAnnotations`                    | Pod annotations (merged with global)                 | `{}`            |
| `apiManager.podLabels`                         | Pod labels (merged with global)                      | `{}`            |
| `apiManager.nodeSelector`                      | Node labels for pod assignment                       | `{}`            |
| `apiManager.tolerations`                       | Pod tolerations                                      | `[]`            |
| `apiManager.affinity`                          | Affinity rules                                       | `{}`            |
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
| `apiManager.clusterRole.annotations`           | Annotations for all ClusterRoles                     | `{}`            |

A minimum of 2 replicas is recommended in production for high availability.

## App Config

Component-specific app config rendered into the api-manager ConfigMap. Overrides values from the common ConfigMap (`ngrok.*`).

| Parameter                                        | Description                                   | Default  |
|--------------------------------------------------|-----------------------------------------------|----------|
| `apiManager.config.oneClickDemoMode`             | Start without credentials for demo purposes   | `false`  |
