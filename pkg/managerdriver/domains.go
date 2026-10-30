package managerdriver

import (
	"context"
	"errors"

	ngrokv1 "github.com/ngrok/ngrok-operator/api/ngrok/v1"
	domainpkg "github.com/ngrok/ngrok-operator/internal/domain"
	"github.com/ngrok/ngrok-operator/internal/util"
	"golang.org/x/sync/errgroup"
	netv1 "k8s.io/api/networking/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"
)

// ingressToDomains constructs domains for edges/endpoints from an input ingress
func ingressToDomains(in *netv1.Ingress, existingDomains map[string]ngrokv1.Domain) (endpointDomains map[string]ngrokv1.Domain) {
	endpointDomains = make(map[string]ngrokv1.Domain)

	for _, rule := range in.Spec.Rules {
		domainName := rule.Host
		if domainName == "" {
			continue
		}
		// Skip internal domains - they cannot be reserved via the ngrok API
		if util.IsInternalDomain(domainName) {
			continue
		}
		if _, found := existingDomains[domainName]; found {
			// TODO update ingress status
			// also add error to error page
			continue
		}

		domain := ngrokv1.Domain{
			Name:      ngrokv1.HyphenatedDomainNameFromURL(domainName),
			Namespace: in.Namespace,
			Spec: ngrokv1.DomainSpec{
				Domain: domainName,
			},
		}
		endpointDomains[domainName] = domain
	}
	return endpointDomains
}

// gatewayToDomains constructs domains for edges/endpoints from an input Gateway
func gatewayToDomains(in *gatewayv1.Gateway, existingDomains map[string]ngrokv1.Domain) (endpointDomains map[string]ngrokv1.Domain) {
	endpointDomains = make(map[string]ngrokv1.Domain)
	for _, listener := range in.Spec.Listeners {
		if listener.Hostname == nil {
			continue
		}

		domainName := string(*listener.Hostname)
		if domainName == "" {
			continue
		}
		// Skip internal domains - they cannot be reserved via the ngrok API
		if util.IsInternalDomain(domainName) {
			continue
		}
		if _, found := existingDomains[domainName]; found {
			// TODO update gateway status
			// also add error to error page
			continue
		}

		domain := ngrokv1.Domain{
			Name:      ngrokv1.HyphenatedDomainNameFromURL(domainName),
			Namespace: in.Namespace,
			Spec: ngrokv1.DomainSpec{
				Domain: domainName,
			},
		}

		endpointDomains[domainName] = domain

	}
	return endpointDomains
}

// applyDomains takes a set of the desired domains and current domains, creates any missing desired domains, and updated existing domains if needed
func (d *Driver) applyDomains(ctx context.Context, c client.Client, desiredDomains map[string]ngrokv1.Domain) error {
	var g errgroup.Group

	for _, desiredDomain := range desiredDomains {
		g.Go(func() error {
			domain := &ngrokv1.Domain{
				Name:      desiredDomain.Name,
				Namespace: desiredDomain.Namespace,
			}

			// LEGACY-domain-kind: BEGIN
			// A legacy Domain at this name must be copied to v1 as-is rather
			// than replaced by defaults: the two share one reservation and the
			// v1 object becomes the one that pushes its spec to it. CreateOrPatch
			// can't do that (it knows nothing about the legacy kind), so EnsureV1
			// creates the object first, with the same defaults CreateOrPatch would
			// set; CreateOrPatch then only patches it.
			_, _, err := domainpkg.EnsureV1(ctx, c, d.apiReader, client.ObjectKeyFromObject(domain), func(n *ngrokv1.Domain) {
				n.Spec.Domain = desiredDomain.Spec.Domain
				if d.defaultDomainReclaimPolicy != nil {
					n.Spec.ReclaimPolicy = *d.defaultDomainReclaimPolicy
				}
				d.controllerLabels.EnsureLabels(n)
			})
			if errors.Is(err, domainpkg.ErrMigrationBlocked) {
				// The legacy Domain keeps serving this hostname until it is
				// re-created as ngrok.com/v1 by hand.
				d.log.Info("Not migrating legacy Domain to ngrok.com/v1", "reason", err.Error())
				return nil
			}
			if err != nil {
				d.log.Error(err, "error creating domain", "domain", domain.Name, "namespace", domain.Namespace)
				// Same reasoning as the IsInvalid case below.
				if apierrors.IsInvalid(err) {
					return nil
				}
				return err
			}
			// LEGACY-domain-kind: END

			res, err := controllerutil.CreateOrPatch(ctx, c, domain, func() error {
				domain.Spec.Domain = desiredDomain.Spec.Domain
				// Only set the reclaim policy on create
				if domain.CreationTimestamp.IsZero() && d.defaultDomainReclaimPolicy != nil {
					domain.Spec.ReclaimPolicy = *d.defaultDomainReclaimPolicy
				}
				// Set controller labels inside the mutate so the call covers both
				// create and patch: CreateOrPatch's Get overwrites anything set on
				// the object beforehand, so an ObjectMeta initializer would only
				// survive on create and existing Domains would never get their
				// label pairs backfilled. Keeps this path consistent with
				// internal/domain/manager.go::ensureControllerLabels.
				// LEGACY-PREFIX-MIGRATION: EnsureLabels dual-writes the legacy pair.
				d.controllerLabels.EnsureLabels(domain)
				return nil
			})

			log := d.log.WithValues("domain", domain.Name, "namespace", domain.Namespace, "result", res)

			if err != nil {
				log.Error(err, "error creating or patching domain")

				// An Invalid rejection is the API server refusing this specific
				// object, so retrying the same patch will never succeed. The
				// reachable case is spec.domain: it is immutable, and a Domain that
				// drifted from its desired hostname before that validation shipped
				// makes the rewrite above a rename, which admission rejects.
				//
				// Failing the errgroup here would return early from Sync and skip
				// applyAgentEndpoints, applyCloudEndpoints, and updateStatuses, so
				// one unfixable Domain would wedge reconciliation for every Ingress
				// and Gateway in the cluster. Log it and let the rest of the sync
				// proceed; every other error class stays fatal and retryable.
				if apierrors.IsInvalid(err) {
					return nil
				}
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
	endpointIngressDomains map[string]ngrokv1.Domain
	endpointGatewayDomains map[string]ngrokv1.Domain

	// totalDomains tracks all domains regardless of source
	totalDomains map[string]ngrokv1.Domain
}

func (d *Driver) calculateDomainSet() *domainSet {
	ret := &domainSet{
		endpointIngressDomains: make(map[string]ngrokv1.Domain),
		endpointGatewayDomains: make(map[string]ngrokv1.Domain),
		totalDomains:           make(map[string]ngrokv1.Domain),
	}

	// Calculate domains from ingress resources
	ingresses := d.store.ListNgrokIngressesV1()
	for _, ingress := range ingresses {
		endpointDomains := ingressToDomains(ingress, nil)
		for key, val := range endpointDomains {
			ret.totalDomains[key] = val
			ret.endpointIngressDomains[key] = val
		}
	}

	// Calculate domains from gateway resources
	gateways := d.store.ListNgrokGateways()
	for _, gateway := range gateways {
		endpointDomains := gatewayToDomains(gateway, ret.totalDomains)
		for key, val := range endpointDomains {
			ret.totalDomains[key] = val
			ret.endpointGatewayDomains[key] = val
		}
	}
	return ret
}
