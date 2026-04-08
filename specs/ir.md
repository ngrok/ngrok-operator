# Intermediate Representation (IR) Layer

> Translation layer that converts Kubernetes Ingress and Gateway API resources into a unified, protocol-agnostic model for endpoint materialization.

<!-- Last updated: 2026-04-08 -->

## Overview

The IR package (`internal/ir`) provides a midway representation between Kubernetes networking resources (Ingress, Gateway API) and the ngrok endpoint CRDs (CloudEndpoint, AgentEndpoint). This decouples input resource handling from endpoint generation, allowing the same materialization logic to handle multiple input formats.

## Core Types

### IRVirtualHost

The primary IR type representing a unique hostname and all routing configuration for that hostname.

| Field | Type | Description |
|-------|------|-------------|
| `NamePrefix` | `*string` | Optional name prefix for generated endpoints (enables multiple endpoints per hostname, e.g., from different Gateways) |
| `Namespace` | `string` | Namespace for the hostname — cross-namespace duplicates are not allowed |
| `Listener` | `IRListener` | Hostname, port, and protocol |
| `EndpointPoolingEnabled` | `*bool` | Explicit pooling control (nil = API default) |
| `TrafficPolicy` | `*TrafficPolicy` | Host-level traffic policy applied to all routes |
| `Routes` | `[]*IRRoute` | Routing rules with match criteria and destinations |
| `DefaultDestination` | `*IRDestination` | Fallback destination (default backend) |
| `TLSTermination` | `*IRTLSTermination` | TLS termination configuration |
| `ClientCertRefs` | `[]IRObjectRef` | Client certificate references for upstream mTLS |
| `MappingStrategy` | `IRMappingStrategy` | How this VirtualHost is materialized into endpoints |
| `CollapseIntoServiceKey` | `*IRServiceKey` | When set, this VirtualHost can be collapsed into a single public AgentEndpoint |
| `Bindings` | `[]string` | Binding IDs for generated endpoints |
| `OwningResources` | `[]OwningResource` | References to the Ingress/Gateway resources that contributed to this VirtualHost |

### IRListener

| Field | Type | Values |
|-------|------|--------|
| `Hostname` | `IRHostname` | The hostname to listen on |
| `Port` | `int32` | Port number |
| `Protocol` | `IRProtocol` | `HTTPS`, `HTTP`, `TCP`, `TLS` |

### IRRoute

| Field | Type | Description |
|-------|------|-------------|
| `HTTPMatchCriteria` | `*IRHTTPMatch` | Path, headers, query params, method matching |
| `TrafficPolicies` | `[]*TrafficPolicy` | Per-route non-terminating policies (e.g., header manipulation) |
| `Destinations` | `[]*IRDestination` | Weighted list of upstream targets |

### IRHTTPMatch

| Field | Type | Description |
|-------|------|-------------|
| `Path` | `*string` | Path to match |
| `PathType` | `*IRPathMatchType` | `prefix`, `exact`, or `regex` |
| `Headers` | `[]IRHeaderMatch` | Header match criteria (name, value, exact/regex) |
| `QueryParams` | `[]IRQueryParamMatch` | Query parameter match criteria |
| `Method` | `*IRMethodMatch` | HTTP method to match |

### IRDestination

| Field | Type | Description |
|-------|------|-------------|
| `Weight` | `*int` | Traffic weight (> 0 when set; nil = 100%) |
| `Upstream` | `*IRUpstream` | Target upstream service |
| `TrafficPolicies` | `[]*TrafficPolicy` | Destination-specific policies |

### IRService

| Field | Type | Description |
|-------|------|-------------|
| `UID` | `string` | Service UID (ensures uniqueness across clusters) |
| `Namespace` | `string` | Service namespace |
| `Name` | `string` | Service name |
| `Port` | `int32` | Service port |
| `Scheme` | `IRScheme` | `http://`, `https://`, `tcp://`, `tls://` |
| `Protocol` | `*ApplicationProtocol` | Application protocol (`http1`, `http2`) |
| `ClientCertRefs` | `[]IRObjectRef` | Client certificates for mTLS to this service |

The `Key()` method generates a unique key from `UID/Namespace/Name/Port[/Protocol][/CertRefs]`.

## Mapping Strategies

| Strategy | Constant | Behavior |
|----------|----------|----------|
| **Collapsed** (default) | `endpoints-collapsed` | Single-upstream hostnames get one public AgentEndpoint. Multi-upstream hostnames get a CloudEndpoint + internal AgentEndpoints with generated routing policy. Cost-effective at low agent replica counts. |
| **Verbose** | `endpoints-verbose` | Every hostname gets a CloudEndpoint. Every unique upstream gets an internal AgentEndpoint. AgentEndpoints are reusable across hostnames. Cost-effective at higher agent replica counts. |

The cost trade-off: AgentEndpoints scale in cost with the agent deployment replica count (each pod creates its own tunnel), while CloudEndpoints have a static cost regardless of replicas.

## Route Sorting

`IRVirtualHost.SortRoutes()` orders routes from most specific to least specific:

1. **Path**: Exact > Prefix > Regex. Longer paths first. Lexicographic tiebreak.
2. **Headers**: Routes with header matchers before those without.
3. **Query params**: Routes with query param matchers before those without.
4. **Method**: Routes specifying a method before those that don't.

This ensures that the generated traffic policy rules match traffic in the correct order.

## Source References

| Symbol / Concept | File | Lines |
|-----------------|------|-------|
| IR types | `internal/ir/ir.go` | 1–105 |
| Mapping strategies | `internal/ir/ir.go` | 125–161 |
| Route sorting | `internal/ir/ir.go` | 302–390 |
| IRService key generation | `internal/ir/ir.go` | 275–285 |
