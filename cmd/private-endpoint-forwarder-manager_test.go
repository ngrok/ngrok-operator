package cmd

import (
	"context"
	"errors"
	"net"
	"sync/atomic"
	"testing"
	"time"

	"github.com/go-logr/logr"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	ngrokv1 "github.com/ngrok/ngrok-operator/api/ngrok/v1"
	privateendpointscontroller "github.com/ngrok/ngrok-operator/internal/controller/privateendpoints"
	"github.com/ngrok/ngrok-operator/internal/privateendpoints/forwarder"
	"github.com/ngrok/ngrok-operator/pkg/bindingsdriver"
)

func TestForwarderReadyz(t *testing.T) {
	tests := []struct {
		name      string
		sync      bool
		holdPort  bool
		wantReady bool
	}{
		{name: "not ready before the first sync"},
		{name: "ready once synced with every listener bound", sync: true, wantReady: true},
		{name: "not ready while a wanted port is held by someone else", sync: true, holdPort: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			l, err := net.Listen("tcp", ":0") // wildcard, like bindingsdriver
			require.NoError(t, err)
			port := int32(l.Addr().(*net.TCPAddr).Port)
			if tt.holdPort {
				defer l.Close()
			} else {
				require.NoError(t, l.Close())
			}

			s := runtime.NewScheme()
			require.NoError(t, ngrokv1.AddToScheme(s))
			c := fake.NewClientBuilder().WithScheme(s).WithObjects(&ngrokv1.PrivateEndpoint{
				Name: "a", Namespace: "op",
				Spec:   ngrokv1.PrivateEndpointSpec{Hostname: "a.ngrok.direct", Port: 80},
				Status: ngrokv1.PrivateEndpointStatus{ClusterIP: "10.0.0.1", ForwarderPort: port},
			}).Build()
			table := forwarder.NewTable()
			r := &privateendpointscontroller.ForwarderReconciler{Client: c, Namespace: "op", Table: table, Listeners: bindingsdriver.New(), Log: logr.Discard()}
			defer r.Close()
			if tt.sync {
				_ = r.Sync(context.Background())
			}

			err = forwarderReadyz(table, r)(nil)
			if tt.wantReady {
				assert.NoError(t, err)
			} else {
				assert.Error(t, err)
			}
		})
	}
}

type fakeSessionDialer struct {
	connectErrs []error
	connects    atomic.Int32
	done        chan struct{}
	err         atomic.Value
}

func (f *fakeSessionDialer) Connect(context.Context) error {
	n := int(f.connects.Add(1)) - 1
	if n < len(f.connectErrs) {
		return f.connectErrs[n]
	}
	return nil
}

func (f *fakeSessionDialer) Done() <-chan struct{} { return f.done }

func (f *fakeSessionDialer) Err() error {
	err, _ := f.err.Load().(error)
	return err
}

func (f *fakeSessionDialer) fail(err error) {
	f.err.Store(err)
	close(f.done)
}

func TestDialSession(t *testing.T) {
	errUnauthorized := errors.New("private-dial /session status 401: revoked")
	tests := []struct {
		name        string
		connectErrs []error
		fatal       error
		wantErr     bool
	}{
		{name: "ready after connecting and stays ready until shutdown"},
		{name: "retries failed initial connects without exiting", connectErrs: []error{errors.New("timeout"), errUnauthorized}},
		{name: "exits when the session ends permanently", fatal: errUnauthorized, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := &fakeSessionDialer{connectErrs: tt.connectErrs, done: make(chan struct{})}
			s := &dialSession{dialer: d, log: logr.Discard(), maxBackoff: time.Millisecond}
			require.Error(t, s.readyz(nil), "not ready before the first connect")

			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			startErr := make(chan error, 1)
			go func() { startErr <- s.Start(ctx) }()

			require.Eventually(t, func() bool { return s.readyz(nil) == nil }, time.Second, time.Millisecond)
			assert.Equal(t, int32(len(tt.connectErrs)+1), d.connects.Load())

			if tt.fatal != nil {
				d.fail(tt.fatal)
			} else {
				cancel()
			}
			err := <-startErr
			if tt.wantErr {
				assert.ErrorIs(t, err, tt.fatal)
				assert.ErrorIs(t, s.readyz(nil), tt.fatal)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestDialSessionNotReadyWhileConnectFails(t *testing.T) {
	d := &fakeSessionDialer{connectErrs: []error{errors.New("a"), errors.New("b"), errors.New("c")}, done: make(chan struct{})}
	s := &dialSession{dialer: d, log: logr.Discard(), maxBackoff: time.Hour}
	ctx, cancel := context.WithCancel(context.Background())
	startErr := make(chan error, 1)
	go func() { startErr <- s.Start(ctx) }()

	require.Eventually(t, func() bool { return d.connects.Load() == 1 }, time.Second, time.Millisecond)
	assert.Error(t, s.readyz(nil))
	cancel()
	assert.NoError(t, <-startErr)
	assert.Error(t, s.readyz(nil))
}
