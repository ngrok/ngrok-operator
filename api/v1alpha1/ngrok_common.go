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
	Enabled *bool `json:"enabled,omitempty"`
}

type EndpointIPPolicy struct {
	Enabled     *bool    `json:"enabled,omitempty"`
	IPPolicyIDs []string `json:"policyIDs,omitempty"`
}
