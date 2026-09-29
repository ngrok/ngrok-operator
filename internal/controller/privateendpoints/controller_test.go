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

func setupNS(t *testing.T, withShared bool) (string, *Reconciler) {
	t.Helper()
	ctx := context.Background()
	ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{GenerateName: "pe-test-"}}
	require.NoError(t, envClient.Create(ctx, ns))
	if withShared {
		require.NoError(t, envClient.Create(ctx, &corev1.Service{
			ObjectMeta: metav1.ObjectMeta{Name: "shared", Namespace: ns.Name},
			Spec: corev1.ServiceSpec{
				Selector: map[string]string{"app": "fwd"},
				Ports:    []corev1.ServicePort{{Name: "http", Port: 80}, {Name: "https", Port: 443}},
			},
		}))
	}
	return ns.Name, &Reconciler{
		Client: envClient, APIReader: envClient, Log: logr.Discard(), Namespace: ns.Name,
		SharedServiceName: "shared", ForwarderSelector: map[string]string{"app": "fwd"},
		PortMin: 20000, PortMax: 20999,
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

func sharedIP(t *testing.T, ns string) string {
	t.Helper()
	var svc corev1.Service
	require.NoError(t, envClient.Get(context.Background(), types.NamespacedName{Namespace: ns, Name: "shared"}, &svc))
	return svc.Spec.ClusterIP
}

func hostService(t *testing.T, ns, host string) (*corev1.Service, bool) {
	t.Helper()
	var svc corev1.Service
	err := envClient.Get(context.Background(), types.NamespacedName{Namespace: ns, Name: pe.HostServiceName(pe.HostKey(host))}, &svc)
	if apierrors.IsNotFound(err) {
		return nil, false
	}
	require.NoError(t, err)
	return &svc, true
}

func TestReconcile(t *testing.T) {
	t.Run("shared host uses shared service ip", func(t *testing.T) {
		ns, r := setupNS(t, true)
		createCR(t, ns, "http://foo.internal")
		createCR(t, ns, "https://foo.internal")
		reconcileHost(t, r, "foo.internal")

		for _, u := range []string{"http://foo.internal", "https://foo.internal"} {
			cr := getCR(t, ns, u)
			assert.Equal(t, sharedIP(t, ns), cr.Status.ClusterIP)
			assert.Zero(t, cr.Status.ForwarderPort)
			assert.True(t, meta.IsStatusConditionTrue(cr.Status.Conditions, ngrokv1.PrivateEndpointConditionReady))
		}
		_, ok := hostService(t, ns, "foo.internal")
		assert.False(t, ok)
	})

	t.Run("missing shared service is not ready and requeues", func(t *testing.T) {
		ns, r := setupNS(t, false)
		createCR(t, ns, "http://foo.internal")
		res := reconcileHost(t, r, "foo.internal")
		assert.NotZero(t, res.RequeueAfter)
		cr := getCR(t, ns, "http://foo.internal")
		assert.Empty(t, cr.Status.ClusterIP)
		assert.False(t, meta.IsStatusConditionTrue(cr.Status.Conditions, ngrokv1.PrivateEndpointConditionReady))
	})

	t.Run("tcp host gets dedicated service", func(t *testing.T) {
		ns, r := setupNS(t, true)
		createCR(t, ns, "tcp://bar.internal:6379")
		reconcileHost(t, r, "bar.internal")

		svc, ok := hostService(t, ns, "bar.internal")
		require.True(t, ok)
		require.Len(t, svc.Spec.Ports, 1)
		cr := getCR(t, ns, "tcp://bar.internal:6379")
		assert.Equal(t, int32(6379), svc.Spec.Ports[0].Port)
		assert.Equal(t, cr.Status.ForwarderPort, svc.Spec.Ports[0].TargetPort.IntVal)
		assert.Equal(t, svc.Spec.ClusterIP, cr.Status.ClusterIP)
		assert.Equal(t, map[string]string{"app": "fwd"}, svc.Spec.Selector)
		assert.GreaterOrEqual(t, cr.Status.ForwarderPort, int32(20000))
	})

	t.Run("mixed host goes dedicated", func(t *testing.T) {
		ns, r := setupNS(t, true)
		createCR(t, ns, "http://mix.internal")
		createCR(t, ns, "tcp://mix.internal:6379")
		reconcileHost(t, r, "mix.internal")

		svc, ok := hostService(t, ns, "mix.internal")
		require.True(t, ok)
		assert.Len(t, svc.Spec.Ports, 2)
		httpCR, tcpCR := getCR(t, ns, "http://mix.internal"), getCR(t, ns, "tcp://mix.internal:6379")
		assert.Equal(t, svc.Spec.ClusterIP, httpCR.Status.ClusterIP)
		assert.Equal(t, svc.Spec.ClusterIP, tcpCR.Status.ClusterIP)
		assert.NotZero(t, httpCR.Status.ForwarderPort)
		assert.NotEqual(t, httpCR.Status.ForwarderPort, tcpCR.Status.ForwarderPort)
	})

	t.Run("ports are unique across hosts and stable across reconciles", func(t *testing.T) {
		ns, r := setupNS(t, true)
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
		ns, r := setupNS(t, true)
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
		ns, r := setupNS(t, true)
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

	t.Run("last CR gone deletes host service", func(t *testing.T) {
		ns, r := setupNS(t, true)
		createCR(t, ns, "tcp://gone.internal:6379")
		reconcileHost(t, r, "gone.internal")
		_, ok := hostService(t, ns, "gone.internal")
		require.True(t, ok)

		cr := getCR(t, ns, "tcp://gone.internal:6379")
		require.NoError(t, envClient.Delete(context.Background(), &cr))
		reconcileHost(t, r, "gone.internal")
		_, ok = hostService(t, ns, "gone.internal")
		assert.False(t, ok)
	})

	t.Run("host switching to shared deletes host service", func(t *testing.T) {
		ns, r := setupNS(t, true)
		createCR(t, ns, "http://sw.internal")
		createCR(t, ns, "tcp://sw.internal:6379")
		reconcileHost(t, r, "sw.internal")
		tcp := getCR(t, ns, "tcp://sw.internal:6379")
		require.NoError(t, envClient.Delete(context.Background(), &tcp))
		reconcileHost(t, r, "sw.internal")

		_, ok := hostService(t, ns, "sw.internal")
		assert.False(t, ok)
		cr := getCR(t, ns, "http://sw.internal")
		assert.Equal(t, sharedIP(t, ns), cr.Status.ClusterIP)
		assert.Zero(t, cr.Status.ForwarderPort)
	})
}
