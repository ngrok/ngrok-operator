package config

import (
	"testing"

	"github.com/spf13/pflag"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
)

// TestRegisterFlagsDefaultsComeFromConfig is the property the design rests
// on: a flag's default is whatever the loaded config already holds, so an
// explicitly passed flag beats the file without any per-flag merge logic.
func TestRegisterFlagsDefaultsComeFromConfig(t *testing.T) {
	cfg := Default()
	cfg.Ngrok.Region = "eu"
	cfg.Features.Bindings.IngressEndpoint = "example.test:443"

	fs := pflag.NewFlagSet("test", pflag.ContinueOnError)
	_, err := RegisterAPIManagerFlags(fs, cfg)
	require.NoError(t, err)

	tests := []struct {
		flag string
		want string
	}{
		{"region", "eu"},
		{"bindings-ingress-endpoint", "example.test:443"},
		{"cluster-domain", Default().Ngrok.ClusterDomain},
	}

	for _, tt := range tests {
		t.Run(tt.flag, func(t *testing.T) {
			f := fs.Lookup(tt.flag)
			require.NotNil(t, f, "flag %q is not registered", tt.flag)
			assert.Equal(t, tt.want, f.DefValue)
		})
	}
}

func TestRegisterFlagsWriteThrough(t *testing.T) {
	tests := []struct {
		name   string
		args   []string
		assert func(t *testing.T, cfg *Config)
	}{
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
			args: []string{"--enable-feature-bindings=true", "--bindings-endpoint-selectors=a,b", "--bindings-service-labels=k=v"},
			assert: func(t *testing.T, cfg *Config) {
				assert.True(t, cfg.Features.Bindings.Enabled)
				assert.Equal(t, []string{"a", "b"}, cfg.Features.Bindings.EndpointSelectors)
				assert.Equal(t, map[string]string{"k": "v"}, cfg.Features.Bindings.ServiceLabels)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := Default()
			fs := pflag.NewFlagSet("test", pflag.ContinueOnError)
			_, err := RegisterAPIManagerFlags(fs, cfg)
			require.NoError(t, err)

			require.NoError(t, fs.Parse(tt.args))
			tt.assert(t, cfg)
		})
	}
}

func TestRegisterFlagsLogSeedsZapFlags(t *testing.T) {
	tests := []struct {
		name    string
		log     LogConfig
		args    []string
		want    map[string]string
		wantErr string
	}{
		{
			name: "empty log config keeps the zap defaults",
			want: map[string]string{"zap-log-level": "", "zap-encoder": ""},
		},
		{
			name: "the config file seeds the zap flags",
			log:  LogConfig{Level: "debug", Format: "console", StacktraceLevel: "panic"},
			want: map[string]string{"zap-log-level": "debug", "zap-encoder": "console", "zap-stacktrace-level": "panic"},
		},
		{
			name: "numeric levels keep working",
			log:  LogConfig{Level: "8"},
			want: map[string]string{"zap-log-level": "8"},
		},
		{
			name: "an explicit zap flag beats the config file",
			log:  LogConfig{Level: "debug"},
			args: []string{"--zap-log-level=error"},
			want: map[string]string{"zap-log-level": "error"},
		},
		{
			name:    "an invalid value is reported after registration",
			log:     LogConfig{Level: "loud"},
			wantErr: "--zap-log-level",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := Default()
			cfg.Log = tt.log

			fs := pflag.NewFlagSet("test", pflag.ContinueOnError)
			zapOpts, err := RegisterBindingsForwarderFlags(fs, cfg)
			require.NotNil(t, zapOpts)
			require.NotNil(t, fs.Lookup("zap-log-level"), "zap flags must be registered even on error")

			if tt.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
				return
			}
			require.NoError(t, err)
			require.NoError(t, fs.Parse(tt.args))

			for name, want := range tt.want {
				assert.Equal(t, want, fs.Lookup(name).Value.String(), name)
			}
		})
	}
}

// Each component registers only the flags for settings it reads, so a flag it
// would silently ignore is rejected instead.
func TestComponentFlagSets(t *testing.T) {
	tests := []struct {
		name     string
		register func(*pflag.FlagSet, *Config) (*zap.Options, error)
		has      []string
		lacks    []string
	}{
		{
			name:     "api-manager",
			register: RegisterAPIManagerFlags,
			has:      []string{"region", "api-url", "enable-feature-bindings", "one-click-demo-mode", "zap-log-level"},
			lacks:    []string{"server-addr", "root-cas"},
		},
		{
			name:     "agent-manager",
			register: RegisterAgentFlags,
			has:      []string{"server-addr", "root-cas", "ingress-watch-namespace", "enable-feature-gateway", "default-domain-reclaim-policy", "zap-log-level"},
			lacks:    []string{"region", "api-url", "description", "enable-feature-bindings", "one-click-demo-mode"},
		},
		{
			name:     "bindings-forwarder-manager",
			register: RegisterBindingsForwarderFlags,
			has:      []string{"zap-log-level"},
			lacks:    []string{"description", "region", "enable-feature-bindings"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fs := pflag.NewFlagSet("test", pflag.ContinueOnError)
			_, err := tt.register(fs, Default())
			require.NoError(t, err)

			for _, name := range tt.has {
				assert.NotNil(t, fs.Lookup(name), "missing --%s", name)
			}
			for _, name := range tt.lacks {
				assert.Nil(t, fs.Lookup(name), "unexpected --%s", name)
			}
		})
	}
}
