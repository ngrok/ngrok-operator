package common

import (
	"bytes"
	"encoding/json"
	"errors"
)

// MetadataValue is the free-form "metadata" associated with an ngrok API object
// (Domain, IPPolicy, CloudEndpoint, AgentEndpoint, KubernetesOperator).
//
// Canonically it is a map of string key/value pairs. It is serialized to the
// ngrok API as a compact JSON-object string (see APIString).
//
// LEGACY-metadata-format: for backward compatibility during the metadata-format
// migration the value also accepts a raw JSON string — the wire form the
// operator previously required, where users hand-rolled a JSON object into a
// string (e.g. `metadata: '{"owned-by":"ngrok-operator"}'`). Legacy string
// values are passed through to the ngrok API verbatim. In the cleanup release
// the string form is dropped and this collapses to a plain map[string]string.
//
// The field is schemaless with pruning disabled in the CRD so the API server
// accepts both the string and object forms during the migration window; the
// operator validates and normalizes the value. The raw JSON is preserved so the
// value round-trips through the API server byte-for-byte.
type MetadataValue struct {
	raw json.RawMessage
}

// DefaultMetadataJSON is the operator's default metadata value. It is still a
// JSON string (legacy form) during the migration window so that objects which
// take the default remain readable by a rolled-back prior release.
//
// LEGACY-metadata-format: flip this default to the object form in the cleanup
// release.
const DefaultMetadataJSON = `{"owned-by":"ngrok-operator"}`

// MetadataFromMap builds a canonical (object-form) MetadataValue from a map.
func MetadataFromMap(m map[string]string) *MetadataValue {
	// json.Marshal emits map keys in sorted order, so the encoding is stable.
	b, err := json.Marshal(m)
	if err != nil {
		return &MetadataValue{}
	}
	return &MetadataValue{raw: b}
}

// MetadataFromLegacyString builds a legacy (string-form) MetadataValue that is
// passed through to the ngrok API verbatim. Used by operator-generated objects,
// which keep writing the string form during the migration window for rollback
// safety.
//
// LEGACY-metadata-format: delete in the cleanup release once operator-generated
// objects write the object form only.
func MetadataFromLegacyString(s string) *MetadataValue {
	b, err := json.Marshal(s)
	if err != nil {
		return &MetadataValue{}
	}
	return &MetadataValue{raw: b}
}

// APIString returns the string the ngrok API expects for the metadata field.
//
//   - object form  -> a compact JSON object string with sorted keys (stable, so
//     it does not produce spurious diffs against the API's stored value)
//   - legacy string form -> the string verbatim
//   - unset/null   -> ""
func (m *MetadataValue) APIString() string {
	if m == nil {
		return ""
	}
	t := bytes.TrimSpace(m.raw)
	if len(t) == 0 || string(t) == "null" {
		return ""
	}

	// LEGACY-metadata-format (read-side cleanup): drop this string branch; the
	// value is always an object form after the string form is removed.
	if t[0] == '"' {
		var s string
		if err := json.Unmarshal(t, &s); err == nil {
			return s
		}
		return ""
	}

	// Canonical object form. Re-marshal through map[string]string so the output
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

// UsesDeprecatedStringForm reports whether the value is a user-authored legacy
// JSON string that should be migrated to the map form. The operator default
// value is excluded, so objects that merely took the default are not flagged
// (they would otherwise emit a deprecation event on every reconcile).
//
// LEGACY-metadata-format: delete in the cleanup release.
func (m *MetadataValue) UsesDeprecatedStringForm() bool {
	return m.IsLegacyString() && m.APIString() != DefaultMetadataJSON
}

// IsLegacyString reports whether the value is in the deprecated JSON-string form.
//
// LEGACY-metadata-format: delete in the cleanup release.
func (m *MetadataValue) IsLegacyString() bool {
	if m == nil {
		return false
	}
	t := bytes.TrimSpace(m.raw)
	return len(t) > 0 && t[0] == '"'
}

// Map returns the value as a map when it is in canonical object form. The second
// return is false for the legacy string form or a non-flat object.
func (m *MetadataValue) Map() (map[string]string, bool) {
	if m == nil {
		return nil, false
	}
	t := bytes.TrimSpace(m.raw)
	if len(t) == 0 || t[0] != '{' {
		return nil, false
	}
	var mm map[string]string
	if err := json.Unmarshal(t, &mm); err != nil {
		return nil, false
	}
	return mm, true
}

// MarshalJSON implements json.Marshaler, echoing the raw value so it round-trips.
func (m MetadataValue) MarshalJSON() ([]byte, error) {
	if len(m.raw) == 0 {
		return []byte("null"), nil
	}
	return m.raw, nil
}

// UnmarshalJSON implements json.Unmarshaler, retaining the raw bytes as received.
func (m *MetadataValue) UnmarshalJSON(data []byte) error {
	if m == nil {
		return errors.New("MetadataValue: UnmarshalJSON on nil pointer")
	}
	m.raw = append(m.raw[:0], data...)
	return nil
}

// DeepCopyInto is a hand-written deepcopy (the raw slice must be copied, not
// aliased). controller-gen uses this instead of generating its own.
func (m *MetadataValue) DeepCopyInto(out *MetadataValue) {
	*out = MetadataValue{}
	if m.raw != nil {
		out.raw = make(json.RawMessage, len(m.raw))
		copy(out.raw, m.raw)
	}
}

// DeepCopy is a hand-written deepcopy.
func (m *MetadataValue) DeepCopy() *MetadataValue {
	if m == nil {
		return nil
	}
	out := new(MetadataValue)
	m.DeepCopyInto(out)
	return out
}
