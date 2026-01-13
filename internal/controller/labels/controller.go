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

// ControllerLabels returns the standard labels identifying which operator instance manages a resource
func ControllerLabels(controllerNamespace, controllerName string) map[string]string {
	return map[string]string{
		ControllerNamespace: controllerNamespace,
		ControllerName:      controllerName,
	}
}

// HasControllerLabels checks if an object has the controller labels matching the given operator instance
func HasControllerLabels(obj client.Object, controllerNamespace, controllerName string) bool {
	labels := obj.GetLabels()
	if labels == nil {
		return false
	}
	return labels[ControllerNamespace] == controllerNamespace &&
		labels[ControllerName] == controllerName
}

// EnsureControllerLabels adds controller labels to an object if not already present
// Returns true if labels were added (object was modified)
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

	if modified {
		obj.SetLabels(labels)
	}
	return modified
}

// ControllerLabelSelector returns a client.MatchingLabels for listing resources managed by an operator instance
func ControllerLabelSelector(controllerNamespace, controllerName string) client.MatchingLabels {
	return client.MatchingLabels{
		ControllerNamespace: controllerNamespace,
		ControllerName:      controllerName,
	}
}

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

// HasLabels checks if an object has the controller labels.
func (c ControllerLabelValues) HasLabels(obj client.Object) bool {
	return HasControllerLabels(obj, c.Namespace, c.Name)
}

// EnsureLabels adds controller labels to an object if not already present. If labels were added,
// it returns true (indicating the object was modified).
func (c ControllerLabelValues) EnsureLabels(obj client.Object) bool {
	return EnsureControllerLabels(obj, c.Namespace, c.Name)
}

// Selector returns a client.MatchingLabels for listing resources managed by the operator instance.
func (c ControllerLabelValues) Selector() client.MatchingLabels {
	return ControllerLabelSelector(c.Namespace, c.Name)
}

// ValidateControllerLabelValues checks that both name and namespace are set.
func ValidateControllerLabelValues(clv ControllerLabelValues) error {
	if clv.Name == "" || clv.Namespace == "" {
		return errors.New(ErrControllerLabelsNameAndNamespaceRequired)
	}
	return nil
}
