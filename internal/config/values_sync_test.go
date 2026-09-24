package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"sigs.k8s.io/yaml"
)

// TestValuesDoNotRedeclareGoDefaults is the enforcement behind principle 1: a
// value has exactly one default, and it lives in Go.
//
// For every app config key the chart can send, values.yaml must either hold
// null -- meaning "not set", so Default() wins -- or hold a literal equal to
// the Go default, so the two cannot disagree. The second case exists for
// booleans, which Helm cannot distinguish from unset, and for maps whose
// default is empty.
func TestValuesDoNotRedeclareGoDefaults(t *testing.T) {
	values := readValues(t)
	cfg := Default()

	tests := []struct {
		path string
		want any
	}{
		{"ngrok.description", cfg.Ngrok.Description},
		{"ngrok.region", cfg.Ngrok.Region},
		{"ngrok.serverAddr", cfg.Ngrok.ServerAddr},
		{"ngrok.apiURL", cfg.Ngrok.APIURL},
		{"ngrok.rootCAs", cfg.Ngrok.RootCAs},
		{"ngrok.clusterDomain", cfg.Ngrok.ClusterDomain},
		{"ngrok.metadata", map[string]any{}},

		{"log.level", cfg.Log.Level},
		{"log.format", cfg.Log.Format},
		{"log.stacktraceLevel", cfg.Log.StacktraceLevel},

		{"features.ingress.enabled", cfg.Features.Ingress.Enabled},
		{"features.ingress.controllerName", cfg.Features.Ingress.ControllerName},
		{"features.ingress.watchNamespace", cfg.Features.Ingress.WatchNamespace},
		{"features.gateway.enabled", cfg.Features.Gateway.Enabled},
		{"features.gateway.disableReferenceGrants", cfg.Features.Gateway.DisableReferenceGrants},
		{"features.bindings.enabled", cfg.Features.Bindings.Enabled},
		{"features.bindings.endpointSelectors", cfg.Features.Bindings.EndpointSelectors},
		{"features.bindings.serviceAnnotations", map[string]any{}},
		{"features.bindings.serviceLabels", map[string]any{}},
		{"features.bindings.ingressEndpoint", cfg.Features.Bindings.IngressEndpoint},
		{"features.defaultDomainReclaimPolicy", cfg.Features.DefaultDomainReclaimPolicy},
		{"features.drainPolicy", cfg.Features.DrainPolicy},

		{"apiManager.oneClickDemoMode", cfg.OneClickDemoMode},
		{"agent.watchNamespace", cfg.WatchNamespace},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			got, found := lookup(values, tt.path)
			require.True(t, found, "values.yaml has no key %q; every settable key must be a real key so the generated schema describes it", tt.path)

			if got == nil {
				// null: the chart does not set it, so Default() wins. Nothing
				// can drift.
				return
			}

			assert.Equal(t, asDecoded(t, tt.want), got,
				"values.yaml sets %s to %v but internal/config.Default() says %v; a value written in both places will drift",
				tt.path, got, tt.want)
		})
	}
}

// TestComponentLogOverridesExist checks each component can override the shared
// log settings for itself, which is what the ConfigMap's per-component merge
// exists to deliver.
func TestComponentLogOverridesExist(t *testing.T) {
	values := readValues(t)

	for _, component := range []string{"apiManager", "agent", "bindingsForwarder"} {
		for _, key := range []string{"level", "format", "stacktraceLevel"} {
			path := component + ".log." + key
			t.Run(path, func(t *testing.T) {
				got, found := lookup(values, path)
				require.True(t, found, "values.yaml has no key %q", path)
				assert.Nil(t, got, "%s must be null: a component override that is set by default is not an override", path)
			})
		}
	}
}

// asDecoded round-trips a Go value through YAML so it is compared in the same
// representation the chart value was decoded into: a []string default and a
// []any decoded from values.yaml are the same value, but not the same type,
// and assert.EqualValues does not consider them equal either.
func asDecoded(t *testing.T, v any) any {
	t.Helper()

	raw, err := yaml.Marshal(v)
	require.NoError(t, err)

	var out any
	require.NoError(t, yaml.Unmarshal(raw, &out))

	return out
}

func readValues(t *testing.T) map[string]any {
	t.Helper()

	path := filepath.Join("..", "..", "helm", "ngrok-operator", "values.yaml")
	raw, err := os.ReadFile(path)
	require.NoError(t, err)

	var values map[string]any
	require.NoError(t, yaml.Unmarshal(raw, &values))

	return values
}

// lookup walks a dotted path. The second return distinguishes "the key is
// absent" from "the key is present and null", which mean different things
// here.
func lookup(values map[string]any, path string) (any, bool) {
	var current any = values

	for _, segment := range splitPath(path) {
		node, ok := current.(map[string]any)
		if !ok {
			return nil, false
		}
		current, ok = node[segment]
		if !ok {
			return nil, false
		}
	}

	return current, true
}

func splitPath(path string) []string {
	var segments []string
	start := 0
	for i := 0; i < len(path); i++ {
		if path[i] == '.' {
			segments = append(segments, path[start:i])
			start = i + 1
		}
	}
	return append(segments, path[start:])
}
