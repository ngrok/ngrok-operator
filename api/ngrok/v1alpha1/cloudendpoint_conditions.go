package v1alpha1

// CloudEndpointConditionType is a type of condition for a CloudEndpoint.
type CloudEndpointConditionType string

// CloudEndpointConditionReadyReason is a reason for the Ready condition on a CloudEndpoint.
type CloudEndpointConditionReadyReason string

// CloudEndpointConditionCreatedReason is a reason for the CloudEndpointCreated condition on a CloudEndpoint.
type CloudEndpointConditionCreatedReason string

const (
	// CloudEndpointConditionReady indicates whether the CloudEndpoint is fully ready
	// and active in the ngrok cloud.
	// This condition will be True when the cloud endpoint is active and available.
	CloudEndpointConditionReady CloudEndpointConditionType = "Ready"

	// CloudEndpointConditionCreated indicates whether the cloud endpoint has been
	// successfully created via the ngrok API.
	// This condition will be True when endpoint creation succeeds,
	// and False if endpoint creation fails.
	CloudEndpointConditionCreated CloudEndpointConditionType = "CloudEndpointCreated"
)

// Reasons for Ready condition
const (
	// CloudEndpointReasonActive is used when the CloudEndpoint is fully active
	// and ready in the ngrok cloud.
	CloudEndpointReasonActive CloudEndpointConditionReadyReason = "CloudEndpointActive"

	// CloudEndpointReasonPending is used when the CloudEndpoint creation is pending,
	// waiting for dependencies or preconditions to be met.
	CloudEndpointReasonPending CloudEndpointConditionReadyReason = "Pending"

	// CloudEndpointReasonUnknown is used when the CloudEndpoint status cannot be determined.
	CloudEndpointReasonUnknown CloudEndpointConditionReadyReason = "Unknown"

	// CloudEndpointReasonDomainNotReady is used when the CloudEndpoint is not ready
	// because a referenced Domain resource is not yet ready.
	CloudEndpointReasonDomainNotReady CloudEndpointConditionReadyReason = "DomainNotReady"
)

// Reasons for CloudEndpointCreated condition
const (
	// CloudEndpointReasonCreated is used when the cloud endpoint has been
	// successfully created via the ngrok API.
	CloudEndpointReasonCreated CloudEndpointConditionCreatedReason = "CloudEndpointCreated"

	// CloudEndpointReasonCreationFailed is used when the controller fails to create
	// the cloud endpoint via the ngrok API.
	CloudEndpointReasonCreationFailed CloudEndpointConditionCreatedReason = "CloudEndpointCreationFailed"
)
