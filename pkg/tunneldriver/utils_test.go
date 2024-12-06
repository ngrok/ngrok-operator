package tunneldriver_test

import (
	"net/url"
	"testing"

	"github.com/ngrok/ngrok-operator/pkg/tunneldriver"
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
				result, err := tunneldriver.ParseAndSanitizeEndpointURL(tt.input, tt.isIngressURL)
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
				_, err := tunneldriver.ParseAndSanitizeEndpointURL(tt.input, tt.isIngressURL)
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
