package store

import (
	"context"
	"reflect"

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

func (d *Driver) calculateDomains() ([]ingressv1alpha1.Domain, []ingressv1alpha1.Domain, map[string]ingressv1alpha1.Domain) {
	var domains, ingressDomains []ingressv1alpha1.Domain
	ingressDomainMap := d.calculateDomainsFromIngress()

	ingressDomains = make([]ingressv1alpha1.Domain, 0, len(ingressDomainMap))
	for _, domain := range ingressDomainMap {
		ingressDomains = append(ingressDomains, domain)
		domains = append(domains, domain)
	}

	var gatewayDomainMap map[string]ingressv1alpha1.Domain
	if d.gatewayEnabled {
		gatewayDomainMap = d.calculateDomainsFromGateway(ingressDomainMap)
		for _, domain := range gatewayDomainMap {
			domains = append(domains, domain)
		}
	}

	return domains, ingressDomains, gatewayDomainMap
}

func (d *Driver) calculateDomainsFromIngress() map[string]ingressv1alpha1.Domain {
	domainMap := make(map[string]ingressv1alpha1.Domain)

	ingresses := d.store.ListNgrokIngressesV1()
	for _, ingress := range ingresses {
		for _, rule := range ingress.Spec.Rules {
			if rule.Host == "" {
				continue
			}

			domain := ingressv1alpha1.Domain{
				ObjectMeta: metav1.ObjectMeta{
					Name:      ingressv1alpha1.HyphenatedDomainNameFromURL(rule.Host),
					Namespace: ingress.Namespace,
				},
				Spec: ingressv1alpha1.DomainSpec{
					Domain: rule.Host,
				},
			}
			domain.Spec.Metadata = d.ingressNgrokMetadata
			domainMap[rule.Host] = domain
		}
	}

	return domainMap
}

func (d *Driver) calculateDomainsFromGateway(ingressDomains map[string]ingressv1alpha1.Domain) map[string]ingressv1alpha1.Domain {
	domainMap := make(map[string]ingressv1alpha1.Domain)

	gateways := d.store.ListGateways()
	for _, gw := range gateways {
		for _, listener := range gw.Spec.Listeners {
			if listener.Hostname == nil {
				continue
			}
			domainName := string(*listener.Hostname)
			if _, hasVal := ingressDomains[domainName]; hasVal {
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
			domainMap[domainName] = domain
		}
	}

	return domainMap
}
