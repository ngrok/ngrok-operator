/*
MIT License

Copyright (c) 2022 ngrok, Inc.

Permission is hereby granted, free of charge, to any person obtaining a copy
of this software and associated documentation files (the "Software"), to deal
in the Software without restriction, including without limitation the rights
to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
copies of the Software, and to permit persons to whom the Software is
furnished to do so, subject to the following conditions:

The above copyright notice and this permission notice shall be included in all
copies or substantial portions of the Software.

THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
SOFTWARE.
*/

package gateway

import (
	"context"
	"errors"
	"reflect"

	v1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"
	"k8s.io/client-go/util/retry"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"

	"github.com/go-logr/logr"
	ingressv1alpha1 "github.com/ngrok/ngrok-operator/api/ingress/v1alpha1"
	"github.com/ngrok/ngrok-operator/internal/controller"
	"github.com/ngrok/ngrok-operator/pkg/managerdriver"
	"sigs.k8s.io/controller-runtime/pkg/builder"
)

const (
	ControllerName gatewayv1.GatewayController = "ngrok.com/gateway-controller"
)

// GatewayReconciler reconciles a Gateway object
type GatewayReconciler struct {
	client.Client

	Log      logr.Logger
	Scheme   *runtime.Scheme
	Recorder record.EventRecorder
	Driver   *managerdriver.Driver
}

// +kubebuilder:rbac:groups="",resources=events,verbs=create;patch
// +kubebuilder:rbac:groups="",resources=configmaps,verbs=get;list;watch;create;update;delete
// +kubebuilder:rbac:groups="",resources=secrets,verbs=get;list;watch
// +kubebuilder:rbac:groups="",resources=services,verbs=get;list;watch
// +kubebuilder:rbac:groups=gateway.networking.k8s.io,resources=gateways,verbs=get;list;watch;update
// +kubebuilder:rbac:groups=gateway.networking.k8s.io,resources=gateways/status,verbs=get;list;watch;update
// +kubebuilder:rbac:groups=gateway.networking.k8s.io,resources=gatewayclasses,verbs=get;list;watch;update
// +kubebuilder:rbac:groups=gateway.networking.k8s.io,resources=gatewayclasses/status,verbs=get;list;watch;update

func (r *GatewayReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := ctrl.LoggerFrom(ctx).WithValues("Gateway", req.NamespacedName)
	ctx = ctrl.LoggerInto(ctx, log)

	gw := new(gatewayv1.Gateway)
	err := r.Client.Get(ctx, req.NamespacedName, gw)

	if apierrors.IsNotFound(err) {
		if err := r.Driver.DeleteNamedGateway(req.NamespacedName); err != nil {
			log.Error(err, "Failed to delete gateway from store")
			return ctrl.Result{}, err
		}

		err = r.Driver.Sync(ctx, r.Client)
		if err != nil {
			log.Error(err, "Failed to sync after removing gateway from store")
			return ctrl.Result{}, err
		}

		return ctrl.Result{}, nil
	}

	if err != nil {
		return ctrl.Result{}, err
	}

	if controller.IsCleanedUp(gw) {
		log.V(3).Info("Finalizer not present, skipping cleanup as already done")
		return ctrl.Result{}, nil
	}

	// If the gateway is being deleted, remove the finalizer, delete it from the store and
	// return early.
	if controller.IsDelete(gw) || controller.HasCleanupAnnotation(gw) {
		log.Info("Deleting gateway from store")

		if err := r.Driver.DeleteGateway(gw); err != nil {
			log.Error(err, "Failed to delete gateway from store")
			return ctrl.Result{}, err
		}

		gw.Status = gatewayv1.GatewayStatus{}
		if err := r.updateGatewayStatus(ctx, gw); err != nil {
			log.Error(err, "Failed to update gateway status")
			return ctrl.Result{}, err
		}

		if err := controller.RemoveAndSyncFinalizer(ctx, r.Client, gw); err != nil {
			log.Error(err, "Failed to remove finalizer")
			return ctrl.Result{}, err
		}

		return ctrl.Result{}, nil
	}

	log.V(1).Info("verifying gatewayclass")
	gwClass := &gatewayv1.GatewayClass{}
	if err := r.Client.Get(ctx, client.ObjectKey{Name: string(gw.Spec.GatewayClassName)}, gwClass); err != nil {
		log.V(1).Info("could not retrieve gatewayclass for gateway", "gatewayclass", gwClass.Spec.ControllerName)
		return ctrl.Result{}, err
	}

	if !ShouldHandleGatewayClass(gwClass) {
		log.V(1).Info("unsupported gatewayclass controllername, ignoring", "gatewayclass", gwClass.Name, "controllername", gwClass.Spec.ControllerName)
		return ctrl.Result{}, nil
	}

	// The object is not being deleted, so register and sync finalizer
	if err := controller.RegisterAndSyncFinalizer(ctx, r.Client, gw); err != nil {
		log.Error(err, "Failed to register finalizer")
		return ctrl.Result{}, err
	}

	// Validate the Gateway, conditionally modifying the status of the Gateway
	_ = r.validateGateway(ctx, gw)

	// Update the gateway in the store
	if _, err := r.Driver.UpdateGateway(gw); err != nil {
		log.Error(err, "Failed to update gateway in store")
		return ctrl.Result{}, err
	}

	if err := r.Driver.Sync(ctx, r.Client); err != nil {
		log.Error(err, "Failed to sync")
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

const (
	ListenerReasonHostnameRequired gatewayv1.ListenerConditionReason = "HostnameRequired"
)

// validateGateway validates the Gateway object, it will modify the status of the Gateway
// to set the conditions if there are any errors. You could pass in a copy of the gateway object so
// that the status can be modified without modifying the original object.
func (r *GatewayReconciler) validateGateway(ctx context.Context, gw *gatewayv1.Gateway) error {
	log := ctrl.LoggerFrom(ctx)
	log.V(5).Info("Validating Gateway")

	var changed bool
	setStatusCondition := func(conditions *[]metav1.Condition, newCondition metav1.Condition) {
		if meta.SetStatusCondition(conditions, newCondition) {
			changed = true
		}
	}

	// In order to preserve existing status conditions, we need copy an existing listener status
	// if there is one.
	listenerStatusByName := make(map[gatewayv1.SectionName]gatewayv1.ListenerStatus)
	for _, l := range gw.Status.Listeners {
		listenerStatusByName[l.Name] = l
	}

	// Copy the existing addresses and conditions from the existing status
	newStatus := gatewayv1.GatewayStatus{
		Addresses:  gw.Status.Addresses,
		Conditions: gw.Status.Conditions,
		Listeners:  []gatewayv1.ListenerStatus{},
	}

	// Listener Rules specific to the ngrok operator & ngrok product
	// (1) ngrok does not support listening for HTTP/HTTPS traffic on arbitrary ports.
	// Listeners that specify protocolTypeHTTP must listen on port 80.
	// Listeners that specify protocolTypeHTTPS must listen on port 443.
	// (2) ngrok does not support HTTP/HTTPS listeners without a hostname.
	for _, l := range gw.Spec.Listeners {
		listenerStatus, ok := listenerStatusByName[l.Name]
		if !ok {
			listenerStatus = gatewayv1.ListenerStatus{
				Name:           l.Name,
				SupportedKinds: []gatewayv1.RouteGroupKind{},
				AttachedRoutes: 0,
				Conditions:     []metav1.Condition{},
			}
		}

		listenerValid := false

		switch l.Protocol {
		case gatewayv1.HTTPProtocolType:
			switch {
			case l.Port != 80:
				setStatusCondition(&listenerStatus.Conditions, r.newListenerCondition(
					gw,
					gatewayv1.ListenerConditionAccepted,
					gatewayv1.ListenerReasonPortUnavailable,
					"ngrok only supports HTTP on port 80",
				))
			case l.Hostname == nil || *l.Hostname == "":
				setStatusCondition(&listenerStatus.Conditions, r.newListenerCondition(
					gw,
					gatewayv1.ListenerConditionAccepted,
					ListenerReasonHostnameRequired,
					"ngrok does not support HTTP listeners without a hostname",
				))
			default:
				listenerValid = true
			}
		case gatewayv1.HTTPSProtocolType:
			switch {

			case l.Port != 443:
				setStatusCondition(&listenerStatus.Conditions, r.newListenerCondition(
					gw,
					gatewayv1.ListenerConditionAccepted,
					gatewayv1.ListenerReasonPortUnavailable,
					"ngrok only supports HTTPS on port 443",
				))
			case l.Hostname == nil || *l.Hostname == "":
				setStatusCondition(&listenerStatus.Conditions, r.newListenerCondition(
					gw,
					gatewayv1.ListenerConditionAccepted,
					ListenerReasonHostnameRequired,
					"ngrok does not support HTTPS listeners without a hostname",
				))
			default:
				listenerValid = true
			}
		case gatewayv1.UDPProtocolType:
			setStatusCondition(&listenerStatus.Conditions, r.newListenerCondition(
				gw,
				gatewayv1.ListenerConditionAccepted,
				gatewayv1.ListenerReasonUnsupportedProtocol,
				"ngrok does not currently support UDP listeners",
			))
		default:
			listenerValid = true
		}

		if listenerValid {
			setStatusCondition(&listenerStatus.Conditions, r.newListenerCondition(
				gw,
				gatewayv1.ListenerConditionAccepted,
				gatewayv1.ListenerReasonAccepted,
				"listener accepted by the ngrok operator",
			))

			programmed := meta.FindStatusCondition(listenerStatus.Conditions, string(gatewayv1.ListenerConditionProgrammed))
			if programmed == nil || programmed.Status != metav1.ConditionTrue {
				setStatusCondition(&listenerStatus.Conditions, r.newListenerCondition(
					gw,
					gatewayv1.ListenerConditionProgrammed,
					gatewayv1.ListenerReasonPending,
					"listener is pending programming by the ngrok operator",
				))
			}
		} else {
			setStatusCondition(&listenerStatus.Conditions, r.newListenerCondition(
				gw,
				gatewayv1.ListenerConditionProgrammed,
				gatewayv1.ListenerReasonInvalid,
				"listener is not valid",
			))
		}

		newStatus.Listeners = append(newStatus.Listeners, listenerStatus)
	}

	// Check if we have at least one valid(accepted) listener
	hasValidListener := false
	for _, l := range newStatus.Listeners {
		if meta.IsStatusConditionTrue(l.Conditions, string(gatewayv1.ListenerConditionAccepted)) {
			hasValidListener = true
			break
		}
	}

	// If we have at least one valid listener, we will accept the gateway.
	if !hasValidListener {
		setStatusCondition(&newStatus.Conditions, metav1.Condition{
			Type:               string(gatewayv1.GatewayConditionAccepted),
			Status:             metav1.ConditionFalse,
			Reason:             string(gatewayv1.GatewayReasonListenersNotValid),
			Message:            "gateway listeners are not valid",
			ObservedGeneration: gw.Generation,
		})
	} else {
		setStatusCondition(&newStatus.Conditions, metav1.Condition{
			Type:               string(gatewayv1.GatewayConditionAccepted),
			Status:             metav1.ConditionTrue,
			Reason:             string(gatewayv1.GatewayReasonAccepted),
			Message:            "gateway accepted by the ngrok controller",
			ObservedGeneration: gw.Generation,
		})
	}

	var gatewayValidationError error
	if meta.IsStatusConditionTrue(newStatus.Conditions, string(gatewayv1.GatewayConditionAccepted)) {
		gatewayValidationError = errors.New("gateway validation failed")
	}

	gw.Status = newStatus

	if changed {
		log.V(1).Info("Gateway validation changed, updating status")
		if err := r.updateGatewayStatus(ctx, gw); err != nil {
			log.Error(err, "Failed to update gateway status")
			return err
		}
	}

	return gatewayValidationError
}

func (r GatewayReconciler) newListenerCondition(gw *gatewayv1.Gateway, t gatewayv1.ListenerConditionType, reason gatewayv1.ListenerConditionReason, msg string) metav1.Condition {
	status := metav1.ConditionTrue
	if reason != gatewayv1.ListenerReasonAccepted && reason != gatewayv1.ListenerReasonResolvedRefs {
		status = metav1.ConditionFalse
	}
	return metav1.Condition{
		Type:               string(t),
		Status:             status,
		ObservedGeneration: gw.Generation,
		Reason:             string(reason),
		Message:            msg,
	}
}

func (r *GatewayReconciler) updateGatewayStatus(ctx context.Context, gw *gatewayv1.Gateway) error {
	log := ctrl.LoggerFrom(ctx)
	log.V(1).Info("Updating Gateway status")

	err := retry.RetryOnConflict(retry.DefaultRetry, func() error {
		current := new(gatewayv1.Gateway)
		err := r.Client.Get(ctx, client.ObjectKeyFromObject(gw), current)

		if err != nil {
			return err
		}

		if reflect.DeepEqual(current.Status, gw.Status) {
			log.V(1).Info("Gateway status is already up to date")
			return nil
		}

		current = current.DeepCopy()
		current.Status = gw.Status

		return r.Client.Status().Update(ctx, current)
	})

	if err != nil {
		log.Error(err, "Failed to update gateway status")
	}

	return nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *GatewayReconciler) SetupWithManager(mgr ctrl.Manager) error {
	storedResources := []client.Object{
		&gatewayv1.GatewayClass{},
		&ingressv1alpha1.Domain{},
	}

	bldr := ctrl.NewControllerManagedBy(mgr).For(&gatewayv1.Gateway{})
	for _, obj := range storedResources {
		bldr = bldr.Watches(
			obj,
			managerdriver.NewControllerEventHandler(
				obj.GetObjectKind().GroupVersionKind().Kind,
				r.Driver,
				r.Client,
			),
		)
	}

	// Add watch for Secrets—but filter events so that only Secrets
	// referenced by Gateway TLS certificateRefs trigger reconciliation.
	bldr = bldr.Watches(
		&v1.Secret{},
		managerdriver.NewControllerEventHandler("Secret", r.Driver, r.Client),
		builder.WithPredicates(&referencedResourcePredicate{client: r.Client}),
	)

	// Add watch for ConfigMaps—but filter events so that only ConfigMaps
	// referenced by Gateway TLS frontendValidation.certificateRefs trigger reconciliation.
	bldr = bldr.Watches(
		&v1.ConfigMap{},
		managerdriver.NewControllerEventHandler("ConfigMap", r.Driver, r.Client),
		builder.WithPredicates(&referencedResourcePredicate{client: r.Client}),
	)

	return bldr.Complete(r)
}

// referencedResourcePredicate only allows Secrets or ConfigMaps
// that are referenced in a Gateway's TLS certificateRefs
type referencedResourcePredicate struct {
	client client.Client
}

func (p referencedResourcePredicate) Create(e event.CreateEvent) bool {
	return p.isReferenced(e.Object)
}

func (p referencedResourcePredicate) Update(e event.UpdateEvent) bool {
	return p.isReferenced(e.ObjectNew)
}

func (p referencedResourcePredicate) Delete(e event.DeleteEvent) bool {
	return p.isReferenced(e.Object)
}

func (p referencedResourcePredicate) Generic(e event.GenericEvent) bool {
	return p.isReferenced(e.Object)
}

func (p referencedResourcePredicate) isReferenced(obj client.Object) bool {
	switch o := obj.(type) {
	case *v1.Secret:
		return secretReferencedByGateway(o, p.client)
	case *v1.ConfigMap:
		return configMapReferencedByGateway(o, p.client)
	default:
		return false
	}
}

// secretReferencedByGateway returns true if the provided secret is referenced
// by any Gateway in the same namespace via a TLS certificateRefs entry
func secretReferencedByGateway(secret *v1.Secret, c client.Client) bool {
	var gwList gatewayv1.GatewayList
	if err := c.List(context.TODO(), &gwList); err != nil {
		return false
	}
	for _, gw := range gwList.Items {
		// For the backend CertificateRefs, we don't strictly need to watch them here (in the manager pods) since the Gateway API config gets translated
		// into similar references set on the generated AgentEndpoints and which get processed by the agent pods, but having the validation for whether or not the referenced secrets exist
		// in the same layer as the rest of the translation offers a better user experience with understanding errors with their resources and why they happened.
		if gw.Spec.BackendTLS != nil && gw.Spec.BackendTLS.ClientCertificateRef != nil {
			certRef := gw.Spec.BackendTLS.ClientCertificateRef
			if certRef.Namespace == nil {
				certNs := gatewayv1.Namespace(gw.Namespace)
				certRef.Namespace = &certNs
			}

			if string(certRef.Name) == secret.Name &&
				string(*certRef.Namespace) == secret.Namespace &&
				secret.Type == v1.SecretTypeTLS {
				return true
			}
		}
		for _, listener := range gw.Spec.Listeners {
			if listener.TLS == nil {
				continue
			}
			for _, certRef := range listener.TLS.CertificateRefs {
				if certRef.Namespace == nil {
					certNs := gatewayv1.Namespace(gw.Namespace)
					certRef.Namespace = &certNs
				}
				if string(certRef.Name) == secret.Name &&
					string(*certRef.Namespace) == secret.Name &&
					secret.Type == v1.SecretTypeTLS {
					return true
				}
			}
		}
	}
	return false
}

// configMapReferencedByGateway returns true if the provided ConfigMap is referenced
// by any Gateway in the same namespace via a TLS frontendValidation certificateRefs entry
func configMapReferencedByGateway(cm *v1.ConfigMap, c client.Client) bool {
	var gwList gatewayv1.GatewayList
	if err := c.List(context.TODO(), &gwList); err != nil {
		return false
	}
	for _, gw := range gwList.Items {
		for _, listener := range gw.Spec.Listeners {
			if listener.TLS == nil || listener.TLS.FrontendValidation == nil {
				continue
			}
			for _, certRef := range listener.TLS.FrontendValidation.CACertificateRefs {
				if certRef.Namespace == nil {
					certNs := gatewayv1.Namespace(gw.Namespace)
					certRef.Namespace = &certNs
				}
				if string(certRef.Name) == cm.Name &&
					string(*certRef.Namespace) == cm.Namespace &&
					string(certRef.Kind) == "ConfigMap" {
					return true
				}
			}
		}
	}
	return false
}
