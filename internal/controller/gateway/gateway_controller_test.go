package gateway

import (
	"fmt"
	"time"

	testutils "github.com/ngrok/ngrok-operator/internal/testutils"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/rand"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"
)

var _ = Describe("Gateway controller", Ordered, func() {
	const (
		timeout  = 10 * time.Second
		duration = 10 * time.Second
		interval = 250 * time.Millisecond
	)

	var (
		gatewayClass *gatewayv1.GatewayClass
		gw           *gatewayv1.Gateway
	)

	When("the gateway's gateway class should be handled by us", func() {
		BeforeAll(func(ctx SpecContext) {
			gatewayClass = testutils.NewGatewayClass(true)
			CreateGatewayClassAndWaitForAcceptance(ctx, gatewayClass, timeout, interval)
		})

		AfterAll(func(ctx SpecContext) {
			DeleteAllGatewayClasses(ctx, timeout, interval)
		})

		BeforeEach(func() {
			gw = newGateway(gatewayClass)
		})

		// Create The gateway just before each test. This allows customization of
		// the gateway in the BeforeEach function for scoped test below.
		JustBeforeEach(func(ctx SpecContext) {
			Expect(k8sClient.Create(ctx, gw)).To(Succeed())
		})

		AfterEach(func(ctx SpecContext) {
			Expect(k8sClient.Delete(ctx, gw)).To(Succeed())
		})

		It("Should accept the gatewway", func(ctx SpecContext) {
			ExpectGatewayAccepted(ctx, gw, timeout, interval)
		})

		When("the gateway has a listener with a hostname", func() {
			var (
				domain string
			)

			When("the hostname is a ngrok managed domain", func() {
				BeforeEach(func() {
					domain = fmt.Sprintf("%s.ngrok.io", rand.String(10))
					gw.Spec.Listeners = []gatewayv1.Listener{
						{
							Name:     gatewayv1.SectionName(testutils.RandomName("listener")),
							Hostname: ptr.To(gatewayv1.Hostname(domain)),
							Port:     443,
							Protocol: gatewayv1.HTTPSProtocolType,
						},
					}
				})

				It("The domain should appear in the gateway addresses", func(ctx SpecContext) {
					Eventually(func(g Gomega) {
						obj := &gatewayv1.Gateway{}
						g.Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(gw), obj)).To(Succeed())

						By("Checking the gateway has an address")
						g.Expect(obj.Status.Addresses).To(HaveLen(1))
						g.Expect(obj.Status.Addresses[0].Type).To(Equal(gatewayv1.HostnameAddressType))
						g.Expect(obj.Status.Addresses[0].Value).To(Equal(domain))
					})
				})
			})

			When("the hostname is a custom domain", func() {
				BeforeEach(func() {
					domain = fmt.Sprintf("%s.custom.domain", rand.String(10))
					gw.Spec.Listeners = []gatewayv1.Listener{
						{
							Name:     gatewayv1.SectionName(testutils.RandomName("listener")),
							Hostname: ptr.To(gatewayv1.Hostname(domain)),
							Port:     443,
							Protocol: gatewayv1.HTTPSProtocolType,
						},
					}
				})

				It("The addresses should have a ngrok cname", func(ctx SpecContext) {
					Eventually(func(g Gomega) {
						obj := &gatewayv1.Gateway{}
						g.Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(gw), obj)).To(Succeed())

						By("Checking the gateway has an address")
						g.Expect(obj.Status.Addresses).To(HaveLen(1))
						g.Expect(obj.Status.Addresses[0].Type).To(Equal(gatewayv1.HostnameAddressType))
						g.Expect(obj.Status.Addresses[0].Value).To(MatchRegexp("\\.ngrok-cname\\.com$"))
					})
				})
			})
		})
	})

	When("The gateway's gateway class should not be handled by us", func() {
		BeforeAll(func(ctx SpecContext) {
			gatewayClass = testutils.NewGatewayClass(false)
			Expect(k8sClient.Create(ctx, gatewayClass)).To(Succeed())
		})

		AfterAll(func(ctx SpecContext) {
			DeleteAllGatewayClasses(ctx, timeout, interval)
		})

		BeforeEach(func(ctx SpecContext) {
			gw = newGateway(gatewayClass)
			Expect(k8sClient.Create(ctx, gw)).To(Succeed())
		})

		AfterEach(func(ctx SpecContext) {
			Expect(k8sClient.Delete(ctx, gw)).To(Succeed())
		})

		It("should not accept the gateway", func(ctx SpecContext) {
			Consistently(func(g Gomega) {
				obj := &gatewayv1.Gateway{}
				g.Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(gw), obj)).To(Succeed())
				By("Consistently not having an accepted condition with Status True")
				cond := meta.FindStatusCondition(obj.Status.Conditions, string(gatewayv1.GatewayConditionAccepted))
				g.Expect(cond.Status).NotTo(Equal(metav1.ConditionTrue))
			}, timeout, interval).Should(Succeed())
		})
	})
})

func newGateway(gwc *gatewayv1.GatewayClass) *gatewayv1.Gateway {
	gw := testutils.NewGateway(testutils.RandomName("gateway"), "default")
	gw.Spec.GatewayClassName = gatewayv1.ObjectName(gwc.Name)
	return &gw
}
