package nmockapi

import (
	context "context"
	"errors"
	"fmt"
	"math/rand"

	"github.com/ngrok/ngrok-api-go/v7"
)

type TCPAddressesClient struct {
	baseClient[*ngrok.ReservedAddr]
}

func NewTCPAddressClient() *TCPAddressesClient {
	return &TCPAddressesClient{
		baseClient: newBase[*ngrok.ReservedAddr](
			"ra",
		),
	}
}

func (m *TCPAddressesClient) Create(_ context.Context, item *ngrok.ReservedAddrCreate) (*ngrok.ReservedAddr, error) {
	id := m.newID()

	newAddr := &ngrok.ReservedAddr{
		ID:          id,
		CreatedAt:   m.createdAt(),
		Region:      item.Region,
		Description: item.Description,
		URI:         "https://mock-api.ngrok.com/reserved_addrs/" + id,
		Addr:        "0.tcp.ngrok.io:1",
	}

	// Generate a random port in the range 10000-30000
	newAddr.Addr = fmt.Sprintf("%d.tcp.ngrok.io:%d", rand.Intn(7), rand.Intn(20000)+10000)

	m.items[id] = newAddr
	return newAddr, nil
}

func (m *TCPAddressesClient) Update(_ context.Context, _ *ngrok.ReservedAddrUpdate) (*ngrok.ReservedAddr, error) {
	return nil, errors.New("not implemented")
}
