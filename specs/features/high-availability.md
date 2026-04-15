# High Availability

## Overview

The ngrok-operator supports running multiple replicas for high availability. Only one replica actively reconciles at a time via leader election; standby replicas take over if the leader fails.

## Replica Configuration

| Component          | Helm Value                          | Default | Recommendation       |
|--------------------|-------------------------------------|---------|----------------------|
| Operator           | `replicaCount`                      | `1`     | 2+ in production     |
| Agent              | `agent.replicaCount`                | `1`     |                      |
| Bindings Forwarder | `bindings.forwarder.replicaCount`   | `1`     |                      |

## Leader Election

Leader election ensures only one operator replica actively reconciles at a time.

| Setting         | Description                               | Default                     |
|-----------------|-------------------------------------------|-----------------------------|
| `--election-id` | ConfigMap/Lease name for leader election  | `ngrok-operator-leader`     |

- **Applies to:** api-manager only. Agent-manager and bindings-forwarder have leader election disabled.
- **Mechanism:** controller-runtime's lease-based election via `coordination.k8s.io`.
- **Leader loss:** When the leader pod is lost, the lease expires (~15 seconds default TTL) and a standby replica acquires leadership.
- **Graceful shutdown:** Signal handlers allow cleanup before relinquishing leadership.

## Pod Disruption Budget

| Helm Value                         | Description                                    | Default |
|------------------------------------|------------------------------------------------|---------|
| `podDisruptionBudget.create`       | Enable PDB creation                            | `false` |
| `podDisruptionBudget.maxUnavailable` | Max unavailable pods                         | `"1"`   |
| `podDisruptionBudget.minAvailable`   | Min available pods                           | (unset) |

## Anti-Affinity

The operator defaults to `podAntiAffinityPreset: soft`, which encourages (but does not require) spreading replicas across different nodes.

| Helm Value               | Description                                    | Default |
|--------------------------|------------------------------------------------|---------|
| `podAntiAffinityPreset`  | Pod anti-affinity: `soft` or `hard`            | `soft`  |
| `affinity`               | Full affinity spec (overrides all presets)      | `{}`    |

## Drain State Across Replicas

When draining is initiated, the drain state propagates across replicas via the KubernetesOperator CR's `status.drainStatus` field. This ensures all replicas observe the drain state regardless of which replica is the leader.
