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

package ngrok

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/ngrok/ngrok-api-go/v7"
	ngrokv1alpha1 "github.com/ngrok/ngrok-operator/api/ngrok/v1alpha1"
	"github.com/ngrok/ngrok-operator/internal/controller"
	"github.com/ngrok/ngrok-operator/internal/errors"
	"github.com/ngrok/ngrok-operator/internal/testutils"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/rand"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// Envtest tests using Ginkgo
var _ = Describe("CloudEndpoint Controller", func() {
	const (
		timeout  = 15 * time.Second
		interval = 500 * time.Millisecond
	)

	var (
		namespace     string
		cloudEndpoint *ngrokv1alpha1.CloudEndpoint
	)

	BeforeEach(func(ctx SpecContext) {
		namespace = testutils.RandomName("clep-test-namespace")
		kginkgo.ExpectCreateNamespace(ctx, namespace)

		// Reset mock endpoints client for each test
		mockEndpoints.Reset()
	})

	AfterEach(func(ctx SpecContext) {
		kginkgo.ExpectDeleteNamespace(ctx, namespace)
	})

	Context("Basic endpoint operations", func() {
		It("should successfully create a cloud endpoint with internal domain", func(ctx SpecContext) {
			cloudEndpoint = &ngrokv1alpha1.CloudEndpoint{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-endpoint",
					Namespace: namespace,
				},
				Spec: ngrokv1alpha1.CloudEndpointSpec{
					URL:         "https://test.internal",
					Description: "Test endpoint",
					Metadata:    "{}",
				},
			}

			By("Creating the CloudEndpoint")
			Expect(k8sClient.Create(ctx, cloudEndpoint)).To(Succeed())

			By("Waiting for controller to reconcile and set ready condition")
			Eventually(func(g Gomega) {
				obj := &ngrokv1alpha1.CloudEndpoint{}
				g.Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(cloudEndpoint), obj)).To(Succeed())

				// Check that endpoint was created
				g.Expect(obj.Status.ID).NotTo(BeEmpty())

				// Check ready condition
				cond := findCloudEndpointCondition(obj.Status.Conditions, ConditionCloudEndpointReady)
				g.Expect(cond).NotTo(BeNil())
				g.Expect(cond.Status).To(Equal(metav1.ConditionTrue))
			}, timeout, interval).Should(Succeed())
		})

		Context("TCP URL", func() {
			BeforeEach(func(ctx SpecContext) {
				cloudEndpoint = &ngrokv1alpha1.CloudEndpoint{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "tcp-endpoint",
						Namespace: namespace,
					},
					Spec: ngrokv1alpha1.CloudEndpointSpec{
						URL:         "tcp://1.tcp.ngrok.io:12345",
						Description: "TCP test endpoint",
						Metadata:    "{}",
					},
				}

				By("Creating the CloudEndpoint")
				Expect(k8sClient.Create(ctx, cloudEndpoint)).To(Succeed())
			})

			It("should successfully assign the clep an ID", func(ctx SpecContext) {
				By("Waiting for controller to reconcile")
				kginkgo.EventuallyWithCloudEndpoint(ctx, cloudEndpoint, func(g Gomega, clep *ngrokv1alpha1.CloudEndpoint) {
					g.Expect(clep.Status.ID).NotTo(BeEmpty())
				})

				By("Verifying the endpoint was created in ngrok")
				Eventually(func(g Gomega) {
					ep, err := mockEndpoints.Get(ctx, cloudEndpoint.Status.ID)
					g.Expect(ep).NotTo(BeNil())
					g.Expect(err).To(BeNil())
				})
			})

			When("the cleanup annotation is added", func() {
				var (
					id string
				)

				BeforeEach(func(ctx SpecContext) {
					By("waiting for the CloudEndpoint to have an ID")
					kginkgo.EventuallyWithCloudEndpoint(ctx, cloudEndpoint, func(g Gomega, clep *ngrokv1alpha1.CloudEndpoint) {
						id = clep.Status.ID
						g.Expect(id).NotTo(BeEmpty())
					})
					By("Annotating the CloudEndpoint for cleanup")
					kginkgo.ExpectAddAnnotations(ctx, cloudEndpoint, map[string]string{
						controller.CleanupAnnotation: "true",
					})
				})

				It("should remove the finalizer", func(ctx SpecContext) {
					kginkgo.ExpectFinalizerToBeRemoved(ctx, cloudEndpoint, controller.FinalizerName)
				})

				It("should clear the status", func(ctx SpecContext) {
					kginkgo.EventuallyWithCloudEndpoint(ctx, cloudEndpoint, func(g Gomega, clep *ngrokv1alpha1.CloudEndpoint) {
						g.Expect(clep.Status).To(Equal(ngrokv1alpha1.CloudEndpointStatus{}))
					})
				})

				It("should delete the CloudEndpoint in ngrok", func(ctx SpecContext) {
					Eventually(func(g Gomega) {
						ep, err := mockEndpoints.Get(ctx, id)
						g.Expect(ep).To(BeNil())
						g.Expect(ngrok.IsNotFound(err)).To(BeTrue())
					})
				})
			})
		})

		It("should handle endpoint with traffic policy reference", func(ctx SpecContext) {
			// Create traffic policy first
			trafficPolicy := &ngrokv1alpha1.NgrokTrafficPolicy{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-policy",
					Namespace: namespace,
				},
				Spec: ngrokv1alpha1.NgrokTrafficPolicySpec{
					Policy: []byte(`{"on_http_request":[{"name":"rate-limit"}]}`),
				},
			}
			Expect(k8sClient.Create(ctx, trafficPolicy)).To(Succeed())

			cloudEndpoint = &ngrokv1alpha1.CloudEndpoint{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "policy-endpoint",
					Namespace: namespace,
				},
				Spec: ngrokv1alpha1.CloudEndpointSpec{
					URL:               "https://policy-test.internal",
					Description:       "Endpoint with policy",
					Metadata:          "{}",
					TrafficPolicyName: "test-policy",
				},
			}

			By("Creating the CloudEndpoint with traffic policy")
			Expect(k8sClient.Create(ctx, cloudEndpoint)).To(Succeed())

			By("Waiting for controller to reconcile")
			Eventually(func(g Gomega) {
				obj := &ngrokv1alpha1.CloudEndpoint{}
				g.Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(cloudEndpoint), obj)).To(Succeed())

				// Check that endpoint was created
				g.Expect(obj.Status.ID).NotTo(BeEmpty())
			}, timeout, interval).Should(Succeed())
		})

		It("should handle endpoint deletion", func(ctx SpecContext) {
			cloudEndpoint = &ngrokv1alpha1.CloudEndpoint{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "delete-endpoint",
					Namespace: namespace,
				},
				Spec: ngrokv1alpha1.CloudEndpointSpec{
					URL:         fmt.Sprintf("https://delete-%s.internal", rand.String(8)),
					Description: "Endpoint to delete",
					Metadata:    "{}",
				},
			}

			By("Creating the CloudEndpoint")
			Expect(k8sClient.Create(ctx, cloudEndpoint)).To(Succeed())

			By("Waiting for controller to create the endpoint")
			var endpointID string
			Eventually(func(g Gomega) {
				obj := &ngrokv1alpha1.CloudEndpoint{}
				g.Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(cloudEndpoint), obj)).To(Succeed())
				g.Expect(obj.Status.ID).NotTo(BeEmpty())
				endpointID = obj.Status.ID
			}, timeout, interval).Should(Succeed())

			By("Deleting the CloudEndpoint")
			Expect(k8sClient.Delete(ctx, cloudEndpoint)).To(Succeed())

			By("Waiting for controller to clean up")
			Eventually(func(g Gomega) {
				obj := &ngrokv1alpha1.CloudEndpoint{}
				err := k8sClient.Get(ctx, client.ObjectKeyFromObject(cloudEndpoint), obj)
				g.Expect(err).To(HaveOccurred())
			}, timeout, interval).Should(Succeed())

			// Verify endpoint was deleted from mock
			_, err := mockEndpoints.Get(context.Background(), endpointID)
			Expect(err).To(HaveOccurred())
		})

		It("should handle API errors gracefully", func(ctx SpecContext) {
			// Set the mock to return an error
			mockEndpoints.SetCreateError(errors.New("API error"))

			cloudEndpoint = &ngrokv1alpha1.CloudEndpoint{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "error-endpoint",
					Namespace: namespace,
				},
				Spec: ngrokv1alpha1.CloudEndpointSpec{
					URL:         "https://error.internal",
					Description: "Error endpoint",
					Metadata:    "{}",
				},
			}

			By("Creating the CloudEndpoint")
			Expect(k8sClient.Create(ctx, cloudEndpoint)).To(Succeed())

			By("Waiting for controller to handle the error")
			Eventually(func(g Gomega) {
				obj := &ngrokv1alpha1.CloudEndpoint{}
				g.Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(cloudEndpoint), obj)).To(Succeed())

				// Check that error condition is set
				cond := findCloudEndpointCondition(obj.Status.Conditions, ConditionCloudEndpointCreated)
				g.Expect(cond).NotTo(BeNil())
				g.Expect(cond.Status).To(Equal(metav1.ConditionFalse))
			}, timeout, interval).Should(Succeed())

			// Clear the error for cleanup
			mockEndpoints.SetCreateError(nil)
		})
	})

	Context("Traffic policy validation", func() {
		It("should reject CloudEndpoint with both TrafficPolicyName and TrafficPolicy set", func(ctx SpecContext) {
			cloudEndpoint = &ngrokv1alpha1.CloudEndpoint{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "invalid-policy-endpoint",
					Namespace: namespace,
				},
				Spec: ngrokv1alpha1.CloudEndpointSpec{
					URL:               "https://invalid.internal",
					Description:       "Invalid policy config",
					Metadata:          "{}",
					TrafficPolicyName: "some-policy",
					TrafficPolicy: &ngrokv1alpha1.NgrokTrafficPolicySpec{
						Policy: json.RawMessage(`{"on_http_request":[]}`),
					},
				},
			}

			By("Creating the CloudEndpoint with invalid config")
			Expect(k8sClient.Create(ctx, cloudEndpoint)).To(Succeed())

			By("Waiting for controller to detect the configuration error")
			Eventually(func(g Gomega) {
				obj := &ngrokv1alpha1.CloudEndpoint{}
				g.Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(cloudEndpoint), obj)).To(Succeed())

				// Should not have created an endpoint
				g.Expect(obj.Status.ID).To(BeEmpty())
			}, timeout, interval).Should(Succeed())
		})
	})

	Context("Endpoint updates", func() {
		It("should successfully update endpoint description and metadata", func(ctx SpecContext) {
			cloudEndpoint = &ngrokv1alpha1.CloudEndpoint{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "update-endpoint",
					Namespace: namespace,
				},
				Spec: ngrokv1alpha1.CloudEndpointSpec{
					URL:         "https://update-test.internal",
					Description: "Original description",
					Metadata:    `{"key":"value"}`,
				},
			}

			By("Creating the CloudEndpoint")
			Expect(k8sClient.Create(ctx, cloudEndpoint)).To(Succeed())

			By("Waiting for initial creation")
			var endpointID string
			Eventually(func(g Gomega) {
				obj := &ngrokv1alpha1.CloudEndpoint{}
				g.Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(cloudEndpoint), obj)).To(Succeed())
				g.Expect(obj.Status.ID).NotTo(BeEmpty())
				endpointID = obj.Status.ID
			}, timeout, interval).Should(Succeed())

			By("Updating the description and metadata")
			Eventually(func(g Gomega) {
				obj := &ngrokv1alpha1.CloudEndpoint{}
				g.Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(cloudEndpoint), obj)).To(Succeed())

				obj.Spec.Description = "Updated description"
				obj.Spec.Metadata = `{"key":"updated","new":"field"}`
				g.Expect(k8sClient.Update(ctx, obj)).To(Succeed())
			}, timeout, interval).Should(Succeed())

			By("Verifying the update was applied")
			Eventually(func(g Gomega) {
				// Check in mock client
				endpoint, err := mockEndpoints.Get(context.Background(), endpointID)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(endpoint.Description).To(Equal("Updated description"))
				g.Expect(endpoint.Metadata).To(Equal(`{"key":"updated","new":"field"}`))
			}, timeout, interval).Should(Succeed())
		})

		It("should successfully update traffic policy", func(ctx SpecContext) {
			// Create initial traffic policy
			trafficPolicy1 := &ngrokv1alpha1.NgrokTrafficPolicy{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "policy-v1",
					Namespace: namespace,
				},
				Spec: ngrokv1alpha1.NgrokTrafficPolicySpec{
					Policy: []byte(`{"on_http_request":[{"name":"log"}]}`),
				},
			}
			Expect(k8sClient.Create(ctx, trafficPolicy1)).To(Succeed())

			cloudEndpoint = &ngrokv1alpha1.CloudEndpoint{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "policy-update-endpoint",
					Namespace: namespace,
				},
				Spec: ngrokv1alpha1.CloudEndpointSpec{
					URL:               "https://policy-update.internal",
					Description:       "Endpoint with updatable policy",
					Metadata:          "{}",
					TrafficPolicyName: "policy-v1",
				},
			}

			By("Creating the CloudEndpoint with initial policy")
			Expect(k8sClient.Create(ctx, cloudEndpoint)).To(Succeed())

			By("Waiting for initial creation")
			var endpointID string
			Eventually(func(g Gomega) {
				obj := &ngrokv1alpha1.CloudEndpoint{}
				g.Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(cloudEndpoint), obj)).To(Succeed())
				g.Expect(obj.Status.ID).NotTo(BeEmpty())
				endpointID = obj.Status.ID
			}, timeout, interval).Should(Succeed())

			By("Creating new traffic policy")
			trafficPolicy2 := &ngrokv1alpha1.NgrokTrafficPolicy{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "policy-v2",
					Namespace: namespace,
				},
				Spec: ngrokv1alpha1.NgrokTrafficPolicySpec{
					Policy: []byte(`{"on_http_request":[{"name":"rate-limit"}]}`),
				},
			}
			Expect(k8sClient.Create(ctx, trafficPolicy2)).To(Succeed())

			By("Updating to new traffic policy")
			Eventually(func(g Gomega) {
				obj := &ngrokv1alpha1.CloudEndpoint{}
				g.Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(cloudEndpoint), obj)).To(Succeed())

				obj.Spec.TrafficPolicyName = "policy-v2"
				g.Expect(k8sClient.Update(ctx, obj)).To(Succeed())
			}, timeout, interval).Should(Succeed())

			By("Verifying the policy was updated")
			Eventually(func(g Gomega) {
				endpoint, err := mockEndpoints.Get(context.Background(), endpointID)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(endpoint.TrafficPolicy).To(ContainSubstring("rate-limit"))
			}, timeout, interval).Should(Succeed())
		})

		It("should recreate endpoint when update returns 404", func(ctx SpecContext) {
			cloudEndpoint = &ngrokv1alpha1.CloudEndpoint{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "recreate-endpoint",
					Namespace: namespace,
				},
				Spec: ngrokv1alpha1.CloudEndpointSpec{
					URL:         "https://recreate-test.internal",
					Description: "Endpoint to recreate",
					Metadata:    "{}",
				},
			}

			By("Creating the CloudEndpoint")
			Expect(k8sClient.Create(ctx, cloudEndpoint)).To(Succeed())

			By("Waiting for initial creation")
			var originalID string
			Eventually(func(g Gomega) {
				obj := &ngrokv1alpha1.CloudEndpoint{}
				g.Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(cloudEndpoint), obj)).To(Succeed())
				g.Expect(obj.Status.ID).NotTo(BeEmpty())
				originalID = obj.Status.ID
			}, timeout, interval).Should(Succeed())

			By("Manually deleting the endpoint from mock to simulate 404")
			err := mockEndpoints.Delete(context.Background(), originalID)
			Expect(err).NotTo(HaveOccurred())

			By("Updating the CloudEndpoint spec to trigger reconcile")
			Eventually(func(g Gomega) {
				obj := &ngrokv1alpha1.CloudEndpoint{}
				g.Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(cloudEndpoint), obj)).To(Succeed())

				obj.Spec.Description = "Updated after deletion"
				g.Expect(k8sClient.Update(ctx, obj)).To(Succeed())
			}, timeout, interval).Should(Succeed())

			By("Verifying a new endpoint was created")
			Eventually(func(g Gomega) {
				obj := &ngrokv1alpha1.CloudEndpoint{}
				g.Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(cloudEndpoint), obj)).To(Succeed())

				// Should have a new ID
				g.Expect(obj.Status.ID).NotTo(BeEmpty())

				// Verify the new endpoint exists in mock
				endpoint, err := mockEndpoints.Get(context.Background(), obj.Status.ID)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(endpoint.Description).To(Equal("Updated after deletion"))
			}, timeout, interval).Should(Succeed())
		})

		It("should update URL successfully", func(ctx SpecContext) {
			cloudEndpoint = &ngrokv1alpha1.CloudEndpoint{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "url-update-endpoint",
					Namespace: namespace,
				},
				Spec: ngrokv1alpha1.CloudEndpointSpec{
					URL:         "https://original-url.internal",
					Description: "Endpoint with updatable URL",
					Metadata:    "{}",
				},
			}

			By("Creating the CloudEndpoint")
			Expect(k8sClient.Create(ctx, cloudEndpoint)).To(Succeed())

			By("Waiting for initial creation")
			var endpointID string
			Eventually(func(g Gomega) {
				obj := &ngrokv1alpha1.CloudEndpoint{}
				g.Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(cloudEndpoint), obj)).To(Succeed())
				g.Expect(obj.Status.ID).NotTo(BeEmpty())
				endpointID = obj.Status.ID
			}, timeout, interval).Should(Succeed())

			By("Updating the URL")
			Eventually(func(g Gomega) {
				obj := &ngrokv1alpha1.CloudEndpoint{}
				g.Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(cloudEndpoint), obj)).To(Succeed())

				obj.Spec.URL = "https://updated-url.internal"
				g.Expect(k8sClient.Update(ctx, obj)).To(Succeed())
			}, timeout, interval).Should(Succeed())

			By("Verifying the URL was updated")
			Eventually(func(g Gomega) {
				endpoint, err := mockEndpoints.Get(context.Background(), endpointID)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(endpoint.URL).To(Equal("https://updated-url.internal"))
			}, timeout, interval).Should(Succeed())
		})

		It("should handle update errors gracefully", func(ctx SpecContext) {
			cloudEndpoint = &ngrokv1alpha1.CloudEndpoint{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "update-error-endpoint",
					Namespace: namespace,
				},
				Spec: ngrokv1alpha1.CloudEndpointSpec{
					URL:         "https://update-error.internal",
					Description: "Endpoint for error testing",
					Metadata:    "{}",
				},
			}

			By("Creating the CloudEndpoint")
			Expect(k8sClient.Create(ctx, cloudEndpoint)).To(Succeed())

			By("Waiting for initial creation")
			Eventually(func(g Gomega) {
				obj := &ngrokv1alpha1.CloudEndpoint{}
				g.Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(cloudEndpoint), obj)).To(Succeed())
				g.Expect(obj.Status.ID).NotTo(BeEmpty())
			}, timeout, interval).Should(Succeed())

			By("Setting mock to return error on update")
			mockEndpoints.SetUpdateError(errors.New("API update error"))

			By("Updating the CloudEndpoint")
			Eventually(func(g Gomega) {
				obj := &ngrokv1alpha1.CloudEndpoint{}
				g.Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(cloudEndpoint), obj)).To(Succeed())

				obj.Spec.Description = "This update should fail"
				g.Expect(k8sClient.Update(ctx, obj)).To(Succeed())
			}, timeout, interval).Should(Succeed())

			By("Verifying error condition is set")
			Eventually(func(g Gomega) {
				obj := &ngrokv1alpha1.CloudEndpoint{}
				g.Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(cloudEndpoint), obj)).To(Succeed())

				// Check that error condition is set
				cond := findCloudEndpointCondition(obj.Status.Conditions, ConditionCloudEndpointCreated)
				g.Expect(cond).NotTo(BeNil())
				g.Expect(cond.Status).To(Equal(metav1.ConditionFalse))
			}, timeout, interval).Should(Succeed())

			// Clean up error for other tests
			mockEndpoints.SetUpdateError(nil)
		})
	})
})

// Helper function to find condition by type
func findCloudEndpointCondition(conditions []metav1.Condition, conditionType string) *metav1.Condition {
	for i := range conditions {
		if conditions[i].Type == conditionType {
			return &conditions[i]
		}
	}
	return nil
}
