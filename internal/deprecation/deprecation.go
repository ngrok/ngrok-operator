// Package deprecation emits user-visible signals when deprecated
// k8s.ngrok.com/* annotation keys are in use, so users can find and migrate
// their manifests before the legacy read support is removed. See
// docs/v1-migration-guide.md.
//
// LEGACY-PREFIX-MIGRATION: BEGIN (package scope — read-side cleanup deletes
// this entire package and its call sites)
package deprecation

import (
	"fmt"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/ngrok/ngrok-operator/internal/annotations/parser"
)

// ReasonLegacyAnnotation is the event reason for legacy-prefix annotation
// hits. Events expire, so this is an immediate signal for recently
// reconciled objects, not a complete inventory:
//
//	kubectl get events -A --field-selector reason=LegacyAnnotation
const ReasonLegacyAnnotation = "LegacyAnnotation"

// EventRecorder is the narrow slice of events.EventRecorder this package
// needs. A nil recorder degrades to log-only.
type EventRecorder interface {
	Eventf(regarding runtime.Object, related runtime.Object, eventtype, reason, action, note string, args ...any)
}

// userFacingAnnotationSuffixes are the user-written annotation suffixes that
// ScanAnnotations checks under the legacy prefix.
var userFacingAnnotationSuffixes = []string{
	"url",
	"mapping-strategy",
	"traffic-policy",
	"pooling-enabled",
	"bindings",
	"metadata",
	"description",
	"app-protocols",
}

// ScanAnnotations emits one Warning event and one log line per
// legacy-prefixed user annotation present on obj. Controllers call it once
// per reconcile of a user-owned object.
func ScanAnnotations(log logr.Logger, recorder EventRecorder, obj client.Object) {
	anns := obj.GetAnnotations()
	if len(anns) == 0 {
		return
	}
	for _, suffix := range userFacingAnnotationSuffixes {
		legacyKey := fmt.Sprintf("%s/%s", parser.LegacyAnnotationsPrefix, suffix)
		if _, ok := anns[legacyKey]; !ok {
			continue
		}
		newKey := fmt.Sprintf("%s/%s", parser.CanonicalAnnotationsPrefix, suffix)
		log.Info("legacy annotation key in use; please migrate",
			"legacyKey", legacyKey, "newKey", newKey)
		if recorder != nil {
			recorder.Eventf(obj, nil, corev1.EventTypeWarning, ReasonLegacyAnnotation, "Reconcile",
				"annotation %q is deprecated and support for it will be removed in ngrok-operator 1.0; rename it to %q", legacyKey, newKey)
		}
	}
}

// LEGACY-PREFIX-MIGRATION: END
