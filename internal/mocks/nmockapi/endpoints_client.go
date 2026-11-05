package nmockapi

import (
	context "context"
	"fmt"
	"net/http"
	"sync"

	"github.com/ngrok/ngrok-api-go/v9"
	"k8s.io/utils/ptr"
)

// EndpointsClient is a mock implementation of the ngrok API client for managing endpoints.
type EndpointsClient struct {
	baseClient[*ngrok.Endpoint, ngrok.Paging]

	callsMu sync.Mutex
	calls   []EndpointCall
}

// EndpointCall records one Create, Update or Delete call, whether or not it
// succeeded. URL is the endpoint's URL when known: the requested URL for
// Create and Update, the stored URL for Delete of an existing endpoint.
type EndpointCall struct {
	Method string
	ID     string
	URL    string
}

// Calls returns every Create, Update and Delete call made so far, oldest
// first.
func (m *EndpointsClient) Calls() []EndpointCall {
	m.callsMu.Lock()
	defer m.callsMu.Unlock()
	return append([]EndpointCall(nil), m.calls...)
}

func (m *EndpointsClient) record(method, id, url string) {
	m.callsMu.Lock()
	defer m.callsMu.Unlock()
	m.calls = append(m.calls, EndpointCall{Method: method, ID: id, URL: url})
}

func NewEndpointsClient() *EndpointsClient {
	return &EndpointsClient{
		baseClient: newBase[*ngrok.Endpoint, ngrok.Paging]("ep"),
	}
}

func (m *EndpointsClient) Create(_ context.Context, item *ngrok.EndpointCreate) (*ngrok.Endpoint, error) {
	m.record("Create", "", item.URL)
	if m.createError != nil {
		return nil, m.createError
	}

	// Check if endpoint with this URL already exists. Like the ngrok API,
	// endpoints may share a URL when all of them enable pooling.
	pooling := ptr.Deref(item.PoolingEnabled, false)
	if m.any(func(ep *ngrok.Endpoint) bool { return ep.URL == item.URL && !(pooling && ep.PoolingEnabled) }) {
		return nil, &ngrok.Error{
			StatusCode: http.StatusConflict,
			Msg:        fmt.Sprintf("Endpoint with URL %s already exists", item.URL),
		}
	}

	id := m.newID()
	newEndpoint := &ngrok.Endpoint{
		Description:    ptr.Deref(item.Description, ""),
		Metadata:       ptr.Deref(item.Metadata, ""),
		ID:             id,
		URL:            item.URL,
		Type:           item.Type,
		TrafficPolicy:  item.TrafficPolicy,
		Bindings:       item.Bindings,
		PoolingEnabled: ptr.Deref(item.PoolingEnabled, false),
		CreatedAt:      m.createdAt(),
		URI:            fmt.Sprintf("https://mock-api.ngrok.com/endpoints/%s", id),
	}

	m.items[id] = newEndpoint
	return newEndpoint, nil
}

func (m *EndpointsClient) Update(ctx context.Context, item *ngrok.EndpointUpdate) (*ngrok.Endpoint, error) {
	m.record("Update", item.ID, ptr.Deref(item.Url, ""))
	m.updateCallCount++
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
	if item.Bindings != nil {
		existingItem.Bindings = item.Bindings
	}
	if item.PoolingEnabled != nil {
		existingItem.PoolingEnabled = *item.PoolingEnabled
	}

	m.items[item.ID] = existingItem
	return existingItem, nil
}

func (m *EndpointsClient) Delete(ctx context.Context, id string) error {
	url := ""
	if ep, ok := m.items[id]; ok {
		url = ep.URL
	}
	m.record("Delete", id, url)
	return m.baseClient.Delete(ctx, id)
}
