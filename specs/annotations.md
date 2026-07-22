# Annotations Reference

## Overview

The ngrok-operator uses annotations under the `ngrok.com/` prefix to configure behavior on Kubernetes resources. This document serves as a central reference. See individual CRD specs in [crds/](crds/) for full details on how each annotation is used.

Other user-supplied ngrok-prefixed key namespaces that are *not* object annotations are specced where they belong: Gateway listener TLS option keys in [features/gateway-api.md](features/gateway-api.md), recognized Service port `appProtocol` field values in [upstream-protocols.md](upstream-protocols.md), and pod annotations forwarded as bindings pod identity in [features/bindings.md](features/bindings.md).

During the `k8s.ngrok.com/` → `ngrok.com/` migration window the operator also reads every annotation below under the legacy `k8s.ngrok.com/` prefix; when both prefixes are present on the same object, the `ngrok.com/` key wins. Precedence is decided by key *presence*, even when the canonical value is empty or invalid — what an empty value then means differs per annotation (most annotations reject an empty value as invalid content; `app-protocols` treats empty as unset). Legacy-prefix support is removed in 1.0. See [`docs/v1-migration-guide.md`](../docs/v1-migration-guide.md).

## User-Configurable Annotations

### `ngrok.com/url`

Specifies the public URL for an endpoint.

| Detail          | Value                                                  |
|-----------------|--------------------------------------------------------|
| Applies to      | `Service` (LoadBalancer)                               |
| Default         | (none — a dynamic TCP address is assigned)             |
| Examples        | `tcp://1.tcp.ngrok.io:12345`, `tcp://`, `tls://example.com` |

See: [controllers/service.md](controllers/service.md)

### `ngrok.com/mapping-strategy`

Controls which ngrok endpoint resources are created for a given resource.

| Detail          | Value                                                  |
|-----------------|--------------------------------------------------------|
| Applies to      | `Service` (LoadBalancer), `Ingress`, `Gateway` routes  |
| Allowed values  | `endpoints`, `endpoints-verbose`                       |
| Default         | `endpoints`                                            |

- `endpoints`: Creates only an `AgentEndpoint`.
- `endpoints-verbose`: Creates both a `CloudEndpoint` and an internal `AgentEndpoint` (URL ending in `.internal`).

See: [controllers/service.md](controllers/service.md), [controllers/ingress.md](controllers/ingress.md)

### `ngrok.com/traffic-policy`

References an `TrafficPolicy` resource in the same namespace to apply to the created endpoint(s).

| Detail          | Value                                                  |
|-----------------|--------------------------------------------------------|
| Applies to      | `Service` (LoadBalancer), `Ingress`, `Gateway` routes  |
| Value           | Name of an `TrafficPolicy` resource               |
| Default         | (none)                                                 |

When `mapping-strategy` is `endpoints-verbose`, the traffic policy is applied to the `CloudEndpoint`. When `endpoints`, it is applied to the `AgentEndpoint`.

See: [features/traffic-policy.md](features/traffic-policy.md)

### `ngrok.com/pooling-enabled`

Controls whether the endpoint allows pooling with other endpoints sharing the same URL.

| Detail          | Value                                                  |
|-----------------|--------------------------------------------------------|
| Applies to      | `Service` (LoadBalancer), `Ingress`, `Gateway` routes  |
| Allowed values  | `"true"`, `"false"`                                    |
| Default         | (none — uses ngrok platform default)                   |

### `ngrok.com/description`

Sets a human-readable description on the ngrok endpoint resource.

| Detail          | Value                                                  |
|-----------------|--------------------------------------------------------|
| Applies to      | `Ingress`, `Gateway`                                   |
| Default         | `"Created by the ngrok-operator"`                      |

Note: read from `Ingress` and `Gateway` objects only — LoadBalancer `Service` endpoints always use the operator default description.

### `ngrok.com/metadata`

Sets arbitrary key-value metadata on the ngrok endpoint resource. Value is a JSON object string that is parsed into `map[string]string`. Merged with operator-level ``ngrok.metadata``; annotation keys take precedence on conflict.

| Detail          | Value                                                  |
|-----------------|--------------------------------------------------------|
| Applies to      | `Ingress`, `Gateway`                                   |
| Default         | `{"owned-by": "ngrok-operator"}`                       |

Note: read from `Ingress` and `Gateway` objects only — LoadBalancer `Service` endpoints always use the operator default metadata.

### `ngrok.com/bindings`

Controls traffic visibility for an endpoint. Comma-separated list of binding types.

| Detail          | Value                                                  |
|-----------------|--------------------------------------------------------|
| Applies to      | `Service` (LoadBalancer), `Ingress`, `Gateway` routes  |
| Allowed values  | `public`, `internal`, `kubernetes`                     |
| Default         | (none — uses ngrok platform default)                   |

### `ngrok.com/app-protocols`

Maps upstream Service port names to the protocol the operator should use when proxying to that port. Read from the **backend Service** referenced by an Ingress rule or Gateway route — not from LoadBalancer Services the operator exposes directly.

| Detail          | Value                                                  |
|-----------------|--------------------------------------------------------|
| Applies to      | `Service` referenced as an Ingress / Gateway route backend |
| Value           | JSON object string mapping port name → protocol, e.g. `{"grpc-port":"HTTPS","raw-port":"TCP"}` |
| Allowed values  | `HTTP`, `HTTPS`, `TCP`, `TLS` (case-insensitive)       |
| Default         | (none — the route type's default protocol is used, `HTTP` for HTTP routes) |

An empty value is treated as unset. Invalid non-empty JSON logs an error and the backend falls back to its default protocol — translation still succeeds. A valid JSON map with an unrecognized protocol value likewise logs an error and falls back to the default protocol for that port.

See: [upstream-protocols.md](upstream-protocols.md) for how this interacts with the `appProtocol` field and default protocol selection.

## Internal Annotations (set by the operator)

### `ngrok.com/computed-url`

Set by the Service LoadBalancer controller as the single source of truth for the externally reachable URL. Users should not set this annotation.

| Detail          | Value                                                  |
|-----------------|--------------------------------------------------------|
| Set on          | `Service` (LoadBalancer)                               |
| Example         | `tcp://5.tcp.ngrok.io:12345`, `tls://example.com:443` |

See: [controllers/service.md](controllers/service.md)
