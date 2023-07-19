package store

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/go-logr/logr"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	netv1 "k8s.io/api/networking/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	ingressv1alpha1 "github.com/ngrok/kubernetes-ingress-controller/api/v1alpha1"
	"github.com/ngrok/kubernetes-ingress-controller/internal/annotations"
	"github.com/ngrok/kubernetes-ingress-controller/internal/errors"
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
	customMetadata        string
	deploymentRef         metav1.OwnerReference
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
func (d *Driver) Seed(ctx context.Context, c client.Reader, deploymentName types.NamespacedName) error {
	if deploymentName.Name != "test-deployment" {
		deployment := appsv1.Deployment{}
		if err := c.Get(ctx, deploymentName, &deployment); err != nil {
			return err
		}
		if deployment.APIVersion == "" {
			deployment.APIVersion = "apps/v1"
		}
		if deployment.Kind == "" {
			deployment.Kind = "Deployment"
		}
		b := true
		d.deploymentRef.APIVersion = deployment.APIVersion
		d.deploymentRef.Kind = deployment.Kind
		d.deploymentRef.Name = deployment.Name
		d.deploymentRef.UID = deployment.UID
		d.deploymentRef.Controller = &b
		d.log.Info("loaded reference to deployment", "ref", d)
	}

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

	services := &corev1.ServiceList{}
	if err := c.List(ctx, services); err != nil {
		return err
	}
	for _, svc := range services.Items {
		if err := d.Update(&svc); err != nil {
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

	// update or delete tunnels we don't need anymore
	for _, currTunnel := range currTunnels.Items {
		// extract tunnel key and check if its controller managed
		if tkey, ok := tunnelKeyFromTunnel(currTunnel); ok {
			// check if new state needs this tunnel
			if desiredTunnel, ok := desiredTunnels[tkey]; ok {
				needsUpdate := false
				// check owner references
				if !reflect.DeepEqual(desiredTunnel.OwnerReferences, currTunnel.OwnerReferences) {
					needsUpdate = true
					currTunnel.OwnerReferences = desiredTunnel.OwnerReferences
				}

				// check/update spec
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

				delete(desiredTunnels, tkey)
			} else {
				// no longer needed, delete it
				if err := c.Delete(ctx, &currTunnel); client.IgnoreNotFound(err) != nil {
					d.log.Error(err, "error deleting tunnel", "tunnel", currTunnel)
					return err
				}
			}
		}
	}

	// add any unmatched tunnels
	for _, tunnel := range desiredTunnels {
		if err := c.Create(ctx, &tunnel); err != nil {
			d.log.Error(err, "error creating tunnel", "tunnel", tunnel)
			return err
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
		resolvedMod, err := d.Storer.GetNgrokModuleSetV1(module, ing.Namespace)
		if err != nil {
			return computedModSet, err
		}
		computedModSet.Merge(resolvedMod)
	}

	return computedModSet, nil
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
		edge.Spec.Metadata = d.customMetadata
		var ngrokRoutes []ingressv1alpha1.HTTPSEdgeRouteSpec
		for _, ingress := range ingresses {
			namespace := ingress.Namespace

			modSet, err := d.getNgrokModuleSetForIngress(ingress)
			if err != nil {
				d.log.Error(err, "error getting ngrok moduleset for ingress", "ingress", ingress)
				continue
			}

			// Set edge specific modules
			if modSet.Modules.TLSTermination != nil {
				edge.Spec.TLSTermination = modSet.Modules.TLSTermination
			}

			for _, rule := range ingress.Spec.Rules {
				// If any rule for an ingress matches, then it applies to this ingress
				// TODO: Handle routes without hosts that then apply to all edges
				if rule.Host == domain.Spec.Domain {

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
						servicePort, _, err := d.getBackendServicePort(*httpIngressPath.Backend.Service, namespace)
						if err != nil {
							d.log.Error(err, "could not find port for service", "namespace", namespace, "service", serviceName)
							continue
						}

						route := ingressv1alpha1.HTTPSEdgeRouteSpec{
							Match:     httpIngressPath.Path,
							MatchType: matchType,
							Backend: ingressv1alpha1.TunnelGroupBackend{
								Labels: ngrokLabels(namespace, serviceName, servicePort),
							},
							CircuitBreaker:      modSet.Modules.CircuitBreaker,
							Compression:         modSet.Modules.Compression,
							IPRestriction:       modSet.Modules.IPRestriction,
							Headers:             modSet.Modules.Headers,
							OAuth:               modSet.Modules.OAuth,
							OIDC:                modSet.Modules.OIDC,
							SAML:                modSet.Modules.SAML,
							WebhookVerification: modSet.Modules.WebhookVerification,
						}
						route.Metadata = d.customMetadata

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

type tunnelKey struct {
	namespace string
	service   string
	port      int32
}

func tunnelKeyFromTunnel(tunnel ingressv1alpha1.Tunnel) (tunnelKey, bool) {
	if tunnel.Labels["k8s.ngrok.com/managed"] != "true" {
		return tunnelKey{}, false
	}

	service := tunnel.Labels["k8s.ngrok.com/service"]
	port, err := strconv.ParseInt(tunnel.Labels["k8s.ngrok.com/port"], 10, 32)
	if err != nil {
		// return err?
		return tunnelKey{}, false
	}

	return tunnelKey{tunnel.Namespace, service, int32(port)}, true
}

func (d *Driver) calculateTunnels() map[tunnelKey]ingressv1alpha1.Tunnel {
	tunnels := map[tunnelKey]ingressv1alpha1.Tunnel{}

	for _, ingress := range d.ListNgrokIngressesV1() {
		for _, rule := range ingress.Spec.Rules {
			for _, path := range rule.HTTP.Paths {
				// We only support service backends right now. TODO: support resource backends
				if path.Backend.Service == nil {
					continue
				}

				serviceName := path.Backend.Service.Name
				servicePort, protocol, err := d.getBackendServicePort(*path.Backend.Service, ingress.Namespace)
				if err != nil {
					d.log.Error(err, "could not find port for service", "namespace", ingress.Namespace, "service", serviceName)
				}

				key := tunnelKey{ingress.Namespace, serviceName, servicePort}
				tunnel, found := tunnels[key]
				if !found {
					targetAddr := fmt.Sprintf("%s.%s.%s:%d", serviceName, key.namespace, clusterDomain, servicePort)
					tunnel = ingressv1alpha1.Tunnel{
						ObjectMeta: metav1.ObjectMeta{
							GenerateName:    fmt.Sprintf("%s-%d", serviceName, servicePort),
							Namespace:       ingress.Namespace,
							OwnerReferences: []metav1.OwnerReference{d.deploymentRef},
							Labels:          k8sLabels(serviceName, servicePort),
						},
						Spec: ingressv1alpha1.TunnelSpec{
							ForwardsTo: targetAddr,
							Labels:     ngrokLabels(ingress.Namespace, serviceName, servicePort),
							BackendConfig: &ingressv1alpha1.BackendConfig{
								Protocol: protocol,
							},
						},
					}
				}

				// tval.refs[metav1.OwnerReference{
				// 	APIVersion: ingress.APIVersion,
				// 	Kind:       ingress.Kind,
				// 	Name:       ingress.Name,
				// 	UID:        ingress.UID,
				// }] = struct{}{}

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

func (d *Driver) getBackendServicePort(backendSvc netv1.IngressServiceBackend, namespace string) (int32, string, error) {
	service, err := d.GetServiceV1(backendSvc.Name, namespace)
	if err != nil {
		return 0, "", err
	}

	servicePort, err := d.findServicesPort(service, backendSvc.Port)
	if err != nil {
		return 0, "", err
	}

	protocol, err := d.getPortAnnotatedProtocol(service, servicePort.Name)
	if err != nil {
		return 0, "", err
	}

	return servicePort.Port, protocol, nil
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

func k8sLabels(serviceName string, port int32) map[string]string {
	return map[string]string{
		"k8s.ngrok.com/managed": "true", // TODO should we replace with owner reference?
		"k8s.ngrok.com/service": serviceName,
		"k8s.ngrok.com/port":    strconv.Itoa(int(port)),
	}
}

// Generates a labels map for matching ngrok Routes to Agent Tunnels
func ngrokLabels(namespace, serviceName string, port int32) map[string]string {
	return map[string]string{
		"k8s.ngrok.com/namespace": namespace,
		"k8s.ngrok.com/service":   serviceName,
		"k8s.ngrok.com/port":      strconv.Itoa(int(port)),
	}
}
