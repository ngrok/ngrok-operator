package v1alpha1

// BoundEndpointConditionType is a type of condition for a BoundEndpoint.
type BoundEndpointConditionType string

// BoundEndpointConditionReadyReason is a reason for the Ready condition on a BoundEndpoint.
type BoundEndpointConditionReadyReason string

// BoundEndpointConditionServicesCreatedReason is a reason for the ServicesCreated condition on a BoundEndpoint.
type BoundEndpointConditionServicesCreatedReason string

// BoundEndpointConditionConnectivityVerifiedReason is a reason for the ConnectivityVerified condition on a BoundEndpoint.
type BoundEndpointConditionConnectivityVerifiedReason string

const (
	// BoundEndpointConditionReady indicates whether the BoundEndpoint is fully ready
	// and all required Kubernetes services have been created and connectivity has been verified.
	// This condition will be True when both ServicesCreated and ConnectivityVerified are True.
	BoundEndpointConditionReady BoundEndpointConditionType = "Ready"

	// BoundEndpointConditionServicesCreated indicates whether all required Kubernetes
	// services for the BoundEndpoint have been successfully created.
	// This condition will be True when service creation completes successfully,
	// and False if service creation fails.
	BoundEndpointConditionServicesCreated BoundEndpointConditionType = "ServicesCreated"

	// BoundEndpointConditionConnectivityVerified indicates whether connectivity
	// to the bound endpoint has been successfully verified.
	// This condition will be True when connectivity checks pass,
	// and False if connectivity verification fails.
	BoundEndpointConditionConnectivityVerified BoundEndpointConditionType = "ConnectivityVerified"
)

// Reasons for Ready condition
const (
	// BoundEndpointReasonReady is used when the BoundEndpoint is fully ready,
	// with all services created and connectivity verified.
	BoundEndpointReasonReady BoundEndpointConditionReadyReason = "BoundEndpointReady"

	// BoundEndpointReasonServicesNotCreated is used when the Ready condition is False
	// because required Kubernetes services have not been created yet.
	BoundEndpointReasonServicesNotCreated BoundEndpointConditionReadyReason = "ServicesNotCreated"

	// BoundEndpointReasonConnectivityNotVerified is used when the Ready condition is False
	// because connectivity to the bound endpoint has not been verified yet.
	BoundEndpointReasonConnectivityNotVerified BoundEndpointConditionReadyReason = "ConnectivityNotVerified"
)

// Reasons for ServicesCreated condition
const (
	// BoundEndpointReasonServicesCreated is used when all required Kubernetes services
	// have been successfully created for the BoundEndpoint.
	BoundEndpointReasonServicesCreated BoundEndpointConditionServicesCreatedReason = "ServicesCreated"

	// BoundEndpointReasonServiceCreationFailed is used when the controller fails to create
	// one or more required Kubernetes services for the BoundEndpoint.
	BoundEndpointReasonServiceCreationFailed BoundEndpointConditionServicesCreatedReason = "ServiceCreationFailed"
)

// Reasons for ConnectivityVerified condition
const (
	// BoundEndpointReasonConnectivityVerified is used when connectivity to the
	// BoundEndpoint has been successfully verified.
	BoundEndpointReasonConnectivityVerified BoundEndpointConditionConnectivityVerifiedReason = "ConnectivityVerified"

	// BoundEndpointReasonConnectivityFailed is used when connectivity verification
	// to the BoundEndpoint fails.
	BoundEndpointReasonConnectivityFailed BoundEndpointConditionConnectivityVerifiedReason = "ConnectivityFailed"
)
