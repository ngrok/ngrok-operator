package managerdriver

import (
	"testing"

	"github.com/go-logr/logr"
	"github.com/stretchr/testify/assert"
	netv1 "k8s.io/api/networking/v1"
)

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
			actual := driver.getPathMatchType(tc.input)
			assert.Equal(t, tc.expected, actual)
		})
	}
}

// ptrToPathType is a helper to get a pointer to a PathType.
func ptrToPathType(pt netv1.PathType) *netv1.PathType {
	return &pt
}

func TestBuildCloudEndpoint(t *testing.T) {
	testCases := []struct {
		name      string
		namespace string
		hostname  string
		labels    map[string]string
		metadata  string
	}{
		{
			name:      "simple case",
			namespace: "default",
			hostname:  "example.com",
			labels:    map[string]string{"app": "test"},
			metadata:  "some-metadata",
		},
		{
			name:      "invalid chars in hostname",
			namespace: "my-namespace",
			hostname:  "My*Resource!!!",
			labels:    map[string]string{"env": "prod"},
			metadata:  "meta-value",
		},
		{
			name:      "empty hostname",
			namespace: "empty-ns",
			hostname:  "",
			labels:    nil,
			metadata:  "",
		},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			ce := buildCloudEndpoint(tc.namespace, tc.hostname, tc.labels, tc.metadata)

			assert.NotNil(t, ce, "CloudEndpoint should not be nil")
			assert.Equal(t, tc.namespace, ce.Namespace, "namespace should match expected")
			assert.Equal(t, sanitizeStringForK8sName(tc.hostname), ce.Name, "name should be sanitized version of hostname")
			assert.Equal(t, tc.labels, ce.Labels, "labels should match expected")

			assert.Equal(t, "https://"+tc.hostname, ce.Spec.URL, "URL should be 'https://'+hostname")
			assert.Equal(t, tc.metadata, ce.Spec.Metadata, "metadata should match expected")
		})
	}
}
