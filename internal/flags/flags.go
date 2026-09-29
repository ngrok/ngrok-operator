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

	"github.com/spf13/cobra"
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
)

// logEnv maps controller-runtime's --zap-* flags to their variables. Log
// registers the flags.
var logEnv = map[string]string{
	"zap-log-level":        "NGROK_OPERATOR_LOG_LEVEL",
	"zap-encoder":          "NGROK_OPERATOR_LOG_FORMAT",
	"zap-stacktrace-level": "NGROK_OPERATOR_LOG_STACKTRACE_LEVEL",
}

// Annotations on each setting's flag, read by Validate and the tests.
const (
	AnnotationEnv      = "ngrok-operator/env"
	annotationEnvError = "ngrok-operator/env-error"
)

func String(name, env, def, usage string) func(*pflag.FlagSet, *string) {
	return func(fs *pflag.FlagSet, p *string) {
		fs.StringVar(p, name, def, usage)
		withEnv(fs, name, env)
	}
}

func Bool(name, env string, def bool, usage string) func(*pflag.FlagSet, *bool) {
	return func(fs *pflag.FlagSet, p *bool) {
		fs.BoolVar(p, name, def, usage)
		withEnv(fs, name, env)
	}
}

func List(name, env string, def []string, usage string) func(*pflag.FlagSet, *[]string) {
	return func(fs *pflag.FlagSet, p *[]string) {
		*p = def
		fs.Var(&listValue{p}, name, usage)
		withEnv(fs, name, env)
	}
}

func Map(name, env, usage string) func(*pflag.FlagSet, *map[string]string) {
	return func(fs *pflag.FlagSet, p *map[string]string) {
		*p = map[string]string{}
		fs.Var(&mapValue{p}, name, usage)
		withEnv(fs, name, env)
	}
}

// Log registers controller-runtime's --zap-* flags on fs, with the log
// variables as their defaults.
func Log(fs *pflag.FlagSet) *zap.Options {
	opts := &zap.Options{}
	goFlagSet := flag.NewFlagSet("manager", flag.ContinueOnError)
	opts.BindFlags(goFlagSet)
	fs.AddGoFlagSet(goFlagSet)
	for name, env := range logEnv {
		withEnv(fs, name, env)
	}
	return opts
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
