package managerdriver

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"

	ngrokv1 "github.com/ngrok/ngrok-operator/api/ngrok/v1"
)

func TestGatewayNeedsAssignedDomain(t *testing.T) {
	listener := func(protocol gatewayv1.ProtocolType, hostname string) gatewayv1.Listener {
		l := gatewayv1.Listener{Name: "l", Protocol: protocol, Port: 443}
		if hostname != "" {
			l.Hostname = new(gatewayv1.Hostname(hostname))
		}
		return l
	}
	tests := []struct {
		name      string
		listeners []gatewayv1.Listener
		addresses []gatewayv1.GatewaySpecAddress
		want      bool
	}{
		{name: "HTTP listener without a hostname", listeners: []gatewayv1.Listener{listener(gatewayv1.HTTPProtocolType, "")}, want: true},
		{name: "HTTPS listener without a hostname", listeners: []gatewayv1.Listener{listener(gatewayv1.HTTPSProtocolType, "")}, want: true},
		{name: "TLS listener without a hostname", listeners: []gatewayv1.Listener{listener(gatewayv1.TLSProtocolType, "")}, want: true},
		{name: "every listener has a hostname", listeners: []gatewayv1.Listener{listener(gatewayv1.HTTPProtocolType, "a.com")}, want: false},
		{name: "TCP listener without a hostname", listeners: []gatewayv1.Listener{listener(gatewayv1.TCPProtocolType, "")}, want: false},
		{
			name:      "spec.addresses names the endpoints",
			listeners: []gatewayv1.Listener{listener(gatewayv1.HTTPProtocolType, "")},
			addresses: []gatewayv1.GatewaySpecAddress{{Value: "a.com"}},
			want:      false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gw := &gatewayv1.Gateway{Spec: gatewayv1.GatewaySpec{Listeners: tt.listeners, Addresses: tt.addresses}}
			assert.Equal(t, tt.want, gatewayNeedsAssignedDomain(gw))
		})
	}
}

func TestAssignedDomains(t *testing.T) {
	now := time.Now()
	domain := func(uid, name string, created time.Time) ngrokv1.Domain {
		return ngrokv1.Domain{
			Labels: map[string]string{GatewayUIDLabel: uid}, CreationTimestamp: metav1.NewTime(created),
			Spec: ngrokv1.DomainSpec{Domain: name},
		}
	}
	claim := func(uid, assignedURL string) ngrokv1.AgentEndpoint {
		return ngrokv1.AgentEndpoint{
			Labels: map[string]string{GatewayUIDLabel: uid},
			Status: ngrokv1.AgentEndpointStatus{AssignedURL: assignedURL},
		}
	}
	tests := []struct {
		name    string
		domains []ngrokv1.Domain
		claims  []ngrokv1.AgentEndpoint
		want    map[types.UID]string
	}{
		{
			name: "nothing assigned",
			want: map[types.UID]string{},
		},
		{
			name:   "a claim ngrok has not answered yet",
			claims: []ngrokv1.AgentEndpoint{claim("gw1", "")},
			want:   map[types.UID]string{},
		},
		{
			name:   "the domain ngrok gave the claim",
			claims: []ngrokv1.AgentEndpoint{claim("gw1", "https://abc123.ngrok.app")},
			want:   map[types.UID]string{"gw1": "abc123.ngrok.app"},
		},
		{
			name:    "the pinned Domain wins over a claim",
			domains: []ngrokv1.Domain{domain("gw1", "abc123.ngrok.app", now)},
			claims:  []ngrokv1.AgentEndpoint{claim("gw1", "https://def456.ngrok.app")},
			want:    map[types.UID]string{"gw1": "abc123.ngrok.app"},
		},
		{
			name: "the oldest Domain wins",
			domains: []ngrokv1.Domain{
				domain("gw1", "newer.ngrok.app", now),
				domain("gw1", "older.ngrok.app", now.Add(-time.Hour)),
			},
			want: map[types.UID]string{"gw1": "older.ngrok.app"},
		},
		{
			name:    "each Gateway gets its own",
			domains: []ngrokv1.Domain{domain("gw1", "abc123.ngrok.app", now)},
			claims:  []ngrokv1.AgentEndpoint{claim("gw2", "https://def456.ngrok.app")},
			want:    map[types.UID]string{"gw1": "abc123.ngrok.app", "gw2": "def456.ngrok.app"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, assignedDomains(tt.domains, tt.claims))
		})
	}
}

func TestGatewayToDomainsLabelsAssignedDomain(t *testing.T) {
	gw := &gatewayv1.Gateway{
		Name: "gw", Namespace: "default", UID: "gw1",
		Spec: gatewayv1.GatewaySpec{Listeners: []gatewayv1.Listener{
			{Name: "http", Protocol: gatewayv1.HTTPProtocolType, Port: 80},
			{Name: "named", Protocol: gatewayv1.HTTPProtocolType, Port: 80, Hostname: ptr.To(gatewayv1.Hostname("a.com"))},
		}},
	}
	h := GatewayHostnames{Assigned: map[types.UID]string{"gw1": "abc123.ngrok.app"}}

	domains := gatewayToDomains(gw, nil, h, nil)

	assert.Equal(t, map[string]string{GatewayUIDLabel: "gw1"}, domains["abc123.ngrok.app"].Labels)
	assert.Empty(t, domains["a.com"].Labels)
	assert.NotContains(t, gatewayToDomains(gw, nil, GatewayHostnames{}, nil), "")
}
