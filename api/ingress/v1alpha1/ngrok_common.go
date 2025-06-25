package v1alpha1

import (
	"encoding/json"
)

// common ngrok API/Dashboard fields
type ngrokAPICommon struct {
	// Description is a human-readable description of the object in the ngrok API/Dashboard
	// +kubebuilder:default:=`Created by kubernetes-ingress-controller`
	Description string `json:"description,omitempty"`
	// Metadata is a string of arbitrary data associated with the object in the ngrok API/Dashboard
	// +kubebuilder:default:=`{"owned-by":"kubernetes-ingress-controller"}`
	Metadata string `json:"metadata,omitempty"`
}

type EndpointPolicy struct {
	// Determines if the rule will be applied to traffic
	Enabled *bool `json:"enabled,omitempty"`
	// Inbound traffic rule
	Inbound []EndpointRule `json:"inbound,omitempty"`
	// Outbound traffic rule
	Outbound []EndpointRule `json:"outbound,omitempty"`
}

type EndpointRule struct {
	// Expressions
	Expressions []string `json:"expressions,omitempty"`
	// Actions
	Actions []EndpointAction `json:"actions,omitempty"`
	// Name
	Name string `json:"name,omitempty"`
}

type EndpointAction struct {
	Type string `json:"type,omitempty"`
	// +kubebuilder:validation:Schemaless
	// +kubebuilder:pruning:PreserveUnknownFields
	// +kubebuilder:validation:Type=object
	Config json.RawMessage `json:"config,omitempty"`
}
