package forwarder

import (
	"context"
	"fmt"
	"net"
	"testing"
	"time"

	"github.com/go-logr/logr"
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
