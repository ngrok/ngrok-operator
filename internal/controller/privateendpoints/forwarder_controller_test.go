package privateendpoints

import (
	"fmt"
	"io"
	"net"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// These specs run the forwarder without the PrivateEndpoint controller and
// write endpoint statuses directly, standing in for it.
var _ = Describe("Forwarder controller", func() {
	var (
		opts peEnvOpts
		e    *peEnv
	)

	BeforeEach(func() {
		opts = peEnvOpts{forwarder: true}
	})

	JustBeforeEach(func() {
		e = newPEEnv(opts)
	})

	// holdPort occupies a forwarder port the way another process would. It
	// binds the wildcard address like bindingsdriver: on BSD/macOS a wildcard
	// bind can succeed next to a loopback-only listener, which would block
	// nothing.
	holdPort := func(port int32) net.Listener {
		GinkgoHelper()
		l, err := net.Listen("tcp", fmt.Sprintf(":%d", port))
		Expect(err).NotTo(HaveOccurred())
		DeferCleanup(func() { _ = l.Close() })
		return l
	}

	It("reports synced after the first sync even with no endpoints", func() {
		Eventually(e.forwarder.Table.Synced).Should(BeTrue())
		Expect(e.forwarder.Ports()).To(BeEmpty())
		Expect(e.forwarder.BindErr()).NotTo(HaveOccurred())
	})

	It("listens only for wired endpoints in its namespace, and routes each port to its endpoint", func() {
		createCR(e.ns, "http://unwired.ngrok.direct")
		wireCR(createCR(e.ns, "tcp://bar.ngrok.direct:6379"), "10.0.0.2", e.portMin)
		wireCR(createCR(e.ns, "https://baz.ngrok.direct"), "10.0.0.3", e.portMin+1)
		By("wiring an endpoint in another namespace")
		wireCR(createCR(createNamespace(), "tcp://else.ngrok.direct:80"), "10.0.0.9", e.portMin+2)
		By("half-wiring endpoints: a port without a ClusterIP, and a ClusterIP without a port")
		wireCR(createCR(e.ns, "tcp://noip.ngrok.direct:80"), "", e.portMin+3)
		wireCR(createCR(e.ns, "tcp://noport.ngrok.direct:80"), "10.0.0.4", 0)

		Eventually(e.forwarder.Ports).Should(Equal([]int32{e.portMin, e.portMin + 1}))
		Consistently(e.forwarder.Ports).WithTimeout(time.Second).Should(Equal([]int32{e.portMin, e.portMin + 1}))
		e.eventuallyRoutes(e.portMin, "bar.ngrok.direct:6379")
		e.eventuallyRoutes(e.portMin+1, "baz.ngrok.direct:443")
	})

	It("forwards bytes both ways once routed", func() {
		wireCR(createCR(e.ns, "tcp://echo.ngrok.direct:7000"), "10.0.0.2", e.portMin)
		e.eventuallyRoutes(e.portMin, "echo.ngrok.direct:7000")

		c, err := dialForwarder(e.portMin)
		Expect(err).NotTo(HaveOccurred())
		defer c.Close()
		_ = c.SetDeadline(time.Now().Add(2 * time.Second))
		greeting := make([]byte, len("[echo.ngrok.direct:7000]"))
		_, err = io.ReadFull(c, greeting)
		Expect(err).NotTo(HaveOccurred())
		_, err = io.WriteString(c, "ping")
		Expect(err).NotTo(HaveOccurred())
		reply := make([]byte, 4)
		_, err = io.ReadFull(c, reply)
		Expect(err).NotTo(HaveOccurred())
		Expect(string(reply)).To(Equal("ping"))
	})

	It("closes a listener when its endpoint is deleted", func() {
		wireCR(createCR(e.ns, "tcp://old.ngrok.direct:6379"), "10.0.0.2", e.portMin)
		e.eventuallyRoutes(e.portMin, "old.ngrok.direct:6379")

		deleteCR(e.ns, "tcp://old.ngrok.direct:6379")

		Eventually(e.forwarder.Ports).Should(BeEmpty())
		eventuallyNotListening(e.portMin)
	})

	It("closes a listener when its endpoint is no longer wired", func() {
		cr := createCR(e.ns, "tcp://unwire.ngrok.direct:6379")
		wireCR(cr, "10.0.0.2", e.portMin)
		e.eventuallyRoutes(e.portMin, "unwire.ngrok.direct:6379")

		wireCR(cr, "", 0)

		Eventually(e.forwarder.Ports).Should(BeEmpty())
		eventuallyNotListening(e.portMin)
	})

	It("moves an endpoint to its new forwarder port", func() {
		cr := createCR(e.ns, "tcp://mover.ngrok.direct:6379")
		wireCR(cr, "10.0.0.2", e.portMin)
		e.eventuallyRoutes(e.portMin, "mover.ngrok.direct:6379")

		wireCR(cr, "10.0.0.2", e.portMin+1)

		Eventually(e.forwarder.Ports).Should(Equal([]int32{e.portMin + 1}))
		e.eventuallyRoutes(e.portMin+1, "mover.ngrok.direct:6379")
		eventuallyNotListening(e.portMin)
	})

	It("refuses connections on a port two endpoints claim, then routes it to whichever keeps it", func() {
		a := createCR(e.ns, "tcp://a.ngrok.direct:5432")
		wireCR(a, "10.0.0.2", e.portMin)
		e.eventuallyRoutes(e.portMin, "a.ngrok.direct:5432")

		By("handing the port to b while a still holds it")
		wireCR(createCR(e.ns, "tcp://b.ngrok.direct:5432"), "10.0.0.3", e.portMin)

		Eventually(e.ambiguousLogs.Load).Should(Equal(int32(1)))
		// The earlier dials to a were valid; the forbidden window opens once
		// a connection is refused, which proves the forwarder sees both.
		Eventually(func(g Gomega) { expectRefused(g, e.portMin) }).Should(Succeed())
		dials := len(e.dialer.Targets())
		for range 3 {
			expectRefused(Default, e.portMin)
		}
		Expect(e.dialer.Targets()[dials:]).To(BeEmpty(), "an ambiguous port must not dial")

		By("syncing again without changing the duplicate")
		lists := e.fwdBoundary.count("List", "PrivateEndpointList", "")
		a.Annotations = map[string]string{"touched": "yes"}
		Expect(k8sClient.Update(ctx, a)).To(Succeed())
		Eventually(func() int { return e.fwdBoundary.count("List", "PrivateEndpointList", "") }).Should(BeNumerically(">", lists))
		Expect(e.ambiguousLogs.Load()).To(Equal(int32(1)), "a duplicate is logged once per change, not per sync")
		Expect(e.dialer.Targets()[dials:]).To(BeEmpty())

		By("unwiring a, leaving b the port")
		wireCR(a, "", 0)

		e.eventuallyRoutes(e.portMin, "b.ngrok.direct:5432")
		Expect(e.forwarder.Ports()).To(Equal([]int32{e.portMin}))
	})

	Context("when it starts on existing state", func() {
		BeforeEach(func() { opts.deferStart = true })

		It("doesn't let its startup sync overwrite a newer reconcile", func() {
			By("holding the startup sync after it reads an empty snapshot")
			startup := e.fwdBoundary.inject(&fault{op: "List", kind: "PrivateEndpointList", hold: true, first: true})
			e.launch()
			Eventually(startup.Hits).Should(Equal(int32(1)))

			By("wiring an endpoint, which reconciles while startup still holds its snapshot")
			wireCR(createCR(e.ns, "http://new.ngrok.direct"), "10.0.0.1", e.portMin)
			// The reconcile has to wait for startup. If it didn't, it would
			// open the listener now; a dispatch slower than this window only
			// makes this check vacuous, and the next one still holds. Dial
			// rather than call Ports, which waits on the same lock.
			Consistently(func() error {
				c, err := dialForwarder(e.portMin)
				if err == nil {
					_ = c.Close()
				}
				return err
			}).WithTimeout(500*time.Millisecond).Should(HaveOccurred(), "the newer sync waits for startup")

			startup.Release()
			Eventually(e.forwarder.Table.Synced).Should(BeTrue())
			e.eventuallyRoutes(e.portMin, "new.ngrok.direct:80")
			// Applied after the reconcile, startup's empty snapshot would
			// close the listener with nothing left to reopen it.
			Consistently(e.forwarder.Ports).WithTimeout(time.Second).Should(Equal([]int32{e.portMin}), "startup's stale snapshot doesn't erase newer routes")
		})

		It("never dials for a port that is ambiguous from the start", func() {
			wireCR(createCR(e.ns, "tcp://a.ngrok.direct:5432"), "10.0.0.2", e.portMin)
			wireCR(createCR(e.ns, "tcp://b.ngrok.direct:5432"), "10.0.0.3", e.portMin)

			e.start()

			Eventually(e.forwarder.Ports).Should(Equal([]int32{e.portMin}))
			for range 3 {
				expectRefused(Default, e.portMin)
			}
			Expect(e.dialer.Targets()).To(BeEmpty())
			Expect(e.ambiguousLogs.Load()).To(Equal(int32(1)))
		})

		It("keeps the manager running when a port can't be bound, and binds it once it frees", func() {
			blocker := holdPort(e.portMin)
			wireCR(createCR(e.ns, "tcp://held.ngrok.direct:6379"), "10.0.0.2", e.portMin)

			e.start()

			Eventually(func(g Gomega) {
				g.Expect(e.forwarder.Table.Synced()).To(BeTrue())
				g.Expect(e.forwarder.BindErr()).To(MatchError(And(
					ContainSubstring(fmt.Sprint(e.portMin)),
					ContainSubstring("held.ngrok.direct"),
				)))
			}).Should(Succeed())
			Expect(e.forwarder.Ports()).To(BeEmpty())
			Consistently(e.exited).WithTimeout(time.Second).ShouldNot(BeClosed(), "manager stopped on a bind failure")

			Expect(blocker.Close()).To(Succeed())

			Eventually(e.forwarder.BindErr).WithTimeout(30 * time.Second).ShouldNot(HaveOccurred())
			Expect(e.forwarder.Ports()).To(Equal([]int32{e.portMin}))
			e.eventuallyRoutes(e.portMin, "held.ngrok.direct:6379")
		})
	})

	It("binds the ports it can when one is held", func() {
		holdPort(e.portMin)
		wireCR(createCR(e.ns, "tcp://held.ngrok.direct:6379"), "10.0.0.2", e.portMin)
		wireCR(createCR(e.ns, "tcp://free.ngrok.direct:6379"), "10.0.0.3", e.portMin+1)

		Eventually(e.forwarder.BindErr).Should(HaveOccurred())
		Eventually(e.forwarder.Ports).Should(Equal([]int32{e.portMin + 1}))
		e.eventuallyRoutes(e.portMin+1, "free.ngrok.direct:6379")
	})

	It("clears a bind failure once no endpoint wants the port", func() {
		holdPort(e.portMin)
		wireCR(createCR(e.ns, "tcp://held.ngrok.direct:6379"), "10.0.0.2", e.portMin)
		Eventually(e.forwarder.BindErr).Should(HaveOccurred())

		deleteCR(e.ns, "tcp://held.ngrok.direct:6379")

		Eventually(e.forwarder.BindErr).ShouldNot(HaveOccurred())
	})

	It("closes every listener on Close", func() {
		wireCR(createCR(e.ns, "tcp://a.ngrok.direct:1"), "10.0.0.2", e.portMin)
		wireCR(createCR(e.ns, "tcp://b.ngrok.direct:2"), "10.0.0.3", e.portMin+1)
		Eventually(e.forwarder.Ports).Should(HaveLen(2))

		e.forwarder.Close()

		Expect(e.forwarder.Ports()).To(BeEmpty())
		eventuallyNotListening(e.portMin)
		eventuallyNotListening(e.portMin + 1)
	})
})
