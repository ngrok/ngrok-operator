# CRDs — Common Patterns

## API Versions

All ngrok-operator CRDs use API version `v1alpha1` across three API groups:

| API Group                  | CRDs                                                        |
|----------------------------|-------------------------------------------------------------|
| `ngrok.k8s.ngrok.com`     | AgentEndpoint, CloudEndpoint, KubernetesOperator, NgrokTrafficPolicy |
| `ingress.k8s.ngrok.com`   | Domain, IPPolicy                                            |
| `bindings.k8s.ngrok.com`  | BoundEndpoint                                               |

## Scope

All CRDs are **Namespaced**.

## Status Conditions

All CRDs use `[]metav1.Condition` for status reporting with the following constraints:

- **MaxItems:** 8
- **List type:** map (keyed by `type`)

The common condition type is `Ready`, which summarizes the overall health of the resource. Individual controllers may define additional condition types specific to their resource.

## Finalizers

The operator adds the finalizer `k8s.ngrok.com/finalizer` to resources it manages. This ensures that:

1. The operator can clean up remote ngrok API resources before the Kubernetes resource is deleted.
2. During drain, finalizers are removed to allow Kubernetes garbage collection.

## Owner References

Controllers set owner references on child resources they create. For example, the Service LoadBalancer controller sets an owner reference on created AgentEndpoint/CloudEndpoint resources pointing back to the parent Service.

## Default Field Values

Most CRDs that correspond to ngrok API resources include:

| Field         | Default Value                         |
|---------------|---------------------------------------|
| `description` | `"Created by the ngrok-operator"`     |
| `metadata`    | `"{\"owned-by\":\"ngrok-operator\"}"` |

**Note:** Domain and IPPolicy CRDs use legacy defaults of `"Created by kubernetes-ingress-controller"` and `"{\"owned-by\":\"kubernetes-ingress-controller\"}"`.

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

Configures a traffic policy via either an inline definition or a reference to an NgrokTrafficPolicy resource. Exactly one of `inline` or `targetRef` must be specified (enforced via XValidation rules).

| Field       | Type            | Required | Description                              |
|-------------|-----------------|----------|------------------------------------------|
| `inline`    | json.RawMessage | No       | Inline traffic policy JSON (schemaless)  |
| `targetRef` | K8sObjectRef    | No       | Reference to an NgrokTrafficPolicy       |

### ApplicationProtocol

Enum: `http1`, `http2`

### ProxyProtocolVersion

Enum: `"1"`, `"2"`
