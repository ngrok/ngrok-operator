package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"sigs.k8s.io/yaml"
)

// TestChartValuesMatchDefault keeps the Helm chart from carrying a second
// default. Every config key must exist in values.yaml, so it is documented, and
// hold either an empty value (the chart then omits it and Default() applies)
// or exactly the Go default. Booleans need the second form, since the chart
// cannot tell false from unset.
//
// The keys come from Default() itself, so a new config field fails here until
// it is added to values.yaml.
func TestChartValuesMatchDefault(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("..", "..", "helm", "ngrok-operator", "values.yaml"))
	require.NoError(t, err)
	var values map[string]any
	require.NoError(t, yaml.Unmarshal(raw, &values))

	for path, want := range leaves(t, Default()) {
		t.Run(path, func(t *testing.T) {
			valuesPath := path
			// Settings only the api-manager reads live in its own section.
			if path == "oneClickDemoMode" {
				valuesPath = "apiManager.config.oneClickDemoMode"
			}

			got, found := lookup(values, valuesPath)
			require.True(t, found, "%s is missing from values.yaml", valuesPath)

			if isEmpty(got) {
				return
			}
			assert.Equal(t, want, got, "%s in values.yaml must be empty or equal Default()", valuesPath)
		})
	}
}

// leaves flattens cfg into dotted paths, in the same JSON shape the chart
// renders, down to scalars, lists and maps.
func leaves(t *testing.T, cfg *Config) map[string]any {
	t.Helper()

	raw, err := json.Marshal(cfg)
	require.NoError(t, err)
	var tree map[string]any
	require.NoError(t, json.Unmarshal(raw, &tree))

	out := map[string]any{}
	var walk func(prefix string, node map[string]any)
	walk = func(prefix string, node map[string]any) {
		for key, val := range node {
			path := prefix + key
			// Maps with no Go default are free-form (metadata, labels):
			// treat them as leaves rather than as more structure.
			if m, ok := val.(map[string]any); ok && len(m) > 0 {
				walk(path+".", m)
				continue
			}
			out[path] = val
		}
	}
	walk("", tree)
	return out
}

func lookup(values map[string]any, path string) (any, bool) {
	var node any = values
	for part := range strings.SplitSeq(path, ".") {
		m, ok := node.(map[string]any)
		if !ok {
			return nil, false
		}
		if node, ok = m[part]; !ok {
			return nil, false
		}
	}
	return node, true
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
