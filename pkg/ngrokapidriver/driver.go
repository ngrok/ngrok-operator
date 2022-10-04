package ngrokapidriver

import (
	"context"
	"errors"
	"fmt"
	"net"

	"github.com/ngrok/ngrok-api-go/v4"
	tgb "github.com/ngrok/ngrok-api-go/v4/backends/tunnel_group"
	edge "github.com/ngrok/ngrok-api-go/v4/edges/https"
	edge_route "github.com/ngrok/ngrok-api-go/v4/edges/https_routes"
	"github.com/ngrok/ngrok-api-go/v4/reserved_domains"
	ctrl "sigs.k8s.io/controller-runtime"
)

type NgrokAPIDriver interface {
	FindEdge(ctx context.Context, id string) (*ngrok.HTTPSEdge, error)
	CreateEdge(ctx context.Context, e *Edge) (*ngrok.HTTPSEdge, error)
	UpdateEdge(ctx context.Context, e *Edge) (*ngrok.HTTPSEdge, error)
	DeleteEdge(ctx context.Context, e *Edge) error
	GetReservedDomains(ctx context.Context, edgeID string) ([]ngrok.ReservedDomain, error)
}

type ngrokAPIDriver struct {
	edges           *edge.Client
	tgbs            *tgb.Client
	routes          *edge_route.Client
	reservedDomains *reserved_domains.Client
	metadata        string
}

func NewNgrokApiClient(apiKey string) NgrokAPIDriver {
	config := ngrok.NewClientConfig(apiKey)
	return &ngrokAPIDriver{
		edges:           edge.NewClient(config),
		tgbs:            tgb.NewClient(config),
		routes:          edge_route.NewClient(config),
		reservedDomains: reserved_domains.NewClient(config),
		metadata:        "\"{\"owned-by\":\"ngrok-ingress-controller\"}\"",
	}
}

func (nc ngrokAPIDriver) FindEdge(ctx context.Context, id string) (*ngrok.HTTPSEdge, error) {
	edge, err := nc.edges.Get(ctx, id)
	if err != nil {
		if ngrok.IsNotFound(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to get edge id %s: %w", id, err)
	}
	return edge, nil
}

// Goes through the whole edge object and creates resources for
// * reserved domains
// * tunnel group backends
// * edge routes
// * the edge itself
func (napi ngrokAPIDriver) CreateEdge(ctx context.Context, edgeSummary *Edge) (*ngrok.HTTPSEdge, error) {
	log := ctrl.LoggerFrom(ctx)
	// TODO: Support multiple rules and multiple hostports
	domain, _, err := net.SplitHostPort(edgeSummary.Hostport)
	if err != nil {
		return nil, err
	}
	_, err = napi.reservedDomains.Create(ctx, &ngrok.ReservedDomainCreate{
		Name:        domain,
		Region:      "us", // TODO: Set this from user config
		Description: "Created by ngrok-ingress-controller",
		Metadata:    napi.metadata,
	})
	// Swallow conflicts, just always try to create it and don't delete them upon ingress deletion
	if err != nil {
		var nerr *ngrok.Error
		if errors.As(err, &nerr) {
			switch nerr.ErrorCode {
			case "ERR_NGROK_413", "ERR_NGROK_7122":
				log.Info("Reserved domain already exists, skipping creation", "domain", domain)
			default:
				return nil, err
			}
		}
		return nil, err
	}

	newEdge, err := napi.edges.Create(ctx, &ngrok.HTTPSEdgeCreate{
		Hostports:   &[]string{edgeSummary.Hostport},
		Description: "Created by ngrok-ingress-controller",
		Metadata:    napi.metadata,
	})
	if err != nil {
		return nil, err
	}

	for _, route := range edgeSummary.Routes {
		// Create Tunnel-Group Backend
		backend, err := napi.tgbs.Create(ctx, &ngrok.TunnelGroupBackendCreate{
			Labels:      route.Labels,
			Description: "Created by ngrok-ingress-controller",
			Metadata:    napi.metadata,
		})
		if err != nil {
			return nil, err
		}
		// Create Route
		edgeRouteCreate := ngrok.HTTPSEdgeRouteCreate{
			EdgeID:      newEdge.ID,
			MatchType:   route.MatchType,
			Match:       route.Match,
			Description: "Created by ngrok-ingress-controller",
			Metadata:    napi.metadata,
			Backend: &ngrok.EndpointBackendMutate{
				BackendID: backend.ID,
			},
		}

		// TODO: This is a shortcut and should be replaced
		if route.Compression {
			edgeRouteCreate.Compression = &ngrok.EndpointCompression{}
		}

		if route.GoogleOAuth.ClientID != "" {
			edgeRouteCreate.OAuth = &ngrok.EndpointOAuth{
				Provider: ngrok.EndpointOAuthProvider{
					Google: &ngrok.EndpointOAuthGoogle{
						ClientID:     &route.GoogleOAuth.ClientID,
						ClientSecret: &route.GoogleOAuth.ClientSecret,
						Scopes:       route.GoogleOAuth.Scopes,
						EmailDomains: route.GoogleOAuth.EmailDomains,
					},
				},
			}
		}

		_, err = napi.routes.Create(ctx, &edgeRouteCreate)
		if err != nil {
			return nil, err
		}
	}

	return newEdge, nil
}

// TODO: Implement this
func (nc ngrokAPIDriver) UpdateEdge(ctx context.Context, edgeSummary *Edge) (*ngrok.HTTPSEdge, error) {
	existingEdge, err := nc.FindEdge(ctx, edgeSummary.Id)
	if err != nil {
		return nil, err
	}
	if existingEdge == nil {
		return nil, fmt.Errorf("edge %s does not exist", edgeSummary.Id)
	}

	// For now, we only support 1 hostport so anytime we have more or less than 1, something is different
	hostPortsDifferent := len(*existingEdge.Hostports) != 1 || (*existingEdge.Hostports)[0] != edgeSummary.Hostport
	if hostPortsDifferent {
		err := nc.DeleteEdge(ctx, edgeSummary)
		if err != nil {
			return nil, err
		}
		return nc.CreateEdge(ctx, edgeSummary)
	}

	// If the hostport is the same
	// Loop through the edgeSummary's routes
	// Create a unique key formed from each route's key attributes
	// Create a similar key from the existing edge's routes
	// compare to our list
	// for each one thats not in our list delete
	// for each thats in our list but not remote, create it
	// if it is in our list, then all its attributes match so ignore it
	// TODO: also check for route modules at this point
	return existingEdge, nil
}

// DeleteEdge deletes the edge and routes but doesn't delete reserved domains
func (nc ngrokAPIDriver) DeleteEdge(ctx context.Context, e *Edge) error {
	log := ctrl.LoggerFrom(ctx).WithValues("DeleteEdge", e.Id)
	log.Info("Deleting edge")
	edge, err := nc.FindEdge(ctx, e.Id)
	if err != nil {
		return err
	}
	if edge == nil {
		log.Info("Edge not found, skipping deletion", "edge", e.Id)
		return nil
	}

	for _, route := range edge.Routes {
		if err := nc.tgbs.Delete(ctx, route.Backend.Backend.ID); err != nil && !ngrok.IsNotFound(err) {
			return fmt.Errorf("error deleting backend with id %s: %w", route.Backend.Backend.ID, err)
		}

		if err := nc.routes.Delete(ctx, &ngrok.EdgeRouteItem{EdgeID: e.Id, ID: route.ID}); err != nil && !ngrok.IsNotFound(err) {
			return fmt.Errorf("error deleting route with id %s: %w", route.ID, err)
		}
	}

	if err := nc.edges.Delete(ctx, e.Id); err != nil && !ngrok.IsNotFound(err) {
		return fmt.Errorf("error deleting edge with id %s: %w", e.Id, err)
	}
	return nil
}

func (nc ngrokAPIDriver) GetReservedDomains(ctx context.Context, edgeID string) ([]ngrok.ReservedDomain, error) {
	edge, err := nc.FindEdge(ctx, edgeID)
	if err != nil {
		return nil, err
	}
	hostPortDomains := []string{}
	for _, hostport := range *edge.Hostports {
		domain, _, err := net.SplitHostPort(hostport)
		if err != nil {
			return nil, err
		}
		hostPortDomains = append(hostPortDomains, domain)
	}

	domainsItr := nc.reservedDomains.List(nil)
	var matchingReservedDomains []ngrok.ReservedDomain
	// Loop while there are more domains and check if they match any of the hostPortDomains. If so add it to the reservedDomains
	for domainsItr.Next(ctx) {
		domain := domainsItr.Item()
		for _, hostPortDomain := range hostPortDomains {
			if domain.Domain == hostPortDomain {
				matchingReservedDomains = append(matchingReservedDomains, *domain)
			}
		}
	}

	return matchingReservedDomains, domainsItr.Err()
}
