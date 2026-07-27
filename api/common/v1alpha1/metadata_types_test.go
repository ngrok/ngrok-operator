package common

import (
	"encoding/json"
	"reflect"
	"testing"
)

func TestMetadataValue_APIString(t *testing.T) {
	tests := []struct {
		name string
		json string // the raw JSON value of the metadata field
		want string
	}{
		{name: "unset", json: `null`, want: ""},
		{name: "empty object", json: `{}`, want: "{}"},
		{name: "legacy empty string", json: `""`, want: ""},
		{
			name: "legacy json-object string passthrough",
			json: `"{\"owned-by\":\"ngrok-operator\"}"`,
			want: `{"owned-by":"ngrok-operator"}`,
		},
		{
			name: "legacy non-json string passthrough",
			json: `"some free text"`,
			want: "some free text",
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
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var m MetadataValue
			if err := json.Unmarshal([]byte(tt.json), &m); err != nil {
				t.Fatalf("unmarshal: %v", err)
			}
			if got := m.APIString(); got != tt.want {
				t.Errorf("APIString() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestMetadataValue_NilAPIString(t *testing.T) {
	var m *MetadataValue
	if got := m.APIString(); got != "" {
		t.Errorf("nil APIString() = %q, want empty", got)
	}
	if m.IsLegacyString() {
		t.Errorf("nil IsLegacyString() = true, want false")
	}
}

func TestMetadataValue_IsLegacyString(t *testing.T) {
	tests := []struct {
		json string
		want bool
	}{
		{`"legacy"`, true},
		{`"{\"a\":\"b\"}"`, true},
		{`{"a":"b"}`, false},
		{`{}`, false},
		{`null`, false},
	}
	for _, tt := range tests {
		var m MetadataValue
		if err := json.Unmarshal([]byte(tt.json), &m); err != nil {
			t.Fatalf("unmarshal %s: %v", tt.json, err)
		}
		if got := m.IsLegacyString(); got != tt.want {
			t.Errorf("IsLegacyString(%s) = %v, want %v", tt.json, got, tt.want)
		}
	}
}

func TestMetadataValue_Constructors(t *testing.T) {
	if got := MetadataFromMap(map[string]string{"owned-by": "ngrok-operator"}).APIString(); got != `{"owned-by":"ngrok-operator"}` {
		t.Errorf("MetadataFromMap APIString = %q", got)
	}
	// legacy constructor passes the string through verbatim
	if got := MetadataFromLegacyString(`{"owned-by":"ngrok-operator"}`).APIString(); got != `{"owned-by":"ngrok-operator"}` {
		t.Errorf("MetadataFromLegacyString APIString = %q", got)
	}
	if !MetadataFromLegacyString("x").IsLegacyString() {
		t.Errorf("MetadataFromLegacyString should be legacy form")
	}
	if MetadataFromMap(map[string]string{"a": "b"}).IsLegacyString() {
		t.Errorf("MetadataFromMap should not be legacy form")
	}
}

func TestMetadataValue_RoundTrip(t *testing.T) {
	for _, in := range []string{`{"a":"b"}`, `"legacy"`, `null`} {
		var m MetadataValue
		if err := json.Unmarshal([]byte(in), &m); err != nil {
			t.Fatalf("unmarshal %s: %v", in, err)
		}
		out, err := json.Marshal(m)
		if err != nil {
			t.Fatalf("marshal %s: %v", in, err)
		}
		if string(out) != in {
			t.Errorf("round-trip %s = %s", in, out)
		}
	}
}

func TestMetadataValue_UsesDeprecatedStringForm(t *testing.T) {
	tests := []struct {
		json string
		want bool
	}{
		{`{"a":"b"}`, false}, // object form
		{`null`, false},      // unset
		{`"` + `{\"owned-by\":\"ngrok-operator\"}` + `"`, false}, // legacy but equals default -> not flagged
		{`"{\"owned-by\":\"someone\"}"`, true},                   // user-authored legacy string
		{`"free text"`, true},                                    // user-authored legacy string
	}
	for _, tt := range tests {
		var m MetadataValue
		if err := json.Unmarshal([]byte(tt.json), &m); err != nil {
			t.Fatalf("unmarshal %s: %v", tt.json, err)
		}
		if got := m.UsesDeprecatedStringForm(); got != tt.want {
			t.Errorf("UsesDeprecatedStringForm(%s) = %v, want %v", tt.json, got, tt.want)
		}
	}
	var nilM *MetadataValue
	if nilM.UsesDeprecatedStringForm() {
		t.Errorf("nil UsesDeprecatedStringForm() = true, want false")
	}
}

func TestMetadataValue_HasNonStringValues(t *testing.T) {
	tests := []struct {
		json string
		want bool
	}{
		{`{"a":"b"}`, false},      // flat string map
		{`{}`, false},             // empty object
		{`"legacy"`, false},       // legacy string form
		{`null`, false},           // unset
		{`{"a":{"b":"c"}}`, true}, // nested object
		{`{"count":5}`, true},     // non-string scalar
		{`{"a":"b","c":true}`, true},
	}
	for _, tt := range tests {
		var m MetadataValue
		if err := json.Unmarshal([]byte(tt.json), &m); err != nil {
			t.Fatalf("unmarshal %s: %v", tt.json, err)
		}
		if got := m.HasNonStringValues(); got != tt.want {
			t.Errorf("HasNonStringValues(%s) = %v, want %v", tt.json, got, tt.want)
		}
	}
}

func TestMetadataValue_Map(t *testing.T) {
	m := MetadataFromMap(map[string]string{"a": "b"})
	got, ok := m.Map()
	if !ok || !reflect.DeepEqual(got, map[string]string{"a": "b"}) {
		t.Errorf("Map() = %v, %v", got, ok)
	}
	if _, ok := MetadataFromLegacyString("x").Map(); ok {
		t.Errorf("legacy string Map() ok = true, want false")
	}
}

func TestMetadataValue_DeepCopy(t *testing.T) {
	m := MetadataFromMap(map[string]string{"a": "b"})
	c := m.DeepCopy()
	if c.APIString() != m.APIString() {
		t.Errorf("deepcopy mismatch")
	}
	// mutate original raw; copy must be unaffected
	m.raw[0] = 'X'
	if c.raw[0] == 'X' {
		t.Errorf("deepcopy aliased the raw slice")
	}
	var nilM *MetadataValue
	if nilM.DeepCopy() != nil {
		t.Errorf("nil DeepCopy should be nil")
	}
}
