package privateendpoints

import (
	"time"

	"github.com/ngrok/ngrok-api-go/v9"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"

	"github.com/ngrok/ngrok-operator/internal/mocks/nmockapi"
)

// Runs the poller, the PrivateEndpoint controller and the forwarder together:
// an endpoint showing up in the ngrok API becomes a Service in front of a
// forwarder port that private-dials it, and goes away the same way.
var _ = Describe("Private endpoints end to end", func() {
	var (
		e      *peEnv
		eps    *nmockapi.EndpointsClient
		poller *PrivateEndpointPoller
	)

	BeforeEach(func() {
		e = newPEEnv(peEnvOpts{controller: true, forwarder: true})
		eps = nmockapi.NewEndpointsClient()
		poller = &PrivateEndpointPoller{Client: k8sClient, Log: GinkgoLogr, Namespace: e.ns, Endpoints: eps, Interval: time.Hour}
	})

	It("serves an API endpoint through its Service's forwarder port until it is removed", func() {
		https, err := eps.Create(ctx, &ngrok.EndpointCreate{URL: "https://flow.ngrok.direct"})
		Expect(err).NotTo(HaveOccurred())
		tcp, err := eps.Create(ctx, &ngrok.EndpointCreate{URL: "tcp://flow.ngrok.direct:5432"})
		Expect(err).NotTo(HaveOccurred())

		Expect(poller.Sync(ctx)).To(Succeed())

		a := eventuallyReady(e.ns, "https://flow.ngrok.direct")
		b := eventuallyReady(e.ns, "tcp://flow.ngrok.direct:5432")
		eventuallyService(e.ns, "flow.ngrok.direct", func(g Gomega, svc *corev1.Service) {
			g.Expect(servicePorts(svc)).To(Equal(map[int32]int32{443: a.Status.ForwarderPort, 5432: b.Status.ForwarderPort}))
		})
		Eventually(e.forwarder.Ports).Should(ConsistOf(a.Status.ForwarderPort, b.Status.ForwarderPort))
		e.eventuallyRoutes(a.Status.ForwarderPort, "flow.ngrok.direct:443")
		e.eventuallyRoutes(b.Status.ForwarderPort, "flow.ngrok.direct:5432")

		By("removing the tcp endpoint from the API")
		Expect(eps.Delete(ctx, tcp.ID)).To(Succeed())
		Expect(poller.Sync(ctx)).To(Succeed())

		eventuallyService(e.ns, "flow.ngrok.direct", func(g Gomega, svc *corev1.Service) {
			g.Expect(servicePorts(svc)).To(Equal(map[int32]int32{443: a.Status.ForwarderPort}))
		})
		Eventually(e.forwarder.Ports).Should(Equal([]int32{a.Status.ForwarderPort}))
		eventuallyNotListening(b.Status.ForwarderPort)

		By("removing the last endpoint on the hostname")
		Expect(eps.Delete(ctx, https.ID)).To(Succeed())
		Expect(poller.Sync(ctx)).To(Succeed())

		eventuallyNoService(e.ns, "flow.ngrok.direct")
		Eventually(e.forwarder.Ports).Should(BeEmpty())
		eventuallyNotListening(a.Status.ForwarderPort)
	})
})
