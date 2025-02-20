package tunneldriver

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/go-logr/logr"
	commonv1alpha1 "github.com/ngrok/ngrok-operator/api/common/v1alpha1"
	ingressv1alpha1 "github.com/ngrok/ngrok-operator/api/ingress/v1alpha1"
	ngrokv1alpha1 "github.com/ngrok/ngrok-operator/api/ngrok/v1alpha1"
	"github.com/ngrok/ngrok-operator/internal/version"
	"golang.org/x/exp/maps"
	"golang.org/x/sync/errgroup"
	"sigs.k8s.io/controller-runtime/pkg/log"

	"golang.ngrok.com/ngrok"
	"golang.ngrok.com/ngrok/config"
	logrok "golang.ngrok.com/ngrok/log"
)

type k8sLogger struct {
	logger logr.Logger
}

func (l k8sLogger) Log(ctx context.Context, level logrok.LogLevel, msg string, kvs map[string]interface{}) {
	keysAndValues := []any{}
	for k, v := range kvs {
		keysAndValues = append(keysAndValues, k, v)
	}
	l.logger.V(level-4).Info(msg, keysAndValues...)
}

const (
	// TODO: Make this configurable via helm and document it so users can
	// use it for things like proxies
	customCertsPath = "/etc/ssl/certs/ngrok/"
)

type commonEndpointOption interface {
	config.HTTPEndpointOption
	config.TLSEndpointOption
	config.TCPEndpointOption
}

type agentEndpointMap struct {
	m  map[string]ngrok.Tunnel
	mu sync.Mutex
}

func newAgentEndpointMap() *agentEndpointMap {
	return &agentEndpointMap{
		m: make(map[string]ngrok.Tunnel),
	}
}

func (a *agentEndpointMap) Add(name string, tun ngrok.Tunnel) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.m[name] = tun
}

func (a *agentEndpointMap) Get(name string) (ngrok.Tunnel, bool) {
	a.mu.Lock()
	defer a.mu.Unlock()
	tun, ok := a.m[name]
	return tun, ok
}

func (a *agentEndpointMap) Delete(name string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	delete(a.m, name)
}

// TunnelDriver is a driver for creating and deleting ngrok tunnels
type TunnelDriver struct {
	session        atomic.Pointer[sessionState]
	tunnels        map[string]ngrok.Tunnel
	agentEndpoints *agentEndpointMap
}

// TunnelDriverOpts are options for creating a new TunnelDriver
type TunnelDriverOpts struct {
	ServerAddr string
	Region     string
	RootCAs    string
	Comments   *TunnelDriverComments
}

type TunnelDriverComments struct {
	Gateway string `json:"gateway,omitempty"`
}

type sessionState struct {
	session   ngrok.Session
	readyErr  error
	healthErr error
}

// New creates and initializes a new TunnelDriver
func New(ctx context.Context, logger logr.Logger, opts TunnelDriverOpts) (*TunnelDriver, error) {
	tunnelComment := opts.Comments
	comments := []string{}

	td := &TunnelDriver{
		tunnels:        make(map[string]ngrok.Tunnel),
		agentEndpoints: newAgentEndpointMap(),
	}

	if tunnelComment != nil {
		commentJson, err := json.Marshal(tunnelComment)
		if err != nil {
			return nil, err
		}
		commentString := string(commentJson)
		if commentString != "{}" {
			comments = append(
				comments,
				string(commentString),
			)
		}
	}
	connOpts := []ngrok.ConnectOption{
		ngrok.WithClientInfo("ngrok-operator", version.GetVersion(), comments...),
		ngrok.WithAuthtokenFromEnv(),
		ngrok.WithLogger(k8sLogger{logger}),
		ngrok.WithRestartHandler(func(ctx context.Context, sess ngrok.Session) error {
			sessionState := td.session.Load()
			if sessionState != nil && sessionState.session != nil {
				sessionState.healthErr = fmt.Errorf("ngrok session restarting")
				td.session.Store(sessionState)
				logger.Info("ngrok session restarting")
			}
			return nil
		}),
	}

	if opts.Region != "" {
		connOpts = append(connOpts, ngrok.WithRegion(opts.Region))
	}

	if opts.ServerAddr != "" {
		connOpts = append(connOpts, ngrok.WithServer(opts.ServerAddr))
	}

	isHostCA := opts.RootCAs == "host"

	// validate is "trusted",  "" or "host
	if !isHostCA && opts.RootCAs != "trusted" && opts.RootCAs != "" {
		return nil, fmt.Errorf("invalid value for RootCAs: %s", opts.RootCAs)
	}

	// Configure certs if the custom cert directory exists or host if set
	if _, err := os.Stat(customCertsPath); !os.IsNotExist(err) || isHostCA {
		caCerts, err := caCerts(isHostCA)
		if err != nil {
			return nil, err
		}
		connOpts = append(connOpts, ngrok.WithCA(caCerts))
	}

	if isHostCA {
		connOpts = append(connOpts, ngrok.WithTLSConfig(func(c *tls.Config) {
			c.RootCAs = nil
		}))
	}

	td.session.Store(&sessionState{
		readyErr: fmt.Errorf("attempting to connect"),
	})
	connOpts = append(connOpts,
		ngrok.WithConnectHandler(func(ctx context.Context, sess ngrok.Session) {
			td.session.Store(&sessionState{
				session: sess,
			})
		}),
		ngrok.WithDisconnectHandler(func(ctx context.Context, sess ngrok.Session, err error) {
			state := td.session.Load()

			if state.session != nil {
				// we have established session in the past, so record err only when it is going away
				if err == nil {
					td.session.Store(&sessionState{
						healthErr: fmt.Errorf("session closed"),
					})
				}
				return
			}

			if err == nil {
				// session is disconnecting, do not override error
				if state.healthErr == nil {
					td.session.Store(&sessionState{
						healthErr: fmt.Errorf("session closed"),
					})
				}
				return
			}

			if state.healthErr != nil {
				// we are already at a terminal error, just keep the first one
				return
			}

			// we didn't have a session and we are seeing disconnect error
			userErr := strings.HasPrefix(err.Error(), "authentication failed") && !strings.Contains(err.Error(), "internal server error")
			if userErr {
				// its a user error (e.g. authentication failure), so stop further
				td.session.Store(&sessionState{
					healthErr: err,
				})
				sess.Close()
			} else {
				// mark this as connecting error to return from readyz
				td.session.Store(&sessionState{
					readyErr: err,
				})
			}
		}),
	)
	_, err := ngrok.Connect(ctx, connOpts...)

	return td, err
}

// Ready implements the healthcheck.HealthChecker interface for when the TunnelDriver is ready to serve tunnels
func (td *TunnelDriver) Ready(_ context.Context, _ *http.Request) error {
	state := td.session.Load()
	return state.readyErr
}

// Alive implements the healthcheck.HealthChecker interface for when the TunnelDriver is alive
func (td *TunnelDriver) Alive(_ context.Context, _ *http.Request) error {
	state := td.session.Load()
	return state.healthErr
}

func (td *TunnelDriver) getSession() (ngrok.Session, error) {
	state := td.session.Load()
	switch {
	case state.session != nil:
		return state.session, nil
	case state.healthErr != nil:
		return nil, state.healthErr
	case state.readyErr != nil:
		return nil, state.readyErr
	default:
		return nil, fmt.Errorf("unexpected state")
	}
}

// caCerts combines the system ca certs with a directory of custom ca certs
func caCerts(hostCA bool) (*x509.CertPool, error) {
	systemCertPool, err := x509.SystemCertPool()
	if err != nil {
		return nil, err
	}

	// we're all set if we're using the host CA
	if hostCA {
		return systemCertPool, nil
	}

	// Clone the system cert pool
	customCertPool := systemCertPool.Clone()

	// Read each .crt file in the custom cert directory
	files, err := os.ReadDir(customCertsPath)
	if err != nil {
		return nil, err
	}

	for _, file := range files {
		if filepath.Ext(file.Name()) != ".crt" {
			continue
		}

		// Read the contents of the .crt file
		certBytes, err := os.ReadFile(filepath.Join(customCertsPath, file.Name()))
		if err != nil {
			return nil, err
		}

		// Append the cert to the custom cert pool
		customCertPool.AppendCertsFromPEM(certBytes)
	}

	return customCertPool, nil
}

// CreateTunnel creates and starts a new tunnel in a goroutine. If a tunnel with the same name already exists,
// it will be stopped and replaced with a new tunnel unless the labels match.
func (td *TunnelDriver) CreateTunnel(ctx context.Context, name string, spec ingressv1alpha1.TunnelSpec) error {
	session, err := td.getSession()
	if err != nil {
		return err
	}

	log := log.FromContext(ctx)

	newAppProtocol := ""
	if spec.AppProtocol != nil {
		newAppProtocol = string(*spec.AppProtocol)
	}
	if tun, ok := td.tunnels[name]; ok {
		// Check if the tunnel matches the spec
		var currentAppProtocol string
		if fwdProto, ok := tun.(interface{ ForwardsProto() string }); ok {
			currentAppProtocol = fwdProto.ForwardsProto()
		}

		if maps.Equal(tun.Labels(), spec.Labels) && tun.ForwardsTo() == spec.ForwardsTo && currentAppProtocol == newAppProtocol {
			log.Info("Tunnel already exists and matches spec")
			return nil
		}
		// There is already a tunnel with this name, start the new one and defer closing the old one
		//nolint:errcheck
		defer td.stopTunnel(context.Background(), tun)
	}

	tun, err := session.Listen(ctx, td.buildTunnelConfig(spec.Labels, spec.ForwardsTo, newAppProtocol))
	if err != nil {
		return err
	}
	td.tunnels[name] = tun

	upstreamTLS := false
	if spec.BackendConfig != nil {
		// This is janky but the CRD just supports any random string here so we need to deal with the fact that is in the wild now
		switch strings.ToUpper(spec.BackendConfig.Protocol) {
		case "TLS", "HTTPS":
			upstreamTLS = true
		}
	}

	service, portStr, err := net.SplitHostPort(spec.ForwardsTo)
	if err != nil {
		return fmt.Errorf("invalid spec.forwardsTo (%q): %w", spec.ForwardsTo, err)
	}

	port, err := strconv.Atoi(portStr)
	if err != nil {
		return fmt.Errorf("invalid port for spec.forwardsTo (%q): %w", spec.ForwardsTo, err)
	}

	go handleTCPConnections(
		ctx,
		&net.Dialer{},
		tun,
		service,
		port,
		upstreamTLS,
		spec.AppProtocol,
		nil,
	)
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

// CreateAgentEndpoint will create or update an agent endpoint by name using the provided desired configuration state
func (td *TunnelDriver) CreateAgentEndpoint(ctx context.Context, name string, spec ngrokv1alpha1.AgentEndpointSpec, trafficPolicy string, clientCerts []tls.Certificate) error {
	log := log.FromContext(ctx).WithValues(
		"url", spec.Upstream.URL,
		"upstream.url", spec.Upstream.URL,
		"upstream.protocol", spec.Upstream.Protocol,
	)

	session, err := td.getSession()
	if err != nil {
		return err
	}

	tun, ok := td.agentEndpoints.Get(name)
	if ok {
		// TODO: Check if the tunnel matches the spec. If it does, do nothing.
		// If it doesn't, stop the old tunnel and start a new one.
		// For now, we just stop the old tunnel and always start a new one.

		//nolint:errcheck
		defer td.stopTunnel(context.Background(), tun)
	}

	upstreamURL, err := ParseAndSanitizeEndpointURL(spec.Upstream.URL, false)
	if err != nil {
		err := fmt.Errorf("error parsing spec.upstream.url: %w", err)
		log.Error(err, "upstream url parse failed")
		return err
	}

	ingressURL, err := ParseAndSanitizeEndpointURL(spec.URL, true)
	if err != nil {
		err := fmt.Errorf("error parsing spec.url: %w", err)
		log.Error(err, "url parse failed")
		return err
	}

	var tunnelConfig config.Tunnel
	commonOpts := []commonEndpointOption{
		config.WithURL(spec.URL),
		config.WithForwardsTo(spec.Upstream.URL),
		config.WithBindings(spec.Bindings...),
		config.WithMetadata(spec.Metadata),
		// TODO(stacks): this may end up being configurable on a per-endpoint basis in the future
		config.WithPoolingEnabled(true),
		config.WithDescription(spec.Description),
	}

	if trafficPolicy != "" {
		commonOpts = append(commonOpts, config.WithTrafficPolicy(trafficPolicy))
	}

	// Build the endpoint/tunnel config
	switch ingressURL.Scheme {
	case "https":
		fallthrough
	case "http":
		opts := []config.HTTPEndpointOption{}
		for _, o := range commonOpts {
			opts = append(opts, o)
		}
		// Default upstream protocol to HTTP1 if not configured
		upstreamProto := string(commonv1alpha1.ApplicationProtocol_HTTP1)
		if spec.Upstream.Protocol != nil {
			upstreamProto = string(*spec.Upstream.Protocol)
		}
		opts = append(opts, config.WithAppProtocol(upstreamProto))

		// TODO: This should probably be inferred from the scheme in the URL
		if ingressURL.Scheme == "http" {
			opts = append(opts, config.WithScheme(config.SchemeHTTP))
		}

		tunnelConfig = config.HTTPEndpoint(opts...)
	case "tls":
		opts := []config.TLSEndpointOption{}
		for _, o := range commonOpts {
			opts = append(opts, o)
		}
		tunnelConfig = config.TLSEndpoint(opts...)
	case "tcp":
		opts := []config.TCPEndpointOption{}
		for _, o := range commonOpts {
			opts = append(opts, o)
		}
		tunnelConfig = config.TCPEndpoint(opts...)
	default:
		return fmt.Errorf("unsupported protocol for spec.url: %s", ingressURL.Scheme)
	}

	log.V(1).Info("Adding agent endpoint to ngrok session")
	tun, err = session.Listen(ctx, tunnelConfig)
	if err != nil {
		return err
	}
	td.agentEndpoints.Add(name, tun)

	upstreamPort, err := strconv.Atoi(upstreamURL.Port())
	if err != nil {
		// The port is already validated earlier but this is just to be safe on the Atoi call
		return fmt.Errorf("invalid spec.upstream.url port (%q): %w", upstreamURL.Port(), err)
	}

	upstreamTLS := false
	if upstreamURL.Scheme == "tls" || upstreamURL.Scheme == "https" {
		upstreamTLS = true
	}

	// Start forwarding connections
	go handleTCPConnections(
		ctx,
		&net.Dialer{},
		tun,
		upstreamURL.Hostname(),
		upstreamPort,
		upstreamTLS,
		spec.Upstream.Protocol,
		clientCerts,
	)
	return nil
}

func (td *TunnelDriver) DeleteAgentEndpoint(ctx context.Context, name string) error {
	log := log.FromContext(ctx).WithValues("name", name)

	tun, _ := td.agentEndpoints.Get(name)
	if tun == nil {
		log.Info("Agent Endpoint tunnel not found while trying to delete tunnel")
		return nil
	}

	err := td.stopTunnel(ctx, tun)
	if err != nil {
		return err
	}
	td.agentEndpoints.Delete(name)
	log.Info("Agent Endpoint tunnel deleted successfully")
	return nil
}

func (td *TunnelDriver) stopTunnel(ctx context.Context, tun ngrok.Tunnel) error {
	if tun == nil {
		return nil
	}
	return tun.CloseWithContext(ctx)
}

func (td *TunnelDriver) buildTunnelConfig(labels map[string]string, destination, appProtocol string) config.Tunnel {
	opts := []config.LabeledTunnelOption{}
	for key, value := range labels {
		opts = append(opts, config.WithLabel(key, value))
	}
	opts = append(opts, config.WithForwardsTo(destination))
	opts = append(opts, config.WithAppProtocol(appProtocol))
	return config.LabeledTunnel(opts...)
}

func handleTCPConnections(ctx context.Context, dialer Dialer, tun ngrok.Tunnel, upstreamHostname string, upstreamPort int, upstreamTLS bool, upstreamAppProto *commonv1alpha1.ApplicationProtocol, clientCerts []tls.Certificate) {
	logger := log.FromContext(ctx).WithValues("id", tun.ID(), "upstreamHostname", upstreamHostname, "upstreamPort", upstreamPort, "upstreamTLS", upstreamTLS)
	for {
		ngrokConnection, err := tun.Accept()
		if err != nil {
			logger.Error(err, "Error accepting connection")
			// Right now, this can only be "Tunnel closed" https://github.com/ngrok/ngrok-go/blob/e1d90c382/internal/tunnel/client/tunnel.go#L81-L89
			// Since that's terminal, that means we should give up on this loop to
			// ensure we don't leak a goroutine after a tunnel goes away.
			// Unfortunately, it's not an exported error, so we can't verify with
			// more certainty that's what's going on, but at the time of writing,
			// that should be true.
			return
		}
		connLogger := logger.WithValues("remoteAddr", ngrokConnection.RemoteAddr())
		connLogger.Info("Accepted connection")

		go func() {
			ctx := log.IntoContext(ctx, connLogger)
			err := handleTCPConn(ctx, dialer, ngrokConnection, upstreamHostname, upstreamPort, upstreamTLS, upstreamAppProto, clientCerts)
			if err == nil || errors.Is(err, net.ErrClosed) {
				connLogger.Info("Connection closed")
				return
			}

			connLogger.Error(err, "Error handling connection")
		}()
	}
}

func handleTCPConn(ctx context.Context, dialer Dialer, ngrokConnection net.Conn, upstreamHostname string, upstreamPort int, upstreamTLS bool, upstreamAppProto *commonv1alpha1.ApplicationProtocol, clientCerts []tls.Certificate) error {
	log := log.FromContext(ctx)
	contextDialStr := fmt.Sprintf("%s:%d", upstreamHostname, upstreamPort)
	upstreamConnection, err := dialer.DialContext(ctx, "tcp", contextDialStr)
	if err != nil {
		return err
	}

	if upstreamTLS {
		var nextProtos []string
		if upstreamAppProto != nil {
			switch *upstreamAppProto {
			case commonv1alpha1.ApplicationProtocol_HTTP2:
				nextProtos = []string{"h2", "http/1.1"}
			case commonv1alpha1.ApplicationProtocol_HTTP1:
				nextProtos = []string{"http/1.1"}
			}
		}

		tlsCfg := &tls.Config{
			ServerName:         upstreamHostname,
			InsecureSkipVerify: true,
			Renegotiation:      tls.RenegotiateFreelyAsClient,
			NextProtos:         nextProtos,
		}
		if len(clientCerts) > 0 {
			tlsCfg.Certificates = clientCerts
		}

		upstreamConnection = tls.Client(upstreamConnection, tlsCfg)
	}

	var g errgroup.Group

	// Start forwarding from ngrok to the upstream
	g.Go(func() error {
		defer func() {
			if err := upstreamConnection.Close(); err != nil {
				log.Info("Error closing connection to destination: %v", err)
			}
		}()

		_, err := io.Copy(upstreamConnection, ngrokConnection)
		return err
	})

	// Start forwarding from the upstream back to ngrok
	g.Go(func() error {
		defer func() {
			if err := ngrokConnection.Close(); err != nil {
				log.Info("Error closing connection from ngrok: %v", err)
			}
		}()

		_, err := io.Copy(ngrokConnection, upstreamConnection)
		return err
	})
	return g.Wait()
}
