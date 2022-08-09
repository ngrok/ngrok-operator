package ngrokapidriver

import (
	"context"
	"strings"

	"github.com/ngrok/ngrok-api-go/v4"
	tgb "github.com/ngrok/ngrok-api-go/v4/backends/tunnel_group"
	edge "github.com/ngrok/ngrok-api-go/v4/edges/https"
	edge_route "github.com/ngrok/ngrok-api-go/v4/edges/https_routes"
	"github.com/ngrok/ngrok-api-go/v4/reserved_domains"
)

type NgrokAPIDriver interface {
	FindEdge(ctx context.Context, id string) (*ngrok.HTTPSEdge, error)
	CreateEdge(ctx context.Context, n Edge) (*ngrok.HTTPSEdge, error)
	UpdateEdge(ctx context.Context, n Edge) (*ngrok.HTTPSEdge, error)
	DeleteEdge(ctx context.Context, e Edge) error
}

type ngrokAPIDriver struct {
	edges           edge.Client
	tgbs            tgb.Client
	routes          edge_route.Client
	reservedDomains reserved_domains.Client
	metadata        string
}

func NewNgrokApiClient(apiKey string) NgrokAPIDriver {
	config := ngrok.NewClientConfig(apiKey)
	return &ngrokAPIDriver{
		edges:           *edge.NewClient(config),
		tgbs:            *tgb.NewClient(config),
		routes:          *edge_route.NewClient(config),
		reservedDomains: *reserved_domains.NewClient(config),
		metadata:        "\"{\"owned-by\":\"ngrok-ingress-controller\"}\"",
	}
}

func (nc ngrokAPIDriver) FindEdge(ctx context.Context, id string) (*ngrok.HTTPSEdge, error) {
	return nc.edges.Get(ctx, id)
}

// Goes through the whole edge object and creates resources for
// * reserved domains
// * tunnel group backends
// * edge routes
// * the edge itself
func (napi ngrokAPIDriver) CreateEdge(ctx context.Context, edgeSummary Edge) (*ngrok.HTTPSEdge, error) {
	// TODO: Support multiple rules and multiple hostports
	domain := strings.Split(edgeSummary.Hostport, ":")[0]
	_, err := napi.reservedDomains.Create(ctx, &ngrok.ReservedDomainCreate{
		Name:        domain,
		Region:      "us", // TODO: Set this from user config
		Description: "Created by ngrok-ingress-controller",
		Metadata:    napi.metadata,
	})
	// Swallow conflicts, just always try to create it
	// TODO: Depending on if we choose to clean up reserved domains or not, we may want to surface this conflict to the user
	if err != nil && !strings.Contains(err.Error(), "ERR_NGROK_413") && !strings.Contains(err.Error(), "ERR_NGROK_7122") {
		return nil, err
	}

	var newEdge *ngrok.HTTPSEdge

	// If the edge ID is already set, try to look it up
	if edgeSummary.Id != "" {
		newEdge, err = napi.edges.Get(ctx, edgeSummary.Id)
		if ngrok.IsNotFound(err) {
			edgeSummary.Id = ""
		} else if err != nil {
			return nil, err
		}
	} else { // Otherwise Make it
		newEdge, err = napi.edges.Create(ctx, &ngrok.HTTPSEdgeCreate{
			Hostports:   &[]string{edgeSummary.Hostport},
			Description: "Created by ngrok-ingress-controller",
			Metadata:    napi.metadata,
		})
		if err != nil {
			return nil, err
		}
	}

	backend, err := napi.tgbs.Create(ctx, &ngrok.TunnelGroupBackendCreate{
		Labels:      edgeSummary.Labels,
		Description: "Created by ngrok-ingress-controller",
		Metadata:    napi.metadata,
	})
	if err != nil {
		return nil, err
	}

	for _, route := range edgeSummary.Routes {
		_, err := napi.routes.Create(ctx, &ngrok.HTTPSEdgeRouteCreate{
			EdgeID:      newEdge.ID,
			MatchType:   route.MatchType,
			Match:       route.Match,
			Description: "Created by ngrok-ingress-controller",
			Metadata:    napi.metadata,
			Backend: &ngrok.EndpointBackendMutate{
				BackendID: backend.ID,
			},
		})
		if err != nil {
			return nil, err
		}
	}

	return newEdge, nil
}

// TODO: Implement this
func (nc ngrokAPIDriver) UpdateEdge(ctx context.Context, edgeSummary Edge) (*ngrok.HTTPSEdge, error) {
	return nil, nil
}

func (nc ngrokAPIDriver) DeleteEdge(ctx context.Context, e Edge) error {
	edge, err := nc.edges.Get(ctx, e.Id)
	if err != nil {
		return err
	}
	for _, route := range edge.Routes {
		err := nc.routes.Delete(ctx, &ngrok.EdgeRouteItem{EdgeID: e.Id, ID: route.ID})
		if err != nil {
			return err
		}
	}

	// TODO: I could delete the reserved endpoint, but it might make sense to just leave it reserved. Keeping this for now
	err = nc.edges.Delete(ctx, e.Id)
	if err != nil {
		if !ngrok.IsNotFound(err) {
			return err
		} else {
		}
	}
	return nil
}
