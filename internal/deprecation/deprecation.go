// Package deprecation centralizes structured logging and Kubernetes Event
// emission for legacy "k8s.ngrok.com/" annotations and labels during the
// migration to the unified "ngrok.com/" prefix.
//
// # Cleanup marker convention (LEGACY-PREFIX-MIGRATION)
//
// All read-side compatibility code for the legacy prefix is tagged with the
// sentinel string `LEGACY-PREFIX-MIGRATION` so the cleanup PR is a single,
// auditable sweep. Two forms:
//
//	// LEGACY-PREFIX-MIGRATION: BEGIN
//	// ... block to delete ...
//	// LEGACY-PREFIX-MIGRATION: END
//
//	someLegacyCall(...) // LEGACY-PREFIX-MIGRATION: drop in 1.0
//
// Find every site:
//
//	grep -rn 'LEGACY-PREFIX-MIGRATION' .
//
// The cleanup workflow is, for each hit, either delete the block between
// BEGIN/END or delete the marked line. This whole `deprecation` package is
// itself in scope for deletion in the cleanup PR — it has no callers once
// legacy reads are gone.
//
// See docs/v1-migration-guide.md for the user-facing migration table.
//
// LEGACY-PREFIX-MIGRATION: BEGIN (package scope — delete the entire package in 1.0)
package deprecation

import (
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/go-logr/logr"
)

const (
	// ReasonLegacyAnnotation is the Event reason emitted when a legacy
	// k8s.ngrok.com/* annotation or label is observed on a user-owned object.
	ReasonLegacyAnnotation = "LegacyAnnotation"

	// LegacyPrefix is the deprecated annotation/label prefix.
	LegacyPrefix = "k8s.ngrok.com"

	// NewPrefix is the unified annotation/label prefix.
	NewPrefix = "ngrok.com"
)

// EventRecorder is the minimal subset of k8s.io/client-go/tools/events.EventRecorder
// we need. Using a narrow interface keeps test wiring trivial and lets translator
// paths pass nil where they don't have a recorder.
type EventRecorder interface {
	Eventf(regarding, related runtime.Object, eventtype, reason, action, note string, args ...any)
}

// Annotation reports observation of a legacy annotation key on obj. It always
// emits a structured log line and, if recorder is non-nil, fires a Warning
// Event with reason ReasonLegacyAnnotation against obj.
//
// Translator-style hot paths that do not own a per-object recorder should pass
// recorder=nil; the controller reconcile path fires the Event so users see one
// signal per surface, not per translation pass.
func Annotation(log logr.Logger, recorder EventRecorder, obj client.Object, legacyKey, newKey string) {
	emit(log, recorder, obj, "annotation", legacyKey, newKey)
}

// Label reports observation of a legacy label key on obj. Same semantics as
// Annotation.
//
// Intentional placeholder: no caller yet — the label-prefix migration lands on a
// separate slice and will wire this in. Kept here for symmetry with Annotation.
func Label(log logr.Logger, recorder EventRecorder, obj client.Object, legacyKey, newKey string) {
	emit(log, recorder, obj, "label", legacyKey, newKey)
}

func emit(log logr.Logger, recorder EventRecorder, obj client.Object, kind, legacyKey, newKey string) {
	log.Info("legacy "+kind+" key in use; please migrate",
		"legacy", legacyKey,
		"new", newKey,
	)
	if recorder == nil || obj == nil {
		return
	}
	recorder.Eventf(obj, nil, corev1.EventTypeWarning, ReasonLegacyAnnotation, "Reconcile",
		"use %q instead of legacy %q", newKey, legacyKey)
}

// userFacingAnnotationSuffixes enumerates the annotation suffixes that users
// can set under either prefix and that a controller reconcile path can emit a
// LegacyAnnotation event for. Two categories are intentionally omitted:
//   - Operator-written annotations (computed-url) migrate silently and would
//     just generate event noise on objects mid-reconcile.
//   - app-protocols is read only from the backing Service inside the
//     Ingress/Gateway translators (getProtoForServicePort), which run in a hot
//     loop with no per-object recorder, so its legacy use surfaces via operator
//     logs only — not a LegacyAnnotation event. See docs/v1-migration-guide.md.
var userFacingAnnotationSuffixes = []string{
	"url",
	"mapping-strategy",
	"traffic-policy",
	"pooling-enabled",
	"metadata",
	"description",
	"bindings",
}

// ScanAnnotations emits one Warning event per user-set legacy annotation key
// found on obj. Used by controllers (Ingress, Gateway, Service) that need a
// per-reconcile event signal — the translator paths read these same keys but
// from a hot loop without a per-object recorder, so they only log.
func ScanAnnotations(log logr.Logger, recorder EventRecorder, obj client.Object) {
	if obj == nil {
		return
	}
	anns := obj.GetAnnotations()
	if len(anns) == 0 {
		return
	}
	for _, suffix := range userFacingAnnotationSuffixes {
		legacyKey := LegacyPrefix + "/" + suffix
		if _, ok := anns[legacyKey]; !ok {
			continue
		}
		newKey := NewPrefix + "/" + suffix
		Annotation(log, recorder, obj, legacyKey, newKey)
	}
}

// LEGACY-PREFIX-MIGRATION: END
