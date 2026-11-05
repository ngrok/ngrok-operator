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

	"github.com/ngrok/ngrok-api-go/v9"
	commonv1alpha1 "github.com/ngrok/ngrok-operator/api/common/v1alpha1"
	ngrokv1 "github.com/ngrok/ngrok-operator/api/ngrok/v1"
	ngrokv1alpha1 "github.com/ngrok/ngrok-operator/api/ngrok/v1alpha1"
	"github.com/ngrok/ngrok-operator/internal/cloudendpoint"
	"github.com/ngrok/ngrok-operator/internal/controller/labels"
	"github.com/ngrok/ngrok-operator/internal/errors"
	"github.com/ngrok/ngrok-operator/internal/mocks/nmockapi"
	trafficpolicypkg "github.com/ngrok/ngrok-operator/internal/trafficpolicy"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	v1 "k8s.io/api/core/v1"
	eventsv1 "k8s.io/api/events/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
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
		cloudEndpoint *ngrokv1.CloudEndpoint
	)

	BeforeEach(func() {
		namespace = "test-" + rand.String(8)

		// Create namespace for testing
		ns := &v1.Namespace{
			Name: namespace,
		}
		Expect(k8sClient.Create(context.Background(), ns)).To(Succeed())

		// Reset mock endpoints client for each test
		mockClientset.Endpoints().(*nmockapi.EndpointsClient).Reset()
	})

	AfterEach(func() {
		// Clean up namespace
		if namespace != "" {
			ns := &v1.Namespace{
				Name: namespace,
			}
			_ = k8sClient.Delete(context.Background(), ns)
		}
	})

	Context("Basic endpoint operations", func() {
		It("should successfully create a cloud endpoint with internal domain", func(ctx SpecContext) {
			cloudEndpoint = &ngrokv1.CloudEndpoint{
				Name:      "test-endpoint",
				Namespace: namespace,
				Spec: ngrokv1.CloudEndpointSpec{
					URL:         "https://test.internal",
					Description: "Test endpoint",
				},
			}

			By("Creating the CloudEndpoint")
			Expect(k8sClient.Create(ctx, cloudEndpoint)).To(Succeed())

			By("Waiting for controller to reconcile and set ready condition")
			Eventually(func(g Gomega) {
				obj := &ngrokv1.CloudEndpoint{}
				g.Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(cloudEndpoint), obj)).To(Succeed())

				// Check that endpoint was created
				g.Expect(obj.Status.ID).NotTo(BeEmpty())
				g.Expect(obj.Status.AssignedURL).To(Equal("https://test.internal"))

				// Check ready condition
				cond := findCloudEndpointCondition(obj.Status.Conditions, cloudendpoint.ConditionCloudEndpointReady)
				g.Expect(cond).NotTo(BeNil())
				g.Expect(cond.Status).To(Equal(metav1.ConditionTrue))
			}, timeout, interval).Should(Succeed())
		})

		It("should successfully create a cloud endpoint with TCP URL", func(ctx SpecContext) {
			cloudEndpoint = &ngrokv1.CloudEndpoint{
				Name:      "tcp-endpoint",
				Namespace: namespace,
				Spec: ngrokv1.CloudEndpointSpec{
					URL:         "tcp://1.tcp.ngrok.io:12345",
					Description: "TCP test endpoint",
				},
			}

			By("Creating the CloudEndpoint")
			Expect(k8sClient.Create(ctx, cloudEndpoint)).To(Succeed())

			By("Waiting for controller to reconcile")
			Eventually(func(g Gomega) {
				obj := &ngrokv1.CloudEndpoint{}
				g.Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(cloudEndpoint), obj)).To(Succeed())

				// Check that endpoint was created
				g.Expect(obj.Status.ID).NotTo(BeEmpty())
			}, timeout, interval).Should(Succeed())

			By("Verifying no domain CR was created for TCP endpoint")
			Eventually(func(g Gomega) {
				domains := &ngrokv1.DomainList{}
				g.Expect(k8sClient.List(ctx, domains, client.InNamespace(namespace))).To(Succeed())
				g.Expect(domains.Items).To(BeEmpty())
			}, timeout, interval).Should(Succeed())

			By("Verifying no domain ref is set")
			Eventually(func(g Gomega) {
				obj := &ngrokv1.CloudEndpoint{}
				g.Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(cloudEndpoint), obj)).To(Succeed())
				g.Expect(obj.Status.DomainRef).To(BeNil())
			}, timeout, interval).Should(Succeed())
		})

		It("should not create domain CR for custom TCP endpoint", func(ctx SpecContext) {
			cloudEndpoint = &ngrokv1.CloudEndpoint{
				Name:      "tcp-custom-endpoint",
				Namespace: namespace,
				Spec: ngrokv1.CloudEndpointSpec{
					URL:         "tcp://1.2.3.4:25565",
					Description: "Custom TCP test endpoint",
				},
			}

			By("Creating the CloudEndpoint")
			Expect(k8sClient.Create(ctx, cloudEndpoint)).To(Succeed())

			By("Waiting for controller to reconcile")
			Eventually(func(g Gomega) {
				obj := &ngrokv1.CloudEndpoint{}
				g.Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(cloudEndpoint), obj)).To(Succeed())
				g.Expect(obj.Status.ID).NotTo(BeEmpty())
			}, timeout, interval).Should(Succeed())

			By("Verifying no domain CR was created for custom TCP endpoint")
			Eventually(func(g Gomega) {
				domains := &ngrokv1.DomainList{}
				g.Expect(k8sClient.List(ctx, domains, client.InNamespace(namespace))).To(Succeed())
				g.Expect(domains.Items).To(BeEmpty())
			}, timeout, interval).Should(Succeed())

			By("Verifying no domain ref is set")
			Eventually(func(g Gomega) {
				obj := &ngrokv1.CloudEndpoint{}
				g.Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(cloudEndpoint), obj)).To(Succeed())
				g.Expect(obj.Status.DomainRef).To(BeNil())
			}, timeout, interval).Should(Succeed())
		})

		It("should handle endpoint with trafficPolicy.targetRef", func(ctx SpecContext) {
			trafficPolicy := &ngrokv1alpha1.NgrokTrafficPolicy{
				Name:      "new-shape-policy",
				Namespace: namespace,
				Spec: ngrokv1alpha1.NgrokTrafficPolicySpec{
					Policy: []byte(`{"on_http_request":[{"name":"rate-limit"}]}`),
				},
			}
			Expect(k8sClient.Create(ctx, trafficPolicy)).To(Succeed())

			cloudEndpoint = &ngrokv1.CloudEndpoint{
				Name:      "new-shape-endpoint",
				Namespace: namespace,
				Spec: ngrokv1.CloudEndpointSpec{
					URL: "https://new-shape.internal",
					TrafficPolicy: &ngrokv1.TrafficPolicyCfg{
						Reference: &ngrokv1.K8sObjectRef{
							Name: "new-shape-policy",
						},
					},
				},
			}

			Expect(k8sClient.Create(ctx, cloudEndpoint)).To(Succeed())

			Eventually(func(g Gomega) {
				obj := &ngrokv1.CloudEndpoint{}
				g.Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(cloudEndpoint), obj)).To(Succeed())
				g.Expect(obj.Status.ID).NotTo(BeEmpty())

				tpCond := findCloudEndpointCondition(obj.Status.Conditions, cloudendpoint.ConditionTrafficPolicy)
				g.Expect(tpCond).NotTo(BeNil())
				g.Expect(tpCond.Status).To(Equal(metav1.ConditionTrue))
			}, timeout, interval).Should(Succeed())
		})

		It("should handle endpoint deletion", func(ctx SpecContext) {
			cloudEndpoint = &ngrokv1.CloudEndpoint{
				Name:      "delete-endpoint",
				Namespace: namespace,
				Spec: ngrokv1.CloudEndpointSpec{
					URL:         fmt.Sprintf("https://delete-%s.internal", rand.String(8)),
					Description: "Endpoint to delete",
				},
			}

			By("Creating the CloudEndpoint")
			Expect(k8sClient.Create(ctx, cloudEndpoint)).To(Succeed())

			By("Waiting for controller to create the endpoint")
			var endpointID string
			Eventually(func(g Gomega) {
				obj := &ngrokv1.CloudEndpoint{}
				g.Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(cloudEndpoint), obj)).To(Succeed())
				g.Expect(obj.Status.ID).NotTo(BeEmpty())
				endpointID = obj.Status.ID
			}, timeout, interval).Should(Succeed())

			By("Deleting the CloudEndpoint")
			Expect(k8sClient.Delete(ctx, cloudEndpoint)).To(Succeed())

			By("Waiting for controller to clean up")
			Eventually(func(g Gomega) {
				obj := &ngrokv1.CloudEndpoint{}
				err := k8sClient.Get(ctx, client.ObjectKeyFromObject(cloudEndpoint), obj)
				g.Expect(err).To(HaveOccurred())
			}, timeout, interval).Should(Succeed())

			// Verify endpoint was deleted from mock
			_, err := mockClientset.Endpoints().Get(context.Background(), endpointID)
			Expect(err).To(HaveOccurred())
		})

		It("should handle API errors gracefully", func(ctx SpecContext) {
			// Set the mock to return an error
			mockClientset.Endpoints().(*nmockapi.EndpointsClient).SetCreateError(errors.New("API error"))

			cloudEndpoint = &ngrokv1.CloudEndpoint{
				Name:      "error-endpoint",
				Namespace: namespace,
				Spec: ngrokv1.CloudEndpointSpec{
					URL:         "https://error.internal",
					Description: "Error endpoint",
				},
			}

			By("Creating the CloudEndpoint")
			Expect(k8sClient.Create(ctx, cloudEndpoint)).To(Succeed())

			By("Waiting for controller to handle the error")
			Eventually(func(g Gomega) {
				obj := &ngrokv1.CloudEndpoint{}
				g.Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(cloudEndpoint), obj)).To(Succeed())

				// Check that error condition is set
				cond := findCloudEndpointCondition(obj.Status.Conditions, cloudendpoint.ConditionCloudEndpointCreated)
				g.Expect(cond).NotTo(BeNil())
				g.Expect(cond.Status).To(Equal(metav1.ConditionFalse))
			}, timeout, interval).Should(Succeed())

			// Clear the error for cleanup
			mockClientset.Endpoints().(*nmockapi.EndpointsClient).SetCreateError(nil)
		})
	})

	Context("Traffic policy validation", func() {
		It("rejects spec.trafficPolicy.inline + spec.trafficPolicy.targetRef at admission", func() {
			// Both endpoint kinds use the shared TrafficPolicyCfg union.
			cloudEndpoint = &ngrokv1.CloudEndpoint{
				Name:      "invalid-tp-union-endpoint",
				Namespace: namespace,
				Spec: ngrokv1.CloudEndpointSpec{
					URL: "https://invalid-tp-union.internal",
					TrafficPolicy: &ngrokv1.TrafficPolicyCfg{
						Inline:    json.RawMessage(`{"on_http_request":[]}`),
						Reference: &ngrokv1.K8sObjectRef{Name: "policy"},
					},
				},
			}

			err := k8sClient.Create(context.Background(), cloudEndpoint)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("exactly one of inline or targetRef must be set on trafficPolicy"))
		})

		It("rejects an empty spec.trafficPolicy at admission", func(ctx SpecContext) {
			cloudEndpoint = &ngrokv1.CloudEndpoint{
				Name:      "empty-tp-endpoint",
				Namespace: namespace,
				Spec: ngrokv1.CloudEndpointSpec{
					URL:           "https://empty-tp.internal",
					TrafficPolicy: &ngrokv1.TrafficPolicyCfg{},
				},
			}

			err := k8sClient.Create(ctx, cloudEndpoint)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("exactly one of inline or targetRef must be set on trafficPolicy"))
		})
	})

	Context("Metadata format", func() {
		It("should serialize map-form metadata to a sorted JSON object string for the ngrok API", func(ctx SpecContext) {
			cloudEndpoint = &ngrokv1.CloudEndpoint{
				Name:      "metadata-map-endpoint",
				Namespace: namespace,
				Spec: ngrokv1.CloudEndpointSpec{
					URL:      "https://metadata-map.internal",
					Metadata: map[string]string{"team": "platform", "owned-by": "ngrok-operator"},
				},
			}

			By("Creating the CloudEndpoint with map-form metadata")
			Expect(k8sClient.Create(ctx, cloudEndpoint)).To(Succeed())

			By("Verifying the ngrok API received the map serialized with sorted keys")
			Eventually(func(g Gomega) {
				obj := &ngrokv1.CloudEndpoint{}
				g.Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(cloudEndpoint), obj)).To(Succeed())
				g.Expect(obj.Status.ID).NotTo(BeEmpty())

				endpoint, err := mockClientset.Endpoints().Get(context.Background(), obj.Status.ID)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(endpoint.Metadata).To(Equal(`{"owned-by":"ngrok-operator","team":"platform"}`))
			}, timeout, interval).Should(Succeed())
		})
	})

	Context("Endpoint updates", func() {
		It("should successfully update endpoint description and metadata", func(ctx SpecContext) {
			cloudEndpoint = &ngrokv1.CloudEndpoint{
				Name:      "update-endpoint",
				Namespace: namespace,
				Spec: ngrokv1.CloudEndpointSpec{
					URL:         "https://update-test.internal",
					Description: "Original description",
					Metadata:    map[string]string{"key": "value"},
				},
			}

			By("Creating the CloudEndpoint")
			Expect(k8sClient.Create(ctx, cloudEndpoint)).To(Succeed())

			By("Waiting for initial creation")
			var endpointID string
			Eventually(func(g Gomega) {
				obj := &ngrokv1.CloudEndpoint{}
				g.Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(cloudEndpoint), obj)).To(Succeed())
				g.Expect(obj.Status.ID).NotTo(BeEmpty())
				endpointID = obj.Status.ID
			}, timeout, interval).Should(Succeed())

			By("Updating the description and metadata")
			Eventually(func(g Gomega) {
				obj := &ngrokv1.CloudEndpoint{}
				g.Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(cloudEndpoint), obj)).To(Succeed())

				obj.Spec.Description = "Updated description"
				obj.Spec.Metadata = map[string]string{"key": "updated", "new": "field"}
				g.Expect(k8sClient.Update(ctx, obj)).To(Succeed())
			}, timeout, interval).Should(Succeed())

			By("Verifying the update was applied")
			Eventually(func(g Gomega) {
				// Check in mock client
				endpoint, err := mockClientset.Endpoints().Get(context.Background(), endpointID)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(endpoint.Description).To(Equal("Updated description"))
				g.Expect(endpoint.Metadata).To(Equal(`{"key":"updated","new":"field"}`))
			}, timeout, interval).Should(Succeed())
		})

		It("should successfully update traffic policy", func(ctx SpecContext) {
			// Create initial traffic policy
			trafficPolicy1 := &ngrokv1alpha1.NgrokTrafficPolicy{
				Name:      "policy-v1",
				Namespace: namespace,
				Spec: ngrokv1alpha1.NgrokTrafficPolicySpec{
					Policy: []byte(`{"on_http_request":[{"name":"log"}]}`),
				},
			}
			Expect(k8sClient.Create(ctx, trafficPolicy1)).To(Succeed())

			cloudEndpoint = &ngrokv1.CloudEndpoint{
				Name:      "policy-update-endpoint",
				Namespace: namespace,
				Spec: ngrokv1.CloudEndpointSpec{
					URL:         "https://policy-update.internal",
					Description: "Endpoint with updatable policy",
					TrafficPolicy: &ngrokv1.TrafficPolicyCfg{
						Reference: &ngrokv1.K8sObjectRef{Name: "policy-v1"},
					},
				},
			}

			By("Creating the CloudEndpoint with initial policy")
			Expect(k8sClient.Create(ctx, cloudEndpoint)).To(Succeed())

			By("Waiting for initial creation")
			var endpointID string
			Eventually(func(g Gomega) {
				obj := &ngrokv1.CloudEndpoint{}
				g.Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(cloudEndpoint), obj)).To(Succeed())
				g.Expect(obj.Status.ID).NotTo(BeEmpty())
				endpointID = obj.Status.ID
			}, timeout, interval).Should(Succeed())

			By("Creating new traffic policy")
			trafficPolicy2 := &ngrokv1alpha1.NgrokTrafficPolicy{
				Name:      "policy-v2",
				Namespace: namespace,
				Spec: ngrokv1alpha1.NgrokTrafficPolicySpec{
					Policy: []byte(`{"on_http_request":[{"name":"rate-limit"}]}`),
				},
			}
			Expect(k8sClient.Create(ctx, trafficPolicy2)).To(Succeed())

			By("Updating to new traffic policy")
			Eventually(func(g Gomega) {
				obj := &ngrokv1.CloudEndpoint{}
				g.Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(cloudEndpoint), obj)).To(Succeed())

				obj.Spec.TrafficPolicy = &ngrokv1.TrafficPolicyCfg{
					Reference: &ngrokv1.K8sObjectRef{Name: "policy-v2"},
				}
				g.Expect(k8sClient.Update(ctx, obj)).To(Succeed())
			}, timeout, interval).Should(Succeed())

			By("Verifying the policy was updated")
			Eventually(func(g Gomega) {
				endpoint, err := mockClientset.Endpoints().Get(context.Background(), endpointID)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(endpoint.TrafficPolicy).To(ContainSubstring("rate-limit"))
			}, timeout, interval).Should(Succeed())
		})

		It("should recreate endpoint when update returns 404", func(ctx SpecContext) {
			cloudEndpoint = &ngrokv1.CloudEndpoint{
				Name:      "recreate-endpoint",
				Namespace: namespace,
				Spec: ngrokv1.CloudEndpointSpec{
					URL:         "https://recreate-test.internal",
					Description: "Endpoint to recreate",
				},
			}

			By("Creating the CloudEndpoint")
			Expect(k8sClient.Create(ctx, cloudEndpoint)).To(Succeed())

			By("Waiting for initial creation")
			var originalID string
			Eventually(func(g Gomega) {
				obj := &ngrokv1.CloudEndpoint{}
				g.Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(cloudEndpoint), obj)).To(Succeed())
				g.Expect(obj.Status.ID).NotTo(BeEmpty())
				originalID = obj.Status.ID
			}, timeout, interval).Should(Succeed())

			By("Manually deleting the endpoint from mock to simulate 404")
			err := mockClientset.Endpoints().Delete(context.Background(), originalID)
			Expect(err).NotTo(HaveOccurred())

			By("Updating the CloudEndpoint spec to trigger reconcile")
			Eventually(func(g Gomega) {
				obj := &ngrokv1.CloudEndpoint{}
				g.Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(cloudEndpoint), obj)).To(Succeed())

				obj.Spec.Description = "Updated after deletion"
				g.Expect(k8sClient.Update(ctx, obj)).To(Succeed())
			}, timeout, interval).Should(Succeed())

			By("Verifying a new endpoint was created")
			Eventually(func(g Gomega) {
				obj := &ngrokv1.CloudEndpoint{}
				g.Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(cloudEndpoint), obj)).To(Succeed())

				// Should have a new ID
				g.Expect(obj.Status.ID).NotTo(BeEmpty())

				// Verify the new endpoint exists in mock
				endpoint, err := mockClientset.Endpoints().Get(context.Background(), obj.Status.ID)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(endpoint.Description).To(Equal("Updated after deletion"))
			}, timeout, interval).Should(Succeed())
		})

		It("should update URL successfully", func(ctx SpecContext) {
			cloudEndpoint = &ngrokv1.CloudEndpoint{
				Name:      "url-update-endpoint",
				Namespace: namespace,
				Spec: ngrokv1.CloudEndpointSpec{
					URL:         "https://original-url.internal",
					Description: "Endpoint with updatable URL",
				},
			}

			By("Creating the CloudEndpoint")
			Expect(k8sClient.Create(ctx, cloudEndpoint)).To(Succeed())

			By("Waiting for initial creation")
			var endpointID string
			Eventually(func(g Gomega) {
				obj := &ngrokv1.CloudEndpoint{}
				g.Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(cloudEndpoint), obj)).To(Succeed())
				g.Expect(obj.Status.ID).NotTo(BeEmpty())
				endpointID = obj.Status.ID
			}, timeout, interval).Should(Succeed())

			By("Updating the URL")
			Eventually(func(g Gomega) {
				obj := &ngrokv1.CloudEndpoint{}
				g.Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(cloudEndpoint), obj)).To(Succeed())

				obj.Spec.URL = "https://updated-url.internal"
				g.Expect(k8sClient.Update(ctx, obj)).To(Succeed())
			}, timeout, interval).Should(Succeed())

			By("Verifying the URL was updated")
			Eventually(func(g Gomega) {
				endpoint, err := mockClientset.Endpoints().Get(context.Background(), endpointID)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(endpoint.URL).To(Equal("https://updated-url.internal"))
			}, timeout, interval).Should(Succeed())
		})

		It("should handle update errors gracefully", func(ctx SpecContext) {
			cloudEndpoint = &ngrokv1.CloudEndpoint{
				Name:      "update-error-endpoint",
				Namespace: namespace,
				Spec: ngrokv1.CloudEndpointSpec{
					URL:         "https://update-error.internal",
					Description: "Endpoint for error testing",
				},
			}

			By("Creating the CloudEndpoint")
			Expect(k8sClient.Create(ctx, cloudEndpoint)).To(Succeed())

			By("Waiting for initial creation")
			Eventually(func(g Gomega) {
				obj := &ngrokv1.CloudEndpoint{}
				g.Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(cloudEndpoint), obj)).To(Succeed())
				g.Expect(obj.Status.ID).NotTo(BeEmpty())
			}, timeout, interval).Should(Succeed())

			By("Setting mock to return error on update")
			mockClientset.Endpoints().(*nmockapi.EndpointsClient).SetUpdateError(errors.New("API update error"))

			By("Updating the CloudEndpoint")
			Eventually(func(g Gomega) {
				obj := &ngrokv1.CloudEndpoint{}
				g.Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(cloudEndpoint), obj)).To(Succeed())

				obj.Spec.Description = "This update should fail"
				g.Expect(k8sClient.Update(ctx, obj)).To(Succeed())
			}, timeout, interval).Should(Succeed())

			By("Verifying error condition is set")
			Eventually(func(g Gomega) {
				obj := &ngrokv1.CloudEndpoint{}
				g.Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(cloudEndpoint), obj)).To(Succeed())

				// Check that error condition is set
				cond := findCloudEndpointCondition(obj.Status.Conditions, cloudendpoint.ConditionCloudEndpointCreated)
				g.Expect(cond).NotTo(BeNil())
				g.Expect(cond.Status).To(Equal(metav1.ConditionFalse))
			}, timeout, interval).Should(Succeed())

			// Clean up error for other tests
			mockClientset.Endpoints().(*nmockapi.EndpointsClient).SetUpdateError(nil)
		})
	})

	Context("Domain handling", func() {
		It("should skip domain creation for Kubernetes-bound endpoint", func(ctx SpecContext) {
			cloudEndpoint = &ngrokv1.CloudEndpoint{
				Name:      "k8s-bound-endpoint",
				Namespace: namespace,
				Spec: ngrokv1.CloudEndpointSpec{
					URL:      "http://aws.demo",
					Bindings: []string{"kubernetes"},
				},
			}

			By("Creating the CloudEndpoint")
			Expect(k8sClient.Create(ctx, cloudEndpoint)).To(Succeed())

			By("Verifying endpoint becomes ready without domain creation")
			Eventually(func(g Gomega) {
				obj := &ngrokv1.CloudEndpoint{}
				g.Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(cloudEndpoint), obj)).To(Succeed())

				// Endpoint should be created
				g.Expect(obj.Status.ID).NotTo(BeEmpty())

				// No domain ref should be set (kubernetes binding skips domain)
				g.Expect(obj.Status.DomainRef).To(BeNil())

				// DomainReady condition should be True (no domain reservation needed)
				domainCond := findCloudEndpointCondition(obj.Status.Conditions, "DomainReady")
				g.Expect(domainCond).NotTo(BeNil())
				g.Expect(domainCond.Status).To(Equal(metav1.ConditionTrue))
				g.Expect(domainCond.Message).To(ContainSubstring("Kubernetes binding"))

				// Ready condition should be True (all conditions satisfied)
				readyCond := findCloudEndpointCondition(obj.Status.Conditions, cloudendpoint.ConditionCloudEndpointReady)
				g.Expect(readyCond).NotTo(BeNil())
				g.Expect(readyCond.Status).To(Equal(metav1.ConditionTrue))
			}, timeout, interval).Should(Succeed())

			By("Verifying no Domain CRD was created")
			domains := &ngrokv1.DomainList{}
			Expect(k8sClient.List(ctx, domains, client.InNamespace(namespace))).To(Succeed())
			Expect(domains.Items).To(BeEmpty())

			By("Verifying endpoint was created in mock client")
			Eventually(func(g Gomega) {
				obj := &ngrokv1.CloudEndpoint{}
				g.Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(cloudEndpoint), obj)).To(Succeed())
				g.Expect(obj.Status.ID).NotTo(BeEmpty())

				endpoint, err := mockClientset.Endpoints().Get(ctx, obj.Status.ID)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(endpoint.URL).To(Equal("http://aws.demo"))
			}, timeout, interval).Should(Succeed())
		})

		It("should clear stale domainRef for internal domain endpoint", func(ctx SpecContext) {
			staleDomainName := "stale-domain-ref"
			cloudEndpoint = &ngrokv1.CloudEndpoint{
				Name:      "internal-with-stale-ref",
				Namespace: namespace,
				Spec: ngrokv1.CloudEndpointSpec{
					URL: "http://test.internal",
				},
				Status: ngrokv1.CloudEndpointStatus{
					DomainRef: &ngrokv1.K8sObjectRefOptionalNamespace{
						Name:      staleDomainName,
						Namespace: &namespace,
					},
				},
			}

			By("Creating the CloudEndpoint with stale domainRef")
			Expect(k8sClient.Create(ctx, cloudEndpoint)).To(Succeed())

			By("Verifying controller clears the stale domainRef")
			Eventually(func(g Gomega) {
				obj := &ngrokv1.CloudEndpoint{}
				g.Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(cloudEndpoint), obj)).To(Succeed())

				g.Expect(obj.Status.DomainRef).To(BeNil())

				readyCond := findCloudEndpointCondition(obj.Status.Conditions, cloudendpoint.ConditionCloudEndpointReady)
				g.Expect(readyCond).NotTo(BeNil())
				g.Expect(readyCond.Status).To(Equal(metav1.ConditionTrue))
			}, timeout, interval).Should(Succeed())
		})

		It("should create endpoint even when domain is not ready", func(ctx SpecContext) {
			cloudEndpoint = &ngrokv1.CloudEndpoint{
				Name:      "test-endpoint",
				Namespace: namespace,
				Spec: ngrokv1.CloudEndpointSpec{
					URL: "http://example.com",
				},
			}

			By("Creating the CloudEndpoint")
			Expect(k8sClient.Create(ctx, cloudEndpoint)).To(Succeed())

			By("Waiting for controller to create Domain CR")
			var domain *ngrokv1.Domain
			Eventually(func(g Gomega) {
				domains := &ngrokv1.DomainList{}
				g.Expect(k8sClient.List(ctx, domains, client.InNamespace(namespace))).To(Succeed())
				g.Expect(domains.Items).To(HaveLen(1))
				domain = &domains.Items[0]
				g.Expect(domain.Spec.Domain).To(Equal("example.com"))
			}, timeout, interval).Should(Succeed())

			By("Waiting for endpoint to be created but not ready (domain not ready)")
			Eventually(func(g Gomega) {
				obj := &ngrokv1.CloudEndpoint{}
				g.Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(cloudEndpoint), obj)).To(Succeed())

				// Endpoint should be created
				g.Expect(obj.Status.ID).NotTo(BeEmpty())

				// DomainReady should be False (domain exists but not ready yet)
				domainCond := findCloudEndpointCondition(obj.Status.Conditions, "DomainReady")
				g.Expect(domainCond).NotTo(BeNil())
				g.Expect(domainCond.Status).To(Equal(metav1.ConditionFalse))

				// Ready should be False
				readyCond := findCloudEndpointCondition(obj.Status.Conditions, cloudendpoint.ConditionCloudEndpointReady)
				g.Expect(readyCond).NotTo(BeNil())
				g.Expect(readyCond.Status).To(Equal(metav1.ConditionFalse))
			}, timeout, interval).Should(Succeed())

			By("Making the domain ready")
			Eventually(func(g Gomega) {
				latestDomain := &ngrokv1.Domain{}
				g.Expect(k8sClient.Get(ctx, client.ObjectKey{Name: domain.Name, Namespace: namespace}, latestDomain)).To(Succeed())
				domain = latestDomain
			}, timeout, interval).Should(Succeed())

			// Update domain to be ready
			domain.Status.ID = "dom_123"
			domain.Status.Conditions = []metav1.Condition{
				{
					Type:               "Ready",
					Status:             metav1.ConditionTrue,
					Reason:             "DomainActive",
					Message:            "Domain is active",
					LastTransitionTime: metav1.Now(),
					ObservedGeneration: domain.Generation,
				},
			}
			Expect(k8sClient.Status().Update(ctx, domain)).To(Succeed())

			By("Verifying endpoint becomes ready after domain is ready")
			Eventually(func(g Gomega) {
				obj := &ngrokv1.CloudEndpoint{}
				g.Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(cloudEndpoint), obj)).To(Succeed())

				// DomainReady should be True
				domainCond := findCloudEndpointCondition(obj.Status.Conditions, "DomainReady")
				g.Expect(domainCond).NotTo(BeNil())
				g.Expect(domainCond.Status).To(Equal(metav1.ConditionTrue))

				// Ready should be True
				readyCond := findCloudEndpointCondition(obj.Status.Conditions, cloudendpoint.ConditionCloudEndpointReady)
				g.Expect(readyCond).NotTo(BeNil())
				g.Expect(readyCond.Status).To(Equal(metav1.ConditionTrue))
			}, 45*time.Second, interval).Should(Succeed())

			By("Verifying endpoint was created in mock client")
			Eventually(func(g Gomega) {
				obj := &ngrokv1.CloudEndpoint{}
				g.Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(cloudEndpoint), obj)).To(Succeed())
				g.Expect(obj.Status.ID).NotTo(BeEmpty())

				endpoint, err := mockClientset.Endpoints().Get(ctx, obj.Status.ID)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(endpoint.URL).To(Equal("http://example.com"))
			}, timeout, interval).Should(Succeed())
		})

		It("should not call the ngrok API update while domain is not ready", func(ctx SpecContext) {
			cloudEndpoint = &ngrokv1.CloudEndpoint{
				Name:      "no-unnecessary-updates",
				Namespace: namespace,
				Spec: ngrokv1.CloudEndpointSpec{
					URL: "http://no-update-spam.example.com",
				},
			}

			By("Creating the CloudEndpoint")
			Expect(k8sClient.Create(ctx, cloudEndpoint)).To(Succeed())

			By("Waiting for the endpoint to be created (Status.ID set)")
			Eventually(func(g Gomega) {
				obj := &ngrokv1.CloudEndpoint{}
				g.Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(cloudEndpoint), obj)).To(Succeed())
				g.Expect(obj.Status.ID).NotTo(BeEmpty())
			}, timeout, interval).Should(Succeed())

			By("Recording the update call count after initial creation")
			mockEndpoints := mockClientset.Endpoints().(*nmockapi.EndpointsClient)
			mockEndpoints.ResetUpdateCallCount()

			By("Waiting several requeue cycles to verify no spurious API updates")
			// The controller requeues every 10s when domain is not ready.
			// Wait long enough for multiple requeue cycles to have fired.
			Consistently(func() int {
				return mockEndpoints.UpdateCallCount()
			}, 5*time.Second, 500*time.Millisecond).Should(Equal(0))
		})

		It("should handle multiple Kubernetes-bound endpoints with different domains", func(ctx SpecContext) {
			// Use different URLs to avoid pooling issues in mock
			endpoints := []*ngrokv1.CloudEndpoint{
				{
					Name:      "k8s-endpoint-1",
					Namespace: namespace,
					Spec: ngrokv1.CloudEndpointSpec{
						URL:      "http://aws1.demo",
						Bindings: []string{"kubernetes"},
					},
				},
				{
					Name:      "k8s-endpoint-2",
					Namespace: namespace,
					Spec: ngrokv1.CloudEndpointSpec{
						URL:      "http://aws2.demo",
						Bindings: []string{"kubernetes"},
					},
				},
			}

			By("Creating multiple endpoints with same domain")
			for _, ep := range endpoints {
				Expect(k8sClient.Create(ctx, ep)).To(Succeed())
			}

			By("Verifying all endpoints become ready without domain conflicts")
			for _, ep := range endpoints {
				Eventually(func(g Gomega) {
					obj := &ngrokv1.CloudEndpoint{}
					g.Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(ep), obj)).To(Succeed())

					readyCond := findCloudEndpointCondition(obj.Status.Conditions, cloudendpoint.ConditionCloudEndpointReady)
					g.Expect(readyCond).NotTo(BeNil())
					g.Expect(readyCond.Status).To(Equal(metav1.ConditionTrue))
				}, timeout, interval).Should(Succeed())
			}

			By("Verifying no Domain CRD was created")
			domains := &ngrokv1.DomainList{}
			Expect(k8sClient.List(ctx, domains, client.InNamespace(namespace))).To(Succeed())
			Expect(domains.Items).To(BeEmpty())

			By("Verifying both endpoints were created in mock client")
			for _, ep := range endpoints {
				Eventually(func(g Gomega) {
					obj := &ngrokv1.CloudEndpoint{}
					g.Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(ep), obj)).To(Succeed())
					g.Expect(obj.Status.ID).NotTo(BeEmpty())

					endpoint, err := mockClientset.Endpoints().Get(ctx, obj.Status.ID)
					g.Expect(err).NotTo(HaveOccurred())
					g.Expect(endpoint.URL).To(Or(Equal("http://aws1.demo"), Equal("http://aws2.demo")))
				}, timeout, interval).Should(Succeed())
			}
		})
	})

	Context("Wildcard-covered domains", func() {
		var cloudEndpoint *ngrokv1.CloudEndpoint

		AfterEach(func(ctx SpecContext) {
			if cloudEndpoint != nil {
				_ = k8sClient.Delete(ctx, cloudEndpoint)
				cloudEndpoint = nil
			}
		})

		// A Domain whose reservation the Domain reconciler skipped in favor of a
		// wildcard parent has an empty status.id. This asserts the CloudEndpoint
		// controller treats that as usable: IsDomainReady must accept it, or
		// DomainResult.RequeueError() blocks the endpoint forever.
		It("should treat a wildcard-covered Domain as ready", func(ctx SpecContext) {
			hostname := "a.wildcard-covered.example.com"
			wildcard := "*.wildcard-covered.example.com"

			By("Creating a Domain that a wildcard already covers")
			domain := &ngrokv1.Domain{
				Name:      ngrokv1.HyphenatedDomainNameFromURL(hostname),
				Namespace: namespace,
				Spec: ngrokv1.DomainSpec{
					Domain: hostname,
				},
			}
			Expect(k8sClient.Create(ctx, domain)).To(Succeed())

			Eventually(func(g Gomega) {
				latest := &ngrokv1.Domain{}
				g.Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(domain), latest)).To(Succeed())
				domain = latest
			}, timeout, interval).Should(Succeed())

			// No reservation of its own: exactly the state the reconciler writes.
			domain.Status.ID = ""
			domain.Status.Domain = hostname
			domain.Status.CoveredByWildcardDomain = &ngrokv1.DomainStatusWildcardDomain{ID: "rd_wildcard", Domain: wildcard}
			domain.Status.Conditions = []metav1.Condition{
				{
					Type:               "Ready",
					Status:             metav1.ConditionTrue,
					Reason:             "CoveredByWildcardDomain",
					Message:            "Domain ready for use (covered by wildcard domain " + wildcard + ")",
					LastTransitionTime: metav1.Now(),
					ObservedGeneration: domain.Generation,
				},
			}
			Expect(k8sClient.Status().Update(ctx, domain)).To(Succeed())

			By("Creating a CloudEndpoint for the covered hostname")
			cloudEndpoint = &ngrokv1.CloudEndpoint{
				Name:      "wildcard-covered-endpoint",
				Namespace: namespace,
				Spec: ngrokv1.CloudEndpointSpec{
					URL: "https://" + hostname,
				},
			}
			Expect(k8sClient.Create(ctx, cloudEndpoint)).To(Succeed())

			By("Verifying the endpoint reports the domain ready and becomes ready itself")
			Eventually(func(g Gomega) {
				obj := &ngrokv1.CloudEndpoint{}
				g.Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(cloudEndpoint), obj)).To(Succeed())

				domainCond := findCloudEndpointCondition(obj.Status.Conditions, "DomainReady")
				g.Expect(domainCond).NotTo(BeNil())
				g.Expect(domainCond.Status).To(Equal(metav1.ConditionTrue))
				// The reason propagates from the Domain, so users see why no
				// reservation exists without inspecting the Domain CR.
				g.Expect(domainCond.Reason).To(Equal("CoveredByWildcardDomain"))

				readyCond := findCloudEndpointCondition(obj.Status.Conditions, cloudendpoint.ConditionCloudEndpointReady)
				g.Expect(readyCond).NotTo(BeNil())
				g.Expect(readyCond.Status).To(Equal(metav1.ConditionTrue))

				g.Expect(obj.Status.ID).NotTo(BeEmpty())
				g.Expect(obj.Status.DomainRef).NotTo(BeNil())
			}, timeout, interval).Should(Succeed())
		})
	})

	Context("Bindings validation", func() {
		It("should reject endpoint with multiple bindings", func() {
			// This should be caught by k8s validation (MaxItems=1), so we expect the Create to fail
			cloudEndpoint := &ngrokv1.CloudEndpoint{
				Name:      "invalid-multiple-bindings",
				Namespace: namespace,
				Spec: ngrokv1.CloudEndpointSpec{
					URL:      "http://test.demo",
					Bindings: []string{"public", "kubernetes"}, // Multiple bindings should be rejected
				},
			}

			err := k8sClient.Create(context.Background(), cloudEndpoint)
			Expect(err).To(HaveOccurred()) // Should be rejected by validation
			Expect(err.Error()).To(Or(
				ContainSubstring("must have at most 1 item"),
				ContainSubstring("maxItems"),
			))
		})

		It("should accept endpoint with single binding", func(ctx SpecContext) {
			cloudEndpoint := &ngrokv1.CloudEndpoint{
				Name:      "valid-single-binding",
				Namespace: namespace,
				Spec: ngrokv1.CloudEndpointSpec{
					URL:      "http://test-internal.demo",
					Bindings: []string{"internal"}, // Single binding is valid
				},
			}

			By("Creating the CloudEndpoint with single binding")
			err := k8sClient.Create(ctx, cloudEndpoint)
			Expect(err).NotTo(HaveOccurred()) // Should be accepted

			By("Verifying endpoint is created successfully")
			Eventually(func(g Gomega) {
				obj := &ngrokv1.CloudEndpoint{}
				g.Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(cloudEndpoint), obj)).To(Succeed())
				g.Expect(obj.Spec.Bindings).To(Equal([]string{"internal"}))
			}, timeout, interval).Should(Succeed())
		})
	})
})

// LEGACY-cloudendpoint-kind: delete this Describe at cleanup. A legacy object
// with no v1 twin is still reconciled as before the move.
var _ = Describe("CloudEndpoint Controller (legacy kind only)", func() {
	const (
		timeout  = 15 * time.Second
		interval = 500 * time.Millisecond
	)

	var namespace string

	BeforeEach(func(ctx SpecContext) {
		namespace = "test-" + rand.String(8)
		Expect(k8sClient.Create(ctx, &v1.Namespace{Name: namespace})).To(Succeed())
	})

	AfterEach(func(ctx SpecContext) {
		_ = k8sClient.Delete(ctx, &v1.Namespace{Name: namespace})
	})

	It("creates, updates and deletes its remote endpoint", func(ctx SpecContext) {
		url := fmt.Sprintf("https://legacy-only-%s.internal", rand.String(8))
		cloudEndpoint := &ngrokv1alpha1.CloudEndpoint{
			Name:      "legacy-only",
			Namespace: namespace,
			Spec: ngrokv1alpha1.CloudEndpointSpec{
				URL:         url,
				Description: "Original description",
				Metadata:    commonv1alpha1.MetadataFromLegacyString(`{"key":"value"}`),
			},
		}
		Expect(k8sClient.Create(ctx, cloudEndpoint)).To(Succeed())

		By("creating the remote endpoint")
		var endpointID string
		Eventually(func(g Gomega) {
			obj := &ngrokv1alpha1.CloudEndpoint{}
			g.Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(cloudEndpoint), obj)).To(Succeed())
			g.Expect(obj.Status.ID).NotTo(BeEmpty())
			cond := findCloudEndpointCondition(obj.Status.Conditions, cloudendpoint.ConditionCloudEndpointReady)
			g.Expect(cond).NotTo(BeNil())
			g.Expect(cond.Status).To(Equal(metav1.ConditionTrue))
			g.Expect(cond.Reason).To(Equal(cloudendpoint.ReasonCloudEndpointActive))
			endpointID = obj.Status.ID
		}, timeout, interval).Should(Succeed())

		By("pushing spec updates")
		Eventually(func(g Gomega) {
			obj := &ngrokv1alpha1.CloudEndpoint{}
			g.Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(cloudEndpoint), obj)).To(Succeed())
			obj.Spec.Description = "Updated description"
			g.Expect(k8sClient.Update(ctx, obj)).To(Succeed())
		}, timeout, interval).Should(Succeed())
		Eventually(func(g Gomega) {
			endpoint, err := mockClientset.Endpoints().Get(ctx, endpointID)
			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(endpoint.Description).To(Equal("Updated description"))
		}, timeout, interval).Should(Succeed())

		By("deleting the remote endpoint with the object")
		Expect(k8sClient.Delete(ctx, cloudEndpoint)).To(Succeed())
		Eventually(func() bool {
			return apierrors.IsNotFound(k8sClient.Get(ctx, client.ObjectKeyFromObject(cloudEndpoint), &ngrokv1alpha1.CloudEndpoint{}))
		}, timeout, interval).Should(BeTrue())
		_, err := mockClientset.Endpoints().Get(ctx, endpointID)
		Expect(ngrok.IsNotFound(err)).To(BeTrue())
	})

	// LEGACY-metadata-format: BEGIN — delete this test in the cleanup release
	// once the raw JSON string form is no longer accepted.
	It("should pass legacy string-form metadata through to the ngrok API verbatim", func(ctx SpecContext) {
		cloudEndpoint := &ngrokv1alpha1.CloudEndpoint{
			Name:      "metadata-legacy-endpoint",
			Namespace: namespace,
			Spec: ngrokv1alpha1.CloudEndpointSpec{
				URL:      "https://metadata-legacy.internal",
				Metadata: commonv1alpha1.MetadataFromLegacyString(`{"owned-by":"ngrok-operator"}`),
			},
		}

		By("Creating the CloudEndpoint with legacy string-form metadata")
		Expect(k8sClient.Create(ctx, cloudEndpoint)).To(Succeed())

		By("Verifying the ngrok API received the string verbatim")
		Eventually(func(g Gomega) {
			obj := &ngrokv1alpha1.CloudEndpoint{}
			g.Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(cloudEndpoint), obj)).To(Succeed())
			g.Expect(obj.Status.ID).NotTo(BeEmpty())

			endpoint, err := mockClientset.Endpoints().Get(context.Background(), obj.Status.ID)
			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(endpoint.Metadata).To(Equal(`{"owned-by":"ngrok-operator"}`))
		}, timeout, interval).Should(Succeed())
	})
	// LEGACY-metadata-format: END
})

// LEGACY-cloudendpoint-kind: delete this Describe at cleanup.
var _ = Describe("CloudEndpoint migration", func() {
	const (
		timeout  = 15 * time.Second
		interval = 250 * time.Millisecond
	)

	var namespace string

	BeforeEach(func(ctx SpecContext) {
		namespace = "test-" + rand.String(8)
		Expect(k8sClient.Create(ctx, &v1.Namespace{Name: namespace})).To(Succeed())
	})

	AfterEach(func(ctx SpecContext) {
		_ = k8sClient.Delete(ctx, &v1.Namespace{Name: namespace})
	})

	mockEndpoints := func() *nmockapi.EndpointsClient {
		return mockClientset.Endpoints().(*nmockapi.EndpointsClient)
	}

	// Each spec uses its own URL, so calls made for other specs' leftover
	// objects don't affect the counts.
	uniqueURL := func(name string) string {
		return fmt.Sprintf("https://%s-%s.internal", name, rand.String(8))
	}

	callsTo := func(method, url string) func() int {
		return func() int {
			n := 0
			for _, c := range mockEndpoints().Calls() {
				if c.Method == method && c.URL == url {
					n++
				}
			}
			return n
		}
	}

	deletesOf := func(id string) func() int {
		return func() int {
			n := 0
			for _, c := range mockEndpoints().Calls() {
				if c.Method == "Delete" && c.ID == id {
					n++
				}
			}
			return n
		}
	}

	remoteExists := func(ctx context.Context, id string) func() bool {
		return func() bool {
			_, err := mockClientset.Endpoints().Get(ctx, id)
			return err == nil
		}
	}

	newLegacy := func(name, url string) *ngrokv1alpha1.CloudEndpoint {
		return &ngrokv1alpha1.CloudEndpoint{
			Name: name, Namespace: namespace,
			Spec: ngrokv1alpha1.CloudEndpointSpec{URL: url, Description: "migration"},
		}
	}

	newV1 := func(name, url string) *ngrokv1.CloudEndpoint {
		return &ngrokv1.CloudEndpoint{
			Name: name, Namespace: namespace,
			Spec: ngrokv1.CloudEndpointSpec{URL: url, Description: "migration"},
		}
	}

	fetch := func(ctx context.Context, g Gomega, ep cloudendpoint.Resource) cloudendpoint.Resource {
		obj := ep.DeepCopyObject().(cloudendpoint.Resource)
		g.Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(ep), obj)).To(Succeed())
		return obj
	}

	idOf := func(ctx context.Context, ep cloudendpoint.Resource) func(g Gomega) string {
		return func(g Gomega) string {
			return fetch(ctx, g, ep).GetID()
		}
	}

	readyReason := func(ctx context.Context, ep cloudendpoint.Resource) func(g Gomega) string {
		return func(g Gomega) string {
			c := meta.FindStatusCondition(*fetch(ctx, g, ep).GetConditions(), cloudendpoint.ConditionCloudEndpointReady)
			g.Expect(c).NotTo(BeNil())
			g.Expect(c.Status).To(Equal(metav1.ConditionTrue))
			return c.Reason
		}
	}

	gone := func(ctx context.Context, obj client.Object) func() bool {
		return func() bool {
			return apierrors.IsNotFound(k8sClient.Get(ctx, client.ObjectKeyFromObject(obj), obj.DeepCopyObject().(client.Object)))
		}
	}

	// eventsOn counts the events with reason recorded on obj, matching its
	// kind's API group so a pair's events are told apart.
	eventsOn := func(ctx context.Context, obj client.Object, apiVersion, reason string) func(g Gomega) int {
		return func(g Gomega) int {
			var list eventsv1.EventList
			g.Expect(k8sClient.List(ctx, &list, client.InNamespace(obj.GetNamespace()))).To(Succeed())
			n := 0
			for _, e := range list.Items {
				if e.Regarding.Name == obj.GetName() && e.Regarding.APIVersion == apiVersion && e.Reason == reason {
					n++
				}
			}
			return n
		}
	}
	const (
		v1APIVersion     = "ngrok.com/v1"
		legacyAPIVersion = "ngrok.k8s.ngrok.com/v1alpha1"
	)

	annotate := func(ctx context.Context, ep cloudendpoint.Resource) {
		Eventually(func(g Gomega) {
			obj := fetch(ctx, g, ep)
			obj.SetAnnotations(map[string]string{"test/poke": rand.String(8)})
			g.Expect(k8sClient.Update(ctx, obj)).To(Succeed())
		}, timeout, interval).Should(Succeed())
	}

	// twinned creates a legacy object, waits for its remote endpoint, then
	// applies its v1 twin and waits for the hand-over. It returns the shared ID.
	twinned := func(ctx context.Context, legacy *ngrokv1alpha1.CloudEndpoint, canonical *ngrokv1.CloudEndpoint) string {
		Expect(k8sClient.Create(ctx, legacy)).To(Succeed())
		Eventually(idOf(ctx, legacy), timeout, interval).ShouldNot(BeEmpty())
		id := idOf(ctx, legacy)(Default)

		Expect(k8sClient.Create(ctx, canonical)).To(Succeed())
		Eventually(idOf(ctx, canonical), timeout, interval).Should(Equal(id))
		Eventually(readyReason(ctx, legacy), timeout, interval).Should(Equal(cloudendpoint.ReasonSupersededByV1))
		Eventually(readyReason(ctx, canonical), timeout, interval).Should(Equal(cloudendpoint.ReasonCloudEndpointActive))
		return id
	}

	It("creates one endpoint for a v1 object on its own", func(ctx SpecContext) {
		url := uniqueURL("mig-v1-only")
		canonical := newV1("mig-v1-only", url)
		Expect(k8sClient.Create(ctx, canonical)).To(Succeed())

		Eventually(readyReason(ctx, canonical), timeout, interval).Should(Equal(cloudendpoint.ReasonCloudEndpointActive))
		id := idOf(ctx, canonical)(Default)
		Expect(id).NotTo(BeEmpty())
		Expect(remoteExists(ctx, id)()).To(BeTrue())
		Consistently(callsTo("Create", url), 2*time.Second, interval).Should(Equal(1))
	})

	It("adopts the legacy endpoint when v1 is applied over it", func(ctx SpecContext) {
		url := uniqueURL("mig-adopt")
		legacy, canonical := newLegacy("mig-adopt", url), newV1("mig-adopt", url)
		canonical.Spec.Description = "written by v1"
		id := twinned(ctx, legacy, canonical)

		Eventually(eventsOn(ctx, canonical, v1APIVersion, EventCloudEndpointAdopted), timeout, interval).Should(BeNumerically(">=", 1))
		Eventually(eventsOn(ctx, legacy, legacyAPIVersion, trafficpolicypkg.EventDeprecatedAPIGroup), timeout, interval).Should(BeNumerically(">=", 1))
		Consistently(callsTo("Create", url), 2*time.Second, interval).Should(Equal(1))

		By("writing only the v1 spec to the shared endpoint")
		annotate(ctx, legacy)
		Consistently(func(g Gomega) string {
			endpoint, err := mockClientset.Endpoints().Get(ctx, id)
			g.Expect(err).NotTo(HaveOccurred())
			return endpoint.Description
		}, 2*time.Second, interval).Should(Equal("written by v1"))
	})

	It("doesn't warn about the deprecated group on an operator-generated legacy object", func(ctx SpecContext) {
		url := uniqueURL("mig-generated")
		legacy, canonical := newLegacy("mig-generated", url), newV1("mig-generated", url)
		legacy.Labels = map[string]string{labels.ControllerName: controllerName}
		twinned(ctx, legacy, canonical)

		Consistently(eventsOn(ctx, legacy, legacyAPIVersion, trafficpolicypkg.EventDeprecatedAPIGroup), 2*time.Second, interval).Should(BeZero())
	})

	It("keeps the endpoint when the twinned legacy object is deleted", func(ctx SpecContext) {
		url := uniqueURL("mig-delete-legacy")
		legacy, canonical := newLegacy("mig-delete-legacy", url), newV1("mig-delete-legacy", url)
		id := twinned(ctx, legacy, canonical)

		Expect(k8sClient.Delete(ctx, legacy)).To(Succeed())
		Eventually(gone(ctx, legacy), timeout, interval).Should(BeTrue())

		Consistently(deletesOf(id), 2*time.Second, interval).Should(BeZero())
		Expect(remoteExists(ctx, id)()).To(BeTrue())
		Expect(idOf(ctx, canonical)(Default)).To(Equal(id))
	})

	It("keeps the endpoint when v1 is applied and legacy deleted back to back", func(ctx SpecContext) {
		url := uniqueURL("mig-race")
		legacy, canonical := newLegacy("mig-race", url), newV1("mig-race", url)
		Expect(k8sClient.Create(ctx, legacy)).To(Succeed())
		Eventually(idOf(ctx, legacy), timeout, interval).ShouldNot(BeEmpty())
		id := idOf(ctx, legacy)(Default)

		Expect(k8sClient.Create(ctx, canonical)).To(Succeed())
		Expect(k8sClient.Delete(ctx, legacy)).To(Succeed())

		Eventually(gone(ctx, legacy), timeout, interval).Should(BeTrue())
		Eventually(idOf(ctx, canonical), timeout, interval).Should(Equal(id))
		Consistently(deletesOf(id), 2*time.Second, interval).Should(BeZero())
		Expect(callsTo("Create", url)()).To(Equal(1))
	})

	It("hands the endpoint back to legacy when the twinned v1 object is deleted", func(ctx SpecContext) {
		url := uniqueURL("mig-delete-v1")
		legacy, canonical := newLegacy("mig-delete-v1", url), newV1("mig-delete-v1", url)
		id := twinned(ctx, legacy, canonical)

		Expect(k8sClient.Delete(ctx, canonical)).To(Succeed())
		Eventually(gone(ctx, canonical), timeout, interval).Should(BeTrue())

		By("the legacy object becoming the writer again")
		Eventually(readyReason(ctx, legacy), timeout, interval).Should(Equal(cloudendpoint.ReasonCloudEndpointActive))
		Expect(idOf(ctx, legacy)(Default)).To(Equal(id))
		Eventually(func(g Gomega) {
			obj := fetch(ctx, g, legacy).(*ngrokv1alpha1.CloudEndpoint)
			obj.Spec.Description = "written by legacy"
			g.Expect(k8sClient.Update(ctx, obj)).To(Succeed())
		}, timeout, interval).Should(Succeed())
		Eventually(func(g Gomega) {
			endpoint, err := mockClientset.Endpoints().Get(ctx, id)
			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(endpoint.Description).To(Equal("written by legacy"))
		}, timeout, interval).Should(Succeed())

		Expect(deletesOf(id)()).To(BeZero())
		Expect(callsTo("Create", url)()).To(Equal(1))
	})

	It("deletes the shared endpoint exactly once when both objects are deleted", func(ctx SpecContext) {
		url := uniqueURL("mig-delete-both")
		legacy, canonical := newLegacy("mig-delete-both", url), newV1("mig-delete-both", url)
		id := twinned(ctx, legacy, canonical)

		Expect(k8sClient.Delete(ctx, legacy)).To(Succeed())
		Expect(k8sClient.Delete(ctx, canonical)).To(Succeed())
		Eventually(gone(ctx, legacy), timeout, interval).Should(BeTrue())
		Eventually(gone(ctx, canonical), timeout, interval).Should(BeTrue())

		Eventually(remoteExists(ctx, id), timeout, interval).Should(BeFalse())
		Consistently(deletesOf(id), 2*time.Second, interval).Should(Equal(1))
	})

	It("reports twins holding different IDs and deletes neither endpoint", func(ctx SpecContext) {
		url := uniqueURL("mig-conflict")
		canonical := newV1("mig-conflict", url)
		Expect(k8sClient.Create(ctx, canonical)).To(Succeed())
		Eventually(idOf(ctx, canonical), timeout, interval).ShouldNot(BeEmpty())
		v1ID := idOf(ctx, canonical)(Default)

		By("giving a legacy twin its own remote endpoint")
		other, err := mockEndpoints().Create(ctx, &ngrok.EndpointCreate{Type: "cloud", URL: uniqueURL("mig-conflict-other")})
		Expect(err).NotTo(HaveOccurred())
		legacy := newLegacy("mig-conflict", url)
		Expect(k8sClient.Create(ctx, legacy)).To(Succeed())
		Eventually(readyReason(ctx, legacy), timeout, interval).Should(Equal(cloudendpoint.ReasonSupersededByV1))
		Eventually(func(g Gomega) {
			obj := fetch(ctx, g, legacy)
			obj.SetID(other.ID)
			g.Expect(k8sClient.Status().Update(ctx, obj)).To(Succeed())
		}, timeout, interval).Should(Succeed())
		annotate(ctx, legacy)

		Eventually(eventsOn(ctx, canonical, v1APIVersion, EventCloudEndpointIDConflict), timeout, interval).Should(BeNumerically(">=", 1))
		Eventually(eventsOn(ctx, legacy, legacyAPIVersion, EventCloudEndpointIDConflict), timeout, interval).Should(BeNumerically(">=", 1))

		Consistently(func() int { return deletesOf(v1ID)() + deletesOf(other.ID)() }, 2*time.Second, interval).Should(BeZero())
		Expect(idOf(ctx, canonical)(Default)).To(Equal(v1ID))
		Expect(idOf(ctx, legacy)(Default)).To(Equal(other.ID))
		Expect(readyReason(ctx, legacy)(Default)).To(Equal(cloudendpoint.ReasonSupersededByV1))
	})

	It("moves both twins to one new endpoint after an out-of-band remote delete", func(ctx SpecContext) {
		url := uniqueURL("mig-oob")
		legacy, canonical := newLegacy("mig-oob", url), newV1("mig-oob", url)
		id := twinned(ctx, legacy, canonical)

		Expect(mockEndpoints().Delete(ctx, id)).To(Succeed())
		annotate(ctx, canonical)

		Eventually(idOf(ctx, canonical), timeout, interval).ShouldNot(Or(BeEmpty(), Equal(id)))
		newID := idOf(ctx, canonical)(Default)
		Eventually(idOf(ctx, legacy), timeout, interval).Should(Equal(newID))
		Expect(remoteExists(ctx, newID)()).To(BeTrue())
		Expect(readyReason(ctx, legacy)(Default)).To(Equal(cloudendpoint.ReasonSupersededByV1))

		By("creating exactly one replacement, without re-adopting the dead ID")
		Consistently(callsTo("Create", url), 3*time.Second, interval).Should(Equal(2))
		Expect(idOf(ctx, canonical)(Default)).To(Equal(newID))
		Expect(idOf(ctx, legacy)(Default)).To(Equal(newID))
	})

	It("adopts the legacy endpoint when v1's own endpoint is gone", func(ctx SpecContext) {
		// Roll-forward after a rollback: the old operator created endpoint Z
		// for the legacy object, and the one v1 still holds was deleted.
		url := uniqueURL("mig-rollforward")
		canonical := newV1("mig-rollforward", url)
		Expect(k8sClient.Create(ctx, canonical)).To(Succeed())
		Eventually(idOf(ctx, canonical), timeout, interval).ShouldNot(BeEmpty())
		deadID := idOf(ctx, canonical)(Default)

		legacy := newLegacy("mig-rollforward", url)
		Expect(k8sClient.Create(ctx, legacy)).To(Succeed())
		Eventually(readyReason(ctx, legacy), timeout, interval).Should(Equal(cloudendpoint.ReasonSupersededByV1))

		By("replacing v1's endpoint with one only the legacy object holds")
		Expect(mockEndpoints().Delete(ctx, deadID)).To(Succeed())
		legacyEndpoint, err := mockEndpoints().Create(ctx, &ngrok.EndpointCreate{Type: "cloud", URL: url})
		Expect(err).NotTo(HaveOccurred())
		Eventually(func(g Gomega) {
			obj := fetch(ctx, g, legacy)
			obj.SetID(legacyEndpoint.ID)
			g.Expect(k8sClient.Status().Update(ctx, obj)).To(Succeed())
		}, timeout, interval).Should(Succeed())
		creates := callsTo("Create", url)()
		annotate(ctx, canonical)

		Eventually(idOf(ctx, canonical), timeout, interval).Should(Equal(legacyEndpoint.ID))
		Eventually(eventsOn(ctx, canonical, v1APIVersion, EventCloudEndpointAdopted), timeout, interval).Should(BeNumerically(">=", 1))
		Consistently(callsTo("Create", url), 2*time.Second, interval).Should(Equal(creates))
		Expect(idOf(ctx, legacy)(Default)).To(Equal(legacyEndpoint.ID))
		Expect(readyReason(ctx, legacy)(Default)).To(Equal(cloudendpoint.ReasonSupersededByV1))
		Expect(eventsOn(ctx, canonical, v1APIVersion, EventCloudEndpointIDConflict)(Default)).To(BeZero())
	})

	It("recreates the endpoint without adopting when both twins' endpoints are gone", func(ctx SpecContext) {
		url := uniqueURL("mig-both-dead")
		canonical := newV1("mig-both-dead", url)
		Expect(k8sClient.Create(ctx, canonical)).To(Succeed())
		Eventually(idOf(ctx, canonical), timeout, interval).ShouldNot(BeEmpty())
		deadID := idOf(ctx, canonical)(Default)

		legacy := newLegacy("mig-both-dead", url)
		Expect(k8sClient.Create(ctx, legacy)).To(Succeed())
		Eventually(readyReason(ctx, legacy), timeout, interval).Should(Equal(cloudendpoint.ReasonSupersededByV1))

		By("deleting v1's endpoint and pointing legacy at another dead one")
		Expect(mockEndpoints().Delete(ctx, deadID)).To(Succeed())
		Eventually(func(g Gomega) {
			obj := fetch(ctx, g, legacy)
			obj.SetID("ep_dead" + rand.String(8))
			g.Expect(k8sClient.Status().Update(ctx, obj)).To(Succeed())
		}, timeout, interval).Should(Succeed())
		annotate(ctx, canonical)

		Eventually(idOf(ctx, canonical), timeout, interval).ShouldNot(Or(BeEmpty(), Equal(deadID)))
		newID := idOf(ctx, canonical)(Default)
		Expect(remoteExists(ctx, newID)()).To(BeTrue())
		Eventually(idOf(ctx, legacy), timeout, interval).Should(Equal(newID))
		Consistently(eventsOn(ctx, canonical, v1APIVersion, EventCloudEndpointAdopted), 2*time.Second, interval).Should(BeZero())
		Expect(callsTo("Create", url)()).To(Equal(2))
	})

	It("moves a legacy object holding a dead ID onto v1's endpoint on a later pass", func(ctx SpecContext) {
		// The state left when legacy's follow write failed after v1 replaced
		// an endpoint deleted outside the operator.
		url := uniqueURL("mig-dead-legacy")
		canonical := newV1("mig-dead-legacy", url)
		Expect(k8sClient.Create(ctx, canonical)).To(Succeed())
		Eventually(idOf(ctx, canonical), timeout, interval).ShouldNot(BeEmpty())
		id := idOf(ctx, canonical)(Default)

		legacy := newLegacy("mig-dead-legacy", url)
		Expect(k8sClient.Create(ctx, legacy)).To(Succeed())
		Eventually(readyReason(ctx, legacy), timeout, interval).Should(Equal(cloudendpoint.ReasonSupersededByV1))
		Eventually(func(g Gomega) {
			obj := fetch(ctx, g, legacy)
			obj.SetID("ep_dead" + rand.String(8))
			g.Expect(k8sClient.Status().Update(ctx, obj)).To(Succeed())
		}, timeout, interval).Should(Succeed())
		annotate(ctx, legacy)

		Eventually(idOf(ctx, legacy), timeout, interval).Should(Equal(id))
		Consistently(func() int {
			return eventsOn(ctx, canonical, v1APIVersion, EventCloudEndpointIDConflict)(Default) +
				eventsOn(ctx, legacy, legacyAPIVersion, EventCloudEndpointIDConflict)(Default)
		}, 2*time.Second, interval).Should(BeZero())
		Expect(idOf(ctx, canonical)(Default)).To(Equal(id))
		Expect(callsTo("Create", url)()).To(Equal(1))
		Expect(deletesOf(id)()).To(BeZero())
	})

	It("keeps one endpoint per name on a pooled URL", func(ctx SpecContext) {
		url := uniqueURL("mig-pooled")
		legacyA, canonicalA, canonicalB := newLegacy("a", url), newV1("a", url), newV1("b", url)
		legacyA.Spec.PoolingEnabled = new(true)
		canonicalA.Spec.PoolingEnabled = new(true)
		canonicalB.Spec.PoolingEnabled = new(true)
		idA := twinned(ctx, legacyA, canonicalA)

		Expect(k8sClient.Create(ctx, canonicalB)).To(Succeed())
		Eventually(readyReason(ctx, canonicalB), timeout, interval).Should(Equal(cloudendpoint.ReasonCloudEndpointActive))
		idB := idOf(ctx, canonicalB)(Default)

		Expect(idB).NotTo(Or(BeEmpty(), Equal(idA)))
		Expect(idOf(ctx, canonicalA)(Default)).To(Equal(idA))
		Expect(remoteExists(ctx, idA)()).To(BeTrue())
		Expect(remoteExists(ctx, idB)()).To(BeTrue())
		Consistently(callsTo("Create", url), 2*time.Second, interval).Should(Equal(2))
	})

	It("makes no update call when the twins' metadata differs only in encoding", func(ctx SpecContext) {
		url := uniqueURL("mig-metadata")
		legacy, canonical := newLegacy("mig-metadata", url), newV1("mig-metadata", url)
		legacy.Spec.Metadata = commonv1alpha1.MetadataFromLegacyString(`{"team": "edge", "app": "x"}`)
		canonical.Spec.Metadata = map[string]string{"app": "x", "team": "edge"}
		id := twinned(ctx, legacy, canonical)

		endpoint, err := mockClientset.Endpoints().Get(ctx, id)
		Expect(err).NotTo(HaveOccurred())
		Expect(endpoint.Metadata).To(Equal(`{"team": "edge", "app": "x"}`))
		Consistently(callsTo("Update", url), 2*time.Second, interval).Should(BeZero())
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
