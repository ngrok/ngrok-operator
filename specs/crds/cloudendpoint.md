# CloudEndpoint

## Resource Identity

The deprecated `ngrok.k8s.ngrok.com/v1alpha1` `CloudEndpoint` is still served during the migration window. Its schema is the same except `spec.metadata`, which also accepts a raw JSON string. See [crd-group-migration.md](../crd-group-migration.md).

| Property    | Value                    |
|-------------|--------------------------|
| Group       | `ngrok.com`              |
| Version     | `v1`                     |
| Kind        | `CloudEndpoint`          |
| Short Name  | `clep`                   |
| Categories  | `networking`, `ngrok`    |
| Scope       | Namespaced               |

## Spec

| Field               | Type                      | Required | Default                                | Validation                            |
|---------------------|---------------------------|----------|----------------------------------------|---------------------------------------|
| `url`               | string                    | Yes      |                                        |                                       |
| `trafficPolicy`     | TrafficPolicyCfg          | No       |                                        | XValidation: exactly one of `inline` or `targetRef` |
| `poolingEnabled`    | *bool                     | No       |                                        |                                       |
| `description`       | string                    | No       | `"Created by the ngrok-operator"`      |                                       |
| `metadata`          | map[string]string         | No       | `{"owned-by": "ngrok-operator"}`      |                                       |
| `bindings`          | []string                  | No       |                                        | MaxItems: 1, Pattern: `^(public\|internal\|kubernetes)$` |

## Status

| Field                    | Type                            | Description                              |
|--------------------------|---------------------------------|------------------------------------------|
| `observedGeneration`     | int64                           | Generation last reconciled by the controller |
| `id`                     | string                          | The ngrok API resource ID                |
| `assignedURL`            | string                          | The URL assigned by ngrok                |
| `attachedTrafficPolicy`  | string                          | `"none"`, `"inline"`, or policy ref name |
| `domainRef`              | *K8sObjectRefOptionalNamespace  | Reference to the associated Domain CR    |
| `conditions`             | []Condition                     | MaxItems: 8                              |

## Conditions

| Type    | Description                                    |
|---------|------------------------------------------------|
| `Ready` | Overall readiness of the cloud endpoint        |

## Printer Columns

| Name           | Source                                                        | Priority |
|----------------|---------------------------------------------------------------|----------|
| ID             | `.status.id`                                                  | 0        |
| URL            | `.spec.url`                                                   | 0        |
| Bindings       | `.spec.bindings`                                              | 0        |
| Age            | `.metadata.creationTimestamp`                                 | 0        |
| Ready          | `.status.conditions[?(@.type=='Ready')].status`               | 0        |
| Reason         | `.status.conditions[?(@.type=='Ready')].reason`               | 1        |
| Message        | `.status.conditions[?(@.type=='Ready')].message`              | 1        |

## Deprecated kind

`ngrok.k8s.ngrok.com/v1alpha1` `CloudEndpoint` is served during the migration window. Both kinds use the plural `cloudendpoints` and the short name `clep`. The legacy CRD is marked deprecated, so applying it prints a warning, and a legacy object whose v1 twin manages the endpoint is `Ready` with reason `SupersededByV1`. The v1 CRD carries `helm.sh/resource-policy: keep`. See [the CloudEndpoint controller](../controllers/cloudendpoint.md#migration-from-ngrokk8sngrokcom) for how the two kinds share one ngrok endpoint.

## Annotations

The CloudEndpoint CRD does not directly consume annotations. When created by a parent controller, the following parent annotations influence the CloudEndpoint:

- `ngrok.com/description` — Sets `spec.description`
- `ngrok.com/metadata` — Sets `spec.metadata`
- `ngrok.com/traffic-policy` — Sets `spec.trafficPolicy.targetRef`
- `ngrok.com/pooling-enabled` — Sets `spec.poolingEnabled`
- `ngrok.com/bindings` — Sets `spec.bindings`

See [annotations.md](../annotations.md) for the full reference.
