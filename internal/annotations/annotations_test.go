/*
Copyright 2017 The Kubernetes Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package annotations_test

import (
	"fmt"
	"testing"

	"github.com/ngrok/ngrok-operator/internal/annotations"
	"github.com/ngrok/ngrok-operator/internal/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	networking "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestExtractNgrokTrafficPolicyFromAnnotations(t *testing.T) {
	tests := []struct {
		name        string
		annotations map[string]string
		expected    string
		expectedErr error
	}{
		{
			name: "Valid traffic policy",
			annotations: map[string]string{
				"k8s.ngrok.com/traffic-policy": "policy1",
			},
			expected:    "policy1",
			expectedErr: nil,
		},
		{
			name:        "No annotations",
			annotations: nil,
			expected:    "",
			expectedErr: errors.ErrMissingAnnotations,
		},
		{
			name: "Multiple traffic policies (invalid)",
			annotations: map[string]string{
				"k8s.ngrok.com/traffic-policy": "policy1,policy2",
			},
			expected:    "",
			expectedErr: errors.New("multiple traffic policies are not supported: [policy1 policy2]"),
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			obj := &networking.Ingress{
				ObjectMeta: metav1.ObjectMeta{
					Name:        "test-ingress",
					Namespace:   "default",
					Annotations: tc.annotations,
				},
			}

			policy, err := annotations.ExtractNgrokTrafficPolicyFromAnnotations(obj)
			if tc.expectedErr != nil {
				require.Error(t, err)
				assert.Equal(t, tc.expectedErr, err)
				assert.Empty(t, policy)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tc.expected, policy)
			}
		})
	}
}

func TestExtractUseEdges(t *testing.T) {
	tests := []struct {
		name        string
		annotations map[string]string
		expected    bool
		expectedErr error
	}{
		{
			name: "Valid mapping strategy: edges",
			annotations: map[string]string{
				"k8s.ngrok.com/mapping-strategy": "edges",
			},
			expected:    true,
			expectedErr: nil,
		},
		{
			name: "Valid mapping strategy: endpoints",
			annotations: map[string]string{
				"k8s.ngrok.com/mapping-strategy": "endpoints",
			},
			expected:    false,
			expectedErr: nil,
		},
		{
			name:        "No annotations (default)",
			annotations: nil,
			expected:    false,
			expectedErr: nil,
		},
		{
			name: "Invalid mapping strategy",
			annotations: map[string]string{
				"k8s.ngrok.com/mapping-strategy": "invalid",
			},
			expected:    false,
			expectedErr: nil,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			obj := &networking.Ingress{
				ObjectMeta: metav1.ObjectMeta{
					Name:        "test-ingress",
					Namespace:   "default",
					Annotations: tc.annotations,
				},
			}

			useEdges, err := annotations.ExtractUseEdges(obj)
			if tc.expectedErr != nil {
				require.Error(t, err)
				assert.Equal(t, tc.expectedErr, err)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tc.expected, useEdges)
			}
		})
	}
}

func TestExtractUseEndpointPooling(t *testing.T) {
	tests := []struct {
		name        string
		annotations map[string]string
		expected    bool
		expectedErr error
	}{
		{
			name: "Pooling enabled",
			annotations: map[string]string{
				"k8s.ngrok.com/pooling-enabled": "true",
			},
			expected:    true,
			expectedErr: nil,
		},
		{
			name: "Pooling disabled",
			annotations: map[string]string{
				"k8s.ngrok.com/pooling-enabled": "false",
			},
			expected:    false,
			expectedErr: nil,
		},
		{
			name: "Invalid value",
			annotations: map[string]string{
				"k8s.ngrok.com/pooling-enabled": "foo",
			},
			expected:    false,
			expectedErr: nil,
		},
		{
			name:        "Annotation not present",
			annotations: nil,
			expected:    false,
			expectedErr: nil,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			obj := &networking.Ingress{
				ObjectMeta: metav1.ObjectMeta{
					Name:        "test-ingress",
					Namespace:   "default",
					Annotations: tc.annotations,
				},
			}

			useEndpoints, err := annotations.ExtractUseEndpointPooling(obj)
			if tc.expectedErr != nil {
				require.Error(t, err)
				assert.Equal(t, tc.expectedErr, err)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tc.expected, useEndpoints)
			}
		})
	}
}

func TestExtractUseBindings(t *testing.T) {
	tests := []struct {
		name        string
		annotations map[string]string
		expected    []string
		expectedErr error
	}{
		{
			name: "Valid Bindings: Public",
			annotations: map[string]string{
				"k8s.ngrok.com/bindings": "public",
			},
			expected:    []string{"public"},
			expectedErr: nil,
		},
		{
			name: "Invalid Binding Annotation",
			annotations: map[string]string{
				"k8s.ngrok.com/bindings": "foo",
			},
			expected:    []string{"foo"},
			expectedErr: nil,
		},
		{
			name: "Invalid Binding Errs When n > 1",
			annotations: map[string]string{
				"k8s.ngrok.com/bindings": "public,internal",
			},
			expectedErr: fmt.Errorf("multiple bindings are not supported: [public internal]"),
		},
		{
			name: "Missing Bindings Value",
			annotations: map[string]string{
				"k8s.ngrok.com/bindings": "",
			},
			expectedErr: errors.NewInvalidAnnotationContent("k8s.ngrok.com/bindings", ""),
		},
		{
			name:        "Annotation Not present",
			annotations: nil,
			expected:    []string{"public"},
			expectedErr: nil,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			obj := &networking.Ingress{
				ObjectMeta: metav1.ObjectMeta{
					Name:        "test-ingress",
					Namespace:   "default",
					Annotations: tc.annotations,
				},
			}

			binding, err := annotations.ExtractUseBindings(obj)
			if tc.expectedErr != nil {
				require.Error(t, err)
				assert.Equal(t, tc.expectedErr, err)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tc.expected, binding)
			}
		})
	}
}
