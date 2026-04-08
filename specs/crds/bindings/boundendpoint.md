# BoundEndpoint

> Represents a ngrok-managed endpoint bound to a Kubernetes Service, enabling cluster-local access to external ngrok endpoints.

<!-- Last updated: 2026-04-08 -->

## Overview

A `BoundEndpoint` CRD is created by the `BoundEndpointPoller` to represent an ngrok endpoint that should be exposed as a Kubernetes Service within the cluster. The poller syncs these from the ngrok API based on the KubernetesOperator's endpoint selectors.

**API Group:** `bindings.k8s.ngrok.com`
**Version:** `v1alpha1`
**Kind:** `BoundEndpoint`
**Scope:** Namespaced

## Spec Fields

| Field | Type | Required | Default | Description |
|-------|------|----------|---------|-------------|
| `endpointURL` | `string` | No | — | Endpoint URL identifier in format `<scheme>://<service>.<namespace>:<port>` |
| `endpointURI` | `string` | No | — | Deprecated, falls back from endpointURL |
| `scheme` | `string` | Yes | `https` | How data packets are framed: `tcp`, `http`, `https`, `tls` |
| `port` | `uint16` | Yes | — | Service port for upstream communication |
| `target.service` | `string` | Yes | — | Target service name |
| `target.namespace` | `string` | Yes | — | Target namespace |
| `target.protocol` | `string` | Yes | — | Service protocol (e.g., `TCP`) |
| `target.port` | `int32` | Yes | — | Service target port |
| `target.metadata` | `TargetMetadata` | No | — | Labels and annotations for the target service |

## Status Fields

| Field | Type | Description |
|-------|------|-------------|
| `endpoints` | `[]BindingEndpoint` | List of ngrok API endpoint references |
| `hashedName` | `string` | Hash of target service and namespace |
| `endpointsSummary` | `string` | Human-readable count (e.g., `"2 endpoints"`) |
| `conditions` | `[]metav1.Condition` | Standard Kubernetes conditions |
| `targetServiceRef` | `K8sObjectRefOptionalNamespace` | Reference to the created ExternalName Service |
| `upstreamServiceRef` | `K8sObjectRef` | Reference to the created ClusterIP Service |

## Status Conditions

| Condition | Meaning |
|-----------|---------|
| `ServicesCreated` | Both target and upstream Kubernetes Services exist |
| `ConnectivityVerified` | Connection test to the bound endpoint succeeded |
| `Ready` | Composite of the above |

## Materialized Services

Each BoundEndpoint creates two Kubernetes Services:

1. **Upstream Service** (ClusterIP) — in the operator's namespace, with a pod selector targeting the Bindings Forwarder pods. Listens on an allocated port.
2. **Target Service** (ExternalName) — in the target namespace, pointing to the upstream service's DNS name. This is what application workloads reference.

## Source References

| Symbol / Concept | File | Lines |
|-----------------|------|-------|
| BoundEndpoint types | `api/bindings/v1alpha1/boundendpoint_types.go` | — |
| BoundEndpoint controller | `internal/controller/bindings/boundendpoint_controller.go` | — |
| BoundEndpoint poller | `internal/controller/bindings/boundendpoint_poller.go` | — |
