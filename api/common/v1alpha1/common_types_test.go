package common

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestHasUseEndpointsAnnotation(t *testing.T) {
	tests := []struct {
		name        string
		annotations map[string]string
		expected    bool
	}{
		{
			name:        "annotation missing",
			annotations: map[string]string{},
			expected:    false,
		},
		{
			name: "annotation exists but value is false",
			annotations: map[string]string{
				AnnotationUseEndpoints: "false",
			},
			expected: false,
		},
		{
			name: "annotation exists with value TRUE (case-insensitive)",
			annotations: map[string]string{
				AnnotationUseEndpoints: "TRUE",
			},
			expected: true,
		},
		{
			name: "annotation exists with value true (lowercase)",
			annotations: map[string]string{
				AnnotationUseEndpoints: "true",
			},
			expected: true,
		},
		{
			name: "annotation exists but with an unrelated value",
			annotations: map[string]string{
				AnnotationUseEndpoints: "some-random-value",
			},
			expected: false,
		},
		{
			name: "multiple annotations, correct one is true",
			annotations: map[string]string{
				"some-other-annotation": "value",
				AnnotationUseEndpoints:  "true",
			},
			expected: true,
		},
		{
			name: "multiple annotations, correct one is missing",
			annotations: map[string]string{
				"some-other-annotation": "value",
			},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := HasUseEndpointsAnnotation(tt.annotations)
			assert.Equal(t, tt.expected, result, "unexpected result for test case: %s", tt.name)
		})
	}
}
