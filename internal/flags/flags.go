// Package flags defines every operator setting once: its flag name,
// environment variable, default and help. Commands bind the settings they
// read onto their own options.
//
// A setting's environment variable, when set, replaces its default, so the
// precedence is flag > environment variable > default. The Helm chart renders
// the same variable names from files/operator-env.yaml; a test in this package
// checks the two lists match.
package flags

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/pflag"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	common "github.com/ngrok/ngrok-operator/api/common/v1alpha1"
	ingressv1alpha1 "github.com/ngrok/ngrok-operator/api/ingress/v1alpha1"
	ngrokv1alpha1 "github.com/ngrok/ngrok-operator/api/ngrok/v1alpha1"
)

var (
	// ngrok
	NgrokDescription   = String("description", "NGROK_OPERATOR_NGROK_DESCRIPTION", "Created by the ngrok-operator", "Description for this installation")
	NgrokRegion        = String("region", "NGROK_OPERATOR_NGROK_REGION", "", "The region to use for ngrok tunnels")
	NgrokServerAddr    = String("server-addr", "NGROK_OPERATOR_NGROK_SERVER_ADDR", "", "The address of the ngrok server to use for tunnels")
	NgrokAPIURL        = String("api-url", "NGROK_OPERATOR_NGROK_API_URL", "", "The base URL to use for the ngrok api")
	NgrokRootCAs       = String("root-cas", "NGROK_OPERATOR_NGROK_ROOT_CAS", "trusted", "trusted (default) or host: use the trusted ngrok agent CA or the host CA")
	NgrokMetadata      = Map("ngrokMetadata", "NGROK_OPERATOR_NGROK_METADATA", "Metadata added to the ngrok API resources the operator creates, as a YAML or JSON map")
	NgrokClusterDomain = String("cluster-domain", "NGROK_OPERATOR_NGROK_CLUSTER_DOMAIN", common.DefaultClusterDomain, "Cluster domain used in the cluster")

	// features
	IngressEnabled                = Bool("enable-feature-ingress", "NGROK_OPERATOR_FEATURES_INGRESS_ENABLED", true, "Enables the Ingress controller")
	IngressControllerName         = String("ingress-controller-name", "NGROK_OPERATOR_FEATURES_INGRESS_CONTROLLER_NAME", "k8s.ngrok.com/ingress-controller", "The name of the controller to use for matching ingresses classes")
	IngressWatchNamespace         = String("ingress-watch-namespace", "NGROK_OPERATOR_FEATURES_INGRESS_WATCH_NAMESPACE", "", "Namespace to watch for Ingress and AgentEndpoint resources. Defaults to all namespaces.")
	GatewayEnabled                = Bool("enable-feature-gateway", "NGROK_OPERATOR_FEATURES_GATEWAY_ENABLED", true, "When true, enables support for Gateway API if the CRDs are detected. When false, Gateway API support will not be enabled")
	GatewayDisableReferenceGrants = Bool("disable-reference-grants", "NGROK_OPERATOR_FEATURES_GATEWAY_DISABLE_REFERENCE_GRANTS", false, "Opts-out of requiring ReferenceGrants for cross namespace references in Gateway API config")
	BindingsEnabled               = Bool("enable-feature-bindings", "NGROK_OPERATOR_FEATURES_BINDINGS_ENABLED", false, "Enables the Endpoint Bindings controller")
	BindingsEndpointSelectors     = List("bindings-endpoint-selectors", "NGROK_OPERATOR_FEATURES_BINDINGS_ENDPOINT_SELECTORS", []string{"true"}, "CEL expressions selecting the endpoints to project into this cluster, as a YAML or JSON list")
	BindingsServiceAnnotations    = Map("bindings-service-annotations", "NGROK_OPERATOR_FEATURES_BINDINGS_SERVICE_ANNOTATIONS", "Service Annotations to propagate to the target service, as a YAML or JSON map")
	BindingsServiceLabels         = Map("bindings-service-labels", "NGROK_OPERATOR_FEATURES_BINDINGS_SERVICE_LABELS", "Service Labels to propagate to the target service, as a YAML or JSON map")
	BindingsIngressEndpoint       = String("bindings-ingress-endpoint", "NGROK_OPERATOR_FEATURES_BINDINGS_INGRESS_ENDPOINT", "", "The endpoint the bindings forwarder connects to")
	DefaultDomainReclaimPolicy    = String("default-domain-reclaim-policy", "NGROK_OPERATOR_FEATURES_DEFAULT_DOMAIN_RECLAIM_POLICY", string(ingressv1alpha1.DomainReclaimPolicyDelete), "The default domain reclaim policy to apply to created domains")
	DrainPolicy                   = String("drain-policy", "NGROK_OPERATOR_FEATURES_DRAIN_POLICY", string(ngrokv1alpha1.DrainPolicyRetain), "Policy for draining resources during uninstall: Delete or Retain")

	// api-manager only
	OneClickDemoMode = Bool("one-click-demo-mode", "NGROK_OPERATOR_API_MANAGER_ONE_CLICK_DEMO_MODE", false, "Run the operator in one-click-demo mode (Ready, but not running)")

	// log: controller-runtime defines these flags; Log gives them defaults
	LogLevel           = zapSetting("zap-log-level", "NGROK_OPERATOR_LOG_LEVEL")
	LogFormat          = zapSetting("zap-encoder", "NGROK_OPERATOR_LOG_FORMAT")
	LogStacktraceLevel = zapSetting("zap-stacktrace-level", "NGROK_OPERATOR_LOG_STACKTRACE_LEVEL")
)

// Setting is one operator setting of type T.
type Setting[T any] struct {
	Flag  string
	Env   string
	Usage string
	def   T
	add   func(fs *pflag.FlagSet, p *T, name string, def T, usage string)
}

// all is every setting, for Validate and the tests.
var all []setting

type setting interface {
	env() string
	// check parses value the way the setting's flag would.
	check(value string) error
}

func register[T any](s *Setting[T]) *Setting[T] {
	all = append(all, s)
	return s
}

func String(flag, env, def, usage string) *Setting[string] {
	return register(&Setting[string]{Flag: flag, Env: env, Usage: usage, def: def, add: (*pflag.FlagSet).StringVar})
}

func Bool(flag, env string, def bool, usage string) *Setting[bool] {
	return register(&Setting[bool]{Flag: flag, Env: env, Usage: usage, def: def, add: (*pflag.FlagSet).BoolVar})
}

func List(flag, env string, def []string, usage string) *Setting[[]string] {
	return register(&Setting[[]string]{Flag: flag, Env: env, Usage: usage, def: def, add: func(fs *pflag.FlagSet, p *[]string, name string, def []string, usage string) {
		*p = def
		fs.Var(&listValue{p}, name, usage)
	}})
}

func Map(flag, env, usage string) *Setting[map[string]string] {
	return register(&Setting[map[string]string]{Flag: flag, Env: env, Usage: usage, def: map[string]string{}, add: func(fs *pflag.FlagSet, p *map[string]string, name string, _ map[string]string, usage string) {
		*p = map[string]string{}
		fs.Var(&mapValue{p}, name, usage)
	}})
}

// Bind registers the setting's flag on fs, writing into p. The default is the
// setting's environment variable when set, so --help shows the value the
// command will use. An invalid variable is left to Validate to report.
func (s *Setting[T]) Bind(fs *pflag.FlagSet, p *T) {
	s.add(fs, p, s.Flag, s.def, s.Usage)
	if value := os.Getenv(s.Env); value != "" {
		setDefault(fs.Lookup(s.Flag), value)
	}
}

func (s *Setting[T]) env() string { return s.Env }

func (s *Setting[T]) check(value string) error {
	var p T
	fs := pflag.NewFlagSet(s.Flag, pflag.ContinueOnError)
	s.add(fs, &p, s.Flag, s.def, s.Usage)
	return fs.Set(s.Flag, value)
}

// Default is the setting's built-in default as its flag prints it.
func (s *Setting[T]) Default() string {
	var p T
	fs := pflag.NewFlagSet(s.Flag, pflag.ContinueOnError)
	s.add(fs, &p, s.Flag, s.def, s.Usage)
	return fs.Lookup(s.Flag).DefValue
}

// setDefault makes value the flag's default. An invalid value leaves the
// default in place; pflag's Set can write a zero value before failing.
func setDefault(f *pflag.Flag, value string) {
	if f.Value.Set(value) != nil {
		_ = f.Value.Set(f.DefValue)
		return
	}
	f.DefValue = f.Value.String()
}

// zapFlag is a log setting whose flag controller-runtime defines.
type zapFlag struct{ Flag, Env string }

func zapSetting(flag, env string) *zapFlag {
	s := &zapFlag{flag, env}
	all = append(all, s)
	return s
}

func (s *zapFlag) env() string { return s.Env }

func (s *zapFlag) check(value string) error {
	fs := flag.NewFlagSet(s.Flag, flag.ContinueOnError)
	(&zap.Options{}).BindFlags(fs)
	return fs.Set(s.Flag, value)
}

// Log registers controller-runtime's --zap-* flags on fs, with the log
// settings' environment variables as their defaults.
func Log(fs *pflag.FlagSet) *zap.Options {
	opts := &zap.Options{}
	goFlagSet := flag.NewFlagSet("manager", flag.ContinueOnError)
	opts.BindFlags(goFlagSet)
	fs.AddGoFlagSet(goFlagSet)
	for _, s := range []*zapFlag{LogLevel, LogFormat, LogStacktraceLevel} {
		if value := os.Getenv(s.Env); value != "" {
			setDefault(fs.Lookup(s.Flag), value)
		}
	}
	return opts
}

// Validate reports every NGROK_OPERATOR_* variable that is invalid for its
// setting or names no setting, so a misspelled or malformed setting stops the
// operator instead of being ignored.
func Validate() error {
	var errs []error
	known := map[string]bool{}
	for _, s := range all {
		known[s.env()] = true
		if value := os.Getenv(s.env()); value != "" {
			if err := s.check(value); err != nil {
				errs = append(errs, fmt.Errorf("%s: %w", s.env(), err))
			}
		}
	}
	for _, kv := range os.Environ() {
		name, _, _ := strings.Cut(kv, "=")
		if strings.HasPrefix(name, "NGROK_OPERATOR_") && !known[name] {
			errs = append(errs, fmt.Errorf("%s is not an ngrok-operator setting", name))
		}
	}
	return errors.Join(errs...)
}
