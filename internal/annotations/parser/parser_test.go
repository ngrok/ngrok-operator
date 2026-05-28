package parser

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/ngrok/ngrok-operator/internal/errors"
)

func TestGetStringAnnotationWithFallback(t *testing.T) {
	tests := []struct {
		name           string
		anns           map[string]string
		expectVal      string
		expectLegacy   bool
		expectMissing  bool
	}{
		{
			name:         "new key only",
			anns:         map[string]string{"ngrok.com/url": "tls://example.com"},
			expectVal:    "tls://example.com",
			expectLegacy: false,
		},
		{
			name:         "legacy key only fires callback",
			anns:         map[string]string{"k8s.ngrok.com/url": "tls://legacy.example.com"},
			expectVal:    "tls://legacy.example.com",
			expectLegacy: true,
		},
		{
			name: "both set, new wins, no callback",
			anns: map[string]string{
				"ngrok.com/url":     "tls://new.example.com",
				"k8s.ngrok.com/url": "tls://legacy.example.com",
			},
			expectVal:    "tls://new.example.com",
			expectLegacy: false,
		},
		{
			name:          "neither set",
			anns:          nil,
			expectMissing: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			obj := &corev1.Service{
				ObjectMeta: metav1.ObjectMeta{Annotations: tc.anns},
			}

			var legacyHit bool
			val, err := GetStringAnnotationWithFallback("url", obj, func(legacyKey, newKey string) {
				legacyHit = true
				assert.Equal(t, "k8s.ngrok.com/url", legacyKey)
				assert.Equal(t, "ngrok.com/url", newKey)
			})

			if tc.expectMissing {
				require.Error(t, err)
				assert.True(t, errors.IsMissingAnnotations(err))
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tc.expectVal, val)
			assert.Equal(t, tc.expectLegacy, legacyHit)
		})
	}
}

func TestGetBoolAnnotationWithFallback(t *testing.T) {
	obj := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{Annotations: map[string]string{
			"k8s.ngrok.com/pooling-enabled": "true",
		}},
	}
	var hit bool
	v, err := GetBoolAnnotationWithFallback("pooling-enabled", obj, func(string, string) { hit = true })
	require.NoError(t, err)
	assert.True(t, v)
	assert.True(t, hit, "callback should fire on legacy hit")
}

func TestGetStringSliceAnnotationWithFallback_NewWins(t *testing.T) {
	obj := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{Annotations: map[string]string{
			"ngrok.com/bindings":     "public",
			"k8s.ngrok.com/bindings": "internal",
		}},
	}
	var hit bool
	v, err := GetStringSliceAnnotationWithFallback("bindings", obj, func(string, string) { hit = true })
	require.NoError(t, err)
	assert.Equal(t, []string{"public"}, v)
	assert.False(t, hit, "callback must not fire when new key wins")
}
