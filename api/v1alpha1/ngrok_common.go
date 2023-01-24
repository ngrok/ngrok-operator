package v1alpha1

// common ngrok API/Dashboard fields
type ngrokAPICommon struct {
	// Description is a human-readable description of the object in the ngrok API/Dashboard
	// +kubebuilder:default:=`Created by ngrok-ingress-controller`
	Description string `json:"description,omitempty"`
	// Metadata is a string of arbitrary data associated with the object in the ngrok API/Dashboard
	// +kubebuilder:default:=`{"owned-by":"ngrok-ingress-controller"}`
	Metadata string `json:"metadata,omitempty"`
}

// Route Module Types

type EndpointCompression struct {
	// Enabled is whether or not to enable compression for this endpoint
	Enabled bool `json:"enabled,omitempty"`
}

type EndpointIPPolicy struct {
	IPPolicyIDs []string `json:"policyIDs,omitempty"`
}

// EndpointRequestHeaders is the configuration for a HTTPSEdgeRoute's request headers
// to be added or removed from the request before it is sent to the backend service.
type EndpointRequestHeaders struct {
	// a map of header key to header value that will be injected into the HTTP Request
	// before being sent to the upstream application server
	Add map[string]string `json:"add,omitempty"`
	// a list of header names that will be removed from the HTTP Request before being
	// sent to the upstream application server
	Remove []string `json:"remove,omitempty"`
}

// EndpointResponseHeaders is the configuration for a HTTPSEdgeRoute's response headers
// to be added or removed from the response before it is sent to the client.
type EndpointResponseHeaders struct {
	// a map of header key to header value that will be injected into the HTTP Response
	// returned to the HTTP client
	Add map[string]string `json:"add,omitempty"`
	// a list of header names that will be removed from the HTTP Response returned to
	// the HTTP client
	Remove []string `json:"remove,omitempty"`
}

type EndpointHeaders struct {
	// Request headers are the request headers module configuration or null
	Request *EndpointRequestHeaders `json:"request,omitempty"`
	// Response headers are the response headers module configuration or null
	Response *EndpointResponseHeaders `json:"response,omitempty"`
}
