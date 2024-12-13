package managerdriver

import (
	"context"
	"reflect"
	"strings"

	ingressv1alpha1 "github.com/ngrok/ngrok-operator/api/ingress/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

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

// Domain set is a helper data type to encapsulate all of the domains and what sources they are from
// The key for the domain maps is "name.namespace" of the associated ingress/gateway
type domainSet struct {
	// The following two domain maps track domains for ingress/gateway resources that contain the
	// `ngrok.k8s.io/use-endpoints: "true"` annotation. This causes them to be backed by endpoints instead of edges
	endpointIngressDomains map[string]ingressv1alpha1.Domain
	endpointGatewayDomains map[string]ingressv1alpha1.Domain

	// The following two domain maps track domains for ingress/gateway resources that do not contian the
	// `ngrok.k8s.io/use-endpoints: "true"` annotation. Without this annotation, they are backed by edges (the default behaviour)
	edgeIngressDomains map[string]ingressv1alpha1.Domain
	edgeGatewayDomains map[string]ingressv1alpha1.Domain

	// totalDomains tracks all domains regardless of source
	totalDomains []ingressv1alpha1.Domain
}

func (d *Driver) calculateDomainSet() *domainSet {
	ret := &domainSet{
		endpointIngressDomains: make(map[string]ingressv1alpha1.Domain),
		endpointGatewayDomains: make(map[string]ingressv1alpha1.Domain),
		edgeIngressDomains:     make(map[string]ingressv1alpha1.Domain),
		edgeGatewayDomains:     make(map[string]ingressv1alpha1.Domain),
		totalDomains:           []ingressv1alpha1.Domain{},
	}

	hostnamesOnIngresses := map[string]bool{} // keep track of the hostnames used on ingresses. If the same hostname is used on an ingress and a gateway, it is an error

	// Calculate domains from ingress resources
	ingresses := d.store.ListNgrokIngressesV1()
	for _, ingress := range ingresses {
		for _, rule := range ingress.Spec.Rules {
			domainName := rule.Host
			if domainName == "" {
				continue
			}

			domain := ingressv1alpha1.Domain{
				ObjectMeta: metav1.ObjectMeta{
					Name:      ingressv1alpha1.HyphenatedDomainNameFromURL(rule.Host),
					Namespace: ingress.Namespace,
				},
				Spec: ingressv1alpha1.DomainSpec{
					Domain: domainName,
				},
			}
			domain.Spec.Metadata = d.ingressNgrokMetadata

			// Check the annotation to see if an edge or endpoint is desired from this ingress resource

			hostnamesOnIngresses[domainName] = true
			if val, found := ingress.Annotations[annotationUseEndpoint]; found && strings.ToLower(val) == "true" {
				ret.endpointIngressDomains[domainName] = domain
			} else {
				ret.edgeIngressDomains[domainName] = domain
			}
			ret.totalDomains = append(ret.totalDomains, domain)
		}
	}

	// Calculate domains from gateway resources
	gateways := d.store.ListGateways()
	for _, gw := range gateways {
		for _, listener := range gw.Spec.Listeners {
			if listener.Hostname == nil {
				continue
			}
			domainName := string(*listener.Hostname)
			if _, found := hostnamesOnIngresses[domainName]; found {
				// TODO update gateway status
				// also add error to error page
				continue
			}
			domain := ingressv1alpha1.Domain{
				ObjectMeta: metav1.ObjectMeta{
					Name:      ingressv1alpha1.HyphenatedDomainNameFromURL(domainName),
					Namespace: gw.Namespace,
				},
				Spec: ingressv1alpha1.DomainSpec{
					Domain: domainName,
				},
			}
			domain.Spec.Metadata = d.gatewayNgrokMetadata

			// Check the annotation to see if an edge or endpoint is desired from this ingress resource
			if val, found := gw.Annotations[annotationUseEndpoint]; found && strings.ToLower(val) == "true" {
				ret.endpointIngressDomains[domainName] = domain
			} else {
				ret.edgeIngressDomains[domainName] = domain
			}
			ret.totalDomains = append(ret.totalDomains, domain)
		}
	}
	return ret
}
