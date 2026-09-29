// Package flags defines every operator setting once: its flag name,
// environment variable, default and help. Each setting is a function that
// binds its flag onto a command's flag set, writing into the command's own
// options.
//
// A setting's environment variable, when set, becomes its flag's default, so
// the precedence is flag > environment variable > default. The Helm chart
// names the same variables in files/operator-env.yaml; a test checks the two
// agree.
package flags

import (
	"errors"
	"flag"
	"fmt"
	"iter"
	"os"
	"strings"

	"github.com/go-logr/logr"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	common "github.com/ngrok/ngrok-operator/api/common/v1alpha1"
	ingressv1alpha1 "github.com/ngrok/ngrok-operator/api/ingress/v1alpha1"
	ngrokv1alpha1 "github.com/ngrok/ngrok-operator/api/ngrok/v1alpha1"
)

// Environment variables are NGROK_OPERATOR_ and the setting's path in the
// chart's values, with "__" between levels and "_" between words:
// features.gateway.enabled is NGROK_OPERATOR_FEATURES__GATEWAY__ENABLED. The
// shared ngrok settings sit at the top level, like clusterDomain, which is a
// top-level value: ngrok.region is NGROK_OPERATOR_REGION. A flag is its
// variable without the prefix, in kebab case. TestSettingNames in cmd/ checks
// both rules.
var (
	// ngrok
	Description   = String("description", "NGROK_OPERATOR_DESCRIPTION", "Created by the ngrok-operator", "Description for this installation")
	Region        = String("region", "NGROK_OPERATOR_REGION", "", "The region to use for ngrok tunnels")
	ServerAddr    = String("server-addr", "NGROK_OPERATOR_SERVER_ADDR", "", "The address of the ngrok server to use for tunnels")
	APIURL        = String("api-url", "NGROK_OPERATOR_API_URL", "", "The base URL to use for the ngrok api")
	RootCAs       = String("root-cas", "NGROK_OPERATOR_ROOT_CAS", "trusted", "trusted (default) or host: use the trusted ngrok agent CA or the host CA")
	Metadata      = Map("metadata", "NGROK_OPERATOR_METADATA", "Metadata added to the ngrok API resources the operator creates, as a YAML or JSON map")
	ClusterDomain = String("cluster-domain", "NGROK_OPERATOR_CLUSTER_DOMAIN", common.DefaultClusterDomain, "Cluster domain used in the cluster")

	// log
	LogLevel           = String("log-level", "NGROK_OPERATOR_LOG__LEVEL", "info", "Log level: debug, info, error, panic, or an integer > 0 for more verbose debug levels")
	LogFormat          = String("log-format", "NGROK_OPERATOR_LOG__FORMAT", "json", "Log format: json or console")
	LogStacktraceLevel = String("log-stacktrace-level", "NGROK_OPERATOR_LOG__STACKTRACE_LEVEL", "error", "Level at and above which stacktraces are captured: info, error or panic")

	// features
	IngressEnabled                = Bool("features-ingress-enabled", "NGROK_OPERATOR_FEATURES__INGRESS__ENABLED", true, "Enables the Ingress controller")
	IngressControllerName         = String("features-ingress-controller-name", "NGROK_OPERATOR_FEATURES__INGRESS__CONTROLLER_NAME", "k8s.ngrok.com/ingress-controller", "The name of the controller to use for matching ingresses classes")
	IngressWatchNamespace         = String("features-ingress-watch-namespace", "NGROK_OPERATOR_FEATURES__INGRESS__WATCH_NAMESPACE", "", "Namespace to watch for Ingress and AgentEndpoint resources. Defaults to all namespaces.")
	GatewayEnabled                = Bool("features-gateway-enabled", "NGROK_OPERATOR_FEATURES__GATEWAY__ENABLED", true, "When true, enables support for Gateway API if the CRDs are detected. When false, Gateway API support will not be enabled")
	GatewayDisableReferenceGrants = Bool("features-gateway-disable-reference-grants", "NGROK_OPERATOR_FEATURES__GATEWAY__DISABLE_REFERENCE_GRANTS", false, "Opts-out of requiring ReferenceGrants for cross namespace references in Gateway API config")
	BindingsEnabled               = Bool("features-bindings-enabled", "NGROK_OPERATOR_FEATURES__BINDINGS__ENABLED", false, "Enables the Endpoint Bindings controller")
	BindingsEndpointSelectors     = List("features-bindings-endpoint-selectors", "NGROK_OPERATOR_FEATURES__BINDINGS__ENDPOINT_SELECTORS", []string{"true"}, "CEL expressions selecting the endpoints to project into this cluster, as a YAML or JSON list")
	BindingsServiceAnnotations    = Map("features-bindings-service-annotations", "NGROK_OPERATOR_FEATURES__BINDINGS__SERVICE_ANNOTATIONS", "Service Annotations to propagate to the target service, as a YAML or JSON map")
	BindingsServiceLabels         = Map("features-bindings-service-labels", "NGROK_OPERATOR_FEATURES__BINDINGS__SERVICE_LABELS", "Service Labels to propagate to the target service, as a YAML or JSON map")
	BindingsIngressEndpoint       = String("features-bindings-ingress-endpoint", "NGROK_OPERATOR_FEATURES__BINDINGS__INGRESS_ENDPOINT", "", "The endpoint the bindings forwarder connects to")
	DefaultDomainReclaimPolicy    = String("features-domains-default-reclaim-policy", "NGROK_OPERATOR_FEATURES__DOMAINS__DEFAULT_RECLAIM_POLICY", string(ingressv1alpha1.DomainReclaimPolicyDelete), "The default domain reclaim policy to apply to created domains")
	DrainPolicy                   = String("features-cleanup-drain-policy", "NGROK_OPERATOR_FEATURES__CLEANUP__DRAIN_POLICY", string(ngrokv1alpha1.DrainPolicyRetain), "Policy for draining resources during uninstall: Delete or Retain")
	OneClickDemoMode              = Bool("features-one-click-demo-mode-enabled", "NGROK_OPERATOR_FEATURES__ONE_CLICK_DEMO_MODE__ENABLED", false, "Run the operator in one-click-demo mode (Ready, but not running)")
)

// otherEnv are the NGROK_OPERATOR_ variables read outside these settings.
//
// TODO: make NGROK_OPERATOR_RESTART_ON_CERT_CHANGE a setting (and a values
// key) like the others, instead of reading it in internal/util.
var otherEnv = []string{"NGROK_OPERATOR_RESTART_ON_CERT_CHANGE"}

// Annotations withEnv puts on each setting's flag, so code holding only the
// command tree can find a flag's variable. AnnotationEnv is the variable's
// name: Validate collects them to reject unknown NGROK_OPERATOR_ variables,
// and the tests in cmd/ compare them with the naming rule and the chart.
// annotationEnvError is why the variable's value could not be parsed, which
// Validate reports; binding a flag cannot return an error itself.
const (
	AnnotationEnv      = "ngrok-operator/env"
	annotationEnvError = "ngrok-operator/env-error"
)

// String defines a string setting. The returned function binds its flag onto
// a command's flag set, writing into p.
func String(name, env, def, usage string) func(*pflag.FlagSet, *string) {
	return func(fs *pflag.FlagSet, p *string) {
		fs.StringVar(p, name, def, usage)
		withEnv(fs, name, env)
	}
}

// Bool defines a boolean setting. See String.
func Bool(name, env string, def bool, usage string) func(*pflag.FlagSet, *bool) {
	return func(fs *pflag.FlagSet, p *bool) {
		fs.BoolVar(p, name, def, usage)
		withEnv(fs, name, env)
	}
}

// List defines a list setting, written as a YAML or JSON list. See String.
func List(name, env string, def []string, usage string) func(*pflag.FlagSet, *[]string) {
	return func(fs *pflag.FlagSet, p *[]string) {
		*p = def
		fs.Var(&yamlValue[[]string]{p, "list"}, name, usage)
		withEnv(fs, name, env)
	}
}

// Map defines a string map setting, written as a YAML or JSON map, empty by
// default. See String.
func Map(name, env, usage string) func(*pflag.FlagSet, *map[string]string) {
	return func(fs *pflag.FlagSet, p *map[string]string) {
		*p = map[string]string{}
		fs.Var(&yamlValue[map[string]string]{p, "map"}, name, usage)
		withEnv(fs, name, env)
	}
}

// ManagerOptions are the controller-runtime manager's flags, which every
// command has. They are runtime plumbing the chart passes as args, not
// settings, so they have no environment variable.
type ManagerOptions struct {
	ReleaseName string
	MetricsAddr string
	ProbeAddr   string
	ManagerName string
}

// Manager binds the manager flags onto fs, writing into o. managerName is the
// command's default manager name.
func Manager(fs *pflag.FlagSet, o *ManagerOptions, managerName string) {
	fs.StringVar(&o.ReleaseName, "release-name", "ngrok-operator", "Helm Release name for the deployed operator")
	fs.StringVar(&o.MetricsAddr, "metrics-bind-address", ":8080", "The address the metric endpoint binds to")
	fs.StringVar(&o.ProbeAddr, "health-probe-bind-address", ":8081", "The address the probe endpoint binds to.")
	fs.StringVar(&o.ManagerName, "manager-name", managerName, "Manager name to identify unique instances of this component")
}

// LogOptions holds the log settings. Commands build their logger from it,
// so they do not depend on the logging library.
type LogOptions struct{ level, format, stacktraceLevel string }

// Log binds the log settings onto fs.
func Log(fs *pflag.FlagSet) *LogOptions {
	o := &LogOptions{}
	LogLevel(fs, &o.level)
	LogFormat(fs, &o.format)
	LogStacktraceLevel(fs, &o.stacktraceLevel)
	return o
}

// Logger builds the logger. It parses the settings with controller-runtime's
// zap flags, so the accepted values are theirs.
func (o *LogOptions) Logger() (logr.Logger, error) {
	var opts zap.Options
	zapFlags := flag.NewFlagSet("log", flag.ContinueOnError)
	opts.BindFlags(zapFlags)
	for _, s := range []struct{ setting, zapFlag, value string }{
		{"log-level", "zap-log-level", o.level},
		{"log-format", "zap-encoder", o.format},
		{"log-stacktrace-level", "zap-stacktrace-level", o.stacktraceLevel},
	} {
		if err := zapFlags.Set(s.zapFlag, s.value); err != nil {
			return logr.Logger{}, fmt.Errorf("--%s: %w", s.setting, err)
		}
	}
	return zap.New(zap.UseFlagOptions(&opts)), nil
}

// withEnv tags the flag with its variable and, when the variable is set,
// makes its value the flag's default, so --help shows the value the command
// will use. An invalid value keeps the built-in default and is recorded for
// Validate; pflag's Set can write a zero value before failing.
func withEnv(fs *pflag.FlagSet, name, env string) {
	_ = fs.SetAnnotation(name, AnnotationEnv, []string{env})
	value := os.Getenv(env)
	if value == "" {
		return
	}
	f := fs.Lookup(name)
	if err := f.Value.Set(value); err != nil {
		_ = f.Value.Set(f.DefValue)
		_ = fs.SetAnnotation(name, annotationEnvError, []string{fmt.Sprintf("%s: %v", env, err)})
		return
	}
	f.DefValue = f.Value.String()
}

// Validate reports every NGROK_OPERATOR_* variable that its setting could not
// parse, or that no command's flag reads, so a misspelled or malformed
// setting stops the operator instead of being ignored.
func Validate(root *cobra.Command) error {
	var errs []error
	known := map[string]bool{}
	for _, env := range otherEnv {
		known[env] = true
	}
	for f := range Flags(root) {
		if env := f.Annotations[AnnotationEnv]; env != nil {
			known[env[0]] = true
		}
		if msg := f.Annotations[annotationEnvError]; msg != nil {
			errs = append(errs, errors.New(msg[0]))
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

// Flags yields every flag of root and its subcommands.
func Flags(root *cobra.Command) iter.Seq[*pflag.Flag] {
	return func(yield func(*pflag.Flag) bool) {
		cmds := []*cobra.Command{root}
		for len(cmds) > 0 {
			c := cmds[0]
			cmds = append(cmds[1:], c.Commands()...)
			stop := false
			c.Flags().VisitAll(func(f *pflag.Flag) {
				if !stop && !yield(f) {
					stop = true
				}
			})
			if stop {
				return
			}
		}
	}
}
