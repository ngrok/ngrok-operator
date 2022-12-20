package ngrokgodriver

import (
	"context"
	"io"
	"log"
	"net"
	"strings"

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
	if tun != nil {
		err := tun.CloseWithContext(ctx)
		if err != nil {
			return err
		}
		delete(tm.tunnels, name)
	}
	return nil
}

func (tm *TunnelManager) buildTunnelConfig(t TunnelsAPIBody) config.Tunnel {
	opts := []config.LabeledTunnelOption{}
	for _, label := range t.Labels {
		parts := strings.Split(label, "=")
		key := parts[0]
		value := parts[1]
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
	Addr      string   `json:"addr"`
	Name      string   `json:"name"`
	SubDomain string   `json:"subdomain,omitempty"`
	Labels    []string `json:"labels"`
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
