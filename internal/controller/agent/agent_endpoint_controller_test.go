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
	"context"
	"errors"
	"math/rand"
	"time"

	ingressv1alpha1 "github.com/ngrok/ngrok-operator/api/ingress/v1alpha1"
	ngrokv1alpha1 "github.com/ngrok/ngrok-operator/api/ngrok/v1alpha1"
	domainpkg "github.com/ngrok/ngrok-operator/internal/domain"
	"github.com/ngrok/ngrok-operator/internal/testutils"
	"github.com/ngrok/ngrok-operator/pkg/agent"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"

	"sigs.k8s.io/controller-runtime/pkg/client"
)

var _ = Describe("AgentEndpoint Controller", func() {
	const (
		timeout  = 15 * time.Second
		interval = 500 * time.Millisecond
	)

	var (
		namespace     string
		agentEndpoint *ngrokv1alpha1.AgentEndpoint
	)

	BeforeEach(func() {
		namespace = "test-" + RandomString(8)

		// Create namespace for testing
		ns := &v1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: namespace,
			},
		}
		Expect(k8sClient.Create(context.Background(), ns)).To(Succeed())

		// Reset the shared mock driver for each test
		envMockDriver.Reset()

	})

	AfterEach(func() {
		// Clean up namespace
		if namespace != "" {
			ns := &v1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: namespace,
				},
			}
			err := k8sClient.Delete(context.Background(), ns)
			Expect(err).NotTo(HaveOccurred())
		}
	})

	Context("Basic endpoint operations", func() {
		It("should successfully reconcile TCP endpoint", func(ctx SpecContext) {
			agentEndpoint = &ngrokv1alpha1.AgentEndpoint{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "tcp-endpoint",
					Namespace: namespace,
				},
				Spec: ngrokv1alpha1.AgentEndpointSpec{
					URL: "tcp://1.tcp.ngrok.io:12345",
					Upstream: ngrokv1alpha1.EndpointUpstream{
						URL: "http://test-service:80",
					},
				},
			}

			envMockDriver.SetEndpointResult(namespace+"/tcp-endpoint", &agent.EndpointResult{
				URL: "tcp://1.tcp.ngrok.io:12345",
			})

			By("Creating the AgentEndpoint")
			Expect(k8sClient.Create(ctx, agentEndpoint)).To(Succeed())

			By("Waiting for controller to reconcile and set ready condition")
			Eventually(func(g Gomega) {
				obj := &ngrokv1alpha1.AgentEndpoint{}
				g.Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(agentEndpoint), obj)).To(Succeed())

				// Check ready condition set by running controller
				cond := testutils.FindCondition(obj.Status.Conditions, ConditionReady)
				g.Expect(cond).NotTo(BeNil())
				g.Expect(cond.Status).To(Equal(metav1.ConditionTrue))
				g.Expect(cond.Reason).To(Equal(ReasonEndpointActive))

				// Verify status fields set by controller
				g.Expect(obj.Status.AssignedURL).To(Equal("tcp://1.tcp.ngrok.io:12345"))
				g.Expect(obj.Status.AttachedTrafficPolicy).To(Equal("none"))
			}, timeout, interval).Should(Succeed())
		})

		It("should not update LastTransitionTime on re-reconcile when ready state unchanged", func(ctx SpecContext) {
			agentEndpoint = &ngrokv1alpha1.AgentEndpoint{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "stable-endpoint",
					Namespace: namespace,
				},
				Spec: ngrokv1alpha1.AgentEndpointSpec{
					URL: "tcp://1.tcp.ngrok.io:12345",
					Upstream: ngrokv1alpha1.EndpointUpstream{
						URL: "http://test-service:80",
					},
				},
			}

			envMockDriver.SetEndpointResult(namespace+"/stable-endpoint", &agent.EndpointResult{
				URL: "tcp://1.tcp.ngrok.io:12345",
			})

			By("Creating the AgentEndpoint")
			Expect(k8sClient.Create(ctx, agentEndpoint)).To(Succeed())

			var initialTransitionTime metav1.Time

			By("Waiting for controller to reconcile and become ready")
			Eventually(func(g Gomega) {
				obj := &ngrokv1alpha1.AgentEndpoint{}
				g.Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(agentEndpoint), obj)).To(Succeed())

				cond := testutils.FindCondition(obj.Status.Conditions, ConditionReady)
				g.Expect(cond).NotTo(BeNil())
				g.Expect(cond.Status).To(Equal(metav1.ConditionTrue))
				g.Expect(cond.Reason).To(Equal(ReasonEndpointActive))

				initialTransitionTime = cond.LastTransitionTime
			}, timeout, interval).Should(Succeed())

			By("Sleeping to ensure time passes")
			time.Sleep(2 * time.Second)

			By("Triggering a re-reconcile by updating an annotation")
			Eventually(func(g Gomega) {
				obj := &ngrokv1alpha1.AgentEndpoint{}
				g.Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(agentEndpoint), obj)).To(Succeed())

				if obj.Annotations == nil {
					obj.Annotations = make(map[string]string)
				}
				obj.Annotations["test-annotation"] = "trigger-reconcile"
				g.Expect(k8sClient.Update(ctx, obj)).To(Succeed())
			}, timeout, interval).Should(Succeed())

			By("Waiting for re-reconcile to complete")
			time.Sleep(1 * time.Second)

			By("Verifying LastTransitionTime has NOT changed")
			obj := &ngrokv1alpha1.AgentEndpoint{}
			Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(agentEndpoint), obj)).To(Succeed())

			cond := testutils.FindCondition(obj.Status.Conditions, ConditionReady)
			Expect(cond).NotTo(BeNil())
			Expect(cond.Status).To(Equal(metav1.ConditionTrue))
			Expect(cond.Reason).To(Equal(ReasonEndpointActive))

			Expect(cond.LastTransitionTime).To(Equal(initialTransitionTime),
				"LastTransitionTime should not change when ready state remains True")
		})

		It("should handle internal endpoints without domain creation", func(ctx SpecContext) {
			agentEndpoint = &ngrokv1alpha1.AgentEndpoint{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "internal-endpoint",
					Namespace: namespace,
				},
				Spec: ngrokv1alpha1.AgentEndpointSpec{
					URL: "https://test.internal",
					Upstream: ngrokv1alpha1.EndpointUpstream{
						URL: "http://internal-service:80",
					},
				},
			}

			envMockDriver.SetEndpointResult(namespace+"/internal-endpoint", &agent.EndpointResult{
				URL: "https://test.internal",
			})

			By("Creating the AgentEndpoint")
			Expect(k8sClient.Create(ctx, agentEndpoint)).To(Succeed())

			By("Waiting for controller to reconcile internal endpoint")
			Eventually(func(g Gomega) {
				obj := &ngrokv1alpha1.AgentEndpoint{}
				g.Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(agentEndpoint), obj)).To(Succeed())

				cond := testutils.FindCondition(obj.Status.Conditions, ConditionReady)
				g.Expect(cond).NotTo(BeNil())
				g.Expect(cond.Status).To(Equal(metav1.ConditionTrue))
				g.Expect(obj.Status.AssignedURL).To(Equal("https://test.internal"))
			}, timeout, interval).Should(Succeed())
		})

		It("should create domain CR for custom domains", func(ctx SpecContext) {
			agentEndpoint = &ngrokv1alpha1.AgentEndpoint{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "custom-domain-endpoint",
					Namespace: namespace,
				},
				Spec: ngrokv1alpha1.AgentEndpointSpec{
					URL: "https://custom.example.com",
					Upstream: ngrokv1alpha1.EndpointUpstream{
						URL: "http://test-service:80",
					},
				},
			}

			By("Creating the AgentEndpoint")
			Expect(k8sClient.Create(ctx, agentEndpoint)).To(Succeed())

			By("Waiting for controller to create domain and set domain creation condition")
			Eventually(func(g Gomega) {
				obj := &ngrokv1alpha1.AgentEndpoint{}
				g.Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(agentEndpoint), obj)).To(Succeed())

				// Check domain creation condition
				domainCond := testutils.FindCondition(obj.Status.Conditions, domainpkg.ConditionEndpointDomainReady)
				g.Expect(domainCond).NotTo(BeNil())
				g.Expect(domainCond.Status).To(Equal(metav1.ConditionFalse))
				g.Expect(domainCond.Reason).To(Equal(domainpkg.ReasonDomainCreating))
			}, timeout, interval).Should(Succeed())

			By("Verifying domain CR was created by controller")
			Eventually(func(g Gomega) {
				domainList := &ingressv1alpha1.DomainList{}
				g.Expect(k8sClient.List(ctx, domainList, client.InNamespace(namespace))).To(Succeed())
				g.Expect(domainList.Items).To(HaveLen(1))
				g.Expect(domainList.Items[0].Spec.Domain).To(Equal("custom.example.com"))
			}, timeout, interval).Should(Succeed())
		})

		It("should handle endpoint creation failure", func(ctx SpecContext) {
			agentEndpoint = &ngrokv1alpha1.AgentEndpoint{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "failing-endpoint",
					Namespace: namespace,
				},
				Spec: ngrokv1alpha1.AgentEndpointSpec{
					URL: "tcp://2.tcp.ngrok.io:54321",
					Upstream: ngrokv1alpha1.EndpointUpstream{
						URL: "http://test-service:80",
					},
				},
			}

			envMockDriver.SetEndpointError(namespace+"/failing-endpoint", errors.New("endpoint creation failed"))

			By("Creating the AgentEndpoint")
			Expect(k8sClient.Create(ctx, agentEndpoint)).To(Succeed())

			By("Waiting for controller to reconcile and set error condition")
			Eventually(func(g Gomega) {
				obj := &ngrokv1alpha1.AgentEndpoint{}
				g.Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(agentEndpoint), obj)).To(Succeed())

				// Check error condition set by running controller
				cond := testutils.FindCondition(obj.Status.Conditions, ConditionReady)
				g.Expect(cond).NotTo(BeNil())
				g.Expect(cond.Status).To(Equal(metav1.ConditionFalse))
				g.Expect(cond.Reason).To(Equal(ReasonNgrokAPIError))
			}, timeout, interval).Should(Succeed())
		})

		It("should handle endpoint deletion", func(ctx SpecContext) {
			agentEndpoint = &ngrokv1alpha1.AgentEndpoint{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "delete-endpoint",
					Namespace: namespace,
				},
				Spec: ngrokv1alpha1.AgentEndpointSpec{
					URL: "tcp://3.tcp.ngrok.io:11111",
					Upstream: ngrokv1alpha1.EndpointUpstream{
						URL: "http://test-service:80",
					},
				},
			}

			envMockDriver.SetEndpointResult(namespace+"/delete-endpoint", &agent.EndpointResult{
				URL: "tcp://3.tcp.ngrok.io:11111",
			})

			By("Creating the AgentEndpoint")
			Expect(k8sClient.Create(ctx, agentEndpoint)).To(Succeed())

			By("Waiting for controller to create the endpoint")
			Eventually(func() bool {
				for _, call := range envMockDriver.CreateCalls {
					if call.Name == namespace+"/delete-endpoint" {
						return true
					}
				}
				return false
			}, timeout, interval).Should(BeTrue())

			By("Deleting the AgentEndpoint")
			Expect(k8sClient.Delete(ctx, agentEndpoint)).To(Succeed())

			By("Waiting for controller to call delete on the driver")
			Eventually(func() bool {
				for _, call := range envMockDriver.DeleteCalls {
					if call.Name == namespace+"/delete-endpoint" {
						return true
					}
				}
				return false
			}, timeout, interval).Should(BeTrue())
		})
	})

	Context("Traffic policy handling", func() {
		It("should handle inline traffic policy", func(ctx SpecContext) {
			agentEndpoint = &ngrokv1alpha1.AgentEndpoint{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "inline-policy-endpoint",
					Namespace: namespace,
				},
				Spec: ngrokv1alpha1.AgentEndpointSpec{
					URL: "tcp://4.tcp.ngrok.io:44444",
					Upstream: ngrokv1alpha1.EndpointUpstream{
						URL: "http://test-service:80",
					},
					TrafficPolicy: &ngrokv1alpha1.TrafficPolicyCfg{
						Inline: []byte(`{"on_http_request":[]}`),
					},
				},
			}

			envMockDriver.SetEndpointResult(namespace+"/inline-policy-endpoint", &agent.EndpointResult{
				URL: "tcp://4.tcp.ngrok.io:44444",
			})

			By("Creating the AgentEndpoint")
			Expect(k8sClient.Create(ctx, agentEndpoint)).To(Succeed())

			By("Waiting for controller to reconcile and apply inline policy")
			Eventually(func(g Gomega) {
				obj := &ngrokv1alpha1.AgentEndpoint{}
				g.Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(agentEndpoint), obj)).To(Succeed())

				g.Expect(obj.Status.AttachedTrafficPolicy).To(Equal("inline"))

				cond := testutils.FindCondition(obj.Status.Conditions, ConditionReady)
				g.Expect(cond).NotTo(BeNil())
				g.Expect(cond.Status).To(Equal(metav1.ConditionTrue))
			}, timeout, interval).Should(Succeed())

			By("Verifying the controller passed correct policy to mock driver")
			Eventually(func() bool {
				for _, call := range envMockDriver.CreateCalls {
					if call.Name == namespace+"/inline-policy-endpoint" &&
						call.TrafficPolicy == `{"on_http_request":[]}` {
						return true
					}
				}
				return false
			}, 5*time.Second, interval).Should(BeTrue())
		})

		It("should resolve traffic policy reference", func(ctx SpecContext) {
			// Create traffic policy
			trafficPolicy := &ngrokv1alpha1.NgrokTrafficPolicy{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "referenced-policy",
					Namespace: namespace,
				},
				Spec: ngrokv1alpha1.NgrokTrafficPolicySpec{
					Policy: []byte(`{"on_http_request":[{"name":"rate-limit"}]}`),
				},
			}
			Expect(k8sClient.Create(ctx, trafficPolicy)).To(Succeed())

			agentEndpoint = &ngrokv1alpha1.AgentEndpoint{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "ref-policy-endpoint",
					Namespace: namespace,
				},
				Spec: ngrokv1alpha1.AgentEndpointSpec{
					URL: "tcp://5.tcp.ngrok.io:55555",
					Upstream: ngrokv1alpha1.EndpointUpstream{
						URL: "http://test-service:80",
					},
					TrafficPolicy: &ngrokv1alpha1.TrafficPolicyCfg{
						Reference: &ngrokv1alpha1.K8sObjectRef{
							Name: "referenced-policy",
						},
					},
				},
			}

			envMockDriver.SetEndpointResult(namespace+"/ref-policy-endpoint", &agent.EndpointResult{
				URL: "tcp://5.tcp.ngrok.io:55555",
			})

			By("Creating the AgentEndpoint")
			Expect(k8sClient.Create(ctx, agentEndpoint)).To(Succeed())

			By("Waiting for controller to reconcile and resolve policy reference")
			Eventually(func(g Gomega) {
				obj := &ngrokv1alpha1.AgentEndpoint{}
				g.Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(agentEndpoint), obj)).To(Succeed())

				g.Expect(obj.Status.AttachedTrafficPolicy).To(Equal("referenced-policy"))

				cond := testutils.FindCondition(obj.Status.Conditions, ConditionReady)
				g.Expect(cond).NotTo(BeNil())
				g.Expect(cond.Status).To(Equal(metav1.ConditionTrue))
			}, timeout, interval).Should(Succeed())

			By("Verifying the controller resolved and passed the traffic policy")
			Eventually(func() bool {
				for _, call := range envMockDriver.CreateCalls {
					if call.Name == namespace+"/ref-policy-endpoint" {
						matched, _ := ContainSubstring(`"name":"rate-limit"`).Match(call.TrafficPolicy)
						return matched
					}
				}
				return false
			}, 5*time.Second, interval).Should(BeTrue())
		})

		It("should handle missing traffic policy reference", func(ctx SpecContext) {
			agentEndpoint = &ngrokv1alpha1.AgentEndpoint{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "missing-policy-endpoint",
					Namespace: namespace,
				},
				Spec: ngrokv1alpha1.AgentEndpointSpec{
					URL: "tcp://6.tcp.ngrok.io:66666",
					Upstream: ngrokv1alpha1.EndpointUpstream{
						URL: "http://test-service:80",
					},
					TrafficPolicy: &ngrokv1alpha1.TrafficPolicyCfg{
						Reference: &ngrokv1alpha1.K8sObjectRef{
							Name: "missing-policy",
						},
					},
				},
			}

			By("Creating the AgentEndpoint")
			Expect(k8sClient.Create(ctx, agentEndpoint)).To(Succeed())

			By("Waiting for controller to detect missing policy and set error condition")
			Eventually(func(g Gomega) {
				obj := &ngrokv1alpha1.AgentEndpoint{}
				g.Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(agentEndpoint), obj)).To(Succeed())

				policyCondition := testutils.FindCondition(obj.Status.Conditions, ConditionTrafficPolicy)
				g.Expect(policyCondition).NotTo(BeNil())
				g.Expect(policyCondition.Status).To(Equal(metav1.ConditionFalse))
				g.Expect(policyCondition.Reason).To(Equal(ReasonTrafficPolicyError))
			}, timeout, interval).Should(Succeed())
		})

		It("should reject both inline and reference policy at validation", func() {
			// This should be caught by k8s validation, so we expect the Create to fail
			agentEndpoint = &ngrokv1alpha1.AgentEndpoint{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "invalid-policy-endpoint",
					Namespace: namespace,
				},
				Spec: ngrokv1alpha1.AgentEndpointSpec{
					URL: "tcp://7.tcp.ngrok.io:77777",
					Upstream: ngrokv1alpha1.EndpointUpstream{
						URL: "http://test-service:80",
					},
					TrafficPolicy: &ngrokv1alpha1.TrafficPolicyCfg{
						Inline:    []byte(`{}`),
						Reference: &ngrokv1alpha1.K8sObjectRef{Name: "policy"},
					},
				},
			}

			err := k8sClient.Create(context.Background(), agentEndpoint)
			Expect(err).To(HaveOccurred()) // Should be rejected by validation
			Expect(err.Error()).To(ContainSubstring("Only one of inline and targetRef can be configured"))
		})
	})

	Context("when AgentEndpoint references a TrafficPolicy", func() {
		It("should successfully apply traffic policy", func(ctx SpecContext) {
			// Create a traffic policy
			trafficPolicy := &ngrokv1alpha1.NgrokTrafficPolicy{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-policy",
					Namespace: namespace,
				},
				Spec: ngrokv1alpha1.NgrokTrafficPolicySpec{
					Policy: []byte(`{"inbound":[{"type":"deny"}]}`),
				},
			}
			Expect(k8sClient.Create(ctx, trafficPolicy)).To(Succeed())

			agentEndpoint = &ngrokv1alpha1.AgentEndpoint{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-endpoint-with-policy",
					Namespace: namespace,
				},
				Spec: ngrokv1alpha1.AgentEndpointSpec{
					URL: "tcp://10.tcp.ngrok.io:10101",
					Upstream: ngrokv1alpha1.EndpointUpstream{
						URL: "http://test-service:80",
					},
					TrafficPolicy: &ngrokv1alpha1.TrafficPolicyCfg{
						Reference: &ngrokv1alpha1.K8sObjectRef{
							Name: "test-policy",
						},
					},
				},
			}

			// Mock successful endpoint creation with policy
			envMockDriver.SetEndpointResult(namespace+"/test-endpoint-with-policy", &agent.EndpointResult{
				URL: "tcp://10.tcp.ngrok.io:10101",
			})

			By("Creating the AgentEndpoint")
			Expect(k8sClient.Create(ctx, agentEndpoint)).To(Succeed())

			By("Waiting for controller to reconcile and apply traffic policy")
			Eventually(func(g Gomega) {
				obj := &ngrokv1alpha1.AgentEndpoint{}
				g.Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(agentEndpoint), obj)).To(Succeed())

				// Check ready condition
				cond := testutils.FindCondition(obj.Status.Conditions, ConditionReady)
				g.Expect(cond).NotTo(BeNil())
				g.Expect(cond.Status).To(Equal(metav1.ConditionTrue))

				// Verify traffic policy status
				g.Expect(obj.Status.AttachedTrafficPolicy).To(Equal("test-policy"))
			}, timeout, interval).Should(Succeed())

			By("Verifying the mock driver received the traffic policy")
			Eventually(func() bool {
				for _, call := range envMockDriver.CreateCalls {
					if call.Name == namespace+"/test-endpoint-with-policy" {
						matched, _ := ContainSubstring(`"inbound":[{"type":"deny"}]`).Match(call.TrafficPolicy)
						return matched
					}
				}
				return false
			}, 5*time.Second, interval).Should(BeTrue())
		})

		It("should reconcile when traffic policy is updated", func(ctx SpecContext) {
			// Create a traffic policy
			trafficPolicy := &ngrokv1alpha1.NgrokTrafficPolicy{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "update-policy",
					Namespace: namespace,
				},
				Spec: ngrokv1alpha1.NgrokTrafficPolicySpec{
					Policy: []byte(`{"inbound":[{"type":"deny"}]}`),
				},
			}
			Expect(k8sClient.Create(ctx, trafficPolicy)).To(Succeed())

			agentEndpoint = &ngrokv1alpha1.AgentEndpoint{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "update-policy-endpoint",
					Namespace: namespace,
				},
				Spec: ngrokv1alpha1.AgentEndpointSpec{
					URL: "tcp://11.tcp.ngrok.io:11111",
					Upstream: ngrokv1alpha1.EndpointUpstream{
						URL: "http://test-service:80",
					},
					TrafficPolicy: &ngrokv1alpha1.TrafficPolicyCfg{
						Reference: &ngrokv1alpha1.K8sObjectRef{
							Name: "update-policy",
						},
					},
				},
			}

			// Mock successful endpoint creation
			envMockDriver.SetEndpointResult(namespace+"/update-policy-endpoint", &agent.EndpointResult{
				URL: "tcp://11.tcp.ngrok.io:11111",
			})

			By("Creating the AgentEndpoint")
			Expect(k8sClient.Create(ctx, agentEndpoint)).To(Succeed())

			By("Waiting for initial reconciliation")
			Eventually(func() bool {
				for _, call := range envMockDriver.CreateCalls {
					if call.Name == namespace+"/update-policy-endpoint" {
						return true
					}
				}
				return false
			}, timeout, interval).Should(BeTrue())

			By("Updating the traffic policy")
			Eventually(func() error {
				key := types.NamespacedName{Name: trafficPolicy.Name, Namespace: trafficPolicy.Namespace}
				err := k8sClient.Get(ctx, key, trafficPolicy)
				if err != nil {
					return err
				}

				trafficPolicy.Spec.Policy = []byte(`{"inbound":[{"type":"allow"}]}`)
				return k8sClient.Update(ctx, trafficPolicy)
			}, 2*time.Second, 100*time.Millisecond).Should(Succeed())

			By("Verifying the updated policy is applied")
			Eventually(func() bool {
				for _, call := range envMockDriver.CreateCalls {
					if call.Name == namespace+"/update-policy-endpoint" {
						matched, _ := ContainSubstring(`"inbound":[{"type":"allow"}]`).Match(call.TrafficPolicy)
						if matched {
							return true
						}
					}
				}
				return false
			}, timeout, interval).Should(BeTrue())
		})
	})

	Context("when AgentEndpoint creation fails", func() {
		It("should set appropriate error conditions", func(ctx SpecContext) {
			agentEndpoint = &ngrokv1alpha1.AgentEndpoint{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "failing-endpoint",
					Namespace: namespace,
				},
				Spec: ngrokv1alpha1.AgentEndpointSpec{
					URL: "tcp://12.tcp.ngrok.io:12121",
					Upstream: ngrokv1alpha1.EndpointUpstream{
						URL: "http://test-service:80",
					},
				},
			}

			// Mock endpoint creation failure
			envMockDriver.SetEndpointError(namespace+"/failing-endpoint", errors.New("endpoint creation failed"))

			By("Creating the AgentEndpoint")
			Expect(k8sClient.Create(ctx, agentEndpoint)).To(Succeed())

			By("Waiting for controller to reconcile and set error conditions")
			Eventually(func(g Gomega) {
				obj := &ngrokv1alpha1.AgentEndpoint{}
				g.Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(agentEndpoint), obj)).To(Succeed())

				// Check ready condition is false
				cond := testutils.FindCondition(obj.Status.Conditions, ConditionReady)
				g.Expect(cond).NotTo(BeNil())
				g.Expect(cond.Status).To(Equal(metav1.ConditionFalse))
				g.Expect(cond.Reason).To(Equal(ReasonNgrokAPIError))
			}, timeout, interval).Should(Succeed())

			By("Verifying the mock driver was called for this specific endpoint")
			Eventually(func() bool {
				for _, call := range envMockDriver.CreateCalls {
					if call.Name == namespace+"/failing-endpoint" {
						return true
					}
				}
				return false
			}, 5*time.Second, interval).Should(BeTrue())
		})
	})

	Context("when AgentEndpoint is deleted", func() {
		It("should call delete on the mock driver", func(ctx SpecContext) {
			agentEndpoint = &ngrokv1alpha1.AgentEndpoint{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "delete-test-endpoint",
					Namespace: namespace,
				},
				Spec: ngrokv1alpha1.AgentEndpointSpec{
					URL: "tcp://13.tcp.ngrok.io:13131",
					Upstream: ngrokv1alpha1.EndpointUpstream{
						URL: "http://test-service:80",
					},
				},
			}

			// Mock successful creation
			envMockDriver.SetEndpointResult(namespace+"/delete-test-endpoint", &agent.EndpointResult{
				URL: "tcp://13.tcp.ngrok.io:13131",
			})

			By("Creating the AgentEndpoint")
			Expect(k8sClient.Create(ctx, agentEndpoint)).To(Succeed())

			By("Waiting for creation")
			Eventually(func() bool {
				for _, call := range envMockDriver.CreateCalls {
					if call.Name == namespace+"/delete-test-endpoint" {
						return true
					}
				}
				return false
			}, timeout, interval).Should(BeTrue())

			By("Deleting the AgentEndpoint")
			Expect(k8sClient.Delete(ctx, agentEndpoint)).To(Succeed())

			By("Verifying delete was called on the driver")
			Eventually(func() bool {
				for _, call := range envMockDriver.DeleteCalls {
					if call.Name == namespace+"/delete-test-endpoint" {
						return true
					}
				}
				return false
			}, timeout, interval).Should(BeTrue())
		})
	})

	Context("Client certificate handling", func() {
		It("should handle missing client certificate secret", func(ctx SpecContext) {
			agentEndpoint = &ngrokv1alpha1.AgentEndpoint{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "missing-cert-endpoint",
					Namespace: namespace,
				},
				Spec: ngrokv1alpha1.AgentEndpointSpec{
					URL: "tcp://8.tcp.ngrok.io:88888",
					Upstream: ngrokv1alpha1.EndpointUpstream{
						URL: "http://test-service:80",
					},
					ClientCertificateRefs: []ngrokv1alpha1.K8sObjectRefOptionalNamespace{
						{Name: "missing-secret"},
					},
				},
			}

			By("Creating the AgentEndpoint")
			Expect(k8sClient.Create(ctx, agentEndpoint)).To(Succeed())

			By("Waiting for controller to reconcile and set config error condition")
			Eventually(func(g Gomega) {
				obj := &ngrokv1alpha1.AgentEndpoint{}
				g.Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(agentEndpoint), obj)).To(Succeed())

				cond := testutils.FindCondition(obj.Status.Conditions, ConditionReady)
				g.Expect(cond).NotTo(BeNil())
				g.Expect(cond.Status).To(Equal(metav1.ConditionFalse))
				g.Expect(cond.Reason).To(Equal(ReasonConfigError))
			}, timeout, interval).Should(Succeed())
		})

		It("should handle invalid certificate data", func(ctx SpecContext) {
			// Create secret with invalid cert data
			secret := &v1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "invalid-cert",
					Namespace: namespace,
				},
				Data: map[string][]byte{
					"tls.crt": []byte("invalid-cert-data"),
					"tls.key": []byte("invalid-key-data"),
				},
			}
			Expect(k8sClient.Create(ctx, secret)).To(Succeed())

			agentEndpoint = &ngrokv1alpha1.AgentEndpoint{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "invalid-cert-endpoint",
					Namespace: namespace,
				},
				Spec: ngrokv1alpha1.AgentEndpointSpec{
					URL: "tcp://9.tcp.ngrok.io:99999",
					Upstream: ngrokv1alpha1.EndpointUpstream{
						URL: "http://test-service:80",
					},
					ClientCertificateRefs: []ngrokv1alpha1.K8sObjectRefOptionalNamespace{
						{Name: "invalid-cert"},
					},
				},
			}

			By("Creating the AgentEndpoint")
			Expect(k8sClient.Create(ctx, agentEndpoint)).To(Succeed())

			By("Waiting for controller to reconcile and set config error condition")
			Eventually(func(g Gomega) {
				obj := &ngrokv1alpha1.AgentEndpoint{}
				g.Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(agentEndpoint), obj)).To(Succeed())

				cond := testutils.FindCondition(obj.Status.Conditions, ConditionReady)
				g.Expect(cond).NotTo(BeNil())
				g.Expect(cond.Status).To(Equal(metav1.ConditionFalse))
				g.Expect(cond.Reason).To(Equal(ReasonConfigError))
			}, timeout, interval).Should(Succeed())
		})

		It("should handle missing tls.crt in secret", func(ctx SpecContext) {
			// Create secret missing tls.crt
			secret := &v1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "no-crt-secret",
					Namespace: namespace,
				},
				Data: map[string][]byte{
					"tls.key": []byte("some-key-data"),
				},
			}
			Expect(k8sClient.Create(ctx, secret)).To(Succeed())

			agentEndpoint = &ngrokv1alpha1.AgentEndpoint{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "no-crt-endpoint",
					Namespace: namespace,
				},
				Spec: ngrokv1alpha1.AgentEndpointSpec{
					URL: "tcp://10.tcp.ngrok.io:10000",
					Upstream: ngrokv1alpha1.EndpointUpstream{
						URL: "http://test-service:80",
					},
					ClientCertificateRefs: []ngrokv1alpha1.K8sObjectRefOptionalNamespace{
						{Name: "no-crt-secret"},
					},
				},
			}

			By("Creating the AgentEndpoint")
			Expect(k8sClient.Create(ctx, agentEndpoint)).To(Succeed())

			By("Waiting for controller to reconcile and set config error condition")
			Eventually(func(g Gomega) {
				obj := &ngrokv1alpha1.AgentEndpoint{}
				g.Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(agentEndpoint), obj)).To(Succeed())

				cond := testutils.FindCondition(obj.Status.Conditions, ConditionReady)
				g.Expect(cond).NotTo(BeNil())
				g.Expect(cond.Status).To(Equal(metav1.ConditionFalse))
				g.Expect(cond.Reason).To(Equal(ReasonConfigError))
			}, timeout, interval).Should(Succeed())
		})
	})

	Context("Controller runtime behavior", func() {
		const (
			timeout  = 15 * time.Second
			interval = 500 * time.Millisecond
		)

		It("should reconcile TCP endpoint automatically using Eventually", func(ctx SpecContext) {
			agentEndpoint = &ngrokv1alpha1.AgentEndpoint{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "tcp-runtime-auto",
					Namespace: namespace,
				},
				Spec: ngrokv1alpha1.AgentEndpointSpec{
					URL: "tcp://99.tcp.ngrok.io:99999",
					Upstream: ngrokv1alpha1.EndpointUpstream{
						URL: "http://test-service:80",
					},
				},
			}

			// Mock successful endpoint creation for the controller runtime
			envMockDriver.SetEndpointResult(namespace+"/tcp-runtime-auto", &agent.EndpointResult{
				URL: "tcp://99.tcp.ngrok.io:99999",
			})

			By("Creating the AgentEndpoint")
			Expect(k8sClient.Create(ctx, agentEndpoint)).To(Succeed())

			By("Waiting for controller to automatically reconcile")
			Eventually(func(g Gomega) {
				obj := &ngrokv1alpha1.AgentEndpoint{}
				g.Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(agentEndpoint), obj)).To(Succeed())

				// Check ready condition set by running controller
				cond := testutils.FindCondition(obj.Status.Conditions, ConditionReady)
				g.Expect(cond).NotTo(BeNil())
				g.Expect(cond.Status).To(Equal(metav1.ConditionTrue))
				g.Expect(cond.Reason).To(Equal(ReasonEndpointActive))

				// Verify status fields set by controller
				g.Expect(obj.Status.AssignedURL).To(Equal("tcp://99.tcp.ngrok.io:99999"))
				g.Expect(obj.Status.AttachedTrafficPolicy).To(Equal("none"))
			}, timeout, interval).Should(Succeed())

			By("Verifying the controller called the mock driver")
			Eventually(func() int {
				return len(envMockDriver.CreateCalls)
			}, 5*time.Second, interval).Should(Equal(1))

			createCall := envMockDriver.CreateCalls[0]
			Expect(createCall.Name).To(Equal(namespace + "/tcp-runtime-auto"))
			Expect(createCall.Spec.URL).To(Equal("tcp://99.tcp.ngrok.io:99999"))
		})

		It("should handle endpoint creation failure with runtime controller", func(ctx SpecContext) {
			agentEndpoint = &ngrokv1alpha1.AgentEndpoint{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "fail-runtime-auto",
					Namespace: namespace,
				},
				Spec: ngrokv1alpha1.AgentEndpointSpec{
					URL: "tcp://98.tcp.ngrok.io:98989",
					Upstream: ngrokv1alpha1.EndpointUpstream{
						URL: "http://test-service:80",
					},
				},
			}

			// Mock endpoint creation failure
			envMockDriver.SetEndpointError(namespace+"/fail-runtime-auto", errors.New("creation failed"))

			By("Creating the AgentEndpoint")
			Expect(k8sClient.Create(ctx, agentEndpoint)).To(Succeed())

			By("Waiting for controller to automatically reconcile and set error condition")
			Eventually(func(g Gomega) {
				obj := &ngrokv1alpha1.AgentEndpoint{}
				g.Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(agentEndpoint), obj)).To(Succeed())

				// Check ready condition set by running controller
				cond := testutils.FindCondition(obj.Status.Conditions, ConditionReady)
				g.Expect(cond).NotTo(BeNil())
				g.Expect(cond.Status).To(Equal(metav1.ConditionFalse))
				g.Expect(cond.Reason).To(Equal(ReasonNgrokAPIError))
			}, timeout, interval).Should(Succeed())
		})

		It("should handle endpoint deletion with runtime controller", func(ctx SpecContext) {
			agentEndpoint = &ngrokv1alpha1.AgentEndpoint{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "delete-runtime-auto",
					Namespace: namespace,
				},
				Spec: ngrokv1alpha1.AgentEndpointSpec{
					URL: "tcp://97.tcp.ngrok.io:97979",
					Upstream: ngrokv1alpha1.EndpointUpstream{
						URL: "http://test-service:80",
					},
				},
			}

			// Mock successful creation
			envMockDriver.SetEndpointResult(namespace+"/delete-runtime-auto", &agent.EndpointResult{
				URL: "tcp://97.tcp.ngrok.io:97979",
			})

			By("Creating the AgentEndpoint")
			Expect(k8sClient.Create(ctx, agentEndpoint)).To(Succeed())

			By("Waiting for creation to complete")
			Eventually(func() bool {
				// Check that this specific endpoint was created
				for _, call := range envMockDriver.CreateCalls {
					if call.Name == namespace+"/delete-runtime-auto" {
						return true
					}
				}
				return false
			}, timeout, interval).Should(BeTrue())

			By("Deleting the AgentEndpoint")
			Expect(k8sClient.Delete(ctx, agentEndpoint)).To(Succeed())

			By("Waiting for controller to call delete on the driver")
			Eventually(func() bool {
				// Check that this specific endpoint was deleted
				for _, call := range envMockDriver.DeleteCalls {
					if call.Name == namespace+"/delete-runtime-auto" {
						return true
					}
				}
				return false
			}, timeout, interval).Should(BeTrue())
		})

		It("should reconcile traffic policy reference with runtime controller", func(ctx SpecContext) {
			// Create traffic policy first
			trafficPolicy := &ngrokv1alpha1.NgrokTrafficPolicy{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "runtime-auto-policy",
					Namespace: namespace,
				},
				Spec: ngrokv1alpha1.NgrokTrafficPolicySpec{
					Policy: []byte(`{"on_http_request":[{"name":"rate-limit"}]}`),
				},
			}
			Expect(k8sClient.Create(ctx, trafficPolicy)).To(Succeed())

			agentEndpoint = &ngrokv1alpha1.AgentEndpoint{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "policy-runtime-auto",
					Namespace: namespace,
				},
				Spec: ngrokv1alpha1.AgentEndpointSpec{
					URL: "tcp://96.tcp.ngrok.io:96969",
					Upstream: ngrokv1alpha1.EndpointUpstream{
						URL: "http://test-service:80",
					},
					TrafficPolicy: &ngrokv1alpha1.TrafficPolicyCfg{
						Reference: &ngrokv1alpha1.K8sObjectRef{
							Name: "runtime-auto-policy",
						},
					},
				},
			}

			// Mock successful endpoint creation
			envMockDriver.SetEndpointResult(namespace+"/policy-runtime-auto", &agent.EndpointResult{
				URL: "tcp://96.tcp.ngrok.io:96969",
			})

			By("Creating the AgentEndpoint")
			Expect(k8sClient.Create(ctx, agentEndpoint)).To(Succeed())

			By("Waiting for controller to reconcile and apply traffic policy")
			Eventually(func(g Gomega) {
				obj := &ngrokv1alpha1.AgentEndpoint{}
				g.Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(agentEndpoint), obj)).To(Succeed())

				// Check ready condition
				cond := testutils.FindCondition(obj.Status.Conditions, ConditionReady)
				g.Expect(cond).NotTo(BeNil())
				g.Expect(cond.Status).To(Equal(metav1.ConditionTrue))

				// Verify traffic policy status
				g.Expect(obj.Status.AttachedTrafficPolicy).To(Equal("runtime-auto-policy"))
			}, timeout, interval).Should(Succeed())

			By("Verifying the mock driver received the traffic policy for this endpoint")
			var foundCall *agent.CreateCall
			Eventually(func() bool {
				for _, call := range envMockDriver.CreateCalls {
					if call.Name == namespace+"/policy-runtime-auto" {
						foundCall = &call
						return true
					}
				}
				return false
			}, 5*time.Second, interval).Should(BeTrue())

			Expect(foundCall.TrafficPolicy).To(ContainSubstring("rate-limit"))
		})
	})

	Context("Client certificate secret watch behavior", func() {
		const testCert = `-----BEGIN CERTIFICATE-----
MIIC/zCCAeegAwIBAgIUMC1u8LkeVxeTak8pRSvi2usKbvAwDQYJKoZIhvcNAQEL
BQAwDzENMAsGA1UEAwwEdGVzdDAeFw0yNTA5MTEyMzE2NTNaFw0yNjA5MTEyMzE2
NTNaMA8xDTALBgNVBAMMBHRlc3QwggEiMA0GCSqGSIb3DQEBAQUAA4IBDwAwggEK
AoIBAQC+cPOgW2avpjo5aJK2yNNfFEUSx/2+/rl13LT4sJUkPE9SdYODlYTkUnQ+
Kbaxo3zcIHipkon4lgxUyfVowQn8jFtGlIgdWEWqSM+AIfb6+2fCtiy9+GStgcvv
fJn/Gs8SvB0vb0kdZ0gIP57mhg7ky5d/DUb8PuAN2KyHRWPm/LrVLxVg2N1lHXuZ
k6CWDYC/hk0uM/A0CPTDQF+sJfSV9LBvdaUMRkbY1z3sUO7bnsdtU+bVjj6zpWor
BS4ycX/UL7GWXYE5K1s+gaMwfQ8vGarI91p+arBV8eeWmhs29WvDwCo0C19rFetQ
y2JfZkRiq65NiUwlfQ943UIP9iSnAgMBAAGjUzBRMB0GA1UdDgQWBBSr6a8SDz5e
Oo9S9Rl+R5dq6pyOLzAfBgNVHSMEGDAWgBSr6a8SDz5eOo9S9Rl+R5dq6pyOLzAP
BgNVHRMBAf8EBTADAQH/MA0GCSqGSIb3DQEBCwUAA4IBAQAvx42jnrJZJwtc2ElU
OBtp0YLQipD5gWo0pLzbxAQb0UnngElDNe5BCZqzcWm26bQ3RGLSi72nuHcpOALX
ahuV/k13LZZeM/aIUhoHoMhCma5WDhlvDUNmIukAI5RrnMTvi7vaso9eUAZO1VPx
4YXYx0F5O1YlR6NiLVWQEAR8hzRS0QVEwvWS+5ncVH3XB9OoVXFZ7LAD7uQRS5Gy
AoICy/E6NLhgyLqdn0rpSS1pj8QIZe6x7qghntyHHN50Dm402lrhrrgr0Rw1X0Ua
NPBpoWqPs08ADzHDQRxlbTkkGYLX2VsSRC9Nwz1z1ol6fT/ytZ2MP+qYJccGvscx
KQxp
-----END CERTIFICATE-----`

		const testKey = `-----BEGIN PRIVATE KEY-----
MIIEvgIBADANBgkqhkiG9w0BAQEFAASCBKgwggSkAgEAAoIBAQC+cPOgW2avpjo5
aJK2yNNfFEUSx/2+/rl13LT4sJUkPE9SdYODlYTkUnQ+Kbaxo3zcIHipkon4lgxU
yfVowQn8jFtGlIgdWEWqSM+AIfb6+2fCtiy9+GStgcvvfJn/Gs8SvB0vb0kdZ0gI
P57mhg7ky5d/DUb8PuAN2KyHRWPm/LrVLxVg2N1lHXuZk6CWDYC/hk0uM/A0CPTD
QF+sJfSV9LBvdaUMRkbY1z3sUO7bnsdtU+bVjj6zpWorBS4ycX/UL7GWXYE5K1s+
gaMwfQ8vGarI91p+arBV8eeWmhs29WvDwCo0C19rFetQy2JfZkRiq65NiUwlfQ94
3UIP9iSnAgMBAAECggEAEMBux1RC6Obku3GinJSqj/QaGPGhzs/2EGALRM6DnMRe
Iqq9Mf3pyqVWHegdbrkq0OvD5JndrET50+hP8svIM4B4WiUGmoFnrCrrb1UnHIl9
mKsQ4UUDb7lTIj9obGads5z7rdVKlF0mNCBXu39fUzdCN3LVI6sdzYBmhlLPdUOI
pU4PjBAIOu3BR262W6SaG1JJPn/ZTfgRXKEpnEJCZFqEfaIIcIdl2NfncvVbF8Hi
pwEYLyGTp0AQezhUsxq8oNHCKVv2tLTsmKziodvtaMjuUy2yXnWrak67CEMakfnI
2ShlGYuGSslvqmZ2j4gz/DpoQFKM1KuKCL9COUj9QQKBgQDgYqCc6ZmBcPsRANSN
fgvQAcwZv7UWDVKa/iIZv6aFjcvXPevfdP5k6BN6a77b/BgHoQVQYuE+m10R2MIV
AfLSypYbhXWPnVW+LngKdsMULMJHEUqy2ibUgLb1iIIdMhmrhDK2HSx1lUjoNyHd
zARdER7NsAJgsy9HdOyErz5iwwKBgQDZRf5Nz7nN3EeIIxW2w4lbe3Q5pjkET5PH
Mclfi8QzaO1pWePhVFtsRJBPIlsGJBVirjpNiecRd0GJM4gX5mmoDEzuUO+dT7f/
gUyqMGBGQcsZ/wuMB/L9TKTuVYM51Q9sIl22gf1CgwmU3tffQYO1z89TPjFtuCGl
AnZBpvzQTQKBgQCglhVql0hUOj6E2bpFFTtw/4hJuUjpYlmHMW/IS7/qfyOuhNNl
lj5miy09hRUQLWgpNZUvBcU8YEaIej/UdxOIxpINWkNbp/dwZ6NjocFVk/7qi7aR
L81wcjn+mVa9fFigxrjgWxqxgEiwYJytNtC8pn8MJ/ZbrIGeu1B2WVDlrwKBgB4g
PV2OouWvWF/A9Z7Mx/veR0RDDv7RBd2FwrUzzPWP4/NKmnVA3BhL/XJrghF86VYw
cDcWGurqDTU35vPhZ978LaKRqFe4mPudcwLaCE9VihLFsVUuOPv0J55ATxyytRu6
PCI1LeeOAcMZjvcOv3NzJ/0Tz4i2Ejwt9jWuMLm1AoGBAIOoXDYDI2DVHX0eOmDl
385e8iuwqrfj2jtCEJHH4cPt+2M5LsPpkD8BDmyRr5JzGgA118qSmCJwEDp9SMBY
hzBRRr30H7ehjAmTAyWu81tPtJLtuWP/DByCgzxgxHSuMNoM38iLY9AomFZY2Sxn
cCzFoVcb6XWg4MpPeZ25v+xA
-----END PRIVATE KEY-----`

		It("should reconcile when client certificate secret is created after AgentEndpoint", func(ctx SpecContext) {
			// Create AgentEndpoint first, without the certificate secret existing yet
			agentEndpoint = &ngrokv1alpha1.AgentEndpoint{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "cert-watch-endpoint",
					Namespace: namespace,
				},
				Spec: ngrokv1alpha1.AgentEndpointSpec{
					URL: "tcp://cert-watch.tcp.ngrok.io:12345",
					Upstream: ngrokv1alpha1.EndpointUpstream{
						URL: "http://test-service:80",
					},
					ClientCertificateRefs: []ngrokv1alpha1.K8sObjectRefOptionalNamespace{
						{Name: "watch-test-cert"},
					},
				},
			}

			By("Creating the AgentEndpoint (before certificate exists)")
			Expect(k8sClient.Create(ctx, agentEndpoint)).To(Succeed())

			By("Waiting for controller to reconcile and set config error condition")
			Eventually(func(g Gomega) {
				obj := &ngrokv1alpha1.AgentEndpoint{}
				g.Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(agentEndpoint), obj)).To(Succeed())

				// Should have config error because certificate doesn't exist yet
				cond := testutils.FindCondition(obj.Status.Conditions, ConditionReady)
				g.Expect(cond).NotTo(BeNil())
				g.Expect(cond.Status).To(Equal(metav1.ConditionFalse))
				g.Expect(cond.Reason).To(Equal(ReasonConfigError))
			}, timeout, interval).Should(Succeed())

			By("Creating the client certificate secret")
			certSecret := &v1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "watch-test-cert",
					Namespace: namespace,
				},
				Type: v1.SecretTypeTLS,
				Data: map[string][]byte{
					"tls.crt": []byte(testCert),
					"tls.key": []byte(testKey),
				},
			}
			Expect(k8sClient.Create(ctx, certSecret)).To(Succeed())

			// Mock successful endpoint creation for when certificate becomes available
			envMockDriver.SetEndpointResult(namespace+"/cert-watch-endpoint", &agent.EndpointResult{
				URL: "tcp://cert-watch.tcp.ngrok.io:12345",
			})

			By("Waiting for controller to automatically reconcile AgentEndpoint when secret is created")
			Eventually(func(g Gomega) {
				obj := &ngrokv1alpha1.AgentEndpoint{}
				g.Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(agentEndpoint), obj)).To(Succeed())

				// Should now be ready because certificate exists
				cond := testutils.FindCondition(obj.Status.Conditions, ConditionReady)
				g.Expect(cond).NotTo(BeNil())
				g.Expect(cond.Status).To(Equal(metav1.ConditionTrue))
				g.Expect(cond.Reason).To(Equal(ReasonEndpointActive))

				// Verify status was updated
				g.Expect(obj.Status.AssignedURL).To(Equal("tcp://cert-watch.tcp.ngrok.io:12345"))
			}, timeout, interval).Should(Succeed())

			By("Verifying that the controller was triggered to create the endpoint")
			Eventually(func() bool {
				for _, call := range envMockDriver.CreateCalls {
					if call.Name == namespace+"/cert-watch-endpoint" {
						return true
					}
				}
				return false
			}, 5*time.Second, interval).Should(BeTrue())
		})
	})

	Context("Domain handling", func() {
		It("should skip domain creation for Kubernetes-bound endpoint", func(ctx SpecContext) {
			agentEndpoint = &ngrokv1alpha1.AgentEndpoint{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "k8s-bound-endpoint",
					Namespace: namespace,
				},
				Spec: ngrokv1alpha1.AgentEndpointSpec{
					URL:      "http://aws.demo",
					Bindings: []string{"kubernetes"},
					Upstream: ngrokv1alpha1.EndpointUpstream{
						URL: "http://hello-aws.demo:80",
					},
				},
			}

			// Setup mock driver to return success
			envMockDriver.SetEndpointResult(namespace+"/k8s-bound-endpoint", &agent.EndpointResult{
				URL: "http://aws.demo",
			})

			By("Creating the AgentEndpoint")
			Expect(k8sClient.Create(ctx, agentEndpoint)).To(Succeed())

			By("Verifying endpoint becomes ready without domain creation")
			Eventually(func(g Gomega) {
				obj := &ngrokv1alpha1.AgentEndpoint{}
				g.Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(agentEndpoint), obj)).To(Succeed())

				// Endpoint should be created
				g.Expect(obj.Status.AssignedURL).To(Equal("http://aws.demo"))

				// No domain ref should be set (kubernetes binding skips domain)
				g.Expect(obj.Status.DomainRef).To(BeNil())

				// DomainReady condition should be True (no domain reservation needed)
				domainCond := testutils.FindCondition(obj.Status.Conditions, domainpkg.ConditionEndpointDomainReady)
				g.Expect(domainCond).NotTo(BeNil())
				g.Expect(domainCond.Status).To(Equal(metav1.ConditionTrue))
				g.Expect(domainCond.Message).To(ContainSubstring("Kubernetes binding"))

				// Ready condition should be True (all conditions satisfied)
				readyCond := testutils.FindCondition(obj.Status.Conditions, ConditionReady)
				g.Expect(readyCond).NotTo(BeNil())
				g.Expect(readyCond.Status).To(Equal(metav1.ConditionTrue))

				// Verify no Domain CRD was created
				domains := &ingressv1alpha1.DomainList{}
				g.Expect(k8sClient.List(ctx, domains, client.InNamespace(namespace))).To(Succeed())
				g.Expect(domains.Items).To(BeEmpty())
			}, timeout, interval).Should(Succeed())

			By("Verifying mock driver was called to create endpoint")
			Eventually(func() bool {
				for _, call := range envMockDriver.CreateCalls {
					if call.Name == namespace+"/k8s-bound-endpoint" {
						return true
					}
				}
				return false
			}, timeout, interval).Should(BeTrue())
		})

		When("the domainRef for kubernetes-bound endpoint is stale", func() {
			var staleDomain *ingressv1alpha1.Domain
			BeforeEach(func(ctx SpecContext) {
				By("Creating a domain that would be stale")
				staleDomain = &ingressv1alpha1.Domain{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "stale-domain-ref",
						Namespace: namespace,
					},
					Spec: ingressv1alpha1.DomainSpec{
						Domain: "test.default",
					},
				}
				Expect(k8sClient.Create(ctx, staleDomain)).To(Succeed())

				agentEndpoint = &ngrokv1alpha1.AgentEndpoint{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "k8s-binding-with-stale-ref",
						Namespace: namespace,
					},
					Spec: ngrokv1alpha1.AgentEndpointSpec{
						URL:      "http://test.default",
						Bindings: []string{"kubernetes"},
						Upstream: ngrokv1alpha1.EndpointUpstream{
							URL: "http://test-service:80",
						},
					},
					Status: ngrokv1alpha1.AgentEndpointStatus{
						DomainRef: &ngrokv1alpha1.K8sObjectRefOptionalNamespace{
							Name:      staleDomain.GetName(),
							Namespace: ptr.To(staleDomain.GetNamespace()),
						},
					},
				}
				envMockDriver.SetEndpointResult(namespace+"/k8s-binding-with-stale-ref", &agent.EndpointResult{
					URL: "http://test.default",
				})

				By("Creating the AgentEndpoint with stale domainRef")
				Expect(k8sClient.Create(ctx, agentEndpoint)).To(Succeed())
			})

			It("should clear the stale domainRef", func(ctx SpecContext) {
				Eventually(func(g Gomega) {
					obj := &ngrokv1alpha1.AgentEndpoint{}
					g.Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(agentEndpoint), obj)).To(Succeed())

					g.Expect(obj.Status.DomainRef).To(BeNil())

					readyCond := testutils.FindCondition(obj.Status.Conditions, ConditionReady)
					g.Expect(readyCond).NotTo(BeNil())
					g.Expect(readyCond.Status).To(Equal(metav1.ConditionTrue))
				}, timeout, interval).Should(Succeed())
			})
		})

		It("should create endpoint even when domain is not ready", func(ctx SpecContext) {
			agentEndpoint = &ngrokv1alpha1.AgentEndpoint{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-endpoint",
					Namespace: namespace,
				},
				Spec: ngrokv1alpha1.AgentEndpointSpec{
					URL: "http://example.com",
					Upstream: ngrokv1alpha1.EndpointUpstream{
						URL: "http://test-service:80",
					},
				},
			}

			envMockDriver.SetEndpointResult(namespace+"/test-endpoint", &agent.EndpointResult{
				URL: "http://example.com",
			})

			By("Creating the AgentEndpoint")
			Expect(k8sClient.Create(ctx, agentEndpoint)).To(Succeed())

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
				obj := &ngrokv1alpha1.AgentEndpoint{}
				g.Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(agentEndpoint), obj)).To(Succeed())

				// Endpoint should be created
				g.Expect(obj.Status.AssignedURL).To(Equal("http://example.com"))

				// DomainReady should be False (domain exists but not ready yet)
				domainCond := testutils.FindCondition(obj.Status.Conditions, domainpkg.ConditionEndpointDomainReady)
				g.Expect(domainCond).NotTo(BeNil())
				g.Expect(domainCond.Status).To(Equal(metav1.ConditionFalse))

				// Ready should be False
				readyCond := testutils.FindCondition(obj.Status.Conditions, ConditionReady)
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
				obj := &ngrokv1alpha1.AgentEndpoint{}
				g.Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(agentEndpoint), obj)).To(Succeed())

				// DomainReady should be True
				domainCond := testutils.FindCondition(obj.Status.Conditions, domainpkg.ConditionEndpointDomainReady)
				g.Expect(domainCond).NotTo(BeNil())
				g.Expect(domainCond.Status).To(Equal(metav1.ConditionTrue))

				// Ready should be True
				readyCond := testutils.FindCondition(obj.Status.Conditions, ConditionReady)
				g.Expect(readyCond).NotTo(BeNil())
				g.Expect(readyCond.Status).To(Equal(metav1.ConditionTrue))
			}, 45*time.Second, interval).Should(Succeed())

			By("Verifying mock driver was called to create endpoint")
			Eventually(func() bool {
				for _, call := range envMockDriver.CreateCalls {
					if call.Name == namespace+"/test-endpoint" {
						return true
					}
				}
				return false
			}, timeout, interval).Should(BeTrue())
		})

		It("should reconcile endpoint when domain status becomes ready", func(ctx SpecContext) {
			agentEndpoint = &ngrokv1alpha1.AgentEndpoint{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-domain-reconcile",
					Namespace: namespace,
				},
				Spec: ngrokv1alpha1.AgentEndpointSpec{
					URL: "https://test-reconcile.example.com",
					Upstream: ngrokv1alpha1.EndpointUpstream{
						URL: "http://test-service:80",
					},
				},
			}

			envMockDriver.SetEndpointResult(namespace+"/test-domain-reconcile", &agent.EndpointResult{
				URL: "https://test-reconcile.example.com",
			})

			By("Creating the AgentEndpoint")
			Expect(k8sClient.Create(ctx, agentEndpoint)).To(Succeed())

			By("Waiting for Domain to be created")
			var domain *ingressv1alpha1.Domain
			Eventually(func(g Gomega) {
				domains := &ingressv1alpha1.DomainList{}
				g.Expect(k8sClient.List(ctx, domains, client.InNamespace(namespace))).To(Succeed())
				g.Expect(domains.Items).To(HaveLen(1))
				domain = &domains.Items[0]
				g.Expect(domain.Spec.Domain).To(Equal("test-reconcile.example.com"))
			}, timeout, interval).Should(Succeed())

			By("Verifying endpoint is not ready (domain not ready)")
			Eventually(func(g Gomega) {
				obj := &ngrokv1alpha1.AgentEndpoint{}
				g.Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(agentEndpoint), obj)).To(Succeed())

				domainCond := testutils.FindCondition(obj.Status.Conditions, domainpkg.ConditionEndpointDomainReady)
				g.Expect(domainCond).NotTo(BeNil())
				g.Expect(domainCond.Status).To(Equal(metav1.ConditionFalse))
			}, timeout, interval).Should(Succeed())

			By("Updating domain status to ready")
			Eventually(func(g Gomega) {
				latestDomain := &ingressv1alpha1.Domain{}
				g.Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(domain), latestDomain)).To(Succeed())
				latestDomain.Status.ID = "dom_456"
				latestDomain.Status.CNAMETarget = ptr.To("test.ngrok-cname.com")
				latestDomain.Status.Conditions = []metav1.Condition{
					{
						Type:               "Ready",
						Status:             metav1.ConditionTrue,
						Reason:             "DomainActive",
						Message:            "Domain is active",
						LastTransitionTime: metav1.Now(),
						ObservedGeneration: latestDomain.Generation,
					},
				}
				g.Expect(k8sClient.Status().Update(ctx, latestDomain)).To(Succeed())
			}, timeout, interval).Should(Succeed())

			By("Verifying endpoint becomes ready after domain status update")
			Eventually(func(g Gomega) {
				obj := &ngrokv1alpha1.AgentEndpoint{}
				g.Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(agentEndpoint), obj)).To(Succeed())

				// DomainReady should be True
				domainCond := testutils.FindCondition(obj.Status.Conditions, domainpkg.ConditionEndpointDomainReady)
				g.Expect(domainCond).NotTo(BeNil())
				g.Expect(domainCond.Status).To(Equal(metav1.ConditionTrue))

				// Ready should be True
				readyCond := testutils.FindCondition(obj.Status.Conditions, ConditionReady)
				g.Expect(readyCond).NotTo(BeNil())
				g.Expect(readyCond.Status).To(Equal(metav1.ConditionTrue))
			}, 45*time.Second, interval).Should(Succeed())
		})

		It("should clear stale domainRef when endpoint has kubernetes binding", func(ctx SpecContext) {
			agentEndpoint = &ngrokv1alpha1.AgentEndpoint{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "endpoint-with-stale-ref",
					Namespace: namespace,
				},
				Spec: ngrokv1alpha1.AgentEndpointSpec{
					URL:      "http://stale.example.com",
					Bindings: []string{"kubernetes"},
					Upstream: ngrokv1alpha1.EndpointUpstream{
						URL: "http://test-service:80",
					},
				},
			}

			envMockDriver.SetEndpointResult(namespace+"/endpoint-with-stale-ref", &agent.EndpointResult{
				URL: "http://stale.example.com",
			})

			By("Creating the AgentEndpoint")
			Expect(k8sClient.Create(ctx, agentEndpoint)).To(Succeed())

			By("Waiting for endpoint to be created and ready (no domain needed for k8s binding)")
			Eventually(func(g Gomega) {
				obj := &ngrokv1alpha1.AgentEndpoint{}
				g.Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(agentEndpoint), obj)).To(Succeed())
				g.Expect(obj.Status.DomainRef).To(BeNil())

				readyCond := testutils.FindCondition(obj.Status.Conditions, ConditionReady)
				g.Expect(readyCond).NotTo(BeNil())
				g.Expect(readyCond.Status).To(Equal(metav1.ConditionTrue))
			}, timeout, interval).Should(Succeed())

			By("Simulating a stale domainRef by updating status")
			Eventually(func(g Gomega) {
				obj := &ngrokv1alpha1.AgentEndpoint{}
				g.Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(agentEndpoint), obj)).To(Succeed())
				obj.Status.DomainRef = &ngrokv1alpha1.K8sObjectRefOptionalNamespace{
					Name:      "stale-example-com",
					Namespace: ptr.To(namespace),
				}
				g.Expect(k8sClient.Status().Update(ctx, obj)).To(Succeed())
			}, timeout, interval).Should(Succeed())

			By("Verifying stale domainRef is cleared on next reconcile")
			// Touch the endpoint to trigger reconcile
			Eventually(func(g Gomega) {
				obj := &ngrokv1alpha1.AgentEndpoint{}
				g.Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(agentEndpoint), obj)).To(Succeed())
				if obj.Annotations == nil {
					obj.Annotations = map[string]string{}
				}
				obj.Annotations["test.trigger"] = "reconcile"
				g.Expect(k8sClient.Update(ctx, obj)).To(Succeed())
			}, timeout, interval).Should(Succeed())

			Eventually(func(g Gomega) {
				obj := &ngrokv1alpha1.AgentEndpoint{}
				g.Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(agentEndpoint), obj)).To(Succeed())
				g.Expect(obj.Status.DomainRef).To(BeNil())
			}, timeout, interval).Should(Succeed())
		})

		It("should delete stale domain when endpoint has kubernetes binding and domainRef", func(ctx SpecContext) {
			// Create domain first
			staleDomain := &ingressv1alpha1.Domain{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "stale-to-delete-example-com",
					Namespace: namespace,
				},
				Spec: ingressv1alpha1.DomainSpec{
					Domain: "stale-to-delete.example.com",
				},
			}
			Expect(k8sClient.Create(ctx, staleDomain)).To(Succeed())

			agentEndpoint = &ngrokv1alpha1.AgentEndpoint{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "endpoint-triggers-domain-delete",
					Namespace: namespace,
				},
				Spec: ngrokv1alpha1.AgentEndpointSpec{
					URL:      "https://stale-to-delete.example.com",
					Bindings: []string{"kubernetes"},
					Upstream: ngrokv1alpha1.EndpointUpstream{
						URL: "http://test-service:80",
					},
				},
			}

			envMockDriver.SetEndpointResult(namespace+"/endpoint-triggers-domain-delete", &agent.EndpointResult{
				URL: "https://stale-to-delete.example.com",
			})

			By("Creating the AgentEndpoint with kubernetes binding")
			Expect(k8sClient.Create(ctx, agentEndpoint)).To(Succeed())

			By("Waiting for endpoint to become ready initially")
			Eventually(func(g Gomega) {
				obj := &ngrokv1alpha1.AgentEndpoint{}
				g.Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(agentEndpoint), obj)).To(Succeed())

				readyCond := testutils.FindCondition(obj.Status.Conditions, ConditionReady)
				g.Expect(readyCond).NotTo(BeNil())
				g.Expect(readyCond.Status).To(Equal(metav1.ConditionTrue))
			}, timeout, interval).Should(Succeed())

			By("Simulating stale domainRef by setting it (as if domain was created before binding was added)")
			Eventually(func(g Gomega) {
				obj := &ngrokv1alpha1.AgentEndpoint{}
				g.Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(agentEndpoint), obj)).To(Succeed())
				obj.Status.DomainRef = &ngrokv1alpha1.K8sObjectRefOptionalNamespace{
					Name:      staleDomain.Name,
					Namespace: ptr.To(namespace),
				}
				g.Expect(k8sClient.Status().Update(ctx, obj)).To(Succeed())
			}, timeout, interval).Should(Succeed())

			By("Triggering reconcile by updating annotation")
			Eventually(func(g Gomega) {
				obj := &ngrokv1alpha1.AgentEndpoint{}
				g.Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(agentEndpoint), obj)).To(Succeed())
				if obj.Annotations == nil {
					obj.Annotations = map[string]string{}
				}
				obj.Annotations["test.trigger"] = "reconcile-domain-delete"
				g.Expect(k8sClient.Update(ctx, obj)).To(Succeed())
			}, timeout, interval).Should(Succeed())

			By("Verifying domain has deletion timestamp or is deleted")
			Eventually(func(g Gomega) {
				latestDomain := &ingressv1alpha1.Domain{}
				err := k8sClient.Get(ctx, client.ObjectKeyFromObject(staleDomain), latestDomain)
				if err != nil {
					// Domain deleted if there's no finalizer
					g.Expect(client.IgnoreNotFound(err)).To(Succeed())
				} else {
					// Domain exists but should have DeletionTimestamp set
					g.Expect(latestDomain.DeletionTimestamp).NotTo(BeNil(), "Domain should be marked for deletion")
				}
			}, timeout, interval).Should(Succeed())

			By("Verifying domainRef is cleared after reconciliation")
			Eventually(func(g Gomega) {
				obj := &ngrokv1alpha1.AgentEndpoint{}
				g.Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(agentEndpoint), obj)).To(Succeed())
				g.Expect(obj.Status.DomainRef).To(BeNil())
			}, timeout, interval).Should(Succeed())
		})

		It("should handle multiple Kubernetes-bound endpoints with same domain", func(ctx SpecContext) {
			endpoints := []*ngrokv1alpha1.AgentEndpoint{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "k8s-endpoint-1",
						Namespace: namespace,
					},
					Spec: ngrokv1alpha1.AgentEndpointSpec{
						URL:      "http://aws.demo",
						Bindings: []string{"kubernetes"},
						Upstream: ngrokv1alpha1.EndpointUpstream{
							URL: "http://service-1:80",
						},
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "k8s-endpoint-2",
						Namespace: namespace,
					},
					Spec: ngrokv1alpha1.AgentEndpointSpec{
						URL:      "http://aws.demo",
						Bindings: []string{"kubernetes"},
						Upstream: ngrokv1alpha1.EndpointUpstream{
							URL: "http://service-2:80",
						},
					},
				},
			}

			// Setup mock results
			envMockDriver.SetEndpointResult(namespace+"/k8s-endpoint-1", &agent.EndpointResult{URL: "http://aws.demo"})
			envMockDriver.SetEndpointResult(namespace+"/k8s-endpoint-2", &agent.EndpointResult{URL: "http://aws.demo"})

			By("Creating multiple endpoints with same domain")
			for _, ep := range endpoints {
				Expect(k8sClient.Create(ctx, ep)).To(Succeed())
			}

			By("Verifying all endpoints become ready without domain conflicts")
			for _, ep := range endpoints {
				Eventually(func(g Gomega) {
					obj := &ngrokv1alpha1.AgentEndpoint{}
					g.Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(ep), obj)).To(Succeed())

					readyCond := testutils.FindCondition(obj.Status.Conditions, ConditionReady)
					g.Expect(readyCond).NotTo(BeNil())
					g.Expect(readyCond.Status).To(Equal(metav1.ConditionTrue))
				}, timeout, interval).Should(Succeed())
			}

			By("Verifying no Domain CRD was created")
			Eventually(func(g Gomega) {
				domains := &ingressv1alpha1.DomainList{}
				g.Expect(k8sClient.List(ctx, domains, client.InNamespace(namespace))).To(Succeed())
				g.Expect(domains.Items).To(BeEmpty())
			}, timeout, interval).Should(Succeed())

			By("Verifying mock driver was called for both endpoints")
			Eventually(func() int {
				count := 0
				for _, call := range envMockDriver.CreateCalls {
					if call.Name == namespace+"/k8s-endpoint-1" || call.Name == namespace+"/k8s-endpoint-2" {
						count++
					}
				}
				return count
			}, timeout, interval).Should(Equal(2))
		})
	})

	Context("Bindings validation", func() {
		It("should reject endpoint with multiple bindings", func() {
			// This should be caught by k8s validation (MaxItems=1), so we expect the Create to fail
			agentEndpoint = &ngrokv1alpha1.AgentEndpoint{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "invalid-multiple-bindings",
					Namespace: namespace,
				},
				Spec: ngrokv1alpha1.AgentEndpointSpec{
					URL:      "http://test.demo",
					Bindings: []string{"public", "internal"}, // Multiple bindings should be rejected
					Upstream: ngrokv1alpha1.EndpointUpstream{
						URL: "http://test-service:80",
					},
				},
			}

			err := k8sClient.Create(context.Background(), agentEndpoint)
			Expect(err).To(HaveOccurred()) // Should be rejected by validation
			Expect(err.Error()).To(Or(
				ContainSubstring("must have at most 1 item"),
				ContainSubstring("maxItems"),
			))
		})

		It("should accept endpoint with single binding", func(ctx SpecContext) {
			agentEndpoint = &ngrokv1alpha1.AgentEndpoint{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "valid-single-binding",
					Namespace: namespace,
				},
				Spec: ngrokv1alpha1.AgentEndpointSpec{
					URL:      "http://test.demo",
					Bindings: []string{"internal"}, // Single binding is valid
					Upstream: ngrokv1alpha1.EndpointUpstream{
						URL: "http://test-service:80",
					},
				},
			}

			// Setup mock driver to return success
			envMockDriver.SetEndpointResult(namespace+"/valid-single-binding", &agent.EndpointResult{
				URL: "http://test.demo",
			})

			By("Creating the AgentEndpoint with single binding")
			err := k8sClient.Create(ctx, agentEndpoint)
			Expect(err).NotTo(HaveOccurred()) // Should be accepted

			By("Verifying endpoint is created successfully")
			Eventually(func(g Gomega) {
				obj := &ngrokv1alpha1.AgentEndpoint{}
				g.Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(agentEndpoint), obj)).To(Succeed())
				g.Expect(obj.Spec.Bindings).To(Equal([]string{"internal"}))
			}, timeout, interval).Should(Succeed())
		})
	})
})

// findCondition finds a condition by type in a slice of conditions
// RandomString generates a random string of specified length for test isolation
func RandomString(length int) string {
	const charset = "abcdefghijklmnopqrstuvwxyz0123456789"
	b := make([]byte, length)
	for i := range b {
		b[i] = charset[rand.Intn(len(charset))]
	}
	return string(b)
}
