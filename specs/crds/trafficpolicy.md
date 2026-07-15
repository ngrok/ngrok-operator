# TrafficPolicy

## Resource Identity

| Property    | Value                    |
|-------------|--------------------------|
| Group       | `ngrok.com`              |
| Version     | `v1`                     |
| Kind        | `TrafficPolicy`     |
| Scope       | Namespaced               |

## Spec

| Field    | Type            | Required | Validation                                           |
|----------|-----------------|----------|------------------------------------------------------|
| `policy` | json.RawMessage | No       | Schemaless, PreserveUnknownFields, Type: object      |

The `policy` field contains the raw traffic policy JSON. The operator validates JSON syntax but does not enforce schema on the policy content — it is passed through to the ngrok API. The field is intentionally schemaless: the traffic policy language is defined and versioned by the ngrok API, so enforcing a schema here would break whenever new phases, actions, or fields ship server-side.

## Status

| Field        | Type        | Description   |
|--------------|-------------|---------------|
| `conditions` | []Condition | MaxItems: 8   |

TrafficPolicy is not reconciled against the ngrok API directly; conditions reflect local parse/validation of `spec.policy` only. The legacy `status.policy` field (a mirror of `spec.policy`) was removed — `observedGeneration` on conditions is the "what did the controller see" signal.

## Conditions

| Type    | Description                                              |
|---------|----------------------------------------------------------|
| `Ready` | Overall readiness; False when `spec.policy` fails to parse |
| `Valid` | Parse/validation result of `spec.policy`                 |

Both conditions share the same reason so deprecation warnings surface in the Ready-based printer columns. Reasons: `TrafficPolicyValid`, `TrafficPolicyParseFailed`, `LegacyPolicyFormat`, `EnabledFieldDeprecated`.

## Printer Columns

| Name   | Source                                            | Priority |
|--------|---------------------------------------------------|----------|
| Ready  | `.status.conditions[?(@.type=='Ready')].status`   | 0        |
| Reason | `.status.conditions[?(@.type=='Ready')].reason`   | 1        |
| Age    | `.metadata.creationTimestamp`                     | 0        |

## Annotations

The TrafficPolicy CRD does not consume annotations.

## Notes

- This is a "pass-through" resource: the operator validates JSON syntax and warns on deprecated features but does not enforce the policy schema.
- Deprecated features that trigger warnings: legacy `directions` field, `enabled` field on rules. These surface both as Events and as condition reasons.
- Changes to a TrafficPolicy trigger re-reconciliation of all endpoints that reference it.
- See [features/traffic-policy.md](../features/traffic-policy.md) for how traffic policies are resolved across the system.
