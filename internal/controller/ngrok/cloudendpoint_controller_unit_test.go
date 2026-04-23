package ngrok

import (
	"testing"

	"github.com/ngrok/ngrok-api-go/v7"
	ngrokv1alpha1 "github.com/ngrok/ngrok-operator/api/ngrok/v1alpha1"
	"github.com/stretchr/testify/assert"
)

func TestEndpointNeedsUpdate(t *testing.T) {
	baseEndpoint := func() *ngrok.Endpoint {
		return &ngrok.Endpoint{
			URL:            "https://example.ngrok.app",
			Description:    "Created by the ngrok-operator",
			Metadata:       `{"owned-by":"ngrok-operator"}`,
			TrafficPolicy:  `{"on_http_request":[]}`,
			Bindings:       []string{"public"},
			PoolingEnabled: false,
		}
	}

	baseSpec := func() ngrokv1alpha1.CloudEndpointSpec {
		return ngrokv1alpha1.CloudEndpointSpec{
			URL:            "https://example.ngrok.app",
			Description:    "Created by the ngrok-operator",
			Metadata:       `{"owned-by":"ngrok-operator"}`,
			Bindings:       []string{"public"},
			PoolingEnabled: new(false),
		}
	}

	basePolicy := `{"on_http_request":[]}`

	tests := []struct {
		name     string
		endpoint *ngrok.Endpoint
		spec     ngrokv1alpha1.CloudEndpointSpec
		policy   string
		want     bool
	}{
		{
			name:     "no update needed when everything matches",
			endpoint: baseEndpoint(),
			spec:     baseSpec(),
			policy:   basePolicy,
			want:     false,
		},
		{
			name:     "update needed when URL changes",
			endpoint: baseEndpoint(),
			spec: func() ngrokv1alpha1.CloudEndpointSpec {
				s := baseSpec()
				s.URL = "https://other.ngrok.app"
				return s
			}(),
			policy: basePolicy,
			want:   true,
		},
		{
			name:     "update needed when description changes",
			endpoint: baseEndpoint(),
			spec: func() ngrokv1alpha1.CloudEndpointSpec {
				s := baseSpec()
				s.Description = "new description"
				return s
			}(),
			policy: basePolicy,
			want:   true,
		},
		{
			name:     "update needed when metadata changes",
			endpoint: baseEndpoint(),
			spec: func() ngrokv1alpha1.CloudEndpointSpec {
				s := baseSpec()
				s.Metadata = `{"owned-by":"someone-else"}`
				return s
			}(),
			policy: basePolicy,
			want:   true,
		},
		{
			name:     "update needed when traffic policy changes",
			endpoint: baseEndpoint(),
			spec:     baseSpec(),
			policy:   `{"on_http_request":[{"actions":[]}]}`,
			want:     true,
		},
		{
			name:     "update needed when bindings change",
			endpoint: baseEndpoint(),
			spec: func() ngrokv1alpha1.CloudEndpointSpec {
				s := baseSpec()
				s.Bindings = []string{"internal"}
				return s
			}(),
			policy: basePolicy,
			want:   true,
		},
		{
			name:     "update needed when pooling enabled changes",
			endpoint: baseEndpoint(),
			spec: func() ngrokv1alpha1.CloudEndpointSpec {
				s := baseSpec()
				s.PoolingEnabled = new(true)
				return s
			}(),
			policy: basePolicy,
			want:   true,
		},
		{
			name:     "nil pooling enabled in spec skips comparison",
			endpoint: baseEndpoint(),
			spec: func() ngrokv1alpha1.CloudEndpointSpec {
				s := baseSpec()
				s.PoolingEnabled = nil
				return s
			}(),
			policy: basePolicy,
			want:   false,
		},
		{
			name: "nil and empty bindings are equal",
			endpoint: func() *ngrok.Endpoint {
				e := baseEndpoint()
				e.Bindings = nil
				return e
			}(),
			spec: func() ngrokv1alpha1.CloudEndpointSpec {
				s := baseSpec()
				s.Bindings = nil
				return s
			}(),
			policy: basePolicy,
			want:   false,
		},
		{
			name: "empty slice and nil bindings are equal",
			endpoint: func() *ngrok.Endpoint {
				e := baseEndpoint()
				e.Bindings = []string{}
				return e
			}(),
			spec: func() ngrokv1alpha1.CloudEndpointSpec {
				s := baseSpec()
				s.Bindings = nil
				return s
			}(),
			policy: basePolicy,
			want:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := endpointNeedsUpdate(tt.endpoint, tt.spec, tt.policy)
			assert.Equal(t, tt.want, got)
		})
	}
}
