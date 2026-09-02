package gateway

import (
	"fmt"
	"time"

	"github.com/ngrok/ngrok-api-go/v9"
	ingressv1alpha1 "github.com/ngrok/ngrok-operator/api/ingress/v1alpha1"
	"github.com/ngrok/ngrok-operator/internal/deprecation"
	testutils "github.com/ngrok/ngrok-operator/internal/testutils"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
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
							Hostname: new(gatewayv1.Hostname(domain)),
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

			When("a wildcard parent for the hostname is already reserved", func() {
				var wildcard string

				BeforeEach(func(ctx SpecContext) {
					zone := fmt.Sprintf("%s.ngrok.io", rand.String(10))
					wildcard = "*." + zone
					domain = "a." + zone

					// Pre-reserve only the wildcard, as a customer who set one up
					// in the ngrok dashboard would have.
					_, err := domainClient.Create(ctx, &ngrok.ReservedDomainCreate{Domain: wildcard})
					Expect(err).ToNot(HaveOccurred())

					gw.Spec.Listeners = []gatewayv1.Listener{
						{
							Name:     gatewayv1.SectionName(testutils.RandomName("listener")),
							Hostname: new(gatewayv1.Hostname(domain)),
							Port:     443,
							Protocol: gatewayv1.HTTPSProtocolType,
						},
					}
				})

				It("Should not reserve the domain, and still assign the gateway address", func(ctx SpecContext) {
					By("Checking the Domain CR is marked as covered rather than reserved")
					Eventually(func(g Gomega) {
						found := &ingressv1alpha1.Domain{}
						g.Expect(k8sClient.Get(ctx, client.ObjectKey{
							Name:      ingressv1alpha1.HyphenatedDomainNameFromURL(domain),
							Namespace: gw.Namespace,
						}, found)).To(Succeed())

						g.Expect(found.Status.CoveredByWildcardDomain).To(Equal(wildcard))
						g.Expect(found.Status.ID).To(BeEmpty())
						g.Expect(found.Status.Domain).To(Equal(domain))
					}, timeout, interval).Should(Succeed())

					By("Checking the hostname itself was never reserved in ngrok")
					// Scoped to this spec's own zone on purpose: Domain CRs are not
					// deleted between gateway specs, and domainClient.Reset() clears
					// the mock each spec, so leftover CRs from sibling specs
					// re-reserve themselves here. A global count would be flaky.
					reserved := map[string]bool{}
					iter := domainClient.List(&ngrok.FilteredPaging{})
					for iter.Next(ctx) {
						reserved[iter.Item().Domain] = true
					}
					Expect(iter.Err()).To(BeNil())
					Expect(reserved).To(HaveKey(wildcard), "the seeded wildcard should still be reserved")
					Expect(reserved).ToNot(HaveKey(domain), "the covered hostname must not be reserved")

					By("Checking the gateway still advertises the listener hostname")
					Eventually(func(g Gomega) {
						obj := &gatewayv1.Gateway{}
						g.Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(gw), obj)).To(Succeed())
						g.Expect(obj.Status.Addresses).To(HaveLen(1))
						g.Expect(obj.Status.Addresses[0].Value).To(Equal(domain))
					}, timeout, interval).Should(Succeed())
				})
			})

			When("the hostname is a custom domain", func() {
				BeforeEach(func() {
					domain = fmt.Sprintf("%s.custom.domain", rand.String(10))
					gw.Spec.Listeners = []gatewayv1.Listener{
						{
							Name:     gatewayv1.SectionName(testutils.RandomName("listener")),
							Hostname: new(gatewayv1.Hostname(domain)),
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

		When("the gateway has a HTTP listener with no hostname", func() {
			BeforeEach(func() {
				gw.Spec.Listeners = []gatewayv1.Listener{
					{
						Name:     gatewayv1.SectionName(testutils.RandomName("listener")),
						Port:     80,
						Protocol: gatewayv1.HTTPProtocolType,
					},
				}
			})

			It("Should not accept the gateway", func(ctx SpecContext) {
				ExpectGatewayNotAccepted(ctx, gw).WithTimeout(timeout).WithPolling(interval).Should(Succeed())
			})

			It("Should set the listener to not accepted and have a reason of HostnameRequired", func(ctx SpecContext) {
				ExpectListenerStatus(
					ctx,
					gw,
					gw.Spec.Listeners[0].Name,
					gatewayv1.ListenerConditionAccepted,
					metav1.ConditionFalse,
					ListenerReasonHostnameRequired,
				).WithTimeout(timeout).WithPolling(interval).Should(Succeed())
			})

			It("Should set the listener programmed condition to invalid", func(ctx SpecContext) {
				ExpectListenerStatus(
					ctx,
					gw,
					gw.Spec.Listeners[0].Name,
					gatewayv1.ListenerConditionProgrammed,
					metav1.ConditionFalse,
					gatewayv1.ListenerReasonInvalid,
				).WithTimeout(timeout).WithPolling(interval).Should(Succeed())
			})
		})

		When("the gateway has a HTTPS listener with no hostname", func() {
			BeforeEach(func() {
				gw.Spec.Listeners = []gatewayv1.Listener{
					{
						Name:     gatewayv1.SectionName(testutils.RandomName("listener")),
						Port:     443,
						Protocol: gatewayv1.HTTPSProtocolType,
					},
				}
			})

			It("Should not accept the gateway", func(ctx SpecContext) {
				ExpectGatewayNotAccepted(ctx, gw).WithTimeout(timeout).WithPolling(interval).Should(Succeed())
			})

			It("Should set the listener to not accepted and have a reason of HostnameRequired", func(ctx SpecContext) {
				ExpectListenerStatus(
					ctx,
					gw,
					gw.Spec.Listeners[0].Name,
					gatewayv1.ListenerConditionAccepted,
					metav1.ConditionFalse,
					ListenerReasonHostnameRequired,
				).WithTimeout(timeout).WithPolling(interval).Should(Succeed())
			})

			It("Should set the listener programmed condition to invalid", func(ctx SpecContext) {
				ExpectListenerStatus(
					ctx,
					gw,
					gw.Spec.Listeners[0].Name,
					gatewayv1.ListenerConditionProgrammed,
					metav1.ConditionFalse,
					gatewayv1.ListenerReasonInvalid,
				).WithTimeout(timeout).WithPolling(interval).Should(Succeed())
			})
		})

		When("the gateway has a HTTP listener with port other than 80", func() {
			BeforeEach(func() {
				gw.Spec.Listeners = []gatewayv1.Listener{
					{
						Name:     gatewayv1.SectionName(testutils.RandomName("listener")),
						Port:     8080,
						Hostname: ptr.To(gatewayv1.Hostname("example.com")),
						Protocol: gatewayv1.HTTPProtocolType,
					},
				}
			})

			It("Should not accept the gateway", func(ctx SpecContext) {
				ExpectGatewayNotAccepted(ctx, gw).WithTimeout(timeout).WithPolling(interval).Should(Succeed())
			})

			It("Should set the listener to not accepted and have a reason of PortUnavailable", func(ctx SpecContext) {
				ExpectListenerStatus(
					ctx,
					gw,
					gw.Spec.Listeners[0].Name,
					gatewayv1.ListenerConditionAccepted,
					metav1.ConditionFalse,
					gatewayv1.ListenerReasonPortUnavailable,
				).WithTimeout(timeout).WithPolling(interval).Should(Succeed())
			})

			It("Should set the listener programmed condition to invalid", func(ctx SpecContext) {
				ExpectListenerStatus(
					ctx,
					gw,
					gw.Spec.Listeners[0].Name,
					gatewayv1.ListenerConditionProgrammed,
					metav1.ConditionFalse,
					gatewayv1.ListenerReasonInvalid,
				).WithTimeout(timeout).WithPolling(interval).Should(Succeed())
			})
		})

		When("the gateway has a HTTPS listener with port other than 443", func() {
			BeforeEach(func() {
				gw.Spec.Listeners = []gatewayv1.Listener{
					{
						Name:     gatewayv1.SectionName(testutils.RandomName("listener")),
						Port:     8443,
						Hostname: ptr.To(gatewayv1.Hostname("example.com")),
						Protocol: gatewayv1.HTTPProtocolType,
					},
				}
			})

			It("Should not accept the gateway", func(ctx SpecContext) {
				ExpectGatewayNotAccepted(ctx, gw).WithTimeout(timeout).WithPolling(interval).Should(Succeed())
			})

			It("Should set the listener to not accepted and have a reason of PortUnavailable", func(ctx SpecContext) {
				ExpectListenerStatus(
					ctx,
					gw,
					gw.Spec.Listeners[0].Name,
					gatewayv1.ListenerConditionAccepted,
					metav1.ConditionFalse,
					gatewayv1.ListenerReasonPortUnavailable,
				).WithTimeout(timeout).WithPolling(interval).Should(Succeed())
			})

			It("Should set the listener programmed condition to invalid", func(ctx SpecContext) {
				ExpectListenerStatus(
					ctx,
					gw,
					gw.Spec.Listeners[0].Name,
					gatewayv1.ListenerConditionProgrammed,
					metav1.ConditionFalse,
					gatewayv1.ListenerReasonInvalid,
				).WithTimeout(timeout).WithPolling(interval).Should(Succeed())
			})
		})

		When("the gateway has a legacy-prefixed annotation", func() {
			BeforeEach(func() {
				gw.Annotations = map[string]string{"k8s.ngrok.com/pooling-enabled": "true"}
			})

			It("emits a LegacyAnnotation warning event", func(ctx SpecContext) {
				Eventually(func(g Gomega) {
					events := &corev1.EventList{}
					g.Expect(k8sClient.List(ctx, events, client.InNamespace(gw.Namespace))).To(Succeed())
					g.Expect(events.Items).To(ContainElement(SatisfyAll(
						HaveField("Reason", deprecation.ReasonLegacyAnnotation),
						HaveField("InvolvedObject.Name", gw.Name),
					)))
				}, timeout, interval).Should(Succeed())
			})
		})

		When("the gateway has a UDP listener", func() {
			BeforeEach(func() {
				gw.Spec.Listeners = []gatewayv1.Listener{
					{
						Name:     gatewayv1.SectionName(testutils.RandomName("listener")),
						Port:     53,
						Protocol: gatewayv1.UDPProtocolType,
					},
				}
			})

			It("Should set the listener to not accepted and have a reason of UnsupportedProtocol", func(ctx SpecContext) {
				ExpectListenerStatus(
					ctx,
					gw,
					gw.Spec.Listeners[0].Name,
					gatewayv1.ListenerConditionAccepted,
					metav1.ConditionFalse,
					gatewayv1.ListenerReasonUnsupportedProtocol,
				).WithTimeout(timeout).WithPolling(interval).Should(Succeed())
			})

			It("Should set the listener programmed condition to invalid", func(ctx SpecContext) {
				ExpectListenerStatus(
					ctx,
					gw,
					gw.Spec.Listeners[0].Name,
					gatewayv1.ListenerConditionProgrammed,
					metav1.ConditionFalse,
					gatewayv1.ListenerReasonInvalid,
				).WithTimeout(timeout).WithPolling(interval).Should(Succeed())
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

		When("the gateway has a legacy-prefixed annotation", func() {
			// Placement pin: this proves ScanAnnotations sits AFTER the
			// ShouldHandleGatewayClass filter, since gw's class is not ours.
			It("does not emit a LegacyAnnotation warning event", func(ctx SpecContext) {
				Eventually(func() error {
					fetched := &gatewayv1.Gateway{}
					if err := k8sClient.Get(ctx, client.ObjectKeyFromObject(gw), fetched); err != nil {
						return err
					}
					fetched.Annotations = map[string]string{"k8s.ngrok.com/pooling-enabled": "true"}
					return k8sClient.Update(ctx, fetched)
				}, timeout, interval).Should(Succeed())

				Consistently(func(g Gomega) {
					events := &corev1.EventList{}
					g.Expect(k8sClient.List(ctx, events, client.InNamespace(gw.Namespace))).To(Succeed())
					g.Expect(events.Items).NotTo(ContainElement(SatisfyAll(
						HaveField("Reason", deprecation.ReasonLegacyAnnotation),
						HaveField("InvolvedObject.Name", gw.Name),
					)))
				}, "2s", interval).Should(Succeed())
			})
		})
	})
})

func newGateway(gwc *gatewayv1.GatewayClass) *gatewayv1.Gateway {
	gw := testutils.NewGateway(testutils.RandomName("gateway"), "default")
	gw.Spec.GatewayClassName = gatewayv1.ObjectName(gwc.Name)
	return &gw
}

var _ = Describe("secretReferencedByGateway", func() {
	It("should return true when a TLS secret is referenced by a gateway listener in the same namespace", func(ctx SpecContext) {
		namespace := "test-ns-" + rand.String(5)

		ns := &corev1.Namespace{
			Name: namespace,
		}
		Expect(k8sClient.Create(ctx, ns)).To(Succeed())

		gatewayClass := testutils.NewGatewayClass(true)
		Expect(k8sClient.Create(ctx, gatewayClass)).To(Succeed())

		secretName := "my-tls-secret"
		secretNs := gatewayv1.Namespace(namespace)
		gw := testutils.NewGateway("test-gateway", namespace)
		gw.Spec.GatewayClassName = gatewayv1.ObjectName(gatewayClass.Name)
		gw.Spec.Listeners = []gatewayv1.Listener{
			{
				Name:     "https",
				Hostname: ptr.To(gatewayv1.Hostname("example.com")),
				Port:     443,
				Protocol: gatewayv1.HTTPSProtocolType,
				TLS: &gatewayv1.ListenerTLSConfig{
					CertificateRefs: []gatewayv1.SecretObjectReference{
						{
							Name:      gatewayv1.ObjectName(secretName),
							Namespace: &secretNs,
						},
					},
				},
			},
		}
		Expect(k8sClient.Create(ctx, &gw)).To(Succeed())

		secret := &corev1.Secret{
			Name:      secretName,
			Namespace: namespace,
			Type:      corev1.SecretTypeTLS,
			Data: map[string][]byte{
				"tls.crt": []byte("cert"),
				"tls.key": []byte("key"),
			},
		}
		Expect(k8sClient.Create(ctx, secret)).To(Succeed())

		result := secretReferencedByGateway(secret, k8sClient)
		Expect(result).To(BeTrue(), "secretReferencedByGateway should return true when secret is referenced by gateway listener")

		Expect(k8sClient.Delete(ctx, &gw)).To(Succeed())
		Expect(k8sClient.Delete(ctx, secret)).To(Succeed())
		Expect(k8sClient.Delete(ctx, gatewayClass)).To(Succeed())
		Expect(k8sClient.Delete(ctx, ns)).To(Succeed())
	})
})
