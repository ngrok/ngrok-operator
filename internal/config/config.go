// Package config defines the ngrok-operator configuration struct shared by
// every component, its built-in defaults, and loading from YAML config files.
//
// The struct is the single source of truth for default values. The Helm chart
// renders only the values a user actually overrides; anything absent falls back
// to Default(), so a value never has two defaults to keep in sync.
package config

import (
	common "github.com/ngrok/ngrok-operator/api/common/v1alpha1"
)

// Config is the full operator configuration. Every component decodes the same
// struct and ignores the fields it does not use.
type Config struct {
	Log      LogConfig      `json:"log"`
	Ngrok    NgrokConfig    `json:"ngrok"`
	Features FeaturesConfig `json:"features"`

	// Component-owned settings. The chart hands each component a config
	// document containing only the keys that component owns, so these sit at
	// the top level rather than in a per-component section. Names match their
	// flags: --one-click-demo-mode and --watch-namespace.
	OneClickDemoMode bool `json:"oneClickDemoMode"`

	// WatchNamespace limits which namespaces the agent reads AgentEndpoint
	// resources from. The api-manager's equivalent for Ingress resources is
	// Features.Ingress.WatchNamespace.
	WatchNamespace string `json:"watchNamespace"`
}

type LogConfig struct {
	Level           string `json:"level"`
	Format          string `json:"format"`
	StacktraceLevel string `json:"stacktraceLevel"`
}

// NgrokConfig is platform connection config shared by every component. These
// are deliberately not overridable per component: divergent values across
// components are a misconfiguration, not a use case.
type NgrokConfig struct {
	Description   string            `json:"description"`
	Region        string            `json:"region"`
	ServerAddr    string            `json:"serverAddr"`
	RootCAs       string            `json:"rootCAs"`
	APIURL        string            `json:"apiURL"`
	Metadata      map[string]string `json:"metadata"`
	ClusterDomain string            `json:"clusterDomain"`
}

type FeaturesConfig struct {
	Ingress  IngressFeature  `json:"ingress"`
	Gateway  GatewayFeature  `json:"gateway"`
	Bindings BindingsFeature `json:"bindings"`

	DefaultDomainReclaimPolicy string `json:"defaultDomainReclaimPolicy"`
	DrainPolicy                string `json:"drainPolicy"`
}

type IngressFeature struct {
	Enabled        bool   `json:"enabled"`
	ControllerName string `json:"controllerName"`
	WatchNamespace string `json:"watchNamespace"`
}

type GatewayFeature struct {
	Enabled                bool `json:"enabled"`
	DisableReferenceGrants bool `json:"disableReferenceGrants"`
}

type BindingsFeature struct {
	Enabled            bool              `json:"enabled"`
	EndpointSelectors  []string          `json:"endpointSelectors"`
	ServiceAnnotations map[string]string `json:"serviceAnnotations"`
	ServiceLabels      map[string]string `json:"serviceLabels"`
	IngressEndpoint    string            `json:"ingressEndpoint"`
}

// Default returns the built-in configuration. Callers get an independent copy;
// no backing arrays or maps are shared between calls.
func Default() *Config {
	return &Config{
		Log: LogConfig{
			Level:           "info",
			Format:          "json",
			StacktraceLevel: "error",
		},
		Ngrok: NgrokConfig{
			Description:   "The official ngrok Kubernetes Operator.",
			RootCAs:       "trusted",
			ClusterDomain: common.DefaultClusterDomain,
			Metadata:      map[string]string{},
		},
		Features: FeaturesConfig{
			Ingress: IngressFeature{
				Enabled:        true,
				ControllerName: "k8s.ngrok.com/ingress-controller",
			},
			Gateway: GatewayFeature{
				Enabled: true,
			},
			Bindings: BindingsFeature{
				EndpointSelectors:  []string{"true"},
				ServiceAnnotations: map[string]string{},
				ServiceLabels:      map[string]string{},
				IngressEndpoint:    "kubernetes-binding-ingress.ngrok.io:443",
			},
			DefaultDomainReclaimPolicy: "Delete",
			DrainPolicy:                "Retain",
		},
	}
}
