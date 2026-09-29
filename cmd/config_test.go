package cmd

import (
	"flag"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	"github.com/ngrok/ngrok-operator/internal/config"
)

// The chart passes only deployment identity and nothing else as flags; every
// setting arrives as an NGROK_OPERATOR_* variable. A flag the chart passes but
// the binary no longer registers would crashloop the pod.
func TestChartArgsParse(t *testing.T) {
	tests := []struct {
		name string
		cmd  func() *cobra.Command
		args []string
	}{
		{
			name: "api-manager",
			cmd:  apiCmd,
			args: []string{"--release-name=t", "--health-probe-bind-address=:8081", "--metrics-bind-address=:8080", "--election-id=t-ngrok-operator-leader", "--manager-name=t-ngrok-operator-manager"},
		},
		{
			name: "agent-manager",
			cmd:  agentCmd,
			args: []string{"--release-name=t", "--health-probe-bind-address=:8081", "--metrics-bind-address=:8080", "--manager-name=t-ngrok-operator-agent-manager"},
		},
		{
			name: "bindings-forwarder-manager",
			cmd:  bindingsForwarderCmd,
			args: []string{"--release-name=t", "--health-probe-bind-address=:8081", "--metrics-bind-address=:8080", "--manager-name=t-ngrok-operator-bindings-forwarder"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.NoError(t, tt.cmd().ParseFlags(tt.args))
		})
	}
}

// newConfigCmd registers the api-manager's config and zap flags the same way
// apiCmd does.
func newConfigCmd() (*cobra.Command, *config.Flags) {
	c := &cobra.Command{}
	flags := config.RegisterFlags(c.Flags(), config.APIManager)
	goFlagSet := flag.NewFlagSet("zap", flag.ContinueOnError)
	(&zap.Options{}).BindFlags(goFlagSet)
	c.Flags().AddGoFlagSet(goFlagSet)
	return c, flags
}

func TestLoadConfig(t *testing.T) {
	tests := []struct {
		name       string
		env        map[string]string
		args       []string
		wantRegion string
		wantLevel  string
		wantErr    string
	}{
		{
			name: "built-in defaults",
		},
		{
			name:       "the environment sets config and log settings",
			env:        map[string]string{"NGROK_OPERATOR_NGROK_REGION": "eu", "NGROK_OPERATOR_LOG_LEVEL": "debug"},
			wantRegion: "eu",
			wantLevel:  "debug",
		},
		{
			name:       "a flag beats the environment",
			env:        map[string]string{"NGROK_OPERATOR_NGROK_REGION": "eu", "NGROK_OPERATOR_LOG_LEVEL": "debug"},
			args:       []string{"--ngrok-region=us", "--log-level=info"},
			wantRegion: "us",
			wantLevel:  "info",
		},
		{
			name:      "a --zap flag beats --log-level",
			args:      []string{"--log-level=debug", "--zap-log-level=error"},
			wantLevel: "error",
		},
		{
			name:    "an invalid environment value names the variable",
			env:     map[string]string{"NGROK_OPERATOR_FEATURES_INGRESS_ENABLED": "maybe"},
			wantErr: "NGROK_OPERATOR_FEATURES_INGRESS_ENABLED",
		},
		{
			name:    "an invalid flag value names the flag",
			args:    []string{"--features-bindings-endpoint-selectors=[unclosed"},
			wantErr: "--features-bindings-endpoint-selectors",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for k, v := range tt.env {
				t.Setenv(k, v)
			}
			c, flags := newConfigCmd()
			require.NoError(t, c.ParseFlags(tt.args))

			cfg, err := loadConfig(c, flags)

			if tt.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.wantRegion, cfg.Ngrok.Region)
			assert.Equal(t, tt.wantLevel, c.Flags().Lookup("zap-log-level").Value.String())
		})
	}
}
