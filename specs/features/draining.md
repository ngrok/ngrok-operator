# Draining

## Overview

Draining is the process of gracefully removing ngrok API resources when the operator is uninstalled. It ensures that endpoints, domains, and other resources are cleaned up according to a configurable policy.

## Trigger

Draining is triggered by deletion of the KubernetesOperator CR. This typically happens via the Helm pre-delete cleanup hook, but can also be triggered manually.

## Drain Workflow

1. KubernetesOperator deletion is detected (DeletionTimestamp set).
2. Controller sets `status.drainStatus` to `draining`.
3. In-memory drain flag is set via `StateChecker.SetDraining()` for fast propagation to other controllers in the same pod.
4. A 2-second pause allows other controllers to observe the drain state.
5. `Drainer.DrainAll()` processes all managed resources.
6. Status is updated with progress, errors, and final outcome.
7. On completion, the finalizer is removed and the KubernetesOperator CR is deleted.

## Drain State Propagation

- The drain state is **monotonic** — once set to draining, it never resets.
- Other controllers check `DrainState.IsDraining()` before processing non-delete reconciles.
- During drain, create/update reconciles are skipped (no new finalizers are added).
- Delete reconciles proceed normally to allow cleanup.
- Across multiple replicas, drain state propagates via the KubernetesOperator CR's `status.drainStatus` field.

## Resource Processing

### User Resources

User-created resources (HTTPRoute, TCPRoute, TLSRoute, Ingress, Service, Gateway) are processed by **removing their finalizers only**. This allows Kubernetes garbage collection to delete them without waiting for controller cleanup.

### Operator Resources

Operator-managed resources (CloudEndpoint, AgentEndpoint, Domain, IPPolicy, BoundEndpoint) are processed according to the drain policy:

| Policy   | Behavior                                                        |
|----------|-----------------------------------------------------------------|
| `Delete` | Issues delete on the CR. The controller handles ngrok API cleanup before removing the finalizer. |
| `Retain` | Only removes finalizers. ngrok API resources are preserved.     |

Deletion polling waits up to 60 seconds at 500ms intervals for each resource to be fully deleted.

## Drain Outcomes

| Outcome    | Description                                          |
|------------|------------------------------------------------------|
| `Complete` | All resources drained successfully                   |
| `Retry`    | Transient errors encountered; will retry             |
| `Failed`   | Non-transient errors; drain cannot complete          |

## Configuration

| Source                         | Parameter              | Default    |
|--------------------------------|------------------------|------------|
| KubernetesOperator CR          | `spec.drain.policy`    | `Retain`   |
| Helm values                    | `features.drainPolicy` | `"Retain"` |

## Cleanup Hook

The Helm chart includes a pre-delete hook that automates the drain process during `helm uninstall`:

1. A Kubernetes Job runs with `helm.sh/hook: pre-delete` annotation.
2. The job executes `kubectl delete kubernetesoperator <release-name> -n <namespace>`.
3. It waits for the KubernetesOperator CR to be fully deleted (with timeout).
4. The job self-deletes on success.

| Helm Value                | Description                    | Default            |
|---------------------------|--------------------------------|--------------------|
| `cleanupHook.enabled`     | Enable the pre-delete hook     | `true`             |
| `cleanupHook.timeout`     | Timeout in seconds             | `300`              |
| `cleanupHook.image`       | kubectl image configuration    | `bitnami/kubectl`  |
| `cleanupHook.resources`   | Job resource limits/requests   | 100m/128Mi         |
