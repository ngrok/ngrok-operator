# Helm Chart — Operator Deployment

## Replicas

| Parameter      | Description                                          | Default |
|----------------|------------------------------------------------------|---------|
| `replicaCount` | Number of operator (api-manager) replicas            | `1`     |

A minimum of 2 is recommended in production for high availability.

## Pod Scheduling

| Parameter                     | Description                                              | Default |
|-------------------------------|----------------------------------------------------------|---------|
| `affinity`                    | Full affinity spec (overrides all presets)                | `{}`    |
| `podAffinityPreset`           | Pod affinity preset: `soft` or `hard`                    | `""`    |
| `podAntiAffinityPreset`       | Pod anti-affinity preset: `soft` or `hard`               | `soft`  |
| `nodeAffinityPreset.type`     | Node affinity type: `soft` or `hard`                     | `""`    |
| `nodeAffinityPreset.key`      | Node label key to match                                  | `""`    |
| `nodeAffinityPreset.values`   | Node label values to match                               | `[]`    |
| `nodeSelector`                | Node labels for pod assignment                           | `{}`    |
| `tolerations`                 | Pod tolerations                                          | `[]`    |
| `topologySpreadConstraints`   | Topology spread constraints                              | `[]`    |
| `priorityClassName`           | Pod priority class                                       | `""`    |

## Pod Lifecycle

| Parameter                          | Description                              | Default |
|------------------------------------|------------------------------------------|---------|
| `terminationGracePeriodSeconds`    | Graceful shutdown time                   | `30`    |
| `lifecycle`                        | Container lifecycle hooks                | `{}`    |

## Pod Disruption Budget

| Parameter                       | Description                                    | Default  |
|---------------------------------|------------------------------------------------|----------|
| `podDisruptionBudget.create`    | Enable PDB creation                            | `false`  |
| `podDisruptionBudget.maxUnavailable` | Max unavailable pods (number or percentage) | `"1"`    |
| `podDisruptionBudget.minAvailable`   | Min available pods (number or percentage)   | (unset)  |

## Resources

| Parameter             | Description                | Default |
|-----------------------|----------------------------|---------|
| `resources.limits`    | Container resource limits  | `{}`    |
| `resources.requests`  | Container resource requests| `{}`    |

## Extra Configuration

| Parameter          | Description                                              | Default |
|--------------------|----------------------------------------------------------|---------|
| `extraVolumes`     | Additional volumes to add to the controller pod          | `[]`    |
| `extraVolumeMounts`| Additional volume mounts for the controller container    | `[]`    |
| `extraEnv`         | Additional environment variables (plain values or secretKeyRef) | `{}`    |

## Service Account

| Parameter                     | Description                            | Default |
|-------------------------------|----------------------------------------|---------|
| `serviceAccount.create`       | Create a ServiceAccount                | `true`  |
| `serviceAccount.name`         | ServiceAccount name (auto-generated)   | `""`    |
| `serviceAccount.annotations`  | ServiceAccount annotations             | `{}`    |

## Cluster Role

| Parameter                  | Description                                          | Default |
|----------------------------|------------------------------------------------------|---------|
| `clusterRole.annotations`  | Annotations for all ClusterRoles (e.g., for RBAC aggregation) | `{}`    |
