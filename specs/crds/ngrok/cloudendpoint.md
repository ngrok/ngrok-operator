# CloudEndpoint

> Represents a ngrok cloud endpoint — a public-facing endpoint managed entirely by the ngrok API without an agent tunnel.

<!-- Last updated: 2026-04-08 -->

## Overview

A `CloudEndpoint` defines a ngrok endpoint that exists in the ngrok cloud. Unlike AgentEndpoints, CloudEndpoints do not require an agent tunnel — they are managed via the ngrok API and typically route traffic to internal AgentEndpoints using `forward-internal` traffic policy actions.

**API Group:** `ngrok.k8s.ngrok.com`
**Version:** `v1alpha1`
**Kind:** `CloudEndpoint`
**Short Name:** `clep`
**Categories:** `networking`, `ngrok`
**Scope:** Namespaced

## Spec Fields

| Field | Type | Required | Default | Description |
|-------|------|----------|---------|-------------|
| `url` | `string` | Yes | — | Public address. Same formats as AgentEndpoint |
| `trafficPolicyName` | `string` | No | — | Name of a `NgrokTrafficPolicy` resource (same namespace) |
| `trafficPolicy` | `*NgrokTrafficPolicySpec` | No | — | Inline traffic policy definition |
| `poolingEnabled` | `*bool` | No | — | Allow pooling with other CloudEndpoints sharing the same URL |
| `description` | `string` | No | `"Created by the ngrok-operator"` | Human-readable description |
| `metadata` | `string` | No | `"{"owned-by":"ngrok-operator"}"` | JSON string of arbitrary key-value data |
| `bindings` | `[]string` | No | — | Binding IDs: `public`, `internal`, or `kubernetes`. Max 1 item |

`trafficPolicyName` and `trafficPolicy` are mutually exclusive.

## Status Fields

| Field | Type | Description |
|-------|------|-------------|
| `id` | `string` | ngrok API endpoint identifier |
| `domainRef` | `*K8sObjectRefOptionalNamespace` | Reference to the associated Domain CRD |
| `conditions` | `[]metav1.Condition` | Standard Kubernetes conditions |

## Relationships

| Related Resource | Relationship | Description |
|-----------------|--------------|-------------|
| `Domain` | References (via `status.domainRef`) | Domain reserved for this endpoint's hostname |
| `NgrokTrafficPolicy` | References (via `spec.trafficPolicyName`) | Traffic policy applied to this endpoint |
| `Service` (LoadBalancer) | Owner | Created by the Service controller when using `endpoints-verbose` mapping |
| `Ingress` / `Gateway` | Indirect owner | Created by the Manager Driver during translation |

## Source References

| Symbol / Concept | File | Lines |
|-----------------|------|-------|
| CloudEndpoint types | `api/ngrok/v1alpha1/cloudendpoint_types.go` | — |
| CloudEndpoint controller | `internal/controller/ngrok/cloudendpoint_controller.go` | — |
