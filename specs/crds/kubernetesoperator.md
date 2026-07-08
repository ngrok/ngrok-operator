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

| Field                      | Type                          | Description                                         |
|----------------------------|-------------------------------|-----------------------------------------------------|
| `observedGeneration`       | int64                         | Generation last reconciled by the controller        |
| `id`                       | string                        | ngrok API resource ID                               |
| `uri`                      | string                        | ngrok API resource URI                              |
| `conditions`               | []Condition                   | MaxItems: 8                                         |
| `enabledFeatures`          | []string                      | Enabled features reported by the ngrok API          |
| `bindingsIngressEndpoint`  | string                        | Resolved bindings ingress endpoint                  |
| `drain`                    | *KubernetesOperatorDrainStatus | Drain progress, present only once deletion starts  |

### KubernetesOperatorDrainStatus

| Field              | Type     | Description                                        |
|--------------------|----------|----------------------------------------------------|
| `drainedResources` | int      | Resources processed so far, including failures     |
| `totalResources`   | int      | Total resources the drain will process             |
| `errors`           | []string | Most recent errors encountered during drain        |

## Conditions

| Type         | Description                                                                 |
|--------------|-----------------------------------------------------------------------------|
| `Ready`      | Overall readiness. `False` with reason `Draining`/`DrainFailed`/`DrainCompleted` during deletion |
| `Registered` | Whether this operator is registered with the ngrok API. Failure reasons use the `ERR_NGROK_*` error code when available, otherwise `RegistrationFailed`; `Pending` before registration |
| `Draining`   | Present only during deletion. `True` while the drain is in progress or retrying after errors (reason `DrainInProgress`/`DrainFailed`), `False` with reason `DrainCompleted` once finished |

## Printer Columns

| Name                      | Source                                            | Priority |
|---------------------------|---------------------------------------------------|----------|
| ID                        | `.status.id`                                      | 0        |
| Ready                     | `.status.conditions[?(@.type=='Ready')].status`   | 0        |
| Enabled Features          | `.status.enabledFeatures`                         | 0        |
| Endpoint Selectors        | `.spec.binding.endpointSelectors`                 | 0        |
| Binding Ingress Endpoint  | `.spec.binding.ingressEndpoint`                   | 2        |
| Age                       | `.metadata.creationTimestamp`                     | 0        |
| Reason                    | `.status.conditions[?(@.type=='Ready')].reason`   | 1        |
| Drained                   | `.status.drain.drainedResources`                  | 1        |
| Drain Total               | `.status.drain.totalResources`                    | 1        |

## Annotations

The KubernetesOperator CRD does not consume user-facing annotations.

## Notes

- There is typically one KubernetesOperator CR per operator deployment. The controller uses a namespace+name predicate to only reconcile its own CR.
- Deletion of this resource triggers the drain workflow. See [features/draining.md](../features/draining.md).
- The `deployment` field is populated automatically by the operator with its own Helm release name, namespace, and version.
