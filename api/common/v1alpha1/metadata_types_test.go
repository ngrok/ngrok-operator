package common

import (
	"encoding/json"
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
}

func TestMetadataValue_Constructors(t *testing.T) {
	if got := MetadataFromMap(map[string]string{"owned-by": "ngrok-operator"}).APIString(); got != `{"owned-by":"ngrok-operator"}` {
		t.Errorf("MetadataFromMap APIString = %q", got)
	}
	// legacy constructor passes the string through verbatim
	if got := MetadataFromLegacyString(`{"owned-by":"ngrok-operator"}`).APIString(); got != `{"owned-by":"ngrok-operator"}` {
		t.Errorf("MetadataFromLegacyString APIString = %q", got)
	}
	// a non-JSON legacy string is passed through verbatim
	if got := MetadataFromLegacyString("free text").APIString(); got != "free text" {
		t.Errorf("MetadataFromLegacyString(non-json) APIString = %q", got)
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
