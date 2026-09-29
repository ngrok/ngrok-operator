package cmd

import (
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newRoot builds a fresh command tree, so flag state does not leak between
// tests the way it would through the package's rootCmd.
func newRoot() (root, api, agent, forwarder *cobra.Command) {
	root = &cobra.Command{Use: "ngrok-operator"}
	api, agent, forwarder = apiCmd(), agentCmd(), bindingsForwarderCmd()
	root.AddCommand(api, agent, forwarder)
	return root, api, agent, forwarder
}

func TestEnvName(t *testing.T) {
	for _, tc := range []struct{ flag, want string }{
		{"ngrok-root-cas", "NGROK_OPERATOR_NGROK_ROOT_CAS"},
		{"features-bindings-endpoint-selectors", "NGROK_OPERATOR_FEATURES_BINDINGS_ENDPOINT_SELECTORS"},
		{"api-manager-one-click-demo-mode", "NGROK_OPERATOR_API_MANAGER_ONE_CLICK_DEMO_MODE"},
	} {
		assert.Equal(t, tc.want, envName(tc.flag))
	}
}

func TestApplyEnv(t *testing.T) {
	for _, tc := range []struct {
		name    string
		env     map[string]string
		args    []string
		want    map[string]string
		wantErr string
	}{
		{
			name: "env sets a string",
			env:  map[string]string{"NGROK_OPERATOR_NGROK_REGION": "eu"},
			want: map[string]string{"ngrok-region": "eu"},
		},
		{
			name: "flag wins over env",
			env:  map[string]string{"NGROK_OPERATOR_NGROK_REGION": "eu"},
			args: []string{"--ngrok-region=us"},
			want: map[string]string{"ngrok-region": "us"},
		},
		{
			name: "empty env keeps the default",
			env:  map[string]string{"NGROK_OPERATOR_FEATURES_DRAIN_POLICY": ""},
			want: map[string]string{"features-drain-policy": "Retain"},
		},
		{
			name: "bool",
			env:  map[string]string{"NGROK_OPERATOR_FEATURES_GATEWAY_ENABLED": "false"},
			want: map[string]string{"features-gateway-enabled": "false"},
		},
		{
			name: "list keeps commas inside items",
			env:  map[string]string{"NGROK_OPERATOR_FEATURES_BINDINGS_ENDPOINT_SELECTORS": `["a == 'x,y'","true"]`},
			want: map[string]string{"features-bindings-endpoint-selectors": `["a == 'x,y'","true"]`},
		},
		{
			name: "map as YAML",
			env:  map[string]string{"NGROK_OPERATOR_NGROK_METADATA": `{env: dev, "example.com/team": k8s}`},
			want: map[string]string{"ngrok-metadata": `{"env":"dev","example.com/team":"k8s"}`},
		},
		{
			name: "list default",
			want: map[string]string{"features-bindings-endpoint-selectors": `["true"]`},
		},
		{
			name: "variable only another command reads",
			env:  map[string]string{"NGROK_OPERATOR_NGROK_ROOT_CAS": "host"},
			want: map[string]string{"ngrok-region": ""},
		},
		{
			name:    "invalid bool",
			env:     map[string]string{"NGROK_OPERATOR_FEATURES_GATEWAY_ENABLED": "nope"},
			wantErr: "NGROK_OPERATOR_FEATURES_GATEWAY_ENABLED",
		},
		{
			name:    "invalid list",
			env:     map[string]string{"NGROK_OPERATOR_FEATURES_BINDINGS_ENDPOINT_SELECTORS": "{a: b}"},
			wantErr: "NGROK_OPERATOR_FEATURES_BINDINGS_ENDPOINT_SELECTORS",
		},
		{
			name:    "unknown variable",
			env:     map[string]string{"NGROK_OPERATOR_NGROK_REGOIN": "eu"},
			wantErr: "NGROK_OPERATOR_NGROK_REGOIN is not a setting",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			for k, v := range tc.env {
				t.Setenv(k, v)
			}
			_, api, _, _ := newRoot()
			require.NoError(t, api.ParseFlags(tc.args))

			err := applyEnv(api)
			if tc.wantErr != "" {
				require.ErrorContains(t, err, tc.wantErr)
				return
			}
			require.NoError(t, err)
			for name, want := range tc.want {
				assert.Equal(t, want, api.Flags().Lookup(name).Value.String(), name)
			}
		})
	}
}

func TestApplyLogFlags(t *testing.T) {
	for _, tc := range []struct {
		name string
		env  map[string]string
		args []string
		want map[string]string
	}{
		{
			name: "log flag sets zap flag",
			args: []string{"--log-level=debug", "--log-format=console"},
			want: map[string]string{"zap-log-level": "debug", "zap-encoder": "console"},
		},
		{
			name: "log env sets zap flag",
			env:  map[string]string{"NGROK_OPERATOR_LOG_STACKTRACE_LEVEL": "info"},
			want: map[string]string{"zap-stacktrace-level": "info"},
		},
		{
			name: "zap flag wins",
			args: []string{"--log-level=debug", "--zap-log-level=error"},
			want: map[string]string{"zap-log-level": "error"},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			for k, v := range tc.env {
				t.Setenv(k, v)
			}
			_, _, agent, _ := newRoot()
			require.NoError(t, agent.ParseFlags(tc.args))
			require.NoError(t, configureFlags(agent, nil))
			for name, want := range tc.want {
				assert.Equal(t, want, agent.Flags().Lookup(name).Value.String(), name)
			}
		})
	}
}
