package parser

import (
	"testing"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/ngrok/ngrok-operator/internal/errors"
)

func objWithAnnotations(anns map[string]string) client.Object {
	return &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Annotations: anns}}
}

func TestGetStringAnnotationDualRead(t *testing.T) {
	testCases := []struct {
		name        string
		annotations map[string]string
		want        string
		wantErr     func(error) bool
	}{
		{
			name:        "canonical only",
			annotations: map[string]string{"ngrok.com/url": "tcp://a"},
			want:        "tcp://a",
		},
		{
			name:        "legacy only falls back",
			annotations: map[string]string{"k8s.ngrok.com/url": "tcp://b"},
			want:        "tcp://b",
		},
		{
			name: "both present canonical wins",
			annotations: map[string]string{
				"ngrok.com/url":     "tcp://new",
				"k8s.ngrok.com/url": "tcp://old",
			},
			want: "tcp://new",
		},
		{
			name: "canonical present but empty does not fall back",
			annotations: map[string]string{
				"ngrok.com/url":     "",
				"k8s.ngrok.com/url": "tcp://old",
			},
			wantErr: errors.IsInvalidContent,
		},
		{
			name:        "neither present",
			annotations: map[string]string{"other/url": "x"},
			wantErr:     errors.IsMissingAnnotations,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := GetStringAnnotation("url", objWithAnnotations(tc.annotations))
			if tc.wantErr != nil {
				assert.Error(t, err)
				assert.True(t, tc.wantErr(err))
				return
			}
			assert.NoError(t, err)
			assert.Equal(t, tc.want, got)
		})
	}
}

func TestGetBoolAnnotationDualRead(t *testing.T) {
	testCases := []struct {
		name        string
		annotations map[string]string
		want        bool
		wantErr     bool
	}{
		{
			name:        "legacy only falls back",
			annotations: map[string]string{"k8s.ngrok.com/pooling-enabled": "true"},
			want:        true,
		},
		{
			name: "both present canonical wins",
			annotations: map[string]string{
				"ngrok.com/pooling-enabled":     "false",
				"k8s.ngrok.com/pooling-enabled": "true",
			},
			want: false,
		},
		{
			name: "invalid canonical does not fall back",
			annotations: map[string]string{
				"ngrok.com/pooling-enabled":     "not-a-bool",
				"k8s.ngrok.com/pooling-enabled": "true",
			},
			wantErr: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := GetBoolAnnotation("pooling-enabled", objWithAnnotations(tc.annotations))
			if tc.wantErr {
				assert.Error(t, err)
				return
			}
			assert.NoError(t, err)
			assert.Equal(t, tc.want, got)
		})
	}
}

func TestGetStringSliceAnnotationDualRead(t *testing.T) {
	got, err := GetStringSliceAnnotation("bindings", objWithAnnotations(
		map[string]string{"k8s.ngrok.com/bindings": "a, b"},
	))
	assert.NoError(t, err)
	assert.Equal(t, []string{"a", "b"}, got)

	got, err = GetStringSliceAnnotation("bindings", objWithAnnotations(map[string]string{
		"ngrok.com/bindings":     "new",
		"k8s.ngrok.com/bindings": "old",
	}))
	assert.NoError(t, err)
	assert.Equal(t, []string{"new"}, got)
}

// The implementation changes all six Get* helpers via annotationKeyFor, so
// pin the remaining value types too (one legacy-fallback + one canonical-wins
// case each is enough).
func TestGetStringMapAnnotationDualRead(t *testing.T) {
	got, err := GetStringMapAnnotation("metadata", objWithAnnotations(
		map[string]string{"k8s.ngrok.com/metadata": `{"a":"b"}`},
	))
	assert.NoError(t, err)
	assert.Equal(t, map[string]string{"a": "b"}, got)

	got, err = GetStringMapAnnotation("metadata", objWithAnnotations(map[string]string{
		"ngrok.com/metadata":     `{"x":"y"}`,
		"k8s.ngrok.com/metadata": `{"a":"b"}`,
	}))
	assert.NoError(t, err)
	assert.Equal(t, map[string]string{"x": "y"}, got)
}

func TestGetIntAnnotationDualRead(t *testing.T) {
	got, err := GetIntAnnotation("port", objWithAnnotations(
		map[string]string{"k8s.ngrok.com/port": "8080"},
	))
	assert.NoError(t, err)
	assert.Equal(t, 8080, got)

	got, err = GetIntAnnotation("port", objWithAnnotations(map[string]string{
		"ngrok.com/port":     "9090",
		"k8s.ngrok.com/port": "8080",
	}))
	assert.NoError(t, err)
	assert.Equal(t, 9090, got)
}

func TestGetFloatAnnotationDualRead(t *testing.T) {
	// GetFloatAnnotation returns float32 (parser.go:191) — typed expectations
	// or assert.Equal fails on float64-vs-float32.
	got, err := GetFloatAnnotation("weight", objWithAnnotations(
		map[string]string{"k8s.ngrok.com/weight": "0.5"},
	))
	assert.NoError(t, err)
	assert.Equal(t, float32(0.5), got)

	got, err = GetFloatAnnotation("weight", objWithAnnotations(map[string]string{
		"ngrok.com/weight":     "0.75",
		"k8s.ngrok.com/weight": "0.5",
	}))
	assert.NoError(t, err)
	assert.Equal(t, float32(0.75), got)
}
