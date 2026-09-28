package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEnvName(t *testing.T) {
	tests := []struct {
		path string
		want string
	}{
		{"ngrok.region", "NGROK_OPERATOR_NGROK_REGION"},
		{"ngrok.rootCAs", "NGROK_OPERATOR_NGROK_ROOT_CAS"},
		{"ngrok.apiURL", "NGROK_OPERATOR_NGROK_API_URL"},
		{"log.stacktraceLevel", "NGROK_OPERATOR_LOG_STACKTRACE_LEVEL"},
		{"features.bindings.endpointSelectors", "NGROK_OPERATOR_FEATURES_BINDINGS_ENDPOINT_SELECTORS"},
		{"apiManager.oneClickDemoMode", "NGROK_OPERATOR_API_MANAGER_ONE_CLICK_DEMO_MODE"},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			assert.Equal(t, tt.want, EnvName(tt.path))
		})
	}
}

func TestEnvNamesCoverEverySetting(t *testing.T) {
	names := EnvNames()
	assert.Contains(t, names, "NGROK_OPERATOR_NGROK_REGION")
	assert.Contains(t, names, "NGROK_OPERATOR_API_MANAGER_ONE_CLICK_DEMO_MODE")
	assert.Len(t, names, len(leaves(t, Default())))
}

func TestLoad(t *testing.T) {
	tests := []struct {
		name    string
		env     map[string]string
		wantErr string
		assert  func(t *testing.T, cfg *Config)
	}{
		{
			name: "no variables returns the built-in defaults",
			assert: func(t *testing.T, cfg *Config) {
				assert.Equal(t, Default(), cfg)
			},
		},
		{
			name: "a variable overrides only its own setting",
			env:  map[string]string{"NGROK_OPERATOR_NGROK_REGION": "eu"},
			assert: func(t *testing.T, cfg *Config) {
				assert.Equal(t, "eu", cfg.Ngrok.Region)
				assert.Equal(t, Default().Ngrok.RootCAs, cfg.Ngrok.RootCAs)
			},
		},
		{
			name: "strings are taken as-is, even when they look like other types",
			env: map[string]string{
				"NGROK_OPERATOR_LOG_LEVEL":         "8",
				"NGROK_OPERATOR_NGROK_DESCRIPTION": "true",
			},
			assert: func(t *testing.T, cfg *Config) {
				assert.Equal(t, "8", cfg.Log.Level)
				assert.Equal(t, "true", cfg.Ngrok.Description)
			},
		},
		{
			name: "booleans, lists and maps parse as YAML or JSON",
			env: map[string]string{
				"NGROK_OPERATOR_FEATURES_GATEWAY_ENABLED":             "false",
				"NGROK_OPERATOR_API_MANAGER_ONE_CLICK_DEMO_MODE":      "true",
				"NGROK_OPERATOR_FEATURES_BINDINGS_ENDPOINT_SELECTORS": `["a == 'x,y'", "true"]`,
				"NGROK_OPERATOR_NGROK_METADATA":                       `{"env":"dev","team":"k8s"}`,
			},
			assert: func(t *testing.T, cfg *Config) {
				assert.False(t, cfg.Features.Gateway.Enabled)
				assert.True(t, cfg.APIManager.OneClickDemoMode)
				assert.Equal(t, []string{"a == 'x,y'", "true"}, cfg.Features.Bindings.EndpointSelectors)
				assert.Equal(t, map[string]string{"env": "dev", "team": "k8s"}, cfg.Ngrok.Metadata)
			},
		},
		{
			name: "an empty variable is unset",
			env:  map[string]string{"NGROK_OPERATOR_NGROK_ROOT_CAS": ""},
			assert: func(t *testing.T, cfg *Config) {
				assert.Equal(t, Default().Ngrok.RootCAs, cfg.Ngrok.RootCAs)
			},
		},
		{
			name: "a section variable sets several settings and keeps the rest",
			env: map[string]string{
				"NGROK_OPERATOR_FEATURES": `{"gateway":{"enabled":false},"bindings":{"enabled":true,"endpointSelectors":["a"]}}`,
			},
			assert: func(t *testing.T, cfg *Config) {
				assert.False(t, cfg.Features.Gateway.Enabled)
				assert.True(t, cfg.Features.Bindings.Enabled)
				assert.Equal(t, []string{"a"}, cfg.Features.Bindings.EndpointSelectors)
				assert.True(t, cfg.Features.Ingress.Enabled, "keys the section leaves out keep their defaults")
				assert.Equal(t, Default().Features.DrainPolicy, cfg.Features.DrainPolicy)
			},
		},
		{
			name: "a setting variable wins over its section",
			env: map[string]string{
				"NGROK_OPERATOR_LOG":       `{"level":"info","format":"console"}`,
				"NGROK_OPERATOR_LOG_LEVEL": "debug",
			},
			assert: func(t *testing.T, cfg *Config) {
				assert.Equal(t, "debug", cfg.Log.Level)
				assert.Equal(t, "console", cfg.Log.Format)
			},
		},
		{
			name:    "an unknown key in a section is an error",
			env:     map[string]string{"NGROK_OPERATOR_NGROK": `{"regoin":"eu"}`},
			wantErr: "NGROK_OPERATOR_NGROK",
		},
		{
			name:    "an unparseable value names the variable",
			env:     map[string]string{"NGROK_OPERATOR_FEATURES_GATEWAY_ENABLED": "maybe"},
			wantErr: "NGROK_OPERATOR_FEATURES_GATEWAY_ENABLED",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg, err := loadFrom(func(name string) (string, bool) {
				value, ok := tt.env[name]
				return value, ok
			})

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

func TestLoadDoesNotShareState(t *testing.T) {
	first, err := loadFrom(func(name string) (string, bool) {
		if name == "NGROK_OPERATOR_NGROK_METADATA" {
			return `{"env":"test"}`, true
		}
		return "", false
	})
	require.NoError(t, err)
	require.Equal(t, map[string]string{"env": "test"}, first.Ngrok.Metadata)

	second, err := loadFrom(func(string) (string, bool) { return "", false })
	require.NoError(t, err)
	assert.Empty(t, second.Ngrok.Metadata)
}
