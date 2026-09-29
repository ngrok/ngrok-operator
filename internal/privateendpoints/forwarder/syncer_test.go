package forwarder

import (
	"context"
	"testing"

	"github.com/go-logr/logr"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	ngrokv1 "github.com/ngrok/ngrok-operator/api/ngrok/v1"
)

func TestSyncerSync(t *testing.T) {
	s := runtime.NewScheme()
	require.NoError(t, ngrokv1.AddToScheme(s))
	port := freePort(t)
	cr := func(name, ns, host string, p int32, ip string, fwd int32) *ngrokv1.PrivateEndpoint {
		return &ngrokv1.PrivateEndpoint{
			ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: ns},
			Spec:       ngrokv1.PrivateEndpointSpec{Hostname: host, Port: p},
			Status:     ngrokv1.PrivateEndpointStatus{ClusterIP: ip, ForwarderPort: fwd},
		}
	}
	c := fake.NewClientBuilder().WithScheme(s).WithObjects(
		cr("a", "op", "foo.internal", 80, "10.0.0.1", 0),
		cr("b", "op", "bar.internal", 6379, "10.0.0.2", port),
		cr("c", "op", "pending.internal", 6379, "", 0),
		cr("d", "other", "elsewhere.internal", 80, "10.0.0.9", 0),
	).Build()

	tbl := NewTable()
	pl := &PortListeners{Proxy: &Proxy{Table: tbl, Log: logr.Discard()}, Log: logr.Discard(), BindHost: "127.0.0.1"}
	defer pl.Close()
	sy := &Syncer{Client: c, Namespace: "op", Table: tbl, Ports: pl}

	require.NoError(t, sy.Sync(context.Background()))
	assert.True(t, tbl.Synced())
	ip, ok := tbl.IP("foo.internal")
	assert.True(t, ok)
	assert.Equal(t, "10.0.0.1", ip)
	_, ok = tbl.IP("pending.internal")
	assert.False(t, ok)
	_, ok = tbl.IP("elsewhere.internal")
	assert.False(t, ok, "other namespaces are ignored")
	assert.Equal(t, []int32{port}, pl.Ports())
}
