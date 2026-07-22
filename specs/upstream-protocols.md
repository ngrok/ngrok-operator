# Upstream Protocol Selection

## Overview

When the operator proxies traffic to a backend Service referenced by an Ingress rule or Gateway route, it must decide two things about the upstream connection: the **transport/scheme** (HTTP, HTTPS, TCP, TLS) and the **L7 application protocol** (HTTP/1 vs HTTP/2). Two independent user-supplied inputs control these, both read from the backend Service — not from LoadBalancer Services the operator exposes directly.

## Transport: `ngrok.com/app-protocols` annotation

A JSON object string on the backend Service mapping port *names* to the transport protocol used when connecting to that port.

```yaml
metadata:
  annotations:
    ngrok.com/app-protocols: '{"grpc-port":"HTTPS","raw-port":"TCP"}'
```

| Detail          | Value                                                  |
|-----------------|--------------------------------------------------------|
| Applies to      | `Service` referenced as an Ingress / Gateway route backend |
| Allowed values  | `HTTP`, `HTTPS`, `TCP`, `TLS` (case-insensitive)       |
| Default         | (none — the route type's default protocol is used, `HTTP` for HTTP routes) |

An empty value is treated as unset (the default protocol is used). Invalid non-empty JSON logs an error and the backend falls back to its default protocol — translation still succeeds. A valid JSON map with an unrecognized protocol value likewise logs an error and falls back to the default protocol for that port. See [annotations.md](annotations.md) for the full annotation reference.

## Application protocol: the `appProtocol` port field

The standard Kubernetes `Service.spec.ports[].appProtocol` field. The operator recognizes these values on backend Service ports:

| Value                | Meaning                          |
|----------------------|----------------------------------|
| `ngrok.com/http2`    | Upstream speaks HTTP/2           |
| `kubernetes.io/h2c`  | Upstream speaks HTTP/2 cleartext |
| `http`               | Upstream speaks HTTP/1           |

```yaml
spec:
  ports:
  - name: grpc-port
    port: 443
    appProtocol: ngrok.com/http2
```

| Detail          | Value                                                  |
|-----------------|--------------------------------------------------------|
| Applies to      | `Service` referenced as an Ingress / Gateway route backend |
| Default         | (unset — HTTP/1)                                       |

Unrecognized values are ignored (logged at debug level). Each port has exactly one `appProtocol` value.

## Migration note

During the `k8s.ngrok.com/` → `ngrok.com/` migration window the operator also reads the legacy `k8s.ngrok.com/app-protocols` annotation key (the `ngrok.com/` key wins if both are present) and the legacy `k8s.ngrok.com/http2` appProtocol value. Legacy support is removed in 1.0. Because backend Services are read in translation rather than reconciled directly, legacy use here surfaces in the operator logs, not as Kubernetes events. See [`docs/v1-migration-guide.md`](../docs/v1-migration-guide.md).
