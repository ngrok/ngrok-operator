package store

import "sigs.k8s.io/controller-runtime/pkg/client"

const (
	annotationIngressControllerManaged = "ingress.k8s.ngrok.com/ingress-controller-managed"
)

func isControllerManaged(obj client.Object) bool {
	if v, ok := obj.GetAnnotations()[annotationIngressControllerManaged]; ok {
		return v == "true"
	}
	return false
}

func setControllerManaged(obj client.Object) {
	annotations := obj.GetAnnotations()
	if annotations == nil {
		annotations = make(map[string]string)
	}
	annotations[annotationIngressControllerManaged] = "true"
	obj.SetAnnotations(annotations)
}
