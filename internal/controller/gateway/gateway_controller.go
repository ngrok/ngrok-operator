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
	"strings"

	v1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/events"
	"k8s.io/utils/ptr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"

	"github.com/go-logr/logr"
	ingressv1alpha1 "github.com/ngrok/ngrok-operator/api/ingress/v1alpha1"
	ngrokv1 "github.com/ngrok/ngrok-operator/api/ngrok/v1"
	"github.com/ngrok/ngrok-operator/internal/controller"
	"github.com/ngrok/ngrok-operator/internal/deprecation"
	"github.com/ngrok/ngrok-operator/internal/util"
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
	Recorder events.EventRecorder
	Driver   *managerdriver.Driver
	// DrainState is used to check if the operator is draining.
	// If draining, non-delete reconciles are skipped to prevent new finalizers.
	DrainState controller.DrainState
}

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

		return managerdriver.HandleSyncResult(r.Driver.Sync(ctx, r.Client))
	}

	if err != nil {
		return ctrl.Result{}, err
	}

	// If the gateway is being deleted, remove the finalizer, delete it from the store and
	// return early.
	if controller.IsDelete(gw) {
		log.Info("Deleting gateway from store")
		if err := util.RemoveAndSyncFinalizer(ctx, r.Client, gw); err != nil {
			log.Error(err, "Failed to remove finalizer")
			return ctrl.Result{}, err
		}

		return ctrl.Result{}, r.Driver.DeleteGateway(gw)
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

	// LEGACY-PREFIX-MIGRATION (read-side cleanup): drop this scan
	deprecation.ScanAnnotations(log, r.Recorder, gw)
	// LEGACY-PREFIX-MIGRATION (read-side cleanup): drop this warning helper
	r.warnIfLegacyTLSOptions(log, gw)

	// Skip non-delete reconciles during drain to prevent adding new finalizers
	if controller.IsDraining(ctx, r.DrainState) {
		log.V(1).Info("Draining, skipping non-delete reconcile")
		return ctrl.Result{}, nil
	}

	// The object is not being deleted, so register and sync finalizer
	if err := util.RegisterAndSyncFinalizer(ctx, r.Client, gw); err != nil {
		log.Error(err, "Failed to register finalizer")
		return ctrl.Result{}, err
	}

	// The Secret and ConfigMap watches only pass objects some Gateway already references, so ones
	// created before this Gateway (or in the same apply) were never stored. Load them now.
	if err := r.storeReferencedObjects(ctx, gw); err != nil {
		return ctrl.Result{}, err
	}

	// Status is written by the driver at the end of Sync, from the whole store.
	// Update the gateway in the store
	if _, err := r.Driver.UpdateGateway(gw); err != nil {
		log.Error(err, "Failed to update gateway in store")
		return ctrl.Result{}, err
	}

	return managerdriver.HandleSyncResult(r.Driver.Sync(ctx, r.Client))
}

// SetupWithManager sets up the controller with the Manager.
func (r *GatewayReconciler) SetupWithManager(mgr ctrl.Manager) error {
	storedResources := []client.Object{
		&gatewayv1.GatewayClass{},
		&gatewayv1.HTTPRoute{},
		// &corev1.Service{},
		&ngrokv1.Domain{},
		// LEGACY-domain-kind: drop the legacy Domain entry at cleanup.
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

	// The claim that gets a Gateway its domain from ngrok (see pkg/managerdriver/gateway_address.go).
	// When ngrok assigns the domain, the Gateway's reconcile pins it.
	bldr = bldr.Watches(
		&ngrokv1.AgentEndpoint{},
		handler.EnqueueRequestsFromMapFunc(func(_ context.Context, obj client.Object) []reconcile.Request {
			name, ok := obj.GetLabels()[managerdriver.GatewayNameLabel]
			if !ok {
				return nil
			}
			return []reconcile.Request{{Name: name, Namespace: obj.GetNamespace()}}
		}),
	)

	return bldr.Complete(r)
}

// storeReferencedObjects puts the Secrets and ConfigMaps a Gateway references into the driver's store.
// A reference to an object that doesn't exist is left out; status reports it.
func (r *GatewayReconciler) storeReferencedObjects(ctx context.Context, gw *gatewayv1.Gateway) error {
	refs := []client.Object{}
	addSecret := func(name gatewayv1.ObjectName, namespace *gatewayv1.Namespace) {
		ns := gw.Namespace
		if namespace != nil && *namespace != "" {
			ns = string(*namespace)
		}
		refs = append(refs, &v1.Secret{Name: string(name), Namespace: ns})
	}
	addConfigMap := func(ref gatewayv1.ObjectReference) {
		if string(ref.Kind) != "ConfigMap" {
			return
		}
		ns := gw.Namespace
		if ref.Namespace != nil && *ref.Namespace != "" {
			ns = string(*ref.Namespace)
		}
		refs = append(refs, &v1.ConfigMap{Name: string(ref.Name), Namespace: ns})
	}

	for _, l := range gw.Spec.Listeners {
		if l.TLS == nil {
			continue
		}
		for _, ref := range l.TLS.CertificateRefs {
			if (ref.Group == nil || *ref.Group == "") && (ref.Kind == nil || *ref.Kind == "Secret") {
				addSecret(ref.Name, ref.Namespace)
			}
		}
	}
	if tls := gw.Spec.TLS; tls != nil {
		if tls.Backend != nil && tls.Backend.ClientCertificateRef != nil {
			addSecret(tls.Backend.ClientCertificateRef.Name, tls.Backend.ClientCertificateRef.Namespace)
		}
		if tls.Frontend != nil {
			if tls.Frontend.Default.Validation != nil {
				for _, ref := range tls.Frontend.Default.Validation.CACertificateRefs {
					addConfigMap(ref)
				}
			}
			for _, port := range tls.Frontend.PerPort {
				if port.TLS.Validation != nil {
					for _, ref := range port.TLS.Validation.CACertificateRefs {
						addConfigMap(ref)
					}
				}
			}
		}
	}

	for _, obj := range refs {
		if err := r.Client.Get(ctx, client.ObjectKeyFromObject(obj), obj); err != nil {
			if apierrors.IsNotFound(err) {
				continue
			}
			return err
		}
		if err := r.Driver.GetStore().Update(obj); err != nil {
			return err
		}
	}
	return nil
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
		if gw.Spec.TLS != nil && gw.Spec.TLS.Backend != nil && gw.Spec.TLS.Backend.ClientCertificateRef != nil {
			certRef := gw.Spec.TLS.Backend.ClientCertificateRef
			if certRefMatches(certRef.Name, certRef.Namespace, gw.Namespace, secret.Name, secret.Namespace) &&
				secret.Type == v1.SecretTypeTLS {
				return true
			}
		}
		for _, listener := range gw.Spec.Listeners {
			if listener.TLS == nil {
				continue
			}
			for _, certRef := range listener.TLS.CertificateRefs {
				if certRefMatches(certRef.Name, certRef.Namespace, gw.Namespace, secret.Name, secret.Namespace) &&
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
		if gw.Spec.TLS == nil || gw.Spec.TLS.Frontend == nil {
			continue
		}
		if gw.Spec.TLS.Frontend.Default.Validation != nil {
			for _, certRef := range gw.Spec.TLS.Frontend.Default.Validation.CACertificateRefs {
				if certRefMatches(certRef.Name, certRef.Namespace, gw.Namespace, cm.Name, cm.Namespace) &&
					string(certRef.Kind) == "ConfigMap" {
					return true
				}
			}
		}
		for _, portCfg := range gw.Spec.TLS.Frontend.PerPort {
			if portCfg.TLS.Validation == nil {
				continue
			}
			for _, certRef := range portCfg.TLS.Validation.CACertificateRefs {
				if certRefMatches(certRef.Name, certRef.Namespace, gw.Namespace, cm.Name, cm.Namespace) &&
					string(certRef.Kind) == "ConfigMap" {
					return true
				}
			}
		}
	}
	return false
}

// certRefMatches reports whether a Gateway cert reference points to the named
// object in the given namespace. A nil ref namespace defaults to gwNs.
func certRefMatches(refName gatewayv1.ObjectName, refNs *gatewayv1.Namespace, gwNs, objName, objNs string) bool {
	return string(refName) == objName &&
		string(ptr.Deref(refNs, gatewayv1.Namespace(gwNs))) == objNs
}

// LEGACY-PREFIX-MIGRATION: BEGIN (read-side cleanup deletes this helper)

// warnIfLegacyTLSOptions emits one Warning event when any listener on the
// Gateway still uses k8s.ngrok.com/terminate-tls.* TLS option keys.
func (r *GatewayReconciler) warnIfLegacyTLSOptions(log logr.Logger, gw *gatewayv1.Gateway) {
	for _, listener := range gw.Spec.Listeners {
		if listener.TLS == nil {
			continue
		}
		for key := range listener.TLS.Options {
			if strings.HasPrefix(string(key), managerdriver.LegacyTLSOptionKeyPrefix) {
				log.Info("legacy TLS option key in use; please migrate",
					"legacyPrefix", managerdriver.LegacyTLSOptionKeyPrefix,
					"newPrefix", managerdriver.TLSOptionKeyPrefix)
				if r.Recorder != nil {
					r.Recorder.Eventf(gw, nil, v1.EventTypeWarning, deprecation.ReasonLegacyAnnotation, "Reconcile",
						"TLS option keys with the %q prefix are deprecated and support will be removed in ngrok-operator 1.0; rename them to the %q prefix", managerdriver.LegacyTLSOptionKeyPrefix, managerdriver.TLSOptionKeyPrefix)
				}
				return
			}
		}
	}
}

// LEGACY-PREFIX-MIGRATION: END
