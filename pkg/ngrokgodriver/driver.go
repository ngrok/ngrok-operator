package ngrokgodriver

import (
	"context"
	"io"
	"net"
	"os"
	"reflect"

	"github.com/go-logr/logr"
	"golang.ngrok.com/ngrok"
	"golang.ngrok.com/ngrok/config"
	"golang.org/x/sync/errgroup"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

type TunnelManager struct {
	session ngrok.Session
	tunnels map[string]ngrok.Tunnel
}

func NewTunnelManager() (*TunnelManager, error) {
	opts := []ngrok.ConnectOption{
		ngrok.WithAuthtokenFromEnv(),
	}

	serverAddr, ok := os.LookupEnv("NGROK_SEVER_ADDR")
	if ok {
		opts = append(opts, ngrok.WithServer(serverAddr))
	}

	session, err := ngrok.Connect(context.Background(), opts...)
	if err != nil {
		return nil, err
	}
	return &TunnelManager{
		session: session,
		tunnels: make(map[string]ngrok.Tunnel),
	}, nil
}

func (tm *TunnelManager) CreateTunnel(ctx context.Context, t TunnelsAPIBody) error {
	log := log.FromContext(ctx)

	if tun, ok := tm.tunnels[t.Name]; ok {
		if reflect.DeepEqual(tun.Labels(), t.Labels) {
			log.Info("Tunnel labels match existing tunnel, doing nothing")
			return nil
		}
		// There is already a tunnel with this name, start the new one and defer closing the old one
		defer tm.stopTunnel(context.Background(), tun)
	}

	tun, err := tm.session.Listen(ctx, tm.buildTunnelConfig(t))
	if err != nil {
		return err
	}
	tm.tunnels[t.Name] = tun
	go tm.startTunnel(ctx, tun, t.Addr)
	return nil
}

func (tm *TunnelManager) DeleteTunnel(ctx context.Context, name string) error {
	log := log.FromContext(ctx).WithValues("name", name)

	tun := tm.tunnels[name]
	if tun == nil {
		log.Info("Tunnel not found while trying to delete tunnel")
		return nil
	}

	err := tm.stopTunnel(ctx, tun)
	if err != nil {
		return err
	}
	delete(tm.tunnels, name)
	log.Info("Tunnel deleted successfully")
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

func (tm *TunnelManager) startTunnel(ctx context.Context, tun ngrok.Tunnel, dest string) {
	log := log.FromContext(ctx).WithValues("id", tun.ID())
	for {
		conn, err := tun.Accept()
		if err != nil {
			log.Error(err, "Error accepting connection")
		}

		cnxnLogger := log.WithValues("remoteAddr", conn.RemoteAddr())
		cnxnLogger.Info("Accepted connection")

		go func(address string, logger logr.Logger) {
			err := handleConn(context.Background(), address, conn)
			if err != nil {
				logger.Error(err, "Error handling connection")
			} else {
				logger.Info("Connection closed")
			}
		}(dest, cnxnLogger)
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
