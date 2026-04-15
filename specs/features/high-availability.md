# High Availability

## Overview

The ngrok-operator supports running multiple replicas for high availability. Only one replica actively reconciles at a time via leader election; standby replicas take over if the leader fails.

## Replica Configuration

| Component          | Helm Value                            | Default | Recommendation       |
|--------------------|---------------------------------------|---------|----------------------|
| API Manager        | `apiManager.replicaCount`             | `1`     | 2+ in production     |
| Agent              | `agent.replicaCount`                  | `1`     |                      |
| Bindings Forwarder | `bindingsForwarder.replicaCount`      | `1`     |                      |

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

| Helm Value                                    | Description                                    | Default |
|-----------------------------------------------|------------------------------------------------|---------|
| `apiManager.podDisruptionBudget.create`       | Enable PDB creation                            | `false` |
| `apiManager.podDisruptionBudget.maxUnavailable` | Max unavailable pods                         | `"1"`   |
| `apiManager.podDisruptionBudget.minAvailable`   | Min available pods                           | (unset) |

## Anti-Affinity

Anti-affinity is configured via the standard `affinity` field on each component (or `global.affinity` for all components). There are no preset helpers — write affinity rules directly.

| Helm Value                | Description                                    | Default |
|---------------------------|------------------------------------------------|---------|
| `global.affinity`         | Affinity rules for all components              | `{}`    |
| `apiManager.affinity`     | Affinity rules for the api-manager (overrides global) | `{}`    |

## Drain State Across Replicas

When draining is initiated, the drain state propagates across replicas via the KubernetesOperator CR's `status.drainStatus` field. This ensures all replicas observe the drain state regardless of which replica is the leader.
