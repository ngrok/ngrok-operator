package util

import (
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// ToClientObjects converts a slice of objects whose pointer implements client.Object
// to a slice of client.Objects
func ToClientObjects[T any, PT interface {
	*T
	client.Object
}](s []T) []client.Object {
	objs := make([]client.Object, len(s))
	for i, obj := range s {
		var p PT = &obj
		objs[i] = p
	}
	return objs
}

// ObjectsToName converts a client.Object to its name
func ObjToName(obj client.Object) string {
	if obj == nil {
		return ""
	}

	gvk := obj.GetObjectKind().GroupVersionKind()
	if gvk.Empty() {
		return ""
	}

	return obj.GetName()
}

// ObjToKind converts a client.Object to its kind
func ObjToKind(obj client.Object) string {
	if obj == nil {
		return ""
	}

	gvk := obj.GetObjectKind().GroupVersionKind()
	if gvk.Empty() {
		return ""
	}

	return gvk.Kind
}

// ObjToGroupVersionKind converts a client.Object to its GVK
func ObjToGVK(obj client.Object) string {
	if obj == nil {
		return ""
	}

	gvk := obj.GetObjectKind().GroupVersionKind()
	if gvk.Empty() {
		return ""
	}

	return gvk.String()
}

// ObjToHumanName converts a client.Object to a human-readable name
func ObjToHumanName(obj client.Object) string {
	if obj == nil {
		return ""
	}

	gvk := obj.GetObjectKind().GroupVersionKind()
	if gvk.Empty() {
		return ""
	}

	return gvk.Kind + "/" + obj.GetName()
}

// ObjToHumanGvkName converts a client.Object to a human-readable full name including GroupVersionKind
func ObjToHumanGvkName(obj client.Object) string {
	if obj == nil {
		return ""
	}

	gvk := obj.GetObjectKind().GroupVersionKind()
	if gvk.Empty() {
		return ""
	}

	return gvk.String() + " Name=" + obj.GetName()
}
