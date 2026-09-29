package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"sigs.k8s.io/yaml"
)

var chartDir = filepath.Join("..", "helm", "ngrok-operator")

// valuesEnvName is the chart's ngrok-operator.envName: a values path joined
// with "_", in upper snake case.
func valuesEnvName(path []string) string {
	return envPrefix + strings.ToUpper(regexp.MustCompile(`([a-z0-9])([A-Z])`).ReplaceAllString(strings.Join(path, "_"), "${1}_${2}"))
}

// flagsByEnv maps each environment variable to its flag on every command that
// has it.
func flagsByEnv(cmds ...*cobra.Command) map[string][]*pflag.Flag {
	flags := map[string][]*pflag.Flag{}
	for _, c := range cmds {
		c.Flags().VisitAll(func(f *pflag.Flag) {
			flags[envName(f.Name)] = append(flags[envName(f.Name)], f)
		})
	}
	return flags
}

// setting is one operator setting in values.yaml.
type setting struct {
	path  []string
	value any
}

// chartSettings walks the operator settings in values.yaml: the shared ngrok,
// log and features sections, and each component's config section. A map is a
// setting when its path names a flag, and a section to walk into otherwise.
func chartSettings(t *testing.T, known map[string][]*pflag.Flag) []setting {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(chartDir, "values.yaml"))
	require.NoError(t, err)
	var values map[string]any
	require.NoError(t, yaml.Unmarshal(b, &values))

	var settings []setting
	var walk func(path []string, v any)
	walk = func(path []string, v any) {
		m, isMap := v.(map[string]any)
		if !isMap || known[valuesEnvName(path)] != nil {
			settings = append(settings, setting{path, v})
			return
		}
		for k, child := range m {
			walk(append(append([]string{}, path...), k), child)
		}
	}
	features := values["features"].(map[string]any)
	delete(features["ingress"].(map[string]any), "ingressClass") // chart-only: renders the IngressClass
	for _, section := range []string{"ngrok", "log", "features"} {
		walk([]string{section}, values[section])
	}
	// agent.config and bindingsForwarder.config hold only log overrides,
	// already covered by the shared log section.
	walk([]string{"apiManager"}, values["apiManager"].(map[string]any)["config"])
	return settings
}

// TestChartValuesMatchFlags checks that every operator setting in values.yaml
// is a flag, that its value there is empty or the flag's default, and that
// every configuration flag has a values key.
func TestChartValuesMatchFlags(t *testing.T) {
	_, api, agent, forwarder := newRoot()
	flags := flagsByEnv(api, agent, forwarder)
	settings := chartSettings(t, flags)

	inValues := map[string]bool{}
	for _, s := range settings {
		name := valuesEnvName(s.path)
		inValues[name] = true
		key := strings.Join(s.path, ".")
		if !assert.Contains(t, flags, name, "values.yaml %s is not a flag of any command", key) {
			continue
		}
		if isEmpty(s.value) {
			continue
		}
		for _, f := range flags[name] {
			assert.Equal(t, f.DefValue, encode(s.value), "values.yaml %s differs from the default of --%s", key, f.Name)
		}
	}

	for name, fs := range flags {
		if strings.HasPrefix(fs[0].Name, "zap-") || !regexp.MustCompile(`^(ngrok|log|features|api-manager)-`).MatchString(fs[0].Name) {
			continue // runtime plumbing the chart sets as args, not a user setting
		}
		assert.True(t, inValues[name], "--%s has no key in values.yaml", fs[0].Name)
	}
}

// TestChartEnvMatchesFlags renders the chart with every list and map setting
// set, then reads each Deployment's env into that component's flags the way
// the operator does, so the chart and the flags cannot drift apart on names,
// encoding or quoting.
func TestChartEnvMatchesFlags(t *testing.T) {
	helm, err := exec.LookPath("helm")
	if err != nil {
		t.Skip("helm not installed")
	}

	_, api, agent, forwarder := newRoot()
	values := map[string]any{}
	for _, s := range chartSettings(t, flagsByEnv(api, agent, forwarder)) {
		switch s.value.(type) {
		case map[string]any:
			setPath(values, s.path, map[string]any{"env": "dev", "example.com/team": "k8s"})
		case []any:
			setPath(values, s.path, []any{"a == 'x,y'", "true"})
		}
	}
	setPath(values, []string{"credentials", "accessToken"}, "x")
	setPath(values, []string{"ngrok", "region"}, "eu")
	setPath(values, []string{"ngrok", "rootCAs"}, "host")
	setPath(values, []string{"log", "level"}, "8")
	setPath(values, []string{"features", "bindings", "enabled"}, true)
	setPath(values, []string{"features", "gateway", "enabled"}, false)
	setPath(values, []string{"agent", "config", "log", "level"}, "debug")
	input, err := json.Marshal(values)
	require.NoError(t, err)

	cmd := exec.Command(helm, "template", "t", chartDir, "-f", "-")
	cmd.Stdin = strings.NewReader(string(input))
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, string(out))
	envs := deploymentEnv(t, string(out))

	for _, tc := range []struct {
		deployment string
		command    func() *cobra.Command
		want       map[string]string
	}{
		{"t-ngrok-operator-manager", apiCmd, map[string]string{
			"ngrok-region":                           "eu",
			"ngrok-metadata":                         `{"env":"dev","example.com/team":"k8s"}`,
			"features-bindings-endpoint-selectors":   `["a == 'x,y'","true"]`,
			"features-bindings-service-labels":       `{"env":"dev","example.com/team":"k8s"}`,
			"features-bindings-enabled":              "true",
			"features-gateway-enabled":               "false",
			"features-default-domain-reclaim-policy": "Delete",
			"zap-log-level":                          "8",
		}},
		{"t-ngrok-operator-agent", agentCmd, map[string]string{
			"ngrok-root-cas": "host",
			"zap-log-level":  "debug",
		}},
		{"t-ngrok-operator-bindings-forwarder", bindingsForwarderCmd, map[string]string{
			"zap-log-level": "8",
		}},
	} {
		t.Run(tc.deployment, func(t *testing.T) {
			env, ok := envs[tc.deployment]
			require.True(t, ok, "chart did not render %s", tc.deployment)
			for name, value := range env {
				if strings.HasPrefix(name, envPrefix) {
					t.Setenv(name, value)
				}
			}
			root := &cobra.Command{Use: "ngrok-operator"}
			c := tc.command()
			root.AddCommand(c, apiCmd(), agentCmd(), bindingsForwarderCmd())
			require.NoError(t, c.ParseFlags(nil))
			require.NoError(t, configureFlags(c, nil))
			for name, want := range tc.want {
				assert.Equal(t, want, c.Flags().Lookup(name).Value.String(), name)
			}
		})
	}
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

func setPath(m map[string]any, path []string, v any) {
	for _, k := range path[:len(path)-1] {
		next, ok := m[k].(map[string]any)
		if !ok {
			next = map[string]any{}
			m[k] = next
		}
		m = next
	}
	m[path[len(path)-1]] = v
}

func isEmpty(v any) bool {
	switch v := v.(type) {
	case nil:
		return true
	case string:
		return v == ""
	case []any:
		return len(v) == 0
	case map[string]any:
		return len(v) == 0
	}
	return false
}

// encode writes a values.yaml value the way its flag prints its default.
func encode(v any) string {
	switch v.(type) {
	case []any, map[string]any:
		b, _ := json.Marshal(v)
		return string(b)
	}
	return fmt.Sprint(v)
}
