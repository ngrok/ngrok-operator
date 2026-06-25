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
| `clientCertificateRefs` | []K8sObjectRefOptionalNamespace   | No       |                                        |                                       |

### EndpointUpstream

| Field                  | Type                     | Required | Validation              |
|------------------------|--------------------------|----------|-------------------------|
| `url`                  | string                   | Yes      |                         |
| `protocol`             | ApplicationProtocol      | No       | Enum: `http1`, `http2`  |
| `proxyProtocolVersion` | ProxyProtocolVersion     | No       | Enum: `"1"`, `"2"`     |

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
