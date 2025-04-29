package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
)

var (
	scheme   = runtime.NewScheme()
	setupLog = ctrl.Log.WithName("setup")
)

var rootCmd = &cobra.Command{
	Use:   "ngrok-operator",
	Short: "",
	Long:  ``,
}

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}

type operatorCommon struct {
	ReleaseName string `yaml:"release_name"`
	Description string `yaml:"description"`
	MetricsAddr string `yaml:"metrics_addr"`
	ProbeAddr   string `yaml:"probe_addr"`

	// feature flags
	EnableFeatureIngress          bool `yaml:"enable_feature_ingress"`
	EnableFeatureGateway          bool `yaml:"enable_feature_gateway"`
	EnableFeatureBindings         bool `yaml:"enabled_feature_bindings"`
	DisableGatewayReferenceGrants bool `yaml:"disable_gateway_reference_grants"`
}
