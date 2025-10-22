# IPPolicy Conditions

This document describes the status conditions for IPPolicy resources.

## Ready

**Type**: `Ready`

**Description**: Indicates whether the IPPolicy is fully ready and active, with the policy created and all rules properly configured. This condition will be True when both IPPolicyCreated and RulesConfigured are True.

**Possible Values**:
- `True`: IPPolicy is fully active and ready
- `False`: IPPolicy is not ready (creation failed or rules misconfigured)
- `Unknown`: IPPolicy status cannot be determined

**Reasons**:

| Reason | Status | Description |
|--------|--------|-------------|
| IPPolicyActive | True | IPPolicy is fully active and ready, with the policy created and all rules configured |
| IPPolicyRulesConfigurationError | False | There are errors in the IP policy rules configuration |
| IPPolicyCreationFailed | False | IP policy could not be created |
| IPPolicyInvalidCIDR | False | One or more IP policy rules contain an invalid CIDR block specification |

## IPPolicyCreated

**Type**: `IPPolicyCreated`

**Description**: Indicates whether the IP policy has been successfully created via the ngrok API. This condition will be True when policy creation succeeds, and False if policy creation fails.

**Reasons**:

| Reason | Status | Description |
|--------|--------|-------------|
| IPPolicyCreated | True | IP policy has been successfully created via the ngrok API |
| IPPolicyCreationFailed | False | Failed to create the IP policy via the ngrok API |

## RulesConfigured

**Type**: `RulesConfigured`

**Description**: Indicates whether all IP policy rules have been successfully configured and applied. This condition will be True when rule configuration succeeds, and False if there are errors in the rule configuration (e.g., invalid CIDR).

**Reasons**:

| Reason | Status | Description |
|--------|--------|-------------|
| IPPolicyRulesConfigured | True | All IP policy rules have been successfully configured and applied |
| IPPolicyRulesConfigurationError | False | Errors configuring the IP policy rules |
| IPPolicyInvalidCIDR | False | One or more IP policy rules contain an invalid CIDR block specification |

## Example Status

```yaml
status:
  conditions:
  - type: Ready
    status: "True"
    reason: IPPolicyActive
    message: "IPPolicy is active with all rules configured"
    lastTransitionTime: "2024-01-15T10:30:00Z"
  - type: IPPolicyCreated
    status: "True"
    reason: IPPolicyCreated
    message: "IP policy successfully created"
    lastTransitionTime: "2024-01-15T10:29:00Z"
  - type: RulesConfigured
    status: "True"
    reason: IPPolicyRulesConfigured
    message: "All policy rules configured"
    lastTransitionTime: "2024-01-15T10:30:00Z"
```
