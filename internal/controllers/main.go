package controllers

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/ngrok/ngrok-api-go/v4"
	v1 "k8s.io/api/core/v1"
	netv1 "k8s.io/api/networking/v1"
	ctrl "sigs.k8s.io/controller-runtime"

	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

const finalizerName = "k8s.ngrok.com/finalizer"
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
		ctrl.LoggerFrom(ctx).Info(fmt.Sprintf("Ngrok is the default Ingress class  but this ingress object's ingress class doesn't match: %s\n", *ingress.Spec.IngressClassName), "controller", controllerName)
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

	if err := validateIngress(ctx, ingress); err != nil {
		return nil, err
	}

	matches, err := matchesIngressClass(ctx, c, ingress)
	if !matches || err != nil {
		return nil, err
	}

	return ingress, nil
}

func setEdgeId(ctx context.Context, irec *IngressReconciler, ingress *netv1.Ingress, ngrokEdge *ngrok.HTTPSEdge) error {
	irec.Recorder.Event(ingress, v1.EventTypeNormal, "CreatedEdge", "Created edge "+ngrokEdge.ID)
	ingress.ObjectMeta.Annotations["k8s.ngrok.com/edge-id"] = ngrokEdge.ID

	err := irec.Update(ctx, ingress)
	if err != nil {
		irec.Recorder.Event(ingress, v1.EventTypeWarning, "Failed to update ingress", err.Error())
		return err
	}
	err = setStatus(ctx, irec, ingress, ngrokEdge.ID)
	if err != nil {
		irec.Recorder.Event(ingress, v1.EventTypeWarning, "Failed to set status", err.Error())
		return err
	}
	return nil
}

// Sets the hostname that the tunnel is accessible on to the ingress object status
func setStatus(ctx context.Context, irec *IngressReconciler, ingress *netv1.Ingress, edgeID string) error {
	// TODO: Handle multiple rules
	if ingress.Spec.Rules[0].Host == "" || len(ingress.Status.LoadBalancer.Ingress) != 0 && ingress.Status.LoadBalancer.Ingress[0].Hostname == ingress.Spec.Rules[0].Host {
		return nil
	}

	var hostName string
	if strings.Contains(ingress.Spec.Rules[0].Host, ".ngrok.io") {
		hostName = ingress.Spec.Rules[0].Host
	} else {
		domains, err := irec.NgrokAPIDriver.GetReservedDomains(ctx, edgeID)
		if err != nil {
			return err
		}
		if len(domains) != 0 && domains[0].CNAMETarget != nil {
			hostName = *domains[0].CNAMETarget
		}
	}

	ingress.Status.LoadBalancer.Ingress = []v1.LoadBalancerIngress{
		{
			Hostname: hostName,
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
	// At least 1 route must be declared
	// At least 1 host must be declared
	// TODO: Either limit backends to Service or implement support for Resource
	return nil
}

// Generates a labels map for matching Ngrok Routes to Agent Tunnels
func backendToLabelMap(backend netv1.IngressBackend, ingressName, namespace string) map[string]string {
	return map[string]string{
		"k8s.ngrok.com/ingress-name":      ingressName,
		"k8s.ngrok.com/ingress-namespace": namespace,
		"k8s.ngrok.com/service":           backend.Service.Name,
		"k8s.ngrok.com/port":              strconv.Itoa(int(backend.Service.Port.Number)),
	}
}
