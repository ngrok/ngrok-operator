# BoundEndpoint Conditions

This document describes the status conditions for BoundEndpoint resources.

## Ready

**Type**: `Ready`

**Description**: Indicates whether the BoundEndpoint is fully ready and all required Kubernetes services have been created and connectivity has been verified. This condition will be True when both ServicesCreated and ConnectivityVerified are True.

**Possible Values**:
- `True`: BoundEndpoint is fully ready with all services created and connectivity verified
- `False`: BoundEndpoint is not ready (services not created or connectivity not verified)
- `Unknown`: BoundEndpoint status cannot be determined

**Reasons**:

| Reason | Status | Description |
|--------|--------|-------------|
| BoundEndpointReady | True | BoundEndpoint is fully ready, with all services created and connectivity verified |
| ServicesNotCreated | False | Required Kubernetes services have not been created yet |
| ConnectivityNotVerified | False | Connectivity to the bound endpoint has not been verified yet |

## ServicesCreated

**Type**: `ServicesCreated`

**Description**: Indicates whether all required Kubernetes services for the BoundEndpoint have been successfully created. This condition will be True when service creation completes successfully, and False if service creation fails.

**Reasons**:

| Reason | Status | Description |
|--------|--------|-------------|
| ServicesCreated | True | All required Kubernetes services have been successfully created |
| ServiceCreationFailed | False | Failed to create one or more required Kubernetes services |

## ConnectivityVerified

**Type**: `ConnectivityVerified`

**Description**: Indicates whether connectivity to the bound endpoint has been successfully verified. This condition will be True when connectivity checks pass, and False if connectivity verification fails.

**Reasons**:

| Reason | Status | Description |
|--------|--------|-------------|
| ConnectivityVerified | True | Connectivity to the BoundEndpoint has been successfully verified |
| ConnectivityFailed | False | Connectivity verification to the BoundEndpoint fails |

## Example Status

```yaml
status:
  conditions:
  - type: Ready
    status: "True"
    reason: BoundEndpointReady
    message: "BoundEndpoint is fully operational"
    lastTransitionTime: "2024-01-15T10:30:00Z"
  - type: ServicesCreated
    status: "True"
    reason: ServicesCreated
    message: "All required services have been created"
    lastTransitionTime: "2024-01-15T10:29:00Z"
  - type: ConnectivityVerified
    status: "True"
    reason: ConnectivityVerified
    message: "Connectivity has been verified"
    lastTransitionTime: "2024-01-15T10:30:00Z"
```
