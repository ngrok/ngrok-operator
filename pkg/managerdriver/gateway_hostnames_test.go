package managerdriver

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"k8s.io/utils/ptr"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"
)

func TestIntersectHostnames(t *testing.T) {
	tests := []struct {
		listener, route string
		want            string
		ok              bool
	}{
		{"", "", "", true},
		{"", "foo.example.com", "foo.example.com", true},
		{"foo.example.com", "", "foo.example.com", true},
		{"foo.example.com", "foo.example.com", "foo.example.com", true},
		{"foo.example.com", "bar.example.com", "", false},
		{"*.example.com", "foo.example.com", "foo.example.com", true},
		{"*.example.com", "foo.bar.example.com", "foo.bar.example.com", true},
		{"*.example.com", "example.com", "", false},
		{"foo.example.com", "*.example.com", "foo.example.com", true},
		{"example.com", "*.example.com", "", false},
		{"*.example.com", "*.bar.example.com", "*.bar.example.com", true},
		{"*.bar.example.com", "*.example.com", "*.bar.example.com", true},
		{"*.example.com", "*.example.org", "", false},
		{"*.example.com", "*.example.com", "*.example.com", true},
	}
	for _, tt := range tests {
		t.Run(tt.listener+"|"+tt.route, func(t *testing.T) {
			got, ok := intersectHostnames(tt.listener, tt.route)
			assert.Equal(t, tt.ok, ok)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestHostnameCovers(t *testing.T) {
	tests := []struct {
		pattern, host string
		want          bool
	}{
		{"", "anything.example.com", true},
		{"foo.example.com", "foo.example.com", true},
		{"foo.example.com", "bar.example.com", false},
		{"*.example.com", "foo.example.com", true},
		{"*.example.com", "a.b.example.com", true},
		{"*.example.com", "example.com", false},
		{"*.example.com", "*.foo.example.com", true},
		{"*.foo.example.com", "*.example.com", false},
	}
	for _, tt := range tests {
		t.Run(tt.pattern+"|"+tt.host, func(t *testing.T) {
			assert.Equal(t, tt.want, hostnameCovers(tt.pattern, tt.host))
		})
	}
}

func TestGatewayHostnames(t *testing.T) {
	t.Run("Map appends the conformance suffix", func(t *testing.T) {
		h := GatewayHostnames{ConformanceSuffix: "conf.test"}
		assert.Equal(t, "example.com.conf.test", h.Map("example.com"))
		assert.Equal(t, "*.example.com.conf.test", h.Map("*.example.com"))
		assert.Equal(t, "", h.Map(""))
		assert.Equal(t, "example.com", GatewayHostnames{}.Map("example.com"))
	})

	t.Run("Listener maps the hostname and treats none as every hostname", func(t *testing.T) {
		h := GatewayHostnames{ConformanceSuffix: "conf.test"}
		assert.Equal(t, "", h.Listener(gatewayv1.Listener{}))
		assert.Equal(t, "a.com.conf.test", h.Listener(gatewayv1.Listener{Hostname: ptr.To(gatewayv1.Hostname("a.com"))}))
	})

}
