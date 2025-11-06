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

	controller "github.com/ngrok/ngrok-operator/internal/controller"
	"github.com/ngrok/ngrok-operator/internal/errors"
	testutils "github.com/ngrok/ngrok-operator/internal/testutils"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"
	gatewayv1alpha2 "sigs.k8s.io/gateway-api/apis/v1alpha2"
)

var _ = Describe("TCPRoute controller", func() {
	const (
		timeout  = 10 * time.Second
		interval = 250 * time.Millisecond
	)

	var (
		namespace    string
		testTCProute *gatewayv1alpha2.TCPRoute
	)

	BeforeEach(func(ctx SpecContext) {
		namespace = testutils.RandomName("test-namespace")
		kginkgo.ExpectCreateNamespace(ctx, namespace)

		testTCProute = &gatewayv1alpha2.TCPRoute{
			ObjectMeta: metav1.ObjectMeta{
				Name:      testutils.RandomName("test-tcproute"),
				Namespace: namespace,
			},
			Spec: gatewayv1alpha2.TCPRouteSpec{
				CommonRouteSpec: gatewayv1.CommonRouteSpec{
					ParentRefs: []gatewayv1.ParentReference{{
						Kind: ptr.To(gatewayv1.Kind("gateway")),
						Name: gatewayv1.ObjectName("example-gw"),
					}},
				},
				Rules: []gatewayv1alpha2.TCPRouteRule{{
					BackendRefs: []gatewayv1alpha2.BackendRef{{
						BackendObjectReference: gatewayv1.BackendObjectReference{
							Name: gatewayv1.ObjectName("example-svc"),
							Port: ptr.To(gatewayv1.PortNumber(8080)),
						},
					}},
				}},
			},
		}
		Expect(k8sClient.Create(ctx, testTCProute)).To(Succeed())
	})

	It("should add the tcproute to the store", func(_ SpecContext) {
		Eventually(func(g Gomega) {
			store := driver.GetStore()
			fetched, err := store.GetTCPRoute(testTCProute.GetName(), testTCProute.GetNamespace())
			g.Expect(fetched).NotTo(BeNil())
			g.Expect(err).To(BeNil())
		}, timeout, interval).Should(Succeed())
	})

	When("the tcproute is annotated for cleanup", func() {
		BeforeEach(func() {
			By("Annotating the tcproute for cleanup")
			kginkgo.ExpectAddAnnotations(ctx, testTCProute, map[string]string{
				controller.CleanupAnnotation: "true",
			})
		})

		It("should remove the tcproute from the store", func(_ SpecContext) {
			Eventually(func(g Gomega) {
				store := driver.GetStore()
				fetched, err := store.GetTCPRoute(testTCProute.GetName(), testTCProute.GetNamespace())
				g.Expect(fetched).To(BeNil())
				g.Expect(err).NotTo(BeNil())
				g.Expect(errors.IsErrorNotFound(err)).To(BeTrue())
			}, timeout, interval).Should(Succeed())
		})

		It("should remove the finalizer from the tcproute", func(ctx SpecContext) {
			kginkgo.ExpectFinalizerToBeRemoved(ctx, testTCProute, controller.FinalizerName)
		})
	})

	AfterEach(func(ctx SpecContext) {
		Expect(k8sClient.Delete(ctx, testTCProute)).To(Succeed())
		time.Sleep(time.Second * 3)
		tcpRoutes := driver.GetStore().ListTCPRoutes()
		Expect(len(tcpRoutes)).To(Equal(0))

		kginkgo.ExpectDeleteNamespace(ctx, namespace)
	})
})
