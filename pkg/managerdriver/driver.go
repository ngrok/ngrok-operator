package managerdriver

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"sync"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	netv1 "k8s.io/api/networking/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"
	gatewayv1alpha2 "sigs.k8s.io/gateway-api/apis/v1alpha2"

	common "github.com/ngrok/ngrok-operator/api/common/v1alpha1"
	ingressv1alpha1 "github.com/ngrok/ngrok-operator/api/ingress/v1alpha1"
	ngrokv1alpha1 "github.com/ngrok/ngrok-operator/api/ngrok/v1alpha1"
	gatewayv1beta1 "sigs.k8s.io/gateway-api/apis/v1beta1"

	"github.com/ngrok/ngrok-operator/internal/errors"
	"github.com/ngrok/ngrok-operator/internal/store"
	"github.com/ngrok/ngrok-operator/internal/util"
)

const (
	labelControllerNamespace = "k8s.ngrok.com/controller-namespace"
	labelControllerName      = "k8s.ngrok.com/controller-name"
	labelNamespace           = "k8s.ngrok.com/namespace"
	labelServiceUID          = "k8s.ngrok.com/service-uid"
	labelService             = "k8s.ngrok.com/service"
	labelPort                = "k8s.ngrok.com/port"
)

// Driver maintains the store of information, can derive new information from the store, and can
// synchronize the desired state of the store to the actual state of the cluster.
type Driver struct {
	store store.Storer

	cacheStores          store.CacheStores
	log                  logr.Logger
	scheme               *runtime.Scheme
	ingressNgrokMetadata string
	gatewayNgrokMetadata string
	managerName          types.NamespacedName
	clusterDomain        string

	syncMu              sync.Mutex
	syncRunning         bool
	syncFullCh          chan error
	syncPartialCh       chan error
	syncAllowConcurrent bool

	gatewayEnabled                bool
	disableGatewayReferenceGrants bool
}

type DriverOpt func(*Driver)

func WithGatewayEnabled(enabled bool) DriverOpt {
	return func(d *Driver) {
		d.gatewayEnabled = enabled
	}
}

func WithDisableGatewayReferenceGrants(disable bool) DriverOpt {
	return func(d *Driver) {
		d.disableGatewayReferenceGrants = disable
	}
}

func WithSyncAllowConcurrent(allowed bool) DriverOpt {
	return func(d *Driver) {
		d.syncAllowConcurrent = allowed
	}
}

func WithClusterDomain(domain string) DriverOpt {
	return func(d *Driver) {
		d.clusterDomain = domain
	}
}

// NewDriver creates a new driver with a basic logger and cache store setup
func NewDriver(logger logr.Logger, scheme *runtime.Scheme, controllerName string, managerName types.NamespacedName, opts ...DriverOpt) *Driver {
	cacheStores := store.NewCacheStores(logger)
	s := store.New(cacheStores, controllerName, logger)
	d := &Driver{
		store:          s,
		cacheStores:    cacheStores,
		log:            logger,
		scheme:         scheme,
		managerName:    managerName,
		gatewayEnabled: false,
		clusterDomain:  common.DefaultClusterDomain,
	}

	for _, opt := range opts {
		opt(d)
	}
	return d
}

// WithNgrokMetadata allows you to pass in custom ngrokmetadata to be added to all resources created by the controller
func (d *Driver) WithNgrokMetadata(customNgrokMetadata map[string]string) *Driver {
	ingressNgrokMetadata, err := d.setNgrokMetadataOwner("kubernetes-ingress-controller", customNgrokMetadata)
	if err != nil {
		d.log.Error(err, "error marshalling custom ngrokmetadata", "customNgrokMetadata", d.ingressNgrokMetadata)
		return d
	}
	d.ingressNgrokMetadata = ingressNgrokMetadata

	if d.gatewayEnabled {
		gatewayNgrokMetadata, err := d.setNgrokMetadataOwner("kubernetes-gateway-api", customNgrokMetadata)
		if err != nil {
			d.log.Error(err, "error marshalling custom ngrokmetadata", "customNgrokMetadata", d.gatewayNgrokMetadata)
			return d
		}
		d.gatewayNgrokMetadata = gatewayNgrokMetadata

	}
	return d
}

// Useful for tests
func (d *Driver) GetStore() store.Storer {
	return d.store
}

func (d *Driver) setNgrokMetadataOwner(owner string, customNgrokMetadata map[string]string) (string, error) {
	metaData := make(map[string]string)
	for k, v := range customNgrokMetadata {
		metaData[k] = v
	}
	if _, ok := metaData["owned-by"]; !ok {
		metaData["owned-by"] = owner
	}
	jsonString, err := json.Marshal(metaData)
	if err != nil {
		return "", err
	}

	return string(jsonString), nil
}

func listObjectsForType(ctx context.Context, client client.Reader, v interface{}) ([]client.Object, error) {
	switch v.(type) {

	// ----------------------------------------------------------------------------
	// Kubernetes Core API Support
	// ----------------------------------------------------------------------------
	case *corev1.Service:
		services := &corev1.ServiceList{}
		err := client.List(ctx, services)
		return util.ToClientObjects(services.Items), err
	case *corev1.Secret:
		secrets := &corev1.SecretList{}
		err := client.List(ctx, secrets)
		return util.ToClientObjects(secrets.Items), err
	case *corev1.ConfigMap:
		configmaps := &corev1.ConfigMapList{}
		err := client.List(ctx, configmaps)
		return util.ToClientObjects(configmaps.Items), err
	case *corev1.Namespace:
		namespaces := &corev1.NamespaceList{}
		err := client.List(ctx, namespaces)
		return util.ToClientObjects(namespaces.Items), err
	case *netv1.Ingress:
		ingresses := &netv1.IngressList{}
		err := client.List(ctx, ingresses)
		return util.ToClientObjects(ingresses.Items), err
	case *netv1.IngressClass:
		ingressClasses := &netv1.IngressClassList{}
		err := client.List(ctx, ingressClasses)
		return util.ToClientObjects(ingressClasses.Items), err

	// ----------------------------------------------------------------------------
	// Kubernetes Gateway API Support
	// ----------------------------------------------------------------------------
	case *gatewayv1.GatewayClass:
		gatewayClasses := &gatewayv1.GatewayClassList{}
		err := client.List(ctx, gatewayClasses)
		return util.ToClientObjects(gatewayClasses.Items), err
	case *gatewayv1.Gateway:
		gateways := &gatewayv1.GatewayList{}
		err := client.List(ctx, gateways)
		return util.ToClientObjects(gateways.Items), err
	case *gatewayv1.HTTPRoute:
		httproutes := &gatewayv1.HTTPRouteList{}
		err := client.List(ctx, httproutes)
		return util.ToClientObjects(httproutes.Items), err
	case *gatewayv1alpha2.TCPRoute:
		tcpRoutes := &gatewayv1alpha2.TCPRouteList{}
		err := client.List(ctx, tcpRoutes)
		return util.ToClientObjects(tcpRoutes.Items), err
	case *gatewayv1alpha2.TLSRoute:
		tlsRoutes := &gatewayv1alpha2.TLSRouteList{}
		err := client.List(ctx, tlsRoutes)
		return util.ToClientObjects(tlsRoutes.Items), err
	case *gatewayv1beta1.ReferenceGrant:
		referenceGrants := &gatewayv1beta1.ReferenceGrantList{}
		err := client.List(ctx, referenceGrants)
		return util.ToClientObjects(referenceGrants.Items), err

	// ----------------------------------------------------------------------------
	// Ngrok API Support
	// ----------------------------------------------------------------------------
	case *ingressv1alpha1.Domain:
		domains := &ingressv1alpha1.DomainList{}
		err := client.List(ctx, domains)
		return util.ToClientObjects(domains.Items), err
	case *ingressv1alpha1.HTTPSEdge:
		edges := &ingressv1alpha1.HTTPSEdgeList{}
		err := client.List(ctx, edges)
		return util.ToClientObjects(edges.Items), err
	case *ingressv1alpha1.Tunnel:
		tunnels := &ingressv1alpha1.TunnelList{}
		err := client.List(ctx, tunnels)
		return util.ToClientObjects(tunnels.Items), err
	case *ingressv1alpha1.NgrokModuleSet:
		modules := &ingressv1alpha1.NgrokModuleSetList{}
		err := client.List(ctx, modules)
		return util.ToClientObjects(modules.Items), err
	case *ngrokv1alpha1.NgrokTrafficPolicy:
		policies := &ngrokv1alpha1.NgrokTrafficPolicyList{}
		err := client.List(ctx, policies)
		return util.ToClientObjects(policies.Items), err
	case *ngrokv1alpha1.AgentEndpoint:
		agentEndpoints := &ngrokv1alpha1.AgentEndpointList{}
		err := client.List(ctx, agentEndpoints)
		return util.ToClientObjects(agentEndpoints.Items), err
	case *ngrokv1alpha1.CloudEndpoint:
		cloudEndpoints := &ngrokv1alpha1.CloudEndpointList{}
		err := client.List(ctx, cloudEndpoints)
		return util.ToClientObjects(cloudEndpoints.Items), err
	}
	return nil, fmt.Errorf("unsupported type %T", v)
}

// Seed fetches all the upfront information the driver needs to operate
// It needs to be seeded fully before it can be used to make calculations otherwise
// each calculation will be based on an incomplete state of the world. It currently relies on:
// - Ingresses
// - IngressClasses
// - Gateways
// - HTTPRoutes
// - TCPRoutes
// - TLSRoutes
// - ReferenceGrants
// - Services
// - Secrets
// - Namespaces
// - ConfigMaps
// - Domains
// - Edges
// - Tunnels
// - ModuleSets
// - TrafficPolicies
// - AgentEndpoints
// - CloudEndpoints
// When the sync method becomes a background process, this likely won't be needed anymore
func (d *Driver) Seed(ctx context.Context, c client.Reader) error {
	typesToSeed := []interface{}{
		&netv1.Ingress{},
		&netv1.IngressClass{},
		&corev1.Service{},
		&corev1.Secret{},
		&corev1.Namespace{},
		&corev1.ConfigMap{},
		// CRDs
		&ingressv1alpha1.Domain{},
		&ingressv1alpha1.HTTPSEdge{},
		&ingressv1alpha1.Tunnel{},
		&ingressv1alpha1.NgrokModuleSet{},
		&ngrokv1alpha1.NgrokTrafficPolicy{},
		&ngrokv1alpha1.AgentEndpoint{},
		&ngrokv1alpha1.CloudEndpoint{},
	}

	if d.gatewayEnabled {
		typesToSeed = append(typesToSeed,
			&gatewayv1.Gateway{},
			&gatewayv1.GatewayClass{},
			&gatewayv1.HTTPRoute{},
			&gatewayv1alpha2.TCPRoute{},
			&gatewayv1alpha2.TLSRoute{},
			&gatewayv1beta1.ReferenceGrant{},
		)
	}

	for _, v := range typesToSeed {
		objects, err := listObjectsForType(ctx, c, v)
		if err != nil {
			return err
		}

		for _, obj := range objects {
			if err := d.store.Update(obj); err != nil {
				return err
			}
		}
	}
	return nil
}

func (d *Driver) PrintState(setupLog logr.Logger) {
	ings := d.store.ListNgrokIngressesV1()
	for _, ing := range ings {
		setupLog.Info("found matching ingress", "ingress-name", ing.Name, "ingress-namespace", ing.Namespace)
	}

	// Helpful debug information if someone doesn't have their ingress class set up correctly.
	if len(ings) == 0 {
		ingresses := d.store.ListIngressesV1()
		ngrokIngresses := d.store.ListNgrokIngressesV1()
		ingressClasses := d.store.ListIngressClassesV1()
		ngrokIngressClasses := d.store.ListNgrokIngressClassesV1()
		setupLog.Info("no matching ingresses found",
			"all ingresses", ingresses,
			"all ngrok ingresses", ngrokIngresses,
			"all ingress classes", ingressClasses,
			"all ngrok ingress classes", ngrokIngressClasses,
		)
	}
}

func (d *Driver) UpdateIngress(ingress *netv1.Ingress) (*netv1.Ingress, error) {
	if err := d.store.Update(ingress); err != nil {
		return nil, err
	}
	return d.store.GetNgrokIngressV1(ingress.Name, ingress.Namespace)
}

func (d *Driver) UpdateGateway(gateway *gatewayv1.Gateway) (*gatewayv1.Gateway, error) {
	if err := d.store.Update(gateway); err != nil {
		return nil, err
	}
	return d.store.GetGateway(gateway.Name, gateway.Namespace)
}

func (d *Driver) UpdateHTTPRoute(httproute *gatewayv1.HTTPRoute) (*gatewayv1.HTTPRoute, error) {
	if err := d.store.Update(httproute); err != nil {
		return nil, err
	}
	return d.store.GetHTTPRoute(httproute.Name, httproute.Namespace)
}

func (d *Driver) UpdateTCPRoute(tcpRoute *gatewayv1alpha2.TCPRoute) (*gatewayv1alpha2.TCPRoute, error) {
	if err := d.store.Update(tcpRoute); err != nil {
		return nil, err
	}
	return d.store.GetTCPRoute(tcpRoute.Name, tcpRoute.Namespace)
}

func (d *Driver) UpdateTLSRoute(tlsRoute *gatewayv1alpha2.TLSRoute) (*gatewayv1alpha2.TLSRoute, error) {
	if err := d.store.Update(tlsRoute); err != nil {
		return nil, err
	}
	return d.store.GetTLSRoute(tlsRoute.Name, tlsRoute.Namespace)
}

func (d *Driver) UpdateReferenceGrant(referenceGrant *gatewayv1beta1.ReferenceGrant) (*gatewayv1beta1.ReferenceGrant, error) {
	if err := d.store.Update(referenceGrant); err != nil {
		return nil, err
	}
	return d.store.GetReferenceGrant(referenceGrant.Name, referenceGrant.Namespace)
}

func (d *Driver) UpdateNamespace(namespace *corev1.Namespace) (*corev1.Namespace, error) {
	if err := d.store.Update(namespace); err != nil {
		return nil, err
	}
	return d.store.GetNamespaceV1(namespace.Name)
}

func (d *Driver) DeleteIngress(ingress *netv1.Ingress) error {
	return d.store.Delete(ingress)
}

func (d *Driver) DeleteGateway(gateway *gatewayv1.Gateway) error {
	return d.store.Delete(gateway)
}

func (d *Driver) DeleteHTTPRoute(httproute *gatewayv1.HTTPRoute) error {
	return d.store.Delete(httproute)
}

func (d *Driver) DeleteTCPRoute(tcpRoute *gatewayv1alpha2.TCPRoute) error {
	return d.store.Delete(tcpRoute)
}

func (d *Driver) DeleteTLSRoute(tlsRoute *gatewayv1alpha2.TLSRoute) error {
	return d.store.Delete(tlsRoute)
}

// Delete an ingress object given the NamespacedName
// Takes a namespacedName string as a parameter and
// deletes the ingress object from the cacheStores map
func (d *Driver) DeleteNamedIngress(n types.NamespacedName) error {
	ingress := &netv1.Ingress{}
	// set NamespacedName on the ingress object
	ingress.SetNamespace(n.Namespace)
	ingress.SetName(n.Name)
	return d.cacheStores.Delete(ingress)
}

func (d *Driver) DeleteNamedGateway(n types.NamespacedName) error {
	gtw := &gatewayv1.Gateway{}
	// set NamespacedName on the gateway object
	gtw.SetNamespace(n.Namespace)
	gtw.SetName(n.Name)
	return d.cacheStores.Delete(gtw)
}

func (d *Driver) DeleteNamedHTTPRoute(n types.NamespacedName) error {
	httproute := &gatewayv1.HTTPRoute{}
	// set NamespacedName on the httproute object
	httproute.SetNamespace(n.Namespace)
	httproute.SetName(n.Name)
	return d.cacheStores.Delete(httproute)
}

func (d *Driver) DeleteNamedTCPRoute(n types.NamespacedName) error {
	tcpRoute := &gatewayv1alpha2.TCPRoute{}
	// set NamespacedName on the tcproute object
	tcpRoute.SetNamespace(n.Namespace)
	tcpRoute.SetName(n.Name)
	return d.cacheStores.Delete(tcpRoute)
}

func (d *Driver) DeleteNamedTLSRoute(n types.NamespacedName) error {
	tlsRoute := &gatewayv1alpha2.TLSRoute{}
	// set NamespacedName on the tlsroute object
	tlsRoute.SetNamespace(n.Namespace)
	tlsRoute.SetName(n.Name)
	return d.cacheStores.Delete(tlsRoute)
}

func (d *Driver) DeleteReferenceGrant(n types.NamespacedName) error {
	referenceGrant := &gatewayv1beta1.ReferenceGrant{}
	referenceGrant.SetNamespace(n.Namespace)
	referenceGrant.SetName(n.Name)
	return d.cacheStores.Delete(referenceGrant)
}

func (d *Driver) DeleteNamespace(name string) error {
	namespace := &corev1.Namespace{}
	namespace.SetName(name)
	return d.cacheStores.Delete(namespace)
}

// syncStart will:
//   - let the first caller proceed, indicated by returning true
//   - while the first one is running any subsequent calls will be batched to the last call
//   - the callers between first and last will be assumed "success" and wait will return nil
//   - the last one will return an error, which will retrigger reconciliation
func (d *Driver) syncStart(partial bool) (bool, func(ctx context.Context) error) {
	d.log.Info("sync start")
	d.syncMu.Lock()
	defer d.syncMu.Unlock()

	if !d.syncRunning {
		// not running, we can take action
		d.syncRunning = true
		return true, nil
	}

	// already running, overtake any other waiters
	if d.syncFullCh != nil {
		if partial {
			// a full sync is already waiting, ignore non-full ones
			return false, func(_ context.Context) error {
				return nil
			}
		}
		close(d.syncFullCh)
		d.syncFullCh = nil
	}
	if d.syncPartialCh != nil {
		close(d.syncPartialCh)
		d.syncPartialCh = nil
	}

	// put yourself in waiting position
	ch := make(chan error, 1)
	if partial {
		d.syncPartialCh = ch
	} else {
		d.syncFullCh = ch
	}

	return false, func(ctx context.Context) error {
		select {
		case err := <-ch:
			d.log.Error(err, "sync finished with error")
			return err
		case <-ctx.Done():
			return ctx.Err()
		}
	}
}

var errSyncDone = errors.New("sync done")

func (d *Driver) syncDone() {
	d.log.Info("sync done")
	d.syncMu.Lock()
	defer d.syncMu.Unlock()

	if d.syncFullCh != nil {
		d.syncFullCh <- errSyncDone
		close(d.syncFullCh)
		d.syncFullCh = nil
	}
	if d.syncPartialCh != nil {
		d.syncPartialCh <- errSyncDone
		close(d.syncPartialCh)
		d.syncPartialCh = nil
	}
	d.syncRunning = false
}

// Sync calculates what the desired state for each of our CRDs should be based on the ingresses and other
// objects in the store. It then compares that to the actual state of the cluster and updates the cluster
func (d *Driver) Sync(ctx context.Context, c client.Client) error {
	// This function gets called a lot in the current architecture. At the end it also syncs
	// resources which in turn triggers more reconcile events. Its all eventually consistent, but
	// its noisy and can make us hit ngrok api limits. We should probably just change this to be
	// a periodic sync instead of a sync on every reconcile event, but for now this debouncer
	// keeps it in check and syncs in batches
	if !d.syncAllowConcurrent {
		proceed, wait := d.syncStart(false)
		if !proceed {
			return wait(ctx)
		}
		defer d.syncDone()
	}

	d.log.Info("syncing driver state!!")

	// TODO (Alice): move domains, edges, tunnels to translator
	domains := d.calculateDomainSet()
	desiredEdges := d.calculateHTTPSEdges(domains)
	desiredTunnels := d.calculateTunnels()

	translator := NewTranslator(
		d.log,
		d.store,
		d.defaultManagedResourceLabels(),
		d.ingressNgrokMetadata,
		d.gatewayNgrokMetadata,
		d.clusterDomain,
		d.disableGatewayReferenceGrants,
	)
	translationResult := translator.Translate()

	currDomains := &ingressv1alpha1.DomainList{}
	currEdges := &ingressv1alpha1.HTTPSEdgeList{}
	currTunnels := &ingressv1alpha1.TunnelList{}
	currAgentEndpoints := &ngrokv1alpha1.AgentEndpointList{}
	currCloudEndpoints := &ngrokv1alpha1.CloudEndpointList{}

	if err := c.List(ctx, currDomains); err != nil {
		d.log.Error(err, "error listing domains")
		return err
	}
	if err := c.List(ctx, currEdges, client.MatchingLabels{
		labelControllerNamespace: d.managerName.Namespace,
		labelControllerName:      d.managerName.Name,
	}); err != nil {
		d.log.Error(err, "error listing edges")
		return err
	}
	if err := c.List(ctx, currTunnels, client.MatchingLabels{
		labelControllerNamespace: d.managerName.Namespace,
		labelControllerName:      d.managerName.Name,
	}); err != nil {
		d.log.Error(err, "error listing tunnels")
		return err
	}
	if err := c.List(ctx, currAgentEndpoints, client.MatchingLabels{
		labelControllerNamespace: d.managerName.Namespace,
		labelControllerName:      d.managerName.Name,
	}); err != nil {
		d.log.Error(err, "error listing agent endpoints")
		return err
	}
	if err := c.List(ctx, currCloudEndpoints, client.MatchingLabels{
		labelControllerNamespace: d.managerName.Namespace,
		labelControllerName:      d.managerName.Name,
	}); err != nil {
		d.log.Error(err, "error listing cloud endpoints")
		return err
	}

	if err := d.applyDomains(ctx, c, domains.totalDomains, currDomains.Items); err != nil {
		return err
	}

	if err := d.applyHTTPSEdges(ctx, c, desiredEdges, currEdges.Items); err != nil {
		return err
	}

	if err := d.applyAgentEndpoints(ctx, c, translationResult.AgentEndpoints, currAgentEndpoints.Items); err != nil {
		d.log.Error(err, "applying agent endpoints")
		return err
	}

	if err := d.applyCloudEndpoints(ctx, c, translationResult.CloudEndpoints, currCloudEndpoints.Items); err != nil {
		d.log.Error(err, "applying cloud endpoints")
		return err
	}

	if err := d.applyTunnels(ctx, c, desiredTunnels, currTunnels.Items); err != nil {
		return err
	}

	if err := d.updateIngressStatuses(ctx, c); err != nil {
		return err
	}

	// UpdateGatewayStatuses
	// if err := d.updateGatewayStatuses(ctx, c); err != nil {
	// 	return err
	// }

	// UpdateHTTPRouteStatuses
	// if err := d.updateHTTPRouteStatuses(ctx, c); err != nil {
	// 	return err
	// }

	return nil
}

func (d *Driver) updateIngressStatuses(ctx context.Context, c client.Client) error {
	ingresses := d.store.ListNgrokIngressesV1()
	for _, ingress := range ingresses {
		newLBIPStatus := calculateIngressLoadBalancerIPStatus(d.log, ingress, c)
		if !reflect.DeepEqual(ingress.Status.LoadBalancer.Ingress, newLBIPStatus) {
			ingress.Status.LoadBalancer.Ingress = newLBIPStatus
			if err := c.Status().Update(ctx, ingress); err != nil {
				d.log.Error(err, "error updating ingress status", "ingress", ingress)
				return err
			}
		}
	}
	return nil
}

// getTrafficPolicyJSON retrieves the traffic policy for an ingress and falls back to the modSet policy if it doesn't exist.
func (d *Driver) getTrafficPolicyJSON(ingress *netv1.Ingress, modSet *ingressv1alpha1.NgrokModuleSet) (json.RawMessage, error) {
	var err error
	var policyJSON json.RawMessage

	trafficPolicy, err := getNgrokTrafficPolicyForIngress(ingress, d.store)

	if err != nil {
		d.log.Error(err, "error getting ngrok traffic policy for ingress", "ingress", ingress)
		return nil, err
	}

	if modSet.IsEmpty() {
		if trafficPolicy != nil {
			return trafficPolicy.Spec.Policy, nil
		}
		return policyJSON, nil
	}

	if modSet.Modules.Policy != nil && trafficPolicy != nil {
		return nil, fmt.Errorf("cannot have both a traffic policy and a moduleset policy on ingress: %s", ingress.Name)
	}

	if policyJSON, err = json.Marshal(modSet.Modules.Policy); err != nil {
		d.log.Error(err, "cannot convert module-set policy json", "ingress", ingress, "Policy", modSet.Modules.Policy)
		return nil, err
	}

	return policyJSON, nil
}

func (d *Driver) createEndpointPolicyForGateway(rule *gatewayv1.HTTPRouteRule, namespace string) (json.RawMessage, error) {
	pathPrefixMatches := []string{}

	// NOTE: matches are only defined on requests, and filters are only triggered by matches,
	// but some filters define transformations on responses, so we need to define matches on both
	// Policy.Inbound and Policy.Outbound when possible to work with ngrok's system
	for _, match := range rule.Matches {
		if match.Path != nil {
			if match.Path.Type != nil {
				switch *match.Path.Type {
				case gatewayv1.PathMatchExact:
				case gatewayv1.PathMatchPathPrefix:
					if match.Path.Value != nil {
						pathPrefixMatches = append(pathPrefixMatches, *match.Path.Value)
					}
				case gatewayv1.PathMatchRegularExpression:
					return nil, errors.NewErrorNotFound(fmt.Sprintf("unsupported match type PathMatchType %v found", *match.Path.Type))
				default:
					return nil, errors.NewErrorNotFound(fmt.Sprintf("Unknown match type PathMatchType %v found", *match.Path.Type))
				}
			}
		}

		if match.Method != nil {
			d.log.Error(fmt.Errorf("unsupported match type"), "Unsupported match type", "HTTPMethod", *match.Method)
		}

		if len(match.Headers) > 0 {
			d.log.Error(fmt.Errorf("unsupported match type"), "Unsupported match type", "HTTPHeaderMatch", match.Headers)
		}

		if len(match.QueryParams) > 0 {
			d.log.Error(fmt.Errorf("unsupported match type"), "Unsupported match type", "HTTPQueryParamMatch", match.QueryParams)
		}
	}

	fullTrafficPolicy := util.NewTrafficPolicy()

	// "hard-coded" phases. Since Filters are translated to rules in particular phases, the operator has to be aware of these.
	// There isn't really a way around this.
	onHttpRequestActions := util.Actions{}
	onHttpResponseActions := util.Actions{}

	flushCount := 0

	flushActionsToRules := func() error {
		if len(onHttpRequestActions.EndpointActions) == 0 && len(onHttpResponseActions.EndpointActions) == 0 {
			return nil
		}
		// there are actions to flush
		flushCount++
		if len(onHttpRequestActions.EndpointActions) > 0 {
			// flush actions to a rule
			rule := util.EndpointRule{
				Actions: onHttpRequestActions.EndpointActions,
				Name:    fmt.Sprint("Inbound HTTPRouteRule ", flushCount),
			}
			if err := fullTrafficPolicy.MergeEndpointRule(rule, util.PhaseOnHttpRequest); err != nil {
				return err
			}

			// clear
			onHttpRequestActions = util.Actions{}
		}
		if len(onHttpResponseActions.EndpointActions) > 0 {
			// flush actions to a rule
			rule := util.EndpointRule{
				Actions: onHttpResponseActions.EndpointActions,
				Name:    fmt.Sprint("Outbound HTTPRouteRule ", flushCount),
			}
			if err := fullTrafficPolicy.MergeEndpointRule(rule, util.PhaseOnHttpResponse); err != nil {
				return err
			}

			// clear
			onHttpResponseActions = util.Actions{}
		}

		return nil
	}

	responseHeaders := make(map[string]string)
	for _, filter := range rule.Filters {
		switch filter.Type {
		case gatewayv1.HTTPRouteFilterRequestRedirect:
			// NOTE: request redirect is a special case, and is subject to change
			err := d.handleRequestRedirectFilter(filter.RequestRedirect, pathPrefixMatches, &onHttpRequestActions, responseHeaders)
			if err != nil {
				return nil, err
			}
		case gatewayv1.HTTPRouteFilterRequestHeaderModifier:
			err := d.handleHTTPHeaderFilter(filter.RequestHeaderModifier, &onHttpRequestActions, nil)
			if err != nil {
				return nil, err
			}
		case gatewayv1.HTTPRouteFilterResponseHeaderModifier:
			err := d.handleHTTPHeaderFilter(filter.ResponseHeaderModifier, &onHttpResponseActions, responseHeaders)
			if err != nil {
				return nil, err
			}
		case gatewayv1.HTTPRouteFilterURLRewrite:
			err := d.handleURLRewriteFilter(filter.URLRewrite, pathPrefixMatches, &onHttpRequestActions)
			if err != nil {
				return nil, err
			}
		case gatewayv1.HTTPRouteFilterRequestMirror:
			return nil, errors.NewErrorNotFound(fmt.Sprintf("Unsupported filter HTTPRouteFilterType %v found", filter.Type))
		case gatewayv1.HTTPRouteFilterExtensionRef:
			// if there are current actions outstanding, make a rule to hold them before we start a new rule for this PolicyCRD
			if err := flushActionsToRules(); err != nil {
				return nil, err
			}

			// a PolicyCRD can have expressions, so send in rule pointers so expressions can be on those rules
			err := d.handleExtensionRef(filter.ExtensionRef, namespace, fullTrafficPolicy)
			if err != nil {
				return nil, err
			}
		default:
			return nil, errors.NewErrorNotFound(fmt.Sprintf("Unknown filter HTTPRouteFilterType %v found", filter.Type))
		}
	}

	// flush any leftover actions to rules
	if err := flushActionsToRules(); err != nil {
		return nil, err
	}

	policy, err := fullTrafficPolicy.ToCRDJson()
	if err != nil {
		return nil, err
	}

	return policy, nil
}

type RemoveHeadersConfig struct {
	Headers []string `json:"headers"`
}

type AddHeadersConfig struct {
	Headers map[string]string `json:"headers"`
}

func (d *Driver) handleExtensionRef(extensionRef *gatewayv1.LocalObjectReference, namespace string, trafficPolicy util.TrafficPolicy) error {
	switch extensionRef.Kind {
	case "NgrokTrafficPolicy":
		// look up Policy CRD
		policy, err := d.store.GetNgrokTrafficPolicyV1(string(extensionRef.Name), namespace)
		if err != nil {
			return err
		}

		jsonMessage := policy.Spec.Policy
		if jsonMessage == nil {
			return errors.NewErrorNotFound(fmt.Sprintf("PolicyCRD %v found with no policy", extensionRef.Name))
		}

		// transform into structured format
		extensionRefTrafficPolicy, err := extractPolicy(jsonMessage)
		if err != nil {
			return err
		}

		trafficPolicy.Merge(extensionRefTrafficPolicy)
	default:
		return errors.NewErrorNotFound(fmt.Sprintf("Unknown ExtensionRef Kind %v found, Name: %v", extensionRef.Kind, extensionRef.Name))
	}
	return nil
}

func (d *Driver) handleHTTPHeaderFilter(filter *gatewayv1.HTTPHeaderFilter, actions *util.Actions, requestRedirectHeaders map[string]string) error {
	if filter == nil {
		return nil
	}

	if err := d.handleHTTPHeaderFilterRemove(filter.Remove, actions); err != nil {
		return err
	}

	if err := d.handleHTTPHeaderFilterAdd(filter.Add, actions, requestRedirectHeaders); err != nil {
		return err
	}

	return d.handleHTTPHeaderFilterSet(filter, actions, requestRedirectHeaders)
}

func (d *Driver) handleHTTPHeaderFilterRemove(headersToRemove []string, actions *util.Actions) error {
	if len(headersToRemove) == 0 {
		return nil
	}

	removeHeaders, err := json.Marshal(RemoveHeadersConfig{Headers: headersToRemove})
	if err != nil {
		d.log.Error(err, "cannot convert headers to json", "headers", headersToRemove)
		return err
	}

	action := util.EndpointAction{
		Type:   "remove-headers",
		Config: removeHeaders,
	}

	rawAction, err := json.Marshal(&action)
	if err != nil {
		return err
	}

	actions.EndpointActions = append(actions.EndpointActions, rawAction)

	return nil
}

func (d *Driver) handleHTTPHeaderFilterAdd(headersToAdd []gatewayv1.HTTPHeader, actions *util.Actions, requestRedirectHeaders map[string]string) error {
	if len(headersToAdd) == 0 {
		return nil
	}

	config := AddHeadersConfig{Headers: make(map[string]string)}
	for _, header := range headersToAdd {
		config.Headers[string(header.Name)] = header.Value
	}

	if requestRedirectHeaders != nil {
		for k, v := range config.Headers {
			requestRedirectHeaders[k] = v
		}
	}

	addHeaders, err := json.Marshal(config)
	if err != nil {
		d.log.Error(err, "cannot convert headers to json", "headers", headersToAdd)
		return err
	}

	action := util.EndpointAction{
		Type:   "add-headers",
		Config: addHeaders,
	}

	rawAction, err := json.Marshal(&action)
	if err != nil {
		return nil
	}

	actions.EndpointActions = append(actions.EndpointActions, rawAction)

	return nil
}

func (d *Driver) handleHTTPHeaderFilterSet(filter *gatewayv1.HTTPHeaderFilter, actions *util.Actions, requestRedirectHeaders map[string]string) error {
	if filter == nil {
		return nil
	}
	removeHeaders := []string{}
	for _, header := range filter.Set {
		removeHeaders = append(removeHeaders, string(header.Name))
	}

	if err := d.handleHTTPHeaderFilterRemove(removeHeaders, actions); err != nil {
		return err
	}

	return d.handleHTTPHeaderFilterAdd(filter.Set, actions, requestRedirectHeaders)
}

type URLRedirectConfig struct {
	To         *string `json:"to"`
	From       *string `json:"from"`
	StatusCode *int    `json:"status_code"`
	// convert to response headers
	Headers map[string]string `json:"headers"`
}

func (d *Driver) createUrlRedirectConfig(from string, to string, requestHeaders map[string]string, statusCode *int, actions *util.Actions) error {
	urlRedirectAction := URLRedirectConfig{
		To:         &to,
		From:       &from,
		StatusCode: statusCode,
		Headers:    requestHeaders,
	}
	config, err := json.Marshal(urlRedirectAction)

	if err != nil {
		d.log.Error(err, "cannot convert request redirect filter to json", "HTTPRequestRedirectFilter", urlRedirectAction)
		return err
	}

	action := util.EndpointAction{
		Type:   "redirect",
		Config: config,
	}

	rawAction, err := json.Marshal(&action)
	if err != nil {
		return err
	}

	actions.EndpointActions = append(actions.EndpointActions, rawAction)

	return nil
}

type URLRewriteConfig struct {
	To   *string `json:"to"`
	From *string `json:"from"`
}

func (d *Driver) createURLRewriteConfig(from string, to string, actions *util.Actions) error {
	urlRewriteAction := URLRewriteConfig{
		To:   &to,
		From: &from,
	}
	config, err := json.Marshal(urlRewriteAction)

	if err != nil {
		d.log.Error(err, "cannot convert request rewrite filter to json", "HTTPRequestRewriteFilter", urlRewriteAction)
		return err
	}

	action := util.EndpointAction{
		Type:   "url-rewrite",
		Config: config,
	}

	rawAction, err := json.Marshal(&action)
	if err != nil {
		return err
	}

	actions.EndpointActions = append(actions.EndpointActions, rawAction)

	return nil
}

func (d *Driver) handleURLRewriteFilter(filter *gatewayv1.HTTPURLRewriteFilter, pathPrefixMatches []string, actions *util.Actions) error {
	var err error
	if filter == nil {
		return nil
	}

	if filter.Hostname != nil {
		hostname := string(*filter.Hostname)
		err = d.handleHTTPHeaderFilterAdd([]gatewayv1.HTTPHeader{{Name: "Host", Value: hostname}}, actions, nil)
	}

	if err != nil {
		return err
	}

	if filter.Path == nil {
		return nil
	}

	switch filter.Path.Type {
	case "ReplacePrefixMatch":
		for _, pathPrefix := range pathPrefixMatches {
			from := fmt.Sprintf("^https?://[^/:]+(:[0-9]*)?(%s)([^\\?]*)(\\?.*)?$", pathPrefix)
			to := fmt.Sprintf("$scheme://$authority%s$3$is_args$args", *filter.Path.ReplacePrefixMatch)
			err := d.createURLRewriteConfig(from, to, actions)
			if err != nil {
				return err
			}
		}
	case "ReplaceFullPath":
		from := ".*" // "^https?://[^/]+(:[0-9]*)?(/[^\\?]*)?(\\?.*)?$"
		to := fmt.Sprintf("$scheme://$authority%s$is_args$args", *filter.Path.ReplaceFullPath)
		err := d.createURLRewriteConfig(from, to, actions)
		if err != nil {
			return err
		}
	default:
		d.log.Error(fmt.Errorf("unsupported path modifier type"), "unsupported path modifier type", "HTTPPathModifier", filter.Path.Type)
		return nil
	}
	return nil
}

func (d *Driver) handleRequestRedirectFilter(filter *gatewayv1.HTTPRequestRedirectFilter, pathPrefixMatches []string, actions *util.Actions, requestHeaders map[string]string) error {
	if filter == nil {
		return nil
	}

	scheme := "$scheme"
	if filter.Scheme != nil {
		scheme = *filter.Scheme
	}
	hostname := "$host"
	if filter.Hostname != nil {
		hostname = string(*filter.Hostname)
	}
	port := "$1" // (:[0-9]*)?
	if filter.Port != nil {
		port = string(*filter.Port)
	}

	if filter.Path == nil {
		from := ".*" // "^https?://[^/]+(:[0-9]*)?(/[^\\?]*)?(\\?.*)?$"
		to := fmt.Sprintf("%s://%s%s$uri", scheme, hostname, port)
		err := d.createUrlRedirectConfig(from, to, requestHeaders, filter.StatusCode, actions)
		if err != nil {
			return err
		}
		return nil
	}

	switch filter.Path.Type {
	case "ReplacePrefixMatch":
		for _, pathPrefix := range pathPrefixMatches {
			from := fmt.Sprintf("^https?://[^/:]+(:[0-9]*)?(%s)([^\\?]*)(\\?.*)?$", pathPrefix)
			to := fmt.Sprintf("%s://%s%s%s$3$is_args$args", scheme, hostname, port, *filter.Path.ReplacePrefixMatch)
			err := d.createUrlRedirectConfig(from, to, requestHeaders, filter.StatusCode, actions)
			if err != nil {
				return err
			}
		}
	case "ReplaceFullPath":
		from := ".*" // "^https?://[^/]+(:[0-9]*)?(/[^\\?]*)?(\\?.*)?$"
		to := fmt.Sprintf("%s://%s%s%s$is_args$args", scheme, hostname, port, *filter.Path.ReplaceFullPath)
		err := d.createUrlRedirectConfig(from, to, requestHeaders, filter.StatusCode, actions)
		if err != nil {
			return err
		}
	default:
		d.log.Error(fmt.Errorf("unsupported path modifier type"), "unsupported path modifier type", "HTTPPathModifier", filter.Path.Type)
		return nil
	}
	return nil
}

func (d *Driver) findBackendRefServicePort(backendRef gatewayv1.BackendRef, namespace string) (*corev1.Service, *corev1.ServicePort, error) {
	service, err := d.store.GetServiceV1(string(backendRef.Name), namespace)
	if err != nil {
		return nil, nil, err
	}
	servicePort, err := findBackendRefServicesPort(d.log, service, &backendRef)
	if err != nil {
		return nil, nil, err
	}

	return service, servicePort, nil
}

func (d *Driver) findBackendServicePort(backendSvc netv1.IngressServiceBackend, namespace string) (*corev1.Service, *corev1.ServicePort, error) {
	service, err := d.store.GetServiceV1(backendSvc.Name, namespace)
	if err != nil {
		return nil, nil, err
	}

	servicePort, err := findServicesPort(d.log, service, backendSvc.Port)
	if err != nil {
		return nil, nil, err
	}

	return service, servicePort, nil
}

// MigrateKubernetesIngressControllerLabelsToNgrokOperator migrates the labels from the old Kubernetes Ingress Controller to the new ngrok operator labels
// so that the ngrok operator can take over management of items previously managed by the Kubernetes Ingress Controller.
// TODO: Delete this function after users have migrated from the ngrok Kubernetes Ingress Controller to the ngrok Operator.
func (d *Driver) MigrateKubernetesIngressControllerLabelsToNgrokOperator(ctx context.Context, k8sClient client.Client) error {
	typesToMigrate := []interface{}{
		&ingressv1alpha1.Domain{},
		&ingressv1alpha1.Tunnel{},
		&ingressv1alpha1.HTTPSEdge{},
	}

	for _, t := range typesToMigrate {
		objs, err := listObjectsForType(ctx, k8sClient, t)
		if err != nil {
			return err
		}

		for _, obj := range objs {

			name := obj.GetName()
			namespace := obj.GetNamespace()
			kind := obj.GetObjectKind().GroupVersionKind().Kind

			log := d.log.WithValues("name", name, "namespace", namespace, "kind", kind)

			labels := obj.GetLabels()

			controllerName, controllerNameOk := labels[labelControllerName]
			controllerNamespace, controllerNamespaceOk := labels[labelControllerNamespace]

			// If it doesn't have both of the controller name and namespace labels, skip it
			if !controllerNameOk || !controllerNamespaceOk {
				log.V(1).Info("Skipping object without controller name and namespace labels")
				continue
			}

			// If the controller name and namespace are the same as the current controller name and namespace, skip it
			if controllerName == d.managerName.Name && controllerNamespace == d.managerName.Namespace {
				log.V(1).Info("Skipping object with matching controller name and namespace labels")
				continue
			}

			// Deep copy the object so we can modify it.
			// We must also re-assign the labels to the new object so that
			// we don't modify the original object's labels.
			newObj := obj.DeepCopyObject().(client.Object)
			labels = newObj.GetLabels()

			// Now we know it has both labels and at least one of the two labels doesn't match
			// so we can update the labels to match what the new operator expects.
			labels[labelControllerName] = d.managerName.Name
			labels[labelControllerNamespace] = d.managerName.Namespace

			newObj.SetLabels(labels)

			if err := k8sClient.Patch(ctx, newObj, client.MergeFrom(obj)); err != nil {
				return err
			}
			log.V(1).Info("Migrated labels")
		}
	}
	return nil
}

func (d *Driver) defaultManagedResourceLabels() map[string]string {
	return map[string]string{
		labelControllerNamespace: d.managerName.Namespace,
		labelControllerName:      d.managerName.Name,
	}
}
