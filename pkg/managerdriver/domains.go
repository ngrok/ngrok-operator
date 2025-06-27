package managerdriver

import (
	"context"

	ingressv1alpha1 "github.com/ngrok/ngrok-operator/api/ingress/v1alpha1"
	"golang.org/x/sync/errgroup"
	netv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"
)

// ingressToDomains constructs domains for edges/endpoints from an input ingress
func ingressToDomains(in *netv1.Ingress, newDomainMetadata string, existingDomains map[string]ingressv1alpha1.Domain) (endpointDomains map[string]ingressv1alpha1.Domain) {
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
		endpointDomains[domainName] = domain
	}
	return endpointDomains
}

// gatewayToDomains constructs domains for edges/endpoints from an input Gateway
func gatewayToDomains(in *gatewayv1.Gateway, newDomainMetadata string, existingDomains map[string]ingressv1alpha1.Domain) (endpointDomains map[string]ingressv1alpha1.Domain) {
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

		endpointDomains[domainName] = domain

	}
	return endpointDomains
}

// applyDomains takes a set of the desired domains and current domains, creates any missing desired domains, and updated existing domains if needed
func (d *Driver) applyDomains(ctx context.Context, c client.Client, desiredDomains map[string]ingressv1alpha1.Domain) error {
	var g errgroup.Group

	for _, desiredDomain := range desiredDomains {
		g.Go(func() error {
			domain := &ingressv1alpha1.Domain{
				ObjectMeta: metav1.ObjectMeta{
					Name:      desiredDomain.Name,
					Namespace: desiredDomain.Namespace,
				},
			}

			res, err := controllerutil.CreateOrPatch(ctx, c, domain, func() error {
				domain.Spec.Domain = desiredDomain.Spec.Domain
				// Only set the reclaim policy on create
				if domain.CreationTimestamp.IsZero() && d.defaultDomainReclaimPolicy != nil {
					domain.Spec.ReclaimPolicy = *d.defaultDomainReclaimPolicy
				}
				return nil
			})

			log := d.log.WithValues("domain", domain.Name, "namespace", domain.Namespace, "result", res)

			if err != nil {
				log.Error(err, "error creating or patching domain")
			} else {
				log.V(3).Info("create or patched domain")
			}

			return err
		})
	}

	return g.Wait()
}

// Domain set is a helper data type to encapsulate all of the domains and what sources they are from
// The key for the domain maps is "name.namespace" of the associated ingress/gateway
type domainSet struct {
	// The following two domain maps track domains for ingress/gateway resources that have opted to
	// use endpoints
	endpointIngressDomains map[string]ingressv1alpha1.Domain
	endpointGatewayDomains map[string]ingressv1alpha1.Domain

	// totalDomains tracks all domains regardless of source
	totalDomains map[string]ingressv1alpha1.Domain
}

func (d *Driver) calculateDomainSet() *domainSet {
	ret := &domainSet{
		endpointIngressDomains: make(map[string]ingressv1alpha1.Domain),
		endpointGatewayDomains: make(map[string]ingressv1alpha1.Domain),
		totalDomains:           make(map[string]ingressv1alpha1.Domain),
	}

	// Calculate domains from ingress resources
	ingresses := d.store.ListNgrokIngressesV1()
	for _, ingress := range ingresses {
		endpointDomains := ingressToDomains(ingress, d.ingressNgrokMetadata, nil)
		for key, val := range endpointDomains {
			ret.totalDomains[key] = val
			ret.endpointIngressDomains[key] = val
		}
	}

	// Calculate domains from gateway resources
	gateways := d.store.ListGateways()
	for _, gateway := range gateways {
		endpointDomains := gatewayToDomains(gateway, d.gatewayNgrokMetadata, ret.totalDomains)
		for key, val := range endpointDomains {
			ret.totalDomains[key] = val
			ret.endpointGatewayDomains[key] = val
		}
	}
	return ret
}
