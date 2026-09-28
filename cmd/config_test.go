package cmd

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// parse builds a command the way main does and runs everything up to RunE:
// flag parsing and PreRunE, which applies the environment and reports config
// errors. The constructors pre-parse os.Args for --config, so it is set too.
func parse(t *testing.T, newCmd func() *cobra.Command, args []string) (*cobra.Command, error) {
	t.Helper()

	saved := os.Args
	t.Cleanup(func() { os.Args = saved })
	os.Args = append([]string{"ngrok-operator", "subcommand"}, args...)

	c := newCmd()
	if err := c.ParseFlags(args); err != nil {
		return c, err
	}
	return c, c.PreRunE(c, nil)
}

// The argument lists are what `helm template` renders for each Deployment with
// every optional value set. A flag the chart passes but the binary no longer
// registers would crashloop the pod, so these must all parse.
func TestChartArgsParse(t *testing.T) {
	sharedArgs := []string{
		"--release-name=t",
		"--zap-log-level=8",
		"--zap-stacktrace-level=error",
		"--zap-encoder=json",
		"--health-probe-bind-address=:8081",
		"--metrics-bind-address=:8080",
	}

	tests := []struct {
		name   string
		cmd    func() *cobra.Command
		args   []string
		assert func(t *testing.T, c *cobra.Command)
	}{
		{
			name: "api-manager",
			cmd:  apiCmd,
			args: []string{
				"--description=The official ngrok Kubernetes Operator.",
				"--drain-policy=Retain",
				"--default-domain-reclaim-policy=Delete",
				"--enable-feature-ingress=true",
				"--enable-feature-gateway=true",
				"--disable-reference-grants=true",
				"--enable-feature-bindings=true",
				"--bindings-endpoint-selectors=true",
				"--bindings-service-annotations=a=b",
				"--bindings-service-labels=c=d",
				"--bindings-ingress-endpoint=kubernetes-binding-ingress.ngrok.io:443",
				"--region=eu",
				"--api-url=https://api.example",
				"--ngrok-metadata=e=f",
				"--ingress-controller-name=k8s.ngrok.com/ingress-controller",
				"--ingress-watch-namespace=ns",
				"--election-id=t-ngrok-operator-leader",
				"--manager-name=t-ngrok-operator-manager",
				"--cluster-domain=cluster.local",
				"--one-click-demo-mode",
			},
		},
		{
			name: "agent-manager",
			cmd:  agentCmd,
			args: []string{
				"--enable-feature-gateway=true",
				"--root-cas=host",
				"--server-addr=x.example:443",
				"--manager-name=t-ngrok-operator-agent-manager",
				"--default-domain-reclaim-policy=Delete",
				"--ingress-watch-namespace=ns",
			},
		},
		{
			name: "bindings-forwarder-manager",
			cmd:  bindingsForwarderCmd,
			args: []string{
				"--manager-name=t-ngrok-operator-bindings-forwarder",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := parse(t, tt.cmd, append(sharedArgs, tt.args...))
			require.NoError(t, err)
		})
	}
}

func TestPrecedence(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	require.NoError(t, os.WriteFile(path, []byte("ngrok:\n  region: eu\nlog:\n  level: debug\n"), 0o600))

	tests := []struct {
		name       string
		env        map[string]string
		args       []string
		wantRegion string
		wantLevel  string
	}{
		{
			name:       "built-in default",
			wantRegion: "",
			wantLevel:  "",
		},
		{
			name:       "file beats the default",
			args:       []string{"--config=" + path},
			wantRegion: "eu",
			wantLevel:  "debug",
		},
		{
			name:       "environment beats the file",
			env:        map[string]string{"NGROK_OPERATOR_REGION": "us", "NGROK_OPERATOR_ZAP_LOG_LEVEL": "info"},
			args:       []string{"--config=" + path},
			wantRegion: "us",
			wantLevel:  "info",
		},
		{
			name:       "flag beats the environment",
			env:        map[string]string{"NGROK_OPERATOR_REGION": "us", "NGROK_OPERATOR_ZAP_LOG_LEVEL": "info"},
			args:       []string{"--config=" + path, "--region=ap", "--zap-log-level=error"},
			wantRegion: "ap",
			wantLevel:  "error",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for k, v := range tt.env {
				t.Setenv(k, v)
			}

			c, err := parse(t, apiCmd, tt.args)
			require.NoError(t, err)

			region, err := c.Flags().GetString("region")
			require.NoError(t, err)
			assert.Equal(t, tt.wantRegion, region)
			assert.Equal(t, tt.wantLevel, c.Flags().Lookup("zap-log-level").Value.String())
		})
	}
}

// A malformed config file must be reported by name, not hidden behind an
// unknown-flag error for the chart's other arguments.
func TestConfigErrorsNameTheFile(t *testing.T) {
	dir := t.TempDir()
	malformed := filepath.Join(dir, "malformed.yaml")
	require.NoError(t, os.WriteFile(malformed, []byte("log: [unclosed\n"), 0o600))
	badLevel := filepath.Join(dir, "bad-level.yaml")
	require.NoError(t, os.WriteFile(badLevel, []byte("log:\n  level: loud\n"), 0o600))

	tests := []struct {
		name    string
		path    string
		wantErr string
	}{
		{"malformed yaml", malformed, malformed},
		{"invalid log level", badLevel, "--zap-log-level"},
	}

	for _, tt := range tests {
		for name, newCmd := range map[string]func() *cobra.Command{
			"api-manager":                apiCmd,
			"agent-manager":              agentCmd,
			"bindings-forwarder-manager": bindingsForwarderCmd,
		} {
			t.Run(tt.name+"/"+name, func(t *testing.T) {
				_, err := parse(t, newCmd, []string{"--config=" + tt.path, "--release-name=t", "--manager-name=t"})
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
				assert.NotContains(t, err.Error(), "unknown flag")
			})
		}
	}
}
