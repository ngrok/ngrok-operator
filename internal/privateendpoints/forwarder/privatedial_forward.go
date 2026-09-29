package forwarder

// Forwarding one accepted connection to a private endpoint.
//
// Everything in this file is generic "private dial client" plumbing, not
// Kubernetes logic, and is meant to move into golang.ngrok.com/ngrok/privatedial.
// It is isolated here so that move is a file deletion.
//
// Why it exists: privatedial.Dialer gives us DialContext and nothing that
// accepts or joins connections. Every consumer re-implements the same
// "accept on a local port, dial host:port, copy both ways" loop:
//
//   - agent v4 `ngrok bind` (ngrok-agent internal/daemon/bind.go, proxyConn in
//     internal/daemon/dial_session.go) and its TUN path (internal/netstack/tcp.go pipe)
//   - ngrok-go main's endpointForwarder.join (forwarder.go)
//   - this operator's old bindings forwarder (joinConnections in
//     internal/controller/bindings/forwarder_controller.go)
//   - this file
//
// None of those are exported, and they differ on the details that matter:
// half-close propagation, how long the second direction may keep running
// after the first finishes, and which errors count as a clean close.
//
// Proposed upstream API (privatedial):
//
//	// Join copies between a and b until both directions finish, half-closing
//	// each side at EOF and force-closing both drain after the first finishes.
//	func Join(a, b net.Conn, drain time.Duration) error
//
//	// Forward accepts on l until ctx is done and joins each connection with a
//	// dial to target. It returns when l is closed or ctx is done.
//	func (d *Dialer) Forward(ctx context.Context, l net.Listener, target string) error
//
// When that exists: replace ForwardConn/join/closeWrite with privatedial.Join
// (or have the Syncer hand each listener to Dialer.Forward and drop the
// per-connection handler entirely).
//
// Behavior we rely on today, matching agent v4:
//   - Each direction half-closes its destination (CloseWrite) at EOF, so a
//     client that shuts down its write side still gets the full response.
//     privatedial's conn implements CloseWrite by closing the request body.
//   - After the first direction finishes, the other gets drain before both
//     conns are closed, so a client that never closes can't pin a stream.
//   - Dial time is bounded by privatedial itself (5s budget per dial).

import (
	"context"
	"fmt"
	"io"
	"net"
	"time"
)

// DialFunc dials a private endpoint at host:port.
type DialFunc func(ctx context.Context, address string) (net.Conn, error)

// ForwardConn dials target and joins it with client. It always closes client.
func ForwardConn(ctx context.Context, dial DialFunc, client net.Conn, target string, drain time.Duration) error {
	upstream, err := dial(ctx, target)
	if err != nil {
		_ = client.Close()
		return fmt.Errorf("private dial to %s: %w", target, err)
	}
	join(client, upstream, drain)
	return nil
}

func join(a, b net.Conn, drain time.Duration) {
	done := make(chan struct{}, 2)
	go func() {
		_, _ = io.Copy(b, a)
		closeWrite(b)
		done <- struct{}{}
	}()
	go func() {
		_, _ = io.Copy(a, b)
		closeWrite(a)
		done <- struct{}{}
	}()
	<-done
	select {
	case <-done:
	case <-time.After(drain):
	}
	_ = a.Close()
	_ = b.Close()
}

func closeWrite(c net.Conn) {
	if cw, ok := c.(interface{ CloseWrite() error }); ok {
		_ = cw.CloseWrite()
		return
	}
	_ = c.Close()
}
