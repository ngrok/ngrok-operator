# Traffic Policy

## Overview

TrafficPolicy is a cross-cutting resource that defines traffic handling rules (rate limiting, header manipulation, authentication, etc.) applied to ngrok endpoints. It can be referenced by multiple controllers and applied to endpoints in several ways.

See the [ngrok Traffic Policy documentation](https://ngrok.com/docs/traffic-policy/) for the full reference on available actions, phases, and expressions.

## Internal Forwarding

When a CloudEndpoint forwards traffic to an AgentEndpoint (e.g. when using the `endpoints-verbose` mapping strategy), the operator injects a `forward_internal` action into the traffic policy to route traffic from the cloud endpoint to the in-cluster agent. This injection happens transparently — the user-supplied traffic policy is preserved and the forwarding action is appended by the operator. This is an exception to the general design principle of operator transparency with respect to traffic policy; the operator must manipulate the policy here because forwarding between cloud and agent endpoints requires an explicit phase action that the user cannot supply themselves.

## Application Methods

### 1. CRD Reference

Traffic policies can be referenced directly on endpoint CRDs:

- **AgentEndpoint**: `spec.trafficPolicy.targetRef` (K8sObjectRef) or `spec.trafficPolicy.inline` (raw JSON)
- **CloudEndpoint**: `spec.trafficPolicy.targetRef` (K8sObjectRef) or `spec.trafficPolicy.inline` (raw JSON)

For AgentEndpoint, exactly one of `inline` or `targetRef` must be specified when `trafficPolicy` is present (enforced by XValidation).

### 2. Annotation

The `ngrok.com/traffic-policy` annotation on parent resources (Service, Ingress, Gateway routes) references a TrafficPolicy by name in the same namespace:

```yaml
annotations:
  ngrok.com/traffic-policy: "my-policy"
```

### 3. Reference on Parent Resources

Some parent controllers support traffic policy references via their own mechanisms (e.g., the Gateway API's `extensionRef` on a `Gateway` resource). This is distinct from the annotation-based approach and is scoped to the resource type that supports it.

See [mapping-strategy.md](../mapping-strategy.md) for how the mapping strategy determines where the traffic policy is applied.

## Resolution with Mapping Strategy

The `ngrok.com/mapping-strategy` annotation affects where the traffic policy is applied:

| Mapping Strategy     | Policy Applied To   |
|----------------------|---------------------|
| `endpoints`          | `AgentEndpoint`     |
| `endpoints-verbose`  | `CloudEndpoint`     |

## Watch Behavior

Controllers that support traffic policy references watch TrafficPolicy resources and re-reconcile when the referenced policy changes. This ensures endpoint configuration stays in sync with policy updates.

## Validation

The TrafficPolicy controller performs:

1. **JSON syntax validation**: Ensures the policy field contains valid JSON.
2. **Deprecation warnings**: Emits events for deprecated features:
   - Legacy `directions` field
   - `enabled` field on rules

The policy content is **schemaless** — the operator does not enforce structure beyond valid JSON. Schema validation is performed by the ngrok API when the policy is applied to an endpoint.

## Events

| Event                     | Description                         |
|---------------------------|-------------------------------------|
| `TrafficPolicyParseFailed`| Emitted when JSON parsing fails     |
| `PolicyDeprecation`       | Emitted when deprecated features are used |
