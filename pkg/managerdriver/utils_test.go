package managerdriver

import (
	"testing"

	"github.com/go-logr/logr"
	"github.com/ngrok/ngrok-operator/internal/ir"
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
		name                   string
		serviceName            string
		namespace              string
		clusterDomain          string
		port                   int32
		serviceUID             string
		expectedResult         string
		upstreamClientCertRefs []ir.IRObjectRef
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
		{
			name:           "Client cert refs",
			serviceName:    "svc",
			namespace:      "ns",
			clusterDomain:  "",
			port:           8081,
			serviceUID:     "123456",
			expectedResult: "https://8d969-svc-ns-mtls-d025c-8081.internal",
			upstreamClientCertRefs: []ir.IRObjectRef{{
				Name:      "client-cert-secret",
				Namespace: "secrets",
			}},
		},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			result := internalAgentEndpointURL(tc.serviceUID, tc.serviceName, tc.namespace, tc.clusterDomain, tc.port, tc.upstreamClientCertRefs)
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

func TestNetv1PathTypeToIR(t *testing.T) {
	// We'll use an "unknown" value for testing the default branch
	unknown := netv1.PathType("Unknown")

	newPathType := func(v netv1.PathType) *netv1.PathType {
		return &v
	}

	testCases := []struct {
		name           string
		pathType       *netv1.PathType
		expected       ir.IRPathMatchType
		expectLogError bool
	}{
		{
			name:     "nil pathType returns Prefix",
			pathType: nil,
			expected: ir.IRPathType_Prefix,
		},
		{
			name:     "PathTypePrefix returns Prefix",
			pathType: newPathType(netv1.PathTypePrefix),
			expected: ir.IRPathType_Prefix,
		},
		{
			name:     "PathTypeImplementationSpecific returns Prefix",
			pathType: newPathType(netv1.PathTypeImplementationSpecific),
			expected: ir.IRPathType_Prefix,
		},
		{
			name:     "PathTypeExact returns Exact",
			pathType: newPathType(netv1.PathTypeExact),
			expected: ir.IRPathType_Exact,
		},
		{
			name:     "Unknown path type logs error and returns Prefix",
			pathType: newPathType(unknown),
			expected: ir.IRPathType_Prefix,
		},
	}

	for _, tc := range testCases {
		tc := tc // capture range variable
		t.Run(tc.name, func(t *testing.T) {
			logger := logr.New(logr.Discard().GetSink())
			result := netv1PathTypeToIR(logger, tc.pathType)
			assert.Equal(t, tc.expected, result, "unexpected IR path match type for test case: %s", tc.name)
		})
	}
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

func TestDoHostGlobsMatch(t *testing.T) {
	testCases := []struct {
		name        string
		hostname1   string
		hostname2   string
		expected    bool
		expectedErr string
	}{
		{
			name:      "Both non-glob equal",
			hostname1: "example.com",
			hostname2: "example.com",
			expected:  true,
		},
		{
			name:      "Both non-glob not equal",
			hostname1: "example.com",
			hostname2: "example.org",
			expected:  false,
		},
		{
			name:      "Hostname1 is glob, match",
			hostname1: "*.example.com",
			hostname2: "foo.example.com",
			expected:  true,
		},
		{
			name:      "Hostname1 is glob, no match",
			hostname1: "*.example.com",
			hostname2: "example.com",
			expected:  false,
		},
		{
			name:      "Hostname2 is glob, match",
			hostname1: "foo.example.com",
			hostname2: "*.example.com",
			expected:  true,
		},
		{
			name:      "Hostname2 is glob, no match",
			hostname1: "example.com",
			hostname2: "*.example.com",
			expected:  false,
		},
		{
			name:        "Both globs, match (hostname1 wins)",
			hostname1:   "*.example.com",
			hostname2:   "bar.example.com",
			expected:    true,
			expectedErr: "",
		},
		{
			name:      "Both globs, no match (hostname1 wins)",
			hostname1: "*.example.com",
			hostname2: "example.com",
			expected:  false,
		},
		{
			name:        "Invalid glob in hostname1",
			hostname1:   "foo[bar*",
			hostname2:   "foobar",
			expected:    false,
			expectedErr: "unexpected end of input",
		},
		{
			name:        "Invalid glob in hostname2",
			hostname1:   "foobar",
			hostname2:   "foo[bar*",
			expected:    false,
			expectedErr: "unexpected end of input",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result, err := doHostGlobsMatch(tc.hostname1, tc.hostname2)
			if tc.expectedErr != "" {
				assert.Error(t, err, "expected an error for test case: %s", tc.name)
				if err != nil {
					assert.Contains(t, err.Error(), tc.expectedErr, "error message mismatch for test case: %s", tc.name)
				}
			} else {
				assert.NoError(t, err, "did not expect an error for test case: %s", tc.name)
				assert.Equal(t, tc.expected, result, "unexpected result for test case: %s", tc.name)
			}
		})
	}
}
