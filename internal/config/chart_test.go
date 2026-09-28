package config

import (
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"sigs.k8s.io/yaml"
)

// TestChartEnvLoads renders the chart and loads each Deployment's env the way
// the operator does, so the chart and the loader cannot drift apart on names,
// encoding or quoting.
func TestChartEnvLoads(t *testing.T) {
	helm, err := exec.LookPath("helm")
	if err != nil {
		t.Skip("helm not installed")
	}

	values := `
credentials: {accessToken: x}
ngrok:
  region: eu
  rootCAs: host
  metadata: {env: dev, "example.com/team": k8s}
log: {level: "8", format: console}
features:
  bindings:
    enabled: true
    endpointSelectors: ["a == 'x,y'", "true"]
    serviceLabels: {tier: edge}
  gateway: {enabled: false}
  drainPolicy: Delete
agent:
  config:
    log: {level: debug}
`
	cmd := exec.Command(helm, "template", "t", filepath.Join("..", "..", "helm", "ngrok-operator"), "-f", "-")
	cmd.Stdin = strings.NewReader(values)
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, string(out))

	envs := deploymentEnv(t, string(out))
	require.Contains(t, envs, "t-ngrok-operator-manager")
	require.Contains(t, envs, "t-ngrok-operator-agent")

	for name, env := range envs {
		for key := range env {
			if strings.HasPrefix(key, EnvPrefix) {
				assert.Contains(t, sectionNames(), key, "%s renders %s, which the operator does not read", name, key)
			}
		}
	}

	api := loadEnv(t, envs["t-ngrok-operator-manager"])
	assert.Equal(t, "eu", api.Ngrok.Region)
	assert.Equal(t, map[string]string{"env": "dev", "example.com/team": "k8s"}, api.Ngrok.Metadata)
	assert.Equal(t, []string{"a == 'x,y'", "true"}, api.Features.Bindings.EndpointSelectors)
	assert.Equal(t, map[string]string{"tier": "edge"}, api.Features.Bindings.ServiceLabels)
	assert.True(t, api.Features.Bindings.Enabled)
	assert.False(t, api.Features.Gateway.Enabled)
	assert.Equal(t, "Delete", api.Features.DrainPolicy)
	assert.Equal(t, "8", api.Log.Level)
	assert.Equal(t, Default().Features.DefaultDomainReclaimPolicy, api.Features.DefaultDomainReclaimPolicy)

	agent := loadEnv(t, envs["t-ngrok-operator-agent"])
	assert.Equal(t, "host", agent.Ngrok.RootCAs)
	assert.Equal(t, "debug", agent.Log.Level)
	assert.Equal(t, "console", agent.Log.Format)
}

// deploymentEnv returns each rendered Deployment's plain env values by name.
func deploymentEnv(t *testing.T, manifests string) map[string]map[string]string {
	t.Helper()

	type deployment struct {
		Kind     string `json:"kind"`
		Metadata struct {
			Name string `json:"name"`
		} `json:"metadata"`
		Spec struct {
			Template struct {
				Spec struct {
					Containers []struct {
						Env []struct {
							Name  string `json:"name"`
							Value string `json:"value"`
						} `json:"env"`
					} `json:"containers"`
				} `json:"spec"`
			} `json:"template"`
		} `json:"spec"`
	}

	envs := map[string]map[string]string{}
	for _, doc := range strings.Split(manifests, "\n---") {
		var d deployment
		require.NoError(t, yaml.Unmarshal([]byte(doc), &d))
		if d.Kind != "Deployment" {
			continue
		}
		env := map[string]string{}
		for _, e := range d.Spec.Template.Spec.Containers[0].Env {
			env[e.Name] = e.Value
		}
		envs[d.Metadata.Name] = env
	}
	return envs
}

func loadEnv(t *testing.T, env map[string]string) *Config {
	t.Helper()
	cfg, err := loadFrom(func(name string) (string, bool) {
		value, ok := env[name]
		return value, ok
	})
	require.NoError(t, err)
	return cfg
}

// sectionNames lists the variables for Config's top-level sections, the only
// ones the chart renders.
func sectionNames() []string {
	var names []string
	t := reflect.TypeFor[Config]()
	for i := range t.NumField() {
		name, _, _ := strings.Cut(t.Field(i).Tag.Get("json"), ",")
		names = append(names, EnvName(name))
	}
	return names
}
