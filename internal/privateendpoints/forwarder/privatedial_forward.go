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
// Proposed upstream API: privatedial.Join (this file's join, exported) and a
// Forwarder that serves a listener.
//
// When that exists: replace ForwardConn/join/closeWrite/abort with privatedial.Join
// (or have the ForwarderReconciler hand each listener to Dialer.Forward and drop the
// per-connection handler entirely).
//
// Behavior today:
//   - Each direction copies to EOF, then half-closes its destination
//     (CloseWrite, or Close if unsupported). privatedial's conn implements
//     CloseWrite by closing the request body; what the endpoint's origin
//     sees then is up to the server.
//   - A client half-close starts no timer: the response may take as long as
//     the endpoint needs. Only when the endpoint finishes first does the
//     client get drain to finish sending, so a client that never closes
//     can't pin a stream. There is no idle timeout (privatedial deadlines
//     are no-ops), so a silent but live peer can still hold a conn open.
//   - A copy error (not EOF) aborts both sides and is returned, so truncation
//     is never reported as an orderly close. A TCP client is reset (RST);
//     privatedial's Close cancels the stream.
//   - A dial failure resets the client too, rather than a clean EOF that
//     looks like an empty response.
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
		abort(client)
		return fmt.Errorf("private dial to %s: %w", target, err)
	}
	if err := join(client, upstream, drain); err != nil {
		return fmt.Errorf("forwarding to %s: %w", target, err)
	}
	return nil
}

// join copies between client and upstream until both directions finish, or
// until drain after upstream finishes first. It closes both conns, and waits
// for both copy goroutines, before returning.
func join(client, upstream net.Conn, drain time.Duration) error {
	type result struct {
		fromUpstream bool
		err          error
	}
	done := make(chan result, 2)
	copyTo := func(dst, src net.Conn, fromUpstream bool) {
		_, err := io.Copy(dst, src)
		if err == nil {
			closeWrite(dst)
		}
		done <- result{fromUpstream, err}
	}
	go copyTo(upstream, client, false)
	go copyTo(client, upstream, true)

	first := <-done
	if first.err != nil {
		abort(client)
		abort(upstream)
		<-done
		return first.err
	}

	var drained <-chan time.Time
	if first.fromUpstream {
		t := time.NewTimer(drain)
		defer t.Stop()
		drained = t.C
	}
	select {
	case second := <-done:
		if second.err != nil {
			abort(client)
			abort(upstream)
			return second.err
		}
		_ = client.Close()
		_ = upstream.Close()
	case <-drained:
		_ = client.Close()
		_ = upstream.Close()
		<-done // its error is from our own Close
	}
	return nil
}

func closeWrite(c net.Conn) {
	if cw, ok := c.(interface{ CloseWrite() error }); ok {
		_ = cw.CloseWrite()
		return
	}
	_ = c.Close()
}

// abort closes c so the peer sees a reset rather than an orderly EOF, where
// the conn type allows it.
func abort(c net.Conn) {
	if l, ok := c.(interface{ SetLinger(int) error }); ok {
		_ = l.SetLinger(0)
	}
	_ = c.Close()
}
