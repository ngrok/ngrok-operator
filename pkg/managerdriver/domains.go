package managerdriver

import (
	"context"
	"fmt"
	"reflect"

	"github.com/go-logr/logr"
	ingressv1alpha1 "github.com/ngrok/ngrok-operator/api/ingress/v1alpha1"
	"github.com/ngrok/ngrok-operator/internal/annotations"
	netv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"
)

// ingressToDomains constructs domains for edges/endpoints from an input ingress
func ingressToDomains(log logr.Logger, in *netv1.Ingress, newDomainMetadata string, existingDomains map[string]ingressv1alpha1.Domain) (edgeDomains map[string]ingressv1alpha1.Domain, endpointDomains map[string]ingressv1alpha1.Domain) {
	edgeDomains = make(map[string]ingressv1alpha1.Domain)
	endpointDomains = make(map[string]ingressv1alpha1.Domain)

	for _, rule := range in.Spec.Rules {
		domainName := rule.Host
		if domainName == "" {
			continue
		}
		if _, found := existingDomains[domainName]; found {
			// TODO update ingress status
			// also add error to error page
			continue
		}

		domain := ingressv1alpha1.Domain{
			ObjectMeta: metav1.ObjectMeta{
				Name:      ingressv1alpha1.HyphenatedDomainNameFromURL(domainName),
				Namespace: in.Namespace,
			},
			Spec: ingressv1alpha1.DomainSpec{
				Domain: domainName,
			},
		}
		domain.Spec.Metadata = newDomainMetadata

		// Check the annotation to see if an edge or endpoint is desired from this ingress resource
		useEdges, err := annotations.ExtractUseEdges(in)
		if err != nil {
			log.Error(err, fmt.Sprintf("failed to check %q annotation. defaulting to using endpoints", annotations.MappingStrategyAnnotation))
		}
		if !useEdges {
			endpointDomains[domainName] = domain
		} else {
			edgeDomains[domainName] = domain
		}
	}
	return edgeDomains, endpointDomains
}

// gatewayToDomains constructs domains for edges/endpoints from an input Gateway
func gatewayToDomains(log logr.Logger, in *gatewayv1.Gateway, newDomainMetadata string, existingDomains map[string]ingressv1alpha1.Domain) (edgeDomains map[string]ingressv1alpha1.Domain, endpointDomains map[string]ingressv1alpha1.Domain) {
	edgeDomains = make(map[string]ingressv1alpha1.Domain)
	endpointDomains = make(map[string]ingressv1alpha1.Domain)
	for _, listener := range in.Spec.Listeners {
		if listener.Hostname == nil {
			continue
		}

		domainName := string(*listener.Hostname)
		if _, found := existingDomains[domainName]; found {
			// TODO update gateway status
			// also add error to error page
			continue
		}
		if domainName == "" {
			continue
		}

		domain := ingressv1alpha1.Domain{
			ObjectMeta: metav1.ObjectMeta{
				Name:      ingressv1alpha1.HyphenatedDomainNameFromURL(domainName),
				Namespace: in.Namespace,
			},
			Spec: ingressv1alpha1.DomainSpec{
				Domain: domainName,
			},
		}
		domain.Spec.Metadata = newDomainMetadata

		// Check the annotation to see if an edge or endpoint is desired from this ingress resource
		useEdges, err := annotations.ExtractUseEdges(in)
		if err != nil {
			log.Error(err, fmt.Sprintf("failed to check %q annotation. defaulting to using endpoints", annotations.MappingStrategyAnnotation))
		}
		if !useEdges {
			endpointDomains[domainName] = domain
		} else {
			edgeDomains[domainName] = domain
		}
	}
	return edgeDomains, endpointDomains
}

// applyDomains takes a set of the desired domains and current domains, creates any missing desired domains, and updated existing domains if needed
func (d *Driver) applyDomains(ctx context.Context, c client.Client, desiredDomains map[string]ingressv1alpha1.Domain, currentDomains []ingressv1alpha1.Domain) error {
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
	// The following two domain maps track domains for ingress/gateway resources that have opted to
	// use endpoints
	endpointIngressDomains map[string]ingressv1alpha1.Domain
	endpointGatewayDomains map[string]ingressv1alpha1.Domain

	// The following two domain maps track domains for ingress/gateway resources that have opted to
	// use edges
	edgeIngressDomains map[string]ingressv1alpha1.Domain
	edgeGatewayDomains map[string]ingressv1alpha1.Domain

	// totalDomains tracks all domains regardless of source
	totalDomains map[string]ingressv1alpha1.Domain
}

func (d *Driver) calculateDomainSet() *domainSet {
	ret := &domainSet{
		endpointIngressDomains: make(map[string]ingressv1alpha1.Domain),
		endpointGatewayDomains: make(map[string]ingressv1alpha1.Domain),
		edgeIngressDomains:     make(map[string]ingressv1alpha1.Domain),
		edgeGatewayDomains:     make(map[string]ingressv1alpha1.Domain),
		totalDomains:           make(map[string]ingressv1alpha1.Domain),
	}

	// Calculate domains from ingress resources
	ingresses := d.store.ListNgrokIngressesV1()
	for _, ingress := range ingresses {
		edgeDomains, endpointDomains := ingressToDomains(d.log, ingress, d.ingressNgrokMetadata, nil)
		for key, val := range edgeDomains {
			ret.totalDomains[key] = val
			ret.edgeIngressDomains[key] = val
		}
		for key, val := range endpointDomains {
			ret.totalDomains[key] = val
			ret.endpointIngressDomains[key] = val
		}
	}

	// Calculate domains from gateway resources
	gateways := d.store.ListGateways()
	for _, gateway := range gateways {
		edgeDomains, endpointDomains := gatewayToDomains(d.log, gateway, d.gatewayNgrokMetadata, ret.totalDomains)
		for key, val := range edgeDomains {
			ret.totalDomains[key] = val
			ret.edgeGatewayDomains[key] = val
		}
		for key, val := range endpointDomains {
			ret.totalDomains[key] = val
			ret.endpointGatewayDomains[key] = val
		}
	}
	return ret
}
