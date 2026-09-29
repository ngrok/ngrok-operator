package forwarder

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"slices"
	"strconv"
	"sync"
	"time"

	"github.com/go-logr/logr"
)

type DialFunc func(ctx context.Context, address string) (net.Conn, error)

// Proxy forwards accepted connections to private endpoints via Dial.
type Proxy struct {
	Table       *Table
	Dial        DialFunc
	Log         logr.Logger
	PeekTimeout time.Duration
}

// ServeShared routes by the name peek extracts (Host header or SNI) for
// endpoints on the given logical port (80 or 443).
func (p *Proxy) ServeShared(ctx context.Context, l net.Listener, port int32, peek func(*bufio.Reader) (string, error)) error {
	return serve(ctx, l, func(c net.Conn) {
		br := bufio.NewReaderSize(c, PeekBufferSize)
		_ = c.SetReadDeadline(time.Now().Add(p.PeekTimeout))
		host, err := peek(br)
		_ = c.SetReadDeadline(time.Time{})
		if err != nil {
			p.Log.V(1).Info("closing connection without a routable name", "port", port, "remote", c.RemoteAddr().String(), "reason", err.Error())
			_ = c.Close()
			return
		}
		target, ok := p.Table.SharedTarget(host, port)
		if !ok {
			p.Log.V(1).Info("closing connection for unknown private endpoint", "host", host, "port", port)
			_ = c.Close()
			return
		}
		p.forward(ctx, c, br, target)
	})
}

// ServePort forwards every connection on a per-endpoint forwarder port.
func (p *Proxy) ServePort(ctx context.Context, l net.Listener, fwdPort int32) error {
	return serve(ctx, l, func(c net.Conn) {
		target, ok := p.Table.PortTarget(fwdPort)
		if !ok {
			_ = c.Close()
			return
		}
		p.forward(ctx, c, c, target)
	})
}

func (p *Proxy) forward(ctx context.Context, client net.Conn, clientR io.Reader, target string) {
	upstream, err := p.Dial(ctx, target)
	if err != nil {
		p.Log.Error(err, "private dial failed", "target", target)
		_ = client.Close()
		return
	}
	pipe(client, clientR, upstream)
}

func serve(ctx context.Context, l net.Listener, handle func(net.Conn)) error {
	stop := context.AfterFunc(ctx, func() { _ = l.Close() })
	defer stop()
	for {
		c, err := l.Accept()
		if err != nil {
			if ctx.Err() != nil || errors.Is(err, net.ErrClosed) {
				return nil
			}
			return err
		}
		go handle(c)
	}
}

// pipe copies both ways. clientR replays any bytes already peeked from client.
func pipe(client net.Conn, clientR io.Reader, upstream net.Conn) {
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		_, _ = io.Copy(upstream, clientR)
		closeWrite(upstream)
	}()
	go func() {
		defer wg.Done()
		_, _ = io.Copy(client, upstream)
		closeWrite(client)
	}()
	wg.Wait()
	_ = client.Close()
	_ = upstream.Close()
}

func closeWrite(c net.Conn) {
	if cw, ok := c.(interface{ CloseWrite() error }); ok {
		_ = cw.CloseWrite()
		return
	}
	_ = c.Close()
}

// PortListeners keeps one listener open per forwarder port in the last Sync.
type PortListeners struct {
	Proxy    *Proxy
	Log      logr.Logger
	BindHost string

	mu sync.Mutex
	ls map[int32]net.Listener
}

// Sync opens listeners for new ports and closes ones no longer wanted. ctx
// bounds the listeners' lifetime and must outlive this call.
func (pl *PortListeners) Sync(ctx context.Context, ports []int32) error {
	pl.mu.Lock()
	defer pl.mu.Unlock()
	if pl.ls == nil {
		pl.ls = map[int32]net.Listener{}
	}
	want := map[int32]bool{}
	var errs []error
	for _, port := range ports {
		want[port] = true
		if _, ok := pl.ls[port]; ok {
			continue
		}
		l, err := net.Listen("tcp", net.JoinHostPort(pl.BindHost, strconv.Itoa(int(port))))
		if err != nil {
			errs = append(errs, fmt.Errorf("listening on forwarder port %d: %w", port, err))
			continue
		}
		pl.ls[port] = l
		go func() {
			if err := pl.Proxy.ServePort(ctx, l, port); err != nil {
				pl.Log.Error(err, "forwarder port listener stopped", "port", port)
			}
		}()
	}
	for port, l := range pl.ls {
		if !want[port] {
			_ = l.Close()
			delete(pl.ls, port)
		}
	}
	return errors.Join(errs...)
}

func (pl *PortListeners) Ports() []int32 {
	pl.mu.Lock()
	defer pl.mu.Unlock()
	var out []int32
	for p := range pl.ls {
		out = append(out, p)
	}
	slices.Sort(out)
	return out
}

func (pl *PortListeners) Close() {
	pl.mu.Lock()
	defer pl.mu.Unlock()
	for port, l := range pl.ls {
		_ = l.Close()
		delete(pl.ls, port)
	}
}
