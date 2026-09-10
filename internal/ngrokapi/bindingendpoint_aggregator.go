package ngrokapi

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strconv"

	"github.com/google/uuid"
	"github.com/ngrok/ngrok-api-go/v9"
	bindingsv1alpha1 "github.com/ngrok/ngrok-operator/api/bindings/v1alpha1"
	ctrl "sigs.k8s.io/controller-runtime"
)

var (
	defaultScheme = "https"
	defaultPort   = map[string]int32{
		"http":  80,
		"https": 443,
		"tls":   443,
	}
)

// AggregatedEndpoints maps a BoundEndpoint name to the BoundEndpoint the
// cluster should hold under that name.
type AggregatedEndpoints map[string]bindingsv1alpha1.BoundEndpoint

// AggregateBindingEndpoints turns the account's endpoints into the set of
// BoundEndpoints this cluster should project.
//
// An endpoint is projected only when it carries kubernetes.targets: the field
// is itself the signal to project, so every other endpoint on the account is
// skipped. The endpoint keeps its own URL, which is the dial identity
// (Spec.EndpointURL), and each target names one place to project it
// (Spec.Target). One endpoint with two targets therefore produces two
// BoundEndpoints, and so two forwarder ports and two pairs of Services.
//
// Endpoints whose URL cannot be parsed, and targets that are malformed or that
// repeat a (service, namespace) pair, are skipped and logged. Their errors are
// joined into the returned error so a caller can surface them without losing
// the valid endpoints from the same batch.
func AggregateBindingEndpoints(ctx context.Context, endpoints []ngrok.Endpoint) (AggregatedEndpoints, error) {
	log := ctrl.LoggerFrom(ctx)
	aggregated := make(AggregatedEndpoints)
	var parseErrs []error

	for _, endpoint := range endpoints {
		if endpoint.Kubernetes == nil || len(endpoint.Kubernetes.Targets) == 0 {
			continue
		}

		parsed, err := parseDialURL(endpoint)
		if err != nil {
			wrapped := fmt.Errorf("failed to parse endpoint: %s: %w", endpoint.ID, err)
			log.Error(wrapped, "Skipping endpoint with an unparseable url", "id", endpoint.ID, "url", endpoint.URL, "proto", endpoint.Proto)
			parseErrs = append(parseErrs, wrapped)
			continue
		}

		seen := make(map[string]struct{}, len(endpoint.Kubernetes.Targets))
		for i, target := range endpoint.Kubernetes.Targets {
			if err := validateTarget(target); err != nil {
				wrapped := fmt.Errorf("failed to project endpoint %s target %d: %w", endpoint.ID, i, err)
				log.Error(wrapped, "Skipping invalid kubernetes target", "id", endpoint.ID, "target", target)
				parseErrs = append(parseErrs, wrapped)
				continue
			}

			// The API rejects a repeated pair, but an older control plane may
			// not, and two BoundEndpoints fighting over one Service is worse
			// than one missing Service.
			projection := fmt.Sprintf("%s.%s", target.Service, target.Namespace)
			if _, duplicate := seen[projection]; duplicate {
				wrapped := fmt.Errorf("endpoint %s repeats kubernetes target %s", endpoint.ID, projection)
				log.Error(wrapped, "Skipping duplicate kubernetes target", "id", endpoint.ID, "target", projection)
				parseErrs = append(parseErrs, wrapped)
				continue
			}
			seen[projection] = struct{}{}

			name := BoundEndpointName(endpoint.ID, target.Service, target.Namespace)
			aggregated[name] = bindingsv1alpha1.BoundEndpoint{
				Name: name,
				Spec: bindingsv1alpha1.BoundEndpointSpec{
					EndpointURL: parsed.String(),
					Scheme:      parsed.Scheme,
					Target: bindingsv1alpha1.EndpointTarget{
						Service:   target.Service,
						Namespace: target.Namespace,
						Port:      target.Port,
						Protocol:  "TCP", // always tcp for now
					},
				},
				Status: bindingsv1alpha1.BoundEndpointStatus{
					Endpoints: []bindingsv1alpha1.BindingEndpoint{{
						Ref: ngrok.Ref{ID: endpoint.ID, URI: endpoint.URI},
					}},
				},
			}
		}
	}

	return aggregated, errors.Join(parseErrs...)
}

// BoundEndpointName is the BoundEndpoint name, and so the Upstream Service
// name, for one projection of one endpoint.
//
// The endpoint ID is part of the hash because an endpoint with several targets
// needs one BoundEndpoint per target: they each get their own forwarder port
// and their own Services, and they would collide under a name derived from the
// endpoint alone.
func BoundEndpointName(endpointID, service, namespace string) string {
	uid := uuid.NewSHA1(uuid.NameSpaceURL, fmt.Appendf(nil, "%s/%s.%s", endpointID, service, namespace))
	return "ngrok-" + uid.String()
}

// validateTarget rejects a target the operator cannot turn into a Service.
// The API validates the same rules, so a failure here means the endpoint was
// written by an older or a different control plane.
func validateTarget(target ngrok.EndpointKubernetesTarget) error {
	if target.Service == "" {
		return errors.New("missing service")
	}
	if target.Namespace == "" {
		return errors.New("missing namespace")
	}
	if target.Port < 1 || target.Port > 65535 {
		return fmt.Errorf("port %d is out of the range 1-65535", target.Port)
	}
	return nil
}

// parsedHostport is a struct to hold the parsed bits
type parsedHostport struct {
	Scheme string
	Host   string
	Port   int32
}

// String prints the parsed hostport as an EndpointURL in the format: <scheme>://<host>:<port>
func (p *parsedHostport) String() string {
	return fmt.Sprintf("%s://%s:%d", p.Scheme, p.Host, p.Port)
}

// parseDialURL parses the endpoint's own url into the scheme, host and port the
// forwarder dials. This is the endpoint's identity on the ngrok side; where the
// endpoint is projected in the cluster comes from kubernetes.targets instead.
func parseDialURL(endpoint ngrok.Endpoint) (*parsedHostport, error) {
	rawURL := endpoint.URL
	if rawURL == "" {
		// Endpoints that predate the url field only carry public_url.
		rawURL = endpoint.PublicURL
	}
	if rawURL == "" {
		return nil, errors.New("missing url")
	}

	parsedURL, err := url.Parse(rawURL)
	if err != nil {
		return nil, err
	}

	proto := endpoint.Proto
	if parsedURL.Scheme == "" {
		// default scheme to https if not provided
		if proto == "" {
			proto = defaultScheme
		}

		// add the proto as the scheme to the URL
		// then reparse the URL so we get the correct Hostname()
		// this is to handle the case where the URL is missing the scheme
		// which is required for the URL to be parsed correctly
		fullURL := fmt.Sprintf("%s://%s", proto, rawURL)
		parsedURL, err = url.Parse(fullURL)
		if err != nil {
			return nil, fmt.Errorf("unable to parse with given proto: %s", fullURL)
		}
	} else if proto != "" && parsedURL.Scheme != proto {
		return nil, fmt.Errorf("mismatched scheme, expected %s: %s", proto, rawURL)
	}

	host := parsedURL.Hostname()
	if host == "" {
		return nil, fmt.Errorf("missing host: %s", rawURL)
	}

	// Parse the port if available, otherwise default based on the scheme.
	urlPort := parsedURL.Port()

	// extra check just in case
	if parsedURL.Scheme == "tcp" && urlPort == "" {
		return nil, fmt.Errorf("missing port for tcp scheme: %s", rawURL)
	}

	var port int32
	if urlPort != "" {
		parsedPort, err := strconv.Atoi(urlPort)
		if err != nil {
			return nil, fmt.Errorf("invalid port value: %s", urlPort)
		}
		port = int32(parsedPort)
	} else {
		port = defaultPort[parsedURL.Scheme]
		if port == 0 {
			return nil, fmt.Errorf("missing port and no default for scheme %s: %s", parsedURL.Scheme, rawURL)
		}
	}

	return &parsedHostport{
		Scheme: parsedURL.Scheme,
		Host:   host,
		Port:   port,
	}, nil
}
