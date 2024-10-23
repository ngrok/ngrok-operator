package util

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestMergeMaps(t *testing.T) {
	assert := assert.New(t)

	testCases := []struct {
		expected map[string]string
		maps     []map[string]string
	}{
		{
			expected: map[string]string{},
			maps:     []map[string]string{},
		},
		{
			expected: map[string]string{"a": "1", "b": "2"},
			maps: []map[string]string{
				{"a": "1"},
				{"b": "2"},
			},
		},
		{
			expected: map[string]string{"a": "3", "b": "2"},
			maps: []map[string]string{
				{"a": "1"},
				{"b": "2"},
				{"a": "3"},
			},
		},
		{
			expected: map[string]string{"a": "3", "b": "4", "c": "5"},
			maps: []map[string]string{
				{"a": "1", "b": "2"},
				{"a": "3"},
				nil,
				{"b": "4", "c": "5"},
			},
		},
	}

	for _, tc := range testCases {
		assert.Equal(tc.expected, MergeMaps(tc.maps...))
	}
}
