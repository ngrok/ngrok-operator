# KubernetesOperator Controller

## Summary

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

## Cluster Identity and Deduplication

The controller stores the UID of the operator's release namespace in the ngrok API resource metadata under the key `namespace.uid`. On each reconcile, it compares the stored UID with the current namespace UID:

- **Match**: The remote resource belongs to this cluster — proceed with updates.
- **Mismatch**: The remote resource was created by a different cluster (e.g., a local dev cluster reusing the same Helm release name after the remote resource was not cleaned up). The controller creates a new ngrok API resource rather than overwriting the existing one.

This prevents different clusters with identical release names from clobbering each other's ngrok API registrations. The tradeoff is that ephemeral clusters (local dev, CI) that are deleted without running `helm uninstall` leak orphaned resources in the ngrok API.

## TLS Secret Lifecycle

The controller calls `findOrCreateTLSSecret` to manage the operator's mTLS certificate:

1. Reads the Secret named `default-tls` (or the configured name) from the release namespace.
2. If it doesn't exist, generates a self-signed certificate and creates the Secret.
3. Submits a CSR to the ngrok API so the certificate is trusted for bindings mTLS.
4. On subsequent reconciles, reads the existing Secret and refreshes the CSR if the certificate is near expiry.

The Secret always lives in the release namespace, regardless of `watchNamespace`. See [rbac/README.md](../rbac/README.md) for the cache scope caveat when `watchNamespace ≠ release namespace`.

## Notes

- Only one KubernetesOperator CR should exist per operator deployment.
- The namespace+name predicate ensures each operator instance only manages its own CR.
- See [features/multi-install.md](../features/multi-install.md) for multi-operator deployments.
