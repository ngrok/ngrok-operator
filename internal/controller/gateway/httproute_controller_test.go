package gateway

import (
	"time"

	"github.com/ngrok/ngrok-operator/internal/controller"
	"github.com/ngrok/ngrok-operator/internal/errors"
	testutils "github.com/ngrok/ngrok-operator/internal/testutils"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"
)

var _ = Describe("HTTPRoute controller", Ordered, func() {
	const (
		ManagedControllerName   = ControllerName
		UnmanagedControllerName = "k8s.io/some-other-controller"

		timeout  = 40 * time.Second
		duration = 10 * time.Second
		interval = 250 * time.Millisecond
	)

	var (
		gatewayClass *gatewayv1.GatewayClass
		route        *gatewayv1.HTTPRoute
	)

	When("the gateway class is managed by us", Ordered, func() {
		BeforeAll(func(ctx SpecContext) {
			gatewayClass = testutils.NewGatewayClass(true)
			CreateGatewayClassAndWaitForAcceptance(ctx, gatewayClass, timeout, interval)
		})

		AfterAll(func(ctx SpecContext) {
			DeleteAllGatewayClasses(ctx, timeout, interval)
		})

		When("the parent ref is a gateway", func() {
			var (
				gw *gatewayv1.Gateway
			)

			BeforeEach(func(ctx SpecContext) {
				gw = newGateway(gatewayClass)
				CreateGatewayAndWaitForAcceptance(ctx, gw, timeout, interval)

				route = &gatewayv1.HTTPRoute{
					ObjectMeta: metav1.ObjectMeta{
						Name:      testutils.RandomName("httproute"),
						Namespace: "default",
					},
					Spec: gatewayv1.HTTPRouteSpec{
						CommonRouteSpec: gatewayv1.CommonRouteSpec{
							ParentRefs: []gatewayv1.ParentReference{
								{
									Name: gatewayv1.ObjectName(gw.Name),
								},
							},
						},
						Rules: []gatewayv1.HTTPRouteRule{
							{
								Matches: []gatewayv1.HTTPRouteMatch{
									{
										Path: &gatewayv1.HTTPPathMatch{},
									},
								},
							},
						},
					},
				}
			})

			AfterEach(func(ctx SpecContext) {
				Expect(client.IgnoreNotFound(k8sClient.Delete(ctx, route))).To(Succeed())
				Expect(client.IgnoreNotFound(k8sClient.Delete(ctx, gw))).To(Succeed())
			})

			JustBeforeEach(func(ctx SpecContext) {
				Expect(k8sClient.Create(ctx, route)).To(Succeed())
			})

			When("the gateway exists", func() {
				It("Should accept the HTTPRoute", func(ctx SpecContext) {
					Eventually(func(g Gomega) {
						obj := &gatewayv1.HTTPRoute{}
						g.Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(route), obj)).To(Succeed())

						g.Expect(obj.Status.Parents).To(HaveLen(1))
						parent := obj.Status.Parents[0]
						g.Expect(parent.ParentRef.Name).To(Equal(gatewayv1.ObjectName(gw.Name)))
						g.Expect(parent.ControllerName).To(Equal(ManagedControllerName))

						g.Expect(parent.Conditions).To(HaveLen(1))
						cond := meta.FindStatusCondition(parent.Conditions, string(gatewayv1.RouteConditionAccepted))
						g.Expect(cond).ToNot(BeNil())
						g.Expect(cond.Status).To(Equal(metav1.ConditionTrue))
						g.Expect(cond.Reason).To(Equal(string(gatewayv1.RouteReasonAccepted)))

					}, timeout, interval).Should(Succeed())
				})

				When("the httproute is annotated for cleanup", func() {
					JustBeforeEach(func(ctx SpecContext) {
						By("Waiting for the route to be added to the store")
						Eventually(func(g Gomega) {
							store := driver.GetStore()
							fetched, err := store.GetHTTPRoute(route.GetName(), route.GetNamespace())
							g.Expect(err).To(BeNil())
							g.Expect(fetched).NotTo(BeNil())
						}, timeout, interval).Should(Succeed())

						By("Adding the cleanup annotation to the httproute")
						kginkgo.ExpectAddAnnotations(ctx, route, map[string]string{
							controller.CleanupAnnotation: "true",
						})
					})

					It("Should remove the finalizer from the httproute", func(ctx SpecContext) {
						kginkgo.ExpectFinalizerToBeRemoved(ctx, route, controller.FinalizerName)
					})

					It("Should remove the httproute from the store", func(_ SpecContext) {
						Eventually(func(g Gomega) {
							By("fetching the httproute from the store")
							store := driver.GetStore()
							fetched, err := store.GetHTTPRoute(route.GetName(), route.GetNamespace())
							GinkgoLogr.Info("fetched httproute from store", "httproute", fetched, "err", err)

							By("verifying the httproute is not found in the store")
							g.Expect(fetched).To(BeNil())
							By("expecting an error indicating the httproute is not found")
							g.Expect(err).NotTo(BeNil())
							g.Expect(errors.IsErrorNotFound(err)).To(BeTrue())
						}, timeout, interval).Should(Succeed())
					})
				})
			})

			When("the gateway does not exist", func() {
				BeforeEach(func(ctx SpecContext) {
					DeleteGatewayAndWaitForDeletion(ctx, gw, timeout, interval)
				})

				It("Should not accept the HTTPRoute", func(ctx SpecContext) {
					Eventually(func(g Gomega) {
						obj := &gatewayv1.HTTPRoute{}
						g.Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(route), obj)).To(Succeed())

						g.Expect(obj.Status.Parents).To(HaveLen(1))

						parent := obj.Status.Parents[0]
						g.Expect(parent.ParentRef.Name).To(Equal(gatewayv1.ObjectName(gw.Name)))

						g.Expect(parent.Conditions).To(HaveLen(1))
						cond := meta.FindStatusCondition(parent.Conditions, string(gatewayv1.RouteConditionAccepted))
						g.Expect(cond).ToNot(BeNil())
						g.Expect(cond.Status).To(Equal(metav1.ConditionFalse))
						g.Expect(cond.Reason).To(Equal(string(gatewayv1.RouteReasonNoMatchingParent)))
					}, timeout, interval).Should(Succeed())
				})
			})
		})

		When("the parent ref is an unsupported type", func() {
			var (
				service *v1.Service
				svcGVK  schema.GroupVersionKind
			)

			BeforeEach(func(ctx SpecContext) {
				var err error
				service = testutils.NewTestServiceV1(testutils.RandomName("svc"), "default")
				Expect(k8sClient.Create(ctx, service)).To(Succeed())
				svcGVK, err = k8sClient.GroupVersionKindFor(service)
				Expect(err).ToNot(HaveOccurred())

				route = &gatewayv1.HTTPRoute{
					ObjectMeta: metav1.ObjectMeta{
						Name:      testutils.RandomName("httproute"),
						Namespace: "default",
					},
					Spec: gatewayv1.HTTPRouteSpec{
						CommonRouteSpec: gatewayv1.CommonRouteSpec{
							ParentRefs: []gatewayv1.ParentReference{
								{
									Kind:      ptr.To(gatewayv1.Kind(svcGVK.Kind)),
									Group:     ptr.To(gatewayv1.Group(svcGVK.Group)),
									Namespace: ptr.To(gatewayv1.Namespace(service.Namespace)),
									Name:      gatewayv1.ObjectName(service.Name),
								},
							},
						},
					},
				}
			})

			JustBeforeEach(func(ctx SpecContext) {
				Expect(k8sClient.Create(ctx, route)).To(Succeed())
			})

			It("Should have a parentRef condition with a reason of Unsupported", func(ctx SpecContext) {
				Eventually(func(g Gomega) {
					obj := &gatewayv1.HTTPRoute{}
					g.Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(route), obj)).To(Succeed())

					g.Expect(obj.Status.Parents).To(HaveLen(1))
					parent := obj.Status.Parents[0]
					g.Expect(parent.ParentRef.Name).To(Equal(gatewayv1.ObjectName(service.Name)))
					g.Expect(parent.ControllerName).To(Equal(ManagedControllerName))

					g.Expect(parent.Conditions).To(HaveLen(1))
					cond := meta.FindStatusCondition(parent.Conditions, string(gatewayv1.RouteConditionAccepted))
					g.Expect(cond).ToNot(BeNil())
					g.Expect(cond.Status).To(Equal(metav1.ConditionFalse))
					g.Expect(cond.Reason).To(Equal(string(gatewayv1.RouteReasonInvalidKind)))
				}, timeout, interval).Should(Succeed())
			})
		})
	})
})
