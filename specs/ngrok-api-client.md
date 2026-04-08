# ngrok API Client

> Typed client abstraction wrapping the ngrok-api-go library for CRUD operations on ngrok platform resources.

<!-- Last updated: 2026-04-08 -->

## Overview

The `internal/ngrokapi` package provides a `Clientset` interface that wraps the `ngrok-api-go/v7` client library. This abstraction layer enables testability (via mock implementations) and provides a single point of configuration for all ngrok API interactions.

## Clientset Interface

```go
type Clientset interface {
    Domains() DomainClient
    Endpoints() EndpointsClient
    IPPolicies() IPPoliciesClient
    IPPolicyRules() IPPolicyRulesClient
    KubernetesOperators() KubernetesOperatorsClient
    TCPAddresses() TCPAddressesClient
}
```

## Client Operations

Each resource client implements a combination of generic CRUD interfaces:

| Interface | Methods | Description |
|-----------|---------|-------------|
| `Creator[R, T]` | `Create(ctx, request) → (resource, error)` | Create a new resource |
| `Reader[T]` | `Get(ctx, id) → (resource, error)` | Read a resource by ID |
| `Updater[R, T]` | `Update(ctx, request) → (resource, error)` | Update an existing resource |
| `Deletor` | `Delete(ctx, id) → error` | Delete a resource by ID |
| `Lister[T]` | `List(paging) → Iter[T]` | Paginated listing |

### Per-Resource Capabilities

| Client | Create | Read | Update | Delete | List | Extra |
|--------|--------|------|--------|--------|------|-------|
| `DomainClient` | Yes | Yes | Yes | Yes | Yes | — |
| `EndpointsClient` | Yes | Yes | Yes | Yes | Yes | — |
| `IPPoliciesClient` | Yes | Yes | Yes | Yes | — | — |
| `IPPolicyRulesClient` | Yes | — | Yes | Yes | Yes | — |
| `KubernetesOperatorsClient` | Yes | Yes | Yes | Yes | Yes | `GetBoundEndpoints(id, paging)` |
| `TCPAddressesClient` | Yes | — | Yes | — | Yes | — |

### Additional Components

- **`enriched_errors.go`**: Defines operator-specific error codes (`NgrokOpErr*`) for well-known failure scenarios:
  - `NgrokOpErrFailedToCreateCSR` — CSR creation failure
  - `NgrokOpErrFailedToCreateUpstreamService` / `NgrokOpErrFailedToCreateTargetService` — Service creation failures
  - `NgrokOpErrEndpointDenied` — Endpoint denied by policy
  - `NgrokOpErrInvalidCIDR` — Invalid CIDR in IP policy rules

- **`bindingendpoint_aggregator.go`**: Aggregates binding endpoint data from the ngrok API, grouping endpoints by service target for the `BoundEndpointPoller`.

## Source References

| Symbol / Concept | File | Lines |
|-----------------|------|-------|
| Clientset interface | `internal/ngrokapi/clientset.go` | 15–22 |
| DefaultClientset | `internal/ngrokapi/clientset.go` | 24–43 |
| Client interfaces | `internal/ngrokapi/clientset.go` | 45–61 |
| Enriched errors | `internal/ngrokapi/enriched_errors.go` | — |
| Binding aggregator | `internal/ngrokapi/bindingendpoint_aggregator.go` | — |
