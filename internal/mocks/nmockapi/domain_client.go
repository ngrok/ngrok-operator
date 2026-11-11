package nmockapi

import (
	context "context"
	"fmt"
	"net/http"
	"slices"
	"strings"

	"github.com/ngrok/ngrok-api-go/v9"
	"k8s.io/apimachinery/pkg/util/rand"
)

// DomainClient is a mock implementation of the ngrok API client for managing reserved domains. It
// tries to mimic the behavior of the actual ngrok API client, but it is not a complete
// implementation. It is used for testing purposes only and should not be used in production
// environments.
type DomainClient struct {
	baseClient[*ngrok.ReservedDomain, ngrok.FilteredPaging]

	// concurrentlyReserved holds domains that get reserved just before the
	// next Create of them, see ReserveConcurrently.
	concurrentlyReserved map[string]bool
}

func NewDomainClient() *DomainClient {
	return &DomainClient{
		baseClient: newBase[*ngrok.ReservedDomain, ngrok.FilteredPaging](
			"rd",
		),
	}
}

// ReserveConcurrently makes the next Create of domain find it already
// reserved, as if something reserved it after the caller looked it up. ngrok
// does this when it creates an endpoint on a hostname that isn't reserved yet.
func (m *DomainClient) ReserveConcurrently(domain string) {
	if m.concurrentlyReserved == nil {
		m.concurrentlyReserved = map[string]bool{}
	}
	m.concurrentlyReserved[domain] = true
}

// Reset clears the client's state between tests.
func (m *DomainClient) Reset() {
	m.baseClient.Reset()
	m.concurrentlyReserved = nil
}

func (m *DomainClient) Create(_ context.Context, item *ngrok.ReservedDomainCreate) (*ngrok.ReservedDomain, error) {
	if m.createError != nil {
		return nil, m.createError
	}

	if m.concurrentlyReserved[item.Domain] {
		delete(m.concurrentlyReserved, item.Domain)
		id := m.newID()
		m.items[id] = &ngrok.ReservedDomain{
			ID:        id,
			CreatedAt: m.createdAt(),
			Domain:    item.Domain,
			URI:       fmt.Sprintf("https://mock-api.ngrok.com/reserved_domains/%s", id),
		}
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
		ID:          id,
		CreatedAt:   m.createdAt(),
		Domain:      item.Domain,
		Description: item.Description,
		Metadata:    item.Metadata,
		Region:      item.Region,
		URI:         fmt.Sprintf("https://mock-api.ngrok.com/reserved_domains/%s", id),
		ResolvesTo:  item.ResolvesTo,
	}

	if !isNgrokManagedDomain(newDomain) {
		cname := fmt.Sprintf("%s.%s.ngrok-cname.com", rand.String(17), rand.String(17))
		newDomain.CNAMETarget = &cname
	}
	m.items[id] = newDomain
	return newDomain, nil
}

func (m *DomainClient) Update(ctx context.Context, item *ngrok.ReservedDomainUpdate) (*ngrok.ReservedDomain, error) {
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

	if item.ResolvesTo != nil {
		existingItem.ResolvesTo = item.ResolvesTo
	}

	m.items[item.ID] = existingItem
	return existingItem, nil
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
