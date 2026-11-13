package managerdriver

import (
	"context"
	"errors"
	"maps"

	ngrokv1 "github.com/ngrok/ngrok-operator/api/ngrok/v1"
	domainpkg "github.com/ngrok/ngrok-operator/internal/domain"
	"github.com/ngrok/ngrok-operator/internal/util"
	"golang.org/x/sync/errgroup"
	netv1 "k8s.io/api/networking/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
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

// gatewayToDomains constructs domains for edges/endpoints from an input Gateway: one for each listener
// hostname, the domain ngrok assigned the Gateway when a listener has no hostname, and endpointHostnames, the
// hostnames translation gave the Gateway's endpoints (which can come from its routes).
func gatewayToDomains(in *gatewayv1.Gateway, existingDomains map[string]ngrokv1.Domain, h GatewayHostnames, endpointHostnames []string) (endpointDomains map[string]ngrokv1.Domain) {
	endpointDomains = make(map[string]ngrokv1.Domain)

	add := func(domainName string) {
		if domainName == "" {
			return
		}
		// Skip internal domains - they cannot be reserved via the ngrok API
		if util.IsInternalDomain(domainName) {
			return
		}
		if _, found := existingDomains[domainName]; found {
			// TODO update gateway status
			// also add error to error page
			return
		}
		endpointDomains[domainName] = ngrokv1.Domain{
			Name:      ngrokv1.HyphenatedDomainNameFromURL(domainName),
			Namespace: in.Namespace,
			Spec: ngrokv1.DomainSpec{
				Domain: domainName,
			},
		}
	}

	for _, listener := range in.Spec.Listeners {
		if hostname := h.Listener(listener); hostname != "" {
			add(hostname)
			continue
		}
		if len(in.Spec.Addresses) == 0 && listenerUsesAssignedDomain(listener) {
			add(h.Assigned[in.UID])
		}
	}
	for _, hostname := range endpointHostnames {
		add(hostname)
	}
	// The label is what keeps the assigned domain the Gateway's (see gateway_address.go).
	if assigned := h.Assigned[in.UID]; assigned != "" {
		if dom, ok := endpointDomains[assigned]; ok {
			dom.Labels = map[string]string{GatewayUIDLabel: string(in.UID)}
			endpointDomains[assigned] = dom
		}
	}
	return endpointDomains
}

// listenerUsesAssignedDomain reports whether a listener without a hostname is served on the domain
// ngrok assigned the Gateway. TCP and UDP endpoints are not addressed by hostname.
func listenerUsesAssignedDomain(l gatewayv1.Listener) bool {
	switch l.Protocol {
	case gatewayv1.HTTPProtocolType, gatewayv1.HTTPSProtocolType, gatewayv1.TLSProtocolType:
		return true
	}
	return false
}

// applyDomains takes a set of the desired domains and current domains, creates any missing desired domains, and updated existing domains if needed
func (d *Driver) applyDomains(ctx context.Context, c client.Client, desiredDomains map[string]ngrokv1.Domain) error {
	var g errgroup.Group

	for _, desiredDomain := range desiredDomains {
		g.Go(func() error {
			// EnsureV1 returns the Domain, creating it if needed. When a legacy
			// Domain exists at this name, the new object is a copy of it rather
			// than defaults: the two share one reservation, and the v1 object
			// becomes the one that pushes its spec to it.
			//
			// The returned object is patched below without another Get. A
			// CreateOrPatch here would re-read through the cache, which may not
			// have seen an object EnsureV1 just created; it would then try to
			// create it again, fail with AlreadyExists, and end the Sync early.
			key := client.ObjectKey{Namespace: desiredDomain.Namespace, Name: desiredDomain.Name}
			domain, created, err := domainpkg.EnsureV1(ctx, c, d.apiReader, key, d.controllerLabels.HasLabels, func(n *ngrokv1.Domain) {
				// Runs only when creating from defaults, so the default reclaim
				// policy is never forced onto an existing Domain.
				n.Spec.Domain = desiredDomain.Spec.Domain
				if d.defaultDomainReclaimPolicy != nil {
					n.Spec.ReclaimPolicy = *d.defaultDomainReclaimPolicy
				}
				d.controllerLabels.EnsureLabels(n)
				maps.Copy(n.Labels, desiredDomain.Labels)
			})
			// LEGACY-domain-kind: BEGIN
			if errors.Is(err, domainpkg.ErrLegacyDomainNotOwned) {
				// A user's legacy Domain: they re-stamp it, and it keeps serving
				// this hostname until then. Leave it alone.
				return nil
			}
			if errors.Is(err, domainpkg.ErrMigrationBlocked) {
				// The legacy Domain keeps serving this hostname until it is
				// re-created as ngrok.com/v1 by hand.
				d.log.Info("Not migrating legacy Domain to ngrok.com/v1", "reason", err.Error())
				return nil
			}
			// LEGACY-domain-kind: END
			if err != nil {
				d.log.Error(err, "error creating domain", "domain", key.Name, "namespace", key.Namespace)
				// Same reasoning as the IsInvalid case below.
				if apierrors.IsInvalid(err) {
					return nil
				}
				return err
			}

			res := controllerutil.OperationResultNone
			if created {
				res = controllerutil.OperationResultCreated
			}
			before := domain.DeepCopy()
			domain.Spec.Domain = desiredDomain.Spec.Domain
			// Backfill the controller labels on existing Domains too: one stamped
			// by an older operator version may carry only the legacy pair.
			// Keeps this path consistent with
			// internal/domain/manager.go::ensureControllerLabels.
			// LEGACY-PREFIX-MIGRATION: EnsureLabels dual-writes the legacy pair.
			d.controllerLabels.EnsureLabels(domain)
			maps.Copy(domain.Labels, desiredDomain.Labels)
			if !equality.Semantic.DeepEqual(before, domain) {
				err = c.Patch(ctx, domain, client.MergeFrom(before))
				if err == nil && !created {
					res = controllerutil.OperationResultUpdated
				}
			}

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

// calculateDomainSet works out every Domain the Ingresses and Gateways need. gatewayHostnames are the
// endpoint hostnames translation gave each Gateway (TranslationResult.GatewayHostnames).
func (d *Driver) calculateDomainSet(h GatewayHostnames, gatewayHostnames map[types.NamespacedName][]string) *domainSet {
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
		endpointDomains := gatewayToDomains(gateway, ret.totalDomains, h, gatewayHostnames[client.ObjectKeyFromObject(gateway)])
		for key, val := range endpointDomains {
			ret.totalDomains[key] = val
			ret.endpointGatewayDomains[key] = val
		}
	}
	return ret
}
