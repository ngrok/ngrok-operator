package controllers

import (
	"context"
	"strings"

	netv1 "k8s.io/api/networking/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func getEdgeName(namespacedName string) string {
	return strings.Replace(namespacedName, "/", "-", -1)
}

// Checks to see if the ingress controller should do anything about
// the ingress object it saw depending on how ingress classes are configured
// Returns a boolean indicating if the ingress object should be processed
func matchesIngressClass(ctx context.Context, t client.Client) (bool, error) {
	ingressClasses := &netv1.IngressClassList{}
	if err := t.List(ctx, ingressClasses); err != nil {
		return false, err
	}

	// TODO: Finish filtering on ingress class (verify the behavior based on how other controllers do it)
	// https://kubernetes.io/docs/concepts/services-networking/ingress/#default-ingress-class
	return true, nil
}

// Lookup the ingress object and provide any filtering or error handling logic to filter things out
func getIngress(ctx context.Context, t client.Client, namespacedName types.NamespacedName) (*netv1.Ingress, error) {
	if matches, err := matchesIngressClass(ctx, t); !matches || err != nil {
		return nil, err
	}

	ingress := &netv1.Ingress{}
	if err := t.Get(ctx, namespacedName, ingress); err != nil {
		return nil, err
	}

	return ingress, nil
}
