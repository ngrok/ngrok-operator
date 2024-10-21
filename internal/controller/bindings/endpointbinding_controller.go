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
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/go-logr/logr"
	bindingsv1alpha1 "github.com/ngrok/ngrok-operator/api/bindings/v1alpha1"
	"github.com/ngrok/ngrok-operator/internal/controller"
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

		Create:    r.create,
		Update:    r.update,
		Delete:    r.delete,
		ErrResult: r.errResult,
	}

	return ctrl.NewControllerManagedBy(mgr).
		For(&bindingsv1alpha1.EndpointBinding{}).
		Complete(r)
}

// Reconcile turns EndpointBindings into 2 Services
// - ExternalName Target Service in the Target Namespace/Service name pointed at the Upstream Service
// - Upstream Service in the ngrok-op namespace pointed at the Pod Forwarders
func (r *EndpointBindingReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	return r.controller.Reconcile(ctx, req, new(bindingsv1alpha1.EndpointBinding))
}

func (r *EndpointBindingReconciler) create(ctx context.Context, cr *bindingsv1alpha1.EndpointBinding) error {
	targetService, upstreamService := r.convertEndpointBindingToServices(cr)

	if err := r.Client.Create(ctx, upstreamService); err != nil {
		r.Recorder.Event(cr, v1.EventTypeWarning, "Failed", "Failed to create Upstream Service")
		r.Log.Error(err, "Failed to create Upstream Service")
		return err
	}
	r.Recorder.Event(cr, v1.EventTypeWarning, "Created", "Created Upstream Service")

	if err := r.Client.Create(ctx, targetService); err != nil {
		r.Recorder.Event(cr, v1.EventTypeWarning, "Failed", "Failed to create Target Service")
		r.Log.Error(err, "Failed to create Target Service")
		return err
	}
	r.Recorder.Event(cr, v1.EventTypeWarning, "Created", "Created Target Service")

	// TODO(hkatz) Implement Status Updates?

	return nil
}

// Note: EndpointBindings are unique by their 4-tuple configuration
// Therefore there are no updates but only create/delete/re-create operations
func (r *EndpointBindingReconciler) update(ctx context.Context, cr *bindingsv1alpha1.EndpointBinding) error {
	r.Recorder.Event(cr, v1.EventTypeWarning, "Updated", "No-Op")
	return nil
}

func (r *EndpointBindingReconciler) delete(ctx context.Context, cr *bindingsv1alpha1.EndpointBinding) error {
	targetService, upstreamService := r.convertEndpointBindingToServices(cr)

	if err := r.Client.Delete(ctx, targetService); err != nil {
		r.Recorder.Event(cr, v1.EventTypeWarning, "Failed", "Failed to delete Target Service")
		r.Log.Error(err, "Failed to delete Target Service")
		return err
	}

	if err := r.Client.Delete(ctx, upstreamService); err != nil {
		r.Recorder.Event(cr, v1.EventTypeWarning, "Failed", "Failed to delete Upstream Service")
		r.Log.Error(err, "Failed to delete Upstream Service")
		return err
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

	endpointURL := fmt.Sprintf("%s.%s.%s", endpointBinding.Status.HashedName, endpointBinding.Namespace, r.ClusterDomain)

	podForwarderSelector := map[string]string{}

	for _, label := range r.PodForwarderLabels {
		parts := strings.Split(label, "=")

		if len(parts) != 2 {
			r.Log.Error(fmt.Errorf("invalid Pod Forwarder label: %s", label), "invalid Pod Forwarder label")
		}

		podForwarderSelector[parts[0]] = parts[1]
	}

	targetLabels := endpointBinding.Spec.Target.Metadata.Labels
	targetAnnotations := endpointBinding.Spec.Target.Metadata.Annotations

	finalLabels := targetLabels // no extra labels to integrate for now
	finalAnnotations := map[string]string{
		"managed-by": "ngrok-operator", // TODO(hkatz) extra metadata?
		"points-to":  endpointBinding.Status.HashedName,
	}

	for k, v := range targetAnnotations {
		finalAnnotations[k] = v
	}

	// targetService represents the user's configured endpoint binding as a Service
	// Clients will send requests to this service: <scheme>://<service>.<namespace>:<port>
	targetService := &v1.Service{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "v1",
			Kind:       "Service",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:        endpointBinding.Spec.Target.Service,
			Namespace:   endpointBinding.Spec.Target.Namespace,
			Labels:      finalLabels,
			Annotations: finalAnnotations,
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

	// upstreamService represents the Pod Forwarders as a Service
	// Target Service will point to this Service via an ExternalName
	// This Service will point to the Pod Forwarders' containers on a dedicated allocated port
	upstreamService := &v1.Service{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "v1",
			Kind:       "Service",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      endpointBinding.Status.HashedName,
			Namespace: endpointBinding.Namespace,
			Annotations: map[string]string{
				// TODO(hkatz) Implement Metadata
				"managed-by":    "ngrok-operator", // TODO(hkatz) extra metadata?
				"receives-from": endpointURL,
			},
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
