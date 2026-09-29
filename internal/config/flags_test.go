package config

import (
	"reflect"
	"strings"
	"testing"

	"github.com/spf13/pflag"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Every setting must say which components read it and describe itself, or it
// silently gets no flag or an empty usage line.
func TestEverySettingIsTagged(t *testing.T) {
	known := []string{APIManager, Agent, BindingsForwarder}

	walk(reflect.ValueOf(Default()).Elem(), "", func(path string, sf reflect.StructField, field reflect.Value) {
		if field.Kind() == reflect.Struct {
			return
		}
		t.Run(path, func(t *testing.T) {
			assert.NotEmpty(t, sf.Tag.Get("help"), "missing help tag")
			components := sf.Tag.Get("components")
			require.NotEmpty(t, components, "missing components tag")
			for _, c := range strings.Split(components, ",") {
				assert.Contains(t, known, c)
			}
		})
	})
}

func TestFlagName(t *testing.T) {
	tests := []struct {
		path string
		want string
	}{
		{"ngrok.rootCAs", "ngrok-root-cas"},
		{"ngrok.apiURL", "ngrok-api-url"},
		{"log.level", "log-level"},
		{"apiManager.oneClickDemoMode", "api-manager-one-click-demo-mode"},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			assert.Equal(t, tt.want, FlagName(tt.path))
		})
	}
}

// Each component gets flags only for the settings it reads, so a flag it would
// ignore is rejected instead.
func TestRegisterFlagsPerComponent(t *testing.T) {
	tests := []struct {
		component string
		has       []string
		lacks     []string
	}{
		{
			component: APIManager,
			has:       []string{"ngrok-region", "ngrok-api-url", "features-bindings-enabled", "api-manager-one-click-demo-mode", "log-level"},
			lacks:     []string{"ngrok-server-addr", "ngrok-root-cas"},
		},
		{
			component: Agent,
			has:       []string{"ngrok-server-addr", "ngrok-root-cas", "features-ingress-watch-namespace", "features-gateway-enabled", "features-default-domain-reclaim-policy", "log-level"},
			lacks:     []string{"ngrok-region", "ngrok-api-url", "ngrok-description", "features-bindings-enabled", "api-manager-one-click-demo-mode"},
		},
		{
			component: BindingsForwarder,
			has:       []string{"log-level", "log-format"},
			lacks:     []string{"ngrok-description", "ngrok-region", "features-bindings-enabled"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.component, func(t *testing.T) {
			fs := pflag.NewFlagSet("test", pflag.ContinueOnError)
			RegisterFlags(fs, tt.component)

			for _, name := range tt.has {
				assert.NotNil(t, fs.Lookup(name), "missing --%s", name)
			}
			for _, name := range tt.lacks {
				assert.Nil(t, fs.Lookup(name), "unexpected --%s", name)
			}
		})
	}
}

func TestFlagsShowDefaults(t *testing.T) {
	fs := pflag.NewFlagSet("test", pflag.ContinueOnError)
	RegisterFlags(fs, APIManager)

	tests := []struct {
		flag string
		want string
	}{
		{"ngrok-description", Default().Ngrok.Description},
		{"features-ingress-enabled", "true"},
		{"features-bindings-enabled", "false"},
		{"features-bindings-endpoint-selectors", `["true"]`},
		{"ngrok-region", ""},
	}

	for _, tt := range tests {
		t.Run(tt.flag, func(t *testing.T) {
			assert.Equal(t, tt.want, fs.Lookup(tt.flag).DefValue)
		})
	}
}

func TestFlagsApply(t *testing.T) {
	tests := []struct {
		name    string
		args    []string
		wantErr string
		assert  func(t *testing.T, cfg *Config)
	}{
		{
			name: "only flags passed on the command line change the config",
			args: []string{"--ngrok-region=us"},
			assert: func(t *testing.T, cfg *Config) {
				assert.Equal(t, "us", cfg.Ngrok.Region)
				assert.Equal(t, "eu", cfg.Ngrok.APIURL, "a value loaded earlier stays when its flag is not passed")
			},
		},
		{
			name: "a boolean flag needs no value",
			args: []string{"--features-bindings-enabled", "--features-gateway-enabled=false"},
			assert: func(t *testing.T, cfg *Config) {
				assert.True(t, cfg.Features.Bindings.Enabled)
				assert.False(t, cfg.Features.Gateway.Enabled)
			},
		},
		{
			name: "lists and maps take the same encoding as the environment",
			args: []string{`--features-bindings-endpoint-selectors=["a == 'x,y'"]`, "--ngrok-metadata={env: dev}"},
			assert: func(t *testing.T, cfg *Config) {
				assert.Equal(t, []string{"a == 'x,y'"}, cfg.Features.Bindings.EndpointSelectors)
				assert.Equal(t, map[string]string{"env": "dev"}, cfg.Ngrok.Metadata)
			},
		},
		{
			name:    "an unparseable value names the flag",
			args:    []string{"--features-bindings-enabled=maybe"},
			wantErr: "--features-bindings-enabled",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fs := pflag.NewFlagSet("test", pflag.ContinueOnError)
			flags := RegisterFlags(fs, APIManager)
			require.NoError(t, fs.Parse(tt.args))

			// Stand in for a value the environment set.
			cfg := Default()
			cfg.Ngrok.APIURL = "eu"

			err := flags.Apply(cfg)

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
