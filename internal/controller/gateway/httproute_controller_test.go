package gateway

import (
	"time"

	testutils "github.com/ngrok/ngrok-operator/internal/testutils"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"
)

var _ = Describe("HTTPRoute controller", Ordered, func() {
	const (
		ManagedControllerName   = ControllerName
		UnmanagedControllerName = "k8s.io/some-other-controller"

		timeout  = 10 * time.Second
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
				Expect(client.IgnoreNotFound(k8sClient.Delete(ctx, gw))).To(Succeed())
				Expect(client.IgnoreNotFound(k8sClient.Delete(ctx, route))).To(Succeed())
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
			})

			When("the gateway does not exist", func() {
				BeforeEach(func(ctx SpecContext) {
					DeleteGatewayAndWaitForDeletion(ctx, gw, timeout, interval)
				})

				It("Should not write any status since the Gateway no longer exists and ownership cannot be confirmed", func(ctx SpecContext) {
					// Per the Gateway API spec, a controller must NOT write status entries for
					// parentRefs it cannot confirm ownership of. When the Gateway doesn't exist,
					// ownership cannot be determined, so the ngrok controller should not write
					// any status entries (including NoMatchingParent).
					Consistently(func(g Gomega) {
						obj := &gatewayv1.HTTPRoute{}
						g.Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(route), obj)).To(Succeed())

						g.Expect(obj.Status.Parents).To(BeEmpty())
						g.Expect(obj.Finalizers).NotTo(ContainElement("k8s.ngrok.com/finalizer"))
					}, duration, interval).Should(Succeed())
				})
			})
		})

		When("the parent ref is an unsupported type", func() {
			var (
				service *v1.Service
			)

			BeforeEach(func(ctx SpecContext) {
				service = testutils.NewTestServiceV1(testutils.RandomName("svc"), "default")
				Expect(k8sClient.Create(ctx, service)).To(Succeed())

				route = &gatewayv1.HTTPRoute{
					ObjectMeta: metav1.ObjectMeta{
						Name:      testutils.RandomName("httproute"),
						Namespace: "default",
					},
					Spec: gatewayv1.HTTPRouteSpec{
						CommonRouteSpec: gatewayv1.CommonRouteSpec{
							ParentRefs: []gatewayv1.ParentReference{
								{
									Kind:      ptr.To(gatewayv1.Kind("Service")),
									Group:     ptr.To(gatewayv1.Group("")),
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

			It("Should not write any status or finalizer since the route does not reference an ngrok Gateway", func(ctx SpecContext) {
				Consistently(func(g Gomega) {
					obj := &gatewayv1.HTTPRoute{}
					g.Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(route), obj)).To(Succeed())

					g.Expect(obj.Status.Parents).To(BeEmpty())
					g.Expect(obj.Finalizers).NotTo(ContainElement("k8s.ngrok.com/finalizer"))
				}, duration, interval).Should(Succeed())
			})
		})
	})

	When("the gateway class is NOT managed by us", Ordered, func() {
		var (
			unmanagedGatewayClass *gatewayv1.GatewayClass
		)

		BeforeAll(func(ctx SpecContext) {
			unmanagedGatewayClass = testutils.NewGatewayClass(false)
			Expect(k8sClient.Create(ctx, unmanagedGatewayClass)).To(Succeed())
		})

		AfterAll(func(ctx SpecContext) {
			Expect(client.IgnoreNotFound(k8sClient.Delete(ctx, unmanagedGatewayClass))).To(Succeed())
		})

		When("an HTTPRoute references a Gateway with an unmanaged GatewayClass", func() {
			var (
				unmanagedGateway *gatewayv1.Gateway
			)

			BeforeEach(func(ctx SpecContext) {
				unmanagedGateway = newGateway(unmanagedGatewayClass)
				Expect(k8sClient.Create(ctx, unmanagedGateway)).To(Succeed())

				route = &gatewayv1.HTTPRoute{
					ObjectMeta: metav1.ObjectMeta{
						Name:      testutils.RandomName("httproute"),
						Namespace: "default",
					},
					Spec: gatewayv1.HTTPRouteSpec{
						CommonRouteSpec: gatewayv1.CommonRouteSpec{
							ParentRefs: []gatewayv1.ParentReference{
								{
									Name: gatewayv1.ObjectName(unmanagedGateway.Name),
								},
							},
						},
					},
				}
				Expect(k8sClient.Create(ctx, route)).To(Succeed())
			})

			AfterEach(func(ctx SpecContext) {
				Expect(client.IgnoreNotFound(k8sClient.Delete(ctx, unmanagedGateway))).To(Succeed())
				Expect(client.IgnoreNotFound(k8sClient.Delete(ctx, route))).To(Succeed())
			})

			It("Should not add a finalizer or write status to the HTTPRoute", func(ctx SpecContext) {
				Consistently(func(g Gomega) {
					obj := &gatewayv1.HTTPRoute{}
					g.Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(route), obj)).To(Succeed())

					// The ngrok controller must not add its finalizer to routes it does not own
					g.Expect(obj.Finalizers).NotTo(ContainElement("k8s.ngrok.com/finalizer"))

					// The ngrok controller must not write any .status.parents entries for routes it does not own
					for _, parent := range obj.Status.Parents {
						g.Expect(parent.ControllerName).NotTo(Equal(UnmanagedControllerName))
					}
				}, duration, interval).Should(Succeed())
			})
		})
	})
})
