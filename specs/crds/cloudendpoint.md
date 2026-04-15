# CloudEndpoint

## Resource Identity

| Property    | Value                    |
|-------------|--------------------------|
| Group       | `ngrok.k8s.ngrok.com`   |
| Version     | `v1alpha1`               |
| Kind        | `CloudEndpoint`          |
| Short Name  | `clep`                   |
| Categories  | `networking`, `ngrok`    |
| Scope       | Namespaced               |

## Spec

| Field               | Type                      | Required | Default                                | Validation                            |
|---------------------|---------------------------|----------|----------------------------------------|---------------------------------------|
| `url`               | string                    | Yes      |                                        |                                       |
| `trafficPolicyName` | string                    | No       |                                        | Name of NgrokTrafficPolicy in same namespace |
| `poolingEnabled`    | *bool                     | No       |                                        |                                       |
| `trafficPolicy`     | *NgrokTrafficPolicySpec   | No       |                                        | Inline traffic policy object          |
| `description`       | string                    | No       | `"Created by the ngrok-operator"`      |                                       |
| `metadata`          | string                    | No       | `"{\"owned-by\":\"ngrok-operator\"}"` |                                       |
| `bindings`          | []string                  | No       |                                        | MaxItems: 1, Pattern: `^(public\|internal\|kubernetes)$` |

## Status

| Field        | Type                            | Description                            |
|--------------|---------------------------------|----------------------------------------|
| `id`         | string                          | The ngrok API resource ID              |
| `domainRef`  | *K8sObjectRefOptionalNamespace  | Reference to the associated Domain CR  |
| `conditions` | []Condition                     | MaxItems: 8                            |

## Conditions

| Type    | Description                                    |
|---------|------------------------------------------------|
| `Ready` | Overall readiness of the cloud endpoint        |

## Printer Columns

| Name           | Source                                                        | Priority |
|----------------|---------------------------------------------------------------|----------|
| ID             | `.status.id`                                                  | 0        |
| URL            | `.spec.url`                                                   | 0        |
| Traffic Policy | `.spec.trafficPolicyName`                                     | 0        |
| Bindings       | `.spec.bindings`                                              | 0        |
| Age            | `.metadata.creationTimestamp`                                 | 0        |
| Ready          | `.status.conditions[?(@.type=='Ready')].status`               | 0        |
| Reason         | `.status.conditions[?(@.type=='Ready')].reason`               | 1        |
| Message        | `.status.conditions[?(@.type=='Ready')].message`              | 1        |

## Annotations

The CloudEndpoint CRD does not directly consume annotations. When created by a parent controller, the following parent annotations influence the CloudEndpoint:

- `k8s.ngrok.com/description` — Sets `spec.description`
- `k8s.ngrok.com/metadata` — Sets `spec.metadata`
- `k8s.ngrok.com/traffic-policy` — Sets `spec.trafficPolicyName`
- `k8s.ngrok.com/pooling-enabled` — Sets `spec.poolingEnabled`
- `k8s.ngrok.com/bindings` — Sets `spec.bindings`

See [annotations.md](../annotations.md) for the full reference.
