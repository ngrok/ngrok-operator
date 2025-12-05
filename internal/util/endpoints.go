package util

import (
	"fmt"
	"net"
	"net/url"
	"strconv"
	"strings"
)

// ParseAndSanitizeEndpointURL parses/sanitizes an input string for an endpoint url and provides a *url.URL following the restrictions for endpoints.
// when isIngressURL is true, the input string does not require a port (excluding tcp addresses)
func ParseAndSanitizeEndpointURL(input string, isIngressURL bool) (*url.URL, error) {
	// Handle shorthand port format, ex: "8080"
	if _, err := strconv.Atoi(input); err == nil {
		// Port shorthand defaults to localhost and http scheme
		return &url.URL{
			Scheme: "http",
			Host:   net.JoinHostPort("localhost", input),
		}, nil
	}

	// Check if the input contains a colon but no scheme (e.g., "service.default:8080")
	if strings.Contains(input, ":") && !strings.Contains(input, "://") {
		// Default to HTTP scheme
		input = "http://" + input
	}

	// Parse the input as a URL
	parsedURL, err := url.Parse(input)
	if err != nil {
		return nil, fmt.Errorf("failed to parse URL: %w", err)
	}

	// No Scheme (domain shorthand), ex: "example.com"
	if parsedURL.Scheme == "" {
		parsedURL.Scheme = "http"
		// Check if Host is set, and assign it if empty
		if parsedURL.Host == "" {
			// Assume the input itself is the hostname
			parsedURL.Host = input
		}
		// Assign default port if no port is present
		if parsedURL.Port() == "" {
			parsedURL.Host = net.JoinHostPort(parsedURL.Hostname(), "80")
		}
		// Clear the Path to avoid appending the hostname as a path
		parsedURL.Path = ""
		return parsedURL, nil
	}

	// Handle unsupported schemes
	switch parsedURL.Scheme {
	case "http", "https", "tcp", "tls":
		// Do nothing because these schemes are supported
	default:
		return nil, fmt.Errorf("unsupported scheme for URL (%q): %q", input, parsedURL.Scheme)
	}

	// Handle Scheme shorthand format, ex: "https://", "http://", "tcp://", "tls://"
	if parsedURL.Host == "" {
		switch parsedURL.Scheme {
		case "http":
			parsedURL.Host = "localhost:80"
		case "https":
			parsedURL.Host = "localhost:443"
		case "tcp", "tls":
			return nil, fmt.Errorf("invalid URL for scheme shorthand format (%q): \"tcp://\" and \"tls://\" must provide a hostname", input)
		}
		return parsedURL, nil
	}

	if parsedURL.Hostname() == "" {
		return nil, fmt.Errorf("invalid URL (%q), shorthand format not detected and URL is missing a hostname", input)
	}

	// Auto infer port when empty or error for tcp schemes without a port
	if parsedURL.Port() == "" {
		switch parsedURL.Scheme {
		// Default port inference for HTTP/S
		case "http":
			parsedURL.Host = net.JoinHostPort(parsedURL.Hostname(), "80")
		case "https", "tls":
			parsedURL.Host = net.JoinHostPort(parsedURL.Hostname(), "443")
		case "tcp":
			return nil, fmt.Errorf("invalid URL (%q), tcp schemes require a port and a hostname", input)
		}
	} else if parsedURL.Scheme == "tls" && isIngressURL && parsedURL.Port() != "443" {
		return nil, fmt.Errorf("invalid url %q, tls:// scheme ingress urls only support port 443 for accepting incoming traffic", input)
	}

	return parsedURL, nil
}

// ParseEndpointHostPort parses an endpoint URL and returns the hostname and port.
// Returns an error if the URL cannot be parsed or has no hostname.
func ParseEndpointHostPort(input string) (hostname string, port int32, err error) {
	parsedURL, err := ParseAndSanitizeEndpointURL(input, true)
	if err != nil {
		return "", 0, err
	}

	hostname = parsedURL.Hostname()
	if hostname == "" {
		return "", 0, fmt.Errorf("URL %q has no hostname", input)
	}

	if p := parsedURL.Port(); p != "" {
		portInt, err := strconv.ParseInt(p, 10, 32)
		if err != nil {
			return "", 0, fmt.Errorf("invalid port in URL %q: %w", input, err)
		}
		port = int32(portInt)
	}

	return hostname, port, nil
}
