package privateendpoints

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"reflect"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/go-logr/logr/funcr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/rand"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"
	"sigs.k8s.io/controller-runtime/pkg/config"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	"sigs.k8s.io/controller-runtime/pkg/metrics/server"

	ngrokv1 "github.com/ngrok/ngrok-operator/api/ngrok/v1"
	pe "github.com/ngrok/ngrok-operator/internal/privateendpoints"
	"github.com/ngrok/ngrok-operator/internal/privateendpoints/forwarder"
	"github.com/ngrok/ngrok-operator/internal/testutils"
	"github.com/ngrok/ngrok-operator/pkg/bindingsdriver"
)

var (
	cfg       *rest.Config
	k8sClient client.WithWatch
	testEnv   *envtest.Environment

	ctx    context.Context
	cancel context.CancelFunc

	kginkgo *testutils.KGinkgo
)

var forwarderSelector = map[string]string{"app": "fwd"}

func TestControllers(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "PrivateEndpoints Controller Suite")
}

var _ = BeforeSuite(func() {
	logf.SetLogger(zap.New(zap.WriteTo(GinkgoWriter), zap.UseDevMode(true)))

	// Each spec starts its own manager; 1s isn't always enough for its
	// informers to sync.
	SetDefaultEventuallyTimeout(testutils.DefaultTimeout)
	SetDefaultEventuallyPollingInterval(100 * time.Millisecond)

	ctx, cancel = context.WithCancel(context.Background())

	By("bootstrapping test environment")
	testEnv = &envtest.Environment{
		CRDDirectoryPaths: []string{
			testutils.OperatorCRDPath("..", "..", ".."),
		},
		ErrorIfCRDPathMissing: true,
	}

	var err error
	cfg, err = testEnv.Start()
	Expect(err).NotTo(HaveOccurred())
	Expect(cfg).NotTo(BeNil())

	Expect(ngrokv1.AddToScheme(scheme.Scheme)).To(Succeed())

	k8sClient, err = client.NewWithWatch(cfg, client.Options{Scheme: scheme.Scheme})
	Expect(err).NotTo(HaveOccurred())

	kginkgo = testutils.NewKGinkgo(k8sClient)

	initPortSlice()
})

var _ = AfterSuite(func() {
	By("tearing down the test environment")
	cancel()
	Expect(testEnv.Stop()).To(Succeed())
})

// Forwarder port ranges. Forwarders bind real host ports (on all
// interfaces), and every Ginkgo process shares the host's port space, so each
// process draws ranges from its own slice of [portFloor, portCeiling), below
// the Linux ephemeral range. Ranges are probed before use to skip ports held
// by anything else, such as a concurrent `go test` of this package. A probe is
// released before the forwarder binds, so a port taken in between still
// fails that spec's bind: probing narrows the window, it can't close it.
const (
	portFloor   = 20000
	portCeiling = 32000
)

var portSliceStart, portSliceEnd, portCursor int32

func initPortSlice() {
	sc, _ := GinkgoConfiguration()
	size := int32((portCeiling - portFloor) / max(sc.ParallelTotal, 1))
	portSliceStart = portFloor + int32(GinkgoParallelProcess()-1)*size
	portSliceEnd = portSliceStart + size
	// A random start makes two invocations of the same process number
	// unlikely to probe the same ranges first.
	portCursor = portSliceStart + int32(rand.Intn(int(size/2)))
}

// reservePorts returns a contiguous range of n ports that were all free when
// probed.
func reservePorts(n int32) (lo, hi int32) {
	GinkgoHelper()
	for range (portSliceEnd - portSliceStart) / n * 2 {
		if portCursor+n > portSliceEnd {
			portCursor = portSliceStart
		}
		lo = portCursor
		portCursor += n
		if probePorts(lo, n) {
			return lo, lo + n - 1
		}
	}
	Fail(fmt.Sprintf("no free run of %d ports in %d-%d", n, portSliceStart, portSliceEnd))
	return 0, 0
}

func probePorts(lo, n int32) bool {
	var held []net.Listener
	defer func() {
		for _, l := range held {
			_ = l.Close()
		}
	}()
	for p := lo; p < lo+n; p++ {
		l, err := net.Listen("tcp", fmt.Sprintf(":%d", p))
		if err != nil {
			return false
		}
		held = append(held, l)
	}
	return true
}

// peEnvOpts picks which controllers a spec's manager runs.
type peEnvOpts struct {
	controller bool
	forwarder  bool
	// ports is the size of the forwarder port range. Defaults to 50.
	ports int32
	// deferStart leaves the manager stopped so a spec can seed state or
	// install faults first, as if the operator were restarting on it.
	deferStart bool
}

// peEnv is one spec's namespace and manager. The controllers are scoped to
// a single namespace and the allocator keeps in-memory state, so every spec
// gets a fresh one rather than sharing a suite-wide manager. restart replaces
// the manager with a fresh one over the same namespace, ports and boundary,
// as after a process restart or a new leader taking over.
type peEnv struct {
	opts             peEnvOpts
	ns               string
	portMin, portMax int32

	// boundary sits between the PrivateEndpoint controller and the API
	// server, on both its cached client and its APIReader.
	boundary *boundary
	// fwdBoundary sits between the forwarder and the API server.
	fwdBoundary *boundary
	dialer      *recordingDialer

	ambiguousLogs atomic.Int32

	// The running manager's forwarder, kept after stop for assertions.
	forwarder *ForwarderReconciler

	exited  chan struct{}
	exitErr error
	stopFn  func()
}

func newPEEnv(opts peEnvOpts) *peEnv {
	GinkgoHelper()
	if opts.ports == 0 {
		opts.ports = 50
	}
	e := &peEnv{opts: opts, ns: createNamespace(), boundary: &boundary{}, fwdBoundary: &boundary{}, dialer: &recordingDialer{}}
	e.portMin, e.portMax = reservePorts(opts.ports)
	if !opts.deferStart {
		e.start()
	}
	return e
}

// start builds a fresh manager and returns once its caches have synced and,
// if it runs one, the forwarder has finished its first sync.
func (e *peEnv) start() {
	GinkgoHelper()
	e.launch()
	if e.forwarder != nil {
		Eventually(e.forwarder.Table.Synced).Should(BeTrue(), "forwarder's first sync")
	}
}

// launch is start without waiting for the forwarder's first sync.
func (e *peEnv) launch() {
	GinkgoHelper()
	Expect(e.stopFn).To(BeNil(), "manager already running")

	mgr, err := ctrl.NewManager(cfg, ctrl.Options{
		Scheme:                 scheme.Scheme,
		Metrics:                server.Options{BindAddress: "0"},
		HealthProbeBindAddress: "0",
		// Every spec registers controllers under the same names.
		Controller: config.Controller{SkipNameValidation: new(true)},
	})
	Expect(err).NotTo(HaveOccurred())
	mctx, mcancel := context.WithCancel(ctx)

	var watched []client.Object
	if e.opts.controller {
		r := &PrivateEndpointReconciler{
			Client:            interceptor.NewClient(cachedClient{mgr.GetClient()}, e.boundary.funcs("")),
			APIReader:         interceptor.NewClient(readerClient{r: mgr.GetAPIReader()}, e.boundary.funcs("Reader")),
			Log:               logf.Log.WithName("privateendpoint-controller"),
			Namespace:         e.ns,
			ForwarderSelector: forwarderSelector,
			PortMin:           e.portMin,
			PortMax:           e.portMax,
		}
		Expect(r.SetupWithManager(mgr)).To(Succeed())
		watched = append(watched, &ngrokv1.PrivateEndpoint{}, &corev1.Service{})
	}
	var fwd *ForwarderReconciler
	if e.opts.forwarder {
		fwd = &ForwarderReconciler{
			Client:       interceptor.NewClient(cachedClient{mgr.GetClient()}, e.fwdBoundary.funcs("")),
			Namespace:    e.ns,
			Table:        forwarder.NewTable(),
			Listeners:    bindingsdriver.New(),
			Dial:         e.dialer.Dial,
			DrainTimeout: time.Second,
			Log: funcr.New(func(prefix, args string) {
				if strings.Contains(args, "ambiguous") {
					e.ambiguousLogs.Add(1)
				}
				_, _ = fmt.Fprintln(GinkgoWriter, prefix, args)
			}, funcr.Options{}),
		}
		Expect(fwd.SetupWithManager(mgr)).To(Succeed())
		e.forwarder = fwd
		watched = append(watched, &ngrokv1.PrivateEndpoint{})
	}
	// Controllers create their informers lazily once their workers start.
	// Creating them up front makes WaitForCacheSync below cover them.
	for _, obj := range watched {
		_, err := mgr.GetCache().GetInformer(mctx, obj)
		Expect(err).NotTo(HaveOccurred())
	}

	exited := make(chan struct{})
	e.exited = exited
	go func() {
		defer close(exited)
		e.exitErr = mgr.Start(mctx)
	}()
	e.stopFn = func() {
		mcancel()
		Eventually(exited).WithTimeout(30 * time.Second).Should(BeClosed())
		if fwd != nil {
			fwd.Close()
		}
		Expect(e.exitErr).NotTo(HaveOccurred())
	}
	DeferCleanup(e.stop)

	synced := make(chan bool, 1)
	go func() {
		sctx, scancel := context.WithTimeout(mctx, 30*time.Second)
		defer scancel()
		synced <- mgr.GetCache().WaitForCacheSync(sctx)
	}()
	select {
	case <-exited:
		Fail(fmt.Sprintf("manager exited during startup: %v", e.exitErr))
	case ok := <-synced:
		Expect(ok).To(BeTrue(), "manager caches did not sync")
	}
}

// stop stops the running manager, if any, and waits for it to exit.
func (e *peEnv) stop() {
	GinkgoHelper()
	if e.stopFn == nil {
		return
	}
	stop := e.stopFn
	e.stopFn = nil
	stop()
}

func (e *peEnv) restart() {
	GinkgoHelper()
	e.stop()
	e.start()
}

// cachedClient and readerClient let interceptor wrap the manager's client
// and APIReader separately; neither is ever asked to watch.
type cachedClient struct{ client.Client }

func (cachedClient) Watch(context.Context, client.ObjectList, ...client.ListOption) (watch.Interface, error) {
	return nil, errors.New("watch not supported")
}

type readerClient struct {
	client.WithWatch // nil: only Get and List are called
	r                client.Reader
}

func (c readerClient) Get(ctx context.Context, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
	return c.r.Get(ctx, key, obj, opts...)
}

func (c readerClient) List(ctx context.Context, list client.ObjectList, opts ...client.ListOption) error {
	return c.r.List(ctx, list, opts...)
}

// boundary records the calls made through a wrapped client and injects
// faults into selected ones. Manager goroutines call into it, so everything
// is behind mu; specs assert on snapshots.
type boundary struct {
	mu     sync.Mutex
	calls  []call
	faults []*fault
}

type call struct {
	// Op is the client method, prefixed "Reader" for APIReader calls.
	Op, Kind, Name string
	// TargetPorts are a Service write's target ports.
	TargetPorts []int32
	Err         error
}

// fault fails, rewrites the result of, or holds the calls it matches until
// it is released. Empty match fields match anything.
type fault struct {
	op, kind, name string
	// err is returned instead of making the call.
	err error
	// mutate rewrites the result of a successful call.
	mutate func(runtime.Object)
	// hold blocks a call after it completes until the fault is released or
	// the call's context ends.
	hold bool
	// first limits the fault to the first matching call.
	first bool

	hits      atomic.Int32
	released  atomic.Bool
	releaseCh chan struct{}
	once      sync.Once
}

func (f *fault) Release() {
	f.once.Do(func() {
		f.released.Store(true)
		close(f.releaseCh)
	})
}

func (f *fault) Hits() int32 { return f.hits.Load() }

func (b *boundary) inject(f *fault) *fault {
	b.mu.Lock()
	defer b.mu.Unlock()
	f.releaseCh = make(chan struct{})
	b.faults = append(b.faults, f)
	DeferCleanup(f.Release)
	return f
}

func (b *boundary) match(op, kind, name string) *fault {
	b.mu.Lock()
	defer b.mu.Unlock()
	for _, f := range b.faults {
		if f.released.Load() || (f.op != "" && f.op != op) || (f.kind != "" && f.kind != kind) || (f.name != "" && f.name != name) {
			continue
		}
		if f.first && f.hits.Load() > 0 {
			continue
		}
		f.hits.Add(1)
		return f
	}
	return nil
}

func (b *boundary) do(ctx context.Context, op string, obj runtime.Object, name string, next func() error) error {
	kind := reflect.TypeOf(obj).Elem().Name()
	f := b.match(op, kind, name)
	var err error
	if f != nil && f.err != nil {
		err = f.err
	} else {
		err = next()
		if err == nil && f != nil && f.mutate != nil {
			f.mutate(obj)
		}
		if f != nil && f.hold {
			select {
			case <-f.releaseCh:
			case <-ctx.Done():
				err = ctx.Err()
			}
		}
	}
	c := call{Op: op, Kind: kind, Name: name, Err: err}
	if svc, ok := obj.(*corev1.Service); ok && (op == "Create" || op == "Update") {
		for _, p := range svc.Spec.Ports {
			c.TargetPorts = append(c.TargetPorts, p.TargetPort.IntVal)
		}
	}
	b.mu.Lock()
	b.calls = append(b.calls, c)
	b.mu.Unlock()
	return err
}

func (b *boundary) funcs(prefix string) interceptor.Funcs {
	return interceptor.Funcs{
		Get: func(ctx context.Context, c client.WithWatch, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
			return b.do(ctx, prefix+"Get", obj, key.Name, func() error { return c.Get(ctx, key, obj, opts...) })
		},
		List: func(ctx context.Context, c client.WithWatch, list client.ObjectList, opts ...client.ListOption) error {
			return b.do(ctx, prefix+"List", list, "", func() error { return c.List(ctx, list, opts...) })
		},
		Create: func(ctx context.Context, c client.WithWatch, obj client.Object, opts ...client.CreateOption) error {
			return b.do(ctx, prefix+"Create", obj, obj.GetName(), func() error { return c.Create(ctx, obj, opts...) })
		},
		Update: func(ctx context.Context, c client.WithWatch, obj client.Object, opts ...client.UpdateOption) error {
			return b.do(ctx, prefix+"Update", obj, obj.GetName(), func() error { return c.Update(ctx, obj, opts...) })
		},
		Delete: func(ctx context.Context, c client.WithWatch, obj client.Object, opts ...client.DeleteOption) error {
			return b.do(ctx, prefix+"Delete", obj, obj.GetName(), func() error { return c.Delete(ctx, obj, opts...) })
		},
		SubResourceUpdate: func(ctx context.Context, c client.Client, sub string, obj client.Object, opts ...client.SubResourceUpdateOption) error {
			return b.do(ctx, prefix+"UpdateStatus", obj, obj.GetName(), func() error { return c.SubResource(sub).Update(ctx, obj, opts...) })
		},
	}
}

func (b *boundary) snapshot() []call {
	b.mu.Lock()
	defer b.mu.Unlock()
	return slices.Clone(b.calls)
}

// count returns how many calls matched; empty fields match anything.
func (b *boundary) count(op, kind, name string) int {
	n := 0
	for _, c := range b.snapshot() {
		if (op == "" || c.Op == op) && (kind == "" || c.Kind == kind) && (name == "" || c.Name == name) {
			n++
		}
	}
	return n
}

// crossRoutes replays the Service writes made through the boundary and
// reports every write that routed a target port another Service was still
// routing. Checking each write, not just the final state, catches a
// cross-route that a later reconcile healed.
func (b *boundary) crossRoutes() []string {
	routes := map[string][]int32{}
	var out []string
	for _, c := range b.snapshot() {
		if c.Kind != "Service" || c.Err != nil {
			continue
		}
		switch c.Op {
		case "Delete":
			delete(routes, c.Name)
		case "Create", "Update":
			for other, ports := range routes {
				for _, p := range c.TargetPorts {
					if other != c.Name && slices.Contains(ports, p) {
						out = append(out, fmt.Sprintf("%s routed port %d while %s did", c.Name, p, other))
					}
				}
			}
			routes[c.Name] = c.TargetPorts
		}
	}
	return out
}

// recordingDialer stands in for private dial with testutils.EchoDial and
// records every target it is asked for.
type recordingDialer struct {
	mu      sync.Mutex
	targets []string
}

func (d *recordingDialer) Dial(ctx context.Context, target string) (net.Conn, error) {
	d.mu.Lock()
	d.targets = append(d.targets, target)
	d.mu.Unlock()
	return testutils.EchoDial(ctx, target)
}

func (d *recordingDialer) Targets() []string {
	d.mu.Lock()
	defer d.mu.Unlock()
	return slices.Clone(d.targets)
}

func createNamespace() string {
	GinkgoHelper()
	name := "pe-" + rand.String(8)
	kginkgo.ExpectCreateNamespace(ctx, name)
	DeferCleanup(kginkgo.ExpectDeleteNamespace, ctx, name)
	return name
}

func crName(url string) string {
	spec, err := pe.ParseURL(url)
	Expect(err).NotTo(HaveOccurred())
	return pe.CRName(spec)
}

// newCR builds a PrivateEndpoint for url labeled the way the poller labels
// the ones it manages.
func newCR(ns, url string) *ngrokv1.PrivateEndpoint {
	GinkgoHelper()
	spec, err := pe.ParseURL(url)
	Expect(err).NotTo(HaveOccurred())
	return &ngrokv1.PrivateEndpoint{
		Name: pe.CRName(spec), Namespace: ns,
		Labels: map[string]string{pe.ManagedByLabel: pe.ManagedByValue, pe.HostLabel: pe.HostKey(spec.Hostname)},
		Spec:   spec,
	}
}

func createCR(ns, url string) *ngrokv1.PrivateEndpoint {
	GinkgoHelper()
	cr := newCR(ns, url)
	Expect(k8sClient.Create(ctx, cr)).To(Succeed())
	return cr
}

func getCR(ns, url string) *ngrokv1.PrivateEndpoint {
	GinkgoHelper()
	cr := &ngrokv1.PrivateEndpoint{}
	Expect(k8sClient.Get(ctx, types.NamespacedName{Namespace: ns, Name: crName(url)}, cr)).To(Succeed())
	return cr
}

func deleteCR(ns, url string) {
	GinkgoHelper()
	Expect(k8sClient.Delete(ctx, getCR(ns, url))).To(Succeed())
}

// wireCR writes the status the controller would, for specs that run the
// forwarder without it or seed state before startup.
func wireCR(cr *ngrokv1.PrivateEndpoint, clusterIP string, port int32) {
	GinkgoHelper()
	Eventually(func(g Gomega) {
		g.Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(cr), cr)).To(Succeed())
		cr.Status.ClusterIP, cr.Status.ForwarderPort = clusterIP, port
		g.Expect(k8sClient.Status().Update(ctx, cr)).To(Succeed())
	}).Should(Succeed())
}

// fastPoll polls more often than testutils.DefaultInterval: these specs chain
// many short waits.
var fastPoll = testutils.WithInterval(50 * time.Millisecond)

func eventuallyReady(ns, url string) *ngrokv1.PrivateEndpoint {
	GinkgoHelper()
	cr := &ngrokv1.PrivateEndpoint{Name: crName(url), Namespace: ns}
	kginkgo.EventuallyWithObject(ctx, cr, func(g Gomega, _ client.Object) {
		cond := meta.FindStatusCondition(cr.Status.Conditions, ngrokv1.PrivateEndpointConditionReady)
		g.Expect(cond).NotTo(BeNil())
		g.Expect(cond.Status).To(Equal(metav1.ConditionTrue), "condition: %+v", cond)
		g.Expect(cond.Reason).To(Equal("Ready"))
		g.Expect(cond.ObservedGeneration).To(Equal(cr.Generation))
		g.Expect(cr.Status.ObservedGeneration).To(Equal(cr.Generation))
		g.Expect(cr.Status.ClusterIP).NotTo(BeEmpty())
		g.Expect(cr.Status.ForwarderPort).NotTo(BeZero())
	}, fastPoll)
	return cr
}

// eventuallyNotReady waits for url's Ready condition to be False with
// reason, for the current generation, with no ClusterIP or forwarder port,
// and returns the condition.
func eventuallyNotReady(ns, url, reason string, opts ...testutils.KGinkgoOpt) *metav1.Condition {
	GinkgoHelper()
	cr := &ngrokv1.PrivateEndpoint{Name: crName(url), Namespace: ns}
	var cond *metav1.Condition
	kginkgo.EventuallyWithObject(ctx, cr, func(g Gomega, _ client.Object) {
		cond = meta.FindStatusCondition(cr.Status.Conditions, ngrokv1.PrivateEndpointConditionReady)
		g.Expect(cond).NotTo(BeNil())
		g.Expect(cond.Status).To(Equal(metav1.ConditionFalse))
		g.Expect(cond.Reason).To(Equal(reason))
		g.Expect(cond.ObservedGeneration).To(Equal(cr.Generation))
		g.Expect(cr.Status.ObservedGeneration).To(Equal(cr.Generation))
		g.Expect(cr.Status.ClusterIP).To(BeEmpty())
		g.Expect(cr.Status.ForwarderPort).To(BeZero())
	}, append([]testutils.KGinkgoOpt{fastPoll}, opts...)...)
	return cond
}

func serviceKey(ns, host string) types.NamespacedName {
	GinkgoHelper()
	name, err := pe.ServiceName(host)
	Expect(err).NotTo(HaveOccurred())
	return types.NamespacedName{Namespace: ns, Name: name}
}

func eventuallyService(ns, host string, inner func(g Gomega, svc *corev1.Service), opts ...testutils.KGinkgoOpt) *corev1.Service {
	GinkgoHelper()
	key := serviceKey(ns, host)
	svc := &corev1.Service{Name: key.Name, Namespace: key.Namespace}
	kginkgo.EventuallyWithObject(ctx, svc, func(g Gomega, _ client.Object) { inner(g, svc) }, append([]testutils.KGinkgoOpt{fastPoll}, opts...)...)
	return svc
}

func eventuallyNoService(ns, host string) {
	GinkgoHelper()
	key := serviceKey(ns, host)
	Eventually(func(g Gomega) {
		err := k8sClient.Get(ctx, key, &corev1.Service{})
		g.Expect(apierrors.IsNotFound(err)).To(BeTrue(), "Service %s still exists (err=%v)", key, err)
	}).WithTimeout(testutils.DefaultTimeout).WithPolling(50 * time.Millisecond).Should(Succeed())
}

// servicePorts maps each Service port to its target (forwarder) port.
func servicePorts(svc *corev1.Service) map[int32]int32 {
	out := map[int32]int32{}
	for _, p := range svc.Spec.Ports {
		out[p.Port] = p.TargetPort.IntVal
	}
	return out
}

func dialForwarder(port int32) (net.Conn, error) {
	return net.DialTimeout("tcp", fmt.Sprintf("127.0.0.1:%d", port), time.Second)
}

// expectRefused dials port and expects the forwarder to accept and then
// close the connection without sending anything.
func expectRefused(g Gomega, port int32) {
	c, err := dialForwarder(port)
	g.Expect(err).NotTo(HaveOccurred(), "the listener stays open")
	defer c.Close()
	_ = c.SetReadDeadline(time.Now().Add(time.Second))
	_, err = c.Read(make([]byte, 1))
	g.Expect(err).To(MatchError(io.EOF), "connection is closed without forwarding")
}

// eventuallyRoutes waits until a connection to the forwarder port reaches
// target through this spec's forwarder: the echo greeting names the target,
// and this env's dialer recorded the dial, so a listener left by some other
// process can't satisfy it.
func (e *peEnv) eventuallyRoutes(port int32, target string) {
	GinkgoHelper()
	want := "[" + target + "]"
	Eventually(func(g Gomega) {
		before := len(e.dialer.Targets())
		c, err := dialForwarder(port)
		g.Expect(err).NotTo(HaveOccurred())
		defer c.Close()
		_ = c.SetReadDeadline(time.Now().Add(time.Second))
		buf := make([]byte, len(want))
		_, err = io.ReadFull(c, buf)
		g.Expect(err).NotTo(HaveOccurred())
		g.Expect(string(buf)).To(Equal(want))
		g.Expect(e.dialer.Targets()[before:]).To(ContainElement(target))
	}).WithTimeout(testutils.DefaultTimeout).WithPolling(50 * time.Millisecond).Should(Succeed())
}

func eventuallyNotListening(port int32) {
	GinkgoHelper()
	Eventually(func() error {
		c, err := dialForwarder(port)
		if err == nil {
			_ = c.Close()
		}
		return err
	}).WithTimeout(testutils.DefaultTimeout).WithPolling(50 * time.Millisecond).Should(HaveOccurred())
}
