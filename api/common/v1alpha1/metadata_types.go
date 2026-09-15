package common

import (
	"bytes"
	"encoding/json"
	"fmt"
)

// The ngrok "metadata" field on a CRD (Domain, IPPolicy, CloudEndpoint,
// AgentEndpoint, KubernetesOperator) is arbitrary key/value data forwarded to
// the ngrok API as a single string. Canonically it is an object of string
// key/value pairs; MetadataAPIString serializes it to the string the ngrok API
// expects. On the deprecated v1alpha1 CRDs the field is a bare json.RawMessage
// with schemaless/preserve-unknown-fields markers, so encoding/json and
// controller-gen handle (un)marshaling and deepcopy — no wrapper type is
// needed. On the canonical ngrok.com/v1 CRDs it is a plain map[string]string.
//
// LEGACY-metadata-format: the v1alpha1 field also accepts a raw JSON string
// (the wire form the operator required before 0.24, e.g.
// `metadata: '{"owned-by":"ngrok-operator"}'`). The operator no longer writes
// that form; it is read-only compatibility for objects users have not yet
// converted, and it is removed when the v1alpha1 CRDs are.

// DefaultMetadataOwnedBy is the value of the `owned-by` key in the CRD-level
// metadata default. It must stay in sync with the `+kubebuilder:default`
// markers on every `spec.metadata` field: write paths fall back to this exact
// map so a defaulted object and an operator-written object compare equal, and
// the operator does not rewrite the same object on every sync.
const DefaultMetadataOwnedBy = "ngrok-operator"

// DefaultMetadataMap returns the map the CRD default stamps onto an object
// that does not set spec.metadata.
func DefaultMetadataMap() map[string]string {
	return map[string]string{"owned-by": DefaultMetadataOwnedBy}
}

// MetadataAPIString converts a raw CRD metadata value into the string the ngrok
// API expects:
//
//   - object form -> compact JSON object with sorted keys
//   - legacy string form holding a flat JSON object -> the same canonical
//     bytes as the object form, so the two shapes are indistinguishable to the
//     ngrok API and to every drift comparison against it
//   - legacy string form holding anything else -> the string verbatim
//   - unset/null / non-flat object -> "" / raw JSON verbatim respectively
//
// Canonicalizing both shapes is load-bearing during the ngrok.com/v1
// migration: a v1alpha1 object and its v1 twin can resolve to the same ngrok
// resource, and if the two controllers disagreed byte-for-byte on the metadata
// they would take turns updating it forever.
func MetadataAPIString(raw json.RawMessage) string {
	t := bytes.TrimSpace(raw)
	if len(t) == 0 || string(t) == "null" {
		return ""
	}

	// LEGACY-metadata-format (read-side cleanup): drop this branch with the
	// v1alpha1 CRDs; the value is always object form on ngrok.com/v1.
	if t[0] == '"' {
		var s string
		if err := json.Unmarshal(t, &s); err != nil {
			return ""
		}
		if m, ok := flatStringMap([]byte(s)); ok {
			return MetadataAPIStringFromMap(m)
		}
		// Not a JSON object of strings (free-form text, an array, a nested
		// object): forward it to ngrok unchanged rather than failing the
		// reconcile. Such values cannot be represented on ngrok.com/v1 and are
		// called out in the upgrade guide.
		return s
	}

	if m, ok := flatStringMap(t); ok {
		return MetadataAPIStringFromMap(m)
	}
	// Not a flat string map (nested/null/non-string values): pass the raw JSON
	// through verbatim rather than rewriting or failing the reconcile.
	return string(t)
}

// MetadataAPIStringFromMap converts a map-form CRD metadata value into the
// string the ngrok API expects. json.Marshal sorts map keys, so the encoding is
// stable and produces no spurious diffs against the API's stored value. An
// empty or unset map yields "", matching an unset raw value.
func MetadataAPIStringFromMap(m map[string]string) string {
	if len(m) == 0 {
		return ""
	}
	b, err := json.Marshal(m)
	if err != nil {
		return ""
	}
	return string(b)
}

// MetadataFromMap builds a canonical (object-form) raw metadata value for the
// v1alpha1 CRDs. An empty map returns nil so the field is omitted and the CRD
// default applies.
func MetadataFromMap(m map[string]string) json.RawMessage {
	if len(m) == 0 {
		return nil
	}
	b, err := json.Marshal(m)
	if err != nil {
		return nil
	}
	return b
}

// MetadataMapFromJSON parses a flat JSON object string — as built by the
// operator's internal metadata merging (ir.MergeMetadata) — into a metadata
// map. It returns an error rather than a nil map for input it cannot represent,
// so callers fall back deliberately instead of silently dropping metadata.
func MetadataMapFromJSON(s string) (map[string]string, error) {
	if s == "" {
		return nil, nil
	}
	m, ok := flatStringMap([]byte(s))
	if !ok {
		return nil, fmt.Errorf("metadata %q is not a JSON object of string values", s)
	}
	return m, nil
}

// flatStringMap decodes b into a map[string]string, reporting false unless b is
// a JSON object whose every value is a JSON string. The probe decodes values as
// raw messages first because unmarshaling straight into map[string]string would
// coerce a null value to "", silently changing it.
func flatStringMap(b []byte) (map[string]string, bool) {
	// Require an object: json.Unmarshal of `null` into a map succeeds and
	// leaves it nil, which would turn a legacy `"null"` into an empty map.
	if t := bytes.TrimSpace(b); len(t) == 0 || t[0] != '{' {
		return nil, false
	}

	var probe map[string]json.RawMessage
	if err := json.Unmarshal(b, &probe); err != nil || !allStringValues(probe) {
		return nil, false
	}
	var m map[string]string
	if err := json.Unmarshal(b, &m); err != nil {
		return nil, false
	}
	return m, true
}

// allStringValues reports whether every value in the decoded object is a JSON
// string (not null, number, bool, array, or object).
func allStringValues(m map[string]json.RawMessage) bool {
	for _, v := range m {
		t := bytes.TrimSpace(v)
		if len(t) == 0 || t[0] != '"' {
			return false
		}
	}
	return true
}
