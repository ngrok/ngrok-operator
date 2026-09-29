package privateendpoints

import (
	"context"
	"os"
	"testing"

	"github.com/go-logr/logr"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"
	"sigs.k8s.io/controller-runtime/pkg/envtest"

	ngrokv1 "github.com/ngrok/ngrok-operator/api/ngrok/v1"
	pe "github.com/ngrok/ngrok-operator/internal/privateendpoints"
	"github.com/ngrok/ngrok-operator/internal/testutils"
)

var envClient client.WithWatch

func TestMain(m *testing.M) {
	env := &envtest.Environment{
		CRDDirectoryPaths:     []string{testutils.OperatorCRDPath("..", "..", "..")},
		ErrorIfCRDPathMissing: true,
	}
	cfg, err := env.Start()
	if err != nil {
		panic(err)
	}
	s := runtime.NewScheme()
	_ = clientgoscheme.AddToScheme(s)
	_ = ngrokv1.AddToScheme(s)
	envClient, err = client.NewWithWatch(cfg, client.Options{Scheme: s})
	if err != nil {
		panic(err)
	}
	code := m.Run()
	_ = env.Stop()
	os.Exit(code)
}

func setupNS(t *testing.T) (string, *Reconciler) {
	t.Helper()
	ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{GenerateName: "pe-test-"}}
	require.NoError(t, envClient.Create(context.Background(), ns))
	return ns.Name, &Reconciler{
		Client: envClient, APIReader: envClient, Log: logr.Discard(), Namespace: ns.Name,
		ForwarderSelector: map[string]string{"app": "fwd"},
		PortMin:           20000, PortMax: 20999,
	}
}

func createCR(t *testing.T, ns, url string) {
	t.Helper()
	spec, err := pe.ParseURL(url)
	require.NoError(t, err)
	require.NoError(t, envClient.Create(context.Background(), &ngrokv1.PrivateEndpoint{
		ObjectMeta: metav1.ObjectMeta{
			Name: pe.CRName(url), Namespace: ns,
			Labels: map[string]string{pe.ManagedByLabel: pe.ManagedByValue, pe.HostLabel: pe.HostKey(spec.Hostname)},
		},
		Spec: spec,
	}))
}

func getCR(t *testing.T, ns, url string) ngrokv1.PrivateEndpoint {
	t.Helper()
	var cr ngrokv1.PrivateEndpoint
	require.NoError(t, envClient.Get(context.Background(), types.NamespacedName{Namespace: ns, Name: pe.CRName(url)}, &cr))
	return cr
}

func reconcileHost(t *testing.T, r *Reconciler, host string) ctrl.Result {
	t.Helper()
	res, err := r.Reconcile(context.Background(), ctrl.Request{NamespacedName: types.NamespacedName{Namespace: r.Namespace, Name: pe.HostKey(host)}})
	require.NoError(t, err)
	return res
}

func hostService(t *testing.T, ns, host string) (*corev1.Service, bool) {
	t.Helper()
	name, err := pe.ServiceName(host)
	require.NoError(t, err)
	var svc corev1.Service
	err = envClient.Get(context.Background(), types.NamespacedName{Namespace: ns, Name: name}, &svc)
	if apierrors.IsNotFound(err) {
		return nil, false
	}
	require.NoError(t, err)
	return &svc, true
}

func servicePorts(svc *corev1.Service) map[int32]int32 {
	out := map[int32]int32{}
	for _, p := range svc.Spec.Ports {
		out[p.Port] = p.TargetPort.IntVal
	}
	return out
}

func TestReconcile(t *testing.T) {
	t.Run("http host gets its own service named after the hostname", func(t *testing.T) {
		ns, r := setupNS(t)
		createCR(t, ns, "http://foo.internal")
		reconcileHost(t, r, "foo.internal")

		svc, ok := hostService(t, ns, "foo.internal")
		require.True(t, ok)
		assert.Equal(t, "foo-internal", svc.Name)
		assert.Equal(t, map[string]string{"app": "fwd"}, svc.Spec.Selector)
		cr := getCR(t, ns, "http://foo.internal")
		assert.Equal(t, map[int32]int32{80: cr.Status.ForwarderPort}, servicePorts(svc))
		assert.Equal(t, svc.Spec.ClusterIP, cr.Status.ClusterIP)
		assert.GreaterOrEqual(t, cr.Status.ForwarderPort, int32(20000))
		assert.True(t, meta.IsStatusConditionTrue(cr.Status.Conditions, ngrokv1.PrivateEndpointConditionReady))
	})

	t.Run("every endpoint on a hostname gets its own port on one service", func(t *testing.T) {
		ns, r := setupNS(t)
		createCR(t, ns, "http://mix.internal")
		createCR(t, ns, "https://mix.internal")
		createCR(t, ns, "tcp://mix.internal:6379")
		reconcileHost(t, r, "mix.internal")

		svc, ok := hostService(t, ns, "mix.internal")
		require.True(t, ok)
		h, s, tc := getCR(t, ns, "http://mix.internal"), getCR(t, ns, "https://mix.internal"), getCR(t, ns, "tcp://mix.internal:6379")
		assert.Equal(t, map[int32]int32{80: h.Status.ForwarderPort, 443: s.Status.ForwarderPort, 6379: tc.Status.ForwarderPort}, servicePorts(svc))
		assert.Len(t, map[int32]bool{h.Status.ForwarderPort: true, s.Status.ForwarderPort: true, tc.Status.ForwarderPort: true}, 3)
		for _, cr := range []ngrokv1.PrivateEndpoint{h, s, tc} {
			assert.Equal(t, svc.Spec.ClusterIP, cr.Status.ClusterIP)
		}
	})

	t.Run("same label under both TLDs gets separate services", func(t *testing.T) {
		ns, r := setupNS(t)
		createCR(t, ns, "tcp://same.internal:6379")
		createCR(t, ns, "tcp://same.ngrok.direct:6379")
		reconcileHost(t, r, "same.internal")
		reconcileHost(t, r, "same.ngrok.direct")

		a, ok := hostService(t, ns, "same.internal")
		require.True(t, ok)
		b, ok := hostService(t, ns, "same.ngrok.direct")
		require.True(t, ok)
		assert.NotEqual(t, a.Spec.ClusterIP, b.Spec.ClusterIP)
	})

	t.Run("unsupported hostname is not ready and gets no service", func(t *testing.T) {
		ns, r := setupNS(t)
		createCR(t, ns, "http://api.multi.internal")
		reconcileHost(t, r, "api.multi.internal")

		cr := getCR(t, ns, "http://api.multi.internal")
		assert.Empty(t, cr.Status.ClusterIP)
		cond := meta.FindStatusCondition(cr.Status.Conditions, ngrokv1.PrivateEndpointConditionReady)
		require.NotNil(t, cond)
		assert.Equal(t, metav1.ConditionFalse, cond.Status)
		assert.Equal(t, "UnsupportedHostname", cond.Reason)
		var svcs corev1.ServiceList
		require.NoError(t, envClient.List(context.Background(), &svcs, client.InNamespace(ns)))
		assert.Empty(t, svcs.Items)
	})

	t.Run("ports are unique across hosts and stable across reconciles", func(t *testing.T) {
		ns, r := setupNS(t)
		createCR(t, ns, "tcp://a.internal:5432")
		createCR(t, ns, "tcp://b.internal:5432")
		reconcileHost(t, r, "a.internal")
		reconcileHost(t, r, "b.internal")
		a, b := getCR(t, ns, "tcp://a.internal:5432"), getCR(t, ns, "tcp://b.internal:5432")
		assert.NotEqual(t, a.Status.ForwarderPort, b.Status.ForwarderPort)

		reconcileHost(t, r, "a.internal")
		assert.Equal(t, a.Status.ForwarderPort, getCR(t, ns, "tcp://a.internal:5432").Status.ForwarderPort)
	})

	t.Run("port allocation ignores cache lag", func(t *testing.T) {
		ns, r := setupNS(t)
		// An informer that hasn't yet seen other reconciles' status writes.
		r.Client = interceptor.NewClient(envClient, interceptor.Funcs{
			List: func(ctx context.Context, c client.WithWatch, list client.ObjectList, opts ...client.ListOption) error {
				if err := c.List(ctx, list, opts...); err != nil {
					return err
				}
				if l, ok := list.(*ngrokv1.PrivateEndpointList); ok {
					for i := range l.Items {
						l.Items[i].Status = ngrokv1.PrivateEndpointStatus{}
					}
				}
				return nil
			},
		})
		createCR(t, ns, "tcp://lag-a.internal:5432")
		createCR(t, ns, "tcp://lag-b.internal:6379")
		reconcileHost(t, r, "lag-a.internal")
		reconcileHost(t, r, "lag-b.internal")
		a, b := getCR(t, ns, "tcp://lag-a.internal:5432"), getCR(t, ns, "tcp://lag-b.internal:6379")
		require.NotZero(t, a.Status.ForwarderPort)
		assert.NotEqual(t, a.Status.ForwarderPort, b.Status.ForwarderPort)
	})

	t.Run("duplicate forwarder port across hosts is reallocated", func(t *testing.T) {
		ns, r := setupNS(t)
		createCR(t, ns, "tcp://dup-a.internal:5432")
		createCR(t, ns, "tcp://dup-b.internal:5432")
		reconcileHost(t, r, "dup-a.internal")
		a := getCR(t, ns, "tcp://dup-a.internal:5432")
		b := getCR(t, ns, "tcp://dup-b.internal:5432")
		b.Status.ForwarderPort = a.Status.ForwarderPort // state left behind by an earlier race
		require.NoError(t, envClient.Status().Update(context.Background(), &b))

		reconcileHost(t, r, "dup-b.internal")
		reconcileHost(t, r, "dup-a.internal")
		a, b = getCR(t, ns, "tcp://dup-a.internal:5432"), getCR(t, ns, "tcp://dup-b.internal:5432")
		assert.NotEqual(t, a.Status.ForwarderPort, b.Status.ForwarderPort)
		svcA, _ := hostService(t, ns, "dup-a.internal")
		svcB, _ := hostService(t, ns, "dup-b.internal")
		assert.Equal(t, a.Status.ForwarderPort, svcA.Spec.Ports[0].TargetPort.IntVal)
		assert.Equal(t, b.Status.ForwarderPort, svcB.Spec.Ports[0].TargetPort.IntVal)
	})

	t.Run("removing one endpoint drops its port, last one deletes the service", func(t *testing.T) {
		ns, r := setupNS(t)
		createCR(t, ns, "http://gone.internal")
		createCR(t, ns, "tcp://gone.internal:6379")
		reconcileHost(t, r, "gone.internal")

		tcp := getCR(t, ns, "tcp://gone.internal:6379")
		require.NoError(t, envClient.Delete(context.Background(), &tcp))
		reconcileHost(t, r, "gone.internal")
		svc, ok := hostService(t, ns, "gone.internal")
		require.True(t, ok)
		assert.Len(t, svc.Spec.Ports, 1)

		http := getCR(t, ns, "http://gone.internal")
		require.NoError(t, envClient.Delete(context.Background(), &http))
		reconcileHost(t, r, "gone.internal")
		_, ok = hostService(t, ns, "gone.internal")
		assert.False(t, ok)
	})
}
