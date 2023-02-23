package store

import (
	"context"
	"fmt"
	"reflect"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/go-logr/logr"
	netv1 "k8s.io/api/networking/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	ingressv1alpha1 "github.com/ngrok/kubernetes-ingress-controller/api/v1alpha1"
	"github.com/ngrok/kubernetes-ingress-controller/internal/annotations"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const clusterDomain = "svc.cluster.local" // TODO: We can technically figure this out by looking at things like our resolv.conf or we can just take this as a helm option

// Driver maintains the store of information, can derive new information from the store, and can
// synchronize the desired state of the store to the actual state of the cluster.
type Driver struct {
	Storer

	cacheStores           CacheStores
	log                   logr.Logger
	scheme                *runtime.Scheme
	reentranceFlag        int64
	bypassReentranceCheck bool
}

// NewDriver creates a new driver with a basic logger and cache store setup
func NewDriver(logger logr.Logger, scheme *runtime.Scheme, controllerName string) *Driver {
	cacheStores := NewCacheStores(logger)
	s := New(cacheStores, controllerName, logger)
	return &Driver{
		Storer:      s,
		cacheStores: cacheStores,
		log:         logger,
		scheme:      scheme,
	}
}

// Seed fetches all the upfront information the driver needs to operate
// It needs to be seeded fully before it can be used to make calculations otherwise
// each calculation will be based on an incomplete state of the world. It currently relies on:
// - Ingresses
// - IngressClasses
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
		if err := d.Update(&ing); err != nil {
			return err
		}
	}

	ingressClasses := &netv1.IngressClassList{}
	if err := c.List(ctx, ingressClasses); err != nil {
		return err
	}
	for _, ingClass := range ingressClasses.Items {
		if err := d.Update(&ingClass); err != nil {
			return err
		}
	}

	domains := &ingressv1alpha1.DomainList{}
	if err := c.List(ctx, domains); err != nil {
		return err
	}
	for _, domain := range domains.Items {
		if err := d.Update(&domain); err != nil {
			return err
		}
	}

	edges := &ingressv1alpha1.HTTPSEdgeList{}
	if err := c.List(ctx, edges); err != nil {
		return err
	}
	for _, edge := range edges.Items {
		if err := d.Update(&edge); err != nil {
			return err
		}
	}

	return nil
}

// Delete an ingress object given the NamespacedName
// Takes a namespacedName string as a parameter and
// deletes the ingress object from the cacheStores map
func (d *Driver) DeleteIngress(n types.NamespacedName) error {
	ingress := &netv1.Ingress{}
	// set NamespacedName on the ingress object
	ingress.SetNamespace(n.Namespace)
	ingress.SetName(n.Name)
	return d.cacheStores.Delete(ingress)
}

// Sync calculates what the desired state for each of our CRDs should be based on the ingresses and other
// objects in the store. It then compares that to the actual state of the cluster and updates the cluster
func (d *Driver) Sync(ctx context.Context, c client.Client) error {
	// This function gets called a lot in the current architecture. At the end it also syncs
	// resources which in turn triggers more reconcile events. Its all eventually consistent, but
	// its noisy and can make us hit ngrok api limits. We should probably just change this to be
	// a periodic sync instead of a sync on every reconcile event, but for now this debouncer
	// keeps it in check and syncs in batches
	if !d.bypassReentranceCheck {
		if atomic.CompareAndSwapInt64(&d.reentranceFlag, 0, 1) {

			defer func() {
				time.Sleep(10 * time.Second)
				atomic.StoreInt64(&(d.reentranceFlag), 0)
			}()
		} else {
			d.log.Info("sync already in progress, skipping")
			return nil
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
	if err := c.List(ctx, currEdges); err != nil {
		d.log.Error(err, "error listing edges")
		return err
	}
	if err := c.List(ctx, currTunnels); err != nil {
		d.log.Error(err, "error listing tunnels")
		return err
	}

	for _, desiredDomain := range desiredDomains {
		found := false
		for _, currDomain := range currDomains.Items {
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
			break
		}
	}
	// Don't delete domains to prevent accidentally de-registering them and making people re-do DNS

	for _, desiredEdge := range desiredEdges {
		found := false
		for _, currEdge := range currEdges.Items {
			if desiredEdge.Name == currEdge.Name && desiredEdge.Namespace == currEdge.Namespace {
				// It matches so lets update it if anything is different
				if !reflect.DeepEqual(desiredEdge.Spec, currEdge.Spec) {
					currEdge.Spec = desiredEdge.Spec
					if err := c.Update(ctx, &currEdge); err != nil {
						d.log.Error(err, "error updating edge", "desiredEdge", desiredEdge, "currEdge", currEdge)
						return err
					}
				}
				found = true
				break
			}
		}
		if !found {
			if err := c.Create(ctx, &desiredEdge); err != nil {
				return err
			}
			break
		}
	}

	for _, existingEdge := range currEdges.Items {
		found := false
		for _, desiredEdge := range desiredEdges {
			if desiredEdge.Name == existingEdge.Name && desiredEdge.Namespace == existingEdge.Namespace {
				found = true
				break
			}
		}
		if !found {
			if err := c.Delete(ctx, &existingEdge); client.IgnoreNotFound(err) != nil {
				d.log.Error(err, "error deleting edge", "edge", existingEdge)
				return err
			}
		}
	}

	for _, desiredTunnel := range desiredTunnels {
		found := false
		for _, currTunnel := range currTunnels.Items {
			if desiredTunnel.Name == currTunnel.Name && desiredTunnel.Namespace == currTunnel.Namespace {
				// It matches so lets update it if anything is different
				if !reflect.DeepEqual(desiredTunnel.Spec, currTunnel.Spec) {
					currTunnel.Spec = desiredTunnel.Spec
					if err := c.Update(ctx, &currTunnel); err != nil {
						d.log.Error(err, "error updating tunnel", "tunnel", desiredTunnel)
						return err
					}
				}
				found = true
				break
			}
		}
		if !found {
			if err := c.Create(ctx, &desiredTunnel); err != nil {
				d.log.Error(err, "error creating tunnel", "tunnel", desiredTunnel)
				return err
			}
			break
		}
	}

	for _, existingTunnel := range currTunnels.Items {
		found := false
		for _, desiredTunnel := range desiredTunnels {
			if desiredTunnel.Name == existingTunnel.Name && desiredTunnel.Namespace == existingTunnel.Namespace {
				found = true
				break
			}
		}
		if !found {
			if err := c.Delete(ctx, &existingTunnel); client.IgnoreNotFound(err) != nil {
				d.log.Error(err, "error deleting tunnel", "tunnel", existingTunnel)
				return err
			}
		}
	}

	return d.updateIngressStatuses(ctx, c)
}

func (d *Driver) updateIngressStatuses(ctx context.Context, c client.Client) error {
	ingresses := d.ListNgrokIngressesV1()
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
	ingresses := d.ListNgrokIngressesV1()
	for _, ingress := range ingresses {
		for _, rule := range ingress.Spec.Rules {
			if rule.Host == "" {
				continue
			}
			domainMap[rule.Host] = ingressv1alpha1.Domain{
				ObjectMeta: metav1.ObjectMeta{
					Name:      strings.Replace(rule.Host, ".", "-", -1),
					Namespace: ingress.Namespace,
				},
				Spec: ingressv1alpha1.DomainSpec{
					Domain: rule.Host,
				},
			}
		}
	}
	domains := make([]ingressv1alpha1.Domain, 0, len(domainMap))
	for _, domain := range domainMap {
		domains = append(domains, domain)
	}
	return domains
}

func (d *Driver) calculateHTTPSEdges() []ingressv1alpha1.HTTPSEdge {
	domains := d.calculateDomains()
	ingresses := d.ListNgrokIngressesV1()
	edges := make([]ingressv1alpha1.HTTPSEdge, 0, len(domains))
	for _, domain := range domains {
		edge := ingressv1alpha1.HTTPSEdge{
			ObjectMeta: metav1.ObjectMeta{
				Name:      domain.Name,
				Namespace: domain.Namespace,
			},
			Spec: ingressv1alpha1.HTTPSEdgeSpec{
				Hostports: []string{domain.Spec.Domain + ":443"},
			},
		}
		var ngrokRoutes []ingressv1alpha1.HTTPSEdgeRouteSpec
		for _, ingress := range ingresses {
			for _, rule := range ingress.Spec.Rules {
				// If any rule for an ingress matches, then it applies to this ingress
				// TODO: Handle routes without hosts that then apply to all edges
				if rule.Host == domain.Spec.Domain {
					// If any of them have the tls termination annotation, then we should set it for the whole edge
					parsedRouteModules := annotations.NewAnnotationsExtractor().Extract(ingress)
					if parsedRouteModules != nil && parsedRouteModules.TLSTermination != nil {
						edge.Spec.TLSTermination = parsedRouteModules.TLSTermination
					}

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
								return nil
							}
						}

						route := ingressv1alpha1.HTTPSEdgeRouteSpec{
							Match:     httpIngressPath.Path,
							MatchType: matchType,
							Backend: ingressv1alpha1.TunnelGroupBackend{
								Labels: backendToLabelMap(httpIngressPath.Backend, ingress.Namespace),
							},
							Compression:   parsedRouteModules.Compression,
							IPRestriction: parsedRouteModules.IPRestriction,
							Headers:       parsedRouteModules.Headers,
						}

						ngrokRoutes = append(ngrokRoutes, route)
					}
				}
			}
		}
		// After all the ingresses, update the edge with the routes
		edge.Spec.Routes = ngrokRoutes
		edges = append(edges, edge)
	}

	return edges
}

func (d *Driver) calculateTunnels() []ingressv1alpha1.Tunnel {
	// Tunnels should be unique on a service and port basis so if they are referenced more than once, we
	// only create one tunnel per service and port.
	tunnelMap := make(map[string]ingressv1alpha1.Tunnel)
	ingresses := d.ListNgrokIngressesV1()
	for _, ingress := range ingresses {
		for _, rule := range ingress.Spec.Rules {
			for _, path := range rule.HTTP.Paths {
				serviceName := path.Backend.Service.Name
				servicePort := path.Backend.Service.Port.Number
				tunnelAddr := fmt.Sprintf("%s.%s.%s:%d", serviceName, ingress.Namespace, clusterDomain, servicePort)
				tunnelName := fmt.Sprintf("%s-%d", serviceName, servicePort)

				tunnelMap[tunnelName] = ingressv1alpha1.Tunnel{
					ObjectMeta: metav1.ObjectMeta{
						Name:      tunnelName,
						Namespace: ingress.Namespace,
					},
					Spec: ingressv1alpha1.TunnelSpec{
						ForwardsTo: tunnelAddr,
						Labels:     backendToLabelMap(path.Backend, ingress.Namespace),
					},
				}
			}
		}
	}

	tunnels := make([]ingressv1alpha1.Tunnel, 0, len(tunnelMap))
	for _, tunnel := range tunnelMap {
		tunnels = append(tunnels, tunnel)
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

// Generates a labels map for matching ngrok Routes to Agent Tunnels
func backendToLabelMap(backend netv1.IngressBackend, namespace string) map[string]string {
	return map[string]string{
		"k8s.ngrok.com/namespace": namespace,
		"k8s.ngrok.com/service":   backend.Service.Name,
		"k8s.ngrok.com/port":      strconv.Itoa(int(backend.Service.Port.Number)),
	}
}
