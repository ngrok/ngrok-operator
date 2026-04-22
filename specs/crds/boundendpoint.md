# BoundEndpoint

## Resource Identity

| Property    | Value                       |
|-------------|-----------------------------|
| Group       | `ngrok.com`                 |
| Version     | `v1`                        |
| Kind        | `BoundEndpoint`             |
| Scope       | Namespaced                  |

## Spec

| Field          | Type           | Required | Default    | Validation                                             |
|----------------|----------------|----------|------------|--------------------------------------------------------|
| `endpointURL`  | string         | No       |            | Pattern: `^((?P<scheme>(tcp\|http\|https\|tls)?)://)?(?P<service>[a-z][a-zA-Z0-9-]{0,62})\.(?P<namespace>[a-z][a-zA-Z0-9-]{0,62})(:(?P<port>\d+))?$` |
| `scheme`       | string         | Yes      | `"https"`  | Enum: `tcp`, `http`, `https`, `tls`                    |
| `port`         | uint16         | Yes      |            |                                                        |
| `target`       | EndpointTarget | Yes      |            |                                                        |

### EndpointTarget

| Field       | Type           | Required | Default  | Validation      |
|-------------|----------------|----------|----------|-----------------|
| `service`   | string         | Yes      |          |                 |
| `namespace` | string         | Yes      |          |                 |
| `protocol`  | string         | Yes      | `"TCP"`  | Enum: `TCP`     |
| `port`      | int32          | Yes      |          |                 |
| `metadata`  | TargetMetadata | No       |          |                 |

### TargetMetadata

| Field         | Type              |
|---------------|-------------------|
| `labels`      | map[string]string |
| `annotations` | map[string]string |

## Status

| Field                | Type                            | Description                                  |
|----------------------|---------------------------------|----------------------------------------------|
| `endpoints`          | []BindingEndpoint               | ngrok endpoint references (id, uri)          |
| `hashedName`         | string                          | Hashed name for the bound endpoint           |
| `endpointsSummary`   | string                          | Human-readable summary of endpoints          |
| `conditions`         | []Condition                     | MaxItems: 8                                  |
| `targetServiceRef`   | *K8sObjectRefOptionalNamespace  | Reference to the created target service      |
| `upstreamServiceRef` | *K8sObjectRef                   | Reference to the created upstream service    |

## Conditions

| Type                   | Description                                       |
|------------------------|---------------------------------------------------|
| `ServicesCreated`      | Whether both target and upstream services exist   |
| `ConnectivityVerified` | Whether TCP connectivity through services works   |
| `Ready`                | Overall readiness                                 |

## Printer Columns

| Name      | Source                                                                  | Priority |
|-----------|-------------------------------------------------------------------------|----------|
| URL       | `.spec.endpointURL`                                                     | 0        |
| Port      | `.spec.port`                                                            | 0        |
| Endpoints | `.status.endpointsSummary`                                              | 0        |
| Services  | `.status.conditions[?(@.type=="ServicesCreated")].status`               | 0        |
| Ready     | `.status.conditions[?(@.type=="Ready")].status`                         | 0        |
| Age       | `.metadata.creationTimestamp`                                           | 0        |
| Reason    | `.status.conditions[?(@.type=="Ready")].reason`                         | 1        |
| Message   | `.status.conditions[?(@.type=="Ready")].message`                        | 1        |

## Annotations

The BoundEndpoint CRD does not consume user-facing annotations.

## Notes

- BoundEndpoint CRs are created by the operator's poller (not by users directly) based on endpoint bindings received from the ngrok API.
- The `endpoints`, `endpointsSummary`, and `hashedName` status fields are managed by the poller, not the BoundEndpoint controller.
- The controller creates two Services per BoundEndpoint: a target ExternalName service and an upstream ClusterIP service.
- See [features/bindings.md](../features/bindings.md) for the full bindings feature overview.
