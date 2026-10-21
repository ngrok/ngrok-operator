package privateendpoints

import (
	"context"
	"net"
	"testing"
	"time"

	"github.com/go-logr/logr"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/utils/ptr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/config"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	"sigs.k8s.io/controller-runtime/pkg/metrics/server"

	ngrokv1 "github.com/ngrok/ngrok-operator/api/ngrok/v1"
	"github.com/ngrok/ngrok-operator/internal/privateendpoints/forwarder"
	"github.com/ngrok/ngrok-operator/internal/testutils"
	"github.com/ngrok/ngrok-operator/pkg/bindingsdriver"
)

// TestForwarderReconcilerStartupBindFailureKeepsManagerRunning covers the path in
// SetupWithManager where the first sync can't bind a port: the manager must
// keep running (not crashloop), report the failure, and bind once the port
// frees up.
func TestForwarderReconcilerStartupBindFailureKeepsManagerRunning(t *testing.T) {
	env := &envtest.Environment{
		CRDDirectoryPaths:     []string{testutils.OperatorCRDPath("..", "..", "..")},
		ErrorIfCRDPathMissing: true,
	}
	cfg, err := env.Start()
	require.NoError(t, err)
	defer func() { _ = env.Stop() }()

	s := runtime.NewScheme()
	require.NoError(t, clientgoscheme.AddToScheme(s))
	require.NoError(t, ngrokv1.AddToScheme(s))
	c, err := client.New(cfg, client.Options{Scheme: s})
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{GenerateName: "fwd-test-"}}
	require.NoError(t, c.Create(ctx, ns))

	blocker := listen(t)
	port := int32(blocker.Addr().(*net.TCPAddr).Port)
	cr := peCR("b", ns.Name, "bar.ngrok.direct", 6379, "", 0)
	cr.Spec.URL, cr.Spec.Scheme = "tcp://bar.ngrok.direct:6379", ngrokv1.PrivateEndpointSchemeTCP
	require.NoError(t, c.Create(ctx, cr))
	cr.Status = ngrokv1.PrivateEndpointStatus{ClusterIP: "10.0.0.2", ForwarderPort: port}
	require.NoError(t, c.Status().Update(ctx, cr))

	mgr, err := ctrl.NewManager(cfg, ctrl.Options{
		Scheme:                 s,
		Cache:                  cache.Options{DefaultNamespaces: map[string]cache.Config{ns.Name: {}}},
		Metrics:                server.Options{BindAddress: "0"},
		HealthProbeBindAddress: "0",
		// -count=N reuses the global controller name registry.
		Controller: config.Controller{SkipNameValidation: ptr.To(true)},
	})
	require.NoError(t, err)
	sy := &ForwarderReconciler{
		Client:       mgr.GetClient(),
		Namespace:    ns.Name,
		Table:        forwarder.NewTable(),
		Listeners:    bindingsdriver.New(),
		Dial:         echoDial,
		DrainTimeout: time.Second,
		Log:          logr.Discard(),
	}
	defer sy.Close()
	require.NoError(t, sy.SetupWithManager(mgr))

	done := make(chan error, 1)
	go func() { done <- mgr.Start(ctx) }()

	require.Eventually(t, func() bool { return sy.Table.Synced() && sy.BindErr() != nil }, 30*time.Second, 50*time.Millisecond,
		"first sync should complete and report the held port")
	select {
	case err := <-done:
		t.Fatalf("manager stopped on a bind failure: %v", err)
	case <-time.After(time.Second):
	}

	require.NoError(t, blocker.Close())
	require.Eventually(t, func() bool { return sy.BindErr() == nil }, 30*time.Second, 50*time.Millisecond,
		"controller should retry and bind once the port is free")
	assert.Equal(t, []int32{port}, sy.Ports())

	cancel()
	select {
	case err := <-done:
		assert.NoError(t, err)
	case <-time.After(30 * time.Second):
		t.Fatal("manager did not stop")
	}
}
