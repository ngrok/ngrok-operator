package common

import (
	"testing"
)

func TestMetadataAPIString(t *testing.T) {
	tests := []struct {
		name string
		m    map[string]string
		want string
	}{
		{name: "unset", m: nil, want: ""},
		{name: "empty map", m: map[string]string{}, want: ""},
		{
			name: "single key",
			m:    map[string]string{"owned-by": "ngrok-operator"},
			want: `{"owned-by":"ngrok-operator"}`,
		},
		{
			name: "keys sorted for stable output",
			m:    map[string]string{"zeta": "1", "alpha": "2"},
			want: `{"alpha":"2","zeta":"1"}`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := MetadataAPIString(tt.m); got != tt.want {
				t.Errorf("MetadataAPIString(%v) = %q, want %q", tt.m, got, tt.want)
			}
		})
	}
}

func TestMetadataMapFromJSON(t *testing.T) {
	tests := []struct {
		name string
		json string
		want map[string]string
	}{
		{name: "empty string", json: "", want: nil},
		{name: "invalid json", json: "not json", want: nil},
		{name: "non-flat json", json: `{"a":{"b":"c"}}`, want: nil},
		{
			name: "flat object",
			json: `{"owned-by":"ngrok-operator"}`,
			want: map[string]string{"owned-by": "ngrok-operator"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := MetadataMapFromJSON(tt.json)
			if len(got) != len(tt.want) {
				t.Fatalf("MetadataMapFromJSON(%s) = %v, want %v", tt.json, got, tt.want)
			}
			for k, v := range tt.want {
				if got[k] != v {
					t.Errorf("MetadataMapFromJSON(%s)[%s] = %q, want %q", tt.json, k, got[k], v)
				}
			}
		})
	}
}
