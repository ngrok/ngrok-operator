package forwarder

import (
	"context"
	"fmt"
	"io"
	"net"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/go-logr/logr"
	"github.com/go-logr/logr/funcr"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	ngrokv1 "github.com/ngrok/ngrok-operator/api/ngrok/v1"
	"github.com/ngrok/ngrok-operator/pkg/bindingsdriver"
)

func peCR(name, ns, host string, port int32, ip string, fwd int32) *ngrokv1.PrivateEndpoint {
	return &ngrokv1.PrivateEndpoint{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: ns},
		Spec:       ngrokv1.PrivateEndpointSpec{Hostname: host, Port: port},
		Status:     ngrokv1.PrivateEndpointStatus{ClusterIP: ip, ForwarderPort: fwd},
	}
}

func newSyncer(t *testing.T, objs ...client.Object) (*Syncer, client.Client) {
	t.Helper()
	s := runtime.NewScheme()
	require.NoError(t, ngrokv1.AddToScheme(s))
	c := fake.NewClientBuilder().WithScheme(s).WithObjects(objs...).WithStatusSubresource(&ngrokv1.PrivateEndpoint{}).Build()
	return &Syncer{
		Client:       c,
		Namespace:    "op",
		Table:        NewTable(),
		Listeners:    bindingsdriver.New(),
		Dial:         echoDial,
		DrainTimeout: time.Second,
		Log:          logr.Discard(),
	}, c
}

func dialPort(t *testing.T, port int32) (net.Conn, error) {
	t.Helper()
	return net.DialTimeout("tcp", fmt.Sprintf("127.0.0.1:%d", port), time.Second)
}

func TestSyncerSync(t *testing.T) {
	port := freePort(t)
	sy, _ := newSyncer(t,
		peCR("a", "op", "foo.internal", 80, "", 0),                  // not wired yet
		peCR("b", "op", "bar.internal", 6379, "10.0.0.2", port),     // ready
		peCR("c", "other", "else.internal", 80, "10.0.0.9", port+1), // other namespace
	)
	defer sy.Close()

	require.NoError(t, sy.Sync(context.Background()))
	assert.True(t, sy.Table.Synced())
	assert.Equal(t, []int32{port}, sy.Ports())

	c, err := dialPort(t, port)
	require.NoError(t, err)
	defer c.Close()
	assert.Equal(t, "[bar.internal:6379]", readN(t, c, len("[bar.internal:6379]")))
}

func TestSyncerFollowsChanges(t *testing.T) {
	ctx := context.Background()
	port := freePort(t)
	sy, c := newSyncer(t, peCR("old", "op", "old.internal", 6379, "10.0.0.2", port))
	defer sy.Close()
	require.NoError(t, sy.Sync(ctx))

	// Endpoint removed: its listener closes.
	require.NoError(t, c.Delete(ctx, peCR("old", "op", "", 0, "", 0)))
	require.NoError(t, sy.Sync(ctx))
	assert.Empty(t, sy.Ports())
	_, err := dialPort(t, port)
	assert.Error(t, err, "listener should be closed")

	// Port reallocated to a different endpoint: new connections go to it.
	require.NoError(t, c.Create(ctx, peCR("new", "op", "new.internal", 5432, "10.0.0.3", port)))
	require.NoError(t, sy.Sync(ctx))
	conn, err := dialPort(t, port)
	require.NoError(t, err)
	defer conn.Close()
	assert.Equal(t, "[new.internal:5432]", readN(t, conn, len("[new.internal:5432]")))
}

func TestSyncerReportsBindFailures(t *testing.T) {
	ctx := context.Background()
	blocker := listen(t)
	port := int32(blocker.Addr().(*net.TCPAddr).Port)
	sy, _ := newSyncer(t, peCR("b", "op", "bar.internal", 6379, "10.0.0.2", port))
	defer sy.Close()

	err := sy.Sync(ctx)
	require.Error(t, err)
	assert.Contains(t, err.Error(), fmt.Sprint(port))
	assert.Contains(t, err.Error(), "bar.internal")
	require.Error(t, sy.BindErr(), "a desired listener that failed to bind must be reported")
	assert.Contains(t, sy.BindErr().Error(), fmt.Sprint(port))
	assert.Empty(t, sy.Ports())

	// The port frees up: the next sync binds it and the failure clears.
	require.NoError(t, blocker.Close())
	require.NoError(t, sy.Sync(ctx))
	assert.NoError(t, sy.BindErr())
	assert.Equal(t, []int32{port}, sy.Ports())
}

func TestSyncerBindFailureClearsWhenNoLongerWanted(t *testing.T) {
	ctx := context.Background()
	blocker := listen(t)
	defer blocker.Close()
	port := int32(blocker.Addr().(*net.TCPAddr).Port)
	sy, c := newSyncer(t, peCR("b", "op", "bar.internal", 6379, "10.0.0.2", port))
	defer sy.Close()

	require.Error(t, sy.Sync(ctx))
	require.Error(t, sy.BindErr())

	require.NoError(t, c.Delete(ctx, peCR("b", "op", "", 0, "", 0)))
	require.NoError(t, sy.Sync(ctx))
	assert.NoError(t, sy.BindErr())
}

func TestSyncerAmbiguousPort(t *testing.T) {
	ctx := context.Background()
	port := freePort(t)
	sy, c := newSyncer(t,
		peCR("a", "op", "a.internal", 5432, "10.0.0.2", port),
		peCR("b", "op", "b.internal", 5432, "10.0.0.3", port),
	)
	defer sy.Close()
	var dials atomic.Int32
	sy.Dial = func(ctx context.Context, target string) (net.Conn, error) {
		dials.Add(1)
		return echoDial(ctx, target)
	}
	var logs atomic.Int32
	sy.Log = funcr.New(func(_, args string) {
		if strings.Contains(args, "ambiguous") {
			logs.Add(1)
		}
	}, funcr.Options{})

	require.NoError(t, sy.Sync(ctx))
	require.NoError(t, sy.Sync(ctx))
	assert.Equal(t, int32(1), logs.Load(), "a duplicate is logged once per change, not per sync")
	assert.Equal(t, []int32{port}, sy.Ports(), "the listener stays open")

	conn, err := dialPort(t, port)
	require.NoError(t, err)
	_ = conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	_, err = conn.Read(make([]byte, 1))
	assert.ErrorIs(t, err, io.EOF, "connection to an ambiguous port is closed")
	conn.Close()
	assert.Zero(t, dials.Load(), "an ambiguous port must not dial")

	// The controller reallocates the loser: the port routes again, without
	// reopening the listener.
	b := peCR("b", "op", "", 0, "", 0)
	require.NoError(t, c.Delete(ctx, b))
	require.NoError(t, sy.Sync(ctx))
	assert.Equal(t, []int32{port}, sy.Ports())
	conn, err = dialPort(t, port)
	require.NoError(t, err)
	defer conn.Close()
	assert.Equal(t, "[a.internal:5432]", readN(t, conn, len("[a.internal:5432]")))
}

// A replica with a stale mapping routes correctly as soon as its table is
// replaced: the target is resolved per connection, not per listener.
func TestSyncerRetargetsOpenListener(t *testing.T) {
	ctx := context.Background()
	port := freePort(t)
	sy, c := newSyncer(t, peCR("a", "op", "a.internal", 5432, "10.0.0.2", port))
	defer sy.Close()
	require.NoError(t, sy.Sync(ctx))

	require.NoError(t, c.Delete(ctx, peCR("a", "op", "", 0, "", 0)))
	require.NoError(t, c.Create(ctx, peCR("b", "op", "b.internal", 6379, "10.0.0.3", port)))
	require.NoError(t, sy.Sync(ctx))
	assert.Equal(t, []int32{port}, sy.Ports())

	conn, err := dialPort(t, port)
	require.NoError(t, err)
	defer conn.Close()
	assert.Equal(t, "[b.internal:6379]", readN(t, conn, len("[b.internal:6379]")))
}
