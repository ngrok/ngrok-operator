# CRDs — Common Patterns

## API Group and Version

All ngrok-operator CRDs belong to a single API group `ngrok.com` at version `v1`:

| API Group   | Version | CRDs                                                                             |
|-------------|---------|----------------------------------------------------------------------------------|
| `ngrok.com` | `v1`    | AgentEndpoint, CloudEndpoint, KubernetesOperator, TrafficPolicy, Domain, IPPolicy, BoundEndpoint |

### Migration from Previous Versions

Previous releases used three separate API groups (`ngrok.k8s.ngrok.com`, `ingress.k8s.ngrok.com`, `bindings.k8s.ngrok.com`) at version `v1alpha1`. A conversion webhook must be provided to handle conversion from the previous group/version combinations to `ngrok.com/v1`.

## Scope

All CRDs are **Namespaced**.

## Field Naming

All JSON field names use **camelCase** (e.g., `trafficPolicy`, `resolvesTo`, `poolingEnabled`). No other conventions (snake_case, PascalCase) are permitted.

## Serialization (`omitempty`)

All fields must follow the Kubernetes API conventions for `omitempty`:

- **Optional fields** use `omitempty`. Pointer types, slices, and maps must always include `omitempty` to avoid serializing `null` or zero values.
- **Required fields** do not use `omitempty`. The field is always present in the serialized form.
- **Bools** that distinguish between unset and `false` must be pointer types with `omitempty`.

## Status Conditions

All CRDs use `[]metav1.Condition` for status reporting with the following constraints:

- **MaxItems:** 8
- **List type:** map (keyed by `type`)

The common condition type is `Ready`, which summarizes the overall health of the resource. Individual controllers may define additional condition types specific to their resource.

Condition type constants must be defined in the API package, not in internal controller code. The API package is the public contract — condition types are part of that contract since users depend on them for `kubectl wait`, health checks, and GitOps tooling.

The API package must only contain API types and constants. Internal implementation details (controller interfaces, reconciliation helpers, etc.) must not be defined in the API package.

## Finalizers

The operator adds the finalizer `ngrok.com/finalizer` to resources it manages. This ensures that:

1. The operator can clean up remote ngrok API resources before the Kubernetes resource is deleted.
2. During drain, finalizers are removed to allow Kubernetes garbage collection.

## Owner References

Controllers set owner references on child resources they create. For example, the Service LoadBalancer controller sets an owner reference on created AgentEndpoint/CloudEndpoint resources pointing back to the parent Service.

## Default Field Values

Most CRDs that correspond to ngrok API resources include:

| Field         | Default Value                         |
|---------------|---------------------------------------|
| `description` | `"Created by the ngrok-operator"`     |
| `metadata`    | `{"owned-by": "ngrok-operator"}`      |

## Shared Types

### K8sObjectRef

A reference to a Kubernetes object in the same namespace.

| Field  | Type   | Required |
|--------|--------|----------|
| `name` | string | Yes      |

### K8sObjectRefOptionalNamespace

A reference to a Kubernetes object, optionally in a different namespace.

| Field       | Type    | Required |
|-------------|---------|----------|
| `name`      | string  | Yes      |
| `namespace` | *string | No       |

### TrafficPolicyCfg

Configures a traffic policy via either an inline definition or a reference to a TrafficPolicy resource. Exactly one of `inline` or `targetRef` must be specified (enforced via XValidation rules).

| Field       | Type            | Required | Description                              |
|-------------|-----------------|----------|------------------------------------------|
| `inline`    | json.RawMessage | No       | Inline traffic policy JSON (schemaless)  |
| `targetRef` | K8sObjectRefOptionalNamespace | No | Reference to a TrafficPolicy |

### ApplicationProtocol

Enum: `http1`, `http2`

### ProxyProtocolVersion

Enum: `"1"`, `"2"`
