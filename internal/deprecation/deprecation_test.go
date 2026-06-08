package deprecation

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestUserFacingAnnotationSuffixes_CoversEveryAnnotationKey is a guardrail
// against future drift between `internal/annotations` and the suffix list
// that drives ScanAnnotations. Whenever a new user-set annotation is added
// in internal/annotations/annotations.go (paired with a `Legacy*Annotation`
// const), its suffix must also appear here so the reconcile path emits a
// LegacyAnnotation event for the legacy form.
//
// We assert by enumerating the suffixes we expect (sourced from the
// `*AnnotationKey` consts in internal/annotations) and checking that each
// is present in the package-private list. We don't import the annotations
// package to avoid an import cycle.
//
// LEGACY-PREFIX-MIGRATION: delete this test file along with the rest of the
// package in the 1.0 cleanup.
func TestUserFacingAnnotationSuffixes_CoversEveryAnnotationKey(t *testing.T) {
	// "app-protocols" is deliberately absent: it is read only from the backing
	// Service inside the Ingress/Gateway translators with no per-object recorder,
	// so it surfaces via logs only and never fires a LegacyAnnotation event. If
	// you add it back here, also wire an event path for it.
	expected := []string{
		"url",
		"mapping-strategy",
		"traffic-policy",
		"pooling-enabled",
		"metadata",
		"description",
		"bindings",
	}

	got := map[string]bool{}
	for _, s := range userFacingAnnotationSuffixes {
		got[s] = true
	}

	for _, want := range expected {
		assert.True(t, got[want],
			"userFacingAnnotationSuffixes is missing %q — if you added an Extract* helper in internal/annotations, also add its suffix here so ScanAnnotations fires the event",
			want)
	}

	for _, s := range userFacingAnnotationSuffixes {
		assert.False(t, strings.Contains(s, "/"),
			"suffix %q must be the bare key (no prefix); ScanAnnotations prepends LegacyPrefix", s)
	}
}
