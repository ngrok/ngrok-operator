package agent

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"net/url"
	"testing"

	ngrokv1alpha1 "github.com/ngrok/ngrok-operator/api/ngrok/v1alpha1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.ngrok.com/ngrok/v2"
)

// fakeAgent stands in for the ngrok agent and the server side of its session:
// the session holds at most limit tunnels, and a tunnel's slot is freed only
// by a successful unbind.
type fakeAgent struct {
	ngrok.Agent
	limit      int
	tunnels    map[string]*fakeForwarder // tunnels the server still holds
	nextID     int
	peak       int   // most tunnels held at once
	forwardErr error // when set, Forward fails with it
	unbindErr  error // when set, Close fails and the server keeps the slot
}

func newFakeAgent(limit int) *fakeAgent {
	return &fakeAgent{limit: limit, tunnels: map[string]*fakeForwarder{}}
}

func (a *fakeAgent) Forward(context.Context, *ngrok.Upstream, ...ngrok.EndpointOption) (ngrok.EndpointForwarder, error) {
	if a.forwardErr != nil {
		return nil, a.forwardErr
	}
	if len(a.tunnels) >= a.limit {
		return nil, fmt.Errorf("Your account may not run more than %d tunnels over a single ngrok agent session. ERR_NGROK_324", a.limit)
	}
	a.nextID++
	f := &fakeForwarder{id: fmt.Sprintf("tn_%d", a.nextID), agent: a, pooled: true, done: make(chan struct{})}
	a.tunnels[f.id] = f
	a.peak = max(a.peak, len(a.tunnels))
	return f, nil
}

// serving reports whether f is still running and the server still holds it.
func (a *fakeAgent) serving(f *fakeForwarder) bool {
	return !isDone(f) && a.tunnels[f.id] == f
}

type fakeForwarder struct {
	ngrok.EndpointForwarder
	id     string
	agent  *fakeAgent
	pooled bool
	closed bool // Close was called
	done   chan struct{}
}

func (f *fakeForwarder) ID() string                             { return f.id }
func (f *fakeForwarder) PoolingEnabled() bool                   { return f.pooled }
func (f *fakeForwarder) URL() *url.URL                          { return &url.URL{Scheme: "https", Host: "example.ngrok.app"} }
func (f *fakeForwarder) Bindings() []string                     { return nil }
func (f *fakeForwarder) TrafficPolicy() string                  { return "" }
func (f *fakeForwarder) Metadata() string                       { return "" }
func (f *fakeForwarder) ProxyProtocol() ngrok.ProxyProtoVersion { return "" }
func (f *fakeForwarder) UpstreamURL() url.URL                   { return url.URL{} }
func (f *fakeForwarder) UpstreamProtocol() string               { return "" }
func (f *fakeForwarder) Done() <-chan struct{}                  { return f.done }
func (f *fakeForwarder) Close() error                           { return f.CloseWithContext(context.Background()) }

// CloseWithContext behaves like ngrok-go: it runs once, Done() closes even if
// the unbind fails, and a failed unbind leaves the slot held on the server.
func (f *fakeForwarder) CloseWithContext(context.Context) error {
	if f.closed {
		return nil
	}
	f.closed = true
	if !isDone(f) {
		close(f.done)
	}
	if f.agent.unbindErr != nil {
		return f.agent.unbindErr
	}
	delete(f.agent.tunnels, f.id)
	return nil
}

func newTestDriver(a *fakeAgent) *driver {
	return &driver{agent: a, forwarders: newEndpointForwarderMap(), done: make(chan bool)}
}

// specWith returns an endpoint spec; a different description makes it a
// different config, so the driver has to rebind.
func specWith(description string) ngrokv1alpha1.AgentEndpointSpec {
	return ngrokv1alpha1.AgentEndpointSpec{
		URL:         "https://example.ngrok.app",
		Upstream:    ngrokv1alpha1.EndpointUpstream{URL: "http://svc.default:80"},
		Description: description,
	}
}

func apply(d *driver, name, description string) error {
	_, err := d.CreateAgentEndpoint(context.Background(), name, specWith(description), "", nil, nil)
	return err
}

// current returns the forwarder the driver holds for name.
func current(t *testing.T, d *driver, name string) *fakeForwarder {
	t.Helper()
	epf, ok := d.forwarders.Get(name)
	require.True(t, ok)
	return epf.(*fakeForwarder)
}

func TestCreateAgentEndpointUpdateReplacesForwarder(t *testing.T) {
	a := newFakeAgent(10)
	d := newTestDriver(a)
	require.NoError(t, apply(d, "ep", "v0"))

	for i := 1; i <= 5; i++ {
		old := current(t, d, "ep")
		require.NoError(t, apply(d, "ep", fmt.Sprintf("v%d", i)))

		cur := current(t, d, "ep")
		assert.NotSame(t, old, cur)
		assert.True(t, old.closed, "old forwarder closed")
		assert.True(t, a.serving(cur))
	}
	assert.Len(t, a.tunnels, 1, "updates don't leak tunnels")
	assert.Equal(t, 2, a.peak, "pooled update binds the new tunnel before closing the old one")
}

// A failed update must leave the old endpoint serving and keep its config as
// the applied one, so a later retry of the same config still rebinds.
func TestCreateAgentEndpointFailedUpdateKeepsOldEndpoint(t *testing.T) {
	tests := []struct {
		name    string
		limit   int
		fail    func(*fakeAgent)
		recover func(*fakeAgent)
		wantErr string
	}{
		{
			name:    "invalid traffic policy",
			limit:   10,
			fail:    func(a *fakeAgent) { a.forwardErr = errors.New("invalid traffic policy: ERR_NGROK_2201") },
			recover: func(a *fakeAgent) { a.forwardErr = nil },
			wantErr: "ERR_NGROK_2201",
		},
		{
			name:    "session tunnel limit",
			limit:   1,
			fail:    func(*fakeAgent) {},
			recover: func(a *fakeAgent) { a.limit = 2 },
			wantErr: "ERR_NGROK_324",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			a := newFakeAgent(tc.limit)
			d := newTestDriver(a)
			require.NoError(t, apply(d, "ep", "old"))
			old := current(t, d, "ep")

			tc.fail(a)
			require.ErrorContains(t, apply(d, "ep", "new"), tc.wantErr)
			assert.Same(t, old, current(t, d, "ep"))
			assert.True(t, a.serving(old), "old endpoint still serving")
			cfg, _ := d.forwarders.Config("ep")
			assert.Equal(t, "old", cfg.spec.Description, "failed config not recorded as applied")

			tc.recover(a)
			require.NoError(t, apply(d, "ep", "new"))
			assert.NotSame(t, old, current(t, d, "ep"))
			assert.True(t, old.closed)
			assert.True(t, a.serving(current(t, d, "ep")))
		})
	}
}

// When unbinds fail, ngrok-go forgets the tunnel but the server keeps its
// slot, so updates slowly fill the session. The driver can't reclaim those
// slots; this checks it keeps serving from the last forwarder it bound.
func TestCreateAgentEndpointLostUnbindsKeepLastForwarder(t *testing.T) {
	a := newFakeAgent(3)
	d := newTestDriver(a)
	require.NoError(t, apply(d, "ep-0", "v0"))
	require.NoError(t, apply(d, "ep-1", "v0"))
	a.unbindErr = errors.New("muxado: stream reset")

	require.NoError(t, apply(d, "ep-0", "v1"), "takes the last free slot; the old one leaks")
	require.ErrorContains(t, apply(d, "ep-0", "v2"), "ERR_NGROK_324")

	assert.Len(t, a.tunnels, 3, "session full with only 2 endpoints")
	assert.True(t, a.serving(current(t, d, "ep-0")))
	assert.True(t, a.serving(current(t, d, "ep-1")))
}

func TestCreateAgentEndpointSkipsUnchangedRebind(t *testing.T) {
	certA := tls.Certificate{Certificate: [][]byte{[]byte("cert-a")}}
	certB := tls.Certificate{Certificate: [][]byte{[]byte("cert-b")}}

	type inputs struct {
		spec     ngrokv1alpha1.AgentEndpointSpec
		policy   string
		certs    []tls.Certificate
		agentTLS *AgentTLSTermination
	}
	// Built fresh for each call, like the controller does every reconcile.
	baseline := func() inputs {
		return inputs{
			spec:     specWith(""),
			certs:    []tls.Certificate{certA},
			agentTLS: &AgentTLSTermination{ServerCert: &certA},
		}
	}

	tests := []struct {
		name       string
		change     func(*inputs)
		serverDrop bool // the server dropped the old tunnel
		wantRebind bool
	}{
		{name: "unchanged config", change: func(*inputs) {}, wantRebind: false},
		{name: "spec changed", change: func(in *inputs) { in.spec.Description = "new" }, wantRebind: true},
		{name: "traffic policy changed", change: func(in *inputs) { in.policy = `{"on_http_request":[]}` }, wantRebind: true},
		{name: "client cert rotated", change: func(in *inputs) { in.certs = []tls.Certificate{certB} }, wantRebind: true},
		{name: "agent TLS server cert rotated", change: func(in *inputs) { in.agentTLS = &AgentTLSTermination{ServerCert: &certB} }, wantRebind: true},
		{name: "unchanged but old tunnel dropped", change: func(*inputs) {}, serverDrop: true, wantRebind: true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			a := newFakeAgent(10)
			d := newTestDriver(a)
			create := func(in inputs) {
				res, err := d.CreateAgentEndpoint(context.Background(), "ep", in.spec, in.policy, in.certs, in.agentTLS)
				require.NoError(t, err)
				assert.True(t, res.Ready)
			}

			create(baseline())
			first := current(t, d, "ep")
			if tc.serverDrop {
				close(first.done)
			}

			in := baseline()
			tc.change(&in)
			create(in)

			assert.Equal(t, tc.wantRebind, current(t, d, "ep") != first, "rebind")
			assert.Len(t, a.tunnels, 1)
		})
	}
}

// Only pooled endpoints can share a URL during an update. With a non-pooled
// endpoint, ngrok misroutes traffic while both are up, so the driver must stop
// the old one before binding its replacement.
func TestCreateAgentEndpointNonPooledStopsBeforeRebind(t *testing.T) {
	tests := []struct {
		name      string
		oldPooled bool
		wantPeak  int
	}{
		{name: "pooled: new tunnel binds before the old one closes", oldPooled: true, wantPeak: 2},
		{name: "non-pooled: old tunnel closes first", oldPooled: false, wantPeak: 1},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			a := newFakeAgent(10)
			d := newTestDriver(a)
			require.NoError(t, apply(d, "ep", "old"))
			old := current(t, d, "ep")
			old.pooled = tc.oldPooled

			require.NoError(t, apply(d, "ep", "new"))

			assert.Equal(t, tc.wantPeak, a.peak, "most tunnels held at once")
			assert.True(t, old.closed)
			assert.Len(t, a.tunnels, 1)
		})
	}
}
