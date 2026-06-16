package labels

import (
	"errors"

	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	// ControllerName identifies the name of the operator deployment managing a resource
	ControllerName = prefix + "controller-name"

	// ControllerNamespace identifies the namespace of the operator instance managing a resource
	ControllerNamespace = prefix + "controller-namespace"

	ErrControllerLabelsNameAndNamespaceRequired = "both controller name and namespace are required"
)

// LEGACY-PREFIX-MIGRATION: BEGIN
// LegacyControllerName / LegacyControllerNamespace are the deprecated label
// keys retained for dual-read+dual-write during the ngrok.com migration
// window. Cleanup happens in two steps: write-side cleanup drops the legacy
// writes in ControllerLabels / EnsureControllerLabels; the later read-side
// cleanup drops this block, the legacy branch in HasControllerLabels, and the
// LegacyControllerLabelSelector / ControllerLabelSelectors helpers.
const (
	LegacyControllerName      = legacyPrefix + "controller-name"
	LegacyControllerNamespace = legacyPrefix + "controller-namespace"
)

// LEGACY-PREFIX-MIGRATION: END

// ControllerLabels returns the standard labels identifying which operator
// instance manages a resource. During the migration window this dual-writes
// both the new (ngrok.com/...) and legacy (k8s.ngrok.com/...) key pairs so a
// rollback to a pre-migration operator can still match its own labels. The
// write-side cleanup drops the legacy entries from this map.
func ControllerLabels(controllerNamespace, controllerName string) map[string]string {
	return map[string]string{
		ControllerNamespace: controllerNamespace,
		ControllerName:      controllerName,
		// LEGACY-PREFIX-MIGRATION (write-side cleanup): drop both legacy entries
		LegacyControllerNamespace: controllerNamespace,
		LegacyControllerName:      controllerName,
	}
}

// HasControllerLabels checks if an object has the controller labels matching
// the given operator instance. During the migration window it matches either
// the new (ngrok.com/...) or legacy (k8s.ngrok.com/...) key pair.
func HasControllerLabels(obj client.Object, controllerNamespace, controllerName string) bool {
	labels := obj.GetLabels()
	if labels == nil {
		return false
	}
	if labels[ControllerNamespace] == controllerNamespace && labels[ControllerName] == controllerName {
		return true
	}
	// LEGACY-PREFIX-MIGRATION (read-side cleanup): drop the legacy-pair check
	if labels[LegacyControllerNamespace] == controllerNamespace && labels[LegacyControllerName] == controllerName {
		return true
	}
	return false
}

// EnsureControllerLabels writes both the new-prefix and legacy-prefix
// controller labels on obj, setting them to the desired values. During the
// migration window we keep the legacy pair set so a rollback to a pre-migration
// operator still finds its own resources. The write-side cleanup removes the
// legacy keys instead of ensure-setting them.
// Returns true if any label changed.
func EnsureControllerLabels(obj client.Object, controllerNamespace, controllerName string) bool {
	labels := obj.GetLabels()
	if labels == nil {
		labels = make(map[string]string)
	}

	modified := false
	if labels[ControllerNamespace] != controllerNamespace {
		labels[ControllerNamespace] = controllerNamespace
		modified = true
	}
	if labels[ControllerName] != controllerName {
		labels[ControllerName] = controllerName
		modified = true
	}
	// LEGACY-PREFIX-MIGRATION: BEGIN — the write-side cleanup replaces this
	// ensure-set with delete(labels, LegacyControllerNamespace) /
	// delete(labels, LegacyControllerName).
	if labels[LegacyControllerNamespace] != controllerNamespace {
		labels[LegacyControllerNamespace] = controllerNamespace
		modified = true
	}
	if labels[LegacyControllerName] != controllerName {
		labels[LegacyControllerName] = controllerName
		modified = true
	}
	// LEGACY-PREFIX-MIGRATION: END

	if modified {
		obj.SetLabels(labels)
	}
	return modified
}

// ControllerLabelSelector returns a client.MatchingLabels for the new-prefix
// label keys.
func ControllerLabelSelector(controllerNamespace, controllerName string) client.MatchingLabels {
	return client.MatchingLabels{
		ControllerNamespace: controllerNamespace,
		ControllerName:      controllerName,
	}
}

// LEGACY-PREFIX-MIGRATION: BEGIN
// LegacyControllerLabelSelector + ControllerLabelSelectors only exist so
// driver list paths can match objects stamped by previous operator versions.
// In the read-side cleanup delete both functions and have callers go back to
// ControllerLabelSelector (single selector, single List call).

// LegacyControllerLabelSelector returns a client.MatchingLabels for the
// legacy-prefix label keys, used during the migration window so List queries
// can dual-match objects stamped before the operator upgraded.
func LegacyControllerLabelSelector(controllerNamespace, controllerName string) client.MatchingLabels {
	return client.MatchingLabels{
		LegacyControllerNamespace: controllerNamespace,
		LegacyControllerName:      controllerName,
	}
}

// ControllerLabelSelectors returns both the new and legacy selectors. Callers
// should issue one List per selector and dedupe by UID. Kubernetes label
// selectors cannot OR across different label keys, so two queries are required.
func ControllerLabelSelectors(controllerNamespace, controllerName string) []client.MatchingLabels {
	return []client.MatchingLabels{
		ControllerLabelSelector(controllerNamespace, controllerName),
		LegacyControllerLabelSelector(controllerNamespace, controllerName),
	}
}

// LEGACY-PREFIX-MIGRATION: END

// ControllerLabelValues encapsulates controller label values for an operator instance.
type ControllerLabelValues struct {
	Namespace string
	Name      string
}

// NewControllerLabelValues creates a new ControllerLabelValues instance.
func NewControllerLabelValues(namespace, name string) ControllerLabelValues {
	return ControllerLabelValues{
		Namespace: namespace,
		Name:      name,
	}
}

// Labels returns the controller labels as a map.
func (c ControllerLabelValues) Labels() map[string]string {
	return ControllerLabels(c.Namespace, c.Name)
}

// HasLabels checks if an object has the controller labels (new or legacy).
func (c ControllerLabelValues) HasLabels(obj client.Object) bool {
	return HasControllerLabels(obj, c.Namespace, c.Name)
}

// EnsureLabels dual-writes the new-prefix and legacy-prefix controller labels.
// Returns true if any label changed.
func (c ControllerLabelValues) EnsureLabels(obj client.Object) bool {
	return EnsureControllerLabels(obj, c.Namespace, c.Name)
}

// Selector returns a client.MatchingLabels for the new-prefix label keys.
func (c ControllerLabelValues) Selector() client.MatchingLabels {
	return ControllerLabelSelector(c.Namespace, c.Name)
}

// LEGACY-PREFIX-MIGRATION: BEGIN
// Selectors returns both the new and legacy selectors. See
// ControllerLabelSelectors for why two queries are required. In the read-side
// cleanup delete this method and have callers use Selector() (single selector).
func (c ControllerLabelValues) Selectors() []client.MatchingLabels {
	return ControllerLabelSelectors(c.Namespace, c.Name)
}

// LEGACY-PREFIX-MIGRATION: END

// ValidateControllerLabelValues checks that both name and namespace are set.
func ValidateControllerLabelValues(clv ControllerLabelValues) error {
	if clv.Name == "" || clv.Namespace == "" {
		return errors.New(ErrControllerLabelsNameAndNamespaceRequired)
	}
	return nil
}
