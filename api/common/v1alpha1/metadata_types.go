package common

import (
	"encoding/json"
)

// The ngrok "metadata" field on a CRD (Domain, IPPolicy, CloudEndpoint,
// AgentEndpoint, KubernetesOperator) is a map[string]string. Canonically it
// is an object of string key/value pairs; the operator serializes it to the
// string the ngrok API expects via MetadataAPIString.

// MetadataAPIString converts a CRD metadata map into the string the ngrok API
// expects: a compact JSON object with sorted keys (stable, no spurious diffs
// against the API's stored value), or "" when unset.
func MetadataAPIString(m map[string]string) string {
	if len(m) == 0 {
		return ""
	}
	b, err := json.Marshal(m)
	if err != nil {
		return ""
	}
	return string(b)
}

// MetadataMapFromJSON parses a flat JSON object string (as built by
// operator-internal metadata merging) into a metadata map. Returns nil for an
// empty string or invalid/non-flat JSON.
func MetadataMapFromJSON(s string) map[string]string {
	if s == "" {
		return nil
	}
	var m map[string]string
	if err := json.Unmarshal([]byte(s), &m); err != nil {
		return nil
	}
	return m
}
