package conformance

import (
	"net"
	"strings"

	"sigs.k8s.io/gateway-api/conformance/utils/roundtripper"
)

// hostMapper maps the hostnames the conformance suite asks for onto
// hostnames the ngrok account can reserve.
//
// The suite sends requests for hosts such as example.com, which another ngrok
// account already holds, so the ngrok edge can never route them to us. The
// operator under test runs with the same suffix
// (ngrok.features.gateway.conformanceHostnameSuffix) and appends it to every
// hostname it reads from a Gateway or route. Appending keeps the relationships
// the suite tests intact: *.example.com still covers foo.example.com once both
// carry the suffix. The request still reaches a real ngrok endpoint for that
// host, through the edge's normal Host routing.
//
// On the way back, the suffix is removed from what the suite inspects (the
// Host the backend saw and the host of a redirect), so its expectations about
// example.com still hold.
type hostMapper struct {
	suffix string
}

func (m hostMapper) mapHost(hostport string) string {
	if m.suffix == "" || hostport == "" {
		return hostport
	}
	host, port, err := net.SplitHostPort(hostport)
	if err != nil {
		host, port = hostport, ""
	}
	if strings.HasSuffix(host, "."+m.suffix) || host == m.suffix || net.ParseIP(host) != nil {
		return hostport
	}
	host = host + "." + m.suffix
	if port != "" {
		return net.JoinHostPort(host, port)
	}
	return host
}

func (m hostMapper) unmapHost(hostport string) string {
	if m.suffix == "" || hostport == "" {
		return hostport
	}
	host, port, err := net.SplitHostPort(hostport)
	if err != nil {
		host, port = hostport, ""
	}
	host = strings.TrimSuffix(host, "."+m.suffix)
	if port != "" {
		return net.JoinHostPort(host, port)
	}
	return host
}

// mappingRoundTripper wraps the suite's DefaultRoundTripper with hostMapper.
type mappingRoundTripper struct {
	inner  roundtripper.RoundTripper
	mapper hostMapper
}

func (r mappingRoundTripper) CaptureRoundTrip(req roundtripper.Request) (*roundtripper.CapturedRequest, *roundtripper.CapturedResponse, error) {
	req.Host = r.mapper.mapHost(req.Host)

	cReq, cRes, err := r.inner.CaptureRoundTrip(req)
	if cReq != nil {
		cReq.Host = r.mapper.unmapHost(cReq.Host)
	}
	if cRes != nil && cRes.RedirectRequest != nil {
		cRes.RedirectRequest.Host = r.mapper.unmapHost(cRes.RedirectRequest.Host)
	}
	return cReq, cRes, err
}
