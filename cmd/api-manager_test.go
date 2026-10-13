package cmd

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	discoveryfake "k8s.io/client-go/discovery/fake"
	clienttesting "k8s.io/client-go/testing"
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

func TestPrivateEndpointCRDInstalled(t *testing.T) {
	tests := []struct {
		name      string
		resources []*metav1.APIResourceList
		reactErr  error
		want      bool
		wantErr   bool
	}{
		{
			name: "group version not served",
		},
		{
			name: "group version served without privateendpoints",
			resources: []*metav1.APIResourceList{{
				GroupVersion: "ngrok.com/v1",
				APIResources: []metav1.APIResource{{Name: "trafficpolicies"}},
			}},
		},
		{
			name: "privateendpoints served",
			resources: []*metav1.APIResourceList{{
				GroupVersion: "ngrok.com/v1",
				APIResources: []metav1.APIResource{{Name: "trafficpolicies"}, {Name: "privateendpoints"}},
			}},
			want: true,
		},
		{
			name:     "discovery error is returned",
			reactErr: errors.New("boom"),
			wantErr:  true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dc := &discoveryfake.FakeDiscovery{Fake: &clienttesting.Fake{Resources: tt.resources}}
			if tt.reactErr != nil {
				dc.AddReactor("*", "*", func(clienttesting.Action) (bool, runtime.Object, error) {
					return true, nil, tt.reactErr
				})
			}

			got, err := privateEndpointCRDInstalled(dc)
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}
