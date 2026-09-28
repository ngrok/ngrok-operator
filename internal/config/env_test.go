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
		{"region", "NGROK_OPERATOR_REGION"},
		{"zap-log-level", "NGROK_OPERATOR_ZAP_LOG_LEVEL"},
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
		assert  func(t *testing.T, fs *pflag.FlagSet, cfg *Config, paths []string)
	}{
		{
			name: "an environment variable overrides the config file",
			env:  map[string]string{"NGROK_OPERATOR_REGION": "us"},
			assert: func(t *testing.T, _ *pflag.FlagSet, cfg *Config, _ []string) {
				assert.Equal(t, "us", cfg.Ngrok.Region)
			},
		},
		{
			name: "an explicit flag beats the environment",
			env:  map[string]string{"NGROK_OPERATOR_REGION": "us"},
			args: []string{"--region=ap"},
			assert: func(t *testing.T, _ *pflag.FlagSet, cfg *Config, _ []string) {
				assert.Equal(t, "ap", cfg.Ngrok.Region)
			},
		},
		{
			name: "no environment variable leaves the loaded value alone",
			assert: func(t *testing.T, _ *pflag.FlagSet, cfg *Config, _ []string) {
				assert.Equal(t, "eu", cfg.Ngrok.Region)
			},
		},
		{
			name: "zap flags read the environment too",
			env:  map[string]string{"NGROK_OPERATOR_ZAP_LOG_LEVEL": "error"},
			assert: func(t *testing.T, fs *pflag.FlagSet, _ *Config, _ []string) {
				assert.Equal(t, "error", fs.Lookup("zap-log-level").Value.String())
			},
		},
		{
			name: "booleans, lists and maps use the encodings pflag already implements",
			env: map[string]string{
				"NGROK_OPERATOR_ENABLE_FEATURE_BINDINGS":     "true",
				"NGROK_OPERATOR_BINDINGS_ENDPOINT_SELECTORS": "a,b",
				"NGROK_OPERATOR_NGROK_METADATA":              "env=test,team=k8s",
			},
			assert: func(t *testing.T, _ *pflag.FlagSet, cfg *Config, _ []string) {
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
			assert: func(t *testing.T, _ *pflag.FlagSet, _ *Config, paths []string) {
				assert.Empty(t, paths)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for k, v := range tt.env {
				t.Setenv(k, v)
			}

			// Stand in for a config file that set ngrok.region.
			cfg := Default()
			cfg.Ngrok.Region = "eu"

			fs := pflag.NewFlagSet("test", pflag.ContinueOnError)
			var paths []string
			fs.StringArrayVar(&paths, ConfigFlag, nil, "")
			_, err := RegisterAPIManagerFlags(fs, cfg)
			require.NoError(t, err)
			require.NoError(t, fs.Parse(tt.args))

			err = ApplyEnv(fs)

			if tt.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
				return
			}

			require.NoError(t, err)
			tt.assert(t, fs, cfg, paths)
		})
	}
}
