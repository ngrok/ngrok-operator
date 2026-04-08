# AgentEndpoint

> Represents a ngrok agent endpoint that establishes a tunnel from the ngrok edge to an upstream Kubernetes Service.

<!-- Last updated: 2026-04-08 -->

## Overview

An `AgentEndpoint` defines a ngrok endpoint backed by an agent tunnel. Each AgentEndpoint resource causes every instance of the Agent Manager to establish a tunnel to the specified upstream, enabling load balancing across replicas.

**API Group:** `ngrok.k8s.ngrok.com`
**Version:** `v1alpha1`
**Kind:** `AgentEndpoint`
**Short Name:** `aep`
**Categories:** `networking`, `ngrok`
**Scope:** Namespaced

## Spec Fields

| Field | Type | Required | Default | Description |
|-------|------|----------|---------|-------------|
| `url` | `string` | Yes | — | Public address. Accepts formats: domain (`example.com`), origin (`https://example.com:443`), scheme-only (`https://`), empty, or internal (`my-endpoint.internal`) |
| `upstream.url` | `string` | Yes | — | Upstream address. Accepts: origin, domain, scheme-only, or port-only (`:8080`) formats |
| `upstream.protocol` | `ApplicationProtocol` | No | — | Application protocol: `http1` or `http2` |
| `upstream.proxyProtocolVersion` | `ProxyProtocolVersion` | No | — | PROXY protocol version: `1` or `2` |
| `trafficPolicy.inline` | `json.RawMessage` | No | — | Inline traffic policy JSON |
| `trafficPolicy.targetRef` | `K8sObjectRef` | No | — | Reference to a `NgrokTrafficPolicy` resource by name (same namespace) |
| `description` | `string` | No | `"Created by the ngrok-operator"` | Human-readable description |
| `metadata` | `string` | No | `"{"owned-by":"ngrok-operator"}"` | JSON string of arbitrary key-value data |
| `bindings` | `[]string` | No | — | Binding IDs: `public`, `internal`, or `kubernetes`. Max 1 item |
| `clientCertificateRefs` | `[]K8sObjectRefOptionalNamespace` | No | — | References to Secrets containing TLS client certificates |

`trafficPolicy.inline` and `trafficPolicy.targetRef` are mutually exclusive.

## Status Fields

| Field | Type | Description |
|-------|------|-------------|
| `assignedURL` | `string` | The user-supplied or dynamically generated URL |
| `trafficPolicy` | `string` | Identifies the attached traffic policy: `inline`, `none`, or the name of the referenced NgrokTrafficPolicy |
| `domainRef` | `*K8sObjectRefOptionalNamespace` | Reference to the associated Domain CRD (nil for TCP, internal, or kubernetes-bound endpoints) |
| `conditions` | `[]metav1.Condition` | Standard Kubernetes conditions |

## Validation Rules

- `url` must be a parseable endpoint URL.
- `trafficPolicy` may specify either `inline` or `targetRef`, not both.
- `bindings` has a maximum of 1 item.
- `clientCertificateRefs` Secrets must contain `tls.crt` and `tls.key` keys.

## Relationships

| Related Resource | Relationship | Description |
|-----------------|--------------|-------------|
| `Domain` | References (via `status.domainRef`) | Domain reserved for this endpoint's hostname |
| `NgrokTrafficPolicy` | References (via `spec.trafficPolicy.targetRef`) | Traffic policy applied to this endpoint |
| `Secret` | References (via `spec.clientCertificateRefs`) | TLS client certificates for upstream mTLS |
| `Service` (LoadBalancer) | Owner | Created by the Service controller for LB Services |
| `Ingress` / `Gateway` | Indirect owner | Created by the Manager Driver during translation |

## Source References

| Symbol / Concept | File | Lines |
|-----------------|------|-------|
| AgentEndpoint types | `api/ngrok/v1alpha1/agentendpoint_types.go` | — |
| AgentEndpoint controller | `internal/controller/agent/agent_endpoint_controller.go` | — |
