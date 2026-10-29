package privateendpoints

import (
	"context"
	"errors"
	"maps"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/ngrok/ngrok-api-go/v9"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/client-go/tools/events"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"

	ngrokv1 "github.com/ngrok/ngrok-operator/api/ngrok/v1"
	"github.com/ngrok/ngrok-operator/internal/mocks/nmockapi"
	pe "github.com/ngrok/ngrok-operator/internal/privateendpoints"
)

// endpointLister guards the mock API, which isn't safe for concurrent use,
// so a running poller can read it while the spec changes it. With err set,
// List yields the endpoints and then reports err, as a list that fails on a
// later page would.
type endpointLister struct {
	mu    sync.Mutex
	eps   *nmockapi.EndpointsClient
	err   error
	lists int
}

func (l *endpointLister) List(paging *ngrok.Paging) ngrok.Iter[*ngrok.Endpoint] {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.lists++
	it := l.eps.List(paging)
	if l.err == nil {
		return it
	}
	var items []*ngrok.Endpoint
	for it.Next(context.Background()) {
		items = append(items, it.Item())
	}
	return &itemsThenErr{items: items, err: l.err}
}

func (l *endpointLister) add(url string, bindings ...string) *ngrok.Endpoint {
	GinkgoHelper()
	l.mu.Lock()
	defer l.mu.Unlock()
	ep, err := l.eps.Create(ctx, &ngrok.EndpointCreate{URL: url, Bindings: bindings})
	Expect(err).NotTo(HaveOccurred())
	return ep
}

func (l *endpointLister) remove(ep *ngrok.Endpoint) {
	GinkgoHelper()
	l.mu.Lock()
	defer l.mu.Unlock()
	Expect(l.eps.Delete(ctx, ep.ID)).To(Succeed())
}

func (l *endpointLister) failWith(err error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.err = err
}

func (l *endpointLister) listCount() int {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.lists
}

type itemsThenErr struct {
	items []*ngrok.Endpoint
	err   error
	n     int
}

func (it *itemsThenErr) Next(context.Context) bool {
	it.n++
	return it.n <= len(it.items)
}

func (it *itemsThenErr) Item() *ngrok.Endpoint { return it.items[it.n-1] }

func (it *itemsThenErr) Err() error {
	if it.n > len(it.items) {
		return it.err
	}
	return nil
}

// The poller isn't a reconciler: it is a runnable that calls Sync on a
// ticker. Like the bindings poller suite, most specs trigger Sync by hand
// against envtest for deterministic API fixtures; the last one runs the
// ticker.
var _ = Describe("PrivateEndpoint poller", func() {
	var (
		ns       string
		api      *endpointLister
		calls    *boundary
		recorder *events.FakeRecorder
		poller   *PrivateEndpointPoller
	)

	BeforeEach(func() {
		ns = createNamespace()
		api = &endpointLister{eps: nmockapi.NewEndpointsClient()}
		calls = &boundary{}
		recorder = events.NewFakeRecorder(10)
		poller = &PrivateEndpointPoller{
			Client:    interceptor.NewClient(k8sClient, calls.funcs("")),
			Log:       GinkgoLogr,
			Recorder:  recorder,
			Namespace: ns,
			Endpoints: api,
			Interval:  time.Hour,
		}
	})

	crURLs := func() []string {
		GinkgoHelper()
		var list ngrokv1.PrivateEndpointList
		Expect(k8sClient.List(ctx, &list, client.InNamespace(ns))).To(Succeed())
		var urls []string
		for _, cr := range list.Items {
			urls = append(urls, cr.Spec.URL)
		}
		return urls
	}
	writes := func() []call {
		var out []call
		for _, c := range calls.snapshot() {
			if c.Op != "Get" && c.Op != "List" {
				out = append(out, c)
			}
		}
		return out
	}
	// indexOf returns the position of the first recorded call matching op
	// and name.
	indexOf := func(op, name string) int {
		GinkgoHelper()
		i := slices.IndexFunc(calls.snapshot(), func(c call) bool { return c.Op == op && c.Name == name })
		Expect(i).NotTo(Equal(-1), "no %s of %s recorded", op, name)
		return i
	}
	drift := func(url string) {
		GinkgoHelper()
		cr := getCR(ns, url)
		cr.Spec.Port = 1
		cr.Labels["team"] = "a"
		Expect(k8sClient.Update(ctx, cr)).To(Succeed())
	}

	It("mirrors only ngrok.direct endpoints that aren't kubernetes-bound", func() {
		api.add("https://foo.ngrok.direct", "internal")
		api.add("tcp://bar.ngrok.direct:6379", "internal")
		api.add("tls://baz.ngrok.direct")
		api.add("http://foo.internal", "internal")
		api.add("https://public.ngrok.app", "public")
		api.add("http://svc.ns", "kubernetes")
		api.add("http://legacy.ngrok.direct", "kubernetes")
		api.add("tcp://noport.ngrok.direct")

		Expect(poller.Sync(ctx)).To(Succeed())

		Expect(crURLs()).To(ConsistOf("https://foo.ngrok.direct", "tcp://bar.ngrok.direct:6379", "tls://baz.ngrok.direct"))
	})

	It("labels the CRs it creates and fills in the parsed spec", func() {
		api.add("tcp://Bar.ngrok.direct:6379")

		Expect(poller.Sync(ctx)).To(Succeed())

		cr := getCR(ns, "tcp://bar.ngrok.direct:6379")
		Expect(cr.Labels).To(Equal(map[string]string{pe.ManagedByLabel: pe.ManagedByValue}))
		Expect(cr.Spec).To(Equal(ngrokv1.PrivateEndpointSpec{
			URL: "tcp://Bar.ngrok.direct:6379", Scheme: ngrokv1.PrivateEndpointSchemeTCP, Hostname: "bar.ngrok.direct", Port: 6379,
		}))
	})

	It("writes nothing when the CRs already match", func() {
		api.add("tcp://bar.ngrok.direct:6379")
		Expect(poller.Sync(ctx)).To(Succeed())
		cr := getCR(ns, "tcp://bar.ngrok.direct:6379")
		Expect(writes()).To(HaveLen(1), "the first sync creates the CR")

		Expect(poller.Sync(ctx)).To(Succeed())

		Expect(writes()).To(HaveLen(1))
		Expect(calls.count("List", "PrivateEndpointList", "")).To(Equal(2), "the second sync ran")
		Expect(getCR(ns, "tcp://bar.ngrok.direct:6379").ResourceVersion).To(Equal(cr.ResourceVersion))
	})

	It("deletes managed CRs whose endpoint is gone and keeps hand-made ones", func() {
		gone := api.add("http://gone.ngrok.direct")
		api.add("http://kept.ngrok.direct")
		Expect(poller.Sync(ctx)).To(Succeed())
		handmade := newCR(ns, "http://handmade.ngrok.direct")
		delete(handmade.Labels, pe.ManagedByLabel)
		Expect(k8sClient.Create(ctx, handmade)).To(Succeed())

		api.remove(gone)
		Expect(poller.Sync(ctx)).To(Succeed())

		Expect(crURLs()).To(ConsistOf("http://kept.ngrok.direct", "http://handmade.ngrok.direct"))
	})

	DescribeTable("changes nothing when the API list fails",
		func(fail func()) {
			api.add("http://foo.ngrok.direct")
			gone := api.add("https://foo.ngrok.direct")
			Expect(poller.Sync(ctx)).To(Succeed())
			synced := len(writes())

			By("changing the API so a successful sync would delete one CR and create another")
			api.remove(gone)
			api.add("tcp://new.ngrok.direct:5432")
			fail()

			Expect(poller.Sync(ctx)).To(MatchError(ContainSubstring("listing endpoints")))

			Expect(writes()).To(HaveLen(synced))
			Expect(crURLs()).To(ConsistOf("http://foo.ngrok.direct", "https://foo.ngrok.direct"))
		},
		Entry("before yielding any endpoints", func() { api.eps.SetListError(errors.New("api down")) }),
		Entry("after yielding some endpoints", func() { api.failWith(errors.New("page 2 failed")) }),
	)

	It("doesn't adopt a hand-made CR with the name it would use", func() {
		handmade := newCR(ns, "tcp://bar.ngrok.direct:6379")
		delete(handmade.Labels, pe.ManagedByLabel)
		handmade.Spec.Port = 22
		Expect(k8sClient.Create(ctx, handmade)).To(Succeed())
		api.add("tcp://bar.ngrok.direct:6379")

		Expect(poller.Sync(ctx)).To(Succeed())

		got := getCR(ns, "tcp://bar.ngrok.direct:6379")
		Expect(got.Labels).NotTo(HaveKey(pe.ManagedByLabel))
		Expect(got.Spec.Port).To(Equal(int32(22)))
		Expect(got.ResourceVersion).To(Equal(handmade.ResourceVersion))
		Expect(recorder.Events).To(Receive(Equal("Warning NotAdopted Not managing this PrivateEndpoint for tcp://bar.ngrok.direct:6379: it has no ngrok.com/managed-by label")))
	})

	DescribeTable("puts a managed CR back when it drifts",
		func(mutate func(*ngrokv1.PrivateEndpoint), wantExtraLabels map[string]string) {
			const url = "tcp://bar.ngrok.direct:6379"
			api.add(url)
			Expect(poller.Sync(ctx)).To(Succeed())
			want := getCR(ns, url)

			cr := getCR(ns, url)
			mutate(cr)
			Expect(k8sClient.Update(ctx, cr)).To(Succeed())

			Expect(poller.Sync(ctx)).To(Succeed())

			got := getCR(ns, url)
			Expect(got.Spec).To(Equal(want.Spec))
			wantLabels := map[string]string{}
			maps.Copy(wantLabels, want.Labels)
			maps.Copy(wantLabels, wantExtraLabels)
			Expect(got.Labels).To(Equal(wantLabels))
		},
		Entry("edited spec", func(cr *ngrokv1.PrivateEndpoint) {
			cr.Spec = ngrokv1.PrivateEndpointSpec{URL: "http://evil.ngrok.direct", Scheme: ngrokv1.PrivateEndpointSchemeHTTP, Hostname: "evil.ngrok.direct", Port: 80}
		}, nil),
		Entry("edited spec, keeping foreign labels", func(cr *ngrokv1.PrivateEndpoint) {
			cr.Spec.Port = 1
			cr.Labels["team"] = "a"
		}, map[string]string{"team": "a"}),
	)

	It("leaves a CR alone once its managed-by label is removed", func() {
		// Without the label it is indistinguishable from a hand-made CR.
		const url = "tcp://bar.ngrok.direct:6379"
		api.add(url)
		Expect(poller.Sync(ctx)).To(Succeed())
		cr := getCR(ns, url)
		delete(cr.Labels, pe.ManagedByLabel)
		cr.Spec.Port = 22
		Expect(k8sClient.Update(ctx, cr)).To(Succeed())

		Expect(poller.Sync(ctx)).To(Succeed())

		got := getCR(ns, url)
		Expect(got.Spec.Port).To(Equal(int32(22)))
		Expect(got.ResourceVersion).To(Equal(cr.ResourceVersion))
	})

	It("leaves status alone when healing", func() {
		const url = "tcp://bar.ngrok.direct:6379"
		api.add(url)
		Expect(poller.Sync(ctx)).To(Succeed())
		cr := getCR(ns, url)
		wireCR(cr, "10.0.0.5", 21000)
		cr.Spec.Port = 1
		Expect(k8sClient.Update(ctx, cr)).To(Succeed())

		Expect(poller.Sync(ctx)).To(Succeed())

		got := getCR(ns, url)
		Expect(got.Spec.Port).To(Equal(int32(6379)))
		Expect(got.Status.ClusterIP).To(Equal("10.0.0.5"))
		Expect(got.Status.ForwarderPort).To(Equal(int32(21000)))
	})

	errBoom := errors.New("boom")

	It("aborts without writing when listing CRs fails", func() {
		api.add("http://new.ngrok.direct")
		list := calls.inject(&fault{op: "List", kind: "PrivateEndpointList", err: errBoom})

		err := poller.Sync(ctx)

		Expect(err).To(MatchError(ContainSubstring("listing PrivateEndpoints")))
		Expect(err).To(MatchError(errBoom))
		Expect(list.Hits()).To(Equal(int32(1)))
		Expect(writes()).To(BeEmpty())
		Expect(crURLs()).To(BeEmpty())
	})

	// Existing CRs are healed or deleted in list order (by name), then
	// missing ones are created. Each spec fails an operation that runs before
	// the one it requires to succeed, so an early return can't pass.
	It("creates missing CRs after failing to heal one", func() {
		api.add("http://a.ngrok.direct")
		Expect(poller.Sync(ctx)).To(Succeed())
		wireCR(getCR(ns, "http://a.ngrok.direct"), "10.0.0.5", 21000)
		drift("http://a.ngrok.direct")
		api.add("http://b.ngrok.direct")
		heal := calls.inject(&fault{op: "Update", name: crName("http://a.ngrok.direct"), err: errBoom})

		err := poller.Sync(ctx)

		Expect(err).To(MatchError(ContainSubstring("restoring PrivateEndpoint " + crName("http://a.ngrok.direct"))))
		Expect(heal.Hits()).To(Equal(int32(1)))
		Expect(indexOf("Update", crName("http://a.ngrok.direct"))).To(BeNumerically("<", indexOf("Create", crName("http://b.ngrok.direct"))))
		Expect(crURLs()).To(ConsistOf("http://a.ngrok.direct", "http://b.ngrok.direct"))
		a := getCR(ns, "http://a.ngrok.direct")
		Expect(a.Spec.Port).To(Equal(int32(1)), "the failed heal changed nothing")
		Expect(a.Labels).To(HaveKeyWithValue("team", "a"))
		Expect(a.Status.ClusterIP).To(Equal("10.0.0.5"))
	})

	It("heals and deletes later CRs after failing to delete one", func() {
		for _, url := range []string{"http://a-gone.ngrok.direct", "http://m.ngrok.direct", "http://z-gone.ngrok.direct"} {
			api.add(url)
		}
		Expect(poller.Sync(ctx)).To(Succeed())
		drift("http://m.ngrok.direct")
		api.eps.Reset()
		api.add("http://m.ngrok.direct")
		del := calls.inject(&fault{op: "Delete", name: crName("http://a-gone.ngrok.direct"), err: errBoom})

		err := poller.Sync(ctx)

		Expect(err).To(MatchError(ContainSubstring("deleting PrivateEndpoint " + crName("http://a-gone.ngrok.direct"))))
		Expect(del.Hits()).To(Equal(int32(1)))
		failed := indexOf("Delete", crName("http://a-gone.ngrok.direct"))
		Expect(failed).To(BeNumerically("<", indexOf("Update", crName("http://m.ngrok.direct"))))
		Expect(failed).To(BeNumerically("<", indexOf("Delete", crName("http://z-gone.ngrok.direct"))))
		Expect(crURLs()).To(ConsistOf("http://a-gone.ngrok.direct", "http://m.ngrok.direct"))
		Expect(getCR(ns, "http://m.ngrok.direct").Spec.Port).To(Equal(int32(80)))
	})

	It("creates the other missing CRs after failing to create one", func() {
		urls := []string{"http://c1.ngrok.direct", "http://c2.ngrok.direct", "http://c3.ngrok.direct"}
		for _, url := range urls {
			api.add(url)
		}
		// Creates run in map order, so fail whichever comes first.
		create := calls.inject(&fault{op: "Create", kind: "PrivateEndpoint", err: errBoom, first: true})

		err := poller.Sync(ctx)

		Expect(err).To(MatchError(errBoom))
		Expect(create.Hits()).To(Equal(int32(1)))
		creates := slices.DeleteFunc(calls.snapshot(), func(c call) bool { return c.Op != "Create" })
		Expect(creates).To(HaveLen(3))
		Expect(creates[0].Err).To(MatchError(errBoom))
		Expect(creates[1].Err).NotTo(HaveOccurred())
		Expect(creates[2].Err).NotTo(HaveOccurred())
		failedURL := "http://" + getCRHost(creates[0].Name) + ".ngrok.direct"
		Expect(err).To(MatchError(ContainSubstring("creating PrivateEndpoint for " + failedURL)))
		Expect(crURLs()).To(ConsistOf(slices.DeleteFunc(slices.Clone(urls), func(u string) bool { return u == failedURL })))
	})

	It("keeps syncing on its interval through API errors until stopped", func() {
		api.failWith(errors.New("api down"))
		poller.Interval = 50 * time.Millisecond
		pctx, pcancel := context.WithCancel(ctx)
		done := make(chan error, 1)
		go func() { done <- poller.Start(pctx) }()
		DeferCleanup(pcancel)

		Eventually(api.listCount).Should(BeNumerically(">=", 3), "failed syncs don't stop the ticker")
		Expect(crURLs()).To(BeEmpty())

		By("recovering once the API does")
		api.add("tcp://tick.ngrok.direct:5432")
		api.failWith(nil)
		Eventually(crURLs).Should(ConsistOf("tcp://tick.ngrok.direct:5432"))

		By("recreating a deleted CR on a later tick")
		deleteCR(ns, "tcp://tick.ngrok.direct:5432")
		Eventually(crURLs).Should(ConsistOf("tcp://tick.ngrok.direct:5432"))

		pcancel()
		Eventually(done).Should(Receive(BeNil()))
	})
})

// getCRHost returns the first DNS label of a CR name such as
// "c1.ngrok.direct-http-80".
func getCRHost(name string) string {
	host, _, _ := strings.Cut(name, ".")
	return host
}

var _ = DescribeTable("desiredSpecs",
	func(eps []*ngrok.Endpoint, want map[string]string) {
		got := map[string]string{}
		for name, spec := range desiredSpecs(GinkgoLogr, eps) {
			got[name] = spec.URL
		}
		Expect(got).To(Equal(want))
	},
	Entry("no endpoints", nil, map[string]string{}),
	Entry("pooled endpoints sharing a URL collapse into one",
		[]*ngrok.Endpoint{{ID: "ep_1", URL: "tcp://bar.ngrok.direct:6379"}, {ID: "ep_2", URL: "tcp://bar.ngrok.direct:6379"}},
		map[string]string{"bar.ngrok.direct-tcp-6379": "tcp://bar.ngrok.direct:6379"}),
	Entry("URLs differing only in case and default port collapse into one",
		[]*ngrok.Endpoint{{ID: "ep_1", URL: "http://foo.ngrok.direct"}, {ID: "ep_2", URL: "http://FOO.ngrok.direct:80"}},
		map[string]string{"foo.ngrok.direct-http-80": "http://FOO.ngrok.direct:80"}),
	Entry("a kubernetes binding alongside others still skips the endpoint",
		[]*ngrok.Endpoint{{ID: "ep_1", URL: "http://foo.ngrok.direct", Bindings: []string{"internal", "kubernetes"}}},
		map[string]string{}),
	Entry("unparseable and non-private URLs are skipped",
		[]*ngrok.Endpoint{{ID: "ep_1", URL: "://"}, {ID: "ep_2", URL: "udp://foo.ngrok.direct:53"}, {ID: "ep_3", URL: "https://foo.ngrok.app"}, {ID: "ep_4", URL: "https://ngrok.direct"}},
		map[string]string{}),
)
