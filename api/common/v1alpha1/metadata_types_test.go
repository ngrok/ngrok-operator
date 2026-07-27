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

func TestMetadataConstructors(t *testing.T) {
	if got := MetadataAPIString(MetadataFromMap(map[string]string{"owned-by": "ngrok-operator"})); got != `{"owned-by":"ngrok-operator"}` {
		t.Errorf("MetadataFromMap = %q", got)
	}
	// legacy constructor passes the string through verbatim
	if got := MetadataAPIString(MetadataFromLegacyString(`{"owned-by":"ngrok-operator"}`)); got != `{"owned-by":"ngrok-operator"}` {
		t.Errorf("MetadataFromLegacyString(json) = %q", got)
	}
	if got := MetadataAPIString(MetadataFromLegacyString("free text")); got != "free text" {
		t.Errorf("MetadataFromLegacyString(non-json) = %q", got)
	}
}
