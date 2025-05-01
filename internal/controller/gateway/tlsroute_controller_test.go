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

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"
	gatewayv1alpha2 "sigs.k8s.io/gateway-api/apis/v1alpha2"
)

var _ = Describe("TLSRoute controller", func() {
	const (
		TLSRouteName = "test-tlsroute"
	)

	var (
		testTLSroute *gatewayv1alpha2.TLSRoute
	)

	BeforeEach(func() {
		ctx := GinkgoT().Context()
		testTLSroute = &gatewayv1alpha2.TLSRoute{
			ObjectMeta: metav1.ObjectMeta{
				Name: TLSRouteName,
			},
			Spec: gatewayv1alpha2.TLSRouteSpec{
				CommonRouteSpec: gatewayv1.CommonRouteSpec{
					ParentRefs: []gatewayv1.ParentReference{{
						Kind: ptr.To(gatewayv1.Kind("gateway")),
						Name: gatewayv1.ObjectName("example-gw"),
					}},
				},
				Rules: []gatewayv1alpha2.TLSRouteRule{{
					BackendRefs: []gatewayv1alpha2.BackendRef{{
						BackendObjectReference: gatewayv1.BackendObjectReference{
							Name: gatewayv1.ObjectName("example-svc"),
						},
					}},
				}},
			},
		}
		Expect(k8sClient.Create(ctx, testTLSroute)).To(Succeed())
	})

	AfterEach(func() {
		ctx := GinkgoT().Context()
		hasTLSRoute := false
		tlsRoutes := driver.GetStore().ListTLSRoutes()
		for _, tlsRoute := range tlsRoutes {
			if tlsRoute.Name == "test-tlsroute" {
				hasTLSRoute = true
			}
		}
		Expect(len(tlsRoutes)).To(Equal(1))
		Expect(hasTLSRoute).To(Equal(true))
		Expect(k8sClient.Delete(ctx, testTLSroute)).To(Succeed())
		time.Sleep(time.Second * 3)
		tlsRoutes = driver.GetStore().ListTLSRoutes()
		Expect(len(tlsRoutes)).To(Equal(0))
	})
})
