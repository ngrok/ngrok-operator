# Annotations Reference

## Overview

The ngrok-operator uses annotations under the `ngrok.com/` prefix to configure behavior on Kubernetes resources. This document serves as a central reference. See individual CRD specs in [crds/](crds/) for full details on how each annotation is used.

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
| Applies to      | `Service` (LoadBalancer), `Ingress`, `Gateway` routes  |
| Default         | `"Created by the ngrok-operator"`                      |

### `ngrok.com/metadata`

Sets arbitrary key-value metadata on the ngrok endpoint resource. Value is a JSON object string that is parsed into `map[string]string`. Merged with operator-level ``ngrok.metadata``; annotation keys take precedence on conflict.

| Detail          | Value                                                  |
|-----------------|--------------------------------------------------------|
| Applies to      | `Service` (LoadBalancer), `Ingress`, `Gateway` routes  |
| Default         | `{"owned-by": "ngrok-operator"}`                       |

### `ngrok.com/bindings`

Controls traffic visibility for an endpoint. Comma-separated list of binding types.

| Detail          | Value                                                  |
|-----------------|--------------------------------------------------------|
| Applies to      | `Service` (LoadBalancer), `Ingress`, `Gateway` routes  |
| Allowed values  | `public`, `internal`, `kubernetes`                     |
| Default         | (none — uses ngrok platform default)                   |

## Internal Annotations (set by the operator)

### `ngrok.com/computed-url`

Set by the Service LoadBalancer controller as the single source of truth for the externally reachable URL. Users should not set this annotation.

| Detail          | Value                                                  |
|-----------------|--------------------------------------------------------|
| Set on          | `Service` (LoadBalancer)                               |
| Example         | `tcp://5.tcp.ngrok.io:12345`, `tls://example.com:443` |

See: [controllers/service.md](controllers/service.md)
