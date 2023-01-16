package controllers

import (
	"context"
	"fmt"
	"strconv"

	netv1 "k8s.io/api/networking/v1"
	ctrl "sigs.k8s.io/controller-runtime"

	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// The name of the ingress controller which is uses to match on ingress classes
const controllerName = "k8s.ngrok.com/ingress-controller" // TODO: Let the user configure this

// Checks to see if the ingress controller should do anything about
// the ingress object it saw depending on how ingress classes are configured
// Returns a boolean indicating if the ingress object should be processed
func matchesIngressClass(ctx context.Context, c client.Client, ingress *netv1.Ingress) (bool, error) {
	ingressClasses := &netv1.IngressClassList{}
	if err := c.List(ctx, ingressClasses); err != nil {
		return false, err
	}

	// https://kubernetes.io/docs/concepts/services-networking/ingress/#default-ingress-class
	// lookup cluster ingress classes
	// if none are defined
	// 	then handle this ingress
	// if some are defined
	// 	filter to one that matches our controller
	// 		Look at the ingress object and see if it has a class
	// 			if it doesn't
	// 				check if our matched class is the default
	// 					if it is  handle it
	// 					if it isn't drop it
	// 			if it does
	// 				check if it matches our ingress class
	// 					if it does handle it
	// 					if it doesn't drop it

	if len(ingressClasses.Items) == 0 {
		return true, nil
	}

	var ngrokClass *netv1.IngressClass
	for _, ingressClass := range ingressClasses.Items {
		if ingressClass.Spec.Controller == controllerName {
			ngrokClass = &ingressClass
			break
		}
	}

	if ngrokClass == nil {
		ctrl.LoggerFrom(ctx).Error(fmt.Errorf("No ingress class found for this controller"), "controller", controllerName)
		return false, nil
	}

	if ngrokClass.Annotations["ingressclass.kubernetes.io/is-default-class"] == "default" {
		if ingress.Spec.IngressClassName == nil || ingress.Spec.IngressClassName == &ngrokClass.Name {
			return true, nil
		}
		ctrl.LoggerFrom(ctx).Info(fmt.Sprintf("ngrok is the default Ingress class  but this ingress object's ingress class doesn't match: %s\n", *ingress.Spec.IngressClassName), "controller", controllerName)
		return false, nil
	}

	if ingress.Spec.IngressClassName != nil && *ingress.Spec.IngressClassName == ngrokClass.Name {
		return true, nil
	} else {
		ctrl.LoggerFrom(ctx).Info(fmt.Sprintf("Got our else statement so dump some info: %s\n", ngrokClass.Name), "controller", controllerName)
	}

	if ingress.Spec.IngressClassName == nil {
		ctrl.LoggerFrom(ctx).Info("This ingress object's ingress class is not set so we did not handle this one", "controller", controllerName)
	} else {
		ctrl.LoggerFrom(ctx).Info(fmt.Sprintf("This ingress object's ingress class doesn't match: %s\n", *ingress.Spec.IngressClassName), "controller", controllerName)
	}
	return false, nil
}

// Lookup the ingress object and provide any filtering or error handling logic to filter things out
func getIngress(ctx context.Context, c client.Client, namespacedName types.NamespacedName) (*netv1.Ingress, error) {
	ingress := &netv1.Ingress{}
	if err := c.Get(ctx, namespacedName, ingress); err != nil {
		return nil, err
	}

	matches, err := matchesIngressClass(ctx, c, ingress)
	if !matches || err != nil {
		return nil, err
	}

	return ingress, nil
}

// Checks the ingress object to make sure its using a the limited set of configuration options
// that we support. Returns an error if the ingress object is not valid
func validateIngress(ctx context.Context, ingress *netv1.Ingress) error {
	if len(ingress.Spec.Rules) > 1 {
		return fmt.Errorf("A maximum of one rule is required to be set")
	}
	if len(ingress.Spec.Rules) == 0 {
		return fmt.Errorf("At least one rule is required to be set")
	}
	if ingress.Spec.Rules[0].Host == "" {
		return fmt.Errorf("A host is required to be set")
	}
	for _, path := range ingress.Spec.Rules[0].HTTP.Paths {
		if path.Backend.Resource != nil {
			return fmt.Errorf("Resource backends are not supported")
		}
	}

	return nil
}

// Generates a labels map for matching ngrok Routes to Agent Tunnels
func backendToLabelMap(backend netv1.IngressBackend, namespace string) map[string]string {
	return map[string]string{
		"k8s.ngrok.com/namespace": namespace,
		"k8s.ngrok.com/service":   backend.Service.Name,
		"k8s.ngrok.com/port":      strconv.Itoa(int(backend.Service.Port.Number)),
	}
}
