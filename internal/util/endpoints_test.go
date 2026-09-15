package util

import (
	"net/url"
	"testing"
)

func TestIsInternalDomain(t *testing.T) {
	tests := []struct {
		name     string
		host     string
		expected bool
	}{
		{"simple internal domain", "foo.internal", true},
		{"subdomain internal", "bar.foo.internal", true},
		{"uppercase internal", "FOO.INTERNAL", true},
		{"mixed case internal", "Foo.Internal", true},
		{"trailing dot internal", "foo.internal.", true},
		{"with spaces", "  foo.internal  ", true},
		{"internal as subdomain - not internal TLD", "foo.internal.example.com", false},
		{"regular domain", "example.com", false},
		{"ngrok domain", "app.ngrok.io", false},
		{"empty string", "", false},
		{"just internal", "internal", false},
		{"dot internal only", ".internal", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := IsInternalDomain(tt.host)
			if result != tt.expected {
				t.Errorf("IsInternalDomain(%q) = %v, want %v", tt.host, result, tt.expected)
			}
		})
	}
}

func TestParseAndSanitizeEndpointURL(t *testing.T) {
	successCases := []struct {
		name         string
		input        string
		isIngressURL bool
		expected     *url.URL
	}{
		{
			"Port shorthand",
			"8080",
			false,
			&url.URL{Scheme: "http", Host: "localhost:8080"},
		},
		{
			"Shorthand with colon",
			"service.default:8080",
			false,
			&url.URL{Scheme: "http", Host: "service.default:8080"},
		},
		{
			"HTTP shorthand scheme",
			"http://",
			false, &url.URL{Scheme: "http", Host: "localhost:80"},
		},
		{
			"HTTPS shorthand scheme",
			"https://",
			false,
			&url.URL{Scheme: "https", Host: "localhost:443"},
		},
		{
			"Domain shorthand",
			"example.com",
			false,
			&url.URL{Scheme: "http", Host: "example.com:80"},
		},
		{
			"Domain shorthand with port",
			"example.com:8080",
			false,
			&url.URL{Scheme: "http", Host: "example.com:8080"},
		},
		{
			"HTTP without port",
			"http://example.com",
			false,
			&url.URL{Scheme: "http", Host: "example.com:80"},
		},
		{
			"HTTPS without port",
			"https://example.com",
			false,
			&url.URL{Scheme: "https", Host: "example.com:443"},
		},
		{
			"TLS ingress with 443 port",
			"tls://example.com:443",
			true,
			&url.URL{Scheme: "tls", Host: "example.com:443"},
		},
		{
			"TLS non-ingress URL",
			"tls://example.com:8443",
			false,
			&url.URL{Scheme: "tls", Host: "example.com:8443"},
		},
		{
			"Internal endpoint",
			"https://example.internal",
			false,
			&url.URL{Scheme: "https", Host: "example.internal:443"},
		},
	}

	errorCases := []struct {
		name         string
		input        string
		isIngressURL bool
		expectedErr  string
	}{
		{
			"Invalid TCP scheme",
			"tcp://",
			false,
			`invalid URL for scheme shorthand format ("tcp://"): "tcp://" and "tls://" must provide a hostname`,
		},
		{
			"Unsupported scheme",
			"custom://service",
			false,
			`unsupported scheme for URL ("custom://service"): "custom"`,
		},
		{
			"TCP missing port",
			"tcp://example.com",
			false,
			`invalid URL ("tcp://example.com"), tcp schemes require a port and a hostname`,
		},
		{
			"TLS ingress with non-443 port",
			"tls://example.com:8443",
			true,
			`invalid url "tls://example.com:8443", tls:// scheme ingress urls only support port 443 for accepting incoming traffic`,
		},
		{
			"Invalid URL with empty hostname",
			"http://:8080",
			false,
			`invalid URL ("http://:8080"), shorthand format not detected and URL is missing a hostname`,
		},
	}

	t.Run("Success cases", func(t *testing.T) {
		for _, tt := range successCases {
			t.Run(tt.name, func(t *testing.T) {
				result, err := ParseAndSanitizeEndpointURL(tt.input, tt.isIngressURL)
				if err != nil {
					t.Errorf("Unexpected error for input %q: %v", tt.input, err)
					return
				}
				if result.String() != tt.expected.String() {
					t.Errorf("Expected URL %q, got %q", tt.expected, result)
				}
			})
		}
	})

	t.Run("Error cases", func(t *testing.T) {
		for _, tt := range errorCases {
			t.Run(tt.name, func(t *testing.T) {
				_, err := ParseAndSanitizeEndpointURL(tt.input, tt.isIngressURL)
				if err == nil {
					t.Errorf("Expected error for input %q, but got none", tt.input)
					return
				}
				if err.Error() != tt.expectedErr {
					t.Errorf("Expected error message %q, but got %q", tt.expectedErr, err.Error())
				}
			})
		}
	})
}

func TestIsWildcardDomain(t *testing.T) {
	tests := []struct {
		host     string
		expected bool
	}{
		{"*.example.com", true},
		{"*.EXAMPLE.com.", true},
		{"  *.example.com  ", true},
		{"example.com", false},
		{"a.example.com", false},
		{"", false},
		// A "*" that is not the leading label is not a wildcard hostname.
		{"a.*.example.com", false},
	}

	for _, tt := range tests {
		t.Run(tt.host, func(t *testing.T) {
			if result := IsWildcardDomain(tt.host); result != tt.expected {
				t.Errorf("IsWildcardDomain(%q) = %v, want %v", tt.host, result, tt.expected)
			}
		})
	}
}

func TestWildcardParentDomain(t *testing.T) {
	tests := []struct {
		name       string
		host       string
		wantParent string
		wantOK     bool
	}{
		{
			name:       "direct child",
			host:       "a.mydomain.com",
			wantParent: "*.mydomain.com",
			wantOK:     true,
		},
		{
			// DNS wildcards match exactly one label, so *.mydomain.com does NOT
			// cover a.b.mydomain.com - only *.b.mydomain.com does.
			name:       "grandchild resolves to its direct parent only",
			host:       "a.b.mydomain.com",
			wantParent: "*.b.mydomain.com",
			wantOK:     true,
		},
		{
			name:       "deeply nested child",
			host:       "a.b.c.mydomain.com",
			wantParent: "*.b.c.mydomain.com",
			wantOK:     true,
		},
		{
			name:       "ngrok owned subdomain",
			host:       "a.mytest.ngrok.io",
			wantParent: "*.mytest.ngrok.io",
			wantOK:     true,
		},
		{
			name:   "apex is not covered by its own wildcard",
			host:   "mydomain.com",
			wantOK: false,
		},
		{
			name:   "already a wildcard",
			host:   "*.mydomain.com",
			wantOK: false,
		},
		{
			name:   "single label",
			host:   "localhost",
			wantOK: false,
		},
		{
			name:   "empty",
			host:   "",
			wantOK: false,
		},
		{
			name:       "normalizes case, whitespace and trailing dot",
			host:       "  A.MyDomain.CoM.  ",
			wantParent: "*.mydomain.com",
			wantOK:     true,
		},
		{
			name:   "empty label",
			host:   "a..com",
			wantOK: false,
		},
		{
			name:   "embedded wildcard label",
			host:   "a.*.com",
			wantOK: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			parent, ok := WildcardParentDomain(tt.host)
			if ok != tt.wantOK {
				t.Fatalf("WildcardParentDomain(%q) ok = %v, want %v", tt.host, ok, tt.wantOK)
			}
			if parent != tt.wantParent {
				t.Errorf("WildcardParentDomain(%q) = %q, want %q", tt.host, parent, tt.wantParent)
			}
		})
	}
}
