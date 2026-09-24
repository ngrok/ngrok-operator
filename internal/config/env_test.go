package config

import (
	"testing"

	"github.com/spf13/pflag"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEnvName(t *testing.T) {
	tests := []struct {
		flag string
		want string
	}{
		{"log-level", "NGROK_OPERATOR_LOG_LEVEL"},
		{"region", "NGROK_OPERATOR_REGION"},
		{"bindings-endpoint-selectors", "NGROK_OPERATOR_BINDINGS_ENDPOINT_SELECTORS"},
		{"config", "NGROK_OPERATOR_CONFIG"},
	}

	for _, tt := range tests {
		t.Run(tt.flag, func(t *testing.T) {
			assert.Equal(t, tt.want, EnvName(tt.flag))
		})
	}
}

func TestApplyEnv(t *testing.T) {
	tests := []struct {
		name    string
		env     map[string]string
		args    []string
		wantErr string
		assert  func(t *testing.T, cfg *Config, paths []string)
	}{
		{
			name: "an environment variable overrides the config file default",
			env:  map[string]string{"NGROK_OPERATOR_LOG_LEVEL": "warn"},
			args: nil,
			assert: func(t *testing.T, cfg *Config, paths []string) {
				assert.Equal(t, "warn", cfg.Log.Level)
			},
		},
		{
			name: "an explicit flag beats the environment",
			env:  map[string]string{"NGROK_OPERATOR_LOG_LEVEL": "warn"},
			args: []string{"--log-level=error"},
			assert: func(t *testing.T, cfg *Config, paths []string) {
				assert.Equal(t, "error", cfg.Log.Level)
			},
		},
		{
			name: "no environment variable leaves the loaded value alone",
			env:  nil,
			args: nil,
			assert: func(t *testing.T, cfg *Config, paths []string) {
				assert.Equal(t, "debug", cfg.Log.Level)
			},
		},
		{
			name: "booleans and lists use the encodings pflag already implements",
			env: map[string]string{
				"NGROK_OPERATOR_ENABLE_FEATURE_BINDINGS":     "true",
				"NGROK_OPERATOR_BINDINGS_ENDPOINT_SELECTORS": "a,b",
				"NGROK_OPERATOR_NGROK_METADATA":              "env=test,team=k8s",
			},
			assert: func(t *testing.T, cfg *Config, paths []string) {
				assert.True(t, cfg.Features.Bindings.Enabled)
				assert.Equal(t, []string{"a", "b"}, cfg.Features.Bindings.EndpointSelectors)
				assert.Equal(t, map[string]string{"env": "test", "team": "k8s"}, cfg.Ngrok.Metadata)
			},
		},
		{
			name:    "an unparseable value names the variable",
			env:     map[string]string{"NGROK_OPERATOR_ENABLE_FEATURE_BINDINGS": "yes-please"},
			wantErr: "NGROK_OPERATOR_ENABLE_FEATURE_BINDINGS",
		},
		{
			name: "the config flag is left to PreParseConfigPaths",
			env:  map[string]string{"NGROK_OPERATOR_CONFIG": "ignored.yaml"},
			assert: func(t *testing.T, cfg *Config, paths []string) {
				// ApplyEnv must not touch the config flag at all: its files are
				// already loaded by PreParseConfigPaths before flags are
				// registered, so setting it here would have no effect, and
				// touching it in place would make paths reflect NGROK_OPERATOR_CONFIG
				// instead of the files PreParseConfigPaths actually read.
				assert.Empty(t, paths)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for k, v := range tt.env {
				t.Setenv(k, v)
			}

			// Stand in for a config file that set log.level.
			cfg := Default()
			cfg.Log.Level = "debug"

			fs := pflag.NewFlagSet("test", pflag.ContinueOnError)
			var paths []string
			fs.StringArrayVar(&paths, ConfigFlag, nil, "")
			RegisterLogFlags(fs, cfg)
			RegisterNgrokFlags(fs, cfg)
			RegisterFeatureFlags(fs, cfg)
			require.NoError(t, fs.Parse(tt.args))

			err := ApplyEnv(fs)

			if tt.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
				return
			}

			require.NoError(t, err)
			tt.assert(t, cfg, paths)
		})
	}
}
