package managerdriver

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestHasDefaultManagedResourceLabels(t *testing.T) {
	testCases := []struct {
		name             string
		labels           map[string]string
		managerName      string
		managerNamespace string
		expected         bool
	}{
		{
			name:             "no labels returns false",
			labels:           map[string]string{},
			managerName:      "manager",
			managerNamespace: "default",
			expected:         false,
		},
		{
			name: "missing namespace label returns false",
			labels: map[string]string{
				labelControllerName: "manager",
			},
			managerName:      "manager",
			managerNamespace: "default",
			expected:         false,
		},
		{
			name: "wrong namespace returns false",
			labels: map[string]string{
				labelControllerNamespace: "other",
				labelControllerName:      "manager",
			},
			managerName:      "manager",
			managerNamespace: "default",
			expected:         false,
		},
		{
			name: "missing name label returns false",
			labels: map[string]string{
				labelControllerNamespace: "default",
			},
			managerName:      "manager",
			managerNamespace: "default",
			expected:         false,
		},
		{
			name: "wrong manager name returns false",
			labels: map[string]string{
				labelControllerNamespace: "default",
				labelControllerName:      "not-manager",
			},
			managerName:      "manager",
			managerNamespace: "default",
			expected:         false,
		},
		{
			name: "correct labels returns true",
			labels: map[string]string{
				labelControllerNamespace: "default",
				labelControllerName:      "manager",
			},
			managerName:      "manager",
			managerNamespace: "default",
			expected:         true,
		},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			actual := hasDefaultManagedResourceLabels(tc.labels, tc.managerName, tc.managerNamespace)
			assert.Equal(t, tc.expected, actual)
		})
	}
}

func TestSanitizeStringForURL(t *testing.T) {
	testCases := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "simple alphanumeric",
			input:    "MyService123",
			expected: "MyService123",
		},
		{
			name:     "replace asterisk with wildcard",
			input:    "my*service",
			expected: "mywildcardservice",
		},
		{
			name:     "invalid chars replaced by dash",
			input:    "my@service!",
			expected: "my-service-",
		},
		{
			name:     "only invalid chars",
			input:    "@@@!!!",
			expected: "------",
		},
		{
			name:     "contains multiple asterisks",
			input:    "*my*service*",
			expected: "wildcardmywildcardservicewildcard",
		},
		{
			name:     "contains allowed URL chars",
			input:    "my.service_name~test",
			expected: "my.service_name~test",
		},
		{
			name:     "mixed invalid chars and allowed chars",
			input:    "SeRvIcE$123~ok",
			expected: "SeRvIcE-123~ok",
		},
		{
			name:     "string with multiple invalid chars and asterisks",
			input:    "!!!Service**Name???",
			expected: "---ServicewildcardwildcardName---",
		},
		{
			name:     "empty string",
			input:    "",
			expected: "",
		},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			actual := sanitizeStringForURL(tc.input)
			assert.Equal(t, tc.expected, actual)
		})
	}
}

func TestInternalAgentEndpointURL(t *testing.T) {
	testCases := []struct {
		name           string
		serviceName    string
		namespace      string
		clusterDomain  string
		port           int32
		expectedResult string
	}{
		{
			name:           "Normal case",
			serviceName:    "service",
			namespace:      "default",
			clusterDomain:  "cluster.local",
			port:           443,
			expectedResult: "https://service-default-cluster-local-443.internal",
		},
		{
			name:           "Handles invalid characters in input",
			serviceName:    "service-*",
			namespace:      "namespace-*",
			clusterDomain:  "domain-*",
			port:           8080,
			expectedResult: "https://service-wildcard-namespace-wildcard-domain-wildcard-8080.internal",
		},
		{
			name:           "Replaces dots in cluster domain",
			serviceName:    "svc",
			namespace:      "ns",
			clusterDomain:  "example.com",
			port:           8081,
			expectedResult: "https://svc-ns-example-com-8081.internal",
		},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			result := internalAgentEndpointURL(tc.serviceName, tc.namespace, tc.clusterDomain, tc.port)
			if result != tc.expectedResult {
				t.Errorf("expected %s, got %s", tc.expectedResult, result)
			}
		})
	}
}

func TestSanitizeStringForK8sName(t *testing.T) {
	tests := []struct {
		name           string
		input          string
		expectedResult string
	}{
		{
			name:           "Invalid characters",
			input:          "invalid*chars!",
			expectedResult: "invalidwildcardchars",
		},
		{
			name:           "Exceeds max length with hash",
			input:          "a-very-long-name-that-exceeds-the-kubernetes-name-length-limit-and-needs-truncation",
			expectedResult: "a-very-long-name-that-exceeds-the-kubernetes-name-leng-6282807c",
		},
		{
			name:           "Only invalid characters",
			input:          "!@#$%^&()",
			expectedResult: "default",
		},
		{
			name:           "Invalid characters with truncation and hash",
			input:          "this-name-has-invalid-characters-like-*and-needs-truncation-due-to-length",
			expectedResult: "this-name-has-invalid-characters-like-wildcardand-need-51d3964b",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := sanitizeStringForK8sName(tt.input)
			if result != tt.expectedResult {
				t.Errorf("expected %s, got %s", tt.expectedResult, result)
			}
		})
	}
}
