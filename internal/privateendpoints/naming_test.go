package privateendpoints

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	ngrokv1 "github.com/ngrok/ngrok-operator/api/ngrok/v1"
)

func TestParseURL(t *testing.T) {
	tests := []struct {
		name    string
		raw     string
		want    ngrokv1.PrivateEndpointSpec
		wantErr bool
	}{
		{"http default port", "http://foo.ngrok.direct", ngrokv1.PrivateEndpointSpec{URL: "http://foo.ngrok.direct", Scheme: "http", Hostname: "foo.ngrok.direct", Port: 80}, false},
		{"https default port", "https://foo.ngrok.direct", ngrokv1.PrivateEndpointSpec{URL: "https://foo.ngrok.direct", Scheme: "https", Hostname: "foo.ngrok.direct", Port: 443}, false},
		{"tls default port", "tls://foo.ngrok.direct", ngrokv1.PrivateEndpointSpec{URL: "tls://foo.ngrok.direct", Scheme: "tls", Hostname: "foo.ngrok.direct", Port: 443}, false},
		{"tcp explicit port", "tcp://bar.ngrok.direct:6379", ngrokv1.PrivateEndpointSpec{URL: "tcp://bar.ngrok.direct:6379", Scheme: "tcp", Hostname: "bar.ngrok.direct", Port: 6379}, false},
		{"uppercase host normalized", "http://FOO.Ngrok.Direct:8080", ngrokv1.PrivateEndpointSpec{URL: "http://FOO.Ngrok.Direct:8080", Scheme: "http", Hostname: "foo.ngrok.direct", Port: 8080}, false},
		{"tcp without port", "tcp://bar.ngrok.direct", ngrokv1.PrivateEndpointSpec{}, true},
		{"unknown scheme", "udp://bar.ngrok.direct:53", ngrokv1.PrivateEndpointSpec{}, true},
		{"port out of range", "tcp://bar.ngrok.direct:70000", ngrokv1.PrivateEndpointSpec{}, true},
		{"garbage", "://", ngrokv1.PrivateEndpointSpec{}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseURL(tt.raw)
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestIsPrivateHostname(t *testing.T) {
	tests := []struct {
		host string
		want bool
	}{
		{"foo.ngrok.direct", true},
		{"a.b.ngrok.direct", true},
		{"FOO.Ngrok.Direct.", true},
		{"foo.internal", false}, // *.internal is not supported
		{"ngrok.direct", false},
		{"foo.ngrok.app", false},
		{"foongrok.direct", false},
		{"foo.ngrok.direct.example.com", false},
	}
	for _, tt := range tests {
		t.Run(tt.host, func(t *testing.T) {
			assert.Equal(t, tt.want, IsPrivateHostname(tt.host))
		})
	}
}

func TestServiceName(t *testing.T) {
	tests := []struct {
		host    string
		want    string
		wantErr bool
	}{
		{"foo.ngrok.direct", "foo-ngrok-direct", false},
		{"FOO.Ngrok.Direct.", "foo-ngrok-direct", false},
		{"foo.ngrok.direct", "foo-ngrok-direct", false},
		{"my-app2.ngrok.direct", "my-app2-ngrok-direct", false},
		{"api.foo.ngrok.direct", "", true}, // multi-label: Service names can't hold dots
		{"123foo.ngrok.direct", "", true},  // Service names must start with a letter
		{"foo-.ngrok.direct", "", true},    // or end with an alphanumeric
		{"foo_bar.ngrok.direct", "", true}, // underscores aren't allowed
		{"foo.ngrok.app", "", true},        // not a private TLD
		{"foo.internal", "", true},         // *.internal is not supported
		{strings.Repeat("a", 50) + ".ngrok.direct", strings.Repeat("a", 50) + "-ngrok-direct", false},
		{strings.Repeat("a", 51) + ".ngrok.direct", "", true}, // 51 + len("-ngrok-direct") > 63
	}
	for _, tt := range tests {
		t.Run(tt.host, func(t *testing.T) {
			got, err := ServiceName(tt.host)
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestCRName(t *testing.T) {
	tests := []struct {
		url    string
		want   string
		hashed bool
	}{
		{url: "http://foo.ngrok.direct", want: "foo.ngrok.direct-http-80"},
		{url: "http://FOO.ngrok.direct:80", want: "foo.ngrok.direct-http-80"}, // case and default port normalized
		{url: "https://foo.ngrok.direct", want: "foo.ngrok.direct-https-443"},
		{url: "tls://foo.ngrok.direct", want: "foo.ngrok.direct-tls-443"},
		{url: "tcp://bar.ngrok.direct:6379", want: "bar.ngrok.direct-tcp-6379"},
		{url: "tcp://foo_bar.ngrok.direct:6379", hashed: true}, // not a valid object name
	}
	for _, tt := range tests {
		t.Run(tt.url, func(t *testing.T) {
			spec, err := ParseURL(tt.url)
			require.NoError(t, err)
			got := CRName(spec)
			if tt.hashed {
				assert.Regexp(t, `^pe-[0-9a-f]{16}$`, got)
				return
			}
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestHostnameForService(t *testing.T) {
	for _, host := range []string{"foo.ngrok.direct", "my-app2.ngrok.direct", "a-ngrok-direct.ngrok.direct"} {
		name, err := ServiceName(host)
		require.NoError(t, err)
		got, ok := HostnameForService(name)
		assert.True(t, ok)
		assert.Equal(t, host, got)
	}
	for _, name := range []string{"kubernetes", "-ngrok-direct", "foo-ngrok-direct-x"} {
		_, ok := HostnameForService(name)
		assert.False(t, ok, name)
	}
}
