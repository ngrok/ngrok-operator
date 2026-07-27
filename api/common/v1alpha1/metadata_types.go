package common

import (
	"bytes"
	"encoding/json"
)

// The ngrok "metadata" field on a CRD (Domain, IPPolicy, CloudEndpoint,
// AgentEndpoint, KubernetesOperator) is a raw JSON value. Canonically it is an
// object of string key/value pairs; the operator serializes it to the string
// the ngrok API expects via MetadataAPIString. The CRD field is a bare
// json.RawMessage with schemaless/preserve-unknown-fields markers, so
// encoding/json and controller-gen handle (un)marshaling and deepcopy — no
// wrapper type is needed.
//
// LEGACY-metadata-format: during the migration window the value also accepts a
// raw JSON string (the wire form the operator previously required, e.g.
// `metadata: '{"owned-by":"ngrok-operator"}'`), passed through verbatim. In the
// cleanup release the string form is dropped and these fields become plain
// map[string]string.

// MetadataAPIString converts a raw CRD metadata value into the string the ngrok
// API expects:
//
//   - object form -> compact JSON object with sorted keys (stable, no spurious
//     diffs against the API's stored value)
//   - legacy string form -> the string verbatim
//   - unset/null / non-flat object -> "" / raw JSON verbatim respectively
func MetadataAPIString(raw json.RawMessage) string {
	t := bytes.TrimSpace(raw)
	if len(t) == 0 || string(t) == "null" {
		return ""
	}

	// LEGACY-metadata-format (read-side cleanup): drop this string branch; the
	// value is always object form once the string form is removed.
	if t[0] == '"' {
		var s string
		if err := json.Unmarshal(t, &s); err == nil {
			return s
		}
		return ""
	}

	// Canonical object form: re-marshal through map[string]string so the output
	// is key-sorted and stable.
	var mm map[string]string
	if err := json.Unmarshal(t, &mm); err == nil {
		if out, err := json.Marshal(mm); err == nil {
			return string(out)
		}
	}
	// Not a flat string map (e.g. nested values): pass the raw JSON through
	// verbatim rather than failing the reconcile.
	return string(t)
}

// MetadataFromMap builds a canonical (object-form) metadata value. json.Marshal
// emits map keys in sorted order, so the encoding is stable.
func MetadataFromMap(m map[string]string) json.RawMessage {
	b, err := json.Marshal(m)
	if err != nil {
		return nil
	}
	return b
}

// MetadataFromLegacyString builds a legacy (string-form) metadata value that is
// passed through to the ngrok API verbatim. Used by operator-generated objects,
// which keep writing the string form during the migration window for rollback
// safety.
//
// LEGACY-metadata-format: delete in the cleanup release once operator-generated
// objects write the object form only.
func MetadataFromLegacyString(s string) json.RawMessage {
	b, err := json.Marshal(s)
	if err != nil {
		return nil
	}
	return b
}
