package ngrokapi

import (
	"testing"

	"github.com/stretchr/testify/assert"

	v6 "github.com/ngrok/ngrok-api-go/v6"
	bindingsv1alpha1 "github.com/ngrok/ngrok-operator/api/bindings/v1alpha1"
)

func Test_parseHostport(t *testing.T) {
	t.Parallel()
	assert := assert.New(t)

	tests := []struct {
		name    string
		proto   string
		url     string
		want    *parsedHostport
		wantErr bool
	}{
		{"empty", "", "", nil, true},
		{"invalid", "https", "does-not-parse", nil, true},
		// We trust the api to only support specific schemes
		// {"invalid-scheme", "scheme", "scheme://test.not-working", nil, true},
		{"mismatched-scheme", "tls", "https://test.not-working", nil, true},
		{"missing-tcp-port", "tcp", "tcp://test.not-working", nil, true},
		// with defaults
		{"simple", "", "service.namespace", &parsedHostport{"https", "service", "namespace", 443}, false},
		{"full", "tcp", "tcp://service.namespace:1234", &parsedHostport{"tcp", "service", "namespace", 1234}, false},
		{"http-no-port", "http", "service.namespace", &parsedHostport{"http", "service", "namespace", 80}, false},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			got, err := parseHostport(test.proto, test.url)
			if test.wantErr {
				assert.Error(err)
				return
			}
			assert.NoError(err)
			assert.Equal(test.want, got)
		})
	}
}

func Test_AggregateBindingEndpoints(t *testing.T) {
	t.Parallel()
	assert := assert.New(t)

	tests := []struct {
		name      string
		endpoints []v6.Endpoint
		want      AggregatedEndpoints
		wantErr   bool
	}{
		{"empty", []v6.Endpoint{}, AggregatedEndpoints{}, false},
		{
			name: "single",
			endpoints: []v6.Endpoint{
				{ID: "ep_123", PublicURL: "https://service1.namespace1"},
			},
			want: AggregatedEndpoints{
				"https://service1.namespace1:443": {
					Spec: bindingsv1alpha1.EndpointBindingSpec{
						FQRI:   "https://service1.namespace1:443",
						Scheme: "https",
						Target: bindingsv1alpha1.EndpointTarget{
							Service:   "service1",
							Namespace: "namespace1",
							Port:      443,
							Protocol:  "TCP",
						},
					},
					Status: bindingsv1alpha1.EndpointBindingStatus{
						Endpoints: []bindingsv1alpha1.BindingEndpoint{
							{Ref: v6.Ref{ID: "ep_123"}},
						},
					},
				},
			},
			wantErr: false,
		},
		{
			name: "full",
			endpoints: []v6.Endpoint{
				{ID: "ep_100", PublicURL: "https://service1.namespace1"},
				{ID: "ep_101", PublicURL: "https://service1.namespace1"},
				{ID: "ep_102", PublicURL: "https://service1.namespace1"},
				{ID: "ep_200", PublicURL: "tcp://service2.namespace2:2020"},
				{ID: "ep_201", PublicURL: "tcp://service2.namespace2:2020"},
				{ID: "ep_300", PublicURL: "service3.namespace3"},
				{ID: "ep_400", PublicURL: "http://service4.namespace4:8080"},
			},
			want: AggregatedEndpoints{
				"https://service1.namespace1:443": {
					Spec: bindingsv1alpha1.EndpointBindingSpec{
						FQRI:   "https://service1.namespace1:443",
						Scheme: "https",
						Target: bindingsv1alpha1.EndpointTarget{
							Service:   "service1",
							Namespace: "namespace1",
							Port:      443,
							Protocol:  "TCP",
						},
					},
					Status: bindingsv1alpha1.EndpointBindingStatus{
						Endpoints: []bindingsv1alpha1.BindingEndpoint{
							{Ref: v6.Ref{ID: "ep_100"}},
							{Ref: v6.Ref{ID: "ep_101"}},
							{Ref: v6.Ref{ID: "ep_102"}},
						},
					},
				},
				"tcp://service2.namespace2:2020": {
					Spec: bindingsv1alpha1.EndpointBindingSpec{
						FQRI:   "tcp://service2.namespace2:2020",
						Scheme: "tcp",
						Target: bindingsv1alpha1.EndpointTarget{
							Service:   "service2",
							Namespace: "namespace2",
							Port:      2020,
							Protocol:  "TCP",
						},
					},
					Status: bindingsv1alpha1.EndpointBindingStatus{
						Endpoints: []bindingsv1alpha1.BindingEndpoint{
							{Ref: v6.Ref{ID: "ep_200"}},
							{Ref: v6.Ref{ID: "ep_201"}},
						},
					},
				},
				"https://service3.namespace3:443": {
					Spec: bindingsv1alpha1.EndpointBindingSpec{
						FQRI:   "https://service3.namespace3:443",
						Scheme: "https",
						Target: bindingsv1alpha1.EndpointTarget{
							Service:   "service3",
							Namespace: "namespace3",
							Port:      443,
							Protocol:  "TCP",
						},
					},
					Status: bindingsv1alpha1.EndpointBindingStatus{
						Endpoints: []bindingsv1alpha1.BindingEndpoint{
							{Ref: v6.Ref{ID: "ep_300"}},
						},
					},
				},
				"http://service4.namespace4:8080": {
					Spec: bindingsv1alpha1.EndpointBindingSpec{
						FQRI:   "http://service4.namespace4:8080",
						Scheme: "http",
						Target: bindingsv1alpha1.EndpointTarget{
							Service:   "service4",
							Namespace: "namespace4",
							Port:      8080,
							Protocol:  "TCP",
						},
					},
					Status: bindingsv1alpha1.EndpointBindingStatus{
						Endpoints: []bindingsv1alpha1.BindingEndpoint{
							{Ref: v6.Ref{ID: "ep_400"}},
						},
					},
				},
			},
			wantErr: false,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			got, err := AggregateBindingEndpoints(test.endpoints)
			if test.wantErr {
				assert.Error(err)
				return
			}
			assert.NoError(err)
			assert.Equal(test.want, got)
		})
	}
}
