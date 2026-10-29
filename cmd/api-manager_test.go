package cmd

import (
	"testing"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/cache"

	ngrokv1 "github.com/ngrok/ngrok-operator/api/ngrok/v1"
)

// byObjectNamespaces returns the namespaces pinned for the first ByObject key
// of obj's type, and whether one exists.
func byObjectNamespaces[T any](opts cache.Options) ([]string, bool) {
	for obj, cfg := range opts.ByObject {
		if _, ok := obj.(T); ok {
			var ns []string
			for n := range cfg.Namespaces {
				ns = append(ns, n)
			}
			return ns, true
		}
	}
	return nil, false
}

func TestManagerCacheOptions(t *testing.T) {
	tests := []struct {
		name          string
		opts          apiManagerOpts
		wantPE        []string
		wantService   []string
		wantDefaultNS bool
	}{
		{
			name: "private endpoints disabled pins nothing for them",
			opts: apiManagerOpts{namespace: "op"},
		},
		{
			name:   "enabled pins PrivateEndpoints to the operator namespace",
			opts:   apiManagerOpts{namespace: "op", enableFeaturePrivateEndpoints: true},
			wantPE: []string{"op"},
		},
		{
			name:          "enabled with a different watchNamespace also caches Services in the operator namespace",
			opts:          apiManagerOpts{namespace: "op", ingressWatchNamespace: "apps", enableFeaturePrivateEndpoints: true},
			wantPE:        []string{"op"},
			wantService:   []string{"apps", "op"},
			wantDefaultNS: true,
		},
		{
			name:          "enabled with watchNamespace equal to operator namespace needs no Service override",
			opts:          apiManagerOpts{namespace: "op", ingressWatchNamespace: "op", enableFeaturePrivateEndpoints: true},
			wantPE:        []string{"op"},
			wantDefaultNS: true,
		},
		{
			name:          "disabled with a different watchNamespace leaves Services alone",
			opts:          apiManagerOpts{namespace: "op", ingressWatchNamespace: "apps"},
			wantDefaultNS: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := managerCacheOptions(tt.opts)

			pe, ok := byObjectNamespaces[*ngrokv1.PrivateEndpoint](got)
			assert.Equal(t, tt.wantPE != nil, ok)
			assert.ElementsMatch(t, tt.wantPE, pe)

			svc, ok := byObjectNamespaces[*corev1.Service](got)
			assert.Equal(t, tt.wantService != nil, ok)
			assert.ElementsMatch(t, tt.wantService, svc)

			assert.Equal(t, tt.wantDefaultNS, got.DefaultNamespaces != nil)
		})
	}
}
