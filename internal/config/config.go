// Package config defines the configuration shared by every ngrok-operator
// component, its built-in defaults, and how it is loaded.
//
// Default() is the only place a default value is written. Load applies one
// NGROK_OPERATOR_* environment variable per setting over it.
package config

import (
	common "github.com/ngrok/ngrok-operator/api/common/v1alpha1"
)

// Config is the full operator configuration. Every component loads the same
// struct and ignores the fields it does not use.
//
// Each setting carries three tags:
//
//   - json names it: the key in the chart's values, and through EnvName and
//     FlagName its environment variable and flag.
//   - components lists the components that read it. Only they get its flag.
//   - help describes it, for the flag's usage text.
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
	OneClickDemoMode bool `json:"oneClickDemoMode" components:"apiManager" help:"Start without credentials and become Ready without reconciling"`
}

// LogConfig feeds controller-runtime's --zap-* flags. An empty field leaves the
// zap default in place.
type LogConfig struct {
	Level           string `json:"level" components:"apiManager,agent,bindingsForwarder" help:"Log level: debug, info, error, panic, or an integer for more verbose debug levels. Seeds --zap-log-level"`
	Format          string `json:"format" components:"apiManager,agent,bindingsForwarder" help:"Log format: json or console. Seeds --zap-encoder"`
	StacktraceLevel string `json:"stacktraceLevel" components:"apiManager,agent,bindingsForwarder" help:"Level at and above which stacktraces are captured: info, error or panic. Seeds --zap-stacktrace-level"`
}

type NgrokConfig struct {
	Description   string            `json:"description" components:"apiManager" help:"Description of this installation in the ngrok dashboard"`
	Region        string            `json:"region" components:"apiManager" help:"ngrok region to use"`
	ServerAddr    string            `json:"serverAddr" components:"agent" help:"Address of the ngrok server to use for tunnels"`
	RootCAs       string            `json:"rootCAs" components:"agent" help:"Root CAs to trust: trusted for the ngrok CA, host for the host CA bundle"`
	APIURL        string            `json:"apiURL" components:"apiManager" help:"Base URL for the ngrok API"`
	Metadata      map[string]string `json:"metadata" components:"apiManager" help:"Metadata added to the ngrok API resources the operator creates"`
	ClusterDomain string            `json:"clusterDomain" components:"apiManager" help:"Cluster domain used when resolving in-cluster service addresses"`
}

type FeaturesConfig struct {
	Ingress  IngressFeature  `json:"ingress"`
	Gateway  GatewayFeature  `json:"gateway"`
	Bindings BindingsFeature `json:"bindings"`

	DefaultDomainReclaimPolicy string `json:"defaultDomainReclaimPolicy" components:"apiManager,agent" help:"Default reclaim policy for Domains: Delete or Retain"`
	DrainPolicy                string `json:"drainPolicy" components:"apiManager" help:"What to do with ngrok API resources on uninstall: Delete or Retain"`
}

type IngressFeature struct {
	Enabled        bool   `json:"enabled" components:"apiManager" help:"Enable the Kubernetes Ingress controller"`
	ControllerName string `json:"controllerName" components:"apiManager" help:"Controller name matched by IngressClasses"`
	WatchNamespace string `json:"watchNamespace" components:"apiManager,agent" help:"Namespace to watch for Ingress and AgentEndpoint resources. Empty watches all namespaces"`
}

type GatewayFeature struct {
	Enabled                bool `json:"enabled" components:"apiManager,agent" help:"Enable Gateway API support, if the Gateway API CRDs are detected"`
	DisableReferenceGrants bool `json:"disableReferenceGrants" components:"apiManager" help:"Disable the ReferenceGrant requirement for cross-namespace references"`
}

type BindingsFeature struct {
	Enabled            bool              `json:"enabled" components:"apiManager" help:"Enable the Endpoint Bindings feature"`
	EndpointSelectors  []string          `json:"endpointSelectors" components:"apiManager" help:"CEL expressions filtering which endpoints are projected into this cluster"`
	ServiceAnnotations map[string]string `json:"serviceAnnotations" components:"apiManager" help:"Annotations applied to projected services"`
	ServiceLabels      map[string]string `json:"serviceLabels" components:"apiManager" help:"Labels applied to projected services"`
	IngressEndpoint    string            `json:"ingressEndpoint" components:"apiManager" help:"Hostname of the bindings ingress endpoint"`
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
