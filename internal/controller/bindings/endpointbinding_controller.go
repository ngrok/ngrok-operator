/*
MIT License

Copyright (c) 2024 ngrok, Inc.

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

package bindings

import (
	"context"
	"fmt"
	"strings"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/go-logr/logr"
	bindingsv1alpha1 "github.com/ngrok/ngrok-operator/api/bindings/v1alpha1"
	"github.com/ngrok/ngrok-operator/internal/controller"
	"github.com/ngrok/ngrok-operator/internal/util"
)

const (
	LabelManagedBy                = "app.kubernetes.io/managed-by"
	LabelEndpointBindingName      = "bindings.k8s.ngrok.com/endpoint-binding-name"
	LabelEndpointBindingNamespace = "bindings.k8s.ngrok.com/endpoint-binding-namespace"
	LabelEndpointURL              = "bindings.k8s.ngrok.com/endpoint-url"

	EndpointBindingOwnerKey            = ".metadata.controller"
	EndpointBindingTargetNamespacePath = "spec.target.namespace"
)

var (
	commonEndpointBindingLabels = map[string]string{
		LabelManagedBy: "ngrok-operator",
	}
)

// PortRangeConfig is a configuration for a port range
// Note: PortRange is inclusive: `[Min, Max]`
type PortRangeConfig struct {
	// Min is the minimum port number
	Min int32

	// Max is the maximum port number
	Max int32
}

// EndpointBindingReconciler reconciles a EndpointBinding object
type EndpointBindingReconciler struct {
	client.Client
	Scheme     *runtime.Scheme
	controller *controller.BaseController[*bindingsv1alpha1.EndpointBinding]

	Log      logr.Logger
	Recorder record.EventRecorder

	// ClusterDomain is the last part of the FQDN for Service DNS in-cluster
	ClusterDomain string

	// PodForwarderLabels are the set of labels for the Pod Forwarders
	PodForwarderLabels []string

	// PortRange is the allocatable port range for the Service definitions to Pod Forwarders
	// TODO(hkatz): Implement Me
	PortRange PortRangeConfig
}

// +kubebuilder:rbac:groups=bindings.k8s.ngrok.com,resources=endpointbindings,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=bindings.k8s.ngrok.com,resources=endpointbindings/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=bindings.k8s.ngrok.com,resources=endpointbindings/finalizers,verbs=update

// SetupWithManager sets up the controller with the Manager.
func (r *EndpointBindingReconciler) SetupWithManager(mgr ctrl.Manager) error {
	r.controller = &controller.BaseController[*bindingsv1alpha1.EndpointBinding]{
		Kube:     r.Client,
		Log:      r.Log,
		Recorder: r.Recorder,

		StatusID:  func(obj *bindingsv1alpha1.EndpointBinding) string { return obj.Name },
		Create:    r.create,
		Update:    r.update,
		Delete:    r.delete,
		ErrResult: r.errResult,
	}

	// create field indexer for to mimic OwnerReferences. We are creating services in other namespaces,
	// so we can't use OwnerReferences.
	err := mgr.GetFieldIndexer().IndexField(context.Background(), &v1.Service{}, EndpointBindingOwnerKey, func(obj client.Object) []string {
		service := obj.(*v1.Service)
		svcLabels := service.GetLabels()
		if svcLabels == nil {
			return nil // skip, service has no labels
		}

		epbName := svcLabels[LabelEndpointBindingName]
		epbNamespace := svcLabels[LabelEndpointBindingNamespace]
		if epbName == "" || epbNamespace == "" {
			return nil // skip, service is not part of an EndpointBinding
		}

		return []string{fmt.Sprintf("%s/%s", epbNamespace, epbName)}
	})

	if err != nil {
		return err
	}

	// Index the EndpointBindings by their target namespace
	err = mgr.GetFieldIndexer().IndexField(context.Background(), &bindingsv1alpha1.EndpointBinding{}, EndpointBindingTargetNamespacePath, func(obj client.Object) []string {
		binding, ok := obj.(*bindingsv1alpha1.EndpointBinding)
		if !ok || binding == nil {
			return nil
		}

		return []string{binding.Spec.Target.Namespace}
	})

	if err != nil {
		return err
	}

	return ctrl.NewControllerManagedBy(mgr).
		For(&bindingsv1alpha1.EndpointBinding{}).
		Watches(
			&v1.Service{},
			handler.EnqueueRequestsFromMapFunc(r.findEndpointBindingsForService),
		).
		Watches(
			&v1.Namespace{},
			handler.EnqueueRequestsFromMapFunc(r.findEndpointBindingsForNamespace),
		).
		Complete(r)
}

// Reconcile turns EndpointBindings into 2 Services
// - ExternalName Target Service in the Target Namespace/Service name pointed at the Upstream Service
// - Upstream Service in the ngrok-op namespace pointed at the Pod Forwarders
// TODO(hkatz) How to handle Service deletion? We delete? Need to delete old ones?
func (r *EndpointBindingReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	return r.controller.Reconcile(ctx, req, new(bindingsv1alpha1.EndpointBinding))
}

func (r *EndpointBindingReconciler) create(ctx context.Context, cr *bindingsv1alpha1.EndpointBinding) error {
	targetService, upstreamService := r.convertEndpointBindingToServices(cr)

	if err := r.createUpstreamService(ctx, cr, upstreamService); err != nil {
		return err
	}

	if err := r.createTargetService(ctx, cr, targetService); err != nil {
		return err
	}

	// TODO(hkatz) Implement Status Updates?

	return nil
}

func (r *EndpointBindingReconciler) createTargetService(ctx context.Context, owner *bindingsv1alpha1.EndpointBinding, service *v1.Service) error {
	if err := r.Client.Create(ctx, service); err != nil {
		r.Recorder.Event(owner, v1.EventTypeWarning, "Created", "Failed to create Target Service")
		r.Log.Error(err, "Failed to create Target Service")
		return err
	}
	r.Recorder.Event(service, v1.EventTypeNormal, "Created", "Created Target Service")
	r.Recorder.Event(owner, v1.EventTypeNormal, "Created", "Created Target Service")
	r.Log.Info("Created Upstream Service", "service", service.Name)

	return nil
}

func (r *EndpointBindingReconciler) createUpstreamService(ctx context.Context, owner *bindingsv1alpha1.EndpointBinding, service *v1.Service) error {
	if err := r.Client.Create(ctx, service); err != nil {
		r.Recorder.Event(owner, v1.EventTypeWarning, "Created", "Failed to create Upstream Service")
		r.Log.Error(err, "Failed to create Upstream Service")
		return err
	}
	r.Recorder.Event(service, v1.EventTypeNormal, "Created", "Created Upstream Service")
	r.Recorder.Event(owner, v1.EventTypeNormal, "Created", "Created Upstream Service")
	r.Log.Info("Created Upstream Service", "service", service.Name)

	return nil
}

func (r *EndpointBindingReconciler) update(ctx context.Context, cr *bindingsv1alpha1.EndpointBinding) error {
	desiredTargetService, desiredUpstreamService := r.convertEndpointBindingToServices(cr)

	var existingTargetService v1.Service
	var existingUpstreamService v1.Service

	// upstream service
	err := r.Get(ctx, client.ObjectKey{Namespace: desiredUpstreamService.Namespace, Name: desiredUpstreamService.Name}, &existingUpstreamService)
	if err != nil {
		if client.IgnoreNotFound(err) == nil {
			// Upstream Service doesn't exist, create it
			r.Log.Info("Unable to find existing Upstream Service, creating...", "name", desiredUpstreamService.Name)
			if err := r.createUpstreamService(ctx, cr, desiredUpstreamService); err != nil {
				return err
			}
		} else {
			// real error
			r.Log.Error(err, "Failed to find existing Upstream Service", "name", cr.Name, "uri", cr.Spec.EndpointURI)
			return err
		}
	} else {
		// update upstream service
		existingUpstreamService.Spec = desiredUpstreamService.Spec
		existingUpstreamService.ObjectMeta.Annotations = desiredUpstreamService.ObjectMeta.Annotations
		existingUpstreamService.ObjectMeta.Labels = desiredUpstreamService.ObjectMeta.Labels
		// don't update status

		if err := r.Client.Update(ctx, &existingUpstreamService); err != nil {
			r.Recorder.Event(&existingUpstreamService, v1.EventTypeWarning, "Updated", "Failed to update Upstream Service")
			r.Log.Error(err, "Failed to update Upstream Service")
			return err
		}
		r.Recorder.Event(&existingUpstreamService, v1.EventTypeNormal, "Updated", "Updated Upstream Service")
	}

	// target service
	err = r.Get(ctx, client.ObjectKey{Namespace: desiredTargetService.Namespace, Name: desiredTargetService.Name}, &existingTargetService)
	if err != nil {
		if client.IgnoreNotFound(err) == nil {
			// Target Service doesn't exist, create it
			r.Log.Info("Unable to find existing Target Service, creating...", "name", desiredTargetService.Name)
			if err := r.createTargetService(ctx, cr, desiredTargetService); err != nil {
				return err
			}
		} else {
			// real error
			r.Log.Error(err, "Failed to find existing Target Service", "name", cr.Name, "uri", cr.Spec.EndpointURI)
			return err
		}
	} else {
		// update target service
		existingTargetService.Spec = desiredTargetService.Spec
		existingTargetService.ObjectMeta.Annotations = desiredTargetService.ObjectMeta.Annotations
		existingTargetService.ObjectMeta.Labels = desiredTargetService.ObjectMeta.Labels
		// don't update status

		if err := r.Client.Update(ctx, &existingTargetService); err != nil {
			r.Recorder.Event(&existingTargetService, v1.EventTypeWarning, "Updated", "Failed to update Target Service")
			r.Log.Error(err, "Failed to update Target Service")
			return err
		}
		r.Recorder.Event(&existingTargetService, v1.EventTypeNormal, "Updated", "Updated Target Service")
	}

	r.Recorder.Event(cr, v1.EventTypeNormal, "Updated", "Updated Services")
	return nil
}

func (r *EndpointBindingReconciler) delete(ctx context.Context, cr *bindingsv1alpha1.EndpointBinding) error {
	targetService, upstreamService := r.convertEndpointBindingToServices(cr)

	if err := r.Client.Delete(ctx, targetService); err != nil {
		if client.IgnoreNotFound(err) == nil {
			return nil
		} else {
			r.Recorder.Event(cr, v1.EventTypeWarning, "Delete", "Failed to delete Target Service")
			r.Log.Error(err, "Failed to delete Target Service")
			return err
		}
	}

	if err := r.Client.Delete(ctx, upstreamService); err != nil {
		if client.IgnoreNotFound(err) == nil {
			return nil
		} else {
			r.Recorder.Event(cr, v1.EventTypeWarning, "Delete", "Failed to delete Upstream Service")
			r.Log.Error(err, "Failed to delete Upstream Service")
			return err
		}
	}

	return nil
}

func (r *EndpointBindingReconciler) errResult(op controller.BaseControllerOp, cr *bindingsv1alpha1.EndpointBinding, err error) (ctrl.Result, error) {
	return ctrl.Result{}, err
}

// convertEndpointBindingToServices converts an EndpointBinding into 2 Services: Target(ExternalName) and Upstream(Pod Forwarders)
func (r *EndpointBindingReconciler) convertEndpointBindingToServices(endpointBinding *bindingsv1alpha1.EndpointBinding) (*v1.Service, *v1.Service) {
	// Send traffic to any Node in the cluster
	internalTrafficPolicy := v1.ServiceInternalTrafficPolicyCluster

	endpointURL := fmt.Sprintf("%s.%s.%s", endpointBinding.Name, endpointBinding.Namespace, r.ClusterDomain)

	podForwarderSelector := map[string]string{}

	for _, label := range r.PodForwarderLabels {
		parts := strings.Split(label, "=")

		if len(parts) != 2 {
			r.Log.Error(fmt.Errorf("invalid Pod Forwarder label: %s", label), "invalid Pod Forwarder label")
		}

		podForwarderSelector[parts[0]] = parts[1]
	}

	thisBindingLabels := map[string]string{
		LabelEndpointBindingName:      endpointBinding.Name,
		LabelEndpointBindingNamespace: endpointBinding.Namespace,
	}

	// Target Labels in order of increasing precedence
	// 1. common labels
	// 2. User's labels
	// 3. Our label selectors (endpoint-binding-name, endpoint-binding-namespace) to mimic OwnerReferences
	targetLabels := util.MergeMaps(
		commonEndpointBindingLabels,
		endpointBinding.Spec.Target.Metadata.Labels,
		thisBindingLabels,
	)

	targetAnnotations := endpointBinding.Spec.Target.Metadata.Annotations

	// targetService represents the user's configured endpoint binding as a Service
	// Clients will send requests to this service: <scheme>://<service>.<namespace>:<port>
	targetService := &v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:        endpointBinding.Spec.Target.Service,
			Namespace:   endpointBinding.Spec.Target.Namespace,
			Labels:      targetLabels,
			Annotations: targetAnnotations,
		},
		Spec: v1.ServiceSpec{
			Type:                  v1.ServiceTypeExternalName,
			ExternalName:          endpointURL,
			InternalTrafficPolicy: &internalTrafficPolicy,
			SessionAffinity:       v1.ServiceAffinityClientIP,
			Ports: []v1.ServicePort{
				{
					Name:     endpointBinding.Spec.Scheme,
					Protocol: v1.Protocol(endpointBinding.Spec.Target.Protocol),
					Port:     endpointBinding.Spec.Target.Port,
				},
			},
		},
	}

	upstreamLabels := util.MergeMaps(commonEndpointBindingLabels, thisBindingLabels)
	upstreamAnnotations := map[string]string{
		// TODO(hkatz) Implement Metadata
		LabelEndpointURL: endpointURL,
	}
	// upstreamService represents the Pod Forwarders as a Service
	// Target Service will point to this Service via an ExternalName
	// This Service will point to the Pod Forwarders' containers on a dedicated allocated port
	upstreamService := &v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:        endpointBinding.Name,
			Namespace:   endpointBinding.Namespace,
			Labels:      upstreamLabels,
			Annotations: upstreamAnnotations,
		},
		Spec: v1.ServiceSpec{
			Type:                  v1.ServiceTypeClusterIP,
			InternalTrafficPolicy: &internalTrafficPolicy,
			SessionAffinity:       v1.ServiceAffinityClientIP,
			Selector:              podForwarderSelector,
			Ports: []v1.ServicePort{
				{
					Name:       endpointBinding.Spec.Scheme,
					Protocol:   v1.Protocol(endpointBinding.Spec.Target.Protocol),
					Port:       endpointBinding.Spec.Target.Port,
					TargetPort: intstr.FromInt(1111), // TODO(hkatz) Implement Port Allocation
				},
			},
		},
	}

	return targetService, upstreamService
}

func (r *EndpointBindingReconciler) findEndpointBindingsForNamespace(ctx context.Context, namespace client.Object) []reconcile.Request {
	nsName := namespace.GetName()
	log := r.Log.WithValues("namespace", nsName)

	log.V(3).Info("Finding endpoint bindings for namespace")
	endpointBindings := &bindingsv1alpha1.EndpointBindingList{}
	listOpts := &client.ListOptions{
		FieldSelector: fields.OneTermEqualSelector(EndpointBindingTargetNamespacePath, nsName),
	}

	err := r.Client.List(ctx, endpointBindings, listOpts)
	if err != nil {
		log.Error(err, "Failed to list endpoint bindings for namespace")
		return []reconcile.Request{}
	}

	requests := make([]reconcile.Request, len(endpointBindings.Items))
	for i, binding := range endpointBindings.Items {
		requests[i] = reconcile.Request{
			NamespacedName: types.NamespacedName{
				Namespace: binding.Namespace,
				Name:      binding.Name,
			},
		}
		log.WithValues("endpoint-binding", map[string]string{
			"namespace": binding.Namespace,
			"name":      binding.Name,
		}).V(3).Info("Namespace change detected, triggering reconciliation for endpoint binding")
	}
	return requests
}

func (r *EndpointBindingReconciler) findEndpointBindingsForService(ctx context.Context, svc client.Object) []reconcile.Request {
	log := r.Log.WithValues("service.name", svc.GetName(), "service.namespace", svc.GetNamespace())
	log.V(3).Info("Finding endpoint bindings for service")

	svcLabels := svc.GetLabels()
	if svcLabels == nil {
		log.V(3).Info("Service has no labels")
		return []reconcile.Request{}
	}

	epbName := svcLabels[LabelEndpointBindingName]
	epbNamespace := svcLabels[LabelEndpointBindingNamespace]
	if epbName == "" || epbNamespace == "" {
		log.V(3).Info("Service is not part of an EndpointBinding")
		return []reconcile.Request{}
	}

	epb := &bindingsv1alpha1.EndpointBinding{}
	err := r.Client.Get(ctx, types.NamespacedName{Namespace: epbNamespace, Name: epbName}, epb)
	if err != nil {
		if client.IgnoreNotFound(err) == nil {
			log.V(3).Info("EndpointBinding not found")
			return []reconcile.Request{}
		}

		log.Error(err, "Failed to get EndpointBinding")
		return []reconcile.Request{}
	}

	return []reconcile.Request{
		{
			NamespacedName: types.NamespacedName{
				Namespace: epb.Namespace,
				Name:      epb.Name,
			},
		},
	}
}
