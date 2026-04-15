# KubernetesOperator

## Resource Identity

| Property    | Value                    |
|-------------|--------------------------|
| Group       | `ngrok.com`              |
| Version     | `v1`                     |
| Kind        | `KubernetesOperator`     |
| Scope       | Namespaced               |

## Spec

| Field             | Type                          | Required | Default                                | Validation                               |
|-------------------|-------------------------------|----------|----------------------------------------|------------------------------------------|
| `description`     | string                        | No       | `"Created by the ngrok-operator"`      |                                          |
| `metadata`        | map[string]string             | No       | `{"owned-by": "ngrok-operator"}`      |                                          |
| `enabledFeatures` | []string                      | No       |                                        | Items enum: `ingress`, `gateway`, `bindings` |
| `region`          | string                        | No       | `"global"`                             |                                          |
| `deployment`      | KubernetesOperatorDeployment  | No       |                                        |                                          |
| `binding`         | KubernetesOperatorBinding     | No       |                                        |                                          |
| `drain`           | DrainConfig                   | No       |                                        |                                          |

### KubernetesOperatorDeployment

| Field       | Type   | Description                  |
|-------------|--------|------------------------------|
| `name`      | string | Helm release name            |
| `namespace` | string | Operator namespace           |
| `version`   | string | Operator version             |

### KubernetesOperatorBinding

| Field               | Type     | Required | Default        | Description                           |
|---------------------|----------|----------|----------------|---------------------------------------|
| `endpointSelectors` | []string | Yes      |                | CEL expressions filtering endpoints   |
| `ingressEndpoint`   | *string  | No       |                | Bindings ingress endpoint hostname    |
| `tlsSecretName`     | string   | Yes      | `"default-tls"`| Secret name for mTLS certificate      |

### DrainConfig

| Field    | Type        | Default    | Validation                |
|----------|-------------|------------|---------------------------|
| `policy` | DrainPolicy | `"Retain"` | Enum: `Delete`, `Retain`  |

## Status

| Field                      | Type        | Description                                         |
|----------------------------|-------------|-----------------------------------------------------|
| `id`                       | string      | ngrok API resource ID                               |
| `uri`                      | string      | ngrok API resource URI                              |
| `registrationStatus`       | string      | Enum: `registered`, `error`, `pending` (default: `pending`) |
| `registrationErrorCode`    | string      | Pattern: `^ERR_NGROK_\d+$`                         |
| `registrationErrorMessage` | string      | MaxLength: 4096                                     |
| `enabledFeatures`          | string      | Comma-separated list of enabled features            |
| `bindingsIngressEndpoint`  | string      | Resolved bindings ingress endpoint                  |
| `drainStatus`              | DrainStatus | Enum: `pending`, `draining`, `completed`, `failed`  |
| `drainMessage`             | string      | Human-readable drain progress message               |
| `drainProgress`            | string      | Drain progress indicator                            |
| `drainErrors`              | []string    | Errors encountered during drain                     |

## Printer Columns

| Name                      | Source                                  | Priority |
|---------------------------|-----------------------------------------|----------|
| ID                        | `.status.id`                            | 0        |
| Status                    | `.status.registrationStatus`            | 0        |
| Enabled Features          | `.status.enabledFeatures`               | 0        |
| Endpoint Selectors        | `.spec.binding.endpointSelectors`       | 0        |
| Binding Ingress Endpoint  | `.spec.binding.ingressEndpoint`         | 2        |
| Age                       | `.metadata.creationTimestamp`           | 0        |

## Annotations

The KubernetesOperator CRD does not consume user-facing annotations.

## Notes

- There is typically one KubernetesOperator CR per operator deployment. The controller uses a namespace+name predicate to only reconcile its own CR.
- Deletion of this resource triggers the drain workflow. See [features/draining.md](../features/draining.md).
- The `deployment` field is populated automatically by the operator with its own Helm release name, namespace, and version.
