package nmockapi

import (
	"context"
	"fmt"
	"sync"

	"github.com/ngrok/ngrok-api-go/v7"
)

// KubernetesOperatorsClient is a mock implementation of the ngrok API client for managing kubernetes operators.
type KubernetesOperatorsClient struct {
	baseClient[*ngrok.KubernetesOperator]

	// mu protects boundEndpoints
	mu sync.RWMutex
	// boundEndpoints stores the bound endpoints for testing
	boundEndpoints []ngrok.Endpoint
}

func NewKubernetesOperatorsClient() *KubernetesOperatorsClient {
	return &KubernetesOperatorsClient{
		baseClient:     newBase[*ngrok.KubernetesOperator]("ko"),
		boundEndpoints: []ngrok.Endpoint{},
	}
}

func (m *KubernetesOperatorsClient) Create(_ context.Context, item *ngrok.KubernetesOperatorCreate) (*ngrok.KubernetesOperator, error) {
	if m.createError != nil {
		return nil, m.createError
	}

	id := m.newID()
	newOp := &ngrok.KubernetesOperator{
		ID:          id,
		URI:         fmt.Sprintf("https://mock-api.ngrok.com/kubernetes_operators/%s", id),
		CreatedAt:   m.createdAt(),
		Description: item.Description,
		Metadata:    item.Metadata,
	}

	// Convert binding create to binding (simplified for mock)
	if item.Binding != nil {
		ingressEndpoint := ""
		if item.Binding.IngressEndpoint != nil {
			ingressEndpoint = *item.Binding.IngressEndpoint
		}
		newOp.Binding = &ngrok.KubernetesOperatorBinding{
			EndpointSelectors: item.Binding.EndpointSelectors,
			IngressEndpoint:   ingressEndpoint,
		}
	}

	m.items[id] = newOp
	return newOp, nil
}

func (m *KubernetesOperatorsClient) Update(ctx context.Context, item *ngrok.KubernetesOperatorUpdate) (*ngrok.KubernetesOperator, error) {
	if m.updateError != nil {
		return nil, m.updateError
	}

	existingItem, err := m.Get(ctx, item.ID)
	if err != nil {
		return nil, err
	}

	// Update is not used much in tests, just keep it simple
	if item.Description != nil {
		existingItem.Description = *item.Description
	}
	if item.Metadata != nil {
		existingItem.Metadata = *item.Metadata
	}

	m.items[item.ID] = existingItem
	return existingItem, nil
}

// GetBoundEndpoints returns an iterator of endpoints bound to this kubernetes operator
func (m *KubernetesOperatorsClient) GetBoundEndpoints(_ string, _ *ngrok.Paging) ngrok.Iter[*ngrok.Endpoint] {
	m.mu.RLock()
	defer m.mu.RUnlock()

	// Copy endpoints to avoid race conditions
	endpoints := make([]*ngrok.Endpoint, len(m.boundEndpoints))
	for i := range m.boundEndpoints {
		ep := m.boundEndpoints[i]
		endpoints[i] = &ep
	}

	return NewIter(endpoints, nil)
}

// SetBoundEndpoints sets the endpoints that will be returned by GetBoundEndpoints
func (m *KubernetesOperatorsClient) SetBoundEndpoints(endpoints []ngrok.Endpoint) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.boundEndpoints = endpoints
}

// ResetBoundEndpoints clears all bound endpoints
func (m *KubernetesOperatorsClient) ResetBoundEndpoints() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.boundEndpoints = []ngrok.Endpoint{}
}
