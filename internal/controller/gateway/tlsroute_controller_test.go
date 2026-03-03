/*
MIT License

Copyright (c) 2025 ngrok, Inc.

Permission is hereby granted, free of charge, to any person obtaining a copy
of this software and associated documentation files (the "Software"), to deal
in the Software without restriction, including without limitation the rights
to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
copies of the Software, and to permit persons to whom the Software is
furnished to do so, subject to the following conditions:

The above copyright notice and this permission notice shall be included in all
copies or substantial portions of the Software.

THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
SOFTWARE.
*/

package gateway

import (
	"time"

	testutils "github.com/ngrok/ngrok-operator/internal/testutils"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"
	gatewayv1alpha2 "sigs.k8s.io/gateway-api/apis/v1alpha2"
)

var _ = Describe("TLSRoute controller", Ordered, func() {
	const (
		timeout  = 10 * time.Second
		duration = 10 * time.Second
		interval = 250 * time.Millisecond
	)

	var (
		gatewayClass *gatewayv1.GatewayClass
		route        *gatewayv1alpha2.TLSRoute
	)

	When("the gateway class is managed by us", Ordered, func() {
		BeforeAll(func(ctx SpecContext) {
			gatewayClass = testutils.NewGatewayClass(true)
			CreateGatewayClassAndWaitForAcceptance(ctx, gatewayClass, timeout, interval)
		})

		AfterAll(func(ctx SpecContext) {
			DeleteAllGatewayClasses(ctx, timeout, interval)
		})

		When("the parent ref is an ngrok-managed gateway", func() {
			var gw *gatewayv1.Gateway

			BeforeEach(func(ctx SpecContext) {
				gw = newGateway(gatewayClass)
				CreateGatewayAndWaitForAcceptance(ctx, gw, timeout, interval)

				route = &gatewayv1alpha2.TLSRoute{
					ObjectMeta: metav1.ObjectMeta{
						Name:      testutils.RandomName("tlsroute"),
						Namespace: "default",
					},
					Spec: gatewayv1alpha2.TLSRouteSpec{
						CommonRouteSpec: gatewayv1.CommonRouteSpec{
							ParentRefs: []gatewayv1.ParentReference{{
								Name: gatewayv1.ObjectName(gw.Name),
							}},
						},
						Rules: []gatewayv1alpha2.TLSRouteRule{{
							BackendRefs: []gatewayv1alpha2.BackendRef{{
								BackendObjectReference: gatewayv1.BackendObjectReference{
									Name: gatewayv1.ObjectName("example-svc"),
									Port: (*gatewayv1.PortNumber)(ptr.To[int32](8080)),
								},
							}},
						}},
					},
				}
				Expect(k8sClient.Create(ctx, route)).To(Succeed())
			})

			AfterEach(func(ctx SpecContext) {
				Expect(client.IgnoreNotFound(k8sClient.Delete(ctx, gw))).To(Succeed())
				Expect(client.IgnoreNotFound(k8sClient.Delete(ctx, route))).To(Succeed())
			})

			It("Should add the TLSRoute to the store and finalizer to the route", func(ctx SpecContext) {
				Eventually(func(g Gomega) {
					obj := &gatewayv1alpha2.TLSRoute{}
					g.Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(route), obj)).To(Succeed())
					g.Expect(obj.Finalizers).To(ContainElement("k8s.ngrok.com/finalizer"))

					routes := driver.GetStore().ListTLSRoutes()
					g.Expect(routes).To(HaveLen(1))
					g.Expect(routes[0].Name).To(Equal(route.Name))
				}, timeout, interval).Should(Succeed())
			})

			It("Should remove the TLSRoute from the store when deleted", func(ctx SpecContext) {
				Eventually(func(g Gomega) {
					routes := driver.GetStore().ListTLSRoutes()
					g.Expect(routes).To(HaveLen(1))
				}, timeout, interval).Should(Succeed())

				Expect(k8sClient.Delete(ctx, route)).To(Succeed())

				Eventually(func(g Gomega) {
					routes := driver.GetStore().ListTLSRoutes()
					g.Expect(routes).To(BeEmpty())
				}, timeout, interval).Should(Succeed())
			})
		})
	})

	When("the gateway class is NOT managed by us", Ordered, func() {
		var unmanagedGatewayClass *gatewayv1.GatewayClass

		BeforeAll(func(ctx SpecContext) {
			unmanagedGatewayClass = testutils.NewGatewayClass(false)
			Expect(k8sClient.Create(ctx, unmanagedGatewayClass)).To(Succeed())
		})

		AfterAll(func(ctx SpecContext) {
			Expect(client.IgnoreNotFound(k8sClient.Delete(ctx, unmanagedGatewayClass))).To(Succeed())
		})

		When("a TLSRoute references a Gateway with an unmanaged GatewayClass", func() {
			var unmanagedGateway *gatewayv1.Gateway

			BeforeEach(func(ctx SpecContext) {
				unmanagedGateway = newGateway(unmanagedGatewayClass)
				Expect(k8sClient.Create(ctx, unmanagedGateway)).To(Succeed())

				route = &gatewayv1alpha2.TLSRoute{
					ObjectMeta: metav1.ObjectMeta{
						Name:      testutils.RandomName("tlsroute"),
						Namespace: "default",
					},
					Spec: gatewayv1alpha2.TLSRouteSpec{
						CommonRouteSpec: gatewayv1.CommonRouteSpec{
							ParentRefs: []gatewayv1.ParentReference{{
								Name: gatewayv1.ObjectName(unmanagedGateway.Name),
							}},
						},
						Rules: []gatewayv1alpha2.TLSRouteRule{{
							BackendRefs: []gatewayv1alpha2.BackendRef{{
								BackendObjectReference: gatewayv1.BackendObjectReference{
									Name: gatewayv1.ObjectName("example-svc"),
									Port: (*gatewayv1.PortNumber)(ptr.To[int32](8080)),
								},
							}},
						}},
					},
				}
				Expect(k8sClient.Create(ctx, route)).To(Succeed())
			})

			AfterEach(func(ctx SpecContext) {
				Expect(client.IgnoreNotFound(k8sClient.Delete(ctx, unmanagedGateway))).To(Succeed())
				Expect(client.IgnoreNotFound(k8sClient.Delete(ctx, route))).To(Succeed())
			})

			It("Should not add a finalizer or store the TLSRoute", func(ctx SpecContext) {
				Consistently(func(g Gomega) {
					obj := &gatewayv1alpha2.TLSRoute{}
					g.Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(route), obj)).To(Succeed())
					g.Expect(obj.Finalizers).NotTo(ContainElement("k8s.ngrok.com/finalizer"))

					routes := driver.GetStore().ListTLSRoutes()
					g.Expect(routes).To(BeEmpty())
				}, duration, interval).Should(Succeed())
			})
		})
	})
})
