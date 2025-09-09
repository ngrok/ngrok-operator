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
	"github.com/ngrok/ngrok-operator/pkg/agent"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	"sigs.k8s.io/controller-runtime/pkg/client"
)

var _ = Describe("AgentEndpoint Controller Environment Tests", func() {
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
			k8sClient.Delete(context.Background(), ns)
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
				cond := findCondition(obj.Status.Conditions, ConditionReady)
				g.Expect(cond).NotTo(BeNil())
				g.Expect(cond.Status).To(Equal(metav1.ConditionTrue))
				g.Expect(cond.Reason).To(Equal(ReasonEndpointActive))

				// Verify status fields set by controller
				g.Expect(obj.Status.AssignedURL).To(Equal("tcp://1.tcp.ngrok.io:12345"))
				g.Expect(obj.Status.AttachedTrafficPolicy).To(Equal("none"))
			}, timeout, interval).Should(Succeed())
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

				cond := findCondition(obj.Status.Conditions, ConditionReady)
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
				domainCond := findCondition(obj.Status.Conditions, ConditionDomainReady)
				g.Expect(domainCond).NotTo(BeNil())
				g.Expect(domainCond.Status).To(Equal(metav1.ConditionFalse))
				g.Expect(domainCond.Reason).To(Equal(ReasonDomainCreating))
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
				cond := findCondition(obj.Status.Conditions, ConditionReady)
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

				cond := findCondition(obj.Status.Conditions, ConditionReady)
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

				cond := findCondition(obj.Status.Conditions, ConditionReady)
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

				policyCondition := findCondition(obj.Status.Conditions, ConditionTrafficPolicy)
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
				cond := findCondition(obj.Status.Conditions, ConditionReady)
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
				cond := findCondition(obj.Status.Conditions, ConditionReady)
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

				cond := findCondition(obj.Status.Conditions, ConditionReady)
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

				cond := findCondition(obj.Status.Conditions, ConditionReady)
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

				cond := findCondition(obj.Status.Conditions, ConditionReady)
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
				cond := findCondition(obj.Status.Conditions, ConditionReady)
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
				cond := findCondition(obj.Status.Conditions, ConditionReady)
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
				cond := findCondition(obj.Status.Conditions, ConditionReady)
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
})

// findCondition finds a condition by type in a slice of conditions
func findCondition(conditions []metav1.Condition, conditionType string) *metav1.Condition {
	for _, condition := range conditions {
		if condition.Type == conditionType {
			return &condition
		}
	}
	return nil
}

// RandomString generates a random string of specified length for test isolation
func RandomString(length int) string {
	const charset = "abcdefghijklmnopqrstuvwxyz0123456789"
	b := make([]byte, length)
	for i := range b {
		b[i] = charset[rand.Intn(len(charset))]
	}
	return string(b)
}
