package v1alpha1

import (
	"testing"

	"github.com/ngrok/ngrok-api-go/v6"
)

func TestHTTPSEdgeEqual(t *testing.T) {
	cases := []struct {
		name     string
		a        *HTTPSEdge
		b        *ngrok.HTTPSEdge
		expected bool
	}{
		{
			name:     "nils",
			a:        nil,
			b:        nil,
			expected: true,
		},
		{
			name:     "a nil",
			a:        nil,
			b:        &ngrok.HTTPSEdge{},
			expected: false,
		},
		{
			name:     "b nil",
			a:        &HTTPSEdge{},
			b:        nil,
			expected: false,
		},
		{
			name: "metadata different",
			a: &HTTPSEdge{
				Spec: HTTPSEdgeSpec{
					ngrokAPICommon: ngrokAPICommon{
						Metadata: "a",
					},
				},
			},
			b: &ngrok.HTTPSEdge{
				Metadata: "b",
			},
			expected: false,
		},
		{
			name: "metadata same",
			a: &HTTPSEdge{
				Spec: HTTPSEdgeSpec{
					ngrokAPICommon: ngrokAPICommon{
						Metadata: "a",
					},
				},
			},
			b: &ngrok.HTTPSEdge{
				Metadata: "a",
			},
			expected: true,
		},
		{
			name: "hostports different",
			a: &HTTPSEdge{
				Spec: HTTPSEdgeSpec{
					Hostports: []string{"a"},
				},
			},
			b: &ngrok.HTTPSEdge{
				Hostports: []string{"b"},
			},
			expected: false,
		},
		{
			name: "hostports same",
			a: &HTTPSEdge{
				Spec: HTTPSEdgeSpec{
					Hostports: []string{"a"},
				},
			},
			b: &ngrok.HTTPSEdge{
				Hostports: []string{"a"},
			},
			expected: true,
		},
		{
			name: "tls termination both nil",
			a: &HTTPSEdge{
				Spec: HTTPSEdgeSpec{
					TLSTermination: nil,
				},
			},
			b: &ngrok.HTTPSEdge{
				TlsTermination: nil,
			},
			expected: true,
		},
		{
			name: "tls termination a nil",
			a: &HTTPSEdge{
				Spec: HTTPSEdgeSpec{
					TLSTermination: nil,
				},
			},
			b: &ngrok.HTTPSEdge{
				TlsTermination: &ngrok.EndpointTLSTermination{},
			},
			expected: false,
		},
		{
			name: "tls termination b nil",
			a: &HTTPSEdge{
				Spec: HTTPSEdgeSpec{
					TLSTermination: &EndpointTLSTerminationAtEdge{},
				},
			},
			b: &ngrok.HTTPSEdge{
				TlsTermination: nil,
			},
			expected: false,
		},
		{
			name: "tls termination both empty",
			a: &HTTPSEdge{
				Spec: HTTPSEdgeSpec{
					TLSTermination: nil,
				},
			},
			b: &ngrok.HTTPSEdge{
				TlsTermination: nil,
			},
			expected: true,
		},
		{
			name: "tls termination different",
			a: &HTTPSEdge{
				Spec: HTTPSEdgeSpec{
					TLSTermination: &EndpointTLSTerminationAtEdge{
						MinVersion: "a",
					},
				},
			},
			b: &ngrok.HTTPSEdge{
				TlsTermination: &ngrok.EndpointTLSTermination{
					MinVersion: ngrok.String("b"),
				},
			},
			expected: false,
		},
		{
			name: "tls termination same",
			a: &HTTPSEdge{
				Spec: HTTPSEdgeSpec{
					TLSTermination: &EndpointTLSTerminationAtEdge{
						MinVersion: "a",
					},
				},
			},
			b: &ngrok.HTTPSEdge{
				TlsTermination: &ngrok.EndpointTLSTermination{
					MinVersion: ngrok.String("a"),
				},
			},
			expected: true,
		},
		{
			name: "mtls both nil",
			a: &HTTPSEdge{
				Spec: HTTPSEdgeSpec{
					MutualTLS: nil,
				},
			},
			b: &ngrok.HTTPSEdge{
				MutualTls: nil,
			},
			expected: true,
		},
		{
			name: "mtls a nil",
			a: &HTTPSEdge{
				Spec: HTTPSEdgeSpec{
					MutualTLS: nil,
				},
			},
			b: &ngrok.HTTPSEdge{
				MutualTls: &ngrok.EndpointMutualTLS{},
			},
			expected: false,
		},
		{
			name: "mtls b nil",
			a: &HTTPSEdge{
				Spec: HTTPSEdgeSpec{
					MutualTLS: &EndpointMutualTLS{},
				},
			},
			b: &ngrok.HTTPSEdge{MutualTls: nil},
		},
		{
			name: "tls termination different",
			a: &HTTPSEdge{
				Spec: HTTPSEdgeSpec{
					MutualTLS: &EndpointMutualTLS{
						CertificateAuthorities: []string{"a"},
					},
				},
			},
			b: &ngrok.HTTPSEdge{
				MutualTls: &ngrok.EndpointMutualTLS{
					CertificateAuthorities: []ngrok.Ref{{
						ID: "b",
					}},
				},
			},
			expected: false,
		},
		{
			name: "tls termination same",
			a: &HTTPSEdge{
				Spec: HTTPSEdgeSpec{
					MutualTLS: &EndpointMutualTLS{
						CertificateAuthorities: []string{"a123"},
					},
				},
			},
			b: &ngrok.HTTPSEdge{
				MutualTls: &ngrok.EndpointMutualTLS{
					CertificateAuthorities: []ngrok.Ref{{
						ID: "a123",
					}},
				},
			},
			expected: true,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if c.a.Equal(c.b) != c.expected {
				t.Errorf("expected %v, got %v", c.expected, !c.expected)
			}
		})
	}
}
