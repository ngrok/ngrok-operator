package podidentity

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"
)

func pod(ns, name, ip string) *corev1.Pod {
	return &corev1.Pod{
		UID:       types.UID("uid-" + name),
		Name:      name,
		Namespace: ns,
		Status:    corev1.PodStatus{PodIP: ip},
	}
}

func newClient(t *testing.T, objs ...client.Object) *fake.ClientBuilder {
	t.Helper()
	scheme := runtime.NewScheme()
	require.NoError(t, corev1.AddToScheme(scheme))
	return fake.NewClientBuilder().WithScheme(scheme).WithIndex(&corev1.Pod{}, IPIndex, podIP).WithObjects(objs...)
}

func TestLookup(t *testing.T) {
	tests := []struct {
		name        string
		pods        []client.Object
		ip          string
		wantID      Identity
		wantMatches int
	}{
		{
			name:        "one pod has the IP",
			pods:        []client.Object{pod("app", "web", "10.0.0.1"), pod("app", "other", "10.0.0.2")},
			ip:          "10.0.0.1",
			wantID:      Identity{UID: "uid-web", Name: "web", Namespace: "app", Annotations: map[string]string{}},
			wantMatches: 1,
		},
		{
			name:        "no pod has the IP",
			pods:        []client.Object{pod("app", "web", "10.0.0.1")},
			ip:          "10.0.0.9",
			wantID:      Identity{},
			wantMatches: 0,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := newClient(t, tt.pods...).Build()
			id, matches, err := Lookup(context.Background(), c, tt.ip)
			require.NoError(t, err)
			assert.Equal(t, tt.wantID, id)
			assert.Equal(t, tt.wantMatches, matches)
		})
	}
}

// Host-network Pods on one node share the node's IP. Which one wins is
// arbitrary, so only check that it is one of them.
func TestLookupSeveralPods(t *testing.T) {
	c := newClient(t, pod("b", "second", "10.0.0.1"), pod("a", "first", "10.0.0.1"), pod("a", "other", "10.0.0.2")).Build()

	id, matches, err := Lookup(context.Background(), c, "10.0.0.1")
	require.NoError(t, err)
	assert.Equal(t, 2, matches)
	assert.Contains(t, []string{"uid-first", "uid-second"}, id.UID)
}

func TestLookupListError(t *testing.T) {
	listErr := errors.New("cache not synced")
	c := newClient(t, pod("app", "web", "10.0.0.1")).WithInterceptorFuncs(interceptor.Funcs{
		List: func(context.Context, client.WithWatch, client.ObjectList, ...client.ListOption) error { return listErr },
	}).Build()

	id, _, err := Lookup(context.Background(), c, "10.0.0.1")
	assert.ErrorIs(t, err, listErr)
	assert.Equal(t, Identity{}, id)
}

func TestFromPodKeepsOnlyNgrokAnnotations(t *testing.T) {
	p := pod("app", "web", "10.0.0.1")
	p.Annotations = map[string]string{
		"ngrok.com/foo":     "new",
		"k8s.ngrok.com/bar": "old",
		"unrelated.io/baz":  "skip",
		"ngrok.com.evil/x":  "skip",
		"ngrok.com":         "skip",
	}

	assert.Equal(t, map[string]string{
		"ngrok.com/foo":     "new",
		"k8s.ngrok.com/bar": "old",
	}, FromPod(p).Annotations)
}
