# AgentEndpoint Conditions

This document describes the status conditions for AgentEndpoint resources.

## Ready

**Type**: `Ready`

**Description**: Indicates whether the AgentEndpoint is fully ready and active, with the endpoint created and any traffic policies applied. This condition will be True when the endpoint is active and healthy.

**Possible Values**:
- `True`: AgentEndpoint is fully active and ready to serve traffic
- `False`: AgentEndpoint is not ready (endpoint not created or errors present)
- `Unknown`: AgentEndpoint status cannot be determined

**Reasons**:

| Reason | Status | Description |
|--------|--------|-------------|
| EndpointActive | True | AgentEndpoint is fully active and ready to serve traffic |
| Reconciling | False | AgentEndpoint is currently being reconciled and is not yet ready |
| Pending | False | AgentEndpoint creation is pending, waiting for dependencies or preconditions to be met |
| Unknown | Unknown | AgentEndpoint status cannot be determined |
| DomainNotReady | False | AgentEndpoint is not ready because a referenced Domain resource is not yet ready |

## EndpointCreated

**Type**: `EndpointCreated`

**Description**: Indicates whether the ngrok endpoint has been successfully created via the ngrok API. This condition will be True when endpoint creation succeeds, and False if endpoint creation fails.

**Reasons**:

| Reason | Status | Description |
|--------|--------|-------------|
| EndpointCreated | True | The ngrok endpoint has been successfully created via the ngrok API |
| NgrokAPIError | False | Error communicating with the ngrok API to create the endpoint |
| ConfigurationError | False | AgentEndpoint configuration is invalid or incomplete |
| UpstreamError | False | Error with the upstream service configuration or connectivity |

## TrafficPolicyApplied

**Type**: `TrafficPolicyApplied`

**Description**: Indicates whether any configured traffic policies have been successfully applied to the endpoint. This condition will be True when traffic policy application succeeds, and False if there are errors applying the policy.

**Reasons**:

| Reason | Status | Description |
|--------|--------|-------------|
| TrafficPolicyApplied | True | Configured traffic policies have been successfully applied to the endpoint |
| TrafficPolicyError | False | Error applying traffic policies to the endpoint |

## Example Status

```yaml
status:
  conditions:
  - type: Ready
    status: "True"
    reason: EndpointActive
    message: "AgentEndpoint is active and serving traffic"
    lastTransitionTime: "2024-01-15T10:30:00Z"
  - type: EndpointCreated
    status: "True"
    reason: EndpointCreated
    message: "Endpoint successfully created via ngrok API"
    lastTransitionTime: "2024-01-15T10:29:00Z"
  - type: TrafficPolicyApplied
    status: "True"
    reason: TrafficPolicyApplied
    message: "Traffic policy applied successfully"
    lastTransitionTime: "2024-01-15T10:30:00Z"
```
