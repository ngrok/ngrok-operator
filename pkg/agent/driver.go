package agent

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"time"

	"github.com/go-logr/logr"
	ngrokv1alpha1 "github.com/ngrok/ngrok-operator/api/ngrok/v1alpha1"
	"github.com/ngrok/ngrok-operator/internal/healthcheck"
	"github.com/ngrok/ngrok-operator/internal/util"
	"github.com/ngrok/ngrok-operator/internal/version"
	"golang.ngrok.com/ngrok/v2"
	"golang.ngrok.com/ngrok/v2/rpc"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

// EndpointResult contains information about the created endpoint
type EndpointResult struct {
	URL           string
	TrafficPolicy string
	Ready         bool
}

type Driver interface {
	// CreateAgentEndpoint creates or updates an agent endpoint by name using the provided desired configuration state.
	CreateAgentEndpoint(ctx context.Context, name string, spec ngrokv1alpha1.AgentEndpointSpec, trafficPolicy string, clientCerts []tls.Certificate) (*EndpointResult, error)

	// DeleteAgentEndpoint deletes an agent endpoint by name.
	DeleteAgentEndpoint(ctx context.Context, name string) error

	healthcheck.HealthChecker
}

type driverOpts struct {
	logger          logr.Logger
	agentConnectURL string
	agentConnectCAs string
	agentComments   []string
}

func defaultDriverOpts() *driverOpts {
	return &driverOpts{
		logger: logr.New(nil),
	}
}

// DriverOption is a functional option used to configure NewDriver.
type DriverOption func(*driverOpts)

// WithAgentConnectURL sets the server address that the underlying ngrok agent
// will connect to.
func WithAgentConnectURL(addr string) DriverOption {
	return func(opts *driverOpts) {
		opts.agentConnectURL = addr
	}
}

// WithAgentConnectCAs sets the root CAs that the underlying ngrok agent
// will use to verify the server's certificate.
func WithAgentConnectCAs(cas string) DriverOption {
	return func(opts *driverOpts) {
		opts.agentConnectCAs = cas
	}
}

// WithAgentComments sets the comments that will be added to the agent's
// client Info.
func WithAgentComments(comments ...string) DriverOption {
	return func(opts *driverOpts) {
		if opts.agentComments == nil {
			opts.agentComments = comments
			return
		}
		opts.agentComments = append(opts.agentComments, comments...)
	}
}

// WithLogger sets the logger for the underlying ngrok agent.
func WithLogger(logger logr.Logger) DriverOption {
	return func(opts *driverOpts) {
		opts.logger = logger
	}
}

// driver provides a higher level interface for interacting with the ngrok agent SDK.
// It abstracts the underlying SDK details and provides a more ergonomic API for
// use in controllers and other components that are primarily concerned with
// reconciliation logic rather than low-level SDK operations.
type driver struct {
	agent      ngrok.Agent
	forwarders *endpointForwarderMap
	healthcheck.HealthChecker
	done chan bool
}

// NewDriver creates a new Driver instance with the provided options.
// It initializes the underlying ngrok agent, endpoint forwarder map, sets up the
// health checker to receive updates from the agent, and starts the agent.
func NewDriver(driverOpts ...DriverOption) (Driver, error) {
	// default options
	opts := defaultDriverOpts()
	// apply user-provided options
	for _, opt := range driverOpts {
		opt(opts)
	}

	aliveChan := make(chan error, 1)
	readyChan := make(chan error, 1)
	logger := opts.logger

	d := &driver{
		done:          make(chan bool),
		forwarders:    newEndpointForwarderMap(),
		HealthChecker: healthcheck.NewChannelHealthChecker(readyChan, aliveChan),
	}

	// Initialize the agent as not ready until it connects
	readyChan <- errors.New("attempting to connect")

	agentOpts := []ngrok.AgentOption{
		ngrok.WithClientInfo("ngrok-operator", version.GetVersion(), opts.agentComments...),
		ngrok.WithAuthtoken(os.Getenv("NGROK_AUTHTOKEN")),
		ngrok.WithLogger(slog.New(logr.ToSlogHandler(logger))),
		ngrok.WithRPCHandler(func(_ context.Context, session ngrok.AgentSession, req rpc.Request) ([]byte, error) {
			switch req.Method() {
			case rpc.UpdateAgentMethod:
				// Since the version of the embedded agent is fixed based on the k8s manifest,
				// we don't support updating the agent.
				err := errors.New("UpdateAgentMethod is not supported for the ngrok operator. Please update the ngrok-operator instead.")
				return []byte(err.Error()), err
			case rpc.StopAgentMethod, rpc.RestartAgentMethod:
				// Stopping the agent and restarting the agent are the same thing
				// for our purposes since k8s will restart the agent pod if we stop it.
				// TODO: Maybe stopping the agent should have it disconnect from ngrok and just
				// sleep until the pod is restarted?
				go func() {
					<-time.After(5 * time.Second)
					if err := session.Agent().Disconnect(); err != nil {
						logger.Error(err, "error disconnecting ngrok agent after rpc request")
					}
				}()
				logger.Info("ngrok session stopping or restarting due to rpc request")
				aliveChan <- errors.New("ngrok session stopping or restarting")
				close(d.done) // Signal that the agent is stopping
				return []byte("ngrok session stopping or restarting"), nil
			default:
				// For any other method, we just return an error.
				err := fmt.Errorf("unsupported ngrok agent method: %s", req.Method())
				logger.Error(err, "unsupported ngrok agent method")
				return []byte(err.Error()), err
			}
		}),
		ngrok.WithEventHandler(func(e ngrok.Event) {
			switch v := e.(type) {
			case *ngrok.EventAgentHeartbeatReceived:
				logger.V(7).WithValues(
					"latency", v.Latency.String(),
				).Info("ngrok agent heartbeat received")

				// Don't block the channel
				select {
				case readyChan <- nil:
				default:
					logger.V(5).Info("ngrok agent heartbeat received, but ready channel is full")
				}

				select {
				case aliveChan <- nil:
				default:
					logger.V(5).Info("ngrok agent heartbeat received, but alive channel is full")
				}

			case *ngrok.EventAgentConnectSucceeded:
				logger.Info("ngrok agent connected")
				select {
				case readyChan <- nil:
				default:
					logger.V(5).Info("ngrok agent connected, but ready channel is full")
				}
			case *ngrok.EventAgentDisconnected:
				logger.Error(v.Error, "ngrok agent disconnected")
				err := v.Error

				select {
				case readyChan <- err:
				default:
					logger.V(5).Info("ngrok agent disconnected, but ready channel is full")
				}
			}
		}),
	}

	if opts.agentConnectURL != "" {
		agentOpts = append(agentOpts, ngrok.WithAgentConnectURL(opts.agentConnectURL))
	}

	isHostCA := opts.agentConnectCAs == "host"

	// validate is "trusted",  "" or "host
	if !isHostCA && opts.agentConnectCAs != "trusted" && opts.agentConnectCAs != "" {
		return nil, fmt.Errorf("invalid value for RootCAs: %s", opts.agentConnectCAs)
	}

	certPool, err := util.LoadCerts()
	if err != nil {
		return nil, err
	}
	agentOpts = append(agentOpts, ngrok.WithAgentConnectCAs(certPool))

	if isHostCA {
		agentOpts = append(agentOpts, ngrok.WithTLSConfig(func(c *tls.Config) {
			c.RootCAs = nil
		}))
	}

	agent, err := ngrok.NewAgent(agentOpts...)
	if err != nil {
		return nil, err
	}
	d.agent = agent
	return d, agent.Connect(context.Background())
}

// CreateAgentEndpoint will create or update an agent endpoint by name using the provided desired configuration state
func (d *driver) CreateAgentEndpoint(ctx context.Context, name string, spec ngrokv1alpha1.AgentEndpointSpec, trafficPolicy string, clientCerts []tls.Certificate) (*EndpointResult, error) {
	select {
	case <-d.done:
		return &EndpointResult{Ready: false}, fmt.Errorf("driver is shutting down")
	default:
		// continue
	}

	log := log.FromContext(ctx).WithValues(
		"url", spec.Upstream.URL,
		"upstream.url", spec.Upstream.URL,
		"upstream.protocol", spec.Upstream.Protocol,
	)

	epf, ok := d.forwarders.Get(name)
	if ok {
		// Check if the endpoint matches the spec. If it does, do nothing.
		// If it doesn't, stop the old endpoint and start a new one.
		// For now, we just stop the old endpoint and always start a new one.
		if !epf.PoolingEnabled() {
			// If pooling is not enabled, we have to stop the old endpoint before starting a new one.
			log.Info("Stopping existing agent endpoint", "id", epf.ID())
			if err := epf.CloseWithContext(ctx); err != nil {
				return &EndpointResult{Ready: false}, err
			}
		} else {
			defer epf.Close()
		}
	}

	upstream := buildUpstream(spec.Upstream, clientCerts)
	endpointOpts := []ngrok.EndpointOption{
		ngrok.WithURL(spec.URL),
		ngrok.WithBindings(spec.Bindings...),
		ngrok.WithMetadata(spec.Metadata),
		// TODO(stacks): This may end up being configurable on a per-endpoint basis in the future
		ngrok.WithPoolingEnabled(true),
		ngrok.WithDescription(spec.Description),
	}

	if trafficPolicy != "" {
		endpointOpts = append(endpointOpts, ngrok.WithTrafficPolicy(trafficPolicy))
	}

	epf, err := d.agent.Forward(context.Background(), upstream, endpointOpts...)
	if err != nil {
		return &EndpointResult{Ready: false}, err
	}

	log.WithValues(
		"id", epf.ID(),
		"url", epf.URL(),
		"bindings", epf.Bindings(),
		"poolingEnabled", epf.PoolingEnabled(),
		"trafficPolicy", epf.TrafficPolicy(),
		"metadata", epf.Metadata(),
		"proxyProtocol", epf.ProxyProtocol(),
		"upstream.URL", epf.UpstreamURL(),
		"upstream.Protocol", epf.UpstreamProtocol(),
		"description", spec.Description,
	).Info("Created agent endpoint")

	d.forwarders.Add(name, epf)

	result := &EndpointResult{
		URL:           epf.URL().String(),
		TrafficPolicy: epf.TrafficPolicy(),
		Ready:         true,
	}

	return result, nil
}

func (d *driver) DeleteAgentEndpoint(ctx context.Context, name string) error {
	log := log.FromContext(ctx).WithValues("name", name)

	epf, _ := d.forwarders.Get(name)
	if epf == nil {
		log.Info("AgentEndpoint not found while trying to delete endpoint")
		return nil
	}

	if err := epf.CloseWithContext(ctx); err != nil {
		log.Error(err, "Error closing agent endpoint")
		return err
	}

	d.forwarders.Delete(name)
	log.Info("AgentEndpoint deleted successfully")
	return nil
}

func buildUpstream(upstreamSpec ngrokv1alpha1.EndpointUpstream, clientCerts []tls.Certificate) *ngrok.Upstream {
	upstreamTLSConfig := buildUpstreamTLSConfig(clientCerts)
	upstreamOpts := []ngrok.UpstreamOption{
		ngrok.WithUpstreamTLSClientConfig(upstreamTLSConfig),
	}
	if upstreamSpec.Protocol != nil {
		upstreamOpts = append(upstreamOpts, ngrok.WithUpstreamProtocol(string(*upstreamSpec.Protocol)))
	}

	return ngrok.WithUpstream(upstreamSpec.URL, upstreamOpts...)
}

// Builds a TLS config for the agent endpoint based on the provided client
func buildUpstreamTLSConfig(clientCerts []tls.Certificate) *tls.Config {
	c := &tls.Config{
		// Legacy behavior to bridge the functionality of how this used to work with how ngrok-go
		// v2 works.
		InsecureSkipVerify: true,
	}

	if len(clientCerts) > 0 {
		c.Certificates = clientCerts
	}
	return c
}
