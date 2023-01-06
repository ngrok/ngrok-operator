package tunneldriver

import (
	"context"
	"io"
	"net"
	"reflect"

	"github.com/go-logr/logr"
	"golang.ngrok.com/ngrok"
	"golang.ngrok.com/ngrok/config"
	"golang.org/x/sync/errgroup"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

// TunnelDriver is a driver for creating and deleting ngrok tunnels
type TunnelDriver struct {
	session ngrok.Session
	tunnels map[string]ngrok.Tunnel
}

// New creates and initializes a new TunnelDriver
func New(serverAddr string) (*TunnelDriver, error) {
	opts := []ngrok.ConnectOption{
		ngrok.WithAuthtokenFromEnv(),
	}
	if serverAddr != "" {
		opts = append(opts, ngrok.WithServer(serverAddr))
	}

	session, err := ngrok.Connect(context.Background(), opts...)
	if err != nil {
		return nil, err
	}
	return &TunnelDriver{
		session: session,
		tunnels: make(map[string]ngrok.Tunnel),
	}, nil
}

// CreateTunnel creates and starts a new tunnel in a goroutine. If a tunnel with the same name already exists,
// it will be stopped and replaced with a new tunnel unless the labels match.
func (td *TunnelDriver) CreateTunnel(ctx context.Context, name string, labels map[string]string, destination string) error {
	log := log.FromContext(ctx)

	if tun, ok := td.tunnels[name]; ok {
		if reflect.DeepEqual(tun.Labels(), labels) {
			log.Info("Tunnel labels match existing tunnel, doing nothing")
			return nil
		}
		// There is already a tunnel with this name, start the new one and defer closing the old one
		defer td.stopTunnel(context.Background(), tun)
	}

	tun, err := td.session.Listen(ctx, td.buildTunnelConfig(labels, destination))
	if err != nil {
		return err
	}
	td.tunnels[name] = tun
	go td.startTunnel(ctx, tun, destination)
	return nil
}

// DeleteTunnel stops and deletes a tunnel
func (td *TunnelDriver) DeleteTunnel(ctx context.Context, name string) error {
	log := log.FromContext(ctx).WithValues("name", name)

	tun := td.tunnels[name]
	if tun == nil {
		log.Info("Tunnel not found while trying to delete tunnel")
		return nil
	}

	err := td.stopTunnel(ctx, tun)
	if err != nil {
		return err
	}
	delete(td.tunnels, name)
	log.Info("Tunnel deleted successfully")
	return nil
}

func (td *TunnelDriver) stopTunnel(ctx context.Context, tun ngrok.Tunnel) error {
	if tun == nil {
		return nil
	}
	return tun.CloseWithContext(ctx)
}

func (td *TunnelDriver) buildTunnelConfig(labels map[string]string, destination string) config.Tunnel {
	opts := []config.LabeledTunnelOption{}
	for key, value := range labels {
		opts = append(opts, config.WithLabel(key, value))
	}
	opts = append(opts, config.WithForwardsTo(destination))
	return config.LabeledTunnel(opts...)
}

func (td *TunnelDriver) startTunnel(ctx context.Context, tun ngrok.Tunnel, dest string) {
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
