# High Availability

## Overview

The ngrok-operator supports running multiple replicas for high availability. Only one replica actively reconciles at a time via leader election; standby replicas take over if the leader fails.

## Replica Configuration

| Component          | Helm Value                            | Default | Recommendation       |
|--------------------|---------------------------------------|---------|----------------------|
| API Manager        | `apiManager.replicaCount`             | `1`     | 2+ in production     |
| Agent              | `agent.replicaCount`                  | `1`     | 2+ in production (see note below) |
| Bindings Forwarder | `bindingsForwarder.replicaCount`      | `1`     | 2+ in production (see note below) |

> **Agent and Bindings Forwarder**: Unlike the API Manager, these components do not use leader election — all replicas are active simultaneously. Running 2+ replicas provides redundancy: if one pod is lost, active connections are re-established through the remaining replicas. This comes at the cost of additional ngrok agent connections (one per replica), which may affect account limits. Set `podDisruptionBudget.create: true` to protect replicas during cluster maintenance.

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

Each component has independent PDB configuration. See the component Helm specs for per-component values.

| Helm Value                                           | Description                                    | Default |
|------------------------------------------------------|------------------------------------------------|---------|
| `apiManager.podDisruptionBudget.create`              | Enable PDB for api-manager                     | `false` |
| `apiManager.podDisruptionBudget.maxUnavailable`      | Max unavailable pods                           | `"1"`   |
| `apiManager.podDisruptionBudget.minAvailable`        | Min available pods                             | (unset) |
| `agent.podDisruptionBudget.create`                   | Enable PDB for agent                           | `false` |
| `agent.podDisruptionBudget.maxUnavailable`           | Max unavailable pods                           | `"1"`   |
| `agent.podDisruptionBudget.minAvailable`             | Min available pods                             | (unset) |
| `bindingsForwarder.podDisruptionBudget.create`       | Enable PDB for bindings-forwarder              | `false` |
| `bindingsForwarder.podDisruptionBudget.maxUnavailable` | Max unavailable pods                         | `"1"`   |
| `bindingsForwarder.podDisruptionBudget.minAvailable` | Min available pods                             | (unset) |

## Anti-Affinity

Anti-affinity is configured via the standard `affinity` field on each component (or `global.affinity` for all components). There are no preset helpers — write affinity rules directly.

| Helm Value                | Description                                    | Default |
|---------------------------|------------------------------------------------|---------|
| `global.affinity`         | Affinity rules for all components              | `{}`    |
| `apiManager.affinity`     | Affinity rules for the api-manager (overrides global) | `{}`    |

## Leader Election Scope

Leader election applies **only to the API Manager**. With multiple API Manager replicas, only the elected leader actively reconciles resources; standby replicas watch for lease expiry. The agent and bindings forwarder do not use leader election — all replicas are active.

## Drain State Across Replicas

When draining is initiated, the drain state propagates across replicas via the KubernetesOperator CR's `status.drainStatus` field. This ensures all replicas observe the drain state regardless of which replica is the leader.
