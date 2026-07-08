# AgentEndpoint

## Resource Identity

| Property    | Value                    |
|-------------|--------------------------|
| Group       | `ngrok.com`              |
| Version     | `v1`                     |
| Kind        | `AgentEndpoint`          |
| Short Name  | `aep`                    |
| Categories  | `networking`, `ngrok`    |
| Scope       | Namespaced               |

## Spec

| Field                   | Type                              | Required | Default                                | Validation                            |
|-------------------------|-----------------------------------|----------|----------------------------------------|---------------------------------------|
| `url`                   | string                            | Yes      |                                        |                                       |
| `upstream`              | EndpointUpstream                  | Yes      |                                        |                                       |
| `trafficPolicy`         | TrafficPolicyCfg                  | No       |                                        | XValidation: exactly one of `inline` or `targetRef` |
| `description`           | string                            | No       | `"Created by the ngrok-operator"`      |                                       |
| `metadata`              | map[string]string                 | No       | `{"owned-by": "ngrok-operator"}`      |                                       |
| `bindings`              | []string                          | No       |                                        | MaxItems: 1, Pattern: `^(public\|internal\|kubernetes)$` |
| `clientCertificateRefs` | []K8sObjectRef                    | No       |                                        | Client certificates presented to the upstream during the TLS handshake |
| `tlsTermination`        | EndpointTLSTermination            | No       |                                        | XValidation: `spec.url` must be a `tls://` URL when set |

### EndpointUpstream

| Field                  | Type                     | Required | Validation              |
|------------------------|--------------------------|----------|-------------------------|
| `url`                  | string                   | Yes      |                         |
| `protocol`             | ApplicationProtocol      | No       | Enum: `http1`, `http2`  |
| `proxyProtocolVersion` | ProxyProtocolVersion     | No       | Enum: `"1"`, `"2"`     |

### EndpointTLSTermination

Configures agent-side ("zero-knowledge") TLS termination. When set, the ngrok edge routes the encrypted stream to the in-cluster agent based on SNI; the TLS handshake completes between the client and the agent, so ngrok never sees the plaintext. The agent then forwards to the upstream defined by `spec.upstream`.

Requires `spec.url` to use the `tls://` scheme (enforced by XValidation). Cannot be combined with the edge-side `terminate-tls` traffic-policy action, which terminates before traffic reaches the agent.

| Field                  | Type              | Required | Description |
|------------------------|-------------------|----------|-------------|
| `serverCertificateRef` | K8sObjectRef      | Yes      | Reference to a `kubernetes.io/tls` Secret containing the server certificate (`tls.crt`) and key (`tls.key`) the agent presents to clients. Must be in the same namespace as the AgentEndpoint. |
| `mutualTLS`            | EndpointMutualTLS | No       | When set, enables mTLS — the agent requires or requests client certificates during the handshake and validates them against the supplied CA bundle. |

### EndpointMutualTLS

| Field          | Type         | Required | Default   | Validation                | Description |
|----------------|--------------|----------|-----------|---------------------------|-------------|
| `clientCAsRef` | K8sObjectRef | Yes      |           |                           | Reference to a Secret whose `ca.crt` key holds a PEM-encoded CA bundle trusted to sign client certificates. Must be in the same namespace as the AgentEndpoint. |
| `mode`         | string       | No       | `require` | Enum: `require`, `request` | `require` rejects the handshake if no valid client cert is presented; `request` requests a client cert but does not require one. |

## Status

| Field                    | Type                            | Description                              |
|--------------------------|---------------------------------|------------------------------------------|
| `assignedURL`            | string                          | The URL assigned by ngrok                |
| `attachedTrafficPolicy`  | string                          | `"none"`, `"inline"`, or policy ref name |
| `domainRef`              | *K8sObjectRefOptionalNamespace  | Reference to the associated Domain CR    |
| `conditions`             | []Condition                     | MaxItems: 8                              |

## Conditions

| Type               | Description                                      |
|--------------------|--------------------------------------------------|
| `EndpointCreated`  | Whether the ngrok agent endpoint was created      |
| `TrafficPolicy`    | Whether the traffic policy was applied            |
| `DomainReady`      | Whether the associated Domain is ready            |
| `Ready`            | Overall readiness (aggregates other conditions)   |

## Printer Columns

| Name          | Source                                                        | Priority |
|---------------|---------------------------------------------------------------|----------|
| URL           | `.spec.url`                                                   | 0        |
| Upstream URL  | `.spec.upstream.url`                                          | 0        |
| Bindings      | `.spec.bindings`                                              | 0        |
| Ready         | `.status.conditions[?(@.type=='Ready')].status`               | 0        |
| Age           | `.metadata.creationTimestamp`                                 | 0        |
| Reason        | `.status.conditions[?(@.type=='Ready')].reason`               | 1        |
| Message       | `.status.conditions[?(@.type=='Ready')].message`              | 1        |

## Annotations

The AgentEndpoint CRD does not directly consume annotations. However, when an AgentEndpoint is created by a parent controller (Service, Ingress, Gateway), the following annotations on the parent resource influence the created AgentEndpoint:

- `ngrok.com/description` — Sets `spec.description`
- `ngrok.com/metadata` — Sets `spec.metadata`
- `ngrok.com/traffic-policy` — Sets `spec.trafficPolicy.targetRef`
- `ngrok.com/bindings` — Sets `spec.bindings`

See [annotations.md](../annotations.md) for the full reference.
