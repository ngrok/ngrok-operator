package util

import "encoding/json"

// IsLegacyPolicy determines if the configured policy matches the old inbound/outbound format as opposed to the
// current phase-based format. If the supplied json-message is malformed, this function will return false, as
// it's technically not in a proper legacy format.
func IsLegacyPolicy(msg json.RawMessage) bool {
	var policyData map[string]any

	if err := json.Unmarshal(msg, &policyData); err != nil {
		// if unmarshalling fails, there is something inherently wrong with the configuration. In this
		// case, attempting to process as the new definitions will fail as well, so we can surface the
		// error then.
		return false
	}

	if _, ok := policyData["inbound"]; ok {
		return true
	}

	if _, ok := policyData["outbound"]; ok {
		return true
	}

	return false
}

// ExtractEnabledField looks for a top level "enabled" in the json, removes it, and returns the value. For the module and gateway
// configuration paths, this value may be present. We need to respect this value and also can't include it in the policy payload.
//
// If the "enabled" field is not present in the message, the message is returned unmodified.
func ExtractEnabledField(msg json.RawMessage) (json.RawMessage, *bool, error) {
	if msg == nil || len(msg) == 0 {
		return msg, nil, nil
	}

	var policyData map[string]any
	err := json.Unmarshal(msg, &policyData)
	if err != nil {
		return nil, nil, err
	}

	if enabledSetVal, ok := policyData["enabled"]; ok {
		return handleEnabledInPolicy(policyData, enabledSetVal)
	}

	return msg, nil, nil
}

func handleEnabledInPolicy(policyData map[string]any, enabledSetVal any) (json.RawMessage, *bool, error) {
	delete(policyData, "enabled")
	var setVal *bool
	if enabled, ok := enabledSetVal.(bool); ok {
		setVal = &enabled
	}

	updatedTrafficPolicy, err := json.Marshal(policyData)
	if err != nil {
		return nil, nil, err
	}

	return updatedTrafficPolicy, setVal, nil
}
