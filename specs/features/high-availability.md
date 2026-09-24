# High Availability

## Overview

The ngrok-operator supports running multiple replicas for high availability. Only one replica actively reconciles at a time via leader election; standby replicas take over if the leader fails.

## Replica Configuration

| Component          | Helm Value                            | Default | Recommendation       |
|--------------------|---------------------------------------|---------|----------------------|
| API Manager        | `apiManager.replicaCount`             | `1`     | 2+ in production     |
| Agent              | `agent.replicaCount`                  | `1`     | 2+ in production (see note below) |
| Bindings Forwarder | `bindingsForwarder.replicaCount`      | `1`     | 2+ in production (see note below) |

> **Agent and Bindings Forwarder**: Unlike the API Manager, these components do not use leader election — all replicas are active simultaneously. Running 2+ replicas provides redundancy: if one pod is lost, active connections are re-established through the remaining replicas. This comes at the cost of additional ngrok agent connections (one per replica), which may affect account limits. Neither component has a PodDisruptionBudget (see below), so there is no chart setting to protect their replicas during cluster maintenance.

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

Only the api-manager has a PodDisruptionBudget. The agent and bindings-forwarder do not — there is no `agent.podDisruptionBudget` or `bindingsForwarder.podDisruptionBudget`; setting either is rejected by the chart's schema.

| Helm Value                                      | Description                                                       | Default |
|--------------------------------------------------|---------------------------------------------------------------------|---------|
| `apiManager.podDisruptionBudget.create`          | Enable PDB for api-manager                                          | `false` |
| `apiManager.podDisruptionBudget.maxUnavailable`  | Max unavailable pods                                                 | `"1"`   |
| `apiManager.podDisruptionBudget.minAvailable`    | Min available pods. Set this instead of `maxUnavailable`, not alongside it | (unset) |

## Anti-Affinity

Affinity is `defaults.*`, applied to all three components (api-manager, agent, bindings-forwarder), with `<component>.*` overriding it for one. There is no `global.affinity`: `global` is a passthrough for the chart's subcharts, not a value this chart reads, so setting it renders nothing and is silently accepted rather than rejected — set `defaults.affinity` instead.

Preset helpers exist and are the normal way to set anti-affinity, not raw affinity rules: `podAffinityPreset`, `podAntiAffinityPreset`, and `nodeAffinityPreset.{type,key,values}`, each `""`/unset by default except `podAntiAffinityPreset`, which defaults to `soft`. Setting `affinity` directly overrides the presets for that component.

| Helm Value                             | Description                                                              | Default  |
|------------------------------------------|---------------------------------------------------------------------------|----------|
| `defaults.affinity`                      | Affinity rules for every component's pods. Overrides the presets below when set | `{}`     |
| `defaults.podAffinityPreset`             | Pod affinity preset. Ignored if `affinity` is set. `""`, `soft` or `hard`  | `""`     |
| `defaults.podAntiAffinityPreset`         | Pod anti-affinity preset. Ignored if `affinity` is set. `""`, `soft` or `hard` | `soft`   |
| `defaults.nodeAffinityPreset.type`       | Node affinity preset type. Ignored if `affinity` is set. `""`, `soft` or `hard` | `""`     |
| `defaults.nodeAffinityPreset.key`        | Node label key to match. Ignored if `affinity` is set                     | `""`     |
| `defaults.nodeAffinityPreset.values`     | Node label values to match. Ignored if `affinity` is set                  | `[]`     |
| `apiManager.affinity` (and the presets above, per-component) | Same keys, override the shared value for the api-manager alone | `{}`     |

## Leader Election Scope

Leader election applies **only to the API Manager**. With multiple API Manager replicas, only the elected leader actively reconciles resources; standby replicas watch for lease expiry. The agent and bindings forwarder do not use leader election — all replicas are active.

## Drain State Across Replicas

When draining is initiated, the drain state propagates across replicas via the KubernetesOperator CR's `Draining` status condition. This ensures all replicas observe the drain state regardless of which replica is the leader.
