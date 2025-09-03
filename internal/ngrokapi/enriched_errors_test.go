package ngrokapi

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSanitizeErrorMessage(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "normal message no change",
			input:    "Invalid policy action type",
			expected: "Invalid policy action type",
		},
		{
			name:     "windows line endings",
			input:    "Invalid policy action type 'rate-limit-fake'.\r\n\r\nERR_NGROK_2201\r\n",
			expected: "Invalid policy action type 'rate-limit-fake'. ERR_NGROK_2201",
		},
		{
			name:     "unix line endings",
			input:    "Error occurred\nLine 2\nLine 3",
			expected: "Error occurred Line 2 Line 3",
		},
		{
			name:     "mixed line endings",
			input:    "Error\r\nSecond line\nThird line\r",
			expected: "Error Second line Third line",
		},
		{
			name:     "extra whitespace",
			input:    "Error    with   extra     spaces",
			expected: "Error with extra spaces",
		},
		{
			name:     "leading and trailing whitespace",
			input:    "   Error message   ",
			expected: "Error message",
		},
		{
			name:     "complex real ngrok error",
			input:    "The endpoint 'https://example.ngrok.app' is already online.\r\nEither\r\n1. stop your existing endpoint first, or\r\n2. start both endpoints with `--pooling-enabled`\r\n\r\nERR_NGROK_334\r\n",
			expected: "The endpoint 'https://example.ngrok.app' is already online. Either 1. stop your existing endpoint first, or 2. start both endpoints with `--pooling-enabled` ERR_NGROK_334",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := SanitizeErrorMessage(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestIsTrafficPolicyError(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected bool
	}{
		{
			name:     "contains policy keyword",
			input:    "Invalid policy action type 'rate-limit-fake'",
			expected: true,
		},
		{
			name:     "contains ERR_NGROK_2201",
			input:    "Some error message ERR_NGROK_2201",
			expected: true,
		},
		{
			name:     "real traffic policy error",
			input:    "Invalid policy action type 'rate-limit-fake'.\r\n\r\nERR_NGROK_2201\r\n",
			expected: true,
		},
		{
			name:     "normal ngrok error",
			input:    "The endpoint is already online ERR_NGROK_334",
			expected: false,
		},
		{
			name:     "generic error message",
			input:    "Connection failed",
			expected: false,
		},
		{
			name:     "empty string",
			input:    "",
			expected: false,
		},
		{
			name:     "capital Policy keyword",
			input:    "Invalid Policy action type",
			expected: false, // "Policy" != "policy"
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := IsTrafficPolicyError(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}
