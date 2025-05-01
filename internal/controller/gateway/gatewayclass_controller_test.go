/*
MIT License

Copyright (c) 2022 ngrok, Inc.

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

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"
)

var _ = Describe("GatewayClass controller", func() {
	const (
		GatewayClassName = "test-gateway-class"

		timeout  = 10 * time.Second
		duration = 10 * time.Second
		interval = 250 * time.Millisecond
	)

	var (
		unmanagedGatewayClass          *gatewayv1.GatewayClass
		unmanagedGatewayClassLookupKey = types.NamespacedName{Name: "unaccepted-gateway-class"}

		managedGatewayClass          *gatewayv1.GatewayClass
		managedGatewayClassLookupKey = types.NamespacedName{Name: GatewayClassName}
	)

	BeforeEach(func() {
		ctx := GinkgoT().Context()
		unmanagedGatewayClass = &gatewayv1.GatewayClass{
			ObjectMeta: metav1.ObjectMeta{
				Name: "unaccepted-gateway-class",
			},
			Spec: gatewayv1.GatewayClassSpec{
				ControllerName: "example.com/some-other-controller",
			},
		}
		managedGatewayClass = &gatewayv1.GatewayClass{
			ObjectMeta: metav1.ObjectMeta{
				Name: GatewayClassName,
			},
			Spec: gatewayv1.GatewayClassSpec{
				ControllerName: ControllerName,
			},
		}
		Expect(k8sClient.Create(ctx, managedGatewayClass)).To(Succeed())
		Expect(k8sClient.Create(ctx, unmanagedGatewayClass)).To(Succeed())
	})

	AfterEach(func() {
		ctx := GinkgoT().Context()
		Expect(k8sClient.Delete(ctx, managedGatewayClass)).To(Succeed())
		Expect(k8sClient.Delete(ctx, unmanagedGatewayClass)).To(Succeed())
	})

	Context("When the controllerName does not match the expected value", func() {
		It("Should not accept the new GatewayClass", func() {
			By("By checking the GatewayClass status is not accepted")
			createdGatewayClass := &gatewayv1.GatewayClass{}
			Consistently(func(g Gomega) {
				g.Expect(k8sClient.Get(ctx, unmanagedGatewayClassLookupKey, createdGatewayClass)).To(Succeed())
				g.Expect(gatewayClassIsAccepted(createdGatewayClass)).To(BeFalse())

			}, duration, interval).Should(Succeed())
		})
	})

	Context("When the controllerName matches the expected value", func() {
		It("Should validate and accept the new GatewayClass", func() {
			By("By checking the GatewayClass status has an accepted condition")
			createdGatewayClass := &gatewayv1.GatewayClass{}
			Eventually(func(g Gomega) {
				g.Expect(k8sClient.Get(ctx, managedGatewayClassLookupKey, createdGatewayClass)).To(Succeed())
			}, timeout, interval).Should(Succeed())
		})

		It("Should correctly set the finalizer on the GatewayClass", func() {
			// Create the initial GatewayClass. There should be no gateways that exist, so the finalizer should not be set.
			By("By creating a new GatewayClass")
			Expect(controllerutil.ContainsFinalizer(managedGatewayClass, gatewayv1.GatewayClassFinalizerGatewaysExist)).To(BeFalse())

			// Create a Gateway that references the GatewayClass. This should cause the finalizer to be set.
			By("By creating a new Gateway")
			gateway := &gatewayv1.Gateway{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-gateway",
					Namespace: "default",
				},
				Spec: gatewayv1.GatewaySpec{
					GatewayClassName: GatewayClassName,
					Listeners: []gatewayv1.Listener{
						{
							Name:     "http",
							Port:     80,
							Protocol: gatewayv1.HTTPProtocolType,
						},
					},
				},
			}
			Expect(k8sClient.Create(ctx, gateway)).To(Succeed())

			By("By checking the GatewayClass has the finalizer set")
			createdGatewayClass := &gatewayv1.GatewayClass{}
			Eventually(func(g Gomega) {
				g.Expect(k8sClient.Get(ctx, managedGatewayClassLookupKey, createdGatewayClass)).To(Succeed())
				g.Expect(controllerutil.ContainsFinalizer(createdGatewayClass, gatewayv1.GatewayClassFinalizerGatewaysExist)).To(BeTrue())
			}, timeout, interval).Should(Succeed())

			By("By deleting the Gateway")
			Expect(k8sClient.Delete(ctx, gateway)).To(Succeed())

			By("By checking the GatewayClass has the finalizer removed")
			Eventually(func(g Gomega) {
				g.Expect(k8sClient.Get(ctx, managedGatewayClassLookupKey, createdGatewayClass)).To(Succeed())
				g.Expect(controllerutil.ContainsFinalizer(createdGatewayClass, gatewayv1.GatewayClassFinalizerGatewaysExist)).To(BeFalse())
			}).Should(Succeed())
		})
	})
})
