package ngrokapi

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/ngrok/ngrok-api-go/v9"
	bindingsv1alpha1 "github.com/ngrok/ngrok-operator/api/bindings/v1alpha1"
)

func Test_parseDialURL(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		endpoint ngrok.Endpoint
		want     *parsedHostport
		wantErr  bool
	}{
		{"empty", ngrok.Endpoint{}, nil, true},
		{"invalid", ngrok.Endpoint{Proto: "https", URL: "https://[::1"}, nil, true},
		{"no-host", ngrok.Endpoint{Proto: "https", URL: "https:///path"}, nil, true},
		// We trust the api to only support specific schemes
		{"mismatched-scheme", ngrok.Endpoint{Proto: "tls", URL: "https://test.internal"}, nil, true},
		{"missing-tcp-port", ngrok.Endpoint{Proto: "tcp", URL: "tcp://test.internal"}, nil, true},
		{"unknown-scheme-no-port", ngrok.Endpoint{URL: "ftp://test.internal"}, nil, true},
		// with defaults
		{"simple", ngrok.Endpoint{URL: "foo.internal"}, &parsedHostport{"https", "foo.internal", 443}, false},
		{"full", ngrok.Endpoint{Proto: "tcp", URL: "tcp://foo.internal:1234"}, &parsedHostport{"tcp", "foo.internal", 1234}, false},
		{"http-no-port", ngrok.Endpoint{Proto: "http", URL: "foo.internal"}, &parsedHostport{"http", "foo.internal", 80}, false},
		// The dial host is the endpoint's own hostname, so it is no longer
		// constrained to two labels.
		{"multi-label", ngrok.Endpoint{URL: "tcp://foo.bar.internal:80"}, &parsedHostport{"tcp", "foo.bar.internal", 80}, false},
		// Endpoints that predate the url field only carry public_url.
		{"public-url-fallback", ngrok.Endpoint{PublicURL: "tcp://foo.internal:80"}, &parsedHostport{"tcp", "foo.internal", 80}, false},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			assert := assert.New(t)

			got, err := parseDialURL(test.endpoint)
			if test.wantErr {
				assert.Error(err)
				return
			}
			assert.NoError(err)
			assert.Equal(test.want, got)
		})
	}
}

// boundEndpoint is the BoundEndpoint the aggregator should produce for one
// endpoint projected into one target.
func boundEndpoint(endpointID, dialURL, scheme, service, namespace string, port int32) bindingsv1alpha1.BoundEndpoint {
	return bindingsv1alpha1.BoundEndpoint{
		Name: BoundEndpointName(endpointID, service, namespace),
		Spec: bindingsv1alpha1.BoundEndpointSpec{
			EndpointURL: dialURL,
			Scheme:      scheme,
			Target: bindingsv1alpha1.EndpointTarget{
				Service:   service,
				Namespace: namespace,
				Port:      port,
				Protocol:  "TCP",
			},
		},
		Status: bindingsv1alpha1.BoundEndpointStatus{
			Endpoints: []bindingsv1alpha1.BindingEndpoint{
				{Ref: ngrok.Ref{ID: endpointID}},
			},
		},
	}
}

func keyed(endpoints ...bindingsv1alpha1.BoundEndpoint) AggregatedEndpoints {
	aggregated := AggregatedEndpoints{}
	for _, endpoint := range endpoints {
		aggregated[endpoint.Name] = endpoint
	}
	return aggregated
}

func targets(targets ...ngrok.EndpointKubernetesTarget) *ngrok.EndpointKubernetes {
	return &ngrok.EndpointKubernetes{Targets: targets}
}

func Test_AggregateBindingEndpoints(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		endpoints []ngrok.Endpoint
		want      AggregatedEndpoints
		wantErr   bool
	}{
		{"empty", []ngrok.Endpoint{}, AggregatedEndpoints{}, false},
		{
			// Carrying kubernetes.targets is the signal to project. Every other
			// endpoint on the account is left alone, which is what keeps this
			// operator from projecting the whole account.
			name: "skips-endpoints-without-targets",
			endpoints: []ngrok.Endpoint{
				{ID: "ep_public", URL: "https://example.ngrok.app"},
				{ID: "ep_empty", URL: "tcp://foo.internal:80", Kubernetes: targets()},
			},
			want:    AggregatedEndpoints{},
			wantErr: false,
		},
		{
			name: "single-target",
			endpoints: []ngrok.Endpoint{{
				ID:         "ep_123",
				URL:        "tcp://foo.internal:80",
				Kubernetes: targets(ngrok.EndpointKubernetesTarget{Service: "myservice", Namespace: "team-a", Port: 80}),
			}},
			want:    keyed(boundEndpoint("ep_123", "tcp://foo.internal:80", "tcp", "myservice", "team-a", 80)),
			wantErr: false,
		},
		{
			// The point of the target list: one endpoint, one dial identity,
			// several projections. Each gets its own BoundEndpoint, so each
			// gets its own forwarder port and Services.
			name: "fans-out-over-targets",
			endpoints: []ngrok.Endpoint{{
				ID:  "ep_123",
				URL: "tcp://foo.internal:80",
				Kubernetes: targets(
					ngrok.EndpointKubernetesTarget{Service: "myservice", Namespace: "team-a", Port: 80},
					ngrok.EndpointKubernetesTarget{Service: "myservice", Namespace: "team-b", Port: 80},
				),
			}},
			want: keyed(
				boundEndpoint("ep_123", "tcp://foo.internal:80", "tcp", "myservice", "team-a", 80),
				boundEndpoint("ep_123", "tcp://foo.internal:80", "tcp", "myservice", "team-b", 80),
			),
			wantErr: false,
		},
		{
			// Two endpoints projecting the same service.namespace pair would
			// have collided under the old URL-derived name.
			name: "distinct-endpoints-same-target",
			endpoints: []ngrok.Endpoint{
				{
					ID:         "ep_100",
					URL:        "tcp://foo.internal:80",
					Kubernetes: targets(ngrok.EndpointKubernetesTarget{Service: "myservice", Namespace: "team-a", Port: 80}),
				},
				{
					ID:         "ep_200",
					URL:        "tcp://bar.internal:80",
					Kubernetes: targets(ngrok.EndpointKubernetesTarget{Service: "myservice", Namespace: "team-b", Port: 80}),
				},
			},
			want: keyed(
				boundEndpoint("ep_100", "tcp://foo.internal:80", "tcp", "myservice", "team-a", 80),
				boundEndpoint("ep_200", "tcp://bar.internal:80", "tcp", "myservice", "team-b", 80),
			),
			wantErr: false,
		},
		{
			// A single unparseable endpoint must not abort the aggregation:
			// valid endpoints in the same batch are still returned, and the
			// failure is reported in the returned error.
			name: "skips-unparseable-and-keeps-valid",
			endpoints: []ngrok.Endpoint{
				{
					ID:         "ep_bad",
					Proto:      "tcp",
					URL:        "tcp://foo.internal",
					Kubernetes: targets(ngrok.EndpointKubernetesTarget{Service: "myservice", Namespace: "team-a", Port: 80}),
				},
				{
					ID:         "ep_good",
					URL:        "tcp://bar.internal:80",
					Kubernetes: targets(ngrok.EndpointKubernetesTarget{Service: "myservice", Namespace: "team-b", Port: 80}),
				},
			},
			want:    keyed(boundEndpoint("ep_good", "tcp://bar.internal:80", "tcp", "myservice", "team-b", 80)),
			wantErr: true,
		},
		{
			// A bad target loses only that projection, not the endpoint's
			// other targets.
			name: "skips-invalid-target-and-keeps-valid",
			endpoints: []ngrok.Endpoint{{
				ID:  "ep_123",
				URL: "tcp://foo.internal:80",
				Kubernetes: targets(
					ngrok.EndpointKubernetesTarget{Service: "", Namespace: "team-a", Port: 80},
					ngrok.EndpointKubernetesTarget{Service: "myservice", Namespace: "team-b", Port: 0},
					ngrok.EndpointKubernetesTarget{Service: "myservice", Namespace: "team-c", Port: 80},
				),
			}},
			want:    keyed(boundEndpoint("ep_123", "tcp://foo.internal:80", "tcp", "myservice", "team-c", 80)),
			wantErr: true,
		},
		{
			// The API rejects a repeated pair, so reaching this means the
			// endpoint came from an older control plane. Two BoundEndpoints
			// fighting over one Service is worse than one missing Service.
			name: "skips-duplicate-target",
			endpoints: []ngrok.Endpoint{{
				ID:  "ep_123",
				URL: "tcp://foo.internal:80",
				Kubernetes: targets(
					ngrok.EndpointKubernetesTarget{Service: "myservice", Namespace: "team-a", Port: 80},
					ngrok.EndpointKubernetesTarget{Service: "myservice", Namespace: "team-a", Port: 8080},
				),
			}},
			want:    keyed(boundEndpoint("ep_123", "tcp://foo.internal:80", "tcp", "myservice", "team-a", 80)),
			wantErr: true,
		},
		{
			// The target port is the client-facing Service port and is
			// independent of the port the forwarder dials.
			name: "target-port-differs-from-dial-port",
			endpoints: []ngrok.Endpoint{{
				ID:         "ep_123",
				URL:        "tcp://redis.internal:6379",
				Kubernetes: targets(ngrok.EndpointKubernetesTarget{Service: "myredis", Namespace: "team-a", Port: 6379}),
			}},
			want:    keyed(boundEndpoint("ep_123", "tcp://redis.internal:6379", "tcp", "myredis", "team-a", 6379)),
			wantErr: false,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			assert := assert.New(t)

			got, err := AggregateBindingEndpoints(context.Background(), test.endpoints)
			if test.wantErr {
				assert.Error(err)
			} else {
				assert.NoError(err)
			}
			// Even on partial failure we still expect the valid endpoints
			// to be aggregated, so compare the map unconditionally.
			assert.Equal(test.want, got)
		})
	}
}

func Test_BoundEndpointName(t *testing.T) {
	t.Parallel()
	assert := assert.New(t)

	base := BoundEndpointName("ep_123", "myservice", "team-a")
	assert.Equal(base, BoundEndpointName("ep_123", "myservice", "team-a"), "name must be stable")

	// The name is a Kubernetes object name, so it has to stay a DNS label.
	assert.LessOrEqual(len(base), 63)
	assert.Regexp("^[a-z]([-a-z0-9]*[a-z0-9])?$", base)

	// Every component has to change the name, or two projections would collide
	// on one BoundEndpoint and one forwarder port.
	assert.NotEqual(base, BoundEndpointName("ep_456", "myservice", "team-a"))
	assert.NotEqual(base, BoundEndpointName("ep_123", "other", "team-a"))
	assert.NotEqual(base, BoundEndpointName("ep_123", "myservice", "team-b"))
}
