package flags

import (
	"testing"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ngrok/ngrok-operator/internal/flags/flagstest"
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
			env:         map[string]string{"NGROK_OPERATOR__REGION": "eu"},
			want:        map[string]string{"region": "eu"},
			wantDefault: map[string]string{"region": "eu"},
		},
		{
			name: "flag wins over env",
			env:  map[string]string{"NGROK_OPERATOR__REGION": "eu"},
			args: []string{"--region=us"},
			want: map[string]string{"region": "us"},
		},
		{
			name: "empty env keeps the default",
			env:  map[string]string{"NGROK_OPERATOR__FEATURES__CLEANUP__DRAIN_POLICY": ""},
			want: map[string]string{"features-cleanup-drain-policy": "Retain"},
		},
		{
			name: "bool",
			env:  map[string]string{"NGROK_OPERATOR__FEATURES__GATEWAY__ENABLED": "false"},
			want: map[string]string{"features-gateway-enabled": "false"},
		},
		{
			name: "bool flag without a value",
			env:  map[string]string{"NGROK_OPERATOR__FEATURES__GATEWAY__ENABLED": "false"},
			args: []string{"--features-gateway-enabled"},
			want: map[string]string{"features-gateway-enabled": "true"},
		},
		{
			name: "list keeps commas inside items",
			env:  map[string]string{"NGROK_OPERATOR__FEATURES__BINDINGS__ENDPOINT_SELECTORS": `["a == 'x,y'","true"]`},
			want: map[string]string{"features-bindings-endpoint-selectors": `["a == 'x,y'","true"]`},
		},
		{
			name: "map as YAML",
			env:  map[string]string{"NGROK_OPERATOR__METADATA": `{env: dev, "example.com/team": k8s}`},
			want: map[string]string{"metadata": `{"env":"dev","example.com/team":"k8s"}`},
		},
		{
			name:        "invalid env keeps the built-in default",
			env:         map[string]string{"NGROK_OPERATOR__FEATURES__GATEWAY__ENABLED": "nope"},
			want:        map[string]string{"features-gateway-enabled": "true"},
			wantDefault: map[string]string{"features-gateway-enabled": "true"},
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
			Region(fs, &region)
			DrainPolicy(fs, &drain)
			GatewayEnabled(fs, &gateway)
			BindingsEndpointSelectors(fs, &selectors)
			Metadata(fs, &metadata)
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
			env:  map[string]string{"NGROK_OPERATOR__REGION": "eu", "NGROK_OPERATOR__LOG__LEVEL": "debug"},
		},
		{
			name:    "invalid bool",
			env:     map[string]string{"NGROK_OPERATOR__FEATURES__GATEWAY__ENABLED": "nope"},
			wantErr: "NGROK_OPERATOR__FEATURES__GATEWAY__ENABLED",
		},
		{
			name:    "invalid list",
			env:     map[string]string{"NGROK_OPERATOR__FEATURES__BINDINGS__ENDPOINT_SELECTORS": "{a: b}"},
			wantErr: "NGROK_OPERATOR__FEATURES__BINDINGS__ENDPOINT_SELECTORS",
		},
		{
			name: "unknown variable is not an error",
			env:  map[string]string{"NGROK_OPERATOR__REGOIN": "eu"},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			for k, v := range tc.env {
				t.Setenv(k, v)
			}
			err := Validate(testRoot())
			if tc.wantErr == "" {
				require.NoError(t, err)
				return
			}
			require.ErrorContains(t, err, tc.wantErr)
		})
	}
}

// testRoot is a command tree binding a few settings, read from the
// environment as it is when called.
func testRoot() *cobra.Command {
	var (
		region    string
		gateway   bool
		selectors []string
	)
	root := &cobra.Command{Use: "root"}
	sub := &cobra.Command{Use: "sub"}
	Region(sub.Flags(), &region)
	GatewayEnabled(sub.Flags(), &gateway)
	BindingsEndpointSelectors(sub.Flags(), &selectors)
	Log(sub.Flags())
	root.AddCommand(sub)
	return root
}

func TestLogger(t *testing.T) {
	for _, tc := range []struct {
		name    string
		args    []string
		wantErr string
	}{
		{name: "defaults"},
		{name: "valid", args: []string{"--log-level=8", "--log-format=console", "--log-stacktrace-level=panic"}},
		{name: "invalid level", args: []string{"--log-level=loud"}, wantErr: "--log-level"},
		{name: "invalid format", args: []string{"--log-format=xml"}, wantErr: "--log-format"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fs := pflag.NewFlagSet("test", pflag.ContinueOnError)
			log := Log(fs)
			require.NoError(t, fs.Parse(tc.args))
			_, err := log.Logger()
			if tc.wantErr == "" {
				require.NoError(t, err)
				return
			}
			require.ErrorContains(t, err, tc.wantErr)
		})
	}
}

func TestUnknown(t *testing.T) {
	for _, tc := range []struct {
		name string
		env  map[string]string
		want []string
	}{
		{name: "none", env: map[string]string{"NGROK_OPERATOR__REGION": "eu"}},
		{name: "misspelled", env: map[string]string{"NGROK_OPERATOR__REGOIN": "eu", "NGROK_OPERATOR__REGION": "eu"}, want: []string{"NGROK_OPERATOR__REGOIN"}},
		{
			// Kubernetes service links for a Service named ngrok-operator-*,
			// and variables read outside the settings, use a single "_".
			name: "single underscore after the prefix",
			env:  map[string]string{"NGROK_OPERATOR_METRICS_SERVICE_HOST": "10.0.0.1", "NGROK_OPERATOR_RESTART_ON_CERT_CHANGE": "true"},
		},
		{name: "other prefixes", env: map[string]string{"NGROK_ACCESS_TOKEN": "x", "NGROK_OPERATORX": "y"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			flagstest.ClearEnv(t)
			for k, v := range tc.env {
				t.Setenv(k, v)
			}
			assert.Equal(t, tc.want, Unknown(testRoot()))
		})
	}
}
