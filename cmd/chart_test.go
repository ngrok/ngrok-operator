package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"

	"github.com/spf13/pflag"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"sigs.k8s.io/yaml"

	"github.com/ngrok/ngrok-operator/internal/flags"
)

var chartDir = filepath.Join("..", "helm", "ngrok-operator")

// chartEnv is files/operator-env.yaml: each setting's variable by its path.
func chartEnv(t *testing.T) map[string]string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(chartDir, "files", "operator-env.yaml"))
	require.NoError(t, err)
	var names map[string]string
	require.NoError(t, yaml.Unmarshal(b, &names))
	return names
}

// flagsByEnv is each setting's flag by its variable, from a fresh command
// tree. Settings several commands share appear once.
func flagsByEnv() map[string]*pflag.Flag {
	byEnv := map[string]*pflag.Flag{}
	for f := range flags.Flags(newRoot()) {
		if env := f.Annotations[flags.AnnotationEnv]; env != nil {
			byEnv[env[0]] = f
		}
	}
	return byEnv
}

// envFromPath is the variable the naming rule gives a path under ngrok in the
// values: NGROK_OPERATOR_, then the path with "__" between levels and "_"
// between words.
func envFromPath(path string) string {
	parts := strings.Split(path, ".")
	for i, p := range parts {
		parts[i] = strings.ToUpper(camelWords.ReplaceAllString(p, "${1}_${2}"))
	}
	return "NGROK_OPERATOR_" + strings.Join(parts, "__")
}

var camelWords = regexp.MustCompile(`([a-z0-9])([A-Z])`)

// TestChartEnvNames checks that each variable in the chart's table follows the
// naming rule for its values path.
func TestChartEnvNames(t *testing.T) {
	for path, name := range chartEnv(t) {
		assert.Equal(t, envFromPath(path), name, path)
	}
}

// TestChartEnvMatchesFlags checks that the chart's table and internal/flags
// name the same variables.
func TestChartEnvMatchesFlags(t *testing.T) {
	var inChart, inGo []string
	for _, name := range chartEnv(t) {
		inChart = append(inChart, name)
	}
	for name := range flagsByEnv() {
		inGo = append(inGo, name)
	}
	sort.Strings(inChart)
	sort.Strings(inGo)
	assert.Equal(t, inGo, inChart, "files/operator-env.yaml and internal/flags must list the same variables")
}

// TestChartValuesMatchDefaults checks that every setting in the chart's table
// has a values.yaml key holding empty or its flag's default, and that
// values.yaml has no operator setting the table leaves out.
func TestChartValuesMatchDefaults(t *testing.T) {
	b, err := os.ReadFile(filepath.Join(chartDir, "values.yaml"))
	require.NoError(t, err)
	var chart struct {
		Ngrok map[string]any `json:"ngrok"`
	}
	require.NoError(t, yaml.Unmarshal(b, &chart))
	values := chart.Ngrok
	byEnv := flagsByEnv()
	table := chartEnv(t)

	inTable := map[string]bool{}
	for path, name := range table {
		inTable[path] = true
		v, ok := lookup(values, strings.Split(path, "."))
		if !assert.True(t, ok, "values.yaml has no ngrok.%s", path) || isEmpty(v) {
			continue
		}
		if f, ok := byEnv[name]; ok {
			assert.Equal(t, f.DefValue, encode(v), "values.yaml ngrok.%s differs from the default of %s", path, name)
		}
	}

	var walk func(path []string, v any)
	walk = func(path []string, v any) {
		key := strings.Join(path, ".")
		m, isMap := v.(map[string]any)
		if !isMap || inTable[key] {
			assert.True(t, inTable[key], "values.yaml ngrok.%s is not in files/operator-env.yaml", key)
			return
		}
		for k, child := range m {
			walk(append(append([]string{}, path...), k), child)
		}
	}
	// Values only the chart reads: the credentials Secret, the IngressClass
	// it renders and the cleanup hook it runs.
	delete(values, "credentials")
	features := values["features"].(map[string]any)
	delete(features["ingress"].(map[string]any), "ingressClass")
	delete(features["cleanup"].(map[string]any), "enabled")
	delete(features["cleanup"].(map[string]any), "timeout")
	for key, v := range values {
		walk([]string{key}, v)
	}
}

// TestChartComponentOverrides checks that every components.common key is
// documented, commented out, under each component that takes it, and that no
// component sets one in values.yaml: a set key would always override the
// common value.
func TestChartComponentOverrides(t *testing.T) {
	b, err := os.ReadFile(filepath.Join(chartDir, "values.yaml"))
	require.NoError(t, err)
	var chart struct {
		Components map[string]map[string]any `json:"components"`
	}
	require.NoError(t, yaml.Unmarshal(b, &chart))
	// The common keys, set or commented out, from their documentation.
	common := map[string]bool{}
	for _, m := range regexp.MustCompile(`(?m)^## @(?:param|extra) components\.common\.(\w+)`).FindAllStringSubmatch(string(b), -1) {
		common[m[1]] = true
	}
	require.NotEmpty(t, common)
	for _, component := range []string{"apiManager", "agent", "bindingsForwarder"} {
		for key := range common {
			path := "components." + component + "." + key
			assert.Contains(t, string(b), "## @extra "+path+" ", "values.yaml does not document %s", path)
			assert.NotContains(t, chart.Components[component], key, "values.yaml sets %s, so it always overrides components.common.%s", path, key)
		}
	}
}

// TestChartEnvParses renders the chart with every setting set and checks the
// api-manager gets every variable, each read back by the commands as the
// value that was set.
func TestChartEnvParses(t *testing.T) {
	helm, err := exec.LookPath("helm")
	if err != nil {
		t.Skip("helm not installed")
	}
	byEnv := flagsByEnv()
	table := chartEnv(t)

	values := map[string]any{"credentials": map[string]any{"accessToken": "x"}} // under ngrok
	want := map[string]string{}
	for path, name := range table {
		f := byEnv[name]
		require.NotNil(t, f, "%s is not a setting", name)
		v := sample(name, f.DefValue)
		set(values, strings.Split(path, "."), v)
		want[name] = encode(v)
	}
	input, err := json.Marshal(map[string]any{"ngrok": values})
	require.NoError(t, err)

	cmd := exec.Command(helm, "template", "t", chartDir, "-f", "-", "--show-only", "templates/api-manager/deployment.yaml")
	cmd.Stdin = strings.NewReader(string(input))
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, string(out))

	var d struct {
		Spec struct {
			Template struct {
				Spec struct {
					Containers []struct {
						Env []struct{ Name, Value string } `json:"env"`
					} `json:"containers"`
				} `json:"spec"`
			} `json:"template"`
		} `json:"spec"`
	}
	require.NoError(t, yaml.Unmarshal(out, &d))
	for _, e := range d.Spec.Template.Spec.Containers[0].Env {
		if strings.HasPrefix(e.Name, "NGROK_OPERATOR_") {
			t.Setenv(e.Name, e.Value)
		}
	}

	root := newRoot()
	require.NoError(t, flags.Validate(root))
	got := map[string]string{}
	for f := range flags.Flags(root) {
		if env := f.Annotations[flags.AnnotationEnv]; env != nil {
			got[env[0]] = f.Value.String()
		}
	}
	for name, value := range want {
		assert.Equal(t, value, got[name], name)
	}
}

// sample is a value unlike the default, of the setting's type.
func sample(name, def string) any {
	switch {
	case def == "true":
		return false
	case def == "false":
		return true
	case strings.HasPrefix(def, "["):
		return []any{"a == 'x,y'", "true"}
	case strings.HasPrefix(def, "{"):
		return map[string]any{"env": "dev", "example.com/team": "k8s"}
	case name == "NGROK_OPERATOR_LOG__LEVEL":
		return "debug"
	case name == "NGROK_OPERATOR_LOG__FORMAT":
		return "console"
	case name == "NGROK_OPERATOR_LOG__STACKTRACE_LEVEL":
		return "info"
	}
	return "sample"
}

func lookup(m map[string]any, path []string) (any, bool) {
	var v any = m
	for _, k := range path {
		mm, ok := v.(map[string]any)
		if !ok {
			return nil, false
		}
		if v, ok = mm[k]; !ok {
			return nil, false
		}
	}
	return v, true
}

func set(m map[string]any, path []string, v any) {
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

// encode writes a values.yaml value the way its flag prints it.
func encode(v any) string {
	switch v.(type) {
	case []any, map[string]any:
		b, _ := json.Marshal(v)
		return string(b)
	}
	return fmt.Sprint(v)
}
