package managerdriver

import (
	"context"
	"encoding/json"
	"maps"
	"net/url"
	"sort"

	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"

	ngrokv1 "github.com/ngrok/ngrok-operator/api/ngrok/v1"
)

// A Gateway listener without a hostname still needs one, because ngrok routes
// by hostname. Rather than make a name up, which could collide with one
// someone else holds, the operator lets ngrok pick it, the way `ngrok http 80`
// does:
//
//  1. While the Gateway has no domain, translation adds a claim: an
//     AgentEndpoint with url "https://" that answers every request with a 503.
//     ngrok gives it a random domain, which the agent writes to
//     status.assignedURL.
//  2. The next sync reads that domain and pins it: it creates a Domain for it,
//     labeled with the Gateway's UID, and stops adding the claim. ngrok won't
//     reserve a domain an endpoint is using, so the reservation goes through
//     once the claim is gone.
//  3. From then on the labeled Domain is the Gateway's domain. Its endpoints
//     serve on it and status reports it as the Gateway's address. It survives
//     route changes and operator restarts. A re-created Gateway has a new UID
//     and gets a new domain, the way a re-created cloud load balancer gets a
//     new address.

const (
	// GatewayUIDLabel marks the Domain holding a Gateway's assigned domain,
	// and the claim that asks ngrok for one.
	GatewayUIDLabel = "ngrok.com/gateway-uid"
	// GatewayNameLabel names the Gateway a claim belongs to, so a change to
	// the claim reaches that Gateway's reconcile.
	GatewayNameLabel = "ngrok.com/gateway-name"
)

// gatewayNeedsAssignedDomain reports whether a Gateway has a listener served
// on its assigned domain: an HTTP, HTTPS or TLS listener without a hostname,
// on a Gateway without spec.addresses.
func gatewayNeedsAssignedDomain(gw *gatewayv1.Gateway) bool {
	if len(gw.Spec.Addresses) > 0 {
		return false
	}
	for _, l := range gw.Spec.Listeners {
		if (l.Hostname == nil || *l.Hostname == "") && listenerUsesAssignedDomain(l) {
			return true
		}
	}
	return false
}

var addressClaimPolicy = json.RawMessage(`{"on_http_request":[{"actions":[{"type":"custom-response","config":{"status_code":503,"body":"This Gateway's address is being assigned."}}]}]}`)

// buildAddressClaim returns the AgentEndpoint that gets a Gateway its domain
// from ngrok.
func buildAddressClaim(gw *gatewayv1.Gateway, labels map[string]string) *ngrokv1.AgentEndpoint {
	l := maps.Clone(labels)
	if l == nil {
		l = map[string]string{}
	}
	l[GatewayUIDLabel] = string(gw.UID)
	l[GatewayNameLabel] = gw.Name
	return &ngrokv1.AgentEndpoint{
		Name:      sanitizeStringForK8sName(gw.Name + "-address-claim"),
		Namespace: gw.Namespace,
		Labels:    l,
		Spec: ngrokv1.AgentEndpointSpec{
			URL:         "https://",
			Description: "Gets a domain from ngrok for Gateway " + gw.Namespace + "/" + gw.Name,
			// The policy answers every request, so the upstream is never dialed.
			Upstream:      ngrokv1.EndpointUpstream{URL: "http://localhost:1"},
			TrafficPolicy: &ngrokv1.TrafficPolicyCfg{Inline: addressClaimPolicy},
		},
	}
}

// assignedGatewayDomains returns the domain ngrok assigned each Gateway, by
// Gateway UID: the domain of the Domain labeled with the UID, or, before that
// Domain exists, the domain the Gateway's claim was given.
//
// The Domains are read from the API server, not the cache. A sync that runs
// just after the one that created the Domain and removed the claim must see
// the Domain, or it would add a claim again and the Gateway would get a
// second domain.
func (d *Driver) assignedGatewayDomains(ctx context.Context, c client.Client) (map[types.UID]string, error) {
	reader := client.Reader(c)
	if d.apiReader != nil {
		reader = d.apiReader
	}
	domains := &ngrokv1.DomainList{}
	if err := reader.List(ctx, domains, client.HasLabels{GatewayUIDLabel}); err != nil {
		return nil, err
	}
	claims := &ngrokv1.AgentEndpointList{}
	if err := c.List(ctx, claims, client.HasLabels{GatewayUIDLabel}); err != nil {
		return nil, err
	}
	return assignedDomains(domains.Items, claims.Items), nil
}

// assignedDomains works out each Gateway's domain from the labeled Domains
// and claims. A Domain wins over a claim. When a Gateway has more than one
// Domain, the oldest wins, so the choice doesn't change between syncs.
func assignedDomains(domains []ngrokv1.Domain, claims []ngrokv1.AgentEndpoint) map[types.UID]string {
	sort.SliceStable(domains, func(i, j int) bool {
		a, b := domains[i].CreationTimestamp, domains[j].CreationTimestamp
		if !a.Equal(&b) {
			return a.Before(&b)
		}
		return domains[i].Spec.Domain < domains[j].Spec.Domain
	})
	assigned := map[types.UID]string{}
	for _, dom := range domains {
		uid := types.UID(dom.Labels[GatewayUIDLabel])
		if _, ok := assigned[uid]; ok || dom.Spec.Domain == "" {
			continue
		}
		assigned[uid] = dom.Spec.Domain
	}
	for _, claim := range claims {
		uid := types.UID(claim.Labels[GatewayUIDLabel])
		if _, ok := assigned[uid]; ok {
			continue
		}
		u, err := url.Parse(claim.Status.AssignedURL)
		if err != nil || u.Hostname() == "" {
			continue
		}
		assigned[uid] = u.Hostname()
	}
	return assigned
}
