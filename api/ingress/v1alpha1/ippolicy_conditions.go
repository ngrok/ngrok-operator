package v1alpha1

// IPPolicyConditionType is a type of condition for an IPPolicy.
type IPPolicyConditionType string

// IPPolicyConditionReadyReason is a reason for the Ready condition on an IPPolicy.
type IPPolicyConditionReadyReason string

// IPPolicyConditionCreatedReason is a reason for the IPPolicyCreated condition on an IPPolicy.
type IPPolicyConditionCreatedReason string

// IPPolicyConditionRulesConfiguredReason is a reason for the RulesConfigured condition on an IPPolicy.
type IPPolicyConditionRulesConfiguredReason string

const (
	// IPPolicyConditionReady indicates whether the IPPolicy is fully ready and active,
	// with the policy created and all rules properly configured.
	// This condition will be True when both IPPolicyCreated and RulesConfigured are True.
	IPPolicyConditionReady IPPolicyConditionType = "Ready"

	// IPPolicyConditionCreated indicates whether the IP policy has been successfully
	// created via the ngrok API.
	// This condition will be True when policy creation succeeds,
	// and False if policy creation fails.
	IPPolicyConditionCreated IPPolicyConditionType = "IPPolicyCreated"

	// IPPolicyConditionRulesConfigured indicates whether all IP policy rules have
	// been successfully configured and applied.
	// This condition will be True when rule configuration succeeds,
	// and False if there are errors in the rule configuration (e.g., invalid CIDR).
	IPPolicyConditionRulesConfigured IPPolicyConditionType = "RulesConfigured"
)

// Reasons for Ready condition
const (
	// IPPolicyReasonActive is used when the IPPolicy is fully active and ready,
	// with the policy created and all rules configured.
	IPPolicyReasonActive IPPolicyConditionReadyReason = "IPPolicyActive"

	// IPPolicyReasonRulesConfigurationError is used when the Ready condition is False
	// because there are errors in the IP policy rules configuration.
	IPPolicyReasonRulesConfigurationError IPPolicyConditionReadyReason = "IPPolicyRulesConfigurationError"

	// IPPolicyReasonCreationFailed is used when the Ready condition is False
	// because the IP policy could not be created.
	IPPolicyReasonCreationFailed IPPolicyConditionReadyReason = "IPPolicyCreationFailed"

	// IPPolicyReasonInvalidCIDR is used when the Ready condition is False
	// because one or more IP policy rules contain an invalid CIDR block specification.
	IPPolicyReasonInvalidCIDR IPPolicyConditionReadyReason = "IPPolicyInvalidCIDR"
)

// Reasons for IPPolicyCreated condition
const (
	// IPPolicyCreatedReasonCreated is used when the IP policy has been successfully
	// created via the ngrok API.
	IPPolicyCreatedReasonCreated IPPolicyConditionCreatedReason = "IPPolicyCreated"

	// IPPolicyCreatedReasonCreationFailed is used when the controller fails to create
	// the IP policy via the ngrok API.
	IPPolicyCreatedReasonCreationFailed IPPolicyConditionCreatedReason = "IPPolicyCreationFailed"
)

// Reasons for RulesConfigured condition
const (
	// IPPolicyRulesConfiguredReasonConfigured is used when all IP policy rules have been
	// successfully configured and applied.
	IPPolicyRulesConfiguredReasonConfigured IPPolicyConditionRulesConfiguredReason = "IPPolicyRulesConfigured"

	// IPPolicyRulesConfiguredReasonConfigurationError is used when there are errors
	// configuring the IP policy rules.
	IPPolicyRulesConfiguredReasonConfigurationError IPPolicyConditionRulesConfiguredReason = "IPPolicyRulesConfigurationError"

	// IPPolicyRulesConfiguredReasonInvalidCIDR is used when one or more IP policy rules contain
	// an invalid CIDR block specification.
	IPPolicyRulesConfiguredReasonInvalidCIDR IPPolicyConditionRulesConfiguredReason = "IPPolicyInvalidCIDR"
)
