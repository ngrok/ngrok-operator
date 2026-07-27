package computeapi

import (
	"context"
	"crypto/tls"
	"fmt"
	"log/slog"
	"net/url"
	"os"
	"strings"

	"github.com/ngrok/ngrok-operator/internal/util"
	"github.com/ngrok/ngrok-operator/internal/version"
	"golang.ngrok.com/ngrok/v2"
)

// EndpointConfig configures the private ngrok listener used to serve the
// Compute Kubernetes API gateway.
type EndpointConfig struct {
	URLFile    string
	Authtoken  string
	ConnectURL string
	RootCAs    string
	Logger     *slog.Logger
}

// Listen creates an internal, pooled ngrok endpoint and returns its connections
// as a net.Listener. No local TCP listener is opened for proxied requests.
func Listen(ctx context.Context, cfg EndpointConfig) (ngrok.EndpointListener, error) {
	endpointURL, err := loadEndpointURL(cfg.URLFile)
	if err != nil {
		return nil, err
	}

	agentOpts := []ngrok.AgentOption{
		ngrok.WithClientInfo("ngrok-operator-compute-api", version.GetVersion()),
		ngrok.WithAuthtoken(cfg.Authtoken),
	}
	if cfg.Logger != nil {
		agentOpts = append(agentOpts, ngrok.WithLogger(cfg.Logger))
	}
	if cfg.ConnectURL != "" {
		agentOpts = append(agentOpts, ngrok.WithAgentConnectURL(cfg.ConnectURL))
	}

	switch cfg.RootCAs {
	case "", "trusted":
		certPool, err := util.LoadCerts()
		if err != nil {
			return nil, fmt.Errorf("load ngrok agent CAs: %w", err)
		}
		agentOpts = append(agentOpts, ngrok.WithAgentConnectCAs(certPool))
	case "host":
		agentOpts = append(agentOpts, ngrok.WithTLSConfig(func(c *tls.Config) {
			c.RootCAs = nil
		}))
	default:
		return nil, fmt.Errorf("invalid root CAs value %q: expected trusted or host", cfg.RootCAs)
	}

	agent, err := ngrok.NewAgent(agentOpts...)
	if err != nil {
		return nil, fmt.Errorf("create ngrok agent: %w", err)
	}
	listener, err := agent.Listen(ctx,
		ngrok.WithURL(endpointURL),
		ngrok.WithBindings("internal"),
		ngrok.WithPoolingEnabled(true),
		ngrok.WithDescription("Compute Kubernetes API"),
		ngrok.WithMetadata(`{"owned-by":"ngrok-operator","component":"compute-api"}`),
	)
	if err != nil {
		_ = agent.Disconnect()
		return nil, fmt.Errorf("listen on internal ngrok endpoint: %w", err)
	}
	return listener, nil
}

func loadEndpointURL(path string) (string, error) {
	endpointBytes, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("read endpoint URL: %w", err)
	}
	endpointURL := strings.TrimSpace(string(endpointBytes))
	parsedURL, err := url.Parse(endpointURL)
	if err != nil {
		return "", fmt.Errorf("parse endpoint URL: %w", err)
	}
	if parsedURL.Scheme != "https" ||
		parsedURL.Hostname() == "" ||
		!strings.HasSuffix(parsedURL.Hostname(), ".internal") ||
		(parsedURL.Path != "" && parsedURL.Path != "/") ||
		parsedURL.RawQuery != "" ||
		parsedURL.Fragment != "" ||
		parsedURL.User != nil {
		return "", fmt.Errorf("endpoint URL must be an https:// URL with a .internal hostname")
	}
	return endpointURL, nil
}
