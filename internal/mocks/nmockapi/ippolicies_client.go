package nmockapi

import (
	context "context"
	"fmt"
	"net/http"

	"github.com/ngrok/ngrok-api-go/v7"
)

type IPPolicyClient struct {
	baseClient[*ngrok.IPPolicy]
}

func NewIPPolicyClient() *IPPolicyClient {
	return &IPPolicyClient{
		baseClient: newBase[*ngrok.IPPolicy](
			"ipp",
		),
	}
}

func (m *IPPolicyClient) Create(_ context.Context, item *ngrok.IPPolicyCreate) (*ngrok.IPPolicy, error) {
	// Simple validation for tests: description must be non-empty
	if item.Description == "" {
		return nil, &ngrok.Error{StatusCode: http.StatusBadRequest, Msg: "description required"}
	}
	id := m.newID()

	newIPPolicy := &ngrok.IPPolicy{
		ID:          id,
		CreatedAt:   m.createdAt(),
		Description: item.Description,
		Metadata:    item.Metadata,
		URI:         fmt.Sprintf("https://mock-api.ngrok.com/ip_policies/%s", id),
	}

	m.items[id] = newIPPolicy
	return newIPPolicy, nil
}

func (m *IPPolicyClient) Update(ctx context.Context, item *ngrok.IPPolicyUpdate) (*ngrok.IPPolicy, error) {
	existingItem, err := m.Get(ctx, item.ID)
	if err != nil {
		return nil, err
	}

	if item.Description != nil {
		existingItem.Description = *item.Description
	}
	if item.Metadata != nil {
		existingItem.Metadata = *item.Metadata
	}

	m.items[item.ID] = existingItem
	return existingItem, nil
}

func (m *IPPolicyClient) ConfigureRules(ctx context.Context, id string, rules []*ngrok.IPPolicyRule) (*ngrok.IPPolicy, error) {
	// The real ngrok IPPolicy does not store rules on the policy object in this mock.
	// Rules are managed via the IPPolicyRule client. For compatibility, just verify the
	// policy exists and return it.
	existingItem, err := m.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	return existingItem, nil
}
