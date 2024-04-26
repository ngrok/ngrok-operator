package ngrok

import (
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
)

// common update func predicate filter
func updateFuncPredicateFilter(ue event.UpdateEvent) bool {
	// First check if there are any annotations present that aren't in the old version
	oldAnnotations := ue.ObjectOld.GetAnnotations()
	for newKey, newValue := range ue.ObjectNew.GetAnnotations() {
		if oldAnnotations[newKey] != newValue {
			return true
		}
	}
	// No change to spec, so we can ignore.  This does not filter out updates
	// that set metadata.deletionTimestamp, so this won't undermine finalizer.
	return ue.ObjectNew.GetGeneration() != ue.ObjectOld.GetGeneration()
}

var commonPredicateFilters = predicate.Funcs{
	UpdateFunc: updateFuncPredicateFilter,
}
