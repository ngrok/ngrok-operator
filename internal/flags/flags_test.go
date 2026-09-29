package flags

import (
	"testing"

	"github.com/spf13/pflag"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBind(t *testing.T) {
	for _, tc := range []struct {
		name        string
		env         map[string]string
		args        []string
		want        map[string]string
		wantDefault map[string]string
	}{
		{
			name:        "env replaces the default",
			env:         map[string]string{"NGROK_OPERATOR_NGROK_REGION": "eu"},
			want:        map[string]string{"region": "eu"},
			wantDefault: map[string]string{"region": "eu"},
		},
		{
			name: "flag wins over env",
			env:  map[string]string{"NGROK_OPERATOR_NGROK_REGION": "eu"},
			args: []string{"--region=us"},
			want: map[string]string{"region": "us"},
		},
		{
			name: "empty env keeps the default",
			env:  map[string]string{"NGROK_OPERATOR_FEATURES_DRAIN_POLICY": ""},
			want: map[string]string{"drain-policy": "Retain"},
		},
		{
			name: "bool",
			env:  map[string]string{"NGROK_OPERATOR_FEATURES_GATEWAY_ENABLED": "false"},
			want: map[string]string{"enable-feature-gateway": "false"},
		},
		{
			name: "bool flag without a value",
			env:  map[string]string{"NGROK_OPERATOR_FEATURES_GATEWAY_ENABLED": "false"},
			args: []string{"--enable-feature-gateway"},
			want: map[string]string{"enable-feature-gateway": "true"},
		},
		{
			name: "list keeps commas inside items",
			env:  map[string]string{"NGROK_OPERATOR_FEATURES_BINDINGS_ENDPOINT_SELECTORS": `["a == 'x,y'","true"]`},
			want: map[string]string{"bindings-endpoint-selectors": `["a == 'x,y'","true"]`},
		},
		{
			name: "map as YAML",
			env:  map[string]string{"NGROK_OPERATOR_NGROK_METADATA": `{env: dev, "example.com/team": k8s}`},
			want: map[string]string{"ngrokMetadata": `{"env":"dev","example.com/team":"k8s"}`},
		},
		{
			name:        "invalid env keeps the built-in default",
			env:         map[string]string{"NGROK_OPERATOR_FEATURES_GATEWAY_ENABLED": "nope"},
			want:        map[string]string{"enable-feature-gateway": "true"},
			wantDefault: map[string]string{"enable-feature-gateway": "true"},
		},
		{
			name: "log env seeds the zap flag",
			env:  map[string]string{"NGROK_OPERATOR_LOG_LEVEL": "debug", "NGROK_OPERATOR_LOG_FORMAT": "console"},
			want: map[string]string{"zap-log-level": "debug", "zap-encoder": "console"},
		},
		{
			name: "zap flag wins over log env",
			env:  map[string]string{"NGROK_OPERATOR_LOG_LEVEL": "debug"},
			args: []string{"--zap-log-level=error"},
			want: map[string]string{"zap-log-level": "error"},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			for k, v := range tc.env {
				t.Setenv(k, v)
			}
			var (
				region, drain string
				gateway       bool
				selectors     []string
				metadata      map[string]string
			)
			fs := pflag.NewFlagSet("test", pflag.ContinueOnError)
			NgrokRegion.Bind(fs, &region)
			DrainPolicy.Bind(fs, &drain)
			GatewayEnabled.Bind(fs, &gateway)
			BindingsEndpointSelectors.Bind(fs, &selectors)
			NgrokMetadata.Bind(fs, &metadata)
			Log(fs)
			require.NoError(t, fs.Parse(tc.args))

			for name, want := range tc.want {
				assert.Equal(t, want, fs.Lookup(name).Value.String(), name)
			}
			for name, want := range tc.wantDefault {
				assert.Equal(t, want, fs.Lookup(name).DefValue, name)
			}
		})
	}
}

func TestValidate(t *testing.T) {
	for _, tc := range []struct {
		name    string
		env     map[string]string
		wantErr string
	}{
		{
			name: "valid",
			env:  map[string]string{"NGROK_OPERATOR_NGROK_REGION": "eu", "NGROK_OPERATOR_LOG_LEVEL": "debug"},
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
			name:    "invalid log level",
			env:     map[string]string{"NGROK_OPERATOR_LOG_LEVEL": "loud"},
			wantErr: "NGROK_OPERATOR_LOG_LEVEL",
		},
		{
			name:    "unknown variable",
			env:     map[string]string{"NGROK_OPERATOR_NGROK_REGOIN": "eu"},
			wantErr: "NGROK_OPERATOR_NGROK_REGOIN is not an ngrok-operator setting",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			for k, v := range tc.env {
				t.Setenv(k, v)
			}
			err := Validate()
			if tc.wantErr == "" {
				require.NoError(t, err)
				return
			}
			require.ErrorContains(t, err, tc.wantErr)
		})
	}
}

func TestEnvNamesUnique(t *testing.T) {
	seen := map[string]bool{}
	for _, s := range all {
		assert.False(t, seen[s.env()], "%s is used by two settings", s.env())
		seen[s.env()] = true
	}
}
