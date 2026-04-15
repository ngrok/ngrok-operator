# NgrokTrafficPolicy

## Resource Identity

| Property    | Value                    |
|-------------|--------------------------|
| Group       | `ngrok.k8s.ngrok.com`   |
| Version     | `v1alpha1`               |
| Kind        | `NgrokTrafficPolicy`     |
| Scope       | Namespaced               |

## Spec

| Field    | Type            | Required | Validation                                           |
|----------|-----------------|----------|------------------------------------------------------|
| `policy` | json.RawMessage | No       | Schemaless, PreserveUnknownFields, Type: object      |

The `policy` field contains the raw traffic policy JSON. The operator validates JSON syntax but does not enforce schema on the policy content — it is passed through to the ngrok API.

## Status

| Field    | Type            | Description             |
|----------|-----------------|-------------------------|
| `policy` | json.RawMessage | The resolved policy     |

## Printer Columns

None.

## Annotations

The NgrokTrafficPolicy CRD does not consume annotations.

## Notes

- This is a "pass-through" resource: the operator validates JSON syntax and warns on deprecated features but does not enforce the policy schema.
- Deprecated features that trigger warnings: legacy `directions` field, `enabled` field on rules.
- Changes to an NgrokTrafficPolicy trigger re-reconciliation of all endpoints that reference it.
- See [features/traffic-policy.md](../features/traffic-policy.md) for how traffic policies are resolved across the system.
