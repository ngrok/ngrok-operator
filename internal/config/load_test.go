package config

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoad(t *testing.T) {
	tests := []struct {
		name    string
		files   []string
		wantErr string
		assert  func(t *testing.T, cfg *Config)
	}{
		{
			name:  "no files returns the built-in defaults",
			files: nil,
			assert: func(t *testing.T, cfg *Config) {
				assert.Equal(t, Default(), cfg)
			},
		},
		{
			name:  "a file overrides only the keys it sets",
			files: []string{"shared.yaml"},
			assert: func(t *testing.T, cfg *Config) {
				assert.Equal(t, "debug", cfg.Log.Level)
				assert.Equal(t, "eu", cfg.Ngrok.Region)
				// Untouched keys keep the built-in default.
				assert.Equal(t, Default().Log.StacktraceLevel, cfg.Log.StacktraceLevel)
				assert.Equal(t, Default().Ngrok.RootCAs, cfg.Ngrok.RootCAs)
			},
		},
		{
			name:  "later files override earlier ones",
			files: []string{"shared.yaml", "override.yaml"},
			assert: func(t *testing.T, cfg *Config) {
				assert.Equal(t, "warn", cfg.Log.Level)
				// A key set only by the first file survives the second.
				assert.Equal(t, "eu", cfg.Ngrok.Region)
			},
		},
		{
			name:  "component-owned keys decode from the top level",
			files: []string{"component.yaml"},
			assert: func(t *testing.T, cfg *Config) {
				assert.True(t, cfg.OneClickDemoMode)
				assert.Equal(t, "ngrok-operator", cfg.WatchNamespace)
			},
		},
		{
			name:    "a missing file is an error",
			files:   []string{"does-not-exist.yaml"},
			wantErr: "reading config",
		},
		{
			name:    "a malformed file is an error",
			files:   []string{"malformed.yaml"},
			wantErr: "parsing config",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			paths := make([]string, 0, len(tt.files))
			for _, f := range tt.files {
				paths = append(paths, filepath.Join("testdata", f))
			}

			cfg, err := Load(paths)

			if tt.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
				return
			}

			require.NoError(t, err)
			tt.assert(t, cfg)
		})
	}
}

// TestLoadDoesNotShareState guards Default()'s promise that callers get an
// independent copy: a map decoded into one Config must not be visible from a
// Config loaded afterwards.
func TestLoadDoesNotShareState(t *testing.T) {
	first, err := Load([]string{filepath.Join("testdata", "metadata.yaml")})
	require.NoError(t, err)
	require.Equal(t, map[string]string{"env": "test"}, first.Ngrok.Metadata)

	second, err := Load(nil)
	require.NoError(t, err)
	assert.Empty(t, second.Ngrok.Metadata)
}

func TestPreParseConfigPaths(t *testing.T) {
	tests := []struct {
		name string
		args []string
		env  string
		want []string
	}{
		{
			name: "no config flag",
			args: []string{"api-manager", "--log-level=debug"},
			want: nil,
		},
		{
			name: "one config flag",
			args: []string{"api-manager", "--config=/etc/ngrok-operator/apiManager.yaml"},
			want: []string{"/etc/ngrok-operator/apiManager.yaml"},
		},
		{
			name: "repeated config flags keep their order",
			args: []string{"--config=a.yaml", "--config=b.yaml"},
			want: []string{"a.yaml", "b.yaml"},
		},
		{
			name: "unknown flags are tolerated",
			args: []string{"--not-a-real-flag=1", "--config=a.yaml"},
			want: []string{"a.yaml"},
		},
		{
			name: "the environment supplies the path when the flag does not",
			args: []string{"api-manager"},
			env:  "from-env.yaml",
			want: []string{"from-env.yaml"},
		},
		{
			name: "the flag wins over the environment",
			args: []string{"--config=from-flag.yaml"},
			env:  "from-env.yaml",
			want: []string{"from-flag.yaml"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.env != "" {
				t.Setenv(EnvName(ConfigFlag), tt.env)
			}
			assert.Equal(t, tt.want, PreParseConfigPaths(tt.args))
		})
	}
}
