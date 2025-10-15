package nmockapi

import (
	context "context"
	"fmt"
	"net/http"

	"github.com/ngrok/ngrok-api-go/v7"
)

// EndpointsClient is a mock implementation of the ngrok API client for managing endpoints.
type EndpointsClient struct {
	baseClient[*ngrok.Endpoint]
}

func NewEndpointsClient() *EndpointsClient {
	return &EndpointsClient{
		baseClient: newBase[*ngrok.Endpoint]("ep"),
	}
}

func (m *EndpointsClient) Create(_ context.Context, item *ngrok.EndpointCreate) (*ngrok.Endpoint, error) {
	if m.createError != nil {
		return nil, m.createError
	}

	// Check if endpoint with this URL already exists
	if m.any(func(ep *ngrok.Endpoint) bool { return ep.URL == item.URL }) {
		return nil, &ngrok.Error{
			StatusCode: http.StatusConflict,
			Msg:        fmt.Sprintf("Endpoint with URL %s already exists", item.URL),
		}
	}

	id := m.newID()
	newEndpoint := &ngrok.Endpoint{
		ID:            id,
		URL:           item.URL,
		Type:          item.Type,
		TrafficPolicy: item.TrafficPolicy,
		CreatedAt:     m.createdAt(),
		URI:           fmt.Sprintf("https://mock-api.ngrok.com/endpoints/%s", id),
	}

	if item.Description != nil {
		newEndpoint.Description = *item.Description
	}
	if item.Metadata != nil {
		newEndpoint.Metadata = *item.Metadata
	}

	m.items[id] = newEndpoint
	return newEndpoint, nil
}

func (m *EndpointsClient) Update(ctx context.Context, item *ngrok.EndpointUpdate) (*ngrok.Endpoint, error) {
	if m.updateError != nil {
		return nil, m.updateError
	}

	existingItem, err := m.Get(ctx, item.ID)
	if err != nil {
		return nil, err
	}

	if item.Url != nil {
		existingItem.URL = *item.Url
	}
	if item.Description != nil {
		existingItem.Description = *item.Description
	}
	if item.Metadata != nil {
		existingItem.Metadata = *item.Metadata
	}
	if item.TrafficPolicy != nil {
		existingItem.TrafficPolicy = *item.TrafficPolicy
	}

	m.items[item.ID] = existingItem
	return existingItem, nil
}
