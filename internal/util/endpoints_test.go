package util

import (
	"net/url"
	"testing"
)

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

func TestParseEndpointHostPort(t *testing.T) {
	successCases := []struct {
		name         string
		input        string
		wantHostname string
		wantPort     int32
	}{
		{
			name:         "TLS URL with port",
			input:        "tls://example.ngrok.app:443",
			wantHostname: "example.ngrok.app",
			wantPort:     443,
		},
		{
			name:         "TLS URL without port (infers 443)",
			input:        "tls://example.ngrok.app",
			wantHostname: "example.ngrok.app",
			wantPort:     443,
		},
		{
			name:         "HTTPS URL",
			input:        "https://example.com",
			wantHostname: "example.com",
			wantPort:     443,
		},
		{
			name:         "HTTP URL with port",
			input:        "http://example.com:8080",
			wantHostname: "example.com",
			wantPort:     8080,
		},
		{
			name:         "TCP URL with port",
			input:        "tcp://1.tcp.ngrok.io:12345",
			wantHostname: "1.tcp.ngrok.io",
			wantPort:     12345,
		},
		{
			name:         "Domain shorthand",
			input:        "example.com",
			wantHostname: "example.com",
			wantPort:     80,
		},
	}

	errorCases := []struct {
		name  string
		input string
	}{
		{
			name:  "TCP scheme without hostname",
			input: "tcp://",
		},
		{
			name:  "TLS scheme without hostname",
			input: "tls://",
		},
		{
			name:  "TCP scheme without port",
			input: "tcp://example.com",
		},
		{
			name:  "Unsupported scheme",
			input: "ftp://example.com",
		},
	}

	t.Run("Success cases", func(t *testing.T) {
		for _, tt := range successCases {
			t.Run(tt.name, func(t *testing.T) {
				hostname, port, err := ParseEndpointHostPort(tt.input)
				if err != nil {
					t.Errorf("Unexpected error for input %q: %v", tt.input, err)
					return
				}
				if hostname != tt.wantHostname {
					t.Errorf("Expected hostname %q, got %q", tt.wantHostname, hostname)
				}
				if port != tt.wantPort {
					t.Errorf("Expected port %d, got %d", tt.wantPort, port)
				}
			})
		}
	})

	t.Run("Error cases", func(t *testing.T) {
		for _, tt := range errorCases {
			t.Run(tt.name, func(t *testing.T) {
				_, _, err := ParseEndpointHostPort(tt.input)
				if err == nil {
					t.Errorf("Expected error for input %q, but got none", tt.input)
				}
			})
		}
	})
}
