package cmd

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	"sigs.k8s.io/yaml"

	ingressv1alpha1 "github.com/ngrok/ngrok-operator/api/ingress/v1alpha1"
)

// envPrefix starts every environment variable the operator reads as a flag.
const envPrefix = "NGROK_OPERATOR_"

// envName is the environment variable for a flag: NGROK_OPERATOR_ and the flag
// name in upper snake case. --ngrok-root-cas is NGROK_OPERATOR_NGROK_ROOT_CAS.
func envName(flagName string) string {
	return envPrefix + strings.ToUpper(strings.ReplaceAll(flagName, "-", "_"))
}

// applyEnv sets every flag not passed on the command line from its environment
// variable, so the precedence is flag > environment variable > flag default.
// An empty variable counts as unset. A NGROK_OPERATOR_ variable that names no
// flag of any command is an error, so a misspelled setting is not ignored.
func applyEnv(c *cobra.Command) error {
	var errs []error
	c.Flags().VisitAll(func(f *pflag.Flag) {
		value := os.Getenv(envName(f.Name))
		if f.Changed || value == "" {
			return
		}
		if err := c.Flags().Set(f.Name, value); err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", envName(f.Name), err))
		}
	})

	known := knownEnvNames(c.Root())
	for _, kv := range os.Environ() {
		name, _, _ := strings.Cut(kv, "=")
		if strings.HasPrefix(name, envPrefix) && !known[name] {
			errs = append(errs, fmt.Errorf("%s is not a setting of any ngrok-operator command", name))
		}
	}
	return errors.Join(errs...)
}

// knownEnvNames is the environment variable of every flag of every command.
// The chart renders the shared settings into every component, so a variable
// only another component reads is not an error.
func knownEnvNames(root *cobra.Command) map[string]bool {
	known := map[string]bool{}
	var walk func(*cobra.Command)
	walk = func(c *cobra.Command) {
		c.Flags().VisitAll(func(f *pflag.Flag) { known[envName(f.Name)] = true })
		for _, sub := range c.Commands() {
			walk(sub)
		}
	}
	walk(root)
	return known
}

// logFlags are the operator's own names for controller-runtime's --zap-* flags.
var logFlags = []struct{ name, zap, help string }{
	{"log-level", "zap-log-level", "Log level: debug, info, error, panic, or an integer for more verbose debug levels. Sets --zap-log-level"},
	{"log-format", "zap-encoder", "Log format: json or console. Sets --zap-encoder"},
	{"log-stacktrace-level", "zap-stacktrace-level", "Level at and above which stacktraces are captured: info, error or panic. Sets --zap-stacktrace-level"},
}

// addLogFlags registers the --zap-* flags and the --log-* flags that set them.
func addLogFlags(fs *pflag.FlagSet) *zap.Options {
	opts := &zap.Options{}
	goFlagSet := flag.NewFlagSet("manager", flag.ContinueOnError)
	opts.BindFlags(goFlagSet)
	fs.AddGoFlagSet(goFlagSet)
	for _, f := range logFlags {
		fs.String(f.name, "", f.help)
	}
	return opts
}

// applyLogFlags passes each --log-* flag to its --zap-* flag, unless the
// --zap-* flag was passed itself.
func applyLogFlags(fs *pflag.FlagSet) error {
	var errs []error
	for _, f := range logFlags {
		value, _ := fs.GetString(f.name)
		if value == "" || fs.Changed(f.zap) {
			continue
		}
		if err := fs.Set(f.zap, value); err != nil {
			errs = append(errs, fmt.Errorf("--%s: %w", f.name, err))
		}
	}
	return errors.Join(errs...)
}

// configureFlags reads the environment into c's flags and passes the log
// flags to zap. Every command runs it before RunE.
func configureFlags(c *cobra.Command, _ []string) error {
	return errors.Join(applyEnv(c), applyLogFlags(c.Flags()))
}

// ngrokFlags are the ngrok connection settings shared by the api-manager and
// the agent.
type ngrokFlags struct {
	region     string
	serverAddr string
}

func addNgrokFlags(fs *pflag.FlagSet, o *ngrokFlags) {
	fs.StringVar(&o.region, "ngrok-region", "", "The region to use for ngrok tunnels")
	fs.StringVar(&o.serverAddr, "ngrok-server-addr", "", "The address of the ngrok server to use for tunnels")
}

// featureFlags are the feature settings shared by the api-manager and the
// agent.
type featureFlags struct {
	enableFeatureIngress          bool
	enableFeatureGateway          bool
	enableFeatureBindings         bool
	disableGatewayReferenceGrants bool
	ingressWatchNamespace         string
	defaultDomainReclaimPolicy    string
}

func addFeatureFlags(fs *pflag.FlagSet, o *featureFlags) {
	fs.BoolVar(&o.enableFeatureIngress, "features-ingress-enabled", true, "Enables the Ingress controller")
	fs.BoolVar(&o.enableFeatureGateway, "features-gateway-enabled", true, "When true, enables support for Gateway API if the CRDs are detected. When false, Gateway API support will not be enabled")
	fs.BoolVar(&o.disableGatewayReferenceGrants, "features-gateway-disable-reference-grants", false, "Opts-out of requiring ReferenceGrants for cross namespace references in Gateway API config")
	fs.BoolVar(&o.enableFeatureBindings, "features-bindings-enabled", false, "Enables the Endpoint Bindings controller")
	fs.StringVar(&o.ingressWatchNamespace, "features-ingress-watch-namespace", "", "Namespace to watch for Ingress and AgentEndpoint resources. Defaults to all namespaces.")
	fs.StringVar(&o.defaultDomainReclaimPolicy, "features-default-domain-reclaim-policy", string(ingressv1alpha1.DomainReclaimPolicyDelete), "The default domain reclaim policy to apply to created domains")
}

// stringsValue is a list flag that takes YAML or JSON, such as
// '["a == \'x,y\'", "true"]'. Unlike StringSlice it does not split on commas,
// which CEL expressions contain.
type stringsValue struct{ p *[]string }

func newStringsValue(p *[]string, def []string) *stringsValue {
	*p = def
	return &stringsValue{p}
}

func (v *stringsValue) Set(s string) error {
	var list []string
	if err := yaml.Unmarshal([]byte(s), &list); err != nil {
		return fmt.Errorf("want a YAML or JSON list of strings: %w", err)
	}
	*v.p = list
	return nil
}

func (v *stringsValue) String() string {
	b, _ := json.Marshal(*v.p)
	return string(b)
}

func (v *stringsValue) Type() string { return "list" }

// mapValue is a string map flag that takes YAML or JSON, such as
// '{env: dev, "example.com/team": k8s}'.
type mapValue struct{ p *map[string]string }

func newMapValue(p *map[string]string) *mapValue {
	*p = map[string]string{}
	return &mapValue{p}
}

func (v *mapValue) Set(s string) error {
	m := map[string]string{}
	if err := yaml.Unmarshal([]byte(s), &m); err != nil {
		return fmt.Errorf("want a YAML or JSON map of strings: %w", err)
	}
	*v.p = m
	return nil
}

func (v *mapValue) String() string {
	b, _ := json.Marshal(*v.p)
	return string(b)
}

func (v *mapValue) Type() string { return "map" }
