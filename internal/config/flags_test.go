package config

import (
	"testing"

	"github.com/spf13/pflag"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestRegisterFlagsDefaultsComeFromConfig is the property the whole design
// rests on: a flag's default is whatever the loaded config already holds, so
// an explicitly passed flag beats the file without any per-flag merge logic.
func TestRegisterFlagsDefaultsComeFromConfig(t *testing.T) {
	cfg := Default()
	cfg.Log.Level = "debug"
	cfg.Ngrok.Region = "eu"
	cfg.Features.Bindings.IngressEndpoint = "example.test:443"

	fs := pflag.NewFlagSet("test", pflag.ContinueOnError)
	RegisterLogFlags(fs, cfg)
	RegisterNgrokFlags(fs, cfg)
	RegisterFeatureFlags(fs, cfg)

	tests := []struct {
		flag string
		want string
	}{
		{"log-level", "debug"},
		{"region", "eu"},
		{"bindings-ingress-endpoint", "example.test:443"},
		{"log-format", Default().Log.Format},
		{"root-cas", Default().Ngrok.RootCAs},
	}

	for _, tt := range tests {
		t.Run(tt.flag, func(t *testing.T) {
			f := fs.Lookup(tt.flag)
			require.NotNil(t, f, "flag %q is not registered", tt.flag)
			assert.Equal(t, tt.want, f.DefValue)
		})
	}
}

// TestRegisterFlagsWriteThrough checks each helper actually binds to the
// struct it was given, rather than to a copy.
func TestRegisterFlagsWriteThrough(t *testing.T) {
	tests := []struct {
		name   string
		args   []string
		assert func(t *testing.T, cfg *Config)
	}{
		{
			name: "log",
			args: []string{"--log-level=warn", "--log-format=console"},
			assert: func(t *testing.T, cfg *Config) {
				assert.Equal(t, "warn", cfg.Log.Level)
				assert.Equal(t, "console", cfg.Log.Format)
			},
		},
		{
			name: "ngrok",
			args: []string{"--region=eu", "--ngrok-metadata=env=test,team=k8s"},
			assert: func(t *testing.T, cfg *Config) {
				assert.Equal(t, "eu", cfg.Ngrok.Region)
				assert.Equal(t, map[string]string{"env": "test", "team": "k8s"}, cfg.Ngrok.Metadata)
			},
		},
		{
			name: "features",
			args: []string{"--enable-feature-bindings=true", "--bindings-endpoint-selectors=a,b"},
			assert: func(t *testing.T, cfg *Config) {
				assert.True(t, cfg.Features.Bindings.Enabled)
				assert.Equal(t, []string{"a", "b"}, cfg.Features.Bindings.EndpointSelectors)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := Default()
			fs := pflag.NewFlagSet("test", pflag.ContinueOnError)
			RegisterLogFlags(fs, cfg)
			RegisterNgrokFlags(fs, cfg)
			RegisterFeatureFlags(fs, cfg)

			require.NoError(t, fs.Parse(tt.args))
			tt.assert(t, cfg)
		})
	}
}
