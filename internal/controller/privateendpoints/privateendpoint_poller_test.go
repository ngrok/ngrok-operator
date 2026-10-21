package privateendpoints

import (
	"context"
	"errors"
	"testing"

	"github.com/go-logr/logr"
	"github.com/ngrok/ngrok-api-go/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	ngrokv1 "github.com/ngrok/ngrok-operator/api/ngrok/v1"
	"github.com/ngrok/ngrok-operator/internal/mocks/nmockapi"
	pe "github.com/ngrok/ngrok-operator/internal/privateendpoints"
)

const testNS = "ngrok-operator"

func newScheme(t *testing.T) *runtime.Scheme {
	t.Helper()
	s := runtime.NewScheme()
	require.NoError(t, ngrokv1.AddToScheme(s))
	return s
}

func existingCR(url string, managed bool) *ngrokv1.PrivateEndpoint {
	spec, _ := pe.ParseURL(url)
	labels := map[string]string{pe.HostLabel: pe.HostKey(spec.Hostname)}
	if managed {
		labels[pe.ManagedByLabel] = pe.ManagedByValue
	}
	return &ngrokv1.PrivateEndpoint{
		ObjectMeta: metav1.ObjectMeta{Name: pe.CRName(url), Namespace: testNS, Labels: labels},
		Spec:       spec,
	}
}

func crURLs(t *testing.T, c client.Client) []string {
	t.Helper()
	var list ngrokv1.PrivateEndpointList
	require.NoError(t, c.List(context.Background(), &list, client.InNamespace(testNS)))
	var urls []string
	for _, item := range list.Items {
		urls = append(urls, item.Spec.URL)
	}
	return urls
}

func TestPollerSync(t *testing.T) {
	mixed := []ngrok.EndpointCreate{
		{URL: "http://foo.internal", Bindings: []string{"internal"}},
		{URL: "https://foo.ngrok.direct", Bindings: []string{"internal"}},
		{URL: "tcp://bar.ngrok.direct:6379", Bindings: []string{"internal"}},
		{URL: "tls://baz.ngrok.direct", Bindings: []string{"internal"}},
		{URL: "https://public.ngrok.app", Bindings: []string{"public"}},
		{URL: "http://svc.ns", Bindings: []string{"kubernetes"}},
		{URL: "http://legacy.ngrok.direct", Bindings: []string{"kubernetes"}},
		{URL: "tcp://noport.ngrok.direct"},
	}
	tests := []struct {
		name      string
		endpoints []ngrok.EndpointCreate
		existing  []*ngrokv1.PrivateEndpoint
		listErr   error
		wantURLs  []string
		wantErr   bool
	}{
		{
			name:      "creates only ngrok.direct endpoints, skips internal, public and kubernetes-bound",
			endpoints: mixed,
			wantURLs:  []string{"https://foo.ngrok.direct", "tcp://bar.ngrok.direct:6379", "tls://baz.ngrok.direct"},
		},
		{
			name:      "deletes managed CRs that are gone, keeps unmanaged ones",
			endpoints: []ngrok.EndpointCreate{{URL: "http://foo.ngrok.direct"}},
			existing: []*ngrokv1.PrivateEndpoint{
				existingCR("http://foo.ngrok.direct", true),
				existingCR("http://gone.ngrok.direct", true),
				existingCR("http://foo.internal", true),
				existingCR("http://handmade.ngrok.direct", false),
			},
			wantURLs: []string{"http://foo.ngrok.direct", "http://handmade.ngrok.direct"},
		},
		{
			name:    "list error keeps CRs",
			listErr: errors.New("api down"),
			existing: []*ngrokv1.PrivateEndpoint{
				existingCR("http://foo.ngrok.direct", true),
				existingCR("https://foo.ngrok.direct", true),
			},
			wantURLs: []string{"http://foo.ngrok.direct", "https://foo.ngrok.direct"},
			wantErr:  true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			eps := nmockapi.NewEndpointsClient()
			for _, e := range tt.endpoints {
				_, err := eps.Create(ctx, &e)
				require.NoError(t, err)
			}
			if tt.listErr != nil {
				eps.SetListError(tt.listErr)
			}
			b := fake.NewClientBuilder().WithScheme(newScheme(t))
			for _, cr := range tt.existing {
				b = b.WithObjects(cr)
			}
			c := b.Build()
			p := &PrivateEndpointPoller{Client: c, Log: logr.Discard(), Namespace: testNS, Endpoints: eps}

			err := p.Sync(ctx)
			if tt.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
			assert.ElementsMatch(t, tt.wantURLs, crURLs(t, c))
		})
	}
}

func TestPollerSyncLabelsAndDedupe(t *testing.T) {
	ctx := context.Background()
	// Pooled endpoints share a URL. The mock rejects duplicate URLs on Create,
	// so feed desiredSpecs directly.
	pooled := []*ngrok.Endpoint{
		{ID: "ep_1", URL: "tcp://bar.ngrok.direct:6379"},
		{ID: "ep_2", URL: "tcp://bar.ngrok.direct:6379"},
	}
	desired := desiredSpecs(logr.Discard(), pooled)
	require.Len(t, desired, 1)

	eps := nmockapi.NewEndpointsClient()
	_, err := eps.Create(ctx, &ngrok.EndpointCreate{URL: "tcp://bar.ngrok.direct:6379"})
	require.NoError(t, err)
	c := fake.NewClientBuilder().WithScheme(newScheme(t)).Build()
	require.NoError(t, (&PrivateEndpointPoller{Client: c, Log: logr.Discard(), Namespace: testNS, Endpoints: eps}).Sync(ctx))

	var got ngrokv1.PrivateEndpoint
	require.NoError(t, c.Get(ctx, client.ObjectKey{Namespace: testNS, Name: pe.CRName("tcp://bar.ngrok.direct:6379")}, &got))
	assert.Equal(t, pe.ManagedByValue, got.Labels[pe.ManagedByLabel])
	assert.Equal(t, pe.HostKey("bar.ngrok.direct"), got.Labels[pe.HostLabel])
	assert.Equal(t, int32(6379), got.Spec.Port)
}

func TestPollerSyncHealsDrift(t *testing.T) {
	const url = "tcp://bar.ngrok.direct:6379"
	want, err := pe.ParseURL(url)
	require.NoError(t, err)
	wantLabels := map[string]string{pe.ManagedByLabel: pe.ManagedByValue, pe.HostLabel: pe.HostKey(want.Hostname)}
	unmanagedSpec := want
	unmanagedSpec.Port = 22

	tests := []struct {
		name       string
		mutate     func(*ngrokv1.PrivateEndpoint)
		wantSpec   ngrokv1.PrivateEndpointSpec
		wantLabels map[string]string
	}{
		{
			name:       "no drift leaves CR as is",
			mutate:     func(*ngrokv1.PrivateEndpoint) {},
			wantSpec:   want,
			wantLabels: wantLabels,
		},
		{
			name: "restores edited spec",
			mutate: func(cr *ngrokv1.PrivateEndpoint) {
				cr.Spec.URL = "http://evil.ngrok.direct"
				cr.Spec.Scheme = ngrokv1.PrivateEndpointSchemeHTTP
				cr.Spec.Hostname = "evil.ngrok.direct"
				cr.Spec.Port = 80
			},
			wantSpec:   want,
			wantLabels: wantLabels,
		},
		{
			name: "restores edited host label and keeps foreign labels",
			mutate: func(cr *ngrokv1.PrivateEndpoint) {
				cr.Labels[pe.HostLabel] = "wrong"
				cr.Labels["team"] = "a"
			},
			wantSpec:   want,
			wantLabels: map[string]string{pe.ManagedByLabel: pe.ManagedByValue, pe.HostLabel: pe.HostKey(want.Hostname), "team": "a"},
		},
		{
			// Without the label it is indistinguishable from a hand-made CR.
			name: "leaves CR without managed-by label alone",
			mutate: func(cr *ngrokv1.PrivateEndpoint) {
				delete(cr.Labels, pe.ManagedByLabel)
				cr.Spec.Port = 22
			},
			wantSpec:   unmanagedSpec,
			wantLabels: map[string]string{pe.HostLabel: pe.HostKey(want.Hostname)},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			eps := nmockapi.NewEndpointsClient()
			_, err := eps.Create(ctx, &ngrok.EndpointCreate{URL: url})
			require.NoError(t, err)
			cr := existingCR(url, true)
			tt.mutate(cr)
			c := fake.NewClientBuilder().WithScheme(newScheme(t)).WithObjects(cr).Build()
			p := &PrivateEndpointPoller{Client: c, Log: logr.Discard(), Namespace: testNS, Endpoints: eps}

			require.NoError(t, p.Sync(ctx))

			var got ngrokv1.PrivateEndpoint
			require.NoError(t, c.Get(ctx, client.ObjectKey{Namespace: testNS, Name: pe.CRName(url)}, &got))
			assert.Equal(t, tt.wantSpec, got.Spec)
			assert.Equal(t, tt.wantLabels, got.Labels)
		})
	}
}
