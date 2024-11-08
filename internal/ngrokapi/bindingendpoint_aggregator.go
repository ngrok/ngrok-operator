package ngrokapi

import (
	"fmt"
	"net/url"
	"strconv"
	"strings"

	v6 "github.com/ngrok/ngrok-api-go/v6"
	bindingsv1alpha1 "github.com/ngrok/ngrok-operator/api/bindings/v1alpha1"
)

var (
	defaultScheme = "https"
	defaultPort   = map[string]int32{
		"http":  80,
		"https": 443,
		"tls":   443,
	}
)

// AggregatedEndpoints is a map of hostport to BindingEndpoint (partially filled in)
type AggregatedEndpoints map[string]bindingsv1alpha1.BoundEndpoint

// AggregateBindingEndpoints aggregates the endpoints into a map of hostport to BindingEndpoint
// by parsing the hostport 4-tuple into each piece ([<scheme>://]<service>.<namespcace>[:<port>])
// and collecting together matching endpoints into a single BindingEndpoint
func AggregateBindingEndpoints(endpoints []v6.Endpoint) (AggregatedEndpoints, error) {
	aggregated := make(AggregatedEndpoints)

	for _, endpoint := range endpoints {
		parsed, err := parseHostport(endpoint.Proto, endpoint.PublicURL)
		if err != nil {
			return nil, fmt.Errorf("failed to parse endpoint: %s: %w", endpoint.ID, err)
		}

		endpointURI := parsed.String()

		// Create a new BindingEndpoint if one doesn't exist
		var bindingEndpoint bindingsv1alpha1.BoundEndpoint
		if val, ok := aggregated[endpointURI]; ok {
			bindingEndpoint = val
		} else {
			// newly found hostport, create a new BoundEndpoint
			bindingEndpoint = bindingsv1alpha1.BoundEndpoint{
				// parsed bits are shared across endpoints with the same hostport
				Spec: bindingsv1alpha1.BoundEndpointSpec{
					EndpointURI: endpointURI,
					Scheme:      parsed.Scheme,
					Target: bindingsv1alpha1.EndpointTarget{
						Service:   parsed.ServiceName,
						Namespace: parsed.Namespace,
						Port:      parsed.Port,
						Protocol:  "TCP", // always tcp for now
					},
				},
				Status: bindingsv1alpha1.BoundEndpointStatus{
					Endpoints: []bindingsv1alpha1.BindingEndpoint{},
				},
			}
		}

		// add the found endpoint to the list of endpoints
		bindingEndpoint.Status.Endpoints = append(bindingEndpoint.Status.Endpoints, bindingsv1alpha1.BindingEndpoint{
			Ref: v6.Ref{
				ID:  endpoint.ID,
				URI: endpoint.URI,
			},
		})

		// update the aggregated map
		aggregated[endpointURI] = bindingEndpoint
	}

	return aggregated, nil
}

// parsedHostport is a struct to hold the parsed bits
type parsedHostport struct {
	Scheme      string
	ServiceName string
	Namespace   string
	Port        int32
}

// String prints the parsed hostport as a EndpointURI in the format: <scheme>://<service>.<namespace>:<port>
func (p *parsedHostport) String() string {
	return fmt.Sprintf("%s://%s.%s:%d", p.Scheme, p.ServiceName, p.Namespace, p.Port)
}

// parseHostport parses the hostport from its 4-tuple into a struct
func parseHostport(proto string, publicURL string) (*parsedHostport, error) {
	if publicURL == "" {
		return nil, fmt.Errorf("missing publicURL")
	}

	// to be parsed and filled in
	var scheme string
	var serviceName string
	var namespace string
	var port int32

	parsedURL, err := url.Parse(publicURL)
	if err != nil {
		return nil, err
	}

	if parsedURL.Scheme == "" {
		// default scheme to https if not provided
		if proto == "" {
			proto = defaultScheme
		}

		// add the proto as the scheme to the URL
		// then reparse the URL so we get the correct Hostpath()
		// this is to handle the case where the URL is missing the scheme
		// which is required for the URL to be parsed correctly
		fullUrl := fmt.Sprintf("%s://%s", proto, publicURL)
		parsedURL, err = url.Parse(fullUrl)
		if err != nil {
			return nil, fmt.Errorf("unable to parse with given proto: %s", fullUrl)
		}
	} else {
		if proto != "" && parsedURL.Scheme != proto {
			return nil, fmt.Errorf("mismatched scheme, expected %s: %s", proto, publicURL)
		}
	}

	// set the scheme
	scheme = parsedURL.Scheme

	// Extract the service name and namespace from the URL's host part.
	// Format: <service-name>.<namespace-name>
	parts := strings.Split(parsedURL.Hostname(), ".")
	if len(parts) != 2 {
		return nil, fmt.Errorf("invalid hostname, expected <service-name>.<namespace-name>: %s", parsedURL.Hostname())
	} else {
		serviceName = parts[0]
		namespace = parts[1]
	}

	// Parse the port if available
	// default based on the scheme.
	urlPort := parsedURL.Port()

	// extra check just in case
	if parsedURL.Scheme == "tcp" && urlPort == "" {
		return nil, fmt.Errorf("missing port for tcp scheme: %s", publicURL)
	}

	if urlPort != "" {
		parsedPort, err := strconv.Atoi(urlPort)
		if err != nil {
			return nil, fmt.Errorf("invalid port value: %s", urlPort)
		}
		port = int32(parsedPort)
	} else {
		port = defaultPort[scheme]
	}

	return &parsedHostport{
		Scheme:      scheme,
		ServiceName: serviceName,
		Namespace:   namespace,
		Port:        port,
	}, nil
}
