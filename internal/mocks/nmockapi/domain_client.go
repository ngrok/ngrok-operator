package nmockapi

import (
	context "context"
	"fmt"
	"net/http"
	"slices"
	"strings"

	"github.com/ngrok/ngrok-api-go/v7"
	"k8s.io/apimachinery/pkg/util/rand"
)

// DomainClient is a mock implementation of the ngrok API client for managing reserved domains. It
// tries to mimic the behavior of the actual ngrok API client, but it is not a complete
// implementation. It is used for testing purposes only and should not be used in production
// environments.
type DomainClient struct {
	baseClient[*ngrok.ReservedDomain]

	// Error injection fields for testing
	createError error
	getError    error
	updateError error
	listError   error
}

func NewDomainClient() *DomainClient {
	return &DomainClient{
		baseClient: newBase[*ngrok.ReservedDomain](
			"rd",
		),
	}
}

// SetCreateError configures the client to return an error on Create calls
func (m *DomainClient) SetCreateError(err error) {
	m.createError = err
}

// SetGetError configures the client to return an error on Get calls
func (m *DomainClient) SetGetError(err error) {
	m.getError = err
}

// SetUpdateError configures the client to return an error on Update calls
func (m *DomainClient) SetUpdateError(err error) {
	m.updateError = err
}

// SetListError configures the client to return an error on List calls
func (m *DomainClient) SetListError(err error) {
	m.listError = err
}

// ClearErrors clears all configured errors
func (m *DomainClient) ClearErrors() {
	m.createError = nil
	m.getError = nil
	m.updateError = nil
	m.listError = nil
}

func (m *DomainClient) Create(_ context.Context, item *ngrok.ReservedDomainCreate) (*ngrok.ReservedDomain, error) {
	// Check for injected error first
	if m.createError != nil {
		return nil, m.createError
	}

	if m.any(func(rd *ngrok.ReservedDomain) bool { return rd.Domain == item.Domain }) {
		return nil, &ngrok.Error{
			StatusCode: http.StatusConflict,
			Msg:        fmt.Sprintf("Domain %s already exists", item.Domain),
			ErrorCode:  "ERR_NGROK_413",
		}
	}

	id := m.newID()

	newDomain := &ngrok.ReservedDomain{
		ID:        id,
		CreatedAt: m.createdAt(),
		Domain:    item.Domain,
		Region:    item.Region,
		URI:       fmt.Sprintf("https://mock-api.ngrok.com/reserved_domains/%s", id),
	}

	if !isNgrokManagedDomain(newDomain) {
		cname := fmt.Sprintf("%s.%s.ngrok-cname.com", rand.String(17), rand.String(17))
		newDomain.CNAMETarget = &cname
	}
	m.items[id] = newDomain
	return newDomain, nil
}

func (m *DomainClient) Update(ctx context.Context, item *ngrok.ReservedDomainUpdate) (*ngrok.ReservedDomain, error) {
	// Check for injected error first
	if m.updateError != nil {
		return nil, m.updateError
	}

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

	if item.CertificateID != nil {
		existingItem.Certificate = &ngrok.Ref{
			ID:  *item.CertificateID,
			URI: fmt.Sprintf("https://mock-api.ngrok.com/certificates/%s", *item.CertificateID),
		}
	}

	if item.CertificateManagementPolicy != nil {
		existingItem.CertificateManagementPolicy = item.CertificateManagementPolicy
	}

	m.items[item.ID] = existingItem
	return existingItem, nil
}

// Get overrides the base client Get method to add error injection
func (m *DomainClient) Get(ctx context.Context, id string) (*ngrok.ReservedDomain, error) {
	// Check for injected error first
	if m.getError != nil {
		return nil, m.getError
	}

	// Call the base client Get method
	item, err := m.baseClient.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	return item, nil
}

var (
	ngrokManagedDomainSuffixes = []string{
		"ngrok.app",
		"ngrok.dev",
		"ngrok.pizza",
		"ngrok-free.app",
		"ngrok-free.dev",
		"ngrok-free.pizza",
		"ngrok.io",
	}
)

func isNgrokManagedDomain(domain *ngrok.ReservedDomain) bool {
	if domain == nil {
		return false
	}

	return slices.ContainsFunc(ngrokManagedDomainSuffixes, func(suffix string) bool {
		return strings.HasSuffix(domain.Domain, suffix)
	})
}
