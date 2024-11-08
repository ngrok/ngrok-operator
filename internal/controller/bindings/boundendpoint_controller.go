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
	"net"
	"net/url"
	"time"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/go-logr/logr"
	bindingsv1alpha1 "github.com/ngrok/ngrok-operator/api/bindings/v1alpha1"
	"github.com/ngrok/ngrok-operator/internal/controller"
	"github.com/ngrok/ngrok-operator/internal/util"
)

const (
	LabelManagedBy              = "app.kubernetes.io/managed-by"
	LabelBoundEndpointName      = "bindings.k8s.ngrok.com/endpoint-binding-name"
	LabelBoundEndpointNamespace = "bindings.k8s.ngrok.com/endpoint-binding-namespace"
	LabelEndpointURL            = "bindings.k8s.ngrok.com/endpoint-url"

	// Used for indexing Services by their BoundEndpoint owner. Not an actual
	// field on the Service object.
	BoundEndpointOwnerKey = ".metadata.controller"
	// Used for indexing BoundEndpoints by their target namespace. Not an actual
	// field on the BoundEndpoint object.
	BoundEndpointTargetNamespacePath = ".spec.targetNamespace"

	// TODO(hkatz) ngrok-error-codes
	NgrokErrorUpstreamServiceCreateFailed = "ERR_NGROK_0001"
	NgrokErrorTargetServiceCreateFailed   = "ERR_NGROK_0002"
	NgrokErrorFailedToBind                = "ERR_NGROK_003"
	NgrokErrorNotAllowed                  = "ERR_NGROK_004"
)

var (
	commonBoundEndpointLabels = map[string]string{
		LabelManagedBy: "ngrok-operator",
	}
)

// BoundEndpointReconciler reconciles a BoundEndpoint object
type BoundEndpointReconciler struct {
	client.Client
	Scheme     *runtime.Scheme
	controller *controller.BaseController[*bindingsv1alpha1.BoundEndpoint]

	Log      logr.Logger
	Recorder record.EventRecorder

	// ClusterDomain is the last part of the FQDN for Service DNS in-cluster
	ClusterDomain string

	// UpstreamServiceLabelSelectors are the set of labels for the Pod Forwarders
	UpstreamServiceLabelSelector map[string]string
}

// +kubebuilder:rbac:groups=bindings.k8s.ngrok.com,resources=boundendpoints,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=bindings.k8s.ngrok.com,resources=boundendpoints/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=bindings.k8s.ngrok.com,resources=boundendpoints/finalizers,verbs=update

// SetupWithManager sets up the controller with the Manager.
func (r *BoundEndpointReconciler) SetupWithManager(mgr ctrl.Manager) error {
	r.controller = &controller.BaseController[*bindingsv1alpha1.BoundEndpoint]{
		Kube:     r.Client,
		Log:      r.Log,
		Recorder: r.Recorder,

		StatusID:  func(obj *bindingsv1alpha1.BoundEndpoint) string { return obj.Name },
		Create:    r.create,
		Update:    r.update,
		Delete:    r.delete,
		ErrResult: r.errResult,
	}

	// create field indexer for to mimic OwnerReferences. We are creating services in other namespaces,
	// so we can't use OwnerReferences.
	err := mgr.GetFieldIndexer().IndexField(context.Background(), &v1.Service{}, BoundEndpointOwnerKey, func(obj client.Object) []string {
		service := obj.(*v1.Service)
		svcLabels := service.GetLabels()
		if svcLabels == nil {
			return nil // skip, service has no labels
		}

		epbName := svcLabels[LabelBoundEndpointName]
		epbNamespace := svcLabels[LabelBoundEndpointNamespace]
		if epbName == "" || epbNamespace == "" {
			return nil // skip, service is not part of an BoundEndpoint
		}

		return []string{fmt.Sprintf("%s/%s", epbNamespace, epbName)}
	})

	if err != nil {
		return err
	}

	// Index the BoundEndpoints by their target namespace
	err = mgr.GetFieldIndexer().IndexField(context.Background(), &bindingsv1alpha1.BoundEndpoint{}, BoundEndpointTargetNamespacePath, func(obj client.Object) []string {
		binding, ok := obj.(*bindingsv1alpha1.BoundEndpoint)
		if !ok || binding == nil {
			return nil
		}

		return []string{binding.Spec.Target.Namespace}
	})

	if err != nil {
		return err
	}

	return ctrl.NewControllerManagedBy(mgr).
		For(&bindingsv1alpha1.BoundEndpoint{}).
		Watches(
			&v1.Service{},
			r.controller.NewEnqueueRequestForMapFunc(r.findBoundEndpointsForService),
		).
		Watches(
			&v1.Namespace{},
			r.controller.NewEnqueueRequestForMapFunc(r.findBoundEndpointsForNamespace),
		).
		Complete(r)
}

// Reconcile turns BoundEndpoints into 2 Services
// - ExternalName Target Service in the Target Namespace/Service name pointed at the Upstream Service
// - Upstream Service in the ngrok-op namespace pointed at the Pod Forwarders
func (r *BoundEndpointReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	cr := &bindingsv1alpha1.BoundEndpoint{}
	if ctrlErr, err := r.controller.Reconcile(ctx, req, cr); err != nil {
		return ctrlErr, err
	}

	// update ngrok api resource status on upsert
	if controller.IsUpsert(cr) {
		if err := postBoundEndpointUpdateToNgrokAPI(ctx, cr); err != nil {
			return controller.CtrlResultForErr(r.controller.ReconcileStatus(ctx, cr, err))
		}
	}

	// success
	return ctrl.Result{}, nil
}

func (r *BoundEndpointReconciler) create(ctx context.Context, cr *bindingsv1alpha1.BoundEndpoint) error {
	targetService, upstreamService := r.convertBoundEndpointToServices(cr)

	// binding is not allowed to be created
	if !cr.Spec.Allowed {
		return r.denyBoundEndpoint(ctx, cr)
	}

	if err := r.createUpstreamService(ctx, cr, upstreamService); err != nil {
		return r.controller.ReconcileStatus(ctx, cr, err)
	}

	if err := r.createTargetService(ctx, cr, targetService); err != nil {
		return r.controller.ReconcileStatus(ctx, cr, err)
	}

	if err := r.tryToBindEndpoint(ctx, cr); err != nil {
		return r.controller.ReconcileStatus(ctx, cr, err)
	}

	return r.controller.ReconcileStatus(ctx, cr, nil)
}

// setEndpointsStatus sets the status of every endpoint on boundEndpoint to the desired status
// Note: All endpoints share the same status since they are represented by the same resources (Target/Upstream Services)
func setEndpointsStatus(boundEndpoint *bindingsv1alpha1.BoundEndpoint, desired *bindingsv1alpha1.BindingEndpoint) {
	for i := range boundEndpoint.Status.Endpoints {
		endpoint := &boundEndpoint.Status.Endpoints[i]
		endpoint.Status = desired.Status
		endpoint.ErrorCode = desired.ErrorCode
		endpoint.ErrorMessage = desired.ErrorMessage
	}
}

// postBoundEndpointUpdateToNgrokAPI sends an update to the ngrok API to update the endpoint binding and status fields
func postBoundEndpointUpdateToNgrokAPI(ctx context.Context, boundEndpoint *bindingsv1alpha1.BoundEndpoint) error {
	// TODO(hkatz) Implement me
	return nil
}

func (r *BoundEndpointReconciler) createTargetService(ctx context.Context, owner *bindingsv1alpha1.BoundEndpoint, service *v1.Service) error {
	log := ctrl.LoggerFrom(ctx)

	if err := r.Client.Create(ctx, service); err != nil {
		r.Recorder.Event(owner, v1.EventTypeWarning, "Created", "Failed to create Target Service")
		log.Error(err, "Failed to create Target Service")

		setEndpointsStatus(owner, &bindingsv1alpha1.BindingEndpoint{
			Status:       bindingsv1alpha1.StatusError,
			ErrorCode:    NgrokErrorTargetServiceCreateFailed,
			ErrorMessage: fmt.Sprintf("Failed to create Target Service: %s", err),
		})

		return err
	}

	r.Recorder.Event(service, v1.EventTypeNormal, "Created", "Created Target Service")
	r.Recorder.Event(owner, v1.EventTypeNormal, "Created", "Created Target Service")
	log.Info("Created Upstream Service", "service", service.Name)
	return nil
}

func (r *BoundEndpointReconciler) createUpstreamService(ctx context.Context, owner *bindingsv1alpha1.BoundEndpoint, service *v1.Service) error {
	log := ctrl.LoggerFrom(ctx)

	if err := r.Client.Create(ctx, service); err != nil {
		r.Recorder.Event(owner, v1.EventTypeWarning, "Created", "Failed to create Upstream Service")
		log.Error(err, "Failed to create Upstream Service")

		setEndpointsStatus(owner, &bindingsv1alpha1.BindingEndpoint{
			Status:       bindingsv1alpha1.StatusError,
			ErrorCode:    NgrokErrorUpstreamServiceCreateFailed,
			ErrorMessage: fmt.Sprintf("Failed to create Upstream Service: %s", err),
		})

		return err
	}

	r.Recorder.Event(service, v1.EventTypeNormal, "Created", "Created Upstream Service")
	r.Recorder.Event(owner, v1.EventTypeNormal, "Created", "Created Upstream Service")
	log.Info("Created Upstream Service", "service", service.Name)

	return nil
}

func (r *BoundEndpointReconciler) update(ctx context.Context, cr *bindingsv1alpha1.BoundEndpoint) error {
	log := ctrl.LoggerFrom(ctx)

	// binding is not allowed to be created
	if !cr.Spec.Allowed {
		if err := r.deleteBoundEndpointServices(ctx, cr); err != nil {
			return r.controller.ReconcileStatus(ctx, cr, err)
		}

		return r.denyBoundEndpoint(ctx, cr)
	}

	desiredTargetService, desiredUpstreamService := r.convertBoundEndpointToServices(cr)

	var existingTargetService v1.Service
	var existingUpstreamService v1.Service

	// upstream service
	err := r.Get(ctx, client.ObjectKey{Namespace: desiredUpstreamService.Namespace, Name: desiredUpstreamService.Name}, &existingUpstreamService)
	if err != nil {
		if client.IgnoreNotFound(err) == nil {
			// Upstream Service doesn't exist, create it
			log.Info("Unable to find existing Upstream Service, creating...", "name", desiredUpstreamService.Name)
			if err := r.createUpstreamService(ctx, cr, desiredUpstreamService); err != nil {
				return r.controller.ReconcileStatus(ctx, cr, err)
			}
		} else {
			// real error
			log.Error(err, "Failed to find existing Upstream Service", "name", cr.Name, "uri", cr.Spec.EndpointURI)
			return r.controller.ReconcileStatus(ctx, cr, err)
		}
	} else {
		// update upstream service
		existingUpstreamService.Spec = desiredUpstreamService.Spec
		existingUpstreamService.ObjectMeta.Annotations = desiredUpstreamService.ObjectMeta.Annotations
		existingUpstreamService.ObjectMeta.Labels = desiredUpstreamService.ObjectMeta.Labels
		// don't update status

		if err := r.Client.Update(ctx, &existingUpstreamService); err != nil {
			r.Recorder.Event(&existingUpstreamService, v1.EventTypeWarning, "UpdateFailed", "Failed to update Upstream Service")
			r.Recorder.Event(cr, v1.EventTypeWarning, "UpdateFailed", "Failed to update Upstream Service")
			log.Error(err, "Failed to update Upstream Service")
			return r.controller.ReconcileStatus(ctx, cr, err)
		}
		r.Recorder.Event(&existingUpstreamService, v1.EventTypeNormal, "Updated", "Updated Upstream Service")
	}

	// target service
	err = r.Get(ctx, client.ObjectKey{Namespace: desiredTargetService.Namespace, Name: desiredTargetService.Name}, &existingTargetService)
	if err != nil {
		if client.IgnoreNotFound(err) == nil {
			// Target Service doesn't exist, create it
			log.Info("Unable to find existing Target Service, creating...", "name", desiredTargetService.Name)
			if err := r.createTargetService(ctx, cr, desiredTargetService); err != nil {
				return r.controller.ReconcileStatus(ctx, cr, err)
			}
		} else {
			// real error
			log.Error(err, "Failed to find existing Target Service", "name", cr.Name, "uri", cr.Spec.EndpointURI)
			return r.controller.ReconcileStatus(ctx, cr, err)
		}
	} else {
		// update target service
		existingTargetService.Spec = desiredTargetService.Spec
		existingTargetService.ObjectMeta.Annotations = desiredTargetService.ObjectMeta.Annotations
		existingTargetService.ObjectMeta.Labels = desiredTargetService.ObjectMeta.Labels
		// don't update status

		if err := r.Client.Update(ctx, &existingTargetService); err != nil {
			r.Recorder.Event(&existingTargetService, v1.EventTypeWarning, "UpdateFailed", "Failed to update Target Service")
			r.Recorder.Event(cr, v1.EventTypeWarning, "UpdateFailed", "Failed to update Target Service")
			log.Error(err, "Failed to update Target Service")
			return r.controller.ReconcileStatus(ctx, cr, err)
		}
		r.Recorder.Event(&existingTargetService, v1.EventTypeNormal, "Updated", "Updated Target Service")
	}

	if err := r.tryToBindEndpoint(ctx, cr); err != nil {
		return r.controller.ReconcileStatus(ctx, cr, err)
	}

	r.Recorder.Event(cr, v1.EventTypeNormal, "Updated", "Updated Services")
	return r.controller.ReconcileStatus(ctx, cr, nil)
}

func (r *BoundEndpointReconciler) delete(ctx context.Context, cr *bindingsv1alpha1.BoundEndpoint) error {
	return r.deleteBoundEndpointServices(ctx, cr)
}

// deleteBoundEndpointServices deletes the Target and Upstream Services for the BoundEndpoint
func (r *BoundEndpointReconciler) deleteBoundEndpointServices(ctx context.Context, cr *bindingsv1alpha1.BoundEndpoint) error {
	log := ctrl.LoggerFrom(ctx)

	targetService, upstreamService := r.convertBoundEndpointToServices(cr)

	targetNamespace := &v1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: targetService.Namespace}}
	if err := r.Client.Get(ctx, types.NamespacedName{Name: targetNamespace.Name}, targetNamespace); err != nil {
		if client.IgnoreNotFound(err) == nil {
			// fallthrough, no Target Service to delete
		} else {
			log.Error(err, "Failed to get Target Namespace")
			return err
		}
	} else {
		// Target Namespace exists, try to delete the Target Service

		if err := r.Client.Delete(ctx, targetService); err != nil {
			if client.IgnoreNotFound(err) == nil {
				return nil
			} else {
				r.Recorder.Event(cr, v1.EventTypeWarning, "Delete", "Failed to delete Target Service")
				log.Error(err, "Failed to delete Target Service")
				return err
			}
		}
	}

	if err := r.Client.Delete(ctx, upstreamService); err != nil {
		if client.IgnoreNotFound(err) == nil {
			return nil
		} else {
			r.Recorder.Event(cr, v1.EventTypeWarning, "Delete", "Failed to delete Upstream Service")
			log.Error(err, "Failed to delete Upstream Service")
			return err
		}
	}

	return nil
}

func (r *BoundEndpointReconciler) errResult(op controller.BaseControllerOp, cr *bindingsv1alpha1.BoundEndpoint, err error) (ctrl.Result, error) {
	return ctrl.Result{}, err
}

// convertBoundEndpointToServices converts an BoundEndpoint into 2 Services: Target(ExternalName) and Upstream(Pod Forwarders)
func (r *BoundEndpointReconciler) convertBoundEndpointToServices(boundEndpoint *bindingsv1alpha1.BoundEndpoint) (*v1.Service, *v1.Service) {
	// Send traffic to any Node in the cluster
	internalTrafficPolicy := v1.ServiceInternalTrafficPolicyCluster

	endpointURL := fmt.Sprintf("%s.%s.%s", boundEndpoint.Name, boundEndpoint.Namespace, r.ClusterDomain)

	thisBindingLabels := map[string]string{
		LabelBoundEndpointName:      boundEndpoint.Name,
		LabelBoundEndpointNamespace: boundEndpoint.Namespace,
	}

	// Target Labels in order of increasing precedence
	// 1. common labels
	// 2. User's labels
	// 3. Our label selectors (endpoint-binding-name, endpoint-binding-namespace) to mimic OwnerReferences
	targetLabels := util.MergeMaps(
		commonBoundEndpointLabels,
		boundEndpoint.Spec.Target.Metadata.Labels,
		thisBindingLabels,
	)

	targetAnnotations := boundEndpoint.Spec.Target.Metadata.Annotations

	// targetService represents the user's configured endpoint binding as a Service
	// Clients will send requests to this service: <scheme>://<service>.<namespace>:<port>
	targetService := &v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:        boundEndpoint.Spec.Target.Service,
			Namespace:   boundEndpoint.Spec.Target.Namespace,
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
					Name:     boundEndpoint.Spec.Scheme,
					Protocol: v1.Protocol(boundEndpoint.Spec.Target.Protocol),
					// Both Port and TargetPort for the Target Service should match the expected Target.Port on the BoundEndpoint
					Port:       boundEndpoint.Spec.Target.Port,
					TargetPort: intstr.FromInt(int(boundEndpoint.Spec.Target.Port)),
				},
			},
		},
	}

	upstreamLabels := util.MergeMaps(commonBoundEndpointLabels, thisBindingLabels)
	upstreamAnnotations := map[string]string{
		LabelEndpointURL: endpointURL,
	}
	// upstreamService represents the Pod Forwarders as a Service
	// Target Service will point to this Service via an ExternalName
	// This Service will point to the Pod Forwarders' containers on a dedicated allocated port
	upstreamService := &v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:        boundEndpoint.Name,
			Namespace:   boundEndpoint.Namespace,
			Labels:      upstreamLabels,
			Annotations: upstreamAnnotations,
		},
		Spec: v1.ServiceSpec{
			Type:                  v1.ServiceTypeClusterIP,
			InternalTrafficPolicy: &internalTrafficPolicy,
			SessionAffinity:       v1.ServiceAffinityClientIP,
			Selector:              r.UpstreamServiceLabelSelector,
			Ports: []v1.ServicePort{
				{
					Name:     boundEndpoint.Spec.Scheme,
					Protocol: v1.Protocol(boundEndpoint.Spec.Target.Protocol),
					// ExternalName Target Service's port will need to point to the same port on the Upstream Service
					Port: boundEndpoint.Spec.Target.Port,
					// TargetPort is the port within the pod forwarders' containers that is pre-allocated for this BoundEndpoint
					TargetPort: intstr.FromInt(int(boundEndpoint.Spec.Port)),
				},
			},
		},
	}

	return targetService, upstreamService
}

func (r *BoundEndpointReconciler) findBoundEndpointsForNamespace(ctx context.Context, namespace client.Object) []reconcile.Request {
	nsName := namespace.GetName()
	log := ctrl.LoggerFrom(ctx).WithValues("namespace", nsName)

	log.V(3).Info("Finding endpoint bindings for namespace")
	boundEndpoints := &bindingsv1alpha1.BoundEndpointList{}
	listOpts := &client.ListOptions{
		FieldSelector: fields.OneTermEqualSelector(BoundEndpointTargetNamespacePath, nsName),
	}

	err := r.Client.List(ctx, boundEndpoints, listOpts)
	if err != nil {
		log.Error(err, "Failed to list endpoint bindings for namespace")
		return []reconcile.Request{}
	}

	requests := make([]reconcile.Request, len(boundEndpoints.Items))
	for i, binding := range boundEndpoints.Items {
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

func (r *BoundEndpointReconciler) findBoundEndpointsForService(ctx context.Context, svc client.Object) []reconcile.Request {
	log := ctrl.LoggerFrom(ctx).WithValues("service.name", svc.GetName(), "service.namespace", svc.GetNamespace())
	log.V(3).Info("Finding endpoint bindings for service")

	svcLabels := svc.GetLabels()
	if svcLabels == nil {
		log.V(3).Info("Service has no labels")
		return []reconcile.Request{}
	}

	epbName := svcLabels[LabelBoundEndpointName]
	epbNamespace := svcLabels[LabelBoundEndpointNamespace]
	if epbName == "" || epbNamespace == "" {
		log.V(3).Info("Service is not part of an BoundEndpoint")
		return []reconcile.Request{}
	}

	epb := &bindingsv1alpha1.BoundEndpoint{}
	err := r.Client.Get(ctx, types.NamespacedName{Namespace: epbNamespace, Name: epbName}, epb)
	if err != nil {
		if client.IgnoreNotFound(err) == nil {
			log.V(3).Info("BoundEndpoint not found")
			return []reconcile.Request{}
		}

		log.Error(err, "Failed to get BoundEndpoint")
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

// tryToBindEndpoint attempts a TCP connection through the provisioned services for the BoundEndpoint
func (r *BoundEndpointReconciler) tryToBindEndpoint(ctx context.Context, boundEndpoint *bindingsv1alpha1.BoundEndpoint) error {
	log := ctrl.LoggerFrom(ctx).WithValues("uri", boundEndpoint.Spec.EndpointURI)

	retries := 5
	attempt := 0
	waitDuration := 0 * time.Second    // start immediately
	backoffDuration := 3 * time.Second // increasing duration to wait between retries
	dialTimeout := 1 * time.Second     // timeout for dialing the targetService

	// to be filled in
	var bindErr error
	for attempt < retries {
		attempt++

		// wait for attempt to be ready
		time.Sleep(waitDuration)

		// rely on kube-dns to resolve the targetService's ExternalName
		uri, err := url.Parse(boundEndpoint.Spec.EndpointURI)
		if err != nil {
			bindErr = fmt.Errorf("failed to parse BoundEndpoint URI %s: %w", boundEndpoint.Spec.EndpointURI, err)
			continue
		}

		conn, err := net.DialTimeout("tcp", uri.Host, dialTimeout)
		if err != nil {
			log.Error(err, "Failed to bind BoundEndpoint", "attempt", attempt, "retries", retries)
			bindErr = err
		} else {
			// conn exists, close it
			if err := conn.Close(); err != nil {
				log.Error(err, "Failed to close connection", "attempt", attempt, "retries", retries)
				bindErr = err
			} else {
				// success case: we dialed and closed the connection
				bindErr = nil
				break
			}
		}

		// increase backoff duration for next attempt
		waitDuration += backoffDuration
	}

	// update statuses
	var desired *bindingsv1alpha1.BindingEndpoint
	if bindErr != nil {
		// error
		log.Error(bindErr, "Failed to bind BoundEndpoint, moving to error")
		desired = &bindingsv1alpha1.BindingEndpoint{
			Status:       bindingsv1alpha1.StatusError,
			ErrorCode:    NgrokErrorFailedToBind,
			ErrorMessage: fmt.Sprintf("Failed to bind BoundEndpoint: %s", bindErr),
		}
	} else {
		// success
		log.Info("Bound BoundEndpoint successfully, moving to bound")
		desired = &bindingsv1alpha1.BindingEndpoint{
			Status:       bindingsv1alpha1.StatusBound,
			ErrorCode:    "",
			ErrorMessage: "",
		}

	}

	// set status
	setEndpointsStatus(boundEndpoint, desired)
	return bindErr
}

// denyBoundEndpoint sets the status of the BoundEndpoint to denied
func (r *BoundEndpointReconciler) denyBoundEndpoint(ctx context.Context, boundEndpoint *bindingsv1alpha1.BoundEndpoint) error {
	reason := "Endpoint URI is not allowed by KubernetesOperator allowedURLs configuration"

	setEndpointsStatus(boundEndpoint, &bindingsv1alpha1.BindingEndpoint{
		Status:       bindingsv1alpha1.StatusDenied,
		ErrorCode:    NgrokErrorNotAllowed,
		ErrorMessage: reason,
	})

	r.Recorder.Event(boundEndpoint, v1.EventTypeWarning, "Denied", reason)
	return r.controller.ReconcileStatus(ctx, boundEndpoint, nil)
}
