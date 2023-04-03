package tunneldriver

import (
	"context"
	"net"
)

// Dialer is the portion of *net.Dialer that this package uses.
// This is exported only for testing reasons.
type Dialer interface {
	DialContext(context.Context, string, string) (net.Conn, error)
}

var _ Dialer = &net.Dialer{}
