# Bindings Controllers

> Controllers that manage ngrok endpoint bindings — exposing ngrok-managed endpoints as Kubernetes Services.

<!-- Last updated: 2026-04-08 -->

## Overview

The Bindings controller group implements the ngrok endpoint bindings feature (in-development). Bindings allow ngrok-managed endpoints to be exposed as Kubernetes Services within a cluster, enabling pods to consume external ngrok endpoints as if they were local services.

The controllers are split across two binaries:
- **API Manager**: Runs the `BoundEndpointReconciler` and the `BoundEndpointPoller`.
- **Bindings Forwarder Manager**: Runs the `ForwarderReconciler` and a copy of `BoundEndpointReconciler`.

All bindings controllers are gated behind the `--enable-feature-bindings` flag (disabled by default).

## Controllers

| Controller | Primary Resource | Secondary Watches | Owned Resources |
|------------|-----------------|-------------------|-----------------|
| `BoundEndpointReconciler` | `BoundEndpoint` | `Service`, `Namespace` | Target Service (ExternalName), Upstream Service (ClusterIP) |
| `ForwarderReconciler` | `BoundEndpoint` | — | Connection forwarding (via BindingsDriver) |
| `BoundEndpointPoller` | — (periodic poll) | — | `BoundEndpoint` CRDs (CRUD) |

## Reconciliation Logic

### BoundEndpointPoller

A long-running service that periodically polls the ngrok API for binding endpoints associated with the registered `KubernetesOperator`.

1. Fetches desired binding endpoints from the ngrok API via `KubernetesOperatorsClient.GetBoundEndpoints()`.
2. Aggregates API endpoints by service target.
3. Compares the desired state with existing `BoundEndpoint` CRDs in the cluster.
4. Creates, updates, or deletes `BoundEndpoint` resources to match the API state.
5. Failed actions are retried in background goroutines on a 2-second tick until successful.
6. Respects drain state — skips creating new resources during shutdown.

### BoundEndpointReconciler

Converts each `BoundEndpoint` into two Kubernetes Services:

1. **Upstream Service** (ClusterIP): Created in the operator's namespace with a pod selector targeting the Bindings Forwarder pods. Allocated a port from the `PortAllocator`.
2. **Target Service** (ExternalName): Created in the target namespace, pointing to the upstream service. This is what application pods reference.

On each reconcile, the controller also tests connectivity to the bound endpoint with exponential backoff retry logic.

**Status conditions:**

| Condition | Meaning |
|-----------|---------|
| `ServicesCreated` | Both upstream and target services exist |
| `ConnectivityVerified` | Connection test to the bound endpoint succeeded |
| `Ready` | Composite: all conditions are healthy |

**Requeue strategy:** Periodic refresh at a configured interval.

### ForwarderReconciler

Manages the data plane for bound endpoints. For each `BoundEndpoint`:

1. Allocates a listening port via the `PortAllocator`.
2. Establishes a TLS connection to the ngrok ingress endpoint.
3. Accepts incoming connections from the upstream ClusterIP Service.
4. Looks up the connecting pod's identity by source IP.
5. Upgrades the connection to a binding connection with pod identity metadata (protobuf framing via `internal/mux`).
6. Joins the client and ngrok server connections bidirectionally.

### PortAllocator

A thread-safe utility (`internal/controller/bindings/port_allocator.go`) that manages a bitmap of available ports within a configured range. Used by both `BoundEndpointReconciler` and `ForwarderReconciler` to allocate unique ports for each bound endpoint.

## Source References

| Symbol / Concept | File | Lines |
|-----------------|------|-------|
| BoundEndpoint controller | `internal/controller/bindings/boundendpoint_controller.go` | — |
| Forwarder controller | `internal/controller/bindings/forwarder_controller.go` | — |
| BoundEndpoint poller | `internal/controller/bindings/boundendpoint_poller.go` | — |
| Port allocator | `internal/controller/bindings/port_allocator.go` | — |
| BoundEndpoint conditions | `internal/controller/bindings/boundendpoint_conditions.go` | — |
| Bindings driver | `pkg/bindingsdriver/driver.go` | — |
| Connection upgrade | `internal/mux/header.go` | — |
| Protobuf messages | `internal/pb_agent/conn_header.pb.go` | — |
