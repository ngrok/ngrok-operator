package config

import "github.com/spf13/pflag"

// Each helper registers one subtree of Config against the flag set, using the
// loaded values as the flag defaults so an explicitly passed flag beats the
// config file. A component calls the helpers whose subtree its rendered config
// carries; registering these per component would put three copies of every
// flag name, usage string and default in the tree.

// RegisterLogFlags registers the log.* settings. Every component logs.
func RegisterLogFlags(fs *pflag.FlagSet, cfg *Config) {
	fs.StringVar(&cfg.Log.Level, "log-level", cfg.Log.Level, "Log level: debug, info, warn, or error")
	fs.StringVar(&cfg.Log.Format, "log-format", cfg.Log.Format, "Log format: json or console")
	fs.StringVar(&cfg.Log.StacktraceLevel, "log-stacktrace-level", cfg.Log.StacktraceLevel, "Level at which to emit stacktraces: info or error")
}

// RegisterNgrokFlags registers the ngrok.* platform settings. Every component
// connects to the ngrok platform and is handed the whole block by the chart.
func RegisterNgrokFlags(fs *pflag.FlagSet, cfg *Config) {
	fs.StringVar(&cfg.Ngrok.Description, "description", cfg.Ngrok.Description, "Description for this installation")
	fs.StringVar(&cfg.Ngrok.Region, "region", cfg.Ngrok.Region, "The region to use for ngrok tunnels")
	fs.StringVar(&cfg.Ngrok.ServerAddr, "server-addr", cfg.Ngrok.ServerAddr, "The address of the ngrok server to use for tunnels")
	fs.StringVar(&cfg.Ngrok.RootCAs, "root-cas", cfg.Ngrok.RootCAs, "trusted or host: use the trusted ngrok agent CA or the host CA")
	fs.StringVar(&cfg.Ngrok.APIURL, "api-url", cfg.Ngrok.APIURL, "The base URL to use for the ngrok api")
	fs.StringVar(&cfg.Ngrok.ClusterDomain, "cluster-domain", cfg.Ngrok.ClusterDomain, "Cluster domain used in the cluster")
	fs.StringToStringVar(&cfg.Ngrok.Metadata, "ngrok-metadata", cfg.Ngrok.Metadata, "Key=value pairs added as metadata to the ngrok api resources the operator creates")
}

// RegisterFeatureFlags registers the features.* settings.
func RegisterFeatureFlags(fs *pflag.FlagSet, cfg *Config) {
	fs.BoolVar(&cfg.Features.Ingress.Enabled, "enable-feature-ingress", cfg.Features.Ingress.Enabled, "Enables the Ingress controller")
	fs.StringVar(&cfg.Features.Ingress.ControllerName, "ingress-controller-name", cfg.Features.Ingress.ControllerName, "The name of the controller to use for matching ingresses classes")
	fs.StringVar(&cfg.Features.Ingress.WatchNamespace, "ingress-watch-namespace", cfg.Features.Ingress.WatchNamespace, "Namespace to watch for Kubernetes Ingress resources. Defaults to all namespaces.")

	fs.BoolVar(&cfg.Features.Gateway.Enabled, "enable-feature-gateway", cfg.Features.Gateway.Enabled, "When true, enables support for Gateway API if the CRDs are detected")
	fs.BoolVar(&cfg.Features.Gateway.DisableReferenceGrants, "disable-reference-grants", cfg.Features.Gateway.DisableReferenceGrants, "Opts-out of requiring ReferenceGrants for cross namespace references in Gateway API config")

	fs.BoolVar(&cfg.Features.Bindings.Enabled, "enable-feature-bindings", cfg.Features.Bindings.Enabled, "Enables the Endpoint Bindings controller")
	fs.StringSliceVar(&cfg.Features.Bindings.EndpointSelectors, "bindings-endpoint-selectors", cfg.Features.Bindings.EndpointSelectors, "Endpoint Selectors for Endpoint Bindings")
	fs.StringToStringVar(&cfg.Features.Bindings.ServiceAnnotations, "bindings-service-annotations", cfg.Features.Bindings.ServiceAnnotations, "Service annotations to propagate to the target service")
	fs.StringToStringVar(&cfg.Features.Bindings.ServiceLabels, "bindings-service-labels", cfg.Features.Bindings.ServiceLabels, "Service labels to propagate to the target service")
	fs.StringVar(&cfg.Features.Bindings.IngressEndpoint, "bindings-ingress-endpoint", cfg.Features.Bindings.IngressEndpoint, "The endpoint the bindings forwarder connects to")

	fs.StringVar(&cfg.Features.DefaultDomainReclaimPolicy, "default-domain-reclaim-policy", cfg.Features.DefaultDomainReclaimPolicy, "The default domain reclaim policy to apply to created domains")
	fs.StringVar(&cfg.Features.DrainPolicy, "drain-policy", cfg.Features.DrainPolicy, "Policy for draining resources during uninstall: Delete or Retain")
}
