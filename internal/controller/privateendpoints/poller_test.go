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
	tests := []struct {
		name      string
		endpoints []ngrok.EndpointCreate
		existing  []*ngrokv1.PrivateEndpoint
		listErr   error
		wantURLs  []string
		wantErr   bool
	}{
		{
			name: "creates private endpoints and skips public and kubernetes-bound",
			endpoints: []ngrok.EndpointCreate{
				{URL: "http://foo.internal", Bindings: []string{"internal"}},
				{URL: "https://foo.ngrok.direct", Bindings: []string{"internal"}},
				{URL: "tcp://bar.internal:6379", Bindings: []string{"internal"}},
				{URL: "https://public.ngrok.app", Bindings: []string{"public"}},
				{URL: "http://svc.ns", Bindings: []string{"kubernetes"}},
				{URL: "http://legacy.internal", Bindings: []string{"kubernetes"}},
				{URL: "tcp://noport.internal"},
			},
			wantURLs: []string{"http://foo.internal", "https://foo.ngrok.direct", "tcp://bar.internal:6379"},
		},
		{
			name:      "deletes managed CRs that are gone, keeps unmanaged ones",
			endpoints: []ngrok.EndpointCreate{{URL: "http://foo.internal"}},
			existing: []*ngrokv1.PrivateEndpoint{
				existingCR("http://foo.internal", true),
				existingCR("http://gone.internal", true),
				existingCR("http://handmade.internal", false),
			},
			wantURLs: []string{"http://foo.internal", "http://handmade.internal"},
		},
		{
			name:     "list error keeps CRs",
			listErr:  errors.New("api down"),
			existing: []*ngrokv1.PrivateEndpoint{existingCR("http://foo.internal", true)},
			wantURLs: []string{"http://foo.internal"},
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
			p := &Poller{Client: c, Log: logr.Discard(), Namespace: testNS, Endpoints: eps}

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
		{ID: "ep_1", URL: "tcp://bar.internal:6379"},
		{ID: "ep_2", URL: "tcp://bar.internal:6379"},
	}
	desired := desiredSpecs(logr.Discard(), pooled)
	require.Len(t, desired, 1)

	eps := nmockapi.NewEndpointsClient()
	_, err := eps.Create(ctx, &ngrok.EndpointCreate{URL: "tcp://bar.internal:6379"})
	require.NoError(t, err)
	c := fake.NewClientBuilder().WithScheme(newScheme(t)).Build()
	require.NoError(t, (&Poller{Client: c, Log: logr.Discard(), Namespace: testNS, Endpoints: eps}).Sync(ctx))

	var got ngrokv1.PrivateEndpoint
	require.NoError(t, c.Get(ctx, client.ObjectKey{Namespace: testNS, Name: pe.CRName("tcp://bar.internal:6379")}, &got))
	assert.Equal(t, pe.ManagedByValue, got.Labels[pe.ManagedByLabel])
	assert.Equal(t, pe.HostKey("bar.internal"), got.Labels[pe.HostLabel])
	assert.Equal(t, int32(6379), got.Spec.Port)
}
