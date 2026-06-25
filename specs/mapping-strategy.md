# Mapping Strategy

## Overview

The `ngrok.com/mapping-strategy` annotation controls which ngrok endpoint resources the operator creates for a given Kubernetes resource (Service, Ingress, or Gateway). The strategy determines the endpoint topology and where traffic policy is applied.

## Strategies

### `endpoints` (default)

Creates a single `AgentEndpoint`. The endpoint URL is the public URL ngrok assigns (or the one specified via `ngrok.com/url`).

```
Internet → ngrok cloud → AgentEndpoint (public URL)
                              ↓
                         upstream Service
```

Traffic policy (if any) is applied to the `AgentEndpoint`.

**Use when**: You want a simple, single-hop setup. The ngrok agent handles all traffic and terminates it in-cluster.

### `endpoints-verbose`

Creates two resources:
- A `CloudEndpoint` with the public URL.
- An `AgentEndpoint` with a `.internal` URL (e.g., `https://my-service.my-namespace.internal`).

The `CloudEndpoint` forwards traffic to the `AgentEndpoint` via the `forward_internal` traffic policy action injected by the operator. See [features/traffic-policy.md](features/traffic-policy.md#internal-forwarding) for details on this injection.

```
Internet → ngrok cloud → CloudEndpoint (public URL)
                              ↓ forward_internal
                         AgentEndpoint (.internal URL)
                              ↓
                         upstream Service
```

Traffic policy (if any) is applied to the `CloudEndpoint`.

**Use when**: You need to configure traffic policy at the cloud layer (rate limiting, auth, header manipulation) while keeping the agent endpoint private. Also required for endpoint pooling across multiple clusters.

## Where Traffic Policy is Applied

| Mapping Strategy     | Traffic Policy Applied To |
|----------------------|---------------------------|
| `endpoints`          | `AgentEndpoint`           |
| `endpoints-verbose`  | `CloudEndpoint`           |

## Internal URL Format

For `endpoints-verbose`, the `.internal` URL is derived from the Kubernetes resource:

| Source resource | Internal URL pattern |
|-----------------|----------------------|
| `Service` (name `svc`, namespace `ns`) | `https://svc.ns.internal` |
| `Ingress` (host `example.com`) | derived from Ingress host |
| `Gateway` | derived from Gateway listener |

The `.internal` suffix signals to ngrok that the endpoint is only reachable from within the ngrok network (not publicly accessible on the internet).

## Examples

### Service with default strategy

```yaml
apiVersion: v1
kind: Service
metadata:
  name: my-app
  annotations:
    # mapping-strategy defaults to "endpoints"
spec:
  type: LoadBalancer
```

Creates: one `AgentEndpoint` with an ngrok-assigned TCP/TLS URL.

### Service with verbose strategy and traffic policy

```yaml
apiVersion: v1
kind: Service
metadata:
  name: my-app
  annotations:
    ngrok.com/mapping-strategy: "endpoints-verbose"
    ngrok.com/traffic-policy: "my-rate-limit-policy"
spec:
  type: LoadBalancer
```

Creates:
- `CloudEndpoint` with public URL, `my-rate-limit-policy` applied, forwarding internally.
- `AgentEndpoint` with `https://my-app.<namespace>.internal`, no user-supplied traffic policy.
