// Package config defines the configuration shared by every ngrok-operator
// component, its built-in defaults, and how it is loaded.
//
// Default() is the only place a default value is written. Loading layers
// sources over it, highest precedence first: CLI flag, NGROK_OPERATOR_*
// environment variable, --config file, Default().
package config

import (
	"encoding/json"

	common "github.com/ngrok/ngrok-operator/api/common/v1alpha1"
)

// Config is the full operator configuration. Every component decodes the same
// struct, registers flags only for the fields it reads, and ignores the rest.
type Config struct {
	Log      LogConfig      `json:"log"`
	Ngrok    NgrokConfig    `json:"ngrok"`
	Features FeaturesConfig `json:"features"`

	// Settings that belong to one component alone.
	APIManager APIManagerConfig `json:"apiManager"`
}

type APIManagerConfig struct {
	// OneClickDemoMode starts the api-manager without credentials: it becomes
	// Ready and logs what is missing instead of reconciling. For marketplace
	// installs, where users cannot supply configuration before the first start.
	OneClickDemoMode bool `json:"oneClickDemoMode"`
}

// LogConfig feeds controller-runtime's --zap-* flags. An empty field leaves the
// zap default in place.
type LogConfig struct {
	Level           FlagString `json:"level"`
	Format          FlagString `json:"format"`
	StacktraceLevel FlagString `json:"stacktraceLevel"`
}

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

// FlagString is a string that also decodes from a YAML number, so a numeric
// zap level such as `level: 8` reads the same as `--zap-log-level=8`.
type FlagString string

func (s *FlagString) UnmarshalJSON(data []byte) error {
	var n json.Number
	if err := json.Unmarshal(data, &n); err == nil {
		*s = FlagString(n)
		return nil
	}
	var str string
	if err := json.Unmarshal(data, &str); err != nil {
		return err
	}
	*s = FlagString(str)
	return nil
}

// Default returns the built-in configuration. Callers get an independent copy;
// no backing arrays or maps are shared between calls.
func Default() *Config {
	return &Config{
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
