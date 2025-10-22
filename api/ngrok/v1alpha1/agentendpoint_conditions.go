package v1alpha1

// AgentEndpointConditionType is a type of condition for an AgentEndpoint.
type AgentEndpointConditionType string

// AgentEndpointConditionReadyReason is a reason for the Ready condition on an AgentEndpoint.
type AgentEndpointConditionReadyReason string

// AgentEndpointConditionEndpointCreatedReason is a reason for the EndpointCreated condition on an AgentEndpoint.
type AgentEndpointConditionEndpointCreatedReason string

// AgentEndpointConditionTrafficPolicyReason is a reason for the TrafficPolicyApplied condition on an AgentEndpoint.
type AgentEndpointConditionTrafficPolicyReason string

const (
	// AgentEndpointConditionReady indicates whether the AgentEndpoint is fully ready
	// and active, with the endpoint created and any traffic policies applied.
	// This condition will be True when the endpoint is active and healthy.
	AgentEndpointConditionReady AgentEndpointConditionType = "Ready"

	// AgentEndpointConditionEndpointCreated indicates whether the ngrok endpoint
	// has been successfully created via the ngrok API.
	// This condition will be True when endpoint creation succeeds,
	// and False if endpoint creation fails.
	AgentEndpointConditionEndpointCreated AgentEndpointConditionType = "EndpointCreated"

	// AgentEndpointConditionTrafficPolicy indicates whether any configured traffic
	// policies have been successfully applied to the endpoint.
	// This condition will be True when traffic policy application succeeds,
	// and False if there are errors applying the policy.
	AgentEndpointConditionTrafficPolicy AgentEndpointConditionType = "TrafficPolicyApplied"
)

// Reasons for Ready condition
const (
	// AgentEndpointReasonActive is used when the AgentEndpoint is fully active
	// and ready to serve traffic.
	AgentEndpointReasonActive AgentEndpointConditionReadyReason = "EndpointActive"

	// AgentEndpointReasonReconciling is used when the AgentEndpoint is currently
	// being reconciled and is not yet ready.
	AgentEndpointReasonReconciling AgentEndpointConditionReadyReason = "Reconciling"

	// AgentEndpointReasonPending is used when the AgentEndpoint creation is pending,
	// waiting for dependencies or preconditions to be met.
	AgentEndpointReasonPending AgentEndpointConditionReadyReason = "Pending"

	// AgentEndpointReasonUnknown is used when the AgentEndpoint status cannot be determined.
	AgentEndpointReasonUnknown AgentEndpointConditionReadyReason = "Unknown"

	// AgentEndpointReasonDomainNotReady is used when the AgentEndpoint is not ready
	// because a referenced Domain resource is not yet ready.
	AgentEndpointReasonDomainNotReady AgentEndpointConditionReadyReason = "DomainNotReady"
)

// Reasons for EndpointCreated condition
const (
	// AgentEndpointReasonEndpointCreated is used when the ngrok endpoint has been
	// successfully created via the ngrok API.
	AgentEndpointReasonEndpointCreated AgentEndpointConditionEndpointCreatedReason = "EndpointCreated"

	// AgentEndpointReasonNgrokAPIError is used when there is an error communicating
	// with the ngrok API to create the endpoint.
	AgentEndpointReasonNgrokAPIError AgentEndpointConditionEndpointCreatedReason = "NgrokAPIError"

	// AgentEndpointReasonConfigError is used when the AgentEndpoint configuration
	// is invalid or incomplete.
	AgentEndpointReasonConfigError AgentEndpointConditionEndpointCreatedReason = "ConfigurationError"

	// AgentEndpointReasonUpstreamError is used when there is an error with the
	// upstream service configuration or connectivity.
	AgentEndpointReasonUpstreamError AgentEndpointConditionEndpointCreatedReason = "UpstreamError"
)

// Reasons for TrafficPolicyApplied condition
const (
	// AgentEndpointReasonTrafficPolicyApplied is used when configured traffic policies
	// have been successfully applied to the endpoint.
	AgentEndpointReasonTrafficPolicyApplied AgentEndpointConditionTrafficPolicyReason = "TrafficPolicyApplied"

	// AgentEndpointReasonTrafficPolicyError is used when there is an error applying
	// traffic policies to the endpoint.
	AgentEndpointReasonTrafficPolicyError AgentEndpointConditionTrafficPolicyReason = "TrafficPolicyError"
)
