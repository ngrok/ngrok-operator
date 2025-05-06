package cmd

import (
	"log"

	"github.com/spf13/cobra"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
)

var (
	scheme   = runtime.NewScheme()
	setupLog = ctrl.Log.WithName("setup")
	cfgPath  string
)

var rootCmd = &cobra.Command{
	Use:   "ngrok-operator",
	Short: "",
	Long:  ``,
}

func init() {
	rootCmd.PersistentFlags().StringVar(&cfgPath, "config", "", "path to the ngrok-operator config file")
	_ = cobra.MarkFlagRequired(rootCmd.PersistentFlags(), "config") // can't fail if the config flag exists
}

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		log.Fatalln(err)
	}
}

type operatorCommon struct {
	ReleaseName string `yaml:"release_name"` // Helm Release name for the deployed operator
	Description string `yaml:"description"`  // User-provided description
	MetricsAddr string `yaml:"metrics_addr"` // The address the metric endpoint binds to
	ProbeAddr   string `yaml:"probe_addr"`   // The address the probe endpoint binds to.
	ServerAddr  string `yaml:"server_addr"`  // The address of the ngrok server to use for tunnels
	Namespace   string `yaml:"namespace"`
	Region      string `yaml:"region"` // The region to use for ngrok tunnels

	zapOpts *zap.Options `yaml:"-"`

	// feature flags
	EnableFeatureIngress          bool `yaml:"enable_feature_ingress"`           // Enables the Ingress controller
	EnableFeatureGateway          bool `yaml:"enable_feature_gateway"`           // When true, enables support for Gateway API if the CRDs are detected. When false, Gateway API support will not be enabled
	EnableFeatureBindings         bool `yaml:"enable_feature_bindings"`          // Enables the Endpoint Bindings controller
	DisableGatewayReferenceGrants bool `yaml:"disable_gateway_reference_grants"` // Opts-out of requiring ReferenceGrants for cross namespace references in Gateway API config
}
