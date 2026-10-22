# PrivateEndpoint

## Resource Identity

| Property    | Value                       |
|-------------|-----------------------------|
| Group       | `ngrok.com`                 |
| Version     | `v1`                        |
| Kind        | `PrivateEndpoint`           |
| Scope       | Namespaced                  |
| Categories  | `ngrok`                     |

## Spec

| Field      | Type   | Required | Validation                            |
|------------|--------|----------|---------------------------------------|
| `url`      | string | Yes      |                                       |
| `scheme`   | string | Yes      | Enum: `http`, `https`, `tls`, `tcp`   |
| `hostname` | string | Yes      |                                       |
| `port`     | int32  | Yes      | Minimum: 1, Maximum: 65535            |

The spec is a parsed copy of the endpoint's URL, written by the poller:

| Field      | Value                                                                       |
|------------|-----------------------------------------------------------------------------|
| `url`      | The endpoint URL exactly as the ngrok API returns it                        |
| `scheme`   | The URL scheme, lowercased                                                  |
| `hostname` | The URL hostname, lowercased, without a trailing dot                        |
| `port`     | The URL port, or the scheme default: `http` 80, `https` and `tls` 443. `tcp` has no default. |

Example:

```yaml
apiVersion: ngrok.com/v1
kind: PrivateEndpoint
metadata:
  name: bar.ngrok.direct-tcp-6379
  namespace: ngrok-operator
  labels:
    ngrok.com/managed-by: private-endpoint-poller
    ngrok.com/private-endpoint-host: <sha256(hostname)[:16]>
spec:
  url: tcp://bar.ngrok.direct:6379
  scheme: tcp
  hostname: bar.ngrok.direct
  port: 6379
status:
  clusterIP: 10.96.12.34
  forwarderPort: 20001
  conditions:
  - type: Ready
    status: "True"
    reason: Ready
```

## Status

| Field                | Type        | Description                                                               |
|----------------------|-------------|---------------------------------------------------------------------------|
| `observedGeneration` | int64       | Generation last reconciled by the controller                              |
| `clusterIP`          | string      | ClusterIP of the hostname's Service, which cluster DNS returns for `spec.hostname`. Empty when not Ready. |
| `forwarderPort`      | int32       | Forwarder container port serving this endpoint, unique in the cluster. Unset when not Ready. |
| `conditions`         | []Condition | MaxItems: 8. List map keyed by `type`.                                    |

Status is written only by the PrivateEndpoint controller. The forwarder serves an endpoint when both `clusterIP` and `forwarderPort` are set.

## Conditions

| Type    | Description                                                                       |
|---------|-----------------------------------------------------------------------------------|
| `Ready` | The hostname's Service exists with a ClusterIP and this endpoint has a forwarder port |

`Ready=True` means configured. It does not mean that a forwarder replica has bound the port, that cluster DNS is configured, or that the endpoint is reachable.

| Reason                | Status | Meaning                                                                             |
|-----------------------|--------|-------------------------------------------------------------------------------------|
| `Ready`               | True   | Service and forwarder port are in place                                             |
| `UnsupportedHostname` | False  | The hostname cannot be mapped to a Service name (see [Naming](#naming))             |
| `PortConflict`        | False  | Another PrivateEndpoint already serves this port on this hostname; the message names it |
| `PortsExhausted`      | False  | No free forwarder port in 20000–20999                                               |
| `ServiceConflict`     | False  | A Service with the hostname's Service name exists and is not managed for this hostname |
| `ServiceNotReady`     | False  | The Service has no ClusterIP yet                                                    |

## Labels

| Label                              | Value                          | Set on              |
|------------------------------------|--------------------------------|---------------------|
| `ngrok.com/managed-by`             | `private-endpoint-poller`      | CRs and Services    |
| `ngrok.com/private-endpoint-host`  | First 16 hex characters of the SHA-256 of the normalized hostname | CRs and Services |

The poller manages only CRs that carry the `managed-by` label. The controller groups CRs and Services by the host label.

## Naming

| Object  | Name                                    | Example                                                  |
|---------|-----------------------------------------|----------------------------------------------------------|
| CR      | `<hostname>-<scheme>-<port>`            | `foo.ngrok.direct-https-443`, `bar.ngrok.direct-tcp-6379` |
| Service | `<label>-ngrok-direct` for `<label>.ngrok.direct` | `foo-ngrok-direct`                              |

The CR name uses the normalized `spec.hostname`, `spec.scheme` and `spec.port`, with the scheme's default port filled in, so `https://foo.ngrok.direct` is `foo.ngrok.direct-https-443`. URLs that differ only in case or an explicit default port share a name. If that name is not a valid object name (a DNS-1123 subdomain of at most 253 characters), the CR is named `pe-` followed by the first 16 hex characters of its SHA-256 instead.

The Service name must be a valid DNS-1035 label, so only single-label hostnames are supported: `<label>` must start with a letter, contain only lowercase letters, digits and `-`, and be at most 50 characters. Other `*.ngrok.direct` hostnames are still mirrored as CRs but report `UnsupportedHostname`.

## Printer Columns

| Name      | Source                                              | Priority |
|-----------|-----------------------------------------------------|----------|
| URL       | `.spec.url`                                         | 0        |
| Scheme    | `.spec.scheme`                                      | 0        |
| ClusterIP | `.status.clusterIP`                                 | 0        |
| Ready     | `.status.conditions[?(@.type=='Ready')].status`     | 0        |
| Age       | `.metadata.creationTimestamp`                       | 0        |

## Annotations

The PrivateEndpoint CRD does not consume annotations.

## Notes

- PrivateEndpoint CRs are created by the operator's poller in the operator namespace, not by users. The poller restores any edit to the spec or to its labels on the next poll.
- A same-named CR without the `managed-by` label is never adopted or overwritten.
- No editor/viewer aggregation roles are shipped for PrivateEndpoint.
- See [features/private-endpoints.md](../features/private-endpoints.md) for the full feature overview.

- PrivateEndpoint CRs carry no finalizers, so deleting them, their namespace, or the CRD never waits on the operator. They are not part of the [drain](../features/draining.md).
