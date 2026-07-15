package deprecation

import (
	"fmt"
	"testing"

	"github.com/go-logr/logr"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

type recordedEvent struct {
	eventtype string
	reason    string
	note      string
}

type fakeRecorder struct {
	events []recordedEvent
}

func (f *fakeRecorder) Eventf(_, _ runtime.Object, eventtype, reason, _, note string, args ...any) {
	f.events = append(f.events, recordedEvent{eventtype, reason, fmt.Sprintf(note, args...)})
}

func TestScanAnnotations(t *testing.T) {
	testCases := []struct {
		name        string
		annotations map[string]string
		wantReasons int
		wantNotes   []string
	}{
		{
			name: "one legacy key",
			annotations: map[string]string{
				"k8s.ngrok.com/traffic-policy": "p",
			},
			wantReasons: 1,
			wantNotes:   []string{`"k8s.ngrok.com/traffic-policy"`},
		},
		{
			name: "multiple legacy keys, one event each",
			annotations: map[string]string{
				"k8s.ngrok.com/url":           "tcp://x",
				"k8s.ngrok.com/app-protocols": `{"p":"http"}`,
			},
			wantReasons: 2,
		},
		{
			name: "canonical keys emit nothing",
			annotations: map[string]string{
				"ngrok.com/traffic-policy": "p",
				"ngrok.com/url":            "tcp://x",
			},
			wantReasons: 0,
		},
		{
			name:        "no annotations",
			annotations: nil,
			wantReasons: 0,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			rec := &fakeRecorder{}
			obj := &corev1.Service{ObjectMeta: metav1.ObjectMeta{Annotations: tc.annotations}}
			ScanAnnotations(logr.Discard(), rec, obj)
			assert.Len(t, rec.events, tc.wantReasons)
			for _, ev := range rec.events {
				assert.Equal(t, corev1.EventTypeWarning, ev.eventtype)
				assert.Equal(t, ReasonLegacyAnnotation, ev.reason)
			}
			for _, want := range tc.wantNotes {
				assert.Contains(t, rec.events[0].note, want)
			}
		})
	}
}

func TestScanAnnotationsNilRecorderDoesNotPanic(_ *testing.T) {
	obj := &corev1.Service{ObjectMeta: metav1.ObjectMeta{
		Annotations: map[string]string{"k8s.ngrok.com/url": "tcp://x"},
	}}
	ScanAnnotations(logr.Discard(), nil, obj)
}

// Guardrail: keeps the scanned suffix list from being edited accidentally.
// It compares two manually maintained lists, so it cannot detect a NEW
// user-facing annotation added elsewhere — the Task 10 completeness audit
// (rg for the legacy prefix) is what catches those.
func TestUserFacingAnnotationSuffixes(t *testing.T) {
	want := []string{
		"url",
		"mapping-strategy",
		"traffic-policy",
		"pooling-enabled",
		"bindings",
		"metadata",
		"description",
		"app-protocols",
	}
	assert.ElementsMatch(t, want, userFacingAnnotationSuffixes)
	for _, s := range userFacingAnnotationSuffixes {
		assert.NotContains(t, s, "/")
	}
}
