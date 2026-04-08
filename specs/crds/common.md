# Common CRD Types

> Shared types, enums, and interfaces used across API groups.

<!-- Last updated: 2026-04-08 -->

## Overview

The operator defines shared types in two packages that are referenced by CRDs across all API groups.

## `api/common/v1alpha1`

| Type | Kind | Values / Fields | Description |
|------|------|-----------------|-------------|
| `ApplicationProtocol` | `string` enum | `http1`, `http2` | Protocol used for upstream communication |
| `ProxyProtocolVersion` | `string` enum | `1`, `2` | PROXY protocol version for upstream |
| `DefaultClusterDomain` | `const` | `svc.cluster.local` | Default Kubernetes cluster domain |

## `api/ngrok/v1alpha1` (common_types.go)

| Type | Fields | Description |
|------|--------|-------------|
| `K8sObjectRef` | `name` (required) | Reference to a Kubernetes object by name |
| `K8sObjectRefOptionalNamespace` | `name` (required), `namespace` (optional) | Reference to a Kubernetes object with optional namespace |
| `EndpointWithDomain` | Interface: `GetURL()`, `GetBindings()`, `GetDomainRef()`, `SetDomainRef()`, `GetConditions()`, `GetNamespace()`, `GetGeneration()` | Interface implemented by AgentEndpoint and CloudEndpoint to support Domain Manager operations |

## Source References

| Symbol / Concept | File | Lines |
|-----------------|------|-------|
| Common enums | `api/common/v1alpha1/common_types.go` | — |
| K8s object refs | `api/ngrok/v1alpha1/common_types.go` | — |
