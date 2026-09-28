package cmd

import (
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
			name:      "a --zap flag beats the environment",
			env:       map[string]string{"NGROK_OPERATOR_LOG_LEVEL": "debug"},
			args:      []string{"--zap-log-level=error"},
			wantLevel: "error",
		},
		{
			name:    "an invalid value names the variable",
			env:     map[string]string{"NGROK_OPERATOR_FEATURES_INGRESS_ENABLED": "maybe"},
			wantErr: "NGROK_OPERATOR_FEATURES_INGRESS_ENABLED",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for k, v := range tt.env {
				t.Setenv(k, v)
			}
			c := apiCmd()
			require.NoError(t, c.ParseFlags(tt.args))

			cfg, err := loadConfig(c)

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
