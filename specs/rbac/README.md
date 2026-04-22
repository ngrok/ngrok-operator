# RBAC

## Overview

The ngrok-operator uses Kubernetes Role-Based Access Control (RBAC) to authorize its components to interact with the Kubernetes API. Three separate components each have their own ServiceAccount and RBAC configuration:

| Component            | Scope          | Purpose                                      |
|----------------------|----------------|----------------------------------------------|
| Operator (api-manager) | Cluster + Namespace | Main controller managing all CRDs and resources |
| Agent                | Cluster        | Agent tunnel management                       |
| Bindings Forwarder   | Cluster + Namespace | Endpoint binding forwarding                   |

## Design Principles

- **Least privilege**: Each component only has the permissions it needs.
- **Cluster-scoped for CRDs**: CRDs and cross-namespace resources require ClusterRoles.
- **Namespace-scoped for internal state**: Leader election and secret management use namespaced Roles.
- **Conditional permissions**: Some permissions (e.g., secret management for bindings) are only granted when the corresponding feature is enabled.

## Additional Roles

### Leader Election

The operator uses a namespaced Role for leader election, granting access to ConfigMaps, Leases (`coordination.k8s.io`), and Events in the operator's namespace. See [high-availability](../features/high-availability.md).

### Proxy Authentication

A ClusterRole grants the operator permission to create `TokenReviews` and `SubjectAccessReviews` for webhook authentication.

### Secret Management

When the bindings feature is enabled, a namespaced Role grants full CRUD access to Secrets in the operator's namespace for TLS certificate management.

### Aggregation Roles

Per-CRD editor and viewer ClusterRoles aggregate into Kubernetes built-in roles. See [aggregation.md](aggregation.md).

## Component Details

- [Operator RBAC](operator.md)
- [Agent RBAC](agent.md)
- [Bindings Forwarder RBAC](bindings-forwarder.md)
- [Aggregation Roles](aggregation.md)
