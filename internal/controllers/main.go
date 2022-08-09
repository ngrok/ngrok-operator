package controllers

import (
	"context"

	v1 "k8s.io/api/core/v1"
	netv1 "k8s.io/api/networking/v1"
	"k8s.io/apimachinery/pkg/types"
	"ngrok.io/ngrok-ingress-controller/pkg/ngrokapidriver"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

const finalizerName = "ngrok.io/finalizer"

// Checks to see if the ingress controller should do anything about
// the ingress object it saw depending on how ingress classes are configured
// Returns a boolean indicating if the ingress object should be processed
func matchesIngressClass(ctx context.Context, c client.Client) (bool, error) {
	ingressClasses := &netv1.IngressClassList{}
	if err := c.List(ctx, ingressClasses); err != nil {
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
func getIngress(ctx context.Context, c client.Client, namespacedName types.NamespacedName) (*netv1.Ingress, error) {
	if matches, err := matchesIngressClass(ctx, c); !matches || err != nil {
		return nil, err
	}

	ingress := &netv1.Ingress{}
	if err := c.Get(ctx, namespacedName, ingress); err != nil {
		return nil, err
	}

	if err := validateIngress(ctx, ingress); err != nil {
		return nil, err
	}

	return ingress, nil
}

// Sets the hostname that the tunnel is accessible on to the ingress object status
func setStatus(ctx context.Context, irec *IngressReconciler, ingress *netv1.Ingress) error {
	// TODO: Handle multiple rules
	if ingress.Spec.Rules[0].Host == "" || len(ingress.Status.LoadBalancer.Ingress) != 0 && ingress.Status.LoadBalancer.Ingress[0].Hostname == ingress.Spec.Rules[0].Host {
		return nil
	}

	ingress.Status.LoadBalancer.Ingress = []v1.LoadBalancerIngress{
		{
			Hostname: ingress.Spec.Rules[0].Host,
		},
	}

	if err := irec.Status().Update(ctx, ingress); err != nil {
		return err
	}

	// TODO: This update and the update in setFinalizer both trigger new reconcile loops. We should
	// make these functions just mutate the objects and then we save them once, and/or use
	// updateFunc predicates to filter out updates to status and finalizers
	return irec.Client.Update(ctx, ingress)
}

func setFinalizer(ctx context.Context, irec *IngressReconciler, ingress *netv1.Ingress) error {
	// if the deletion timestamp is set, its being deleted and we don't need to add the finalizer
	if !ingress.ObjectMeta.DeletionTimestamp.IsZero() {
		return nil
	}
	// The object is not being deleted, so if it does not have our finalizer,
	// then lets add the finalizer and update the object. This is equivalent
	// registering our finalizer.
	if !controllerutil.ContainsFinalizer(ingress, finalizerName) {
		controllerutil.AddFinalizer(ingress, finalizerName)
		if err := irec.Update(ctx, ingress); err != nil {
			return err
		}
	}

	return nil
}

// Checks the ingress object to make sure its using a the limited set of configuration options
// that we support. Returns an error if the ingress object is not valid
func validateIngress(ctx context.Context, ingress *netv1.Ingress) error {
	// TODO: restrict the spec to a limited set of usecases for now until we support others
	// Only 1 unique hostname is allowed per object
	// For now, only 1 rule is even allowed
	// same namespace as the controller for now
	// Atleast 1 route must be declared
	return nil
}

// Converts a k8s ingress object into an edge with all its configurations and sub-resources
func IngressToEdge(ctx context.Context, ingress *netv1.Ingress) (*ngrokapidriver.Edge, error) {
	return &ngrokapidriver.Edge{
		Id: ingress.Annotations["ngrok.io/edge-id"],
		// TODO: Support multiple rules
		Hostport: ingress.Spec.Rules[0].Host + ":443",
		Labels: map[string]string{
			"ngrok.io/ingress-name":      ingress.Name,
			"ngrok.io/ingress-namespace": ingress.Namespace,
			// TODO: Maybe I don't need this backend name. Need to figure out if edge labels have to all match or if we can match
			// a subset. In theory the edge can support multiple different backends
			"ngrok.io/k8s-backend-name": ingress.Spec.Rules[0].HTTP.Paths[0].Backend.Service.Name,
		},
		Routes: []ngrokapidriver.Route{
			{
				Match:     ingress.Spec.Rules[0].HTTP.Paths[0].Path,
				MatchType: "exact_path", // TODO: support other match types
				// MatchType: string(*ingress.Spec.Rules[0].HTTP.Paths[0].PathType),
				// Modules:   []ngrokapidriver.Module{},
			}},
	}, nil
}
