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

	ingressv1alpha1 "github.com/ngrok/ngrok-operator/api/ingress/v1alpha1"
	ngrokv1alpha1 "github.com/ngrok/ngrok-operator/api/ngrok/v1alpha1"
	"github.com/ngrok/ngrok-operator/internal/errors"
	"github.com/ngrok/ngrok-operator/internal/mocks/nmockapi"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	v1 "k8s.io/api/core/v1"
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

	BeforeEach(func() {
		namespace = "test-" + rand.String(8)

		// Create namespace for testing
		ns := &v1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: namespace,
			},
		}
		Expect(k8sClient.Create(context.Background(), ns)).To(Succeed())

		// Reset mock endpoints client for each test
		mockClientset.Endpoints().(*nmockapi.EndpointsClient).Reset()
	})

	AfterEach(func() {
		// Clean up namespace
		if namespace != "" {
			ns := &v1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: namespace,
				},
			}
			_ = k8sClient.Delete(context.Background(), ns)
		}
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
				g.Expect(obj.Status.AssignedURL).To(Equal("https://test.internal"))

				// Check ready condition
				cond := findCloudEndpointCondition(obj.Status.Conditions, ConditionCloudEndpointReady)
				g.Expect(cond).NotTo(BeNil())
				g.Expect(cond.Status).To(Equal(metav1.ConditionTrue))
			}, timeout, interval).Should(Succeed())
		})

		It("should successfully create a cloud endpoint with TCP URL", func(ctx SpecContext) {
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

			By("Waiting for controller to reconcile")
			Eventually(func(g Gomega) {
				obj := &ngrokv1alpha1.CloudEndpoint{}
				g.Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(cloudEndpoint), obj)).To(Succeed())

				// Check that endpoint was created
				g.Expect(obj.Status.ID).NotTo(BeEmpty())
			}, timeout, interval).Should(Succeed())

			By("Verifying no domain CR was created for TCP endpoint")
			Eventually(func(g Gomega) {
				domains := &ingressv1alpha1.DomainList{}
				g.Expect(k8sClient.List(ctx, domains, client.InNamespace(namespace))).To(Succeed())
				g.Expect(domains.Items).To(BeEmpty())
			}, timeout, interval).Should(Succeed())

			By("Verifying no domain ref is set")
			Eventually(func(g Gomega) {
				obj := &ngrokv1alpha1.CloudEndpoint{}
				g.Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(cloudEndpoint), obj)).To(Succeed())
				g.Expect(obj.Status.DomainRef).To(BeNil())
			}, timeout, interval).Should(Succeed())
		})

		It("should not create domain CR for custom TCP endpoint", func(ctx SpecContext) {
			cloudEndpoint = &ngrokv1alpha1.CloudEndpoint{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "tcp-custom-endpoint",
					Namespace: namespace,
				},
				Spec: ngrokv1alpha1.CloudEndpointSpec{
					URL:         "tcp://1.2.3.4:25565",
					Description: "Custom TCP test endpoint",
					Metadata:    "{}",
				},
			}

			By("Creating the CloudEndpoint")
			Expect(k8sClient.Create(ctx, cloudEndpoint)).To(Succeed())

			By("Waiting for controller to reconcile")
			Eventually(func(g Gomega) {
				obj := &ngrokv1alpha1.CloudEndpoint{}
				g.Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(cloudEndpoint), obj)).To(Succeed())
				g.Expect(obj.Status.ID).NotTo(BeEmpty())
			}, timeout, interval).Should(Succeed())

			By("Verifying no domain CR was created for custom TCP endpoint")
			Eventually(func(g Gomega) {
				domains := &ingressv1alpha1.DomainList{}
				g.Expect(k8sClient.List(ctx, domains, client.InNamespace(namespace))).To(Succeed())
				g.Expect(domains.Items).To(BeEmpty())
			}, timeout, interval).Should(Succeed())

			By("Verifying no domain ref is set")
			Eventually(func(g Gomega) {
				obj := &ngrokv1alpha1.CloudEndpoint{}
				g.Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(cloudEndpoint), obj)).To(Succeed())
				g.Expect(obj.Status.DomainRef).To(BeNil())
			}, timeout, interval).Should(Succeed())
		})

		It("R1: legacy spec.trafficPolicyName-only manifest resolves via in-memory normalization", func(ctx SpecContext) {
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

			By("Creating the CloudEndpoint with deprecated trafficPolicyName")
			Expect(k8sClient.Create(ctx, cloudEndpoint)).To(Succeed())

			By("Waiting for controller to reconcile")
			Eventually(func(g Gomega) {
				obj := &ngrokv1alpha1.CloudEndpoint{}
				g.Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(cloudEndpoint), obj)).To(Succeed())

				// Check that endpoint was created
				g.Expect(obj.Status.ID).NotTo(BeEmpty())
			}, timeout, interval).Should(Succeed())
		})

		It("should handle endpoint with trafficPolicy.targetRef", func(ctx SpecContext) {
			trafficPolicy := &ngrokv1alpha1.NgrokTrafficPolicy{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "new-shape-policy",
					Namespace: namespace,
				},
				Spec: ngrokv1alpha1.NgrokTrafficPolicySpec{
					Policy: []byte(`{"on_http_request":[{"name":"rate-limit"}]}`),
				},
			}
			Expect(k8sClient.Create(ctx, trafficPolicy)).To(Succeed())

			cloudEndpoint = &ngrokv1alpha1.CloudEndpoint{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "new-shape-endpoint",
					Namespace: namespace,
				},
				Spec: ngrokv1alpha1.CloudEndpointSpec{
					URL: "https://new-shape.internal",
					TrafficPolicy: &ngrokv1alpha1.CloudEndpointTrafficPolicyCfg{
						Reference: &ngrokv1alpha1.K8sObjectRef{
							Name: "new-shape-policy",
						},
					},
				},
			}

			Expect(k8sClient.Create(ctx, cloudEndpoint)).To(Succeed())

			Eventually(func(g Gomega) {
				obj := &ngrokv1alpha1.CloudEndpoint{}
				g.Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(cloudEndpoint), obj)).To(Succeed())
				g.Expect(obj.Status.ID).NotTo(BeEmpty())

				tpCond := findCloudEndpointCondition(obj.Status.Conditions, ConditionTrafficPolicy)
				g.Expect(tpCond).NotTo(BeNil())
				g.Expect(tpCond.Status).To(Equal(metav1.ConditionTrue))
			}, timeout, interval).Should(Succeed())
		})

		It("R1: legacy spec.trafficPolicy.policy-only manifest resolves via the fold path", func(ctx SpecContext) {
			// Verifies the LEGACY-trafficpolicy-policy fold in
			// CloudEndpointTrafficPolicyCfg.ToTrafficPolicyCfg: when only
			// the deprecated `policy` field is set, the resolver still
			// produces the policy JSON. Cleanup release removes this test.
			cloudEndpoint = &ngrokv1alpha1.CloudEndpoint{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "legacy-policy-endpoint",
					Namespace: namespace,
				},
				Spec: ngrokv1alpha1.CloudEndpointSpec{
					URL: "https://legacy-policy.internal",
					TrafficPolicy: &ngrokv1alpha1.CloudEndpointTrafficPolicyCfg{
						Policy: json.RawMessage(`{"on_http_request":[{"name":"legacy"}]}`),
					},
				},
			}
			Expect(k8sClient.Create(ctx, cloudEndpoint)).To(Succeed())

			var endpointID string
			Eventually(func(g Gomega) {
				obj := &ngrokv1alpha1.CloudEndpoint{}
				g.Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(cloudEndpoint), obj)).To(Succeed())
				g.Expect(obj.Status.ID).NotTo(BeEmpty())
				endpointID = obj.Status.ID
			}, timeout, interval).Should(Succeed())

			endpoint, err := mockClientset.Endpoints().Get(context.Background(), endpointID)
			Expect(err).NotTo(HaveOccurred())
			Expect(endpoint.TrafficPolicy).To(ContainSubstring("legacy"))
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
			_, err := mockClientset.Endpoints().Get(context.Background(), endpointID)
			Expect(err).To(HaveOccurred())
		})

		It("should handle API errors gracefully", func(ctx SpecContext) {
			// Set the mock to return an error
			mockClientset.Endpoints().(*nmockapi.EndpointsClient).SetCreateError(errors.New("API error"))

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
			mockClientset.Endpoints().(*nmockapi.EndpointsClient).SetCreateError(nil)
		})
	})

	// R1 of the two-release CloudEndpoint trafficpolicy field migration.
	// These cases assert the legacy/canonical coexistence semantics needed
	// for rollback safety: legacy + canonical together is the recommended
	// R1 manifest shape during the deprecation window. Cleanup release
	// must delete every R1-prefixed case in this Context.
	Context("Traffic policy validation", func() {
		It("R1: trafficPolicyName + trafficPolicy.targetRef coexist, canonical wins", func(ctx SpecContext) {
			// The controller must handle the case where a user has set
			// both top-level fields. Canonical wins; the legacy is
			// ignored. Note: this combination is NOT rollback-safe to
			// pre-0.24 (the prior controller rejects coexistence) — the
			// migration guide recommends keeping `trafficPolicyName`
			// alone until the rollback floor moves past 0.24.
			canonicalPolicy := &ngrokv1alpha1.NgrokTrafficPolicy{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "canonical-policy",
					Namespace: namespace,
				},
				Spec: ngrokv1alpha1.NgrokTrafficPolicySpec{
					Policy: []byte(`{"on_http_request":[{"name":"canonical"}]}`),
				},
			}
			Expect(k8sClient.Create(ctx, canonicalPolicy)).To(Succeed())

			cloudEndpoint = &ngrokv1alpha1.CloudEndpoint{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "both-fields-endpoint",
					Namespace: namespace,
				},
				Spec: ngrokv1alpha1.CloudEndpointSpec{
					URL:               "https://both-fields.internal",
					Description:       "Both legacy and canonical fields set",
					Metadata:          "{}",
					TrafficPolicyName: "ignored-legacy-name",
					TrafficPolicy: &ngrokv1alpha1.CloudEndpointTrafficPolicyCfg{
						Reference: &ngrokv1alpha1.K8sObjectRef{Name: "canonical-policy"},
					},
				},
			}

			Expect(k8sClient.Create(ctx, cloudEndpoint)).To(Succeed())

			By("Verifying the canonical policy was attached and the legacy field ignored")
			var endpointID string
			Eventually(func(g Gomega) {
				obj := &ngrokv1alpha1.CloudEndpoint{}
				g.Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(cloudEndpoint), obj)).To(Succeed())
				g.Expect(obj.Status.ID).NotTo(BeEmpty())
				endpointID = obj.Status.ID

				tpCond := findCloudEndpointCondition(obj.Status.Conditions, ConditionTrafficPolicy)
				g.Expect(tpCond).NotTo(BeNil())
				g.Expect(tpCond.Status).To(Equal(metav1.ConditionTrue))
			}, timeout, interval).Should(Succeed())

			endpoint, err := mockClientset.Endpoints().Get(context.Background(), endpointID)
			Expect(err).NotTo(HaveOccurred())
			Expect(endpoint.TrafficPolicy).To(ContainSubstring("canonical"))
		})

		It("rejects spec.trafficPolicy.inline + spec.trafficPolicy.targetRef at admission", func() {
			// Both canonical fields together is ambiguous — not a
			// migration scenario. The CEL rule on
			// CloudEndpointTrafficPolicyCfg rejects this. `policy` may
			// coexist with either canonical field during R1; only the
			// inline+targetRef union is rejected.
			cloudEndpoint = &ngrokv1alpha1.CloudEndpoint{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "invalid-tp-union-endpoint",
					Namespace: namespace,
				},
				Spec: ngrokv1alpha1.CloudEndpointSpec{
					URL: "https://invalid-tp-union.internal",
					TrafficPolicy: &ngrokv1alpha1.CloudEndpointTrafficPolicyCfg{
						Inline:    json.RawMessage(`{"on_http_request":[]}`),
						Reference: &ngrokv1alpha1.K8sObjectRef{Name: "policy"},
					},
				},
			}

			err := k8sClient.Create(context.Background(), cloudEndpoint)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("inline and spec.trafficPolicy.targetRef cannot both be set"))
		})

		It("R1: trafficPolicy.policy + trafficPolicy.inline coexist (rollback-safe), canonical wins", func(ctx SpecContext) {
			// Inline form of the recommended R1 manifest shape: keep
			// the deprecated nested `policy` field for rollback while
			// the R1+ operator uses the new `inline` field.
			cloudEndpoint = &ngrokv1alpha1.CloudEndpoint{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "policy-and-inline-endpoint",
					Namespace: namespace,
				},
				Spec: ngrokv1alpha1.CloudEndpointSpec{
					URL: "https://policy-and-inline.internal",
					TrafficPolicy: &ngrokv1alpha1.CloudEndpointTrafficPolicyCfg{
						Inline: json.RawMessage(`{"on_http_request":[{"name":"canonical-inline"}]}`),
						Policy: json.RawMessage(`{"on_http_request":[{"name":"legacy-policy"}]}`),
					},
				},
			}

			Expect(k8sClient.Create(ctx, cloudEndpoint)).To(Succeed())

			var endpointID string
			Eventually(func(g Gomega) {
				obj := &ngrokv1alpha1.CloudEndpoint{}
				g.Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(cloudEndpoint), obj)).To(Succeed())
				g.Expect(obj.Status.ID).NotTo(BeEmpty())
				endpointID = obj.Status.ID
			}, timeout, interval).Should(Succeed())

			endpoint, err := mockClientset.Endpoints().Get(context.Background(), endpointID)
			Expect(err).NotTo(HaveOccurred())
			Expect(endpoint.TrafficPolicy).To(ContainSubstring("canonical-inline"))
			Expect(endpoint.TrafficPolicy).NotTo(ContainSubstring("legacy-policy"))
		})

		It("R1: empty trafficPolicy:{} alongside trafficPolicyName falls back to the legacy field", func(ctx SpecContext) {
			// Templating systems often emit an empty object for absent
			// optional fields. The controller must not let a stray
			// `trafficPolicy: {}` silently detach a policy that the user
			// declared via the legacy `trafficPolicyName` field.
			legacyPolicy := &ngrokv1alpha1.NgrokTrafficPolicy{
				ObjectMeta: metav1.ObjectMeta{Name: "fallback-legacy", Namespace: namespace},
				Spec:       ngrokv1alpha1.NgrokTrafficPolicySpec{Policy: []byte(`{"on_http_request":[{"name":"fallback-rule"}]}`)},
			}
			Expect(k8sClient.Create(ctx, legacyPolicy)).To(Succeed())

			cloudEndpoint = &ngrokv1alpha1.CloudEndpoint{
				ObjectMeta: metav1.ObjectMeta{Name: "fallback-endpoint", Namespace: namespace},
				Spec: ngrokv1alpha1.CloudEndpointSpec{
					URL:               "https://fallback.internal",
					TrafficPolicyName: "fallback-legacy",
					// Empty struct mimics a templating system emitting `{}`.
					TrafficPolicy: &ngrokv1alpha1.CloudEndpointTrafficPolicyCfg{},
				},
			}

			Expect(k8sClient.Create(ctx, cloudEndpoint)).To(Succeed())

			var endpointID string
			Eventually(func(g Gomega) {
				obj := &ngrokv1alpha1.CloudEndpoint{}
				g.Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(cloudEndpoint), obj)).To(Succeed())
				g.Expect(obj.Status.ID).NotTo(BeEmpty())
				endpointID = obj.Status.ID

				tpCond := findCloudEndpointCondition(obj.Status.Conditions, ConditionTrafficPolicy)
				g.Expect(tpCond).NotTo(BeNil())
				g.Expect(tpCond.Status).To(Equal(metav1.ConditionTrue))
			}, timeout, interval).Should(Succeed())

			endpoint, err := mockClientset.Endpoints().Get(context.Background(), endpointID)
			Expect(err).NotTo(HaveOccurred())
			Expect(endpoint.TrafficPolicy).To(ContainSubstring("fallback-rule"))
		})

		It("R1: transitioning an endpoint from legacy trafficPolicyName to canonical targetRef updates the attached policy", func(ctx SpecContext) {
			// Mirrors the user's actual R1 migration path: start with
			// the legacy-only shape, then add the canonical field
			// (still keeping the legacy for rollback). The attached
			// policy must reflect the canonical field after the update.
			legacyPolicy := &ngrokv1alpha1.NgrokTrafficPolicy{
				ObjectMeta: metav1.ObjectMeta{Name: "transition-legacy", Namespace: namespace},
				Spec:       ngrokv1alpha1.NgrokTrafficPolicySpec{Policy: []byte(`{"on_http_request":[{"name":"from-legacy"}]}`)},
			}
			Expect(k8sClient.Create(ctx, legacyPolicy)).To(Succeed())

			canonicalPolicy := &ngrokv1alpha1.NgrokTrafficPolicy{
				ObjectMeta: metav1.ObjectMeta{Name: "transition-canonical", Namespace: namespace},
				Spec:       ngrokv1alpha1.NgrokTrafficPolicySpec{Policy: []byte(`{"on_http_request":[{"name":"from-canonical"}]}`)},
			}
			Expect(k8sClient.Create(ctx, canonicalPolicy)).To(Succeed())

			cloudEndpoint = &ngrokv1alpha1.CloudEndpoint{
				ObjectMeta: metav1.ObjectMeta{Name: "transition-endpoint", Namespace: namespace},
				Spec: ngrokv1alpha1.CloudEndpointSpec{
					URL:               "https://transition.internal",
					TrafficPolicyName: "transition-legacy",
				},
			}

			By("Creating with the legacy-only shape")
			Expect(k8sClient.Create(ctx, cloudEndpoint)).To(Succeed())

			var endpointID string
			Eventually(func(g Gomega) {
				obj := &ngrokv1alpha1.CloudEndpoint{}
				g.Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(cloudEndpoint), obj)).To(Succeed())
				g.Expect(obj.Status.ID).NotTo(BeEmpty())
				endpointID = obj.Status.ID
			}, timeout, interval).Should(Succeed())

			Eventually(func(g Gomega) {
				endpoint, err := mockClientset.Endpoints().Get(context.Background(), endpointID)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(endpoint.TrafficPolicy).To(ContainSubstring("from-legacy"))
			}, timeout, interval).Should(Succeed())

			By("Adding the canonical field alongside the legacy one (rollback-safe migration step)")
			Eventually(func(g Gomega) {
				obj := &ngrokv1alpha1.CloudEndpoint{}
				g.Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(cloudEndpoint), obj)).To(Succeed())
				obj.Spec.TrafficPolicy = &ngrokv1alpha1.CloudEndpointTrafficPolicyCfg{
					Reference: &ngrokv1alpha1.K8sObjectRef{Name: "transition-canonical"},
				}
				g.Expect(k8sClient.Update(ctx, obj)).To(Succeed())
			}, timeout, interval).Should(Succeed())

			By("Verifying the canonical policy wins and the legacy is ignored")
			Eventually(func(g Gomega) {
				endpoint, err := mockClientset.Endpoints().Get(context.Background(), endpointID)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(endpoint.TrafficPolicy).To(ContainSubstring("from-canonical"))
				g.Expect(endpoint.TrafficPolicy).NotTo(ContainSubstring("from-legacy"))
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
				endpoint, err := mockClientset.Endpoints().Get(context.Background(), endpointID)
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

				obj.Spec.TrafficPolicyName = "policy-v2" //nolint:staticcheck // test of deprecated field
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
			err := mockClientset.Endpoints().Delete(context.Background(), originalID)
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
				endpoint, err := mockClientset.Endpoints().Get(context.Background(), obj.Status.ID)
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
				endpoint, err := mockClientset.Endpoints().Get(context.Background(), endpointID)
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
			mockClientset.Endpoints().(*nmockapi.EndpointsClient).SetUpdateError(errors.New("API update error"))

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
			mockClientset.Endpoints().(*nmockapi.EndpointsClient).SetUpdateError(nil)
		})
	})

	Context("Domain handling", func() {
		It("should skip domain creation for Kubernetes-bound endpoint", func(ctx SpecContext) {
			cloudEndpoint = &ngrokv1alpha1.CloudEndpoint{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "k8s-bound-endpoint",
					Namespace: namespace,
				},
				Spec: ngrokv1alpha1.CloudEndpointSpec{
					URL:      "http://aws.demo",
					Bindings: []string{"kubernetes"},
				},
			}

			By("Creating the CloudEndpoint")
			Expect(k8sClient.Create(ctx, cloudEndpoint)).To(Succeed())

			By("Verifying endpoint becomes ready without domain creation")
			Eventually(func(g Gomega) {
				obj := &ngrokv1alpha1.CloudEndpoint{}
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
				readyCond := findCloudEndpointCondition(obj.Status.Conditions, ConditionCloudEndpointReady)
				g.Expect(readyCond).NotTo(BeNil())
				g.Expect(readyCond.Status).To(Equal(metav1.ConditionTrue))
			}, timeout, interval).Should(Succeed())

			By("Verifying no Domain CRD was created")
			domains := &ingressv1alpha1.DomainList{}
			Expect(k8sClient.List(ctx, domains, client.InNamespace(namespace))).To(Succeed())
			Expect(domains.Items).To(BeEmpty())

			By("Verifying endpoint was created in mock client")
			Eventually(func(g Gomega) {
				obj := &ngrokv1alpha1.CloudEndpoint{}
				g.Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(cloudEndpoint), obj)).To(Succeed())
				g.Expect(obj.Status.ID).NotTo(BeEmpty())

				endpoint, err := mockClientset.Endpoints().Get(ctx, obj.Status.ID)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(endpoint.URL).To(Equal("http://aws.demo"))
			}, timeout, interval).Should(Succeed())
		})

		It("should clear stale domainRef for internal domain endpoint", func(ctx SpecContext) {
			staleDomainName := "stale-domain-ref"
			cloudEndpoint = &ngrokv1alpha1.CloudEndpoint{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "internal-with-stale-ref",
					Namespace: namespace,
				},
				Spec: ngrokv1alpha1.CloudEndpointSpec{
					URL: "http://test.internal",
				},
				Status: ngrokv1alpha1.CloudEndpointStatus{
					DomainRef: &ngrokv1alpha1.K8sObjectRefOptionalNamespace{
						Name:      staleDomainName,
						Namespace: &namespace,
					},
				},
			}

			By("Creating the CloudEndpoint with stale domainRef")
			Expect(k8sClient.Create(ctx, cloudEndpoint)).To(Succeed())

			By("Verifying controller clears the stale domainRef")
			Eventually(func(g Gomega) {
				obj := &ngrokv1alpha1.CloudEndpoint{}
				g.Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(cloudEndpoint), obj)).To(Succeed())

				g.Expect(obj.Status.DomainRef).To(BeNil())

				readyCond := findCloudEndpointCondition(obj.Status.Conditions, ConditionCloudEndpointReady)
				g.Expect(readyCond).NotTo(BeNil())
				g.Expect(readyCond.Status).To(Equal(metav1.ConditionTrue))
			}, timeout, interval).Should(Succeed())
		})

		It("should create endpoint even when domain is not ready", func(ctx SpecContext) {
			cloudEndpoint = &ngrokv1alpha1.CloudEndpoint{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-endpoint",
					Namespace: namespace,
				},
				Spec: ngrokv1alpha1.CloudEndpointSpec{
					URL: "http://example.com",
				},
			}

			By("Creating the CloudEndpoint")
			Expect(k8sClient.Create(ctx, cloudEndpoint)).To(Succeed())

			By("Waiting for controller to create Domain CR")
			var domain *ingressv1alpha1.Domain
			Eventually(func(g Gomega) {
				domains := &ingressv1alpha1.DomainList{}
				g.Expect(k8sClient.List(ctx, domains, client.InNamespace(namespace))).To(Succeed())
				g.Expect(domains.Items).To(HaveLen(1))
				domain = &domains.Items[0]
				g.Expect(domain.Spec.Domain).To(Equal("example.com"))
			}, timeout, interval).Should(Succeed())

			By("Waiting for endpoint to be created but not ready (domain not ready)")
			Eventually(func(g Gomega) {
				obj := &ngrokv1alpha1.CloudEndpoint{}
				g.Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(cloudEndpoint), obj)).To(Succeed())

				// Endpoint should be created
				g.Expect(obj.Status.ID).NotTo(BeEmpty())

				// DomainReady should be False (domain exists but not ready yet)
				domainCond := findCloudEndpointCondition(obj.Status.Conditions, "DomainReady")
				g.Expect(domainCond).NotTo(BeNil())
				g.Expect(domainCond.Status).To(Equal(metav1.ConditionFalse))

				// Ready should be False
				readyCond := findCloudEndpointCondition(obj.Status.Conditions, ConditionCloudEndpointReady)
				g.Expect(readyCond).NotTo(BeNil())
				g.Expect(readyCond.Status).To(Equal(metav1.ConditionFalse))
			}, timeout, interval).Should(Succeed())

			By("Making the domain ready")
			Eventually(func(g Gomega) {
				latestDomain := &ingressv1alpha1.Domain{}
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
				obj := &ngrokv1alpha1.CloudEndpoint{}
				g.Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(cloudEndpoint), obj)).To(Succeed())

				// DomainReady should be True
				domainCond := findCloudEndpointCondition(obj.Status.Conditions, "DomainReady")
				g.Expect(domainCond).NotTo(BeNil())
				g.Expect(domainCond.Status).To(Equal(metav1.ConditionTrue))

				// Ready should be True
				readyCond := findCloudEndpointCondition(obj.Status.Conditions, ConditionCloudEndpointReady)
				g.Expect(readyCond).NotTo(BeNil())
				g.Expect(readyCond.Status).To(Equal(metav1.ConditionTrue))
			}, 45*time.Second, interval).Should(Succeed())

			By("Verifying endpoint was created in mock client")
			Eventually(func(g Gomega) {
				obj := &ngrokv1alpha1.CloudEndpoint{}
				g.Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(cloudEndpoint), obj)).To(Succeed())
				g.Expect(obj.Status.ID).NotTo(BeEmpty())

				endpoint, err := mockClientset.Endpoints().Get(ctx, obj.Status.ID)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(endpoint.URL).To(Equal("http://example.com"))
			}, timeout, interval).Should(Succeed())
		})

		It("should not call the ngrok API update while domain is not ready", func(ctx SpecContext) {
			cloudEndpoint = &ngrokv1alpha1.CloudEndpoint{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "no-unnecessary-updates",
					Namespace: namespace,
				},
				Spec: ngrokv1alpha1.CloudEndpointSpec{
					URL: "http://no-update-spam.example.com",
				},
			}

			By("Creating the CloudEndpoint")
			Expect(k8sClient.Create(ctx, cloudEndpoint)).To(Succeed())

			By("Waiting for the endpoint to be created (Status.ID set)")
			kginkgo.EventuallyWithCloudEndpoint(ctx, cloudEndpoint, func(g Gomega, fetched *ngrokv1alpha1.CloudEndpoint) {
				g.Expect(fetched.Status.ID).NotTo(BeEmpty())
			})

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
			endpoints := []*ngrokv1alpha1.CloudEndpoint{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "k8s-endpoint-1",
						Namespace: namespace,
					},
					Spec: ngrokv1alpha1.CloudEndpointSpec{
						URL:      "http://aws1.demo",
						Bindings: []string{"kubernetes"},
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "k8s-endpoint-2",
						Namespace: namespace,
					},
					Spec: ngrokv1alpha1.CloudEndpointSpec{
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
					obj := &ngrokv1alpha1.CloudEndpoint{}
					g.Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(ep), obj)).To(Succeed())

					readyCond := findCloudEndpointCondition(obj.Status.Conditions, ConditionCloudEndpointReady)
					g.Expect(readyCond).NotTo(BeNil())
					g.Expect(readyCond.Status).To(Equal(metav1.ConditionTrue))
				}, timeout, interval).Should(Succeed())
			}

			By("Verifying no Domain CRD was created")
			domains := &ingressv1alpha1.DomainList{}
			Expect(k8sClient.List(ctx, domains, client.InNamespace(namespace))).To(Succeed())
			Expect(domains.Items).To(BeEmpty())

			By("Verifying both endpoints were created in mock client")
			for _, ep := range endpoints {
				Eventually(func(g Gomega) {
					obj := &ngrokv1alpha1.CloudEndpoint{}
					g.Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(ep), obj)).To(Succeed())
					g.Expect(obj.Status.ID).NotTo(BeEmpty())

					endpoint, err := mockClientset.Endpoints().Get(ctx, obj.Status.ID)
					g.Expect(err).NotTo(HaveOccurred())
					g.Expect(endpoint.URL).To(Or(Equal("http://aws1.demo"), Equal("http://aws2.demo")))
				}, timeout, interval).Should(Succeed())
			}
		})
	})

	Context("Bindings validation", func() {
		It("should reject endpoint with multiple bindings", func() {
			// This should be caught by k8s validation (MaxItems=1), so we expect the Create to fail
			cloudEndpoint := &ngrokv1alpha1.CloudEndpoint{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "invalid-multiple-bindings",
					Namespace: namespace,
				},
				Spec: ngrokv1alpha1.CloudEndpointSpec{
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
			cloudEndpoint := &ngrokv1alpha1.CloudEndpoint{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "valid-single-binding",
					Namespace: namespace,
				},
				Spec: ngrokv1alpha1.CloudEndpointSpec{
					URL:      "http://test-internal.demo",
					Bindings: []string{"internal"}, // Single binding is valid
				},
			}

			By("Creating the CloudEndpoint with single binding")
			err := k8sClient.Create(ctx, cloudEndpoint)
			Expect(err).NotTo(HaveOccurred()) // Should be accepted

			By("Verifying endpoint is created successfully")
			Eventually(func(g Gomega) {
				obj := &ngrokv1alpha1.CloudEndpoint{}
				g.Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(cloudEndpoint), obj)).To(Succeed())
				g.Expect(obj.Spec.Bindings).To(Equal([]string{"internal"}))
			}, timeout, interval).Should(Succeed())
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
