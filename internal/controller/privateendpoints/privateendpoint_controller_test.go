package privateendpoints

import (
	"errors"
	"slices"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/apimachinery/pkg/util/rand"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	ngrokv1 "github.com/ngrok/ngrok-operator/api/ngrok/v1"
	pe "github.com/ngrok/ngrok-operator/internal/privateendpoints"
	"github.com/ngrok/ngrok-operator/internal/testutils"
)

var _ = Describe("PrivateEndpoint controller", func() {
	var (
		opts peEnvOpts
		e    *peEnv
	)

	BeforeEach(func() {
		opts = peEnvOpts{controller: true}
	})

	JustBeforeEach(func() {
		e = newPEEnv(opts)
	})

	It("creates one Service per hostname, named so the CoreDNS rewrite lands on it", func() {
		createCR(e.ns, "http://foo.ngrok.direct")

		cr := eventuallyReady(e.ns, "http://foo.ngrok.direct")
		Expect(cr.Status.ForwarderPort).To(BeNumerically(">=", e.portMin))
		Expect(cr.Status.ForwarderPort).To(BeNumerically("<=", e.portMax))

		svc := eventuallyService(e.ns, "foo.ngrok.direct", func(g Gomega, svc *corev1.Service) {
			g.Expect(servicePorts(svc)).To(Equal(map[int32]int32{80: cr.Status.ForwarderPort}))
		})
		Expect(svc.Name).To(Equal("foo-ngrok-direct"))
		Expect(svc.Labels).To(Equal(map[string]string{pe.ManagedByLabel: pe.ManagedByValue, pe.HostLabel: pe.HostKey("foo.ngrok.direct")}))
		Expect(svc.Spec.Type).To(Equal(corev1.ServiceTypeClusterIP))
		Expect(svc.Spec.Selector).To(Equal(forwarderSelector))
		Expect(svc.Spec.Ports[0].Name).To(Equal("port-80"))
		Expect(svc.Spec.Ports[0].Protocol).To(Equal(corev1.ProtocolTCP))
		Expect(cr.Status.ClusterIP).To(Equal(svc.Spec.ClusterIP))
	})

	It("gives every endpoint on a hostname its own port on one Service", func() {
		urls := []string{"http://mix.ngrok.direct", "https://mix.ngrok.direct", "tcp://mix.ngrok.direct:6379"}
		for _, url := range urls {
			createCR(e.ns, url)
		}
		h, s, t := eventuallyReady(e.ns, urls[0]), eventuallyReady(e.ns, urls[1]), eventuallyReady(e.ns, urls[2])

		svc := eventuallyService(e.ns, "mix.ngrok.direct", func(g Gomega, svc *corev1.Service) {
			g.Expect(servicePorts(svc)).To(Equal(map[int32]int32{
				80:   h.Status.ForwarderPort,
				443:  s.Status.ForwarderPort,
				6379: t.Status.ForwarderPort,
			}))
		})
		Expect(map[int32]bool{h.Status.ForwarderPort: true, s.Status.ForwarderPort: true, t.Status.ForwarderPort: true}).To(HaveLen(3), "forwarder ports must be distinct")
		for _, cr := range []*ngrokv1.PrivateEndpoint{h, s, t} {
			Expect(cr.Status.ClusterIP).To(Equal(svc.Spec.ClusterIP))
		}
	})

	It("allocates distinct forwarder ports across hostnames", func() {
		createCR(e.ns, "tcp://a.ngrok.direct:5432")
		createCR(e.ns, "tcp://b.ngrok.direct:5432")
		a, b := eventuallyReady(e.ns, "tcp://a.ngrok.direct:5432"), eventuallyReady(e.ns, "tcp://b.ngrok.direct:5432")
		Expect(a.Status.ForwarderPort).NotTo(Equal(b.Status.ForwarderPort))
		Expect(a.Status.ClusterIP).NotTo(Equal(b.Status.ClusterIP), "each hostname gets its own Service")
	})

	It("marks a hostname that can't be a Service name not ready and creates no Service", func() {
		createCR(e.ns, "http://api.multi.ngrok.direct")

		cond := eventuallyNotReady(e.ns, "http://api.multi.ngrok.direct", "UnsupportedHostname")
		Expect(cond.Message).NotTo(BeEmpty())
		Expect(e.boundary.count("Create", "Service", "")).To(BeZero())
		var svcs corev1.ServiceList
		Expect(k8sClient.List(ctx, &svcs, client.InNamespace(e.ns))).To(Succeed())
		Expect(svcs.Items).To(BeEmpty())
	})

	It("doesn't rewrite the Service or status when nothing changed", func() {
		createCR(e.ns, "tcp://calm.ngrok.direct:5432")
		cr := eventuallyReady(e.ns, "tcp://calm.ngrok.direct:5432")
		svc := eventuallyService(e.ns, "calm.ngrok.direct", func(g Gomega, svc *corev1.Service) {
			g.Expect(svc.Spec.Ports).To(HaveLen(1))
		})

		By("touching the Service, which enqueues the hostname")
		gets := e.boundary.count("Get", "Service", svc.Name)
		mark := len(e.boundary.snapshot())
		svc.Annotations = map[string]string{"touched": "yes"}
		Expect(k8sClient.Update(ctx, svc)).To(Succeed())
		Eventually(func() int { return e.boundary.count("Get", "Service", svc.Name) }).Should(BeNumerically(">", gets), "the controller reconciled the touch")

		Consistently(func(g Gomega) {
			var gotSvc corev1.Service
			g.Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(svc), &gotSvc)).To(Succeed())
			g.Expect(gotSvc.ResourceVersion).To(Equal(svc.ResourceVersion))
			var gotCR ngrokv1.PrivateEndpoint
			g.Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(cr), &gotCR)).To(Succeed())
			g.Expect(gotCR.ResourceVersion).To(Equal(cr.ResourceVersion))
		}).WithTimeout(time.Second).Should(Succeed())
		for _, c := range e.boundary.snapshot()[mark:] {
			Expect(c.Op).To(BeElementOf("Get", "List", "ReaderList"), "no writes after the touch: %+v", c)
		}
	})

	It("recreates a managed Service that is deleted, keeping its forwarder ports", func() {
		createCR(e.ns, "http://redo.ngrok.direct")
		cr := eventuallyReady(e.ns, "http://redo.ngrok.direct")
		old := eventuallyService(e.ns, "redo.ngrok.direct", func(Gomega, *corev1.Service) {})

		Expect(k8sClient.Delete(ctx, old)).To(Succeed())

		svc := eventuallyService(e.ns, "redo.ngrok.direct", func(g Gomega, svc *corev1.Service) {
			g.Expect(svc.UID).NotTo(Equal(old.UID))
			g.Expect(servicePorts(svc)).To(Equal(map[int32]int32{80: cr.Status.ForwarderPort}))
		})
		kginkgo.EventuallyWithObject(ctx, cr, func(g Gomega, _ client.Object) {
			g.Expect(cr.Status.ClusterIP).To(Equal(svc.Spec.ClusterIP), "status follows the new Service's ClusterIP")
		})
	})

	It("restores a managed Service that is edited", func() {
		createCR(e.ns, "http://tamper.ngrok.direct")
		cr := eventuallyReady(e.ns, "http://tamper.ngrok.direct")
		svc := eventuallyService(e.ns, "tamper.ngrok.direct", func(Gomega, *corev1.Service) {})

		svc.Labels["extra"] = "label"
		svc.Spec.Selector = map[string]string{"app": "evil"}
		svc.Spec.Ports = []corev1.ServicePort{{Name: "evil", Port: 9999, TargetPort: intstr.FromInt32(9999)}}
		Expect(k8sClient.Update(ctx, svc)).To(Succeed())

		eventuallyService(e.ns, "tamper.ngrok.direct", func(g Gomega, svc *corev1.Service) {
			g.Expect(svc.Labels).To(Equal(map[string]string{pe.ManagedByLabel: pe.ManagedByValue, pe.HostLabel: pe.HostKey("tamper.ngrok.direct")}))
			g.Expect(svc.Spec.Selector).To(Equal(forwarderSelector))
			g.Expect(servicePorts(svc)).To(Equal(map[int32]int32{80: cr.Status.ForwarderPort}))
		})
	})

	It("leaves an unmanaged Service with the same name alone", func() {
		foreign := &corev1.Service{
			Name: "taken-ngrok-direct", Namespace: e.ns, Labels: map[string]string{"app": "someone-else"},
			Spec: corev1.ServiceSpec{
				Selector: map[string]string{"app": "someone-else"},
				Ports:    []corev1.ServicePort{{Name: "web", Port: 8080, TargetPort: intstr.FromInt32(8080)}},
			},
		}
		Expect(k8sClient.Create(ctx, foreign)).To(Succeed())
		createCR(e.ns, "http://taken.ngrok.direct")
		createCR(e.ns, "tcp://taken.ngrok.direct:6379")

		for _, url := range []string{"http://taken.ngrok.direct", "tcp://taken.ngrok.direct:6379"} {
			cond := eventuallyNotReady(e.ns, url, "ServiceConflict")
			Expect(cond.Message).To(ContainSubstring("taken-ngrok-direct"))
		}
		kginkgo.ConsistentlyExpectResourceVersionNotToChange(ctx, foreign, testutils.WithTimeout(2*time.Second))
	})

	It("serves the lowest-named endpoint when two claim one port, and the other once it is gone", func() {
		// http defaults to 80, so both claim port 80 on pc.ngrok.direct.
		winnerURL, loserURL := "http://pc.ngrok.direct", "tcp://pc.ngrok.direct:80"
		Expect(crName(winnerURL) < crName(loserURL)).To(BeTrue())
		createCR(e.ns, loserURL)
		createCR(e.ns, winnerURL)

		winner := eventuallyReady(e.ns, winnerURL)
		cond := eventuallyNotReady(e.ns, loserURL, "PortConflict")
		Expect(cond.Message).To(ContainSubstring(winner.Name))
		Expect(cond.Message).To(ContainSubstring("port 80"))
		eventuallyService(e.ns, "pc.ngrok.direct", func(g Gomega, svc *corev1.Service) {
			g.Expect(servicePorts(svc)).To(Equal(map[int32]int32{80: winner.Status.ForwarderPort}))
		})

		deleteCR(e.ns, winnerURL)

		loser := eventuallyReady(e.ns, loserURL)
		eventuallyService(e.ns, "pc.ngrok.direct", func(g Gomega, svc *corev1.Service) {
			g.Expect(servicePorts(svc)).To(Equal(map[int32]int32{80: loser.Status.ForwarderPort}))
		})
	})

	It("drops a deleted endpoint's port, and deletes the Service with the last one", func() {
		createCR(e.ns, "http://gone.ngrok.direct")
		createCR(e.ns, "tcp://gone.ngrok.direct:6379")
		http := eventuallyReady(e.ns, "http://gone.ngrok.direct")
		eventuallyReady(e.ns, "tcp://gone.ngrok.direct:6379")
		eventuallyService(e.ns, "gone.ngrok.direct", func(g Gomega, svc *corev1.Service) {
			g.Expect(svc.Spec.Ports).To(HaveLen(2))
		})

		deleteCR(e.ns, "tcp://gone.ngrok.direct:6379")
		eventuallyService(e.ns, "gone.ngrok.direct", func(g Gomega, svc *corev1.Service) {
			g.Expect(servicePorts(svc)).To(Equal(map[int32]int32{80: http.Status.ForwarderPort}))
		})

		deleteCR(e.ns, "http://gone.ngrok.direct")
		eventuallyNoService(e.ns, "gone.ngrok.direct")
	})

	It("stops serving an endpoint as soon as it starts deleting", func() {
		createCR(e.ns, "http://fin.ngrok.direct")
		createCR(e.ns, "tcp://fin.ngrok.direct:6379")
		http := eventuallyReady(e.ns, "http://fin.ngrok.direct")
		eventuallyReady(e.ns, "tcp://fin.ngrok.direct:6379")

		By("holding the tcp endpoint in deletion with a finalizer")
		tcp := getCR(e.ns, "tcp://fin.ngrok.direct:6379")
		controllerutil.AddFinalizer(tcp, "test.ngrok.com/hold")
		Expect(k8sClient.Update(ctx, tcp)).To(Succeed())
		DeferCleanup(func() {
			kginkgo.EventuallyWithObject(ctx, tcp, func(g Gomega, _ client.Object) {
				controllerutil.RemoveFinalizer(tcp, "test.ngrok.com/hold")
				g.Expect(k8sClient.Update(ctx, tcp)).To(Succeed())
			})
		})
		Expect(k8sClient.Delete(ctx, tcp)).To(Succeed())

		eventuallyService(e.ns, "fin.ngrok.direct", func(g Gomega, svc *corev1.Service) {
			g.Expect(servicePorts(svc)).To(Equal(map[int32]int32{80: http.Status.ForwarderPort}))
		})
	})

	It("ignores PrivateEndpoints without a host label", func() {
		cr := newCR(e.ns, "http://nolabel.ngrok.direct")
		delete(cr.Labels, pe.HostLabel)
		Expect(k8sClient.Create(ctx, cr)).To(Succeed())
		// The watch handles events in order, so once a later endpoint is
		// served the unlabeled one's event has been handled too.
		createCR(e.ns, "http://sentinel.ngrok.direct")
		eventuallyReady(e.ns, "http://sentinel.ngrok.direct")

		var svcs corev1.ServiceList
		Expect(k8sClient.List(ctx, &svcs, client.InNamespace(e.ns))).To(Succeed())
		Expect(svcs.Items).To(HaveLen(1))
		Expect(svcs.Items[0].Name).To(Equal("sentinel-ngrok-direct"))
		Expect(e.boundary.count("", "Service", "nolabel-ngrok-direct")).To(BeZero(), "the unlabeled endpoint's hostname was never reconciled")
		Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(cr), cr)).To(Succeed())
		Expect(cr.Status.Conditions).To(BeEmpty())
	})

	It("doesn't reuse a freed port while higher ones are free", func() {
		// Forwarder replicas may still map a freed port to its old host, so
		// the allocator cycles through the range before reusing one.
		createCR(e.ns, "tcp://reuse-a.ngrok.direct:5432")
		a := eventuallyReady(e.ns, "tcp://reuse-a.ngrok.direct:5432")
		createCR(e.ns, "tcp://reuse-b.ngrok.direct:5432")
		b := eventuallyReady(e.ns, "tcp://reuse-b.ngrok.direct:5432")
		Expect(b.Status.ForwarderPort).To(Equal(a.Status.ForwarderPort + 1))

		deleteCR(e.ns, "tcp://reuse-a.ngrok.direct:5432")
		deleteCR(e.ns, "tcp://reuse-b.ngrok.direct:5432")
		eventuallyNoService(e.ns, "reuse-a.ngrok.direct")
		eventuallyNoService(e.ns, "reuse-b.ngrok.direct")

		createCR(e.ns, "tcp://reuse-c.ngrok.direct:5432")
		c := eventuallyReady(e.ns, "tcp://reuse-c.ngrok.direct:5432")
		Expect(c.Status.ForwarderPort).To(Equal(b.Status.ForwarderPort + 1))
	})

	Context("with a three-port range", func() {
		BeforeEach(func() { opts.ports = 3 })

		It("wraps around to a freed low port once the top of the range is used", func() {
			var got []int32
			for _, h := range []string{"wrap-a", "wrap-b", "wrap-c"} {
				createCR(e.ns, "tcp://"+h+".ngrok.direct:5432")
				got = append(got, eventuallyReady(e.ns, "tcp://"+h+".ngrok.direct:5432").Status.ForwarderPort)
			}
			Expect(got).To(Equal([]int32{e.portMin, e.portMin + 1, e.portMax}))

			deleteCR(e.ns, "tcp://wrap-b.ngrok.direct:5432")
			eventuallyNoService(e.ns, "wrap-b.ngrok.direct")

			createCR(e.ns, "tcp://wrap-d.ngrok.direct:5432")
			Expect(eventuallyReady(e.ns, "tcp://wrap-d.ngrok.direct:5432").Status.ForwarderPort).To(Equal(e.portMin + 1))
		})
	})

	Context("with a one-port range", func() {
		BeforeEach(func() { opts.ports = 1 })

		It("marks endpoints past the end of the range not ready", func() {
			createCR(e.ns, "tcp://full-a.ngrok.direct:5432")
			Expect(eventuallyReady(e.ns, "tcp://full-a.ngrok.direct:5432").Status.ForwarderPort).To(Equal(e.portMin))

			createCR(e.ns, "tcp://full-b.ngrok.direct:5432")
			cond := eventuallyNotReady(e.ns, "tcp://full-b.ngrok.direct:5432", "PortsExhausted")
			Expect(cond.Message).To(ContainSubstring("no free forwarder ports"))
			eventuallyNoService(e.ns, "full-b.ngrok.direct")
		})

		It("serves the endpoints that fit when one hostname has more than the range", func() {
			createCR(e.ns, "http://over.ngrok.direct")
			createCR(e.ns, "tcp://over.ngrok.direct:6379")

			// Endpoints are served in Service port order.
			http := eventuallyReady(e.ns, "http://over.ngrok.direct")
			eventuallyNotReady(e.ns, "tcp://over.ngrok.direct:6379", "PortsExhausted")
			eventuallyService(e.ns, "over.ngrok.direct", func(g Gomega, svc *corev1.Service) {
				g.Expect(servicePorts(svc)).To(Equal(map[int32]int32{80: http.Status.ForwarderPort}))
			})
		})
	})

	Context("when it starts on existing state", func() {
		BeforeEach(func() { opts.deferStart = true })

		It("keeps the forwarder ports endpoints already have", func() {
			cr := createCR(e.ns, "tcp://kept.ngrok.direct:5432")
			wireCR(cr, "10.0.0.1", e.portMin+7)

			e.start()

			eventuallyService(e.ns, "kept.ngrok.direct", func(g Gomega, svc *corev1.Service) {
				g.Expect(servicePorts(svc)).To(Equal(map[int32]int32{5432: e.portMin + 7}))
			})
			Expect(eventuallyReady(e.ns, "tcp://kept.ngrok.direct:5432").Status.ForwarderPort).To(Equal(e.portMin + 7))
		})

		It("reallocates forwarder ports outside the configured range", func() {
			cr := createCR(e.ns, "tcp://moved.ngrok.direct:5432")
			wireCR(cr, "10.0.0.1", 1)

			e.start()

			Eventually(func(g Gomega) {
				got := getCR(e.ns, "tcp://moved.ngrok.direct:5432")
				g.Expect(got.Status.ForwarderPort).To(BeNumerically(">=", e.portMin))
				g.Expect(got.Status.ForwarderPort).To(BeNumerically("<=", e.portMax))
			}).Should(Succeed())
		})

		It("moves the higher-named of two hostnames claiming one port", func() {
			// State left behind by an earlier race.
			a := createCR(e.ns, "tcp://dup-a.ngrok.direct:5432")
			b := createCR(e.ns, "tcp://dup-b.ngrok.direct:5432")
			wireCR(a, "10.0.0.1", e.portMin)
			wireCR(b, "10.0.0.2", e.portMin)

			e.start()

			Expect(eventuallyReady(e.ns, "tcp://dup-a.ngrok.direct:5432").Status.ForwarderPort).To(Equal(e.portMin))
			Eventually(func(g Gomega) {
				got := getCR(e.ns, "tcp://dup-b.ngrok.direct:5432")
				g.Expect(got.Status.ForwarderPort).NotTo(BeZero())
				g.Expect(got.Status.ForwarderPort).NotTo(Equal(e.portMin))
			}).Should(Succeed())
			assertNoSharedTargetPorts(e.ns)
		})
	})
})

// These specs inject faults at the controller's client boundary (its cached
// client and its APIReader, wrapped separately) and restart the manager over
// persisted state, to cover partial writes, a process restart or new leader
// between two writes, and errors controller-runtime must retry.
var _ = Describe("PrivateEndpoint controller under faults", func() {
	var (
		opts peEnvOpts
		e    *peEnv
	)

	BeforeEach(func() {
		opts = peEnvOpts{controller: true}
	})

	JustBeforeEach(func() {
		e = newPEEnv(opts)
	})

	errBoom := errors.New("boom")
	conflict := apierrors.NewConflict(ngrokv1.GroupVersion.WithResource("privateendpoints").GroupResource(), "", errBoom)

	svcName := func(host string) string {
		GinkgoHelper()
		return serviceKey(e.ns, host).Name
	}
	targetPort := func(host string) int32 {
		GinkgoHelper()
		var svc corev1.Service
		Expect(k8sClient.Get(ctx, serviceKey(e.ns, host), &svc)).To(Succeed())
		Expect(svc.Spec.Ports).To(HaveLen(1))
		return svc.Spec.Ports[0].TargetPort.IntVal
	}
	// touchService updates a managed Service, which enqueues its hostname,
	// and returns the calls of one reconcile pass that started after the
	// touch: from its PrivateEndpoint list (cached or not) up to its read of
	// the Service. A pass already running at the touch listed before it, so
	// can't be mistaken for one.
	touchService := func(host string) []call {
		GinkgoHelper()
		mark := len(e.boundary.snapshot())
		svc := &corev1.Service{}
		Expect(k8sClient.Get(ctx, serviceKey(e.ns, host), svc)).To(Succeed())
		if svc.Annotations == nil {
			svc.Annotations = map[string]string{}
		}
		svc.Annotations["touched"] = rand.String(6)
		Expect(k8sClient.Update(ctx, svc)).To(Succeed())
		var pass []call
		Eventually(func(g Gomega) {
			calls := e.boundary.snapshot()[mark:]
			start := slices.IndexFunc(calls, func(c call) bool { return c.Kind == "PrivateEndpointList" })
			g.Expect(start).NotTo(Equal(-1), "no reconcile started after the touch")
			end := slices.IndexFunc(calls[start:], func(c call) bool {
				return c.Op == "Get" && c.Kind == "Service" && c.Name == svcName(host)
			})
			g.Expect(end).NotTo(Equal(-1), "the reconcile hasn't reached the Service yet")
			pass = calls[start : start+end+1]
		}).Should(Succeed())
		return pass
	}

	It("allocates from uncached reads, including after a restart", func() {
		// The informer may not have seen the forwarder ports the previous
		// reconcile wrote, and reusing one cross-routes traffic, so
		// allocation must read PrivateEndpoints and managed Services through
		// APIReader. A stale cached read is usually masked (a Service keeps
		// routing an endpoint's port), so assert the reads themselves.
		const url = "tcp://lag.ngrok.direct:5432"
		createCR(e.ns, url)
		p := eventuallyReady(e.ns, url).Status.ForwarderPort

		for _, fresh := range []bool{false, true} {
			if fresh {
				By("restarting with a fresh allocator")
				e.restart()
			}
			pass := touchService("lag.ngrok.direct")

			Expect(pass[0].Op).To(Equal("ReaderList"), "PrivateEndpoints are listed uncached: %+v", pass[0])
			Expect(pass).To(ContainElement(And(HaveField("Op", "ReaderList"), HaveField("Kind", "ServiceList"))), "managed Services are listed uncached")
			Expect(pass).NotTo(ContainElement(HaveField("Op", "List")), "nothing in allocation is listed from the cache")
			Expect(e.boundary.count("List", "PrivateEndpointList", "")).To(BeZero(), "PrivateEndpoints are never listed from the cache")
			Expect(getCR(e.ns, url).Status.ForwarderPort).To(Equal(p))
			Expect(targetPort("lag.ngrok.direct")).To(Equal(p))
		}
	})

	DescribeTable("never routes two Services to one port when a status write fails after the Service write",
		func(restart bool) {
			urlA, urlB := "tcp://sw-a.ngrok.direct:5432", "tcp://sw-b.ngrok.direct:5432"
			statusA := e.boundary.inject(&fault{op: "UpdateStatus", kind: "PrivateEndpoint", name: crName(urlA), err: conflict})

			createCR(e.ns, urlA)
			Eventually(statusA.Hits).Should(BeNumerically(">=", 3), "controller-runtime retries A")
			p := targetPort("sw-a.ngrok.direct")
			Expect(getCR(e.ns, urlA).Status.ForwarderPort).To(BeZero(), "A's status write failed")
			// Moving A's port on every retry would rewrite its Service, and
			// each rewrite re-enqueues A ahead of hostnames queued at low
			// priority from a fresh manager's initial list, starving them.
			Expect(serviceTargets(e.boundary, "sw-a-ngrok-direct")).To(HaveEach(Equal([]int32{p})), "A keeps one port across retries")

			if restart {
				By("stopping the manager on the partial write and starting a fresh one")
				e.stop()
				createCR(e.ns, urlB)
				e.start()
			} else {
				createCR(e.ns, urlB)
			}

			b := eventuallyReady(e.ns, urlB)
			Expect(b.Status.ForwarderPort).NotTo(Equal(p), "B must not get the port A's Service routes to")
			Expect(targetPort("sw-a.ngrok.direct")).To(Equal(p))
			Expect(getCR(e.ns, urlA).Status.ForwarderPort).To(BeZero(), "A is still failing")

			statusA.Release()
			a := eventuallyReady(e.ns, urlA)
			Expect(a.Status.ForwarderPort).To(Equal(p), "A settles on the port its Service already routes")
			Expect(a.Status.ForwarderPort).NotTo(Equal(b.Status.ForwarderPort))
			Expect(e.boundary.crossRoutes()).To(BeEmpty())
			assertNoSharedTargetPorts(e.ns)
		},
		Entry("on the same manager", false),
		Entry("across a restart", true),
	)

	Context("with a two-port range", func() {
		BeforeEach(func() { opts.ports = 2 })

		It("keeps a deleted endpoint's port reserved across a restart while its Service survives", func() {
			createCR(e.ns, "tcp://gone-a.ngrok.direct:5432")
			p := eventuallyReady(e.ns, "tcp://gone-a.ngrok.direct:5432").Status.ForwarderPort
			Expect(p).To(Equal(e.portMin))

			By("deleting A while its Service's deletion fails")
			cleanup := e.boundary.inject(&fault{op: "Delete", kind: "Service", name: svcName("gone-a.ngrok.direct"), err: errBoom})
			deleteCR(e.ns, "tcp://gone-a.ngrok.direct:5432")
			Eventually(cleanup.Hits).Should(BeNumerically(">=", 1))

			// A fresh allocator seeds from what's in use: only A's surviving
			// Service still says p is.
			e.restart()
			createCR(e.ns, "tcp://gone-b.ngrok.direct:5432")
			Expect(eventuallyReady(e.ns, "tcp://gone-b.ngrok.direct:5432").Status.ForwarderPort).To(Equal(e.portMax))
			Expect(targetPort("gone-a.ngrok.direct")).To(Equal(p), "A's Service still routes p")

			By("letting the cleanup through, which frees p")
			cleanup.Release()
			eventuallyNoService(e.ns, "gone-a.ngrok.direct")
			createCR(e.ns, "tcp://gone-c.ngrok.direct:5432")
			Expect(eventuallyReady(e.ns, "tcp://gone-c.ngrok.direct:5432").Status.ForwarderPort).To(Equal(p))
			Expect(e.boundary.crossRoutes()).To(BeEmpty())
		})
	})

	It("reports ServiceNotReady until the Service has a ClusterIP, keeping the port", func() {
		const url = "tcp://nr.ngrok.direct:5432"
		// envtest assigns ClusterIPs synchronously. Hide it in the
		// controller's reads and write results instead of storing an invalid
		// Service; with nothing else changed, the controller never writes it.
		noIP := e.boundary.inject(&fault{kind: "Service", name: svcName("nr.ngrok.direct"), mutate: func(o runtime.Object) {
			o.(*corev1.Service).Spec.ClusterIP = ""
		}})
		createCR(e.ns, url)

		var p int32
		notReady := func(g Gomega) {
			cr := getCR(e.ns, url)
			cond := meta.FindStatusCondition(cr.Status.Conditions, ngrokv1.PrivateEndpointConditionReady)
			g.Expect(cond).NotTo(BeNil())
			g.Expect(cond.Status).To(Equal(metav1.ConditionFalse))
			g.Expect(cond.Reason).To(Equal("ServiceNotReady"))
			g.Expect(cr.Status.ClusterIP).To(BeEmpty())
			g.Expect(cr.Status.ForwarderPort).NotTo(BeZero(), "the port is reserved before the IP shows up")
			p = cr.Status.ForwarderPort
		}
		Eventually(notReady).Should(Succeed())
		hits := noIP.Hits()
		By("requeueing while the IP is missing")
		Eventually(noIP.Hits).WithTimeout(5 * time.Second).Should(BeNumerically(">", hits))
		Expect(getCR(e.ns, url).Status.ForwarderPort).To(Equal(p))
		Consistently(notReady).WithTimeout(time.Second).Should(Succeed())

		noIP.Release()
		ready := eventuallyReady(e.ns, url)
		Expect(ready.Status.ForwarderPort).To(Equal(p))
		Expect(ready.Status.ClusterIP).To(Equal(eventuallyService(e.ns, "nr.ngrok.direct", func(Gomega, *corev1.Service) {}).Spec.ClusterIP))
	})

	// Nothing enqueues these hostnames when the obstacle goes away, so only
	// the controller's retryAfter poll recovers them. They run side by side
	// in one spec so the package pays for that wait once.
	It("recovers from conflicts that only its retry poll can see clearing", func() {
		By("blocking one hostname with an unmanaged Service")
		unmanaged := e
		foreign := &corev1.Service{
			Name: "polled-ngrok-direct", Namespace: unmanaged.ns, Labels: map[string]string{"app": "someone-else"},
			Spec: corev1.ServiceSpec{Ports: []corev1.ServicePort{{Port: 8080}}},
		}
		Expect(k8sClient.Create(ctx, foreign)).To(Succeed())
		createCR(unmanaged.ns, "http://polled.ngrok.direct")

		By("blocking another with a managed Service of a different hostname, whose cleanup is held")
		other := newPEEnv(peEnvOpts{controller: true, deferStart: true})
		otherSvc := &corev1.Service{
			Name: "clash-ngrok-direct", Namespace: other.ns,
			Labels: map[string]string{pe.ManagedByLabel: pe.ManagedByValue, pe.HostLabel: "someotherhost"},
			Spec:   corev1.ServiceSpec{Ports: []corev1.ServicePort{{Port: 80}}},
		}
		Expect(k8sClient.Create(ctx, otherSvc)).To(Succeed())
		held := other.boundary.inject(&fault{op: "Delete", kind: "Service", name: "clash-ngrok-direct", err: errBoom})
		other.start()
		createCR(other.ns, "http://clash.ngrok.direct")

		By("exhausting a one-port range")
		full := newPEEnv(peEnvOpts{controller: true, ports: 1})
		createCR(full.ns, "tcp://ex-a.ngrok.direct:5432")
		eventuallyReady(full.ns, "tcp://ex-a.ngrok.direct:5432")
		createCR(full.ns, "tcp://ex-b.ngrok.direct:5432")

		Expect(eventuallyNotReady(unmanaged.ns, "http://polled.ngrok.direct", "ServiceConflict").Message).To(ContainSubstring("polled-ngrok-direct"))
		eventuallyNotReady(other.ns, "http://clash.ngrok.direct", "ServiceConflict")
		Expect(held.Hits()).To(BeNumerically(">=", 1), "the stale Service's cleanup was attempted")
		Expect(eventuallyNotReady(full.ns, "tcp://ex-b.ngrok.direct:5432", "PortsExhausted").Message).To(ContainSubstring("no free forwarder ports"))
		kginkgo.ConsistentlyExpectResourceVersionNotToChange(ctx, foreign, testutils.WithTimeout(time.Second))
		Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(otherSvc), otherSvc)).To(Succeed())
		Expect(otherSvc.Labels[pe.HostLabel]).To(Equal("someotherhost"), "not adopted")

		By("clearing every obstacle without touching the blocked endpoints")
		Expect(k8sClient.Delete(ctx, foreign)).To(Succeed())
		held.Release()
		eventuallyNoService(other.ns, "clash.ngrok.direct")
		deleteCR(full.ns, "tcp://ex-a.ngrok.direct:5432")
		eventuallyNoService(full.ns, "ex-a.ngrok.direct")

		stillBlocked := func(g Gomega) {
			for _, c := range []struct{ ns, url, reason string }{
				{unmanaged.ns, "http://polled.ngrok.direct", "ServiceConflict"},
				{other.ns, "http://clash.ngrok.direct", "ServiceConflict"},
				{full.ns, "tcp://ex-b.ngrok.direct:5432", "PortsExhausted"},
			} {
				cond := meta.FindStatusCondition(getCR(c.ns, c.url).Status.Conditions, ngrokv1.PrivateEndpointConditionReady)
				g.Expect(cond.Reason).To(Equal(c.reason))
			}
		}
		Consistently(stillBlocked).WithTimeout(2*time.Second).Should(Succeed(), "no event recovers them")

		poll := testutils.WithTimeout(retryAfter + 15*time.Second)
		eventuallyService(unmanaged.ns, "polled.ngrok.direct", func(g Gomega, svc *corev1.Service) {
			g.Expect(svc.Labels).To(HaveKeyWithValue(pe.ManagedByLabel, pe.ManagedByValue))
		}, poll)
		eventuallyReady(unmanaged.ns, "http://polled.ngrok.direct")
		eventuallyService(other.ns, "clash.ngrok.direct", func(g Gomega, svc *corev1.Service) {
			g.Expect(svc.Labels).To(HaveKeyWithValue(pe.HostLabel, pe.HostKey("clash.ngrok.direct")))
		}, poll)
		eventuallyReady(other.ns, "http://clash.ngrok.direct")
		b := eventuallyReady(full.ns, "tcp://ex-b.ngrok.direct:5432")
		Expect(b.Status.ForwarderPort).To(Equal(full.portMin))
		eventuallyService(full.ns, "ex-b.ngrok.direct", func(g Gomega, svc *corev1.Service) {
			g.Expect(servicePorts(svc)).To(Equal(map[int32]int32{5432: full.portMin}))
		})
	})

	DescribeTable("retries failed API calls without writing partial state",
		func(newFault func() *fault) {
			const url = "http://retry.ngrok.direct"
			f := e.boundary.inject(newFault())
			createCR(e.ns, url)

			Eventually(f.Hits).Should(BeNumerically(">=", 2), "controller-runtime retries the failure")
			Expect(apierrors.IsNotFound(k8sClient.Get(ctx, serviceKey(e.ns, "retry.ngrok.direct"), &corev1.Service{}))).To(BeTrue())
			Expect(getCR(e.ns, url).Status).To(Equal(ngrokv1.PrivateEndpointStatus{}))

			f.Release()
			cr := eventuallyReady(e.ns, url)
			eventuallyService(e.ns, "retry.ngrok.direct", func(g Gomega, svc *corev1.Service) {
				g.Expect(servicePorts(svc)).To(Equal(map[int32]int32{80: cr.Status.ForwarderPort}))
			})
		},
		Entry("listing PrivateEndpoints through APIReader", func() *fault { return &fault{op: "ReaderList", kind: "PrivateEndpointList", err: errBoom} }),
		Entry("listing managed Services through APIReader", func() *fault { return &fault{op: "ReaderList", kind: "ServiceList", err: errBoom} }),
		Entry("creating the Service", func() *fault { return &fault{op: "Create", kind: "Service", err: errBoom} }),
	)

	It("retries listing a removed hostname's Services until it can clean them up", func() {
		const url = "http://cleanup.ngrok.direct"
		createCR(e.ns, url)
		eventuallyReady(e.ns, url)
		list := e.boundary.inject(&fault{op: "List", kind: "ServiceList", err: errBoom})

		deleteCR(e.ns, url)
		Eventually(list.Hits).Should(BeNumerically(">=", 2))
		Expect(k8sClient.Get(ctx, serviceKey(e.ns, "cleanup.ngrok.direct"), &corev1.Service{})).To(Succeed(), "the Service survives a failed cleanup")

		list.Release()
		eventuallyNoService(e.ns, "cleanup.ngrok.direct")
	})
})

// serviceTargets returns the target ports of every successful write of the
// named Service through b.
func serviceTargets(b *boundary, name string) [][]int32 {
	var out [][]int32
	for _, c := range b.snapshot() {
		if c.Kind == "Service" && c.Name == name && c.Err == nil && (c.Op == "Create" || c.Op == "Update") {
			out = append(out, c.TargetPorts)
		}
	}
	return out
}

// assertNoSharedTargetPorts fails if two managed Services route to the same
// forwarder port.
func assertNoSharedTargetPorts(ns string) {
	GinkgoHelper()
	var svcs corev1.ServiceList
	Expect(k8sClient.List(ctx, &svcs, client.InNamespace(ns), client.MatchingLabels{pe.ManagedByLabel: pe.ManagedByValue})).To(Succeed())
	seen := map[int32]string{}
	for _, svc := range svcs.Items {
		for _, p := range svc.Spec.Ports {
			prev, dup := seen[p.TargetPort.IntVal]
			Expect(dup).To(BeFalse(), "Services %s and %s share targetPort %d", prev, svc.Name, p.TargetPort.IntVal)
			seen[p.TargetPort.IntVal] = svc.Name
		}
	}
}
