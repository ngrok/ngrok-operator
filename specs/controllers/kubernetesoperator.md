# KubernetesOperator Controller

## Executive Summary

The KubernetesOperator controller manages the operator's registration with the ngrok API. It reconciles a single `KubernetesOperator` CR per operator deployment, handling feature configuration, TLS certificate management, and the drain workflow on deletion.

## Watches

| Resource              | Relation   | Predicate                                    |
|-----------------------|------------|----------------------------------------------|
| `KubernetesOperator`  | Primary    | Name+namespace predicate (only its own CR) + AnnotationChanged or GenerationChanged |

The controller only reconciles the specific KubernetesOperator CR matching the operator's Helm release name and namespace.

## Reconciliation Flow

1. Generate or fetch a self-signed TLS certificate for mTLS (bindings feature).
2. Find or create the KubernetesOperator remote resource in the ngrok API.
3. Update the remote resource with feature configuration (`enabledFeatures`, `binding`, `deployment`).
4. Store the bindings ingress endpoint in status.
5. Call `ReconcileStatus()`.

## Delete Flow

Deletion of the KubernetesOperator CR triggers the drain orchestration workflow:

1. Set `status.drainStatus` to `draining`.
2. Set in-memory drain flag for fast propagation.
3. Run `Drainer.DrainAll()` to process all managed resources.
4. Update drain status with progress and errors.
5. Remove finalizer on completion.

See [features/draining.md](../features/draining.md) for full details.

## Created Resources

- KubernetesOperator remote resource (via ngrok API)
- TLS Secret for mTLS certificate (when bindings enabled)

## Status

| Field                      | Description                                         |
|----------------------------|-----------------------------------------------------|
| `id`                       | ngrok API resource ID                               |
| `uri`                      | ngrok API resource URI                              |
| `registrationStatus`       | `registered`, `error`, or `pending`                 |
| `enabledFeatures`          | Comma-separated enabled features                    |
| `bindingsIngressEndpoint`  | Resolved bindings ingress endpoint                  |
| Drain fields               | `drainStatus`, `drainMessage`, `drainProgress`, `drainErrors` |

## Notes

- Only one KubernetesOperator CR should exist per operator deployment.
- The namespace+name predicate ensures each operator instance only manages its own CR.
- See [features/multi-install.md](../features/multi-install.md) for multi-operator deployments.
