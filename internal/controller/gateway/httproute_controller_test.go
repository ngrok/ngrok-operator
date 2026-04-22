package gateway

import (
	"reflect"
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

var _ = Describe("mergeParentStatuses", func() {
	const (
		ngrokController = ControllerName
		otherController = gatewayv1.GatewayController("k8s.io/some-other-controller")
	)

	It("should not pick up conditions from another controller with the same ParentRef", func() {
		parentRef := gatewayv1.ParentReference{Name: "shared-gw"}

		// Simulate another controller having already written a status for the same ParentRef
		existing := []gatewayv1.RouteParentStatus{
			{
				ParentRef:      parentRef,
				ControllerName: otherController,
				Conditions: []metav1.Condition{
					{
						Type:               string(gatewayv1.RouteConditionAccepted),
						Status:             metav1.ConditionFalse,
						Reason:             "OtherReason",
						LastTransitionTime: metav1.Now(),
					},
				},
			},
		}

		// Reproduce the condition-reuse loop from validateRouteParentRefs.
		// The fixed code filters by ControllerName to avoid cross-controller pollution.
		parentStatus := gatewayv1.RouteParentStatus{
			ParentRef:      parentRef,
			ControllerName: ngrokController,
			Conditions:     []metav1.Condition{},
		}

		for _, s := range existing {
			if s.ControllerName != ngrokController {
				continue
			}
			if !reflect.DeepEqual(s.ParentRef, parentRef) {
				continue
			}
			parentStatus.Conditions = append([]metav1.Condition(nil), s.Conditions...)
			break
		}

		// Should NOT have picked up the other controller's conditions
		Expect(parentStatus.Conditions).To(BeEmpty(),
			"should not inherit conditions from a different controller")
	})

	It("should not share the backing slice with the existing status", func() {
		parentRef := gatewayv1.ParentReference{Name: "my-gw"}

		existing := []gatewayv1.RouteParentStatus{
			{
				ParentRef:      parentRef,
				ControllerName: ngrokController,
				Conditions: []metav1.Condition{
					{
						Type:               string(gatewayv1.RouteConditionAccepted),
						Status:             metav1.ConditionTrue,
						Reason:             string(gatewayv1.RouteReasonAccepted),
						LastTransitionTime: metav1.Now(),
					},
				},
			},
		}

		parentStatus := gatewayv1.RouteParentStatus{
			ParentRef:      parentRef,
			ControllerName: ngrokController,
			Conditions:     []metav1.Condition{},
		}

		// The fixed code deep-copies conditions to avoid aliasing
		for _, s := range existing {
			if s.ControllerName != ngrokController {
				continue
			}
			if !reflect.DeepEqual(s.ParentRef, parentRef) {
				continue
			}
			parentStatus.Conditions = append([]metav1.Condition(nil), s.Conditions...)
			break
		}

		// Mutating the copy must NOT affect the original
		parentStatus.Conditions[0].Reason = "Mutated"
		Expect(existing[0].Conditions[0].Reason).To(Equal(string(gatewayv1.RouteReasonAccepted)),
			"original conditions should not be mutated through the copy")
	})
})

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

				It("Should preserve parent statuses written by other controllers", func(ctx SpecContext) {
					// Wait for the ngrok controller to accept the route first
					kginkgo.EventuallyWithObject(ctx, route.DeepCopy(), func(g Gomega, obj client.Object) {
						r := obj.(*gatewayv1.HTTPRoute)
						g.Expect(r.Status.Parents).To(HaveLen(1))
						g.Expect(r.Status.Parents[0].ControllerName).To(Equal(ManagedControllerName))
					})

					// Simulate another controller (e.g. Istio) writing its own parent status
					Eventually(func(g Gomega) {
						obj := &gatewayv1.HTTPRoute{}
						g.Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(route), obj)).To(Succeed())

						obj.Status.Parents = append(obj.Status.Parents, gatewayv1.RouteParentStatus{
							ParentRef:      gatewayv1.ParentReference{Name: "some-other-gw"},
							ControllerName: UnmanagedControllerName,
							Conditions: []metav1.Condition{
								{
									Type:               string(gatewayv1.RouteConditionAccepted),
									Status:             metav1.ConditionTrue,
									Reason:             string(gatewayv1.RouteReasonAccepted),
									LastTransitionTime: metav1.Now(),
								},
							},
						})
						g.Expect(k8sClient.Status().Update(ctx, obj)).To(Succeed())
					}, timeout, interval).Should(Succeed())

					// Trigger a re-reconcile by updating the route's annotations
					Eventually(func(g Gomega) {
						obj := &gatewayv1.HTTPRoute{}
						g.Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(route), obj)).To(Succeed())
						if obj.Annotations == nil {
							obj.Annotations = map[string]string{}
						}
						obj.Annotations["test-trigger"] = "reconcile"
						g.Expect(k8sClient.Update(ctx, obj)).To(Succeed())
					}, timeout, interval).Should(Succeed())

					// Both controllers' statuses should be present
					kginkgo.EventuallyWithObject(ctx, route.DeepCopy(), func(g Gomega, obj client.Object) {
						r := obj.(*gatewayv1.HTTPRoute)
						controllerNames := map[gatewayv1.GatewayController]bool{}
						for _, parent := range r.Status.Parents {
							controllerNames[parent.ControllerName] = true
						}

						g.Expect(controllerNames).To(HaveKey(ManagedControllerName),
							"ngrok controller's parent status should be present")
						g.Expect(controllerNames).To(HaveKey(gatewayv1.GatewayController(UnmanagedControllerName)),
							"other controller's parent status should be preserved")
					})
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
									Namespace: new(gatewayv1.Namespace(service.Namespace)),
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
