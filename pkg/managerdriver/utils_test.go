package managerdriver

import (
	"testing"

	"github.com/go-logr/logr"
	"github.com/stretchr/testify/assert"
	netv1 "k8s.io/api/networking/v1"
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
		serviceUID     string
		expectedResult string
	}{
		{
			name:           "Normal case",
			serviceName:    "service",
			namespace:      "default",
			clusterDomain:  "cluster.local",
			port:           443,
			serviceUID:     "1234",
			expectedResult: "https://03ac6-service-default-cluster-local-443.internal",
		},
		{
			name:           "Handles invalid characters in input",
			serviceName:    "service-*",
			namespace:      "namespace-*",
			clusterDomain:  "domain-*",
			port:           8080,
			serviceUID:     "5678",
			expectedResult: "https://f8638-service-wildcard-namespace-wildcard-domain-wildcard-8080.internal",
		},
		{
			name:           "Replaces dots in cluster domain",
			serviceName:    "svc",
			namespace:      "ns",
			clusterDomain:  "example.com",
			port:           8081,
			serviceUID:     "12X3D5U07876F12J3",
			expectedResult: "https://08ad9-svc-ns-example-com-8081.internal",
		},
		{
			name:           "Empty cluster domain",
			serviceName:    "svc",
			namespace:      "ns",
			clusterDomain:  "",
			port:           8081,
			serviceUID:     "123456",
			expectedResult: "https://8d969-svc-ns-8081.internal",
		},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			result := internalAgentEndpointURL(tc.serviceUID, tc.serviceName, tc.namespace, tc.clusterDomain, tc.port)
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

func TestGetPathMatchType(t *testing.T) {
	driver := &Driver{log: logr.New(logr.Discard().GetSink())}

	// Define a custom unknown path type
	customPathType := netv1.PathType("custom")

	testCases := []struct {
		name     string
		input    *netv1.PathType
		expected netv1.PathType
	}{
		{
			name:     "nil pathType defaults to prefix",
			input:    nil,
			expected: netv1.PathTypePrefix,
		},
		{
			name:     "PathTypePrefix returns prefix",
			input:    ptrToPathType(netv1.PathTypePrefix),
			expected: netv1.PathTypePrefix,
		},
		{
			name:     "PathTypeImplementationSpecific returns prefix",
			input:    ptrToPathType(netv1.PathTypeImplementationSpecific),
			expected: netv1.PathTypePrefix,
		},
		{
			name:     "PathTypeExact returns exact",
			input:    ptrToPathType(netv1.PathTypeExact),
			expected: netv1.PathTypeExact,
		},
		{
			name:     "Unknown pathType logs error and defaults to prefix",
			input:    &customPathType,
			expected: netv1.PathTypePrefix,
		},
	}

	for _, tc := range testCases {
		tc := tc // capture range variable
		t.Run(tc.name, func(t *testing.T) {
			actual := getPathMatchType(driver.log, tc.input)
			assert.Equal(t, tc.expected, actual)
		})
	}
}

// ptrToPathType is a helper to get a pointer to a PathType.
func ptrToPathType(pt netv1.PathType) *netv1.PathType {
	return &pt
}

func TestAppendStringUnique(t *testing.T) {
	tests := []struct {
		name           string
		existing       []string
		newItem        string
		expectedResult []string
	}{
		{
			name:           "Add new item to empty slice",
			existing:       []string{},
			newItem:        "apple",
			expectedResult: []string{"apple"},
		},
		{
			name:           "Add new unique item to non-empty slice",
			existing:       []string{"apple", "banana"},
			newItem:        "cherry",
			expectedResult: []string{"apple", "banana", "cherry"},
		},
		{
			name:           "Do not add duplicate item",
			existing:       []string{"apple", "banana"},
			newItem:        "banana",
			expectedResult: []string{"apple", "banana"},
		},
		{
			name:           "Case-sensitive unique check",
			existing:       []string{"apple", "banana"},
			newItem:        "Apple", // Different casing
			expectedResult: []string{"apple", "banana", "Apple"},
		},
		{
			name:           "Handle slice with one item",
			existing:       []string{"apple"},
			newItem:        "banana",
			expectedResult: []string{"apple", "banana"},
		},
		{
			name:           "Handle slice with duplicates (no-op for duplicates)",
			existing:       []string{"apple", "apple", "banana"},
			newItem:        "apple",
			expectedResult: []string{"apple", "apple", "banana"}, // No changes
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := appendStringUnique(tt.existing, tt.newItem)
			assert.Equal(t, tt.expectedResult, result, "unexpected result for test case: %s", tt.name)
		})
	}
}
