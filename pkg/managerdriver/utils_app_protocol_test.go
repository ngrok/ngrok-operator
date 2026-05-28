package managerdriver

import (
	"testing"

	"github.com/go-logr/logr"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	common "github.com/ngrok/ngrok-operator/api/common/v1alpha1"
	"github.com/ngrok/ngrok-operator/internal/ir"
)

func TestKnownApplicationProtocols_AcceptsBothPrefixes(t *testing.T) {
	cases := map[string]common.ApplicationProtocol{
		"ngrok.com/http2":     common.ApplicationProtocol_HTTP2,
		"k8s.ngrok.com/http2": common.ApplicationProtocol_HTTP2,
		"kubernetes.io/h2c":   common.ApplicationProtocol_HTTP2,
		"http":                common.ApplicationProtocol_HTTP1,
	}
	for key, want := range cases {
		got, ok := knownApplicationProtocols[key]
		require.True(t, ok, "expected %q to be a known appProtocol", key)
		assert.Equal(t, want, got, "wrong protocol for %q", key)
	}
}

func TestGetProtoForServicePort_DualAnnotationPrefix(t *testing.T) {
	tests := []struct {
		name string
		anns map[string]string
		want ir.IRProtocol
	}{
		{
			name: "new prefix is honored",
			anns: map[string]string{AppProtocolsAnnotation: `{"https":"https"}`},
			want: ir.IRProtocol_HTTPS,
		},
		{
			name: "legacy prefix still works",
			anns: map[string]string{LegacyAppProtocolsAnnotation: `{"https":"https"}`},
			want: ir.IRProtocol_HTTPS,
		},
		{
			name: "new wins when both set",
			anns: map[string]string{
				AppProtocolsAnnotation:       `{"https":"https"}`,
				LegacyAppProtocolsAnnotation: `{"https":"http"}`,
			},
			want: ir.IRProtocol_HTTPS,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			svc := &corev1.Service{ObjectMeta: metav1.ObjectMeta{
				Name:        "svc",
				Namespace:   "default",
				Annotations: tc.anns,
			}}
			got, err := getProtoForServicePort(logr.Discard(), svc, "https", ir.IRProtocol_HTTP)
			require.NoError(t, err)
			assert.Equal(t, tc.want, got)
		})
	}
}
