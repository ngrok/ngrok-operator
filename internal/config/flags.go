package config

import (
	"errors"
	"flag"
	"fmt"

	"github.com/spf13/pflag"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
)

// RegisterFlags registers every shared app config flag against cfg, using the
// values cfg already holds as the flag defaults. Called after the config files
// are loaded, this is what makes an explicitly passed flag beat the file with
// no per-flag merge logic.
//
// Logging keeps controller-runtime's own --zap-* flags. The log section of the
// config file seeds them, and the returned options hold the result once the
// flags are parsed. An invalid log value is returned as an error after every
// flag is registered, so the caller can still parse the command line.
func RegisterFlags(fs *pflag.FlagSet, cfg *Config) (*zap.Options, error) {
	fs.StringVar(&cfg.Ngrok.Description, "description", cfg.Ngrok.Description, "Description for this installation")
	fs.StringVar(&cfg.Ngrok.Region, "region", cfg.Ngrok.Region, "The region to use for ngrok tunnels")
	fs.StringVar(&cfg.Ngrok.ServerAddr, "server-addr", cfg.Ngrok.ServerAddr, "The address of the ngrok server to use for tunnels")
	fs.StringVar(&cfg.Ngrok.RootCAs, "root-cas", cfg.Ngrok.RootCAs, "trusted or host: use the trusted ngrok agent CA or the host CA")
	fs.StringVar(&cfg.Ngrok.APIURL, "api-url", cfg.Ngrok.APIURL, "The base URL to use for the ngrok api")
	fs.StringVar(&cfg.Ngrok.ClusterDomain, "cluster-domain", cfg.Ngrok.ClusterDomain, "Cluster domain used in the cluster")
	fs.StringToStringVar(&cfg.Ngrok.Metadata, "ngrok-metadata", cfg.Ngrok.Metadata, "Key=value pairs added as metadata to the ngrok api resources the operator creates")

	fs.BoolVar(&cfg.Features.Ingress.Enabled, "enable-feature-ingress", cfg.Features.Ingress.Enabled, "Enables the Ingress controller")
	fs.StringVar(&cfg.Features.Ingress.ControllerName, "ingress-controller-name", cfg.Features.Ingress.ControllerName, "The name of the controller to use for matching ingresses classes")
	fs.StringVar(&cfg.Features.Ingress.WatchNamespace, "ingress-watch-namespace", cfg.Features.Ingress.WatchNamespace, "Namespace to watch for Kubernetes Ingress and AgentEndpoint resources. Defaults to all namespaces.")

	fs.BoolVar(&cfg.Features.Gateway.Enabled, "enable-feature-gateway", cfg.Features.Gateway.Enabled, "When true, enables support for Gateway API if the CRDs are detected")
	fs.BoolVar(&cfg.Features.Gateway.DisableReferenceGrants, "disable-reference-grants", cfg.Features.Gateway.DisableReferenceGrants, "Opts-out of requiring ReferenceGrants for cross namespace references in Gateway API config")

	fs.BoolVar(&cfg.Features.Bindings.Enabled, "enable-feature-bindings", cfg.Features.Bindings.Enabled, "Enables the Endpoint Bindings controller")
	fs.StringSliceVar(&cfg.Features.Bindings.EndpointSelectors, "bindings-endpoint-selectors", cfg.Features.Bindings.EndpointSelectors, "Endpoint Selectors for Endpoint Bindings")
	fs.StringToStringVar(&cfg.Features.Bindings.ServiceAnnotations, "bindings-service-annotations", cfg.Features.Bindings.ServiceAnnotations, "Service annotations to propagate to the target service")
	fs.StringToStringVar(&cfg.Features.Bindings.ServiceLabels, "bindings-service-labels", cfg.Features.Bindings.ServiceLabels, "Service labels to propagate to the target service")
	fs.StringVar(&cfg.Features.Bindings.IngressEndpoint, "bindings-ingress-endpoint", cfg.Features.Bindings.IngressEndpoint, "The endpoint the bindings forwarder connects to")

	fs.StringVar(&cfg.Features.DefaultDomainReclaimPolicy, "default-domain-reclaim-policy", cfg.Features.DefaultDomainReclaimPolicy, "The default domain reclaim policy to apply to created domains")
	fs.StringVar(&cfg.Features.DrainPolicy, "drain-policy", cfg.Features.DrainPolicy, "Policy for draining resources during uninstall: Delete or Retain")

	zapOpts := &zap.Options{}
	goFlags := flag.NewFlagSet("zap", flag.ContinueOnError)
	zapOpts.BindFlags(goFlags)

	var errs []error
	for name, value := range map[string]FlagString{
		"zap-log-level":        cfg.Log.Level,
		"zap-encoder":          cfg.Log.Format,
		"zap-stacktrace-level": cfg.Log.StacktraceLevel,
	} {
		if value == "" {
			continue
		}
		if err := goFlags.Set(name, string(value)); err != nil {
			errs = append(errs, fmt.Errorf("config log value for --%s: %w", name, err))
		}
	}
	fs.AddGoFlagSet(goFlags)

	return zapOpts, errors.Join(errs...)
}
