package common

type ApplicationProtocol string

const (
	ApplicationProtocol_HTTP1 ApplicationProtocol = "http1"
	ApplicationProtocol_HTTP2 ApplicationProtocol = "http2"
)

func (t ApplicationProtocol) IsKnown() bool {
	switch t {
	case ApplicationProtocol_HTTP1, ApplicationProtocol_HTTP2:
		return true
	default:
		return false
	}
}

type ProxyProtocolVersion string

const (
	ProxyProtocolVersion_1 ProxyProtocolVersion = "1"
	ProxyProtocolVersion_2 ProxyProtocolVersion = "2"
)

func (t ProxyProtocolVersion) IsKnown() bool {
	switch t {
	case ProxyProtocolVersion_1, ProxyProtocolVersion_2:
		return true
	default:
		return false
	}
}

const (
	DefaultClusterDomain = "svc.cluster.local"
)
