/*
MIT License

Copyright (c) 2024 ngrok, Inc.

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

package agent

import (
	"time"

	ngrokv1alpha1 "github.com/ngrok/ngrok-operator/api/ngrok/v1alpha1"
	"github.com/ngrok/ngrok-operator/pkg/agent"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var _ = Describe("AgentEndpoint Controller Namespace Filtering", func() {
	const (
		timeout  = 15 * time.Second
		interval = 500 * time.Millisecond
	)

	BeforeEach(func() {
		// Reset mock driver before each test
		nsMockDriver.Reset()
	})

	Context("Namespace filtering", func() {
		It("should only reconcile AgentEndpoints in the watched namespace", func(ctx SpecContext) {
			// Create AgentEndpoint in watched namespace
			watchedEndpoint := &ngrokv1alpha1.AgentEndpoint{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "watched-endpoint",
					Namespace: watchedNamespace,
				},
				Spec: ngrokv1alpha1.AgentEndpointSpec{
					URL: "tcp://1.tcp.ngrok.io:12345",
					Upstream: ngrokv1alpha1.EndpointUpstream{
						URL: "http://test-service:80",
					},
				},
			}

			// Create AgentEndpoint in unwatched namespace
			unwatchedEndpoint := &ngrokv1alpha1.AgentEndpoint{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "unwatched-endpoint",
					Namespace: unwatchedNamespace,
				},
				Spec: ngrokv1alpha1.AgentEndpointSpec{
					URL: "tcp://2.tcp.ngrok.io:12346",
					Upstream: ngrokv1alpha1.EndpointUpstream{
						URL: "http://test-service:80",
					},
				},
			}

			// Set up mock driver to return success for endpoints
			nsMockDriver.SetEndpointResult(watchedNamespace+"/watched-endpoint", &agent.EndpointResult{
				URL: "tcp://1.tcp.ngrok.io:12345",
			})
			nsMockDriver.SetEndpointResult(unwatchedNamespace+"/unwatched-endpoint", &agent.EndpointResult{
				URL: "tcp://2.tcp.ngrok.io:12346",
			})

			By("Creating AgentEndpoint in watched namespace")
			Expect(k8sClient.Create(ctx, watchedEndpoint)).To(Succeed())

			By("Creating AgentEndpoint in unwatched namespace")
			Expect(k8sClient.Create(ctx, unwatchedEndpoint)).To(Succeed())

			By("Waiting for watched endpoint to be reconciled")
			Eventually(func(g Gomega) {
				obj := &ngrokv1alpha1.AgentEndpoint{}
				g.Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(watchedEndpoint), obj)).To(Succeed())
				g.Expect(obj.Status.AssignedURL).To(Equal("tcp://1.tcp.ngrok.io:12345"))
			}, timeout, interval).Should(Succeed())

			By("Verifying unwatched endpoint was NOT reconciled")
			// Give it some time to potentially be reconciled (it shouldn't be)
			time.Sleep(2 * time.Second)

			unwatchedObj := &ngrokv1alpha1.AgentEndpoint{}
			Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(unwatchedEndpoint), unwatchedObj)).To(Succeed())
			// The unwatched endpoint should have empty status (not reconciled)
			Expect(unwatchedObj.Status.AssignedURL).To(BeEmpty(),
				"Unwatched endpoint should not have been reconciled")

			By("Verifying mock driver was only called for watched namespace")
			watchedCalls := 0
			unwatchedCalls := 0
			for _, call := range nsMockDriver.CreateCalls {
				if call.Name == watchedNamespace+"/watched-endpoint" {
					watchedCalls++
				}
				if call.Name == unwatchedNamespace+"/unwatched-endpoint" {
					unwatchedCalls++
				}
			}
			Expect(watchedCalls).To(BeNumerically(">", 0),
				"Mock driver should have been called for watched endpoint")
			Expect(unwatchedCalls).To(Equal(0),
				"Mock driver should NOT have been called for unwatched endpoint")

			By("Cleaning up test resources")
			Expect(k8sClient.Delete(ctx, watchedEndpoint)).To(Succeed())
			Expect(k8sClient.Delete(ctx, unwatchedEndpoint)).To(Succeed())
		})
	})
})
