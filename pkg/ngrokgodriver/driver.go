package ngrokgodriver

import (
	"context"
	"io"
	"log"
	"net"
	"reflect"

	"golang.ngrok.com/ngrok"
	"golang.ngrok.com/ngrok/config"
	"golang.org/x/sync/errgroup"
)

type TunnelManager struct {
	tunnels map[string]ngrok.Tunnel
}

func NewTunnelManager() *TunnelManager {
	return &TunnelManager{
		tunnels: make(map[string]ngrok.Tunnel),
	}
}

func (tm *TunnelManager) CreateTunnel(ctx context.Context, t TunnelsAPIBody) error {
	if tun, ok := tm.tunnels[t.Name]; ok {
		if reflect.DeepEqual(tun.Labels(), t.Labels) {
			// The tunnel already exists and has the same labels, do nothing
			return nil
		}
		// There is already a tunnel with this name, start the new one and defer closing the old one
		defer tm.stopTunnel(context.Background(), tun)
	}

	tun, err := ngrok.Listen(ctx,
		tm.buildTunnelConfig(t),
		ngrok.WithAuthtokenFromEnv(),
	)
	if err != nil {
		return err
	}
	tm.tunnels[t.Name] = tun
	go tm.startTunnel(tun, t.Addr)
	return nil
}

func (tm *TunnelManager) DeleteTunnel(ctx context.Context, name string) error {
	tun := tm.tunnels[name]
	if tun == nil {
		return nil
	}

	err := tm.stopTunnel(ctx, tun)
	if err != nil {
		return err
	}
	delete(tm.tunnels, name)
	return nil
}

func (tm *TunnelManager) stopTunnel(ctx context.Context, tun ngrok.Tunnel) error {
	if tun == nil {
		return nil
	}
	return tun.CloseWithContext(ctx)
}

func (tm *TunnelManager) buildTunnelConfig(t TunnelsAPIBody) config.Tunnel {
	opts := []config.LabeledTunnelOption{}
	for key, value := range t.Labels {
		opts = append(opts, config.WithLabel(key, value))
	}
	return config.LabeledTunnel(opts...)
}

func (tm *TunnelManager) startTunnel(tun ngrok.Tunnel, dest string) {
	for {
		conn, err := tun.Accept()
		if err != nil {
			log.Println(err)
		}

		log.Println("Accepted connection from ", conn.RemoteAddr())
		go func(address string) {
			err := handleConn(context.Background(), address, conn)
			log.Println("Connection closed: ", err)
		}(dest)
	}
}

type TunnelsAPIBody struct {
	Addr      string            `json:"addr"`
	Name      string            `json:"name"`
	SubDomain string            `json:"subdomain,omitempty"`
	Labels    map[string]string `json:"labels"`
}

func handleConn(ctx context.Context, dest string, conn net.Conn) error {
	next, err := net.Dial("tcp", dest)
	if err != nil {
		return err
	}

	g, _ := errgroup.WithContext(ctx)

	g.Go(func() error {
		_, err := io.Copy(next, conn)
		return err
	})
	g.Go(func() error {
		_, err := io.Copy(conn, next)
		return err
	})
	return g.Wait()
}
