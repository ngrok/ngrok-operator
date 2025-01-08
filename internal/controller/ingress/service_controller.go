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
package ingress

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"strconv"
	"time"

	"github.com/go-logr/logr"
	common "github.com/ngrok/ngrok-operator/api/common/v1alpha1"
	ingressv1alpha1 "github.com/ngrok/ngrok-operator/api/ingress/v1alpha1"
	ngrokv1alpha1 "github.com/ngrok/ngrok-operator/api/ngrok/v1alpha1"
	"github.com/ngrok/ngrok-operator/internal/annotations"
	"github.com/ngrok/ngrok-operator/internal/annotations/parser"
	"github.com/ngrok/ngrok-operator/internal/controller"
	"github.com/ngrok/ngrok-operator/internal/errors"
	"github.com/ngrok/ngrok-operator/internal/resolvers"
	"github.com/ngrok/ngrok-operator/internal/trafficpolicy"
	"github.com/ngrok/ngrok-operator/internal/util"
	"golang.org/x/sync/errgroup"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	"k8s.io/utils/ptr"
	ctrl "sigs.k8s.io/controller-runtime"
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

	owns := []client.Object{
		&ingressv1alpha1.Tunnel{},
		&ingressv1alpha1.TCPEdge{},
		&ingressv1alpha1.TLSEdge{},
		&ngrokv1alpha1.AgentEndpoint{},
		&ngrokv1alpha1.CloudEndpoint{},
	}

	controller := ctrl.NewControllerManagedBy(mgr).
		For(&corev1.Service{}).
		WithEventFilter(predicate.Funcs{
			// Only handle services that are of type LoadBalancer and have the correct load balancer class
			CreateFunc: func(e event.CreateEvent) bool {
				svc, ok := e.Object.(*corev1.Service)
				if !ok {
					return false
				}
				return shouldHandleService(svc)
			},
		}).
		// Watch modulesets for changes
		Watches(
			&ingressv1alpha1.NgrokModuleSet{},
			handler.EnqueueRequestsFromMapFunc(r.findServicesForModuleSet),
		).
		// Watch traffic policies for changes
		Watches(
			&ngrokv1alpha1.NgrokTrafficPolicy{},
			handler.EnqueueRequestsFromMapFunc(r.findServicesForTrafficPolicy),
		)

	// Index the subresources by their owner references
	for _, o := range owns {
		controller = controller.Owns(o)
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

	// Index the services by the module set(s) they reference
	err := mgr.GetFieldIndexer().IndexField(context.Background(), &corev1.Service{}, ModuleSetPath, func(obj client.Object) []string {
		moduleSets, err := annotations.ExtractNgrokModuleSetsFromAnnotations(obj)
		if err != nil {
			return nil
		}

		// Note: We are returning a slice of strings here for the field indexer. Checking for equality later, means
		// that only one of the module sets needs to match for the service to be returned.
		return moduleSets
	})
	if err != nil {
		return err
	}

	// Index the services by the traffic policy they reference
	err = mgr.GetFieldIndexer().IndexField(context.Background(), &corev1.Service{}, TrafficPolicyPath, func(obj client.Object) []string {
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
// +kubebuilder:rbac:groups=ingress.k8s.ngrok.com,resources=ngrokmodulesets,verbs=get;list;watch
// +kubebuilder:rbac:groups=ngrok.k8s.ngrok.com,resources=ngroktrafficpolicies,verbs=get;list;watch
// +kubebuilder:rbac:groups=ingress.k8s.ngrok.com,resources=tunnels,verbs=get;list;watch;create;update;delete
// +kubebuilder:rbac:groups=ingress.k8s.ngrok.com,resources=tcpedges,verbs=get;list;watch;create;update;delete
// +kubebuilder:rbac:groups=ingress.k8s.ngrok.com,resources=tlsedges,verbs=get;list;watch;create;update;delete

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
		newServiceTCPEdgeReconciler(),
		newServiceTLSEdgeReconciler(),
		newServiceTunnelReconciler(),
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
		if controller.HasFinalizer(svc) {
			if err := controller.RemoveAndSyncFinalizer(ctx, r.Client, svc); err != nil {
				log.Error(err, "Failed to remove finalizer")
				return ctrl.Result{}, err
			}
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

		// Once we clean up the tunnels and TCP edges, we can remove the finalizer if it exists. We don't
		// care about registering a finalizer since we only care about load balancer services
		if err := controller.RemoveAndSyncFinalizer(ctx, r.Client, svc); err != nil {
			log.Error(err, "Failed to remove finalizer")
			return ctrl.Result{}, err
		}
		return ctrl.Result{}, nil
	}

	if len(svc.Spec.Ports) < 1 {
		log.Info("Service has no ports, skipping")
		return ctrl.Result{}, nil
	}

	log.Info("Registering and syncing finalizers")
	if err := controller.RegisterAndSyncFinalizer(ctx, r.Client, svc); err != nil {
		log.Error(err, "Failed to register finalizer")
		return ctrl.Result{}, err
	}

	var desired []client.Object
	useEndpoints, err := annotations.ExtractUseEndpoints(svc)
	if err != nil {
		if !errors.IsMissingAnnotations(err) {
			log.Error(err, "Failed to get use-endpoints annotation")
			// TODO: Add an event to the service
			return ctrl.Result{}, err
		}
		useEndpoints = false
	}

	// Best effort to try to use endpoints(if configured via annotation and eventually as a global default).
	// If the conversion of modulesets -> trafficpolicy fails or there is some other error such that we can't
	// build the desired endpoints correctly, we will fall back to using tunnels and edges
	// and just bubble up the error as an event on the service
	if useEndpoints {
		desired, err = r.buildEndpoints(ctx, svc)
		if err != nil {
			log.Error(err, "Failed to build desired endpoints")
			r.Recorder.Event(svc, corev1.EventTypeWarning, "FailedToBuildEndpoints", err.Error())
			desired, err = r.buildTunnelAndEdge(ctx, svc)
		}
	} else {
		desired, err = r.buildTunnelAndEdge(ctx, svc)
	}

	if err != nil {
		log.Error(err, "Failed to build desired resources")
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
	for _, o := range ownedResources {
		if err := subResourceReconcilers.UpdateServiceStatus(ctx, r.Client, svc, o); err != nil {
			log.Error(err, "Failed to update service status")
			return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
		}
	}

	return ctrl.Result{}, nil
}

func (r *ServiceReconciler) findServicesForModuleSet(ctx context.Context, moduleSet client.Object) []reconcile.Request {
	log := r.Log

	moduleSetNamespace := moduleSet.GetNamespace()
	moduleSetName := moduleSet.GetName()

	log.V(3).Info("Finding services for module set", "namespace", moduleSetNamespace, "name", moduleSetName)
	services := &corev1.ServiceList{}
	listOpts := &client.ListOptions{
		Namespace:     moduleSetNamespace,
		FieldSelector: fields.OneTermEqualSelector(ModuleSetPath, moduleSetName),
	}
	err := r.Client.List(ctx, services, listOpts)
	if err != nil {
		log.Error(err, "Failed to list services for module set")
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

// buildEndpoints creates a CloudEndpoint and an AgentEndpoint for the given LoadBalancer service. The CloudEndpoint
// will serve as the public endpoint for the service where we attach the traffic policy if one exists, the AgentEndpoint will
// serve as the internal endpoint.
func (r *ServiceReconciler) buildEndpoints(ctx context.Context, svc *corev1.Service) ([]client.Object, error) {
	log := ctrl.LoggerFrom(ctx)

	port := svc.Spec.Ports[0].Port
	objects := make([]client.Object, 0)

	internalURL := fmt.Sprintf("tcp://%s.%s.%s.internal:%d", svc.UID, svc.Name, svc.Namespace, port)

	// The final traffic policy that will be applied to the CloudEndpoint
	tp := trafficpolicy.NewTrafficPolicy()

	// Get the modules from the service annotations
	moduleSet, err := getNgrokModuleSetForService(ctx, r.Client, svc)
	if err != nil {
		log.Error(err, "Failed to get module sets")
		return objects, err
	}

	// If there are modulesets defined on the service, create a traffic policy from them
	// and merge it with the existing traffic policy
	moduleSetsTrafficPolicy, err := util.NewTrafficPolicyFromModuleset(ctx, moduleSet, r.SecretResolver, r.IPPolicyResolver)
	if err != nil {
		log.Error(err, "Failed to create traffic policy from module set", "moduleSet", moduleSet)
		return objects, err
	}
	tp.Merge(moduleSetsTrafficPolicy)

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

	// Finally, add the forward-internal action to the traffic policy
	tp.AddRuleOnTCPConnect(trafficpolicy.Rule{
		Actions: []trafficpolicy.Action{
			trafficpolicy.NewForwardInternalAction(internalURL),
		},
	})

	rawPolicy, err := json.Marshal(tp)
	if err != nil {
		return objects, err
	}

	domain, err := parser.GetStringAnnotation("domain", svc)
	if err != nil {
		if errors.IsMissingAnnotations(err) {
			domain = ""
		} else {
			return objects, err
		}
	}

	// If there is no domain annotation, use a TCP CloudEndpoint. ngrok will randomly assign a TCP address for us.
	// However, if there is a domain annotation, use a TLS CloudEndpoint and specify the domain.
	cloudEndpointURL := "tcp://"
	if domain != "" {
		cloudEndpointURL = fmt.Sprintf("tls://%s:443", domain)
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
			URL: cloudEndpointURL,
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
	return objects, nil
}

func (r *ServiceReconciler) buildTunnelAndEdge(ctx context.Context, svc *corev1.Service) ([]client.Object, error) {
	log := ctrl.LoggerFrom(ctx)

	port := svc.Spec.Ports[0].Port
	objects := make([]client.Object, 0)

	backendLabels := map[string]string{
		"k8s.ngrok.com/namespace":   svc.Namespace,
		"k8s.ngrok.com/service":     svc.Name,
		"k8s.ngrok.com/service-uid": string(svc.UID),
		"k8s.ngrok.com/port":        strconv.Itoa(int(port)),
	}

	tunnel := &ingressv1alpha1.Tunnel{
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: svc.Name + "-",
			Namespace:    svc.Namespace,
			OwnerReferences: []metav1.OwnerReference{
				*metav1.NewControllerRef(svc, corev1.SchemeGroupVersion.WithKind("Service")),
			},
		},
		Spec: ingressv1alpha1.TunnelSpec{
			ForwardsTo: fmt.Sprintf("%s.%s.%s:%d", svc.Name, svc.Namespace, r.ClusterDomain, port),
			Labels:     backendLabels,
		},
	}
	objects = append(objects, tunnel)

	// Get the modules from the service annotations
	moduleSets, err := getNgrokModuleSetForService(ctx, r.Client, svc)
	if err != nil {
		log.Error(err, "Failed to get module sets")
		return objects, err
	}

	policy, err := getNgrokTrafficPolicyForService(ctx, r.Client, svc)
	if err != nil {
		log.Error(err, "Failed to get traffic policy")
		return objects, err
	}

	domain, err := parser.GetStringAnnotation("domain", svc)
	if err != nil {
		if errors.IsMissingAnnotations(err) {
			domain = ""
		} else {
			return objects, err
		}
	}

	if domain == "" { // No domain annotation, use TCP edge
		edge := &ingressv1alpha1.TCPEdge{
			ObjectMeta: metav1.ObjectMeta{
				GenerateName: svc.Name + "-",
				Namespace:    svc.Namespace,
				OwnerReferences: []metav1.OwnerReference{
					*metav1.NewControllerRef(svc, corev1.SchemeGroupVersion.WithKind("Service")),
				},
				Annotations: map[string]string{},
			},
			Spec: ingressv1alpha1.TCPEdgeSpec{
				Backend: ingressv1alpha1.TunnelGroupBackend{
					Labels: backendLabels,
				},
			},
		}
		if moduleSets != nil {
			edge.Spec.IPRestriction = moduleSets.Modules.IPRestriction
		}
		if policy != nil {
			edge.Spec.Policy = policy.Spec.Policy
		}

		objects = append(objects, edge)
	} else {
		edge := &ingressv1alpha1.TLSEdge{
			ObjectMeta: metav1.ObjectMeta{
				GenerateName: svc.Name + "-",
				Namespace:    svc.Namespace,
				OwnerReferences: []metav1.OwnerReference{
					*metav1.NewControllerRef(svc, corev1.SchemeGroupVersion.WithKind("Service")),
				},
				Annotations: map[string]string{},
			},
			Spec: ingressv1alpha1.TLSEdgeSpec{
				Backend: ingressv1alpha1.TunnelGroupBackend{
					Labels: backendLabels,
				},
				Hostports: []string{
					fmt.Sprintf("%s:443", domain),
				},
			},
		}
		if moduleSets != nil {
			edge.Spec.IPRestriction = moduleSets.Modules.IPRestriction
			edge.Spec.MutualTLS = moduleSets.Modules.MutualTLS
			edge.Spec.TLSTermination = moduleSets.Modules.TLSTermination
		}
		if policy != nil {
			edge.Spec.Policy = policy.Spec.Policy
		}
		objects = append(objects, edge)
	}

	return objects, nil
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
	// If there are cases where we need to create multiple edges or tunnels, we will need to change this handling
	if len(desired) > 1 {
		return fmt.Errorf("multiple desired resources not supported")
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
	switch v := o.(type) {
	case PT:
		return r.updateStatus(ctx, c, svc, v)
	}
	return nil
}

func newServiceTCPEdgeReconciler() serviceSubresourceReconciler {
	return &baseSubresourceReconciler[ingressv1alpha1.TCPEdge, *ingressv1alpha1.TCPEdge]{
		listOwned: func(ctx context.Context, c client.Client, opts ...client.ListOption) ([]ingressv1alpha1.TCPEdge, error) {
			edges := &ingressv1alpha1.TCPEdgeList{}
			if err := c.List(ctx, edges, opts...); err != nil {
				return nil, err
			}
			return edges.Items, nil
		},
		matches: func(desired, existing ingressv1alpha1.TCPEdge) bool {
			return reflect.DeepEqual(existing.Spec, desired.Spec)
		},
		mergeExisting: func(desired ingressv1alpha1.TCPEdge, existing *ingressv1alpha1.TCPEdge) {
			existing.Spec = desired.Spec
		},
		updateStatus: func(ctx context.Context, c client.Client, svc *corev1.Service, edge *ingressv1alpha1.TCPEdge) error {
			clearIngressStatus := func(svc *corev1.Service) error {
				svc.Status.LoadBalancer.Ingress = nil
				return c.Status().Update(ctx, svc)
			}

			if len(edge.Status.Hostports) == 0 {
				return clearIngressStatus(svc)
			}
			host, port, err := parseHostAndPort(edge.Status.Hostports[0])
			if err != nil {
				return err
			}

			svc.Status.LoadBalancer.Ingress = []corev1.LoadBalancerIngress{
				{
					Hostname: host,
					Ports: []corev1.PortStatus{
						{
							Port:     port,
							Protocol: corev1.ProtocolTCP,
						},
					},
				},
			}
			return c.Status().Update(ctx, svc)
		},
	}
}

func newServiceTLSEdgeReconciler() serviceSubresourceReconciler {
	return &baseSubresourceReconciler[ingressv1alpha1.TLSEdge, *ingressv1alpha1.TLSEdge]{
		listOwned: func(ctx context.Context, c client.Client, opts ...client.ListOption) ([]ingressv1alpha1.TLSEdge, error) {
			edges := &ingressv1alpha1.TLSEdgeList{}
			if err := c.List(ctx, edges, opts...); err != nil {
				return nil, err
			}
			return edges.Items, nil
		},
		matches: func(desired, existing ingressv1alpha1.TLSEdge) bool {
			return reflect.DeepEqual(existing.Spec, desired.Spec)
		},
		mergeExisting: func(desired ingressv1alpha1.TLSEdge, existing *ingressv1alpha1.TLSEdge) {
			existing.Spec = desired.Spec
		},
		updateStatus: func(ctx context.Context, c client.Client, svc *corev1.Service, edge *ingressv1alpha1.TLSEdge) error {
			clearIngressStatus := func(svc *corev1.Service) error {
				svc.Status.LoadBalancer.Ingress = nil
				return c.Status().Update(ctx, svc)
			}

			domain, err := parser.GetStringAnnotation("domain", svc)
			if err != nil {
				if errors.IsMissingAnnotations(err) {
					return clearIngressStatus(svc)
				}
				return err
			}

			hostname, ok := edge.Status.CNAMETargets[domain]
			if !ok {
				hostname = domain // ngrok managed domain case
			}

			svc.Status.LoadBalancer.Ingress = []corev1.LoadBalancerIngress{
				{
					Hostname: hostname,
					Ports: []corev1.PortStatus{
						{
							Port:     443,
							Protocol: corev1.ProtocolTCP,
						},
					},
				},
			}
			return c.Status().Update(ctx, svc)
		},
	}
}

func newServiceTunnelReconciler() serviceSubresourceReconciler {
	return &baseSubresourceReconciler[ingressv1alpha1.Tunnel, *ingressv1alpha1.Tunnel]{
		listOwned: func(ctx context.Context, c client.Client, opts ...client.ListOption) ([]ingressv1alpha1.Tunnel, error) {
			tunnels := &ingressv1alpha1.TunnelList{}
			if err := c.List(ctx, tunnels, opts...); err != nil {
				return nil, err
			}
			return tunnels.Items, nil
		},
		matches: func(desired, existing ingressv1alpha1.Tunnel) bool {
			return reflect.DeepEqual(existing.Spec, desired.Spec)
		},
		mergeExisting: func(desired ingressv1alpha1.Tunnel, existing *ingressv1alpha1.Tunnel) {
			existing.Spec = desired.Spec
		},
		updateStatus: func(ctx context.Context, c client.Client, svc *corev1.Service, tunnel *ingressv1alpha1.Tunnel) error {
			// Tunnels don't interact with the service status
			return nil
		},
	}
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
			clearIngressStatus := func(svc *corev1.Service) error {
				svc.Status.LoadBalancer.Ingress = nil
				return c.Status().Update(ctx, svc)
			}

			domain, err := parser.GetStringAnnotation("domain", svc)
			if err != nil {
				if errors.IsMissingAnnotations(err) {
					return clearIngressStatus(svc)
				}
				return err
			}

			hostname := domain
			if endpoint.Status.Domain != nil && endpoint.Status.Domain.CNAMETarget != nil {
				hostname = *endpoint.Status.Domain.CNAMETarget
			}

			svc.Status.LoadBalancer.Ingress = []corev1.LoadBalancerIngress{
				{
					Hostname: hostname,
					Ports: []corev1.PortStatus{
						{
							Port:     443,
							Protocol: corev1.ProtocolTCP,
						},
					},
				},
			}
			return c.Status().Update(ctx, svc)
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
			// AgentEndpoints don't interact with the service status
			return nil
		},
	}
}

// Given a service, it will resolve any ngrok modulesets defined on the service to the
// CRDs and then will merge them in to a single moduleset
func getNgrokModuleSetForService(ctx context.Context, c client.Client, svc *corev1.Service) (*ingressv1alpha1.NgrokModuleSet, error) {
	computedModSet := &ingressv1alpha1.NgrokModuleSet{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: svc.Namespace,
		},
	}

	modules, err := annotations.ExtractNgrokModuleSetsFromAnnotations(svc)
	if err != nil {
		if errors.IsMissingAnnotations(err) {
			return computedModSet, nil
		}
		return computedModSet, err
	}

	for _, module := range modules {
		// TODO: watch these and cache them so we don't have to make tons of requests
		resolvedMod := &ingressv1alpha1.NgrokModuleSet{}
		if err := c.Get(ctx, client.ObjectKey{Namespace: svc.Namespace, Name: module}, resolvedMod); err != nil {
			return computedModSet, err
		}
		computedModSet.Merge(resolvedMod)
	}

	return computedModSet, nil
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
