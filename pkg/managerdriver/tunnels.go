package managerdriver

import (
	"cmp"
	"context"
	"fmt"
	"reflect"
	"strconv"

	common "github.com/ngrok/ngrok-operator/api/common/v1alpha1"
	ingressv1alpha1 "github.com/ngrok/ngrok-operator/api/ingress/v1alpha1"
	"golang.org/x/exp/slices"
	netv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"
)

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
	d.calculateTunnelsFromIngress(tunnels)
	d.calculateTunnelsFromGateway(tunnels)
	return tunnels
}

func (d *Driver) calculateTunnelsFromIngress(tunnels map[tunnelKey]ingressv1alpha1.Tunnel) {
	for _, ingress := range d.store.ListNgrokIngressesV1() {
		for _, rule := range ingress.Spec.Rules {
			for _, path := range rule.HTTP.Paths {
				// We only support service backends right now.
				// TODO: support resource backends
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
					targetAddr := fmt.Sprintf("%s.%s.%s:%d", serviceName, key.namespace, d.clusterDomain, servicePort)
					tunnel = ingressv1alpha1.Tunnel{
						ObjectMeta: metav1.ObjectMeta{
							GenerateName:    fmt.Sprintf("%s-%d-", serviceName, servicePort),
							Namespace:       ingress.Namespace,
							OwnerReferences: nil, // fill owner references below
							Labels:          d.tunnelLabels(serviceName, servicePort),
						},
						Spec: ingressv1alpha1.TunnelSpec{
							ForwardsTo: targetAddr,
							Labels:     ngrokLabels(ingress.Namespace, serviceUID, serviceName, servicePort),
							BackendConfig: &ingressv1alpha1.BackendConfig{
								Protocol: protocol,
							},
						},
					}
					switch appProtocol {
					case "http2":
						proto := common.ApplicationProtocol_HTTP2
						tunnel.Spec.AppProtocol = &proto
					case "http1":
						proto := common.ApplicationProtocol_HTTP1
						tunnel.Spec.AppProtocol = &proto
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
}

func (d *Driver) calculateTunnelsFromGateway(tunnels map[tunnelKey]ingressv1alpha1.Tunnel) {
	httproutes := d.store.ListHTTPRoutes()

	for _, httproute := range httproutes {
		for _, rule := range httproute.Spec.Rules {
			for _, backendRef := range rule.BackendRefs {
				// We only support service backends right now.
				// TODO: support resource backends

				// if path.Backend.Service == nil {
				//   continue
				// }

				serviceName := string(backendRef.Name)
				serviceUID, servicePort, protocol, appProtocol, err := d.getTunnelBackendFromGateway(backendRef.BackendRef, httproute.Namespace)
				if err != nil {
					d.log.Error(err, "could not find port for service", "namespace", httproute.Namespace, "service", serviceName)
				}

				key := tunnelKey{httproute.Namespace, serviceName, strconv.Itoa(int(servicePort))}
				tunnel, found := tunnels[key]
				if !found {
					targetAddr := fmt.Sprintf("%s.%s.%s:%d", serviceName, key.namespace, d.clusterDomain, servicePort)
					tunnel = ingressv1alpha1.Tunnel{
						ObjectMeta: metav1.ObjectMeta{
							GenerateName:    fmt.Sprintf("%s-%d-", serviceName, servicePort),
							Namespace:       httproute.Namespace,
							OwnerReferences: nil, // fill owner references below
							Labels:          d.tunnelLabels(serviceName, servicePort),
						},
						Spec: ingressv1alpha1.TunnelSpec{
							ForwardsTo: targetAddr,
							Labels:     ngrokLabels(httproute.Namespace, serviceUID, serviceName, servicePort),
							BackendConfig: &ingressv1alpha1.BackendConfig{
								Protocol: protocol,
							},
						},
					}
					switch appProtocol {
					case "http2":
						proto := common.ApplicationProtocol_HTTP2
						tunnel.Spec.AppProtocol = &proto
					case "http1":
						proto := common.ApplicationProtocol_HTTP1
						tunnel.Spec.AppProtocol = &proto
					}
				}

				hasReference := false
				for _, ref := range tunnel.OwnerReferences {
					if ref.UID == httproute.UID {
						hasReference = true
						break
					}
				}
				if !hasReference {
					tunnel.OwnerReferences = append(tunnel.OwnerReferences, metav1.OwnerReference{
						APIVersion: httproute.APIVersion,
						Kind:       httproute.Kind,
						Name:       httproute.Name,
						UID:        httproute.UID,
					})
					slices.SortStableFunc(tunnel.OwnerReferences, func(i, j metav1.OwnerReference) int {
						return cmp.Compare(string(i.UID), string(j.UID))
					})
				}

				tunnels[key] = tunnel
			}
		}
	}
}

func (d *Driver) getTunnelBackend(backendSvc netv1.IngressServiceBackend, namespace string) (string, int32, string, string, error) {
	service, servicePort, err := d.findBackendServicePort(backendSvc, namespace)
	if err != nil {
		return "", 0, "", "", err
	}

	protocol, err := getPortAnnotatedProtocol(d.log, service, servicePort.Name)
	if err != nil {
		return "", 0, "", "", err
	}

	appProtocol, err := getPortAppProtocol(service, servicePort)
	if err != nil {
		return "", 0, "", "", err
	}

	return string(service.UID), servicePort.Port, protocol, appProtocol, nil
}

func (d *Driver) getTunnelBackendFromGateway(backendRef gatewayv1.BackendRef, namespace string) (string, int32, string, string, error) {
	service, servicePort, err := d.findBackendRefServicePort(backendRef, namespace)
	if err != nil {
		return "", 0, "", "", err
	}

	protocol, err := getPortAnnotatedProtocol(d.log, service, servicePort.Name)
	if err != nil {
		return "", 0, "", "", err
	}

	appProtocol, err := getPortAppProtocol(service, servicePort)
	if err != nil {
		return "", 0, "", "", err
	}

	return string(service.UID), servicePort.Port, protocol, appProtocol, nil
}

func (d *Driver) tunnelLabels(serviceName string, port int32) map[string]string {
	return map[string]string{
		labelControllerNamespace: d.managerName.Namespace,
		labelControllerName:      d.managerName.Name,
		labelService:             serviceName,
		labelPort:                strconv.Itoa(int(port)),
	}
}
