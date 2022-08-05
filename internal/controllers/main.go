package controllers

import (
	"context"
	"strings"

	v1 "k8s.io/api/core/v1"
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
	// lookup cluster ingress classes
	// if none are defined
	// 	then handle this ingress
	// if some are defined
	// 	filter to ones that match our controller
	// 		Look at the ingress object and see if it has a class
	// 			if it doesn't
	// 				check if our matched class is the default
	// 					if it is  handle it
	// 					if it isn't drop it
	// 			if it does
	// 				check if it matches our ingress class
	// 					if it does handle it
	// 					if it doesn't drop it
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

// Sets the hostname that the tunnel is accessible on to the ingress object status
func setStatus(ctx context.Context, ir *IngressReconciler, ingress *netv1.Ingress, hostname string) error {
	// TODO: Handle multiple rules
	if ingress.Spec.Rules[0].Host == "" {
		return nil
	}

	ingress.Status.LoadBalancer.Ingress = []v1.LoadBalancerIngress{
		{
			Hostname: ingress.Spec.Rules[0].Host,
		},
	}

	if err := ir.Status().Update(ctx, ingress); err != nil {
		return err
	}

	return ir.Client.Update(ctx, ingress)
}
