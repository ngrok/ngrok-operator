package store

import "sigs.k8s.io/controller-runtime/pkg/client"

const (
	annotationIngressControllerManaged = "ingress.k8s.ngrok.com/ingress-controller-managed"
)

func hasControllerManagedAnnotation(obj client.Object) bool {
	annotations := obj.GetAnnotations()
	v, ok := annotations[annotationIngressControllerManaged]
	if !ok {
		return false
	}
	return v == "true"
}
