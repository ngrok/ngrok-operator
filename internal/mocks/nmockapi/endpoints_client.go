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

	// Error injection fields for testing
	createError error
	getError    error
	updateError error
	listError   error
}

func NewEndpointsClient() *EndpointsClient {
	return &EndpointsClient{
		baseClient: newBase[*ngrok.Endpoint]("ep"),
	}
}

// SetCreateError configures the client to return an error on Create calls
func (m *EndpointsClient) SetCreateError(err error) {
	m.createError = err
}

// SetGetError configures the client to return an error on Get calls
func (m *EndpointsClient) SetGetError(err error) {
	m.getError = err
}

// SetUpdateError configures the client to return an error on Update calls
func (m *EndpointsClient) SetUpdateError(err error) {
	m.updateError = err
}

// SetListError configures the client to return an error on List calls
func (m *EndpointsClient) SetListError(err error) {
	m.listError = err
}

// ClearErrors clears all configured errors
func (m *EndpointsClient) ClearErrors() {
	m.createError = nil
	m.getError = nil
	m.updateError = nil
	m.listError = nil
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

// Get overrides the base client Get method to add error injection
func (m *EndpointsClient) Get(ctx context.Context, id string) (*ngrok.Endpoint, error) {
	if m.getError != nil {
		return nil, m.getError
	}

	item, err := m.baseClient.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	return item, nil
}
