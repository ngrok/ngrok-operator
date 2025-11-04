package v1alpha1

import (
	"encoding/json"
)

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
