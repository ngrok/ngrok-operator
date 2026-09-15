package common

import (
	"encoding/json"
	"testing"
)

func TestMetadataAPIString(t *testing.T) {
	tests := []struct {
		name string
		json string // the raw JSON value of the metadata field
		want string
	}{
		{name: "unset", json: `null`, want: ""},
		{name: "empty object", json: `{}`, want: ""},
		{name: "legacy empty string", json: `""`, want: ""},
		{
			name: "legacy json-object string is canonicalized",
			json: `"{\"owned-by\":\"ngrok-operator\"}"`,
			want: `{"owned-by":"ngrok-operator"}`,
		},
		{
			name: "legacy json-object string with whitespace is canonicalized",
			json: `"{\"owned-by\": \"ngrok-operator\"}"`,
			want: `{"owned-by":"ngrok-operator"}`,
		},
		{
			name: "legacy json-object string keys are sorted",
			json: `"{\"zeta\":\"1\",\"alpha\":\"2\"}"`,
			want: `{"alpha":"2","zeta":"1"}`,
		},
		{
			name: "legacy non-json string passthrough",
			json: `"some free text"`,
			want: "some free text",
		},
		{
			name: "legacy string holding a json array passthrough",
			json: `"[\"a\",\"b\"]"`,
			want: `["a","b"]`,
		},
		{
			name: "object form single key",
			json: `{"owned-by":"ngrok-operator"}`,
			want: `{"owned-by":"ngrok-operator"}`,
		},
		{
			name: "object form keys sorted for stable output",
			json: `{"zeta":"1","alpha":"2"}`,
			want: `{"alpha":"2","zeta":"1"}`,
		},
		{
			name: "nested object passthrough verbatim",
			json: `{"a":{"b":"c"}}`,
			want: `{"a":{"b":"c"}}`,
		},
		{
			name: "null-valued key preserved verbatim (not coerced to empty string)",
			json: `{"a":null}`,
			want: `{"a":null}`,
		},
		{
			name: "numeric-valued key passthrough verbatim",
			json: `{"a":1}`,
			want: `{"a":1}`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := MetadataAPIString(json.RawMessage(tt.json)); got != tt.want {
				t.Errorf("MetadataAPIString(%s) = %q, want %q", tt.json, got, tt.want)
			}
		})
	}
	// nil is unset
	if got := MetadataAPIString(nil); got != "" {
		t.Errorf("MetadataAPIString(nil) = %q, want empty", got)
	}
}

// TestMetadataShapesAgree pins the property the ngrok.com/v1 migration depends
// on: a v1alpha1 object holding the legacy string form and its v1 twin holding
// the equivalent map must produce byte-identical metadata for the ngrok API.
// Without it the two controllers disagree on every reconcile and take turns
// updating the same ngrok resource.
func TestMetadataShapesAgree(t *testing.T) {
	tests := []struct {
		name   string
		legacy string // the string stored in the v1alpha1 field
		m      map[string]string
	}{
		{
			name:   "canonical",
			legacy: `{"owned-by":"ngrok-operator"}`,
			m:      map[string]string{"owned-by": "ngrok-operator"},
		},
		{
			name:   "unsorted keys",
			legacy: `{"zeta":"1","alpha":"2"}`,
			m:      map[string]string{"zeta": "1", "alpha": "2"},
		},
		{
			name:   "whitespace",
			legacy: `{"owned-by": "ngrok-operator", "team": "platform"}`,
			m:      map[string]string{"owned-by": "ngrok-operator", "team": "platform"},
		},
		{
			name:   "indented multiline",
			legacy: "{\n  \"a\": \"1\"\n}",
			m:      map[string]string{"a": "1"},
		},
		{
			name:   "unicode value",
			legacy: `{"team":"café"}`,
			m:      map[string]string{"team": "café"},
		},
		{
			name:   "escaped unicode value",
			legacy: "{\"team\":\"caf\\u00e9\"}",
			m:      map[string]string{"team": "café"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			legacyRaw, err := json.Marshal(tt.legacy)
			if err != nil {
				t.Fatalf("marshaling legacy fixture: %v", err)
			}
			fromLegacy := MetadataAPIString(legacyRaw)
			fromMap := MetadataAPIStringFromMap(tt.m)
			if fromLegacy != fromMap {
				t.Errorf("legacy %q -> %q, map -> %q; want identical", tt.legacy, fromLegacy, fromMap)
			}
			// The object form stored on a v1alpha1 CRD must agree too.
			if fromObject := MetadataAPIString(MetadataFromMap(tt.m)); fromObject != fromMap {
				t.Errorf("object form -> %q, map -> %q; want identical", fromObject, fromMap)
			}
		})
	}
}

func TestMetadataAPIStringFromMap(t *testing.T) {
	tests := []struct {
		name string
		m    map[string]string
		want string
	}{
		{name: "nil", m: nil, want: ""},
		{name: "empty", m: map[string]string{}, want: ""},
		{name: "single key", m: map[string]string{"owned-by": "ngrok-operator"}, want: `{"owned-by":"ngrok-operator"}`},
		{name: "keys sorted", m: map[string]string{"zeta": "1", "alpha": "2"}, want: `{"alpha":"2","zeta":"1"}`},
		{name: "value with quotes", m: map[string]string{"a": `he said "hi"`}, want: `{"a":"he said \"hi\""}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := MetadataAPIStringFromMap(tt.m); got != tt.want {
				t.Errorf("MetadataAPIStringFromMap(%v) = %q, want %q", tt.m, got, tt.want)
			}
		})
	}
}

func TestMetadataMapFromJSON(t *testing.T) {
	tests := []struct {
		name    string
		in      string
		want    map[string]string
		wantErr bool
	}{
		{name: "empty string is unset", in: "", want: nil},
		{name: "flat object", in: `{"a":"1","b":"2"}`, want: map[string]string{"a": "1", "b": "2"}},
		{name: "empty object", in: `{}`, want: map[string]string{}},
		{name: "not json", in: "not-json", wantErr: true},
		{name: "nested object", in: `{"a":{"b":"c"}}`, wantErr: true},
		{name: "non-string value", in: `{"a":1}`, wantErr: true},
		{name: "null value", in: `{"a":null}`, wantErr: true},
		{name: "array", in: `["a"]`, wantErr: true},
		{name: "json null", in: `null`, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := MetadataMapFromJSON(tt.in)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("MetadataMapFromJSON(%q) = %v, want error", tt.in, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("MetadataMapFromJSON(%q) returned unexpected error: %v", tt.in, err)
			}
			if len(got) != len(tt.want) {
				t.Fatalf("MetadataMapFromJSON(%q) = %v, want %v", tt.in, got, tt.want)
			}
			for k, v := range tt.want {
				if got[k] != v {
					t.Errorf("MetadataMapFromJSON(%q)[%q] = %q, want %q", tt.in, k, got[k], v)
				}
			}
		})
	}
}

func TestMetadataFromMap(t *testing.T) {
	// Empty inputs return nil so the field is omitted and the CRD default
	// applies, rather than sending an empty value to ngrok.
	if got := MetadataFromMap(nil); got != nil {
		t.Errorf("MetadataFromMap(nil) = %v, want nil", got)
	}
	if got := MetadataFromMap(map[string]string{}); got != nil {
		t.Errorf("MetadataFromMap(empty) = %v, want nil", got)
	}
	if got := string(MetadataFromMap(map[string]string{"owned-by": "ngrok-operator"})); got != `{"owned-by":"ngrok-operator"}` {
		t.Errorf("MetadataFromMap = %q", got)
	}
}

// The CRD-level default is expressed as a kubebuilder marker on every
// spec.metadata field; the write paths fall back to this map so a defaulted
// object and an operator-written object compare equal.
func TestDefaultMetadataMap(t *testing.T) {
	if got := MetadataAPIStringFromMap(DefaultMetadataMap()); got != `{"owned-by":"ngrok-operator"}` {
		t.Errorf("DefaultMetadataMap = %q", got)
	}
}
