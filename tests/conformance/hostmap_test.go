package conformance

import "testing"

func TestHostMapper(t *testing.T) {
	m := hostMapper{suffix: "conf.example.test"}
	tests := []struct {
		name     string
		in       string
		mapped   string
		unmapped string
	}{
		{"empty stays empty", "", "", ""},
		{"plain host", "example.com", "example.com.conf.example.test", "example.com"},
		{"host with port", "example.com:80", "example.com.conf.example.test:80", "example.com:80"},
		{"already mapped", "example.com.conf.example.test", "example.com.conf.example.test", "example.com"},
		{"IP address untouched", "10.0.0.1:80", "10.0.0.1:80", "10.0.0.1:80"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := m.mapHost(tt.in)
			if got != tt.mapped {
				t.Fatalf("mapHost(%q) = %q, want %q", tt.in, got, tt.mapped)
			}
			if back := m.unmapHost(got); back != tt.unmapped {
				t.Fatalf("unmapHost(%q) = %q, want %q", got, back, tt.unmapped)
			}
		})
	}
}
