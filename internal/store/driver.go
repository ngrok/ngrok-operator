package store

import (
	"cmp"
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"strconv"
	"strings"
	"sync"

	"github.com/go-logr/logr"
	"golang.org/x/exp/slices"
	corev1 "k8s.io/api/core/v1"
	netv1 "k8s.io/api/networking/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	ingressv1alpha1 "github.com/ngrok/kubernetes-ingress-controller/api/ingress/v1alpha1"
	"github.com/ngrok/kubernetes-ingress-controller/internal/annotations"
	"github.com/ngrok/kubernetes-ingress-controller/internal/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const clusterDomain = "svc.cluster.local" // TODO: We can technically figure this out by looking at things like our resolv.conf or we can just take this as a helm option

const (
	labelControllerNamespace = "k8s.ngrok.com/controller-namespace"
	labelControllerName      = "k8s.ngrok.com/controller-name"
	labelDomain              = "k8s.ngrok.com/domain"
	labelNamespace           = "k8s.ngrok.com/namespace"
	labelServiceUID          = "k8s.ngrok.com/service-uid"
	labelService             = "k8s.ngrok.com/service"
	labelPort                = "k8s.ngrok.com/port"
)

// Driver maintains the store of information, can derive new information from the store, and can
// synchronize the desired state of the store to the actual state of the cluster.
type Driver struct {
	store Storer

	cacheStores    CacheStores
	log            logr.Logger
	scheme         *runtime.Scheme
	customMetadata string
	managerName    types.NamespacedName

	syncMu              sync.Mutex
	syncRunning         bool
	syncFullCh          chan error
	syncPartialCh       chan error
	syncAllowConcurrent bool
}

// NewDriver creates a new driver with a basic logger and cache store setup
func NewDriver(logger logr.Logger, scheme *runtime.Scheme, controllerName string, managerName types.NamespacedName) *Driver {
	cacheStores := NewCacheStores(logger)
	s := New(cacheStores, controllerName, logger)
	return &Driver{
		store:       s,
		cacheStores: cacheStores,
		log:         logger,
		scheme:      scheme,
		managerName: managerName,
	}
}

// WithMetaData allows you to pass in custom metadata to be added to all resources created by the controller
func (d *Driver) WithMetaData(customMetadata map[string]string) *Driver {
	if _, ok := customMetadata["owned-by"]; !ok {
		customMetadata["owned-by"] = "kubernetes-ingress-controller"
	}
	jsonString, err := json.Marshal(customMetadata)
	if err != nil {
		d.log.Error(err, "error marshalling custom metadata", "customMetadata", d.customMetadata)
		return d
	}
	d.customMetadata = string(jsonString)
	return d
}

// Seed fetches all the upfront information the driver needs to operate
// It needs to be seeded fully before it can be used to make calculations otherwise
// each calculation will be based on an incomplete state of the world. It currently relies on:
// - Ingresses
// - IngressClasses
// - Services
// - Secrets
// - Domains
// - Edges
// When the sync method becomes a background process, this likely won't be needed anymore
func (d *Driver) Seed(ctx context.Context, c client.Reader) error {
	ingresses := &netv1.IngressList{}
	if err := c.List(ctx, ingresses); err != nil {
		return err
	}
	for _, ing := range ingresses.Items {
		if err := d.store.Update(&ing); err != nil {
			return err
		}
	}

	ingressClasses := &netv1.IngressClassList{}
	if err := c.List(ctx, ingressClasses); err != nil {
		return err
	}
	for _, ingClass := range ingressClasses.Items {
		if err := d.store.Update(&ingClass); err != nil {
			return err
		}
	}

	services := &corev1.ServiceList{}
	if err := c.List(ctx, services); err != nil {
		return err
	}
	for _, svc := range services.Items {
		if err := d.store.Update(&svc); err != nil {
			return err
		}
	}

	domains := &ingressv1alpha1.DomainList{}
	if err := c.List(ctx, domains); err != nil {
		return err
	}
	for _, domain := range domains.Items {
		if err := d.store.Update(&domain); err != nil {
			return err
		}
	}

	edges := &ingressv1alpha1.HTTPSEdgeList{}
	if err := c.List(ctx, edges); err != nil {
		return err
	}
	for _, edge := range edges.Items {
		if err := d.store.Update(&edge); err != nil {
			return err
		}
	}

	tunnels := &ingressv1alpha1.TunnelList{}
	if err := c.List(ctx, tunnels); err != nil {
		return err
	}
	for _, tunnel := range tunnels.Items {
		if err := d.store.Update(&tunnel); err != nil {
			return err
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

func (d *Driver) DeleteIngress(ingress *netv1.Ingress) error {
	return d.store.Delete(ingress)
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
			return false, func(ctx context.Context) error {
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
			d.log.Info("sync done", "err", err)
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
		if proceed, wait := d.syncStart(false); proceed {
			defer d.syncDone()
		} else {
			return wait(ctx)
		}
	}

	d.log.Info("syncing driver state!!")
	desiredDomains := d.calculateDomains()
	desiredEdges := d.calculateHTTPSEdges()
	desiredTunnels := d.calculateTunnels()

	currDomains := &ingressv1alpha1.DomainList{}
	currEdges := &ingressv1alpha1.HTTPSEdgeList{}
	currTunnels := &ingressv1alpha1.TunnelList{}

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

	if err := d.applyDomains(ctx, c, desiredDomains, currDomains.Items); err != nil {
		return err
	}

	if err := d.applyHTTPSEdges(ctx, c, desiredEdges, currEdges.Items); err != nil {
		return err
	}

	if err := d.applyTunnels(ctx, c, desiredTunnels, currTunnels.Items); err != nil {
		return err
	}

	if err := d.updateIngressStatuses(ctx, c); err != nil {
		return err
	}

	return nil
}

func (d *Driver) SyncEdges(ctx context.Context, c client.Client) error {
	if !d.syncAllowConcurrent {
		if proceed, wait := d.syncStart(true); proceed {
			defer d.syncDone()
		} else {
			return wait(ctx)
		}
	}

	d.log.Info("syncing edges state!!")

	desiredEdges := d.calculateHTTPSEdges()
	currEdges := &ingressv1alpha1.HTTPSEdgeList{}
	if err := c.List(ctx, currEdges, client.MatchingLabels{
		labelControllerNamespace: d.managerName.Namespace,
		labelControllerName:      d.managerName.Name,
	}); err != nil {
		d.log.Error(err, "error listing edges")
		return err
	}

	if err := d.applyHTTPSEdges(ctx, c, desiredEdges, currEdges.Items); err != nil {
		return err
	}

	return nil
}

func (d *Driver) applyDomains(ctx context.Context, c client.Client, desiredDomains, currentDomains []ingressv1alpha1.Domain) error {
	for _, desiredDomain := range desiredDomains {
		found := false
		for _, currDomain := range currentDomains {
			if desiredDomain.Name == currDomain.Name && desiredDomain.Namespace == currDomain.Namespace {
				// It matches so lets update it if anything is different
				if !reflect.DeepEqual(desiredDomain.Spec, currDomain.Spec) {
					currDomain.Spec = desiredDomain.Spec
					if err := c.Update(ctx, &currDomain); err != nil {
						d.log.Error(err, "error updating domain", "domain", desiredDomain)
						return err
					}
				}
				found = true
				break
			}
		}
		if !found {
			if err := c.Create(ctx, &desiredDomain); err != nil {
				d.log.Error(err, "error creating domain", "domain", desiredDomain)
				return err
			}
		}
	}

	// Don't delete domains to prevent accidentally de-registering them and making people re-do DNS

	return nil
}

func (d *Driver) applyHTTPSEdges(ctx context.Context, c client.Client, desiredEdges map[string]ingressv1alpha1.HTTPSEdge, currentEdges []ingressv1alpha1.HTTPSEdge) error {
	// update or delete edge we don't need anymore
	for _, currEdge := range currentEdges {
		domain := currEdge.Labels[labelDomain]

		if desiredEdge, ok := desiredEdges[domain]; ok {
			needsUpdate := false

			if !reflect.DeepEqual(desiredEdge.Spec, currEdge.Spec) {
				currEdge.Spec = desiredEdge.Spec
				needsUpdate = true
			}

			if needsUpdate {
				if err := c.Update(ctx, &currEdge); err != nil {
					d.log.Error(err, "error updating edge", "desiredEdge", desiredEdge, "currEdge", currEdge)
					return err
				}
			}

			// matched and updated the edge, no longer desired
			delete(desiredEdges, domain)
		} else {
			if err := c.Delete(ctx, &currEdge); client.IgnoreNotFound(err) != nil {
				d.log.Error(err, "error deleting edge", "edge", currEdge)
				return err
			}
		}
	}

	// the set of desired edges now only contains new edges, create them
	for _, edge := range desiredEdges {
		if err := c.Create(ctx, &edge); err != nil {
			d.log.Error(err, "error creating edge", "edge", edge)
			return err
		}
	}

	return nil
}

func (d *Driver) applyTunnels(ctx context.Context, c client.Client, desiredTunnels map[tunnelKey]ingressv1alpha1.Tunnel, currentTunnels []ingressv1alpha1.Tunnel) error {
	// update or delete tunnels we don't need anymore
	for _, currTunnel := range currentTunnels {
		// extract tunnel key
		tkey := d.tunnelKeyFromTunnel(currTunnel)

		// check if new state still needs this tunnel
		if desiredTunnel, ok := desiredTunnels[tkey]; ok {
			needsUpdate := false

			// compare/update owner references
			if !slices.Equal(desiredTunnel.OwnerReferences, currTunnel.OwnerReferences) {
				needsUpdate = true
				currTunnel.OwnerReferences = desiredTunnel.OwnerReferences
			}

			// compare/update desired tunnel spec
			if !reflect.DeepEqual(desiredTunnel.Spec, currTunnel.Spec) {
				needsUpdate = true
				currTunnel.Spec = desiredTunnel.Spec
			}

			if needsUpdate {
				if err := c.Update(ctx, &currTunnel); err != nil {
					d.log.Error(err, "error updating tunnel", "tunnel", desiredTunnel)
					return err
				}
			}

			// matched and updated the tunnel, no longer desired
			delete(desiredTunnels, tkey)
		} else {
			// no longer needed, delete it
			if err := c.Delete(ctx, &currTunnel); client.IgnoreNotFound(err) != nil {
				d.log.Error(err, "error deleting tunnel", "tunnel", currTunnel)
				return err
			}
		}
	}

	// the set of desired tunnels now only contains new tunnels, create them
	for _, tunnel := range desiredTunnels {
		if err := c.Create(ctx, &tunnel); err != nil {
			d.log.Error(err, "error creating tunnel", "tunnel", tunnel)
			return err
		}
	}

	return nil
}

func (d *Driver) updateIngressStatuses(ctx context.Context, c client.Client) error {
	ingresses := d.store.ListNgrokIngressesV1()
	for _, ingress := range ingresses {
		newLBIPStatus := d.calculateIngressLoadBalancerIPStatus(ingress, c)
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

func (d *Driver) calculateDomains() []ingressv1alpha1.Domain {
	// make a map of string to domains
	domainMap := make(map[string]ingressv1alpha1.Domain)
	ingresses := d.store.ListNgrokIngressesV1()
	for _, ingress := range ingresses {
		for _, rule := range ingress.Spec.Rules {
			if rule.Host == "" {
				continue
			}
			domain := ingressv1alpha1.Domain{
				ObjectMeta: metav1.ObjectMeta{
					Name:      strings.Replace(rule.Host, ".", "-", -1),
					Namespace: ingress.Namespace,
				},
				Spec: ingressv1alpha1.DomainSpec{
					Domain: rule.Host,
				},
			}
			domain.Spec.Metadata = d.customMetadata
			domainMap[rule.Host] = domain
		}
	}
	domains := make([]ingressv1alpha1.Domain, 0, len(domainMap))
	for _, domain := range domainMap {
		domains = append(domains, domain)
	}
	return domains
}

// Given an ingress, it will resolve any ngrok modulesets defined on the ingress to the
// CRDs and then will merge them in to a single moduleset
func (d *Driver) getNgrokModuleSetForIngress(ing *netv1.Ingress) (*ingressv1alpha1.NgrokModuleSet, error) {
	computedModSet := &ingressv1alpha1.NgrokModuleSet{}

	modules, err := annotations.ExtractNgrokModuleSetsFromAnnotations(ing)
	if err != nil {
		if errors.IsMissingAnnotations(err) {
			return computedModSet, nil
		}
		return computedModSet, err
	}

	for _, module := range modules {
		resolvedMod, err := d.store.GetNgrokModuleSetV1(module, ing.Namespace)
		if err != nil {
			return computedModSet, err
		}
		computedModSet.Merge(resolvedMod)
	}

	return computedModSet, nil
}

func (d *Driver) calculateHTTPSEdges() map[string]ingressv1alpha1.HTTPSEdge {
	domains := d.calculateDomains()

	edgeMap := make(map[string]ingressv1alpha1.HTTPSEdge, len(domains))
	for _, domain := range domains {
		edge := ingressv1alpha1.HTTPSEdge{
			ObjectMeta: metav1.ObjectMeta{
				GenerateName: domain.Name + "-",
				Namespace:    domain.Namespace,
				Labels:       d.edgeLabels(domain.Spec.Domain),
			},
			Spec: ingressv1alpha1.HTTPSEdgeSpec{
				Hostports: []string{domain.Spec.Domain + ":443"},
			},
		}
		edge.Spec.Metadata = d.customMetadata
		edgeMap[domain.Spec.Domain] = edge
	}

	ingresses := d.store.ListNgrokIngressesV1()
	for _, ingress := range ingresses {
		modSet, err := d.getNgrokModuleSetForIngress(ingress)
		if err != nil {
			d.log.Error(err, "error getting ngrok moduleset for ingress", "ingress", ingress)
			continue
		}

		for _, rule := range ingress.Spec.Rules {
			// TODO: Handle routes without hosts that then apply to all edges
			edge, ok := edgeMap[rule.Host]
			if !ok {
				d.log.Error(err, "could not find edge associated with rule", "host", rule.Host)
				continue
			}

			if modSet.Modules.TLSTermination != nil {
				edge.Spec.TLSTermination = modSet.Modules.TLSTermination
			}

			// If any rule for an ingress matches, then it applies to this ingress
			for _, httpIngressPath := range rule.HTTP.Paths {
				matchType := "path_prefix"
				if httpIngressPath.PathType != nil {
					switch *httpIngressPath.PathType {
					case netv1.PathTypePrefix:
						matchType = "path_prefix"
					case netv1.PathTypeExact:
						matchType = "exact_path"
					case netv1.PathTypeImplementationSpecific:
						matchType = "path_prefix" // Path Prefix seems like a sane default for most cases
					default:
						d.log.Error(fmt.Errorf("unknown path type"), "unknown path type", "pathType", *httpIngressPath.PathType)
						continue
					}
				}

				// We only support service backends right now. TODO: support resource backends
				if httpIngressPath.Backend.Service == nil {
					continue
				}

				serviceName := httpIngressPath.Backend.Service.Name
				serviceUID, servicePort, err := d.getEdgeBackend(*httpIngressPath.Backend.Service, ingress.Namespace)
				if err != nil {
					d.log.Error(err, "could not find port for service", "namespace", ingress.Namespace, "service", serviceName)
					continue
				}

				route := ingressv1alpha1.HTTPSEdgeRouteSpec{
					Match:     httpIngressPath.Path,
					MatchType: matchType,
					Backend: ingressv1alpha1.TunnelGroupBackend{
						Labels: d.ngrokLabels(ingress.Namespace, serviceUID, serviceName, servicePort),
					},
					CircuitBreaker:      modSet.Modules.CircuitBreaker,
					Compression:         modSet.Modules.Compression,
					IPRestriction:       modSet.Modules.IPRestriction,
					Headers:             modSet.Modules.Headers,
					OAuth:               modSet.Modules.OAuth,
					Policies:            modSet.Modules.Policies,
					OIDC:                modSet.Modules.OIDC,
					SAML:                modSet.Modules.SAML,
					WebhookVerification: modSet.Modules.WebhookVerification,
				}
				route.Metadata = d.customMetadata

				edge.Spec.Routes = append(edge.Spec.Routes, route)
			}

			edgeMap[rule.Host] = edge
		}
	}

	return edgeMap
}

type tunnelKey struct {
	namespace string
	service   string
	port      string
}

func (d *Driver) tunnelKeyFromTunnel(tunnel ingressv1alpha1.Tunnel) tunnelKey {
	return tunnelKey{
		namespace: tunnel.Namespace,
		service:   tunnel.Labels[labelService],
		port:      tunnel.Labels[labelPort],
	}
}

func (d *Driver) calculateTunnels() map[tunnelKey]ingressv1alpha1.Tunnel {
	tunnels := map[tunnelKey]ingressv1alpha1.Tunnel{}

	for _, ingress := range d.store.ListNgrokIngressesV1() {
		for _, rule := range ingress.Spec.Rules {
			for _, path := range rule.HTTP.Paths {
				// We only support service backends right now. TODO: support resource backends
				if path.Backend.Service == nil {
					continue
				}

				serviceName := path.Backend.Service.Name
				serviceUID, servicePort, protocol, appProtocol, err := d.getTunnelBackend(*path.Backend.Service, ingress.Namespace)
				if err != nil {
					d.log.Error(err, "could not find port for service", "namespace", ingress.Namespace, "service", serviceName)
				}

				key := tunnelKey{ingress.Namespace, serviceName, strconv.Itoa(int(servicePort))}
				tunnel, found := tunnels[key]
				if !found {
					targetAddr := fmt.Sprintf("%s.%s.%s:%d", serviceName, key.namespace, clusterDomain, servicePort)
					tunnel = ingressv1alpha1.Tunnel{
						ObjectMeta: metav1.ObjectMeta{
							GenerateName:    fmt.Sprintf("%s-%d-", serviceName, servicePort),
							Namespace:       ingress.Namespace,
							OwnerReferences: nil, // fill owner references below
							Labels:          d.tunnelLabels(serviceName, servicePort),
						},
						Spec: ingressv1alpha1.TunnelSpec{
							ForwardsTo: targetAddr,
							Labels:     d.ngrokLabels(ingress.Namespace, serviceUID, serviceName, servicePort),
							BackendConfig: &ingressv1alpha1.BackendConfig{
								Protocol: protocol,
							},
							AppProtocol: appProtocol,
						},
					}
				}

				hasIngressReference := false
				for _, ref := range tunnel.OwnerReferences {
					if ref.UID == ingress.UID {
						hasIngressReference = true
						break
					}
				}
				if !hasIngressReference {
					tunnel.OwnerReferences = append(tunnel.OwnerReferences, metav1.OwnerReference{
						APIVersion: ingress.APIVersion,
						Kind:       ingress.Kind,
						Name:       ingress.Name,
						UID:        ingress.UID,
					})
					slices.SortStableFunc(tunnel.OwnerReferences, func(i, j metav1.OwnerReference) int {
						return cmp.Compare(string(i.UID), string(j.UID))
					})
				}

				tunnels[key] = tunnel
			}
		}
	}

	return tunnels
}

func (d *Driver) calculateIngressLoadBalancerIPStatus(ing *netv1.Ingress, c client.Reader) []netv1.IngressLoadBalancerIngress {
	domains := &ingressv1alpha1.DomainList{}
	if err := c.List(context.Background(), domains); err != nil {
		d.log.Error(err, "failed to list domains")
		return []netv1.IngressLoadBalancerIngress{}
	}

	hostnames := make(map[string]netv1.IngressLoadBalancerIngress)
	for _, domain := range domains.Items {
		for _, rule := range ing.Spec.Rules {
			if rule.Host == domain.Spec.Domain && domain.Status.CNAMETarget != nil {
				hostnames[domain.Spec.Domain] = netv1.IngressLoadBalancerIngress{
					Hostname: *domain.Status.CNAMETarget,
				}
			}
		}
	}
	status := []netv1.IngressLoadBalancerIngress{}
	for _, hostname := range hostnames {
		status = append(status, hostname)
	}
	return status
}

func (d *Driver) getEdgeBackend(backendSvc netv1.IngressServiceBackend, namespace string) (string, int32, error) {
	service, servicePort, err := d.findBackendServicePort(backendSvc, namespace)
	if err != nil {
		return "", 0, err
	}

	return string(service.UID), servicePort.Port, nil
}

func (d *Driver) getTunnelBackend(backendSvc netv1.IngressServiceBackend, namespace string) (string, int32, string, string, error) {
	service, servicePort, err := d.findBackendServicePort(backendSvc, namespace)
	if err != nil {
		return "", 0, "", "", err
	}

	protocol, err := d.getPortAnnotatedProtocol(service, servicePort.Name)
	if err != nil {
		return "", 0, "", "", err
	}

	appProtocol, err := d.getPortAppProtocol(service, servicePort)
	if err != nil {
		return "", 0, "", "", err
	}

	return string(service.UID), servicePort.Port, protocol, appProtocol, nil
}

func (d *Driver) findBackendServicePort(backendSvc netv1.IngressServiceBackend, namespace string) (*corev1.Service, *corev1.ServicePort, error) {
	service, err := d.store.GetServiceV1(backendSvc.Name, namespace)
	if err != nil {
		return nil, nil, err
	}

	servicePort, err := d.findServicesPort(service, backendSvc.Port)
	if err != nil {
		return nil, nil, err
	}

	return service, servicePort, nil
}

func (d *Driver) findServicesPort(service *corev1.Service, backendSvcPort netv1.ServiceBackendPort) (*corev1.ServicePort, error) {
	for _, port := range service.Spec.Ports {
		if (backendSvcPort.Number > 0 && port.Port == backendSvcPort.Number) || port.Name == backendSvcPort.Name {
			d.log.V(3).Info("Found matching port for service", "namespace", service.Namespace, "service", service.Name, "port.name", port.Name, "port.number", port.Port)
			return &port, nil
		}
	}
	return nil, fmt.Errorf("could not find matching port for service %s, backend port %v, name %s", service.Name, backendSvcPort.Number, backendSvcPort.Name)
}

func (d *Driver) getPortAnnotatedProtocol(service *corev1.Service, portName string) (string, error) {
	if service.Annotations != nil {
		annotation := service.Annotations["k8s.ngrok.com/app-protocols"]
		if annotation != "" {
			d.log.V(3).Info("Annotated app-protocols found", "annotation", annotation, "namespace", service.Namespace, "service", service.Name, "portName", portName)
			m := map[string]string{}
			err := json.Unmarshal([]byte(annotation), &m)
			if err != nil {
				return "", fmt.Errorf("Could not parse protocol annotation: '%s' from: %s service: %s", annotation, service.Namespace, service.Name)
			}

			if protocol, ok := m[portName]; ok {
				d.log.V(3).Info("Found protocol for port name", "protocol", protocol, "namespace", service.Namespace, "service", service.Name)
				// only allow cases through where we are sure of intent
				switch upperProto := strings.ToUpper(protocol); upperProto {
				case "HTTP", "HTTPS":
					return upperProto, nil
				default:
					return "", fmt.Errorf("Unhandled protocol annotation: '%s', must be 'HTTP' or 'HTTPS'. From: %s service: %s", upperProto, service.Namespace, service.Name)
				}
			}
		}
	}
	return "HTTP", nil
}

func (d *Driver) getPortAppProtocol(service *corev1.Service, port *corev1.ServicePort) (string, error) {
	if port.AppProtocol == nil {
		return "", nil
	}

	switch proto := *port.AppProtocol; proto {
	case "k8s.ngrok.com/http2", "kubernetes.io/h2c":
		return "http2", nil
	case "":
		return "", nil
	default:
		return "", fmt.Errorf("Unsupported appProtocol: '%s', must be 'k8s.ngrok.com/http2', 'kubernetes.io/h2c' or ''. From: %s service: %s", proto, service.Namespace, service.Name)
	}
}

func (d *Driver) edgeLabels(domain string) map[string]string {
	return map[string]string{
		labelControllerNamespace: d.managerName.Namespace,
		labelControllerName:      d.managerName.Name,
		labelDomain:              domain,
	}
}

func (d *Driver) tunnelLabels(serviceName string, port int32) map[string]string {
	return map[string]string{
		labelControllerNamespace: d.managerName.Namespace,
		labelControllerName:      d.managerName.Name,
		labelService:             serviceName,
		labelPort:                strconv.Itoa(int(port)),
	}
}

// Generates a labels map for matching ngrok Routes to Agent Tunnels
func (d *Driver) ngrokLabels(namespace, serviceUID, serviceName string, port int32) map[string]string {
	return map[string]string{
		labelNamespace:  namespace,
		labelServiceUID: serviceUID,
		labelService:    serviceName,
		labelPort:       strconv.Itoa(int(port)),
	}
}
