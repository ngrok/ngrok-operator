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

	testutils "github.com/ngrok/ngrok-operator/internal/testutils"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"
)

var _ = Describe("GatewayClass controller", func() {
	const (
		timeout  = 10 * time.Second
		duration = 10 * time.Second
		interval = 250 * time.Millisecond
	)

	var (
		isManaged    bool
		gatewayClass *gatewayv1.GatewayClass
	)

	JustBeforeEach(func(ctx SpecContext) {
		// Create the GatewayClass
		gatewayClass = testutils.NewGatewayClass(isManaged)
		Expect(k8sClient.Create(ctx, gatewayClass)).To(Succeed())
	})

	AfterEach(func(ctx SpecContext) {
		DeleteAllGatewayClasses(ctx, timeout, interval)
	})

	When("the controllerName does not match the expected value", func() {
		BeforeEach(func() {
			isManaged = false
		})

		It("Should not accept the new GatewayClass", func(ctx SpecContext) {
			By("By checking the GatewayClass status is not accepted")
			Consistently(func(g Gomega) {
				obj := &gatewayv1.GatewayClass{}
				g.Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(gatewayClass), obj)).To(Succeed())
				g.Expect(gatewayClassIsAccepted(obj)).To(BeFalse())
			}, duration, interval).Should(Succeed())
		})
	})

	When("the controllerName matches the expected value", func() {
		BeforeEach(func() {
			isManaged = true
		})

		It("Should validate and accept the new GatewayClass", func(ctx SpecContext) {
			By("By checking the GatewayClass status has an accepted condition")
			ExpectGatewayClassAccepted(ctx, gatewayClass, timeout, interval)
		})

		It("Should correctly set the finalizer on the GatewayClass", func(ctx SpecContext) {
			// Create the initial GatewayClass. There should be no gateways that exist, so the finalizer should not be set.
			By("By creating a new GatewayClass")
			Expect(controllerutil.ContainsFinalizer(gatewayClass, gatewayv1.GatewayClassFinalizerGatewaysExist)).To(BeFalse())

			// Create a Gateway that references the GatewayClass. This should cause the finalizer to be set.
			By("By creating a new Gateway")
			gateway := &gatewayv1.Gateway{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-gateway",
					Namespace: "default",
				},
				Spec: gatewayv1.GatewaySpec{
					GatewayClassName: gatewayv1.ObjectName(gatewayClass.Name),
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
			Eventually(func(g Gomega) {
				obj := &gatewayv1.GatewayClass{}
				g.Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(gatewayClass), obj)).To(Succeed())
				g.Expect(controllerutil.ContainsFinalizer(obj, gatewayv1.GatewayClassFinalizerGatewaysExist)).To(BeTrue())
			}, timeout, interval).Should(Succeed())

			By("By deleting the Gateway")
			Expect(k8sClient.Delete(ctx, gateway)).To(Succeed())

			By("By checking the GatewayClass has the finalizer removed")
			Eventually(func(g Gomega) {
				obj := &gatewayv1.GatewayClass{}
				g.Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(gatewayClass), obj)).To(Succeed())
				g.Expect(controllerutil.ContainsFinalizer(obj, gatewayv1.GatewayClassFinalizerGatewaysExist)).To(BeFalse())
			}).Should(Succeed())
		})
	})
})
