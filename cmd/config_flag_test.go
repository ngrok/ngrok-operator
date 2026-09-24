package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ngrok/ngrok-operator/internal/config"
)

// A malformed --config file must surface the YAML parse error naming the file,
// not "unknown flag: --config" or "unknown flag: --release-name". The commands
// load the config before registering most of their flags, so --config has to be
// registered ahead of the load and the flags that never got registered have to
// be tolerated.
//
// The argument lists below are the ones the chart's Deployments actually pass
// (templates/<component>/deployment.yaml). Testing --config on its own misses
// the only case that matters: a pod whose mounted ConfigMap is malformed.
func TestMalformedConfigReportsParseError(t *testing.T) {
	path := filepath.Join(t.TempDir(), "bad.yaml")
	if err := os.WriteFile(path, []byte("log: [unclosed\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	for _, tc := range []struct {
		name     string
		cmd      func() *cobra.Command
		chartArg []string
	}{
		{
			name: "api-manager",
			cmd:  apiCmd,
			chartArg: []string{
				"--release-name=ngrok-operator",
				"--health-probe-bind-address=:8081",
				"--metrics-bind-address=:8080",
				"--election-id=ngrok-operator-leader",
				"--manager-name=ngrok-operator-manager",
			},
		},
		{
			name: "agent-manager",
			cmd:  agentCmd,
			chartArg: []string{
				"--release-name=ngrok-operator",
				"--health-probe-bind-address=:8081",
				"--metrics-bind-address=:8080",
				"--manager-name=ngrok-operator-agent-manager",
			},
		},
		{
			name: "bindings-forwarder-manager",
			cmd:  bindingsForwarderCmd,
			chartArg: []string{
				"--release-name=ngrok-operator",
				"--health-probe-bind-address=:8081",
				"--metrics-bind-address=:8080",
				"--manager-name=ngrok-operator-bindings-forwarder",
			},
		},
	} {
		for _, args := range []struct {
			name string
			args []string
		}{
			{"config only", []string{"--config=" + path}},
			{"chart args", append([]string{"--config=" + path}, tc.chartArg...)},
		} {
			t.Run(tc.name+"/"+args.name, func(t *testing.T) {
				// The commands pre-parse os.Args to find --config before cobra runs.
				defer func(saved []string) { os.Args = saved }(os.Args)
				os.Args = append([]string{"ngrok-operator", tc.name}, args.args...)

				c := tc.cmd()
				c.SetArgs(args.args)
				c.SetOut(os.Stderr)
				c.SilenceUsage = true

				err := c.Execute()
				if err == nil {
					t.Fatal("expected an error for a malformed config file, got nil")
				}
				if !strings.Contains(err.Error(), path) {
					t.Errorf("error does not name the config file %q: %v", path, err)
				}
				if strings.Contains(err.Error(), "unknown flag") {
					t.Errorf("error blames a flag instead of the file: %v", err)
				}
			})
		}
	}
}

// TestPrecedenceChain runs the whole chain end to end on a real command: a
// config file sets a value, the environment overrides it, and an explicit flag
// overrides both.
func TestPrecedenceChain(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	require.NoError(t, os.WriteFile(path, []byte("log:\n  level: debug\n"), 0o600))

	tests := []struct {
		name string
		env  string
		args []string
		want string
	}{
		{
			name: "file only",
			args: []string{"--config=" + path},
			want: "debug",
		},
		{
			name: "environment beats the file",
			env:  "warn",
			args: []string{"--config=" + path},
			want: "warn",
		},
		{
			name: "flag beats the environment",
			env:  "warn",
			args: []string{"--config=" + path, "--log-level=error"},
			want: "error",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.env != "" {
				t.Setenv(config.EnvName("log-level"), tt.env)
			}

			cfg, err := config.Load([]string{path})
			require.NoError(t, err)

			fs := pflag.NewFlagSet("test", pflag.ContinueOnError)
			var paths []string
			fs.StringArrayVar(&paths, config.ConfigFlag, nil, "")
			config.RegisterLogFlags(fs, cfg)
			require.NoError(t, fs.Parse(tt.args))
			require.NoError(t, config.ApplyEnv(fs))

			assert.Equal(t, tt.want, cfg.Log.Level)
		})
	}
}
