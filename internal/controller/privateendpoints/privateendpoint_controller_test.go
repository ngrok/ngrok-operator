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
	"k8s.io/apimachinery/pkg/util/intstr"
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

func setupNS(t *testing.T) (string, *PrivateEndpointReconciler) {
	t.Helper()
	ns := &corev1.Namespace{GenerateName: "pe-test-"}
	require.NoError(t, envClient.Create(context.Background(), ns))
	return ns.Name, &PrivateEndpointReconciler{
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
		Name: crName(url), Namespace: ns,
		Labels: map[string]string{pe.ManagedByLabel: pe.ManagedByValue, pe.HostLabel: pe.HostKey(spec.Hostname)},
		Spec:   spec,
	}))
}

func getCR(t *testing.T, ns, url string) ngrokv1.PrivateEndpoint {
	t.Helper()
	var cr ngrokv1.PrivateEndpoint
	require.NoError(t, envClient.Get(context.Background(), types.NamespacedName{Namespace: ns, Name: crName(url)}, &cr))
	return cr
}

func reconcileHost(t *testing.T, r *PrivateEndpointReconciler, host string) ctrl.Result {
	t.Helper()
	res, err := r.Reconcile(context.Background(), ctrl.Request{Namespace: r.Namespace, Name: pe.HostKey(host)})
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
		createCR(t, ns, "http://foo.ngrok.direct")
		reconcileHost(t, r, "foo.ngrok.direct")

		svc, ok := hostService(t, ns, "foo.ngrok.direct")
		require.True(t, ok)
		assert.Equal(t, "foo-ngrok-direct", svc.Name)
		assert.Equal(t, map[string]string{"app": "fwd"}, svc.Spec.Selector)
		cr := getCR(t, ns, "http://foo.ngrok.direct")
		assert.Equal(t, map[int32]int32{80: cr.Status.ForwarderPort}, servicePorts(svc))
		assert.Equal(t, svc.Spec.ClusterIP, cr.Status.ClusterIP)
		assert.GreaterOrEqual(t, cr.Status.ForwarderPort, int32(20000))
		assert.True(t, meta.IsStatusConditionTrue(cr.Status.Conditions, ngrokv1.PrivateEndpointConditionReady))
	})

	t.Run("every endpoint on a hostname gets its own port on one service", func(t *testing.T) {
		ns, r := setupNS(t)
		createCR(t, ns, "http://mix.ngrok.direct")
		createCR(t, ns, "https://mix.ngrok.direct")
		createCR(t, ns, "tcp://mix.ngrok.direct:6379")
		reconcileHost(t, r, "mix.ngrok.direct")

		svc, ok := hostService(t, ns, "mix.ngrok.direct")
		require.True(t, ok)
		h, s, tc := getCR(t, ns, "http://mix.ngrok.direct"), getCR(t, ns, "https://mix.ngrok.direct"), getCR(t, ns, "tcp://mix.ngrok.direct:6379")
		assert.Equal(t, map[int32]int32{80: h.Status.ForwarderPort, 443: s.Status.ForwarderPort, 6379: tc.Status.ForwarderPort}, servicePorts(svc))
		assert.Len(t, map[int32]bool{h.Status.ForwarderPort: true, s.Status.ForwarderPort: true, tc.Status.ForwarderPort: true}, 3)
		for _, cr := range []ngrokv1.PrivateEndpoint{h, s, tc} {
			assert.Equal(t, svc.Spec.ClusterIP, cr.Status.ClusterIP)
		}
	})

	t.Run("unsupported hostname is not ready and gets no service", func(t *testing.T) {
		ns, r := setupNS(t)
		createCR(t, ns, "http://api.multi.ngrok.direct")
		reconcileHost(t, r, "api.multi.ngrok.direct")

		cr := getCR(t, ns, "http://api.multi.ngrok.direct")
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
		createCR(t, ns, "tcp://a.ngrok.direct:5432")
		createCR(t, ns, "tcp://b.ngrok.direct:5432")
		reconcileHost(t, r, "a.ngrok.direct")
		reconcileHost(t, r, "b.ngrok.direct")
		a, b := getCR(t, ns, "tcp://a.ngrok.direct:5432"), getCR(t, ns, "tcp://b.ngrok.direct:5432")
		assert.NotEqual(t, a.Status.ForwarderPort, b.Status.ForwarderPort)

		reconcileHost(t, r, "a.ngrok.direct")
		assert.Equal(t, a.Status.ForwarderPort, getCR(t, ns, "tcp://a.ngrok.direct:5432").Status.ForwarderPort)
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
		createCR(t, ns, "tcp://lag-a.ngrok.direct:5432")
		createCR(t, ns, "tcp://lag-b.ngrok.direct:6379")
		reconcileHost(t, r, "lag-a.ngrok.direct")
		reconcileHost(t, r, "lag-b.ngrok.direct")
		a, b := getCR(t, ns, "tcp://lag-a.ngrok.direct:5432"), getCR(t, ns, "tcp://lag-b.ngrok.direct:6379")
		require.NotZero(t, a.Status.ForwarderPort)
		assert.NotEqual(t, a.Status.ForwarderPort, b.Status.ForwarderPort)
	})

	t.Run("duplicate forwarder port across hosts is reallocated", func(t *testing.T) {
		ns, r := setupNS(t)
		createCR(t, ns, "tcp://dup-a.ngrok.direct:5432")
		createCR(t, ns, "tcp://dup-b.ngrok.direct:5432")
		reconcileHost(t, r, "dup-a.ngrok.direct")
		a := getCR(t, ns, "tcp://dup-a.ngrok.direct:5432")
		b := getCR(t, ns, "tcp://dup-b.ngrok.direct:5432")
		b.Status.ForwarderPort = a.Status.ForwarderPort // state left behind by an earlier race
		require.NoError(t, envClient.Status().Update(context.Background(), &b))

		reconcileHost(t, r, "dup-b.ngrok.direct")
		reconcileHost(t, r, "dup-a.ngrok.direct")
		a, b = getCR(t, ns, "tcp://dup-a.ngrok.direct:5432"), getCR(t, ns, "tcp://dup-b.ngrok.direct:5432")
		assert.NotEqual(t, a.Status.ForwarderPort, b.Status.ForwarderPort)
		svcA, _ := hostService(t, ns, "dup-a.ngrok.direct")
		svcB, _ := hostService(t, ns, "dup-b.ngrok.direct")
		assert.Equal(t, a.Status.ForwarderPort, svcA.Spec.Ports[0].TargetPort.IntVal)
		assert.Equal(t, b.Status.ForwarderPort, svcB.Spec.Ports[0].TargetPort.IntVal)
	})

	t.Run("unmanaged service with the same name is left alone", func(t *testing.T) {
		ns, r := setupNS(t)
		foreign := &corev1.Service{
			Name: "taken-ngrok-direct", Namespace: ns, Labels: map[string]string{"app": "someone-else"},
			Spec: corev1.ServiceSpec{
				Selector: map[string]string{"app": "someone-else"},
				Ports:    []corev1.ServicePort{{Name: "web", Port: 8080, TargetPort: intstr.FromInt32(8080)}},
			},
		}
		require.NoError(t, envClient.Create(context.Background(), foreign))
		createCR(t, ns, "http://taken.ngrok.direct")
		createCR(t, ns, "tcp://taken.ngrok.direct:6379")

		res := reconcileHost(t, r, "taken.ngrok.direct")
		assert.NotZero(t, res.RequeueAfter)

		svc, ok := hostService(t, ns, "taken.ngrok.direct")
		require.True(t, ok)
		assert.Equal(t, foreign.Labels, svc.Labels)
		assert.Equal(t, foreign.Spec.Selector, svc.Spec.Selector)
		assert.Equal(t, map[int32]int32{8080: 8080}, servicePorts(svc))
		for _, url := range []string{"http://taken.ngrok.direct", "tcp://taken.ngrok.direct:6379"} {
			cr := getCR(t, ns, url)
			assert.Empty(t, cr.Status.ClusterIP)
			assert.Zero(t, cr.Status.ForwarderPort)
			cond := meta.FindStatusCondition(cr.Status.Conditions, ngrokv1.PrivateEndpointConditionReady)
			require.NotNil(t, cond)
			assert.Equal(t, metav1.ConditionFalse, cond.Status)
			assert.Equal(t, "ServiceConflict", cond.Reason)
			assert.Contains(t, cond.Message, "taken-ngrok-direct")
		}
	})

	t.Run("a freed port is not reused while higher ports are free", func(t *testing.T) {
		ns, r := setupNS(t)
		createCR(t, ns, "tcp://reuse-a.ngrok.direct:5432")
		createCR(t, ns, "tcp://reuse-b.ngrok.direct:5432")
		reconcileHost(t, r, "reuse-a.ngrok.direct")
		reconcileHost(t, r, "reuse-b.ngrok.direct")
		a, b := getCR(t, ns, "tcp://reuse-a.ngrok.direct:5432"), getCR(t, ns, "tcp://reuse-b.ngrok.direct:5432")

		// Free both the lowest and the highest allocated port.
		require.NoError(t, envClient.Delete(context.Background(), &a))
		require.NoError(t, envClient.Delete(context.Background(), &b))
		reconcileHost(t, r, "reuse-a.ngrok.direct")
		reconcileHost(t, r, "reuse-b.ngrok.direct")

		createCR(t, ns, "tcp://reuse-c.ngrok.direct:5432")
		reconcileHost(t, r, "reuse-c.ngrok.direct")
		c := getCR(t, ns, "tcp://reuse-c.ngrok.direct:5432")
		require.NotZero(t, c.Status.ForwarderPort)
		assert.NotEqual(t, a.Status.ForwarderPort, c.Status.ForwarderPort)
		assert.NotEqual(t, b.Status.ForwarderPort, c.Status.ForwarderPort)
	})

	t.Run("allocation wraps to freed low ports once the top is used", func(t *testing.T) {
		ns, r := setupNS(t)
		r.PortMin, r.PortMax = 20000, 20002
		for _, h := range []string{"wrap-a", "wrap-b", "wrap-c"} {
			createCR(t, ns, "tcp://"+h+".ngrok.direct:5432")
			reconcileHost(t, r, h+".ngrok.direct")
		}
		b := getCR(t, ns, "tcp://wrap-b.ngrok.direct:5432")
		require.NoError(t, envClient.Delete(context.Background(), &b))
		reconcileHost(t, r, "wrap-b.ngrok.direct")

		createCR(t, ns, "tcp://wrap-d.ngrok.direct:5432")
		reconcileHost(t, r, "wrap-d.ngrok.direct")
		assert.Equal(t, b.Status.ForwarderPort, getCR(t, ns, "tcp://wrap-d.ngrok.direct:5432").Status.ForwarderPort)
	})

	t.Run("exhausted port range marks the endpoint not ready", func(t *testing.T) {
		ns, r := setupNS(t)
		r.PortMin, r.PortMax = 20000, 20000
		createCR(t, ns, "tcp://full-a.ngrok.direct:5432")
		createCR(t, ns, "tcp://full-b.ngrok.direct:5432")
		reconcileHost(t, r, "full-a.ngrok.direct")
		res := reconcileHost(t, r, "full-b.ngrok.direct")
		assert.NotZero(t, res.RequeueAfter)

		assert.Equal(t, int32(20000), getCR(t, ns, "tcp://full-a.ngrok.direct:5432").Status.ForwarderPort)
		cr := getCR(t, ns, "tcp://full-b.ngrok.direct:5432")
		assert.Zero(t, cr.Status.ForwarderPort)
		cond := meta.FindStatusCondition(cr.Status.Conditions, ngrokv1.PrivateEndpointConditionReady)
		require.NotNil(t, cond)
		assert.Equal(t, metav1.ConditionFalse, cond.Status)
		assert.Equal(t, "PortsExhausted", cond.Reason)
		_, ok := hostService(t, ns, "full-b.ngrok.direct")
		assert.False(t, ok)
	})

	t.Run("two endpoints on the same hostname and port conflict", func(t *testing.T) {
		ns, r := setupNS(t)
		// http defaults to 80, so both claim port 80 on pc.ngrok.direct.
		createCR(t, ns, "http://pc.ngrok.direct")
		createCR(t, ns, "tcp://pc.ngrok.direct:80")
		reconcileHost(t, r, "pc.ngrok.direct")

		svc, ok := hostService(t, ns, "pc.ngrok.direct")
		require.True(t, ok)
		require.Len(t, svc.Spec.Ports, 1)
		a, b := getCR(t, ns, "http://pc.ngrok.direct"), getCR(t, ns, "tcp://pc.ngrok.direct:80")
		winner, loser := a, b
		if b.Name < a.Name {
			winner, loser = b, a
		}
		assert.True(t, meta.IsStatusConditionTrue(winner.Status.Conditions, ngrokv1.PrivateEndpointConditionReady))
		assert.Equal(t, winner.Status.ForwarderPort, svc.Spec.Ports[0].TargetPort.IntVal)
		assert.Empty(t, loser.Status.ClusterIP)
		assert.Zero(t, loser.Status.ForwarderPort)
		cond := meta.FindStatusCondition(loser.Status.Conditions, ngrokv1.PrivateEndpointConditionReady)
		require.NotNil(t, cond)
		assert.Equal(t, metav1.ConditionFalse, cond.Status)
		assert.Equal(t, "PortConflict", cond.Reason)
		assert.Contains(t, cond.Message, winner.Name)
		assert.Contains(t, cond.Message, "port 80")
	})

	t.Run("service without a cluster IP yet is not ready and requeues", func(t *testing.T) {
		ns, r := setupNS(t)
		// envtest allocates ClusterIPs synchronously; simulate one that
		// hasn't been assigned yet.
		r.Client = interceptor.NewClient(envClient, interceptor.Funcs{
			Create: func(ctx context.Context, c client.WithWatch, obj client.Object, opts ...client.CreateOption) error {
				if err := c.Create(ctx, obj, opts...); err != nil {
					return err
				}
				if svc, ok := obj.(*corev1.Service); ok {
					svc.Spec.ClusterIP = ""
				}
				return nil
			},
		})
		createCR(t, ns, "tcp://nr.ngrok.direct:5432")
		res := reconcileHost(t, r, "nr.ngrok.direct")
		assert.NotZero(t, res.RequeueAfter)

		cr := getCR(t, ns, "tcp://nr.ngrok.direct:5432")
		assert.Empty(t, cr.Status.ClusterIP)
		require.NotZero(t, cr.Status.ForwarderPort)
		cond := meta.FindStatusCondition(cr.Status.Conditions, ngrokv1.PrivateEndpointConditionReady)
		require.NotNil(t, cond)
		assert.Equal(t, metav1.ConditionFalse, cond.Status)
		assert.Equal(t, "ServiceNotReady", cond.Reason)

		// Once the IP shows up the endpoint goes Ready on the same port.
		r.Client = envClient
		res = reconcileHost(t, r, "nr.ngrok.direct")
		assert.Zero(t, res.RequeueAfter)
		ready := getCR(t, ns, "tcp://nr.ngrok.direct:5432")
		assert.NotEmpty(t, ready.Status.ClusterIP)
		assert.Equal(t, cr.Status.ForwarderPort, ready.Status.ForwarderPort)
		assert.True(t, meta.IsStatusConditionTrue(ready.Status.Conditions, ngrokv1.PrivateEndpointConditionReady))
	})

	t.Run("removing one endpoint drops its port, last one deletes the service", func(t *testing.T) {
		ns, r := setupNS(t)
		createCR(t, ns, "http://gone.ngrok.direct")
		createCR(t, ns, "tcp://gone.ngrok.direct:6379")
		reconcileHost(t, r, "gone.ngrok.direct")

		tcp := getCR(t, ns, "tcp://gone.ngrok.direct:6379")
		require.NoError(t, envClient.Delete(context.Background(), &tcp))
		reconcileHost(t, r, "gone.ngrok.direct")
		svc, ok := hostService(t, ns, "gone.ngrok.direct")
		require.True(t, ok)
		assert.Len(t, svc.Spec.Ports, 1)

		http := getCR(t, ns, "http://gone.ngrok.direct")
		require.NoError(t, envClient.Delete(context.Background(), &http))
		reconcileHost(t, r, "gone.ngrok.direct")
		_, ok = hostService(t, ns, "gone.ngrok.direct")
		assert.False(t, ok)
	})
}

// assertNoSharedTargetPorts fails if two managed Services route to the same
// forwarder port.
func assertNoSharedTargetPorts(t *testing.T, ns string) {
	t.Helper()
	var svcs corev1.ServiceList
	require.NoError(t, envClient.List(context.Background(), &svcs, client.InNamespace(ns), client.MatchingLabels{pe.ManagedByLabel: pe.ManagedByValue}))
	seen := map[int32]string{}
	for _, svc := range svcs.Items {
		for _, p := range svc.Spec.Ports {
			prev, dup := seen[p.TargetPort.IntVal]
			assert.False(t, dup, "Services %s and %s share targetPort %d", prev, svc.Name, p.TargetPort.IntVal)
			seen[p.TargetPort.IntVal] = svc.Name
		}
	}
}

// failover returns a PrivateEndpointReconciler like r with no in-memory allocation state,
// as a newly elected leader would have.
func failover(r *PrivateEndpointReconciler) *PrivateEndpointReconciler {
	return &PrivateEndpointReconciler{
		Client: envClient, APIReader: r.APIReader, Log: r.Log, Namespace: r.Namespace,
		ForwarderSelector: r.ForwarderSelector, PortMin: r.PortMin, PortMax: r.PortMax,
	}
}

func TestRoutingSafety(t *testing.T) {
	// A's Service is published but its status write fails.
	tests := []struct {
		name     string
		failover bool
	}{
		{"leader dies after the service write", true},
		{"status write conflict on the same leader", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ns, r := setupNS(t)
			urlA, urlB := "tcp://sw-a.ngrok.direct:5432", "tcp://sw-b.ngrok.direct:5432"
			failA := true
			r.Client = interceptor.NewClient(envClient, interceptor.Funcs{
				SubResourceUpdate: func(ctx context.Context, c client.Client, sub string, obj client.Object, opts ...client.SubResourceUpdateOption) error {
					if failA && obj.GetName() == crName(urlA) {
						return apierrors.NewConflict(ngrokv1.GroupVersion.WithResource("privateendpoints").GroupResource(), obj.GetName(), nil)
					}
					return c.SubResource(sub).Update(ctx, obj, opts...)
				},
			})
			createCR(t, ns, urlA)
			_, err := r.Reconcile(context.Background(), ctrl.Request{Namespace: ns, Name: pe.HostKey("sw-a.ngrok.direct")})
			require.Error(t, err)
			svcA, ok := hostService(t, ns, "sw-a.ngrok.direct")
			require.True(t, ok)
			p := svcA.Spec.Ports[0].TargetPort.IntVal
			require.NotZero(t, p)
			require.Zero(t, getCR(t, ns, urlA).Status.ForwarderPort)

			next := r
			if tt.failover {
				next = failover(r)
			}
			failA = false
			createCR(t, ns, urlB)
			reconcileHost(t, next, "sw-b.ngrok.direct")
			assert.NotEqual(t, p, getCR(t, ns, urlB).Status.ForwarderPort, "B must not get the port A's Service routes to")

			reconcileHost(t, next, "sw-a.ngrok.direct")
			svcA, _ = hostService(t, ns, "sw-a.ngrok.direct")
			assert.Equal(t, getCR(t, ns, urlA).Status.ForwarderPort, svcA.Spec.Ports[0].TargetPort.IntVal)
			assertNoSharedTargetPorts(t, ns)
		})
	}

	// CR A is deleted but the leader dies before removing its Service.
	t.Run("port stays reserved while a deleted endpoint's service survives", func(t *testing.T) {
		ns, r := setupNS(t)
		r.PortMin, r.PortMax = 20000, 20001
		urlA := "tcp://gone-a.ngrok.direct:5432"
		createCR(t, ns, urlA)
		reconcileHost(t, r, "gone-a.ngrok.direct")
		a := getCR(t, ns, urlA)
		p := a.Status.ForwarderPort
		require.NoError(t, envClient.Delete(context.Background(), &a))

		createCR(t, ns, "tcp://gone-b.ngrok.direct:5432")
		reconcileHost(t, failover(r), "gone-b.ngrok.direct")
		assert.NotEqual(t, p, getCR(t, ns, "tcp://gone-b.ngrok.direct:5432").Status.ForwarderPort)

		// Once A's Service is cleaned up, P is free again.
		reconcileHost(t, r, "gone-a.ngrok.direct")
		_, ok := hostService(t, ns, "gone-a.ngrok.direct")
		require.False(t, ok)
		createCR(t, ns, "tcp://gone-c.ngrok.direct:5432")
		reconcileHost(t, failover(r), "gone-c.ngrok.direct")
		assert.Equal(t, p, getCR(t, ns, "tcp://gone-c.ngrok.direct:5432").Status.ForwarderPort)
	})
}
