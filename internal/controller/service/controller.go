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
package service

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"reflect"
	"strconv"
	"time"

	"github.com/go-logr/logr"
	"github.com/ngrok/ngrok-api-go/v7"
	common "github.com/ngrok/ngrok-operator/api/common/v1alpha1"
	ingressv1alpha1 "github.com/ngrok/ngrok-operator/api/ingress/v1alpha1"
	ngrokv1alpha1 "github.com/ngrok/ngrok-operator/api/ngrok/v1alpha1"
	"github.com/ngrok/ngrok-operator/internal/annotations"
	"github.com/ngrok/ngrok-operator/internal/annotations/parser"
	"github.com/ngrok/ngrok-operator/internal/controller"
	"github.com/ngrok/ngrok-operator/internal/errors"
	"github.com/ngrok/ngrok-operator/internal/ir"
	"github.com/ngrok/ngrok-operator/internal/ngrokapi"
	"github.com/ngrok/ngrok-operator/internal/resolvers"
	"github.com/ngrok/ngrok-operator/internal/trafficpolicy"
	"github.com/ngrok/ngrok-operator/pkg/managerdriver"
	"golang.org/x/sync/errgroup"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	"k8s.io/utils/ptr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

const (
	OwnerReferencePath     = "metadata.ownerReferences.uid"
	ModuleSetPath          = "metadata.annotations.k8s.ngrok.com/module-set"
	TrafficPolicyPath      = "metadata.annotations.k8s.ngrok.com/traffic-policy"
	NgrokLoadBalancerClass = "ngrok"
)

var (
	coreGVStr = corev1.SchemeGroupVersion.String()
)

type ServiceReconciler struct {
	client.Client
	Log       logr.Logger
	Scheme    *runtime.Scheme
	Recorder  record.EventRecorder
	Namespace string

	ClusterDomain string

	IPPolicyResolver resolvers.IPPolicyResolver
	SecretResolver   resolvers.SecretResolver
	TCPAddresses     ngrokapi.TCPAddressesClient
}

type ShouldHandleServicePredicate = TypedShouldHandleServicePredicate[client.Object]

type TypedShouldHandleServicePredicate[object client.Object] struct {
	predicate.TypedFuncs[object]
}

func (p TypedShouldHandleServicePredicate[object]) Create(e event.CreateEvent) bool {
	svc, ok := e.Object.(*corev1.Service)
	if !ok {
		return false
	}
	return shouldHandleService(svc)
}

func (p TypedShouldHandleServicePredicate[object]) Update(e event.UpdateEvent) bool {
	if e.ObjectOld == nil || e.ObjectNew == nil {
		return false
	}

	oldSvc, ok1 := e.ObjectOld.(*corev1.Service)
	newSvc, ok2 := e.ObjectNew.(*corev1.Service)
	if !ok1 || !ok2 {
		return false
	}
	// We need to reconcile if either the old or new service should be handled
	return shouldHandleService(oldSvc) || shouldHandleService(newSvc)
}

func (r *ServiceReconciler) SetupWithManager(mgr ctrl.Manager) error {
	if r.ClusterDomain == "" {
		r.ClusterDomain = common.DefaultClusterDomain
	}

	if r.IPPolicyResolver == nil {
		r.IPPolicyResolver = resolvers.NewDefaultIPPolicyResolver(mgr.GetClient())
	}

	if r.SecretResolver == nil {
		r.SecretResolver = resolvers.NewDefaultSecretResovler(mgr.GetClient())
	}

	if r.TCPAddresses == nil {
		return errors.New("TCPAddresses client is required")
	}

	owns := []client.Object{
		&ngrokv1alpha1.AgentEndpoint{},
		&ngrokv1alpha1.CloudEndpoint{},
	}

	controller := ctrl.NewControllerManagedBy(mgr).
		For(&corev1.Service{}, builder.WithPredicates(
			predicate.And(
				ShouldHandleServicePredicate{},
				predicate.ResourceVersionChangedPredicate{},
			),
		)).
		// Watch traffic policies for changes
		Watches(
			&ngrokv1alpha1.NgrokTrafficPolicy{},
			handler.EnqueueRequestsFromMapFunc(r.findServicesForTrafficPolicy),
		)

	// Index the subresources by their owner references
	for _, o := range owns {
		controller = controller.Owns(o, builder.WithPredicates(
			predicate.Or(
				predicate.GenerationChangedPredicate{},
				predicate.AnnotationChangedPredicate{},
			),
		))
		err := mgr.GetFieldIndexer().IndexField(context.Background(), o, OwnerReferencePath, func(obj client.Object) []string {
			owner := metav1.GetControllerOf(obj)
			if owner == nil {
				return nil
			}

			if owner.APIVersion != coreGVStr || owner.Kind != "Service" {
				return nil
			}

			return []string{string(owner.UID)}
		})
		if err != nil {
			return err
		}
	}

	// Index the services by the traffic policy they reference
	err := mgr.GetFieldIndexer().IndexField(context.Background(), &corev1.Service{}, TrafficPolicyPath, func(obj client.Object) []string {
		policy, err := annotations.ExtractNgrokTrafficPolicyFromAnnotations(obj)
		if err != nil {
			return nil
		}

		return []string{policy}
	})
	if err != nil {
		return err
	}

	return controller.Complete(r)
}

// +kubebuilder:rbac:groups="",resources=events,verbs=create;patch
// +kubebuilder:rbac:groups="",resources=configmaps,verbs=get;list;watch;create;update;delete
// +kubebuilder:rbac:groups="",resources=secrets,verbs=get;list;watch
// +kubebuilder:rbac:groups="",resources=services,verbs=get;list;watch;update
// +kubebuilder:rbac:groups="",resources=services/status,verbs=get;list;watch;patch;update
// +kubebuilder:rbac:groups=ngrok.k8s.ngrok.com,resources=ngroktrafficpolicies,verbs=get;list;watch

// This reconcile function is called by the controller-runtime manager.
// It is invoked whenever there is an event that occurs for a resource
// being watched (in our case, service objects). If you tail the controller
// logs and delete, update, edit service objects, you see the events come in.
func (r *ServiceReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := ctrl.LoggerFrom(ctx).WithValues("service", req.NamespacedName)
	ctx = ctrl.LoggerInto(ctx, log)

	svc := &corev1.Service{}
	if err := r.Client.Get(ctx, req.NamespacedName, svc); err != nil {
		log.Error(err, "unable to fetch service")
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	subResourceReconcilers := serviceSubresourceReconcilers{
		newServiceCloudEndpointReconciler(),
		newServiceAgentEndpointReconciler(),
	}

	ownedResources, err := subResourceReconcilers.GetOwnedResources(ctx, r.Client, svc)
	if err != nil {
		log.Error(err, "Failed to get owned resources")
		return ctrl.Result{}, err
	}

	// If the service is being deleted, we need to clean up any resources that are owned by it
	if controller.IsDelete(svc) {
		if err := subResourceReconcilers.Reconcile(ctx, r.Client, nil); err != nil {
			log.Error(err, "Failed to cleanup owned resources")
			return ctrl.Result{}, err
		}

		// re-fetch owned resources after cleanup
		ownedResources, err = subResourceReconcilers.GetOwnedResources(ctx, r.Client, svc)
		if err != nil {
			log.Error(err, "Failed to get owned resources")
			return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
		}

		if len(ownedResources) > 0 {
			log.Info("Service still owns ngrok resources, waiting for deletion...")
			return ctrl.Result{RequeueAfter: 5 * time.Second}, nil
		}

		log.Info("Removing and syncing finalizer")
		if err := controller.RemoveAndSyncFinalizer(ctx, r.Client, svc); err != nil {
			log.Error(err, "Failed to remove finalizer")
			return ctrl.Result{}, err
		}
		return ctrl.Result{}, nil
	}

	if !shouldHandleService(svc) {
		if len(ownedResources) > 0 {
			log.Info("Service is not of type LoadBalancer, performing cleanup...")
			// We need to check if the service is being changed from a LoadBalancer to something else.
			// If it is, we need to clean up any resources that are using it.
			err = subResourceReconcilers.Reconcile(ctx, r.Client, nil)
			if err != nil {
				log.Error(err, "Failed to cleanup owned resources")
				return ctrl.Result{}, err
			}
		}

		// Once we clean up the Cloud/Agent Endpoints, we can remove the finalizer if it exists. We don't
		// care about registering a finalizer since we only care about load balancer services
		if err := controller.RemoveAndSyncFinalizer(ctx, r.Client, svc); err != nil {
			log.Error(err, "Failed to remove finalizer")
			return ctrl.Result{}, err
		}
		return ctrl.Result{}, nil
	}

	if len(svc.Spec.Ports) < 1 {
		r.Recorder.Event(svc, corev1.EventTypeWarning, "NoPorts", "Unable to handle service with no ports")
		return ctrl.Result{}, nil
	}

	log.Info("Registering and syncing finalizers")
	if err := controller.RegisterAndSyncFinalizer(ctx, r.Client, svc); err != nil {
		log.Error(err, "Failed to register finalizer")
		return ctrl.Result{}, err
	}

	var desired []client.Object
	mappingStrategy, err := managerdriver.MappingStrategyAnnotationToIR(svc)
	// If the annotation is not valid, we still return a reasonable default mapping strategy. This error
	// is not fatal, so just log it and an event and continue
	if err != nil {
		log.Error(err, "Failed to get mapping strategy annotation")
		r.Recorder.Event(svc, corev1.EventTypeWarning, "FailedToGetMappingStrategy", err.Error())
	}

	desired, err = r.buildEndpoints(ctx, svc, mappingStrategy)
	if err != nil {
		log.Error(err, "Failed to build desired endpoints")
		r.Recorder.Event(svc, corev1.EventTypeWarning, "FailedToBuildEndpoints", err.Error())
		return ctrl.Result{}, err
	}

	if err := subResourceReconcilers.Reconcile(ctx, r.Client, desired); err != nil {
		log.Error(err, "Failed to reconcile owned resources")
		return ctrl.Result{}, err
	}

	// Refetch owned resources after reconciliation and update the service's status
	ownedResources, err = subResourceReconcilers.GetOwnedResources(ctx, r.Client, svc)
	if err != nil {
		log.Error(err, "Failed to get owned resources")
		return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
	}

	// Determine which object to use for updating the service status based on mapping strategy
	statusObject, err := r.getObjectForStatusUpdate(mappingStrategy, ownedResources)
	if err != nil {
		log.Error(err, "Failed to determine object for status update")
		return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
	}

	if err := subResourceReconcilers.UpdateServiceStatus(ctx, r.Client, svc, statusObject); err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to update service status: %w", err)
	}

	r.Recorder.Event(svc, corev1.EventTypeNormal, "Reconciled", "Successfully reconciled service and its ngrok resources")
	return ctrl.Result{}, nil
}

func (r *ServiceReconciler) findServicesForTrafficPolicy(ctx context.Context, policy client.Object) []reconcile.Request {
	log := r.Log

	policyNamespace := policy.GetNamespace()
	policyName := policy.GetName()

	log.V(3).Info("Finding services for traffic policy", "namespace", policyNamespace, "name", policyName)
	services := &corev1.ServiceList{}
	listOpts := &client.ListOptions{
		Namespace:     policyNamespace,
		FieldSelector: fields.OneTermEqualSelector(TrafficPolicyPath, policyName),
	}
	err := r.Client.List(ctx, services, listOpts)
	if err != nil {
		log.Error(err, "Failed to list services for traffic policy")
		return []reconcile.Request{}
	}

	requests := make([]reconcile.Request, len(services.Items))
	for i, svc := range services.Items {
		svcNamespace := svc.GetNamespace()
		svcName := svc.GetName()
		requests[i] = reconcile.Request{
			NamespacedName: types.NamespacedName{
				Namespace: svcNamespace,
				Name:      svcName,
			},
		}
		log.V(3).Info("Triggering reconciliation for service", "namespace", svcNamespace, "name", svcName)
	}
	return requests
}

func (r *ServiceReconciler) clearComputedURLAnnotation(ctx context.Context, svc *corev1.Service) error {
	a := svc.GetAnnotations()
	delete(a, annotations.ComputedURLAnnotation)
	svc.SetAnnotations(a)
	return r.Client.Update(ctx, svc)
}

func (r *ServiceReconciler) tcpAddressIsReserved(ctx context.Context, hostport string) (bool, error) {
	iter := r.TCPAddresses.List(&ngrok.Paging{})
	for iter.Next(ctx) {
		addr := iter.Item()
		if addr.Addr == hostport {
			return true, nil
		}
	}
	return false, iter.Err()
}

// buildEndpoints creates a CloudEndpoint and an AgentEndpoint for the given LoadBalancer service. The CloudEndpoint
// will serve as the public endpoint for the service where we attach the traffic policy if one exists, the AgentEndpoint will
// serve as the internal endpoint.
func (r *ServiceReconciler) buildEndpoints(ctx context.Context, svc *corev1.Service, mappingStrategy ir.IRMappingStrategy) ([]client.Object, error) {
	log := ctrl.LoggerFrom(ctx)

	port := svc.Spec.Ports[0].Port
	objects := make([]client.Object, 0)

	// Get whether endpoint pooling should be enabled/disabled from annotations
	useEndpointPooling, err := annotations.ExtractUseEndpointPooling(svc)
	if err != nil {
		log.Error(err, "failed to check endpoints-enabled annotation for service",
			"service", fmt.Sprintf("%s.%s", svc.Name, svc.Namespace),
		)
		return objects, err
	}

	useBindings, err := annotations.ExtractUseBindings(svc)
	if err != nil {
		log.Error(err, "failed to get bindings annotation for service")
		return objects, err
	}

	// The final traffic policy that will be applied to the listener endpoint
	tp := trafficpolicy.NewTrafficPolicy()

	// If an explicit traffic policy is defined on the service, merge it with the existing traffic policy
	// before adding the forward-internal action.
	// TODO: We still need to handle legacy traffic policy conversion
	policy, err := getNgrokTrafficPolicyForService(ctx, r.Client, svc)
	if err != nil {
		log.Error(err, "Failed to get traffic policy")
		return objects, err
	}
	if policy != nil {
		explicitTP, err := trafficpolicy.NewTrafficPolicyFromJSON(policy.Spec.Policy)
		if err != nil {
			return objects, err
		}

		tp.Merge(explicitTP)
	}

	rawPolicy, err := json.Marshal(tp)
	if err != nil {
		return objects, err
	}

	listenerEndpointURL, err := r.getListenerURL(svc)
	if err != nil {
		r.Recorder.Event(svc, corev1.EventTypeWarning, "FailedToGetListenerURL", err.Error())
		return objects, err
	}

	// Default to the listener endpoint URL, we'll compute this but only for tcp:// endpoints
	// currently.
	computedEndpointURL := listenerEndpointURL

	if listenerEndpointURL == "tcp://" {
		// The user has either not set a 'url' or 'domain' annotation, and desires a TCP endpoint.

		// First, try to get the computed URL annotation. See the comment in the annotations package
		// for more information on why we are temporarily doing this.
		computedEndpointURL, err = annotations.ExtractComputedURL(svc)
		if err != nil {
			if !errors.IsMissingAnnotations(err) {
				return objects, err
			}
			// We need to reserve a TCP address & update the service with the computed URL
			addr, err := r.TCPAddresses.Create(ctx, &ngrok.ReservedAddrCreate{
				Description: fmt.Sprintf("Reserved for %s/%s", svc.Namespace, svc.Name),
				Metadata:    fmt.Sprintf(`{"namespace":"%s","name":"%s"}`, svc.Namespace, svc.Name),
			})
			if err != nil {
				r.Recorder.Event(svc, corev1.EventTypeWarning, "FailedToReserveTCPAddr", err.Error())
				return objects, err
			}

			// Update the service with the computed URL
			computedEndpointURL = fmt.Sprintf("tcp://%s", addr.Addr)
			a := svc.GetAnnotations()
			if a == nil {
				a = make(map[string]string)
			}
			a[annotations.ComputedURLAnnotation] = computedEndpointURL
			svc.SetAnnotations(a)
			if err := r.Client.Update(ctx, svc); err != nil {
				return objects, err
			}
		} else {
			// We have a computed URL, most likely from the url or domain not being set,
			// implying the user wants a TCP address to be reserved. Let's use that, after
			// verifying that it exists.
			parsedURL, parseErr := url.Parse(computedEndpointURL)
			if parseErr != nil {
				r.Recorder.Event(svc, corev1.EventTypeWarning, "FailedToParseComputedURL", parseErr.Error())
				// If we can't parse the URL, we need to clear the computed URL annotation
				if err := r.clearComputedURLAnnotation(ctx, svc); err != nil {
					return objects, err
				}
				return objects, parseErr
			}

			if parsedURL.Scheme == "tcp" {
				// Check that the Address is still reserved
				reserved, err := r.tcpAddressIsReserved(ctx, parsedURL.Host)
				if err != nil {
					return objects, err
				}
				if !reserved {
					r.Recorder.Event(svc, corev1.EventTypeWarning, "TCPAddrNotReserved", "The computed TCP address is not reserved, recomputing")
					if err := r.clearComputedURLAnnotation(ctx, svc); err != nil {
						return objects, err
					}
				}
			} else {
				// The computed URL is not a TCP URL, so we also need to clear it
				if err := r.clearComputedURLAnnotation(ctx, svc); err != nil {
					return objects, err
				}
			}
		}
	} else {
		// We only store computed URLs for TCP endpoints right now, so we should clear it if it exists
		if err := r.clearComputedURLAnnotation(ctx, svc); err != nil {
			return objects, err
		}
	}

	switch mappingStrategy {
	// For the default/collapse strategy, make a single AgentEndpoint
	case ir.IRMappingStrategy_EndpointsCollapsed:
		agentEndpoint := &ngrokv1alpha1.AgentEndpoint{
			ObjectMeta: metav1.ObjectMeta{
				GenerateName: svc.Name + "-",
				Namespace:    svc.Namespace,
				OwnerReferences: []metav1.OwnerReference{
					*metav1.NewControllerRef(svc, corev1.SchemeGroupVersion.WithKind("Service")),
				},
			},
			Spec: ngrokv1alpha1.AgentEndpointSpec{
				URL:      computedEndpointURL,
				Bindings: useBindings,
				Upstream: ngrokv1alpha1.EndpointUpstream{
					URL: fmt.Sprintf("tcp://%s.%s.%s:%d", svc.Name, svc.Namespace, r.ClusterDomain, port),
				},
				TrafficPolicy: &ngrokv1alpha1.TrafficPolicyCfg{
					Inline: rawPolicy,
				},
			},
		}
		objects = append(objects, agentEndpoint)
	// For the verbose strategy, make a CloudEndpoint that routes to an AgentEndpoint
	case ir.IRMappingStrategy_EndpointsVerbose:
		internalURL := fmt.Sprintf("tcp://%s.%s.%s.internal:%d", svc.UID, svc.Name, svc.Namespace, port)
		tp.AddRuleOnTCPConnect(trafficpolicy.Rule{
			Actions: []trafficpolicy.Action{
				trafficpolicy.NewForwardInternalAction(internalURL),
			},
		})

		// We've added a new rule to the traffic policy, so we need to re-marshall it
		rawPolicy, err = json.Marshal(tp)
		if err != nil {
			return objects, err
		}

		cloudEndpoint := &ngrokv1alpha1.CloudEndpoint{
			ObjectMeta: metav1.ObjectMeta{
				GenerateName: svc.Name + "-",
				Namespace:    svc.Namespace,
				OwnerReferences: []metav1.OwnerReference{
					*metav1.NewControllerRef(svc, corev1.SchemeGroupVersion.WithKind("Service")),
				},
			},
			Spec: ngrokv1alpha1.CloudEndpointSpec{
				URL:            computedEndpointURL,
				Bindings:       useBindings,
				PoolingEnabled: useEndpointPooling,
				TrafficPolicy: &ngrokv1alpha1.NgrokTrafficPolicySpec{
					Policy: rawPolicy,
				},
			},
		}
		objects = append(objects, cloudEndpoint)

		agentEndpoint := &ngrokv1alpha1.AgentEndpoint{
			ObjectMeta: metav1.ObjectMeta{
				GenerateName: svc.Name + "-internal-",
				Namespace:    svc.Namespace,
				OwnerReferences: []metav1.OwnerReference{
					*metav1.NewControllerRef(svc, corev1.SchemeGroupVersion.WithKind("Service")),
				},
			},
			Spec: ngrokv1alpha1.AgentEndpointSpec{
				URL: internalURL,
				Upstream: ngrokv1alpha1.EndpointUpstream{
					URL: fmt.Sprintf("tcp://%s.%s.%s:%d", svc.Name, svc.Namespace, r.ClusterDomain, port),
				},
			},
		}
		objects = append(objects, agentEndpoint)
	}

	return objects, nil
}

func (r *ServiceReconciler) getListenerURL(svc *corev1.Service) (string, error) {
	urlAnnotation, err := annotations.ExtractURL(svc)
	if err == nil {
		return urlAnnotation, nil
	}

	if !errors.IsMissingAnnotations(err) {
		return "", err
	}

	// Fallback to the using the deprecated domain annotation if the URL annotation is not present
	domain, err := annotations.ExtractDomain(svc)
	if err == nil {
		msg := fmt.Sprintf(
			"The '%s' annotation is deprecated and will be removed in a future release. Use the '%s' annotation instead",
			annotations.DomainAnnotation,
			annotations.URLAnnotation,
		)
		r.Recorder.Event(
			svc,
			corev1.EventTypeWarning,
			"DeprecatedDomainAnnotation",
			msg,
		)
		return fmt.Sprintf("tls://%s:443", domain), nil
	}

	if !errors.IsMissingAnnotations(err) {
		return "", err
	}

	// No URL or domain annotation, assume TCP as the default
	return "tcp://", nil
}

func (r *ServiceReconciler) getObjectForStatusUpdate(mappingStrategy ir.IRMappingStrategy, ownedResources []client.Object) (client.Object, error) {
	switch mappingStrategy {
	case ir.IRMappingStrategy_EndpointsCollapsed:
		// We should only have 1 owned resource (the AgentEndpoint)
		if len(ownedResources) != 1 {
			return nil, fmt.Errorf("expected 1 owned resource, got %d", len(ownedResources))
		}
		return ownedResources[0], nil
	case ir.IRMappingStrategy_EndpointsVerbose:
		// We should have 2 owned resources (the CloudEndpoint and AgentEndpoint)
		if len(ownedResources) != 2 {
			return nil, fmt.Errorf("expected 2 owned resources, got %d", len(ownedResources))
		}
		for _, owned := range ownedResources {
			if _, ok := owned.(*ngrokv1alpha1.CloudEndpoint); ok {
				return owned, nil
			}
		}
		return nil, errors.New("could not find CloudEndpoint among owned resources")
	default:
		return nil, fmt.Errorf("unknown mapping strategy: %s", mappingStrategy)
	}
}

func shouldHandleService(svc *corev1.Service) bool {
	return svc.Spec.Type == corev1.ServiceTypeLoadBalancer &&
		ptr.Deref(svc.Spec.LoadBalancerClass, "") == NgrokLoadBalancerClass
}

type serviceSubresourceReconciler interface {
	GetOwnedResources(context.Context, client.Client, *corev1.Service) ([]client.Object, error)
	Reconcile(context.Context, client.Client, []client.Object) error
	UpdateServiceStatus(context.Context, client.Client, *corev1.Service, client.Object) error
}

type serviceSubresourceReconcilers []serviceSubresourceReconciler

func (r serviceSubresourceReconcilers) GetOwnedResources(ctx context.Context, c client.Client, svc *corev1.Service) ([]client.Object, error) {
	resources := make([]client.Object, 0)
	for _, srr := range r {
		owned, err := srr.GetOwnedResources(ctx, c, svc)
		if err != nil {
			return resources, err
		}

		resources = append(resources, owned...)
	}
	return resources, nil
}

func (r serviceSubresourceReconcilers) Reconcile(ctx context.Context, c client.Client, objects []client.Object) error {
	g, gctx := errgroup.WithContext(ctx)
	for _, srr := range r {
		srr := srr
		g.Go(func() error {
			return srr.Reconcile(gctx, c, objects)
		})
	}
	return g.Wait()
}

func (r serviceSubresourceReconcilers) UpdateServiceStatus(ctx context.Context, c client.Client, svc *corev1.Service, o client.Object) error {
	g, gctx := errgroup.WithContext(ctx)
	for _, srr := range r {
		srr := srr
		g.Go(func() error {
			return srr.UpdateServiceStatus(gctx, c, svc, o)
		})
	}
	return g.Wait()
}

type baseSubresourceReconciler[T any, PT interface {
	*T
	client.Object
}] struct {
	owned         []PT
	listOwned     func(context.Context, client.Client, ...client.ListOption) ([]T, error)
	matches       func(T, T) bool
	mergeExisting func(T, PT)
	updateStatus  func(context.Context, client.Client, *corev1.Service, PT) error
}

func (r *baseSubresourceReconciler[T, PT]) GetOwnedResources(ctx context.Context, c client.Client, svc *corev1.Service) ([]client.Object, error) {
	opts := []client.ListOption{
		client.InNamespace(svc.Namespace),
		client.MatchingFields{OwnerReferencePath: string(svc.UID)},
	}
	owned, err := r.listOwned(ctx, c, opts...)
	if err != nil {
		return nil, err
	}

	ptrs := make([]PT, len(owned))
	objects := make([]client.Object, len(owned))

	for i, o := range owned {
		var p PT = &o
		ptrs[i] = p
		objects[i] = p
	}
	r.owned = ptrs
	return objects, nil
}

func (r *baseSubresourceReconciler[T, PT]) Reconcile(ctx context.Context, c client.Client, objects []client.Object) error {
	log := ctrl.LoggerFrom(ctx).WithValues("subresource", fmt.Sprintf("%T", *new(T)))

	// Filter out objects that are not of the desired type for this reconciler
	desired := make([]PT, 0)
	for _, o := range objects {
		if v, ok := o.(PT); ok {
			desired = append(desired, v)
		} else {
			log.V(9).Info("skipping object", "name", o.GetName(), "kind", o.GetObjectKind().GroupVersionKind().Kind)
		}
	}
	log.V(9).Info("Filtered objects", "desired", desired, "owned", r.owned)

	// No desired resources, delete all owned resources if any
	if len(desired) == 0 {
		if len(r.owned) > 0 {
			log.V(1).Info("Deleting owned resources")
			for _, e := range r.owned {
				if err := c.Delete(ctx, e); err != nil {
					return err
				}
			}
		}
		return nil
	}

	// We only support one desired resource of a particular type for now
	// If there are cases where we need to create multiple cloud or agent endpoints, we will need to change this handling
	if len(desired) > 1 {
		return errors.New("multiple desired resources not supported")
	}

	d := desired[0]

	// We have a single desired resource and an existing resource, make them match
	if len(r.owned) == 1 {
		var e = r.owned[0]

		log.Info(fmt.Sprintf("Updating %T", e), "desired", d, "existing", e)
		// Fetch the existing resource as it may have been updated
		if err := c.Get(ctx, client.ObjectKeyFromObject(e), e); err != nil {
			return err
		}

		if r.matches(*d, *e) {
			log.V(5).Info(fmt.Sprintf("%T matches desired state, no update needed", e))
			return nil
		}

		r.mergeExisting(*d, e)

		// Update the resource
		if err := c.Update(ctx, e); err != nil {
			log.Error(err, fmt.Sprintf("Failed to update %T", e))
			return err
		}
		return nil
	}

	// If by this point we have more than one owned resource, something is wrong.
	// Delete the owned resources.
	if len(r.owned) > 1 {
		log.Info(fmt.Sprintf("Found multiple %T resources owned by the service, deleting before creating", d))
		for _, e := range r.owned {
			if err := c.Delete(ctx, e); err != nil {
				return err
			}
		}
	}

	log.Info(fmt.Sprintf("Creating %T", d))
	return c.Create(ctx, d)
}

func (r *baseSubresourceReconciler[T, PT]) UpdateServiceStatus(ctx context.Context, c client.Client, svc *corev1.Service, o client.Object) error {
	v, ok := o.(PT)
	if !ok {
		return nil
	}

	return r.updateStatus(ctx, c, svc, v)
}

func newServiceCloudEndpointReconciler() serviceSubresourceReconciler {
	return &baseSubresourceReconciler[ngrokv1alpha1.CloudEndpoint, *ngrokv1alpha1.CloudEndpoint]{
		listOwned: func(ctx context.Context, c client.Client, opts ...client.ListOption) ([]ngrokv1alpha1.CloudEndpoint, error) {
			endpoints := &ngrokv1alpha1.CloudEndpointList{}
			if err := c.List(ctx, endpoints, opts...); err != nil {
				return nil, err
			}
			return endpoints.Items, nil
		},
		matches: func(desired, existing ngrokv1alpha1.CloudEndpoint) bool {
			return reflect.DeepEqual(existing.Spec, desired.Spec)
		},
		mergeExisting: func(desired ngrokv1alpha1.CloudEndpoint, existing *ngrokv1alpha1.CloudEndpoint) {
			existing.Spec = desired.Spec
		},
		updateStatus: func(ctx context.Context, c client.Client, svc *corev1.Service, endpoint *ngrokv1alpha1.CloudEndpoint) error {
			return updateStatus(ctx, c, svc, endpoint)
		},
	}
}

func newServiceAgentEndpointReconciler() serviceSubresourceReconciler {
	return &baseSubresourceReconciler[ngrokv1alpha1.AgentEndpoint, *ngrokv1alpha1.AgentEndpoint]{
		listOwned: func(ctx context.Context, c client.Client, opts ...client.ListOption) ([]ngrokv1alpha1.AgentEndpoint, error) {
			endpoints := &ngrokv1alpha1.AgentEndpointList{}
			if err := c.List(ctx, endpoints, opts...); err != nil {
				return nil, err
			}
			return endpoints.Items, nil
		},
		matches: func(desired, existing ngrokv1alpha1.AgentEndpoint) bool {
			return reflect.DeepEqual(existing.Spec, desired.Spec)
		},
		mergeExisting: func(desired ngrokv1alpha1.AgentEndpoint, existing *ngrokv1alpha1.AgentEndpoint) {
			existing.Spec = desired.Spec
		},
		updateStatus: func(ctx context.Context, c client.Client, svc *corev1.Service, endpoint *ngrokv1alpha1.AgentEndpoint) error {
			return updateStatus(ctx, c, svc, endpoint)
		},
	}
}

func getNgrokTrafficPolicyForService(ctx context.Context, c client.Client, svc *corev1.Service) (*ngrokv1alpha1.NgrokTrafficPolicy, error) {
	policyName, err := annotations.ExtractNgrokTrafficPolicyFromAnnotations(svc)
	if err != nil {
		if errors.IsMissingAnnotations(err) {
			return nil, nil
		}
		return nil, err
	}

	policy := &ngrokv1alpha1.NgrokTrafficPolicy{}
	err = c.Get(ctx, client.ObjectKey{Namespace: svc.Namespace, Name: policyName}, policy)
	return policy, err
}

func updateStatus(ctx context.Context, c client.Client, svc *corev1.Service, endpoint ngrokv1alpha1.EndpointWithDomain) error {
	clearIngressStatus := func(svc *corev1.Service) error {
		svc.Status.LoadBalancer.Ingress = nil
		return c.Status().Update(ctx, svc)
	}

	hostname := ""
	port := int32(443)

	// Check if the computed URL is set, if so, let's parse and use it
	computedURL, err := annotations.ExtractComputedURL(svc)
	switch {
	case err == nil:
		// Let's parse out the host and port
		targetURL, err := url.Parse(computedURL)
		if err != nil {
			return err
		}
		hostname = targetURL.Hostname()
		if p := targetURL.Port(); p != "" {
			x, err := strconv.ParseInt(p, 10, 32)
			if err != nil {
				return err
			}
			port = int32(x)
		}
	case !errors.IsMissingAnnotations(err): // Some other error
		return err
	default: // computedURL not present, fallback to the domain annotation
		domain, err := parser.GetStringAnnotation("domain", svc)
		if err != nil {
			if errors.IsMissingAnnotations(err) {
				return clearIngressStatus(svc)
			}
			return err
		}

		// Use this domain temporarily, but also check if there is a
		// more specific CNAME value on the domain to use
		hostname = domain

		dr := endpoint.GetDomainRef()
		if dr != nil {
			// Lookup the domain
			domain := &ingressv1alpha1.Domain{}
			if err := c.Get(ctx, client.ObjectKey{Namespace: *dr.Namespace, Name: dr.Name}, domain); err != nil {
				return err
			}
			if domain.Status.CNAMETarget != nil {
				hostname = *domain.Status.CNAMETarget
			}
		}
	}

	newIngressStatus := []corev1.LoadBalancerIngress{
		{
			Hostname: hostname,
			Ports: []corev1.PortStatus{
				{
					Port:     port,
					Protocol: corev1.ProtocolTCP,
				},
			},
		},
	}

	// If the status is already set correctly, do nothing
	if reflect.DeepEqual(svc.Status.LoadBalancer.Ingress, newIngressStatus) {
		return nil
	}

	// Update the service status
	svc.Status.LoadBalancer.Ingress = newIngressStatus
	return c.Status().Update(ctx, svc)
}
