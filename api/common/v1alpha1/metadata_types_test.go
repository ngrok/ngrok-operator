package common

import (
	"encoding/json"
	"reflect"
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
	// Empty inputs return nil so the field is omitted and the CRD default applies
	// (rather than sending an empty value to ngrok).
	if got := MetadataFromLegacyString(""); got != nil {
		t.Errorf("MetadataFromLegacyString(\"\") = %v, want nil", got)
	}
	if got := MetadataFromMap(nil); got != nil {
		t.Errorf("MetadataFromMap(nil) = %v, want nil", got)
	}
	if got := MetadataFromMap(map[string]string{}); got != nil {
		t.Errorf("MetadataFromMap(empty) = %v, want nil", got)
	}
}

func TestMetadataToMap(t *testing.T) {
	tests := []struct {
		name string
		json string // the raw JSON value of the metadata field
		want map[string]string
	}{
		{name: "empty input", json: ``, want: nil},
		{name: "whitespace only", json: "  \n\t", want: nil},
		{name: "unset", json: `null`, want: nil},
		{name: "empty object", json: `{}`, want: map[string]string{}},
		{
			name: "object form single key",
			json: `{"owned-by":"ngrok-operator"}`,
			want: map[string]string{"owned-by": "ngrok-operator"},
		},
		{
			name: "object form multiple keys",
			json: `{"zeta":"1","alpha":"2"}`,
			want: map[string]string{"zeta": "1", "alpha": "2"},
		},
		{
			name: "object form surrounding whitespace",
			json: "  {\"a\":\"b\"}\n",
			want: map[string]string{"a": "b"},
		},
		{
			name: "legacy json-object string unwrapped",
			json: `"{\"owned-by\":\"ngrok-operator\"}"`,
			want: map[string]string{"owned-by": "ngrok-operator"},
		},
		{
			name: "legacy json-object string with inner whitespace",
			json: `"  {\"a\":\"b\"}  "`,
			want: map[string]string{"a": "b"},
		},
		{name: "legacy empty string", json: `""`, want: nil},
		{name: "legacy whitespace-only string", json: `"   "`, want: nil},
		{name: "legacy non-json string", json: `"some free text"`, want: nil},
		{name: "legacy string holding non-flat object", json: `"{\"a\":1}"`, want: nil},
		{name: "invalid json", json: `{"a":`, want: nil},
		{name: "malformed legacy string", json: `"unterminated`, want: nil},
		{name: "nested object not flat", json: `{"a":{"b":"c"}}`, want: nil},
		{name: "numeric value not a string", json: `{"a":1}`, want: nil},
		{name: "array not an object", json: `["a","b"]`, want: nil},
		{name: "bare number", json: `42`, want: nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := MetadataToMap(json.RawMessage(tt.json)); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("MetadataToMap(%q) = %#v, want %#v", tt.json, got, tt.want)
			}
		})
	}
	// nil is unset
	if got := MetadataToMap(nil); got != nil {
		t.Errorf("MetadataToMap(nil) = %#v, want nil", got)
	}
}

func TestMetadataToMapRoundTrip(t *testing.T) {
	in := map[string]string{"owned-by": "ngrok-operator", "team": "edge"}
	if got := MetadataToMap(MetadataFromMap(in)); !reflect.DeepEqual(got, in) {
		t.Errorf("MetadataToMap(MetadataFromMap(%v)) = %v", in, got)
	}
	if got := MetadataToMap(MetadataFromLegacyString(`{"owned-by":"ngrok-operator"}`)); !reflect.DeepEqual(got, map[string]string{"owned-by": "ngrok-operator"}) {
		t.Errorf("MetadataToMap(MetadataFromLegacyString(json)) = %v", got)
	}
}
