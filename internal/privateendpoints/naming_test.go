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
		{"http default port", "http://foo.internal", ngrokv1.PrivateEndpointSpec{URL: "http://foo.internal", Scheme: "http", Hostname: "foo.internal", Port: 80}, false},
		{"https default port", "https://foo.ngrok.direct", ngrokv1.PrivateEndpointSpec{URL: "https://foo.ngrok.direct", Scheme: "https", Hostname: "foo.ngrok.direct", Port: 443}, false},
		{"tls default port", "tls://foo.internal", ngrokv1.PrivateEndpointSpec{URL: "tls://foo.internal", Scheme: "tls", Hostname: "foo.internal", Port: 443}, false},
		{"tcp explicit port", "tcp://bar.internal:6379", ngrokv1.PrivateEndpointSpec{URL: "tcp://bar.internal:6379", Scheme: "tcp", Hostname: "bar.internal", Port: 6379}, false},
		{"uppercase host normalized", "http://FOO.Internal:8080", ngrokv1.PrivateEndpointSpec{URL: "http://FOO.Internal:8080", Scheme: "http", Hostname: "foo.internal", Port: 8080}, false},
		{"tcp without port", "tcp://bar.internal", ngrokv1.PrivateEndpointSpec{}, true},
		{"unknown scheme", "udp://bar.internal:53", ngrokv1.PrivateEndpointSpec{}, true},
		{"port out of range", "tcp://bar.internal:70000", ngrokv1.PrivateEndpointSpec{}, true},
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
		{"foo.internal", true},
		{"a.b.internal", true},
		{"foo.ngrok.direct", true},
		{"internal", false},
		{"foo.ngrok.app", false},
		{"notinternal", false},
		{"foo.internal.example.com", false},
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
		{"foo.internal", "foo-internal", false},
		{"FOO.Internal.", "foo-internal", false},
		{"foo.ngrok.direct", "foo-ngrok-direct", false},
		{"my-app2.internal", "my-app2-internal", false},
		{"api.foo.internal", "", true}, // multi-label: Service names can't hold dots
		{"123foo.internal", "", true},  // Service names must start with a letter
		{"foo-.internal", "", true},    // or end with an alphanumeric
		{"foo_bar.internal", "", true}, // underscores aren't allowed
		{"foo.ngrok.app", "", true},    // not a private TLD
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

func TestNames(t *testing.T) {
	assert.Equal(t, CRName("http://foo.internal"), CRName("http://foo.internal"))
	assert.NotEqual(t, CRName("http://foo.internal"), CRName("https://foo.internal"))
	assert.Regexp(t, `^pe-[0-9a-f]{16}$`, CRName("http://foo.internal"))
	assert.Equal(t, HostKey("FOO.internal"), HostKey("foo.internal."))
	assert.Regexp(t, `^[0-9a-f]{16}$`, HostKey("foo.internal"))
}
