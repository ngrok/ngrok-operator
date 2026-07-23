# IPPolicy

## Resource Identity

| Property    | Value                      |
|-------------|----------------------------|
| Group       | `ngrok.com`                |
| Version     | `v1`                       |
| Kind        | `IPPolicy`                 |
| Scope       | Namespaced                 |

## Spec

| Field         | Type           | Required | Default                                              |
|---------------|----------------|----------|------------------------------------------------------|
| `description` | string         | No       | `"Created by the ngrok-operator"`                     |
| `metadata`    | map[string]string | No    | `{"owned-by": "ngrok-operator"}`                      |
| `rules`       | []IPPolicyRule | No       |                                                      |

### IPPolicyRule

| Field         | Type   | Required | Default                                              | Validation           |
|---------------|--------|----------|------------------------------------------------------|----------------------|
| `description` | string | No       | `"Created by the ngrok-operator"`                     | MaxLength: 255       |
| `metadata`    | map[string]string | No | `{"owned-by": "ngrok-operator"}`                 | MaxLength: 4096      |
| `cidr`        | string | Yes      |                                                      | Pattern: IPv4 or IPv6 CIDR notation (prefix length required) |
| `action`      | string | Yes      |                                                      | Enum: `allow`, `deny`|

## Status

| Field                | Type                 | Description                                  |
|----------------------|----------------------|----------------------------------------------|
| `observedGeneration` | int64                | Generation last reconciled by the controller |
| `id`                 | string               | ngrok IP policy ID         |
| `conditions` | []Condition          | MaxItems: 8                |

## Conditions

| Type                     | Description                               |
|--------------------------|-------------------------------------------|
| `IPPolicyCreated`        | Whether the ngrok IP policy was created   |
| `IPPolicyRulesConfigured`| Whether all rules were configured         |
| `Ready`                  | Overall readiness                         |

## Printer Columns

| Name    | Source                                                        | Priority |
|---------|---------------------------------------------------------------|----------|
| ID      | `.status.id`                                                  | 0        |
| Ready   | `.status.conditions[?(@.type=='Ready')].status`               | 0        |
| Age     | `.metadata.creationTimestamp`                                 | 0        |
| Reason  | `.status.conditions[?(@.type=='Ready')].reason`               | 1        |
| Message | `.status.conditions[?(@.type=='Ready')].message`              | 1        |

## Annotations

The IPPolicy CRD does not consume user-facing annotations.
