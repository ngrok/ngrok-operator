package config

import (
	"net/url"
)

// OperatorConfig represents the unified configuration for all ngrok-operator components
type OperatorConfig struct {
	// Common logging settings
	Log LogConfig `mapstructure:"log"`

	// ngrok connection settings
	Region        string   `mapstructure:"region"`
	ServerAddr    string   `mapstructure:"serverAddr"`
	APIURL        *url.URL `mapstructure:"-"` // Parsed from apiURL string
	APIURLString  string   `mapstructure:"apiURL"` // Raw string from config
	RootCAs       string   `mapstructure:"rootCAs"`
	Description   string   `mapstructure:"description"`
	NgrokMetadata string   `mapstructure:"ngrokMetadata"`

	// Manager settings
	MetricsBindAddress     string `mapstructure:"metricsBindAddress"`
	HealthProbeBindAddress string `mapstructure:"healthProbeBindAddress"`
	ManagerName            string `mapstructure:"managerName"`
	ReleaseName            string `mapstructure:"releaseName"`

	// Runtime environment
	Namespace              string `mapstructure:"-"` // Not from ConfigMap, set from env

	// Feature flags
	EnableFeatureIngress          bool `mapstructure:"enableFeatureIngress"`
	EnableFeatureGateway          bool `mapstructure:"enableFeatureGateway"`
	EnableFeatureBindings         bool `mapstructure:"enableFeatureBindings"`
	DisableGatewayReferenceGrants bool `mapstructure:"disableGatewayReferenceGrants"`
	OneClickDemoMode              bool `mapstructure:"oneClickDemoMode"`

	// Component-specific configurations
	API      APIConfig      `mapstructure:"api"`
	Agent    AgentConfig    `mapstructure:"agent"`
	Bindings BindingsConfig `mapstructure:"bindings"`
}

// LogConfig represents logging configuration
type LogConfig struct {
	Level           string `mapstructure:"level"`
	Format          string `mapstructure:"format"`
	StacktraceLevel string `mapstructure:"stacktraceLevel"`
}

// APIConfig represents API manager specific configuration
type APIConfig struct {
	ElectionID                 string `mapstructure:"electionID"`
	IngressControllerName      string `mapstructure:"ingressControllerName"`
	IngressWatchNamespace      string `mapstructure:"ingressWatchNamespace"`
	DefaultDomainReclaimPolicy string `mapstructure:"defaultDomainReclaimPolicy"`
	ClusterDomain              string `mapstructure:"clusterDomain"`
}

// AgentConfig represents agent manager specific configuration
type AgentConfig struct {
	// Currently no agent-specific configs beyond common ones
}

// BindingsConfig represents bindings forwarder specific configuration
type BindingsConfig struct {
	EndpointSelectors  []string `mapstructure:"endpointSelectors"`
	ServiceAnnotations string   `mapstructure:"serviceAnnotations"`
	ServiceLabels      string   `mapstructure:"serviceLabels"`
	IngressEndpoint    string   `mapstructure:"ingressEndpoint"`
}

// NewDefaultConfig returns a new OperatorConfig with default values
func NewDefaultConfig() *OperatorConfig {
	return &OperatorConfig{
		// Helm values - no defaults here, trust values.yaml
		Log: LogConfig{}, // Trust Helm values
		Region:        "",
		ServerAddr:    "",
		APIURL:        nil,
		APIURLString:  "",
		RootCAs:       "", // Trust Helm values (comments say default "trusted")
		Description:   "", // Trust Helm values
		NgrokMetadata: "",
		ReleaseName:   "", // Set from Helm
		ManagerName:   "", // Set from Helm

		// Internal/computed defaults (not user-configurable via Helm)
		MetricsBindAddress:     ":8080",
		HealthProbeBindAddress: ":8081",

		// Runtime environment (from env vars)
		Namespace: "", // Set from POD_NAMESPACE env var

		// Feature flags - trust Helm values
		EnableFeatureIngress:          false,
		EnableFeatureGateway:          false,
		EnableFeatureBindings:         false,
		DisableGatewayReferenceGrants: false,
		OneClickDemoMode:              false,

		API: APIConfig{
			// Computed/internal values
			IngressControllerName: "k8s.ngrok.com/ingress-controller", // Internal constant
			// Helm values - no defaults
			ElectionID:                 "",
			IngressWatchNamespace:      "",
			DefaultDomainReclaimPolicy: "",
			ClusterDomain:              "",
		},
		
		Bindings: BindingsConfig{
			// Computed/internal values  
			IngressEndpoint: "kubernetes-binding-ingress.ngrok.io:443", // Internal default
			// Helm values - no defaults
			EndpointSelectors:  nil,
			ServiceAnnotations: "",
			ServiceLabels:      "",
		},
	}
}
