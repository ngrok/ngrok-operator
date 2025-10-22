# CloudEndpoint Conditions

This document describes the status conditions for CloudEndpoint resources.

## Ready

**Type**: `Ready`

**Description**: Indicates whether the CloudEndpoint is fully ready and active in the ngrok cloud. This condition will be True when the cloud endpoint is active and available.

**Possible Values**:
- `True`: CloudEndpoint is fully active and ready in the ngrok cloud
- `False`: CloudEndpoint is not ready (not created or errors present)
- `Unknown`: CloudEndpoint status cannot be determined

**Reasons**:

| Reason | Status | Description |
|--------|--------|-------------|
| CloudEndpointActive | True | CloudEndpoint is fully active and ready in the ngrok cloud |
| Pending | False | CloudEndpoint creation is pending, waiting for dependencies or preconditions to be met |
| Unknown | Unknown | CloudEndpoint status cannot be determined |
| DomainNotReady | False | CloudEndpoint is not ready because a referenced Domain resource is not yet ready |

## CloudEndpointCreated

**Type**: `CloudEndpointCreated`

**Description**: Indicates whether the cloud endpoint has been successfully created via the ngrok API. This condition will be True when endpoint creation succeeds, and False if endpoint creation fails.

**Reasons**:

| Reason | Status | Description |
|--------|--------|-------------|
| CloudEndpointCreated | True | Cloud endpoint has been successfully created via the ngrok API |
| CloudEndpointCreationFailed | False | Failed to create the cloud endpoint via the ngrok API |

## Example Status

```yaml
status:
  conditions:
  - type: Ready
    status: "True"
    reason: CloudEndpointActive
    message: "CloudEndpoint is active in ngrok cloud"
    lastTransitionTime: "2024-01-15T10:30:00Z"
  - type: CloudEndpointCreated
    status: "True"
    reason: CloudEndpointCreated
    message: "Cloud endpoint successfully created"
    lastTransitionTime: "2024-01-15T10:29:00Z"
```
