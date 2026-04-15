# Traffic Policy

## Overview

TrafficPolicy is a cross-cutting resource that defines traffic handling rules (rate limiting, header manipulation, authentication, etc.) applied to ngrok endpoints. It can be referenced by multiple controllers and applied to endpoints in several ways.

## Application Methods

### 1. CRD Reference

Traffic policies can be referenced directly on endpoint CRDs:

- **AgentEndpoint**: `spec.trafficPolicy.targetRef` (K8sObjectRef) or `spec.trafficPolicy.inline` (raw JSON)
- **CloudEndpoint**: `spec.trafficPolicyName` (string) or `spec.trafficPolicy` (inline TrafficPolicySpec)

For AgentEndpoint, exactly one of `inline` or `targetRef` must be specified when `trafficPolicy` is present (enforced by XValidation).

### 2. Annotation

The `ngrok.com/traffic-policy` annotation on parent resources (Service, Ingress, Gateway routes) references a TrafficPolicy by name in the same namespace:

```yaml
annotations:
  ngrok.com/traffic-policy: "my-policy"
```

### 3. Inline on Parent Resources

Some parent controllers support inline traffic policy configuration via their own mechanisms (e.g., the Gateway API's extensionRef).

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
