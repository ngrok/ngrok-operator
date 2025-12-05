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
package service

import (
	"encoding/json"
	"fmt"
	"math/rand/v2"
	"time"

	ngrokv1alpha1 "github.com/ngrok/ngrok-operator/api/ngrok/v1alpha1"
	"github.com/ngrok/ngrok-operator/internal/annotations"
	"github.com/ngrok/ngrok-operator/internal/controller"
	"github.com/ngrok/ngrok-operator/internal/testutils"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	LoadBalancer = corev1.ServiceTypeLoadBalancer
	ClusterIP    = corev1.ServiceTypeClusterIP

	FinalizerName = controller.FinalizerName

	Annotation_URL             = annotations.URLAnnotation
	Annotation_MappingStrategy = annotations.MappingStrategyAnnotation
	Annotation_TrafficPolicy   = annotations.TrafficPolicyAnnotation
)

// getCloudEndpoints fetches CloudEndpoints in the given namespace
func getCloudEndpoints(k8sClient client.Client, namespace string) (*ngrokv1alpha1.CloudEndpointList, error) {
	clepList := &ngrokv1alpha1.CloudEndpointList{}
	listOpts := []client.ListOption{
		client.InNamespace(namespace),
	}
	err := k8sClient.List(ctx, clepList, listOpts...)
	return clepList, err
}

// getAgentEndpoints fetches AgentEndpoints in the given namespace
func getAgentEndpoints(k8sClient client.Client, namespace string) (*ngrokv1alpha1.AgentEndpointList, error) {
	aepList := &ngrokv1alpha1.AgentEndpointList{}
	listOpts := []client.ListOption{
		client.InNamespace(namespace),
	}
	err := k8sClient.List(ctx, aepList, listOpts...)
	return aepList, err
}

type ServiceModifier func(*corev1.Service)
type ServiceModifiers struct {
	mods []ServiceModifier
}

func (sm *ServiceModifiers) Add(modifier ServiceModifier) {
	sm.mods = append(sm.mods, modifier)
}

func (sm ServiceModifiers) Apply(svc *corev1.Service) {
	for _, modify := range sm.mods {
		modify(svc)
	}
}

func SetServiceType(svcType corev1.ServiceType) ServiceModifier {
	return func(svc *corev1.Service) {
		svc.Spec.Type = svcType
	}
}

func SetLoadBalancerClass(lbClass string) ServiceModifier {
	return func(svc *corev1.Service) {
		svc.Spec.LoadBalancerClass = ptr.To(lbClass)
	}
}

func AddAnnotation(key, value string) ServiceModifier {
	return func(svc *corev1.Service) {
		if svc.Annotations == nil {
			svc.Annotations = map[string]string{}
		}
		svc.Annotations[key] = value
	}
}

func SetMappingStrategy(strategy annotations.MappingStrategy) ServiceModifier {
	return func(svc *corev1.Service) {
		AddAnnotation(annotations.MappingStrategyAnnotation, string(strategy))(svc)
	}
}

var _ = Describe("ServiceController", func() {
	const (
		timeout  = 10 * time.Second
		duration = 10 * time.Second
		interval = 250 * time.Millisecond
	)

	var (
		namespace string
		svc       *corev1.Service

		modifiers *ServiceModifiers
	)

	BeforeEach(func() {
		modifiers = &ServiceModifiers{
			mods: []ServiceModifier{},
		}

		namespace = fmt.Sprintf("test-namespace-%d", rand.IntN(100000))
		kginkgo.ExpectCreateNamespace(ctx, namespace)
	})

	JustBeforeEach(func() {
		svc = &corev1.Service{
			ObjectMeta: metav1.ObjectMeta{
				Name:        fmt.Sprintf("test-service-%d", rand.IntN(100000)),
				Namespace:   namespace,
				Annotations: map[string]string{},
			},
			Spec: corev1.ServiceSpec{
				Type: corev1.ServiceTypeLoadBalancer,
				Ports: []corev1.ServicePort{
					{
						Name:     "tcp",
						Protocol: corev1.ProtocolTCP,
						Port:     80,
					},
				},
			},
		}
		modifiers.Apply(svc)
		Expect(k8sClient.Create(ctx, svc)).To(Succeed())
	})

	AfterEach(func(ctx SpecContext) {
		kginkgo.ExpectDeleteNamespace(ctx, namespace)
	})

	When("the service type is not a LoadBalancer", func() {
		BeforeEach(func() {
			modifiers.Add(SetServiceType(ClusterIP))
		})

		It("should ignore the service", func() {
			Consistently(func(g Gomega) {
				fetched := &corev1.Service{}
				err := k8sClient.Get(ctx, client.ObjectKeyFromObject(svc), fetched)
				g.Expect(err).NotTo(HaveOccurred())

				By("checking the service is not modified")
				g.Expect(fetched.Finalizers).To(BeEmpty())
			}, duration, interval).Should(Succeed())
		})
	})

	When("service type is LoadBalancer", func() {
		BeforeEach(func() {
			modifiers.Add(SetServiceType(LoadBalancer))
		})

		When("the service has a non-ngrok load balancer class", func() {
			It("should ignore the service", func() {
				Consistently(func(g Gomega) {
					fetched := &corev1.Service{}
					err := k8sClient.Get(ctx, client.ObjectKeyFromObject(svc), fetched)
					g.Expect(err).NotTo(HaveOccurred())

					By("checking the service is not modified")
					g.Expect(fetched.Finalizers).To(BeEmpty())
				}, duration, interval).Should(Succeed())
			})
		})

		When("the service has the ngrok load balancer class", func() {
			BeforeEach(func() {
				modifiers.Add(SetLoadBalancerClass(NgrokLoadBalancerClass))
			})

			It("should have a finalizer added", func() {
				Eventually(func(g Gomega) {
					fetched := &corev1.Service{}
					err := k8sClient.Get(ctx, client.ObjectKeyFromObject(svc), fetched)
					g.Expect(err).NotTo(HaveOccurred())

					By("checking the service has a finalizer added")
					g.Expect(fetched.Finalizers).To(ContainElement(FinalizerName))
				}, timeout, interval).Should(Succeed())
			})

			When("the service does not have a URL annotation", func() {
				It("Should reserve a TCP address", func() {
					kginkgo.EventuallyWithObject(ctx, svc.DeepCopy(), func(g Gomega, fetched client.Object) {
						By("checking the service has a URL annotation")
						GinkgoLogr.Info("Got service", "fetched", fetched)

						a := fetched.GetAnnotations()
						g.Expect(a).NotTo(BeEmpty())

						urlAnnotation, exists := a[annotations.ComputedURLAnnotation]
						g.Expect(exists).To(BeTrue())
						g.Expect(urlAnnotation).To(MatchRegexp(`^tcp://[a-zA-Z0-9\-\.]+:\d+$`))
					})
				})
			})

			When("endpoints verbose", func() {
				BeforeEach(func() {
					modifiers.Add(SetMappingStrategy(annotations.MappingStrategy_EndpointsVerbose))
				})

				It("Should create a cloud endpoint", func() {
					kginkgo.EventuallyWithCloudEndpoints(ctx, namespace, func(g Gomega, cleps []ngrokv1alpha1.CloudEndpoint) {
						By("checking a cloud endpoint exists")
						g.Expect(cleps).To(HaveLen(1))
					})
				})

				It("should update service status with hostname and port", func() {
					Eventually(func(g Gomega) {
						fetched := &corev1.Service{}
						err := k8sClient.Get(ctx, client.ObjectKeyFromObject(svc), fetched)
						g.Expect(err).NotTo(HaveOccurred())

						By("checking the service status is updated")
						g.Expect(fetched.Status.LoadBalancer.Ingress).NotTo(BeEmpty())
						g.Expect(fetched.Status.LoadBalancer.Ingress[0].Hostname).NotTo(BeEmpty())

						By("verifying the resource version does not change unnecessarily")
						kginkgo.ConsistentlyExpectResourceVersionNotToChange(ctx, svc, testutils.WithTimeout(10*time.Second))
					}, timeout, interval).Should(Succeed())
				})

				It("should create an agent endpoint for the cloud endpoint", func() {
					Eventually(func(g Gomega) {
						aeps, err := getAgentEndpoints(k8sClient, namespace)
						g.Expect(err).NotTo(HaveOccurred())

						By("checking an agent endpoint exists")
						g.Expect(aeps.Items).To(HaveLen(1))
					}, timeout, interval).Should(Succeed())
				})

				It("should create an agent endpoint with .internal suffix in URL", func() {
					kginkgo.EventuallyWithAgentEndpoints(ctx, namespace, func(g Gomega, aeps []ngrokv1alpha1.AgentEndpoint) {
						By("checking an agent endpoint exists")
						g.Expect(aeps).To(HaveLen(1))

						By("checking the agent endpoint URL has .internal suffix")
						aep := aeps[0]
						g.Expect(aep.Spec.URL).To(ContainSubstring(".internal"))
					})
				})

				When("the service is deleted", func() {
					It("should clean up all owned resources", func() {
						kginkgo.ExpectFinalizerToBeAdded(ctx, svc, FinalizerName)

						// Wait for resources to be created
						kginkgo.EventuallyWithCloudEndpoints(ctx, namespace, func(g Gomega, cleps []ngrokv1alpha1.CloudEndpoint) {
							By("checking a cloud endpoint exists")
							g.Expect(cleps).To(HaveLen(1))
						})

						By("deleting the service")
						Expect(k8sClient.Delete(ctx, svc)).To(Succeed())

						// Verify all owned resources are cleaned up
						kginkgo.EventuallyExpectNoEndpoints(ctx, namespace)
					})

					It("should remove the finalizer after cleanup", func() {
						// Wait for finalizer to be added
						kginkgo.ExpectFinalizerToBeAdded(ctx, svc, FinalizerName)

						// Delete the service
						Expect(k8sClient.Delete(ctx, svc)).To(Succeed())

						// Verify service is fully deleted (finalizer removed)
						kginkgo.ExpectFinalizerToBeRemoved(ctx, svc, FinalizerName)
					})
				})
			})

			When("endpoints default (no annotation)", func() {
				It("Should not create a cloud endpoint", func() {
					kginkgo.ConsistentlyWithCloudEndpoints(ctx, namespace, func(g Gomega, cleps []ngrokv1alpha1.CloudEndpoint) {
						By("checking no cloud endpoints exist")
						g.Expect(cleps).To(BeEmpty())
					})
				})

				It("Should create an agent endpoint", func() {
					kginkgo.EventuallyWithAgentEndpoints(ctx, namespace, func(g Gomega, aeps []ngrokv1alpha1.AgentEndpoint) {
						By("checking an agent endpoint exists")
						g.Expect(aeps).To(HaveLen(1))
					})
				})

				It("Should update service status with hostname and port", func() {
					Eventually(func(g Gomega) {
						fetched := &corev1.Service{}
						err := k8sClient.Get(ctx, client.ObjectKeyFromObject(svc), fetched)
						g.Expect(err).NotTo(HaveOccurred())

						By("checking the service status is updated")
						g.Expect(fetched.Status.LoadBalancer.Ingress).NotTo(BeEmpty())
						g.Expect(fetched.Status.LoadBalancer.Ingress[0].Hostname).NotTo(BeEmpty())
					}).WithTimeout(timeout).WithPolling(interval).Should(Succeed())
				})

				When("service has a traffic policy annotation", func() {
					var (
						policy     *ngrokv1alpha1.NgrokTrafficPolicy
						policyName string
					)

					BeforeEach(func() {
						policyName = fmt.Sprintf("test-policy-collapsed-%d", rand.IntN(100000))
						policy = &ngrokv1alpha1.NgrokTrafficPolicy{
							ObjectMeta: metav1.ObjectMeta{
								Name:      policyName,
								Namespace: namespace,
							},
							Spec: ngrokv1alpha1.NgrokTrafficPolicySpec{
								Policy: json.RawMessage(`{"on_tcp_connect": [{"actions": [{"type": "restrict-ips", "config": {"deny": ["5.6.7.8/32"]}}]}]}`),
							},
						}
						Expect(k8sClient.Create(ctx, policy)).To(Succeed())

						modifiers.Add(AddAnnotation(Annotation_TrafficPolicy, policyName))
					})

					It("should apply traffic policy to the agent endpoint", func() {
						kginkgo.EventuallyWithAgentEndpoints(ctx, namespace, func(g Gomega, aeps []ngrokv1alpha1.AgentEndpoint) {
							By("checking an agent endpoint exists")
							g.Expect(aeps).To(HaveLen(1))

							By("checking the agent endpoint has the traffic policy")
							aep := aeps[0]
							g.Expect(aep.Spec.TrafficPolicy).NotTo(BeNil())
							g.Expect(string(aep.Spec.TrafficPolicy.Inline)).To(ContainSubstring("deny"))
							g.Expect(string(aep.Spec.TrafficPolicy.Inline)).To(ContainSubstring("5.6.7.8/32"))
						})
					})

					It("should update agent endpoint when traffic policy changes", func() {
						// Wait for initial reconciliation
						kginkgo.EventuallyWithAgentEndpoints(ctx, namespace, func(g Gomega, aeps []ngrokv1alpha1.AgentEndpoint) {
							By("checking an agent endpoint exists")
							g.Expect(aeps).To(HaveLen(1))

							By("checking the agent endpoint has the initial traffic policy")
							aep := aeps[0]
							g.Expect(aep.Spec.TrafficPolicy).NotTo(BeNil())
							g.Expect(string(aep.Spec.TrafficPolicy.Inline)).To(ContainSubstring("deny"))
						})

						// Update the policy
						fetched := &ngrokv1alpha1.NgrokTrafficPolicy{}
						Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(policy), fetched)).To(Succeed())
						fetched.Spec.Policy = json.RawMessage(`{"on_tcp_connect": [{"actions": [{"type": "restrict-ips", "config": {"allow": ["5.6.7.8/32"]}}]}]}`)
						Expect(k8sClient.Update(ctx, fetched)).To(Succeed())

						kginkgo.EventuallyWithAgentEndpoints(ctx, namespace, func(g Gomega, aeps []ngrokv1alpha1.AgentEndpoint) {
							By("checking an agent endpoint exists")
							g.Expect(aeps).To(HaveLen(1))

							By("checking the agent endpoint has the updated traffic policy")
							aep := aeps[0]
							g.Expect(aep.Spec.TrafficPolicy).NotTo(BeNil())
							g.Expect(string(aep.Spec.TrafficPolicy.Inline)).To(ContainSubstring("allow"))
						})
					})
				})
			})

			When("service type changes from LoadBalancer to ClusterIP", func() {
				BeforeEach(func() {
					modifiers.Add(SetMappingStrategy(annotations.MappingStrategy_EndpointsVerbose))
				})

				It("should clean up owned resources and remove finalizer", func() {
					// Wait for resources to be created
					kginkgo.EventuallyWithCloudEndpoints(ctx, namespace, func(g Gomega, cleps []ngrokv1alpha1.CloudEndpoint) {
						By("checking a cloud endpoint exists")
						g.Expect(cleps).To(HaveLen(1))
					})

					By("changing service type from LoadBalancer to ClusterIP")
					Eventually(func() error {
						fetched := &corev1.Service{}
						if err := k8sClient.Get(ctx, client.ObjectKeyFromObject(svc), fetched); err != nil {
							return err
						}
						fetched.Spec.Type = ClusterIP
						return k8sClient.Update(ctx, fetched)
					}, timeout, interval).Should(Succeed())

					// Verify resources are cleaned up
					kginkgo.EventuallyExpectNoEndpoints(ctx, namespace)

					// Verify finalizer is removed
					kginkgo.ExpectFinalizerToBeRemoved(ctx, svc, FinalizerName)
				})

				It("should ensure no cloudendpoints or agentendpoints remain after type change", func() {
					// Wait for initial resources to be created
					Eventually(func(g Gomega) {
						cleps, err := getCloudEndpoints(k8sClient, namespace)
						g.Expect(err).NotTo(HaveOccurred())

						By("verifying cloud endpoint was created initially")
						g.Expect(cleps.Items).To(HaveLen(1))

						aeps, err := getAgentEndpoints(k8sClient, namespace)
						g.Expect(err).NotTo(HaveOccurred())

						By("verifying agent endpoint was created initially")
						g.Expect(aeps.Items).To(HaveLen(1))
					}, timeout, interval).Should(Succeed())

					By("changing service type from LoadBalancer to ClusterIP")
					Eventually(func() error {
						fetched := &corev1.Service{}
						if err := k8sClient.Get(ctx, client.ObjectKeyFromObject(svc), fetched); err != nil {
							return err
						}
						fetched.Spec.Type = ClusterIP
						return k8sClient.Update(ctx, fetched)
					}, timeout, interval).Should(Succeed())

					// Verify all owned endpoints are completely cleaned up
					kginkgo.EventuallyExpectNoEndpoints(ctx, namespace)
				})
			})

			When("service with ngrok load balancer class is deleted", func() {
				BeforeEach(func() {
					modifiers.Add(SetMappingStrategy(annotations.MappingStrategy_EndpointsVerbose))
				})

				It("should clean up owned resources when service is deleted", func() {
					// Wait for resources to be created
					kginkgo.EventuallyWithCloudAndAgentEndpoints(ctx, namespace, func(g Gomega, cleps []ngrokv1alpha1.CloudEndpoint, aeps []ngrokv1alpha1.AgentEndpoint) {
						By("checking a cloud endpoint exists")
						g.Expect(cleps).To(HaveLen(1))
						By("checking an agent endpoint exists")
						g.Expect(aeps).To(HaveLen(1))
					})

					By("deleting the service")
					Expect(k8sClient.Delete(ctx, svc)).To(Succeed())

					// Verify resources are cleaned up
					kginkgo.EventuallyExpectNoEndpoints(ctx, namespace)

					// Verify finalizer is removed
					kginkgo.ExpectFinalizerToBeRemoved(ctx, svc, FinalizerName)
				})
			})

			When("service has explicit tcp:// URL annotation", func() {
				BeforeEach(func() {
					modifiers.Add(AddAnnotation(Annotation_URL, "tcp://1.tcp.ngrok.io:12345"))
				})

				It("should create an agent endpoint with the explicit TCP URL", func() {
					kginkgo.EventuallyWithAgentEndpoints(ctx, namespace, func(g Gomega, aeps []ngrokv1alpha1.AgentEndpoint) {
						By("checking an agent endpoint exists")
						g.Expect(aeps).To(HaveLen(1))

						By("checking the agent endpoint has the explicit TCP URL")
						aep := aeps[0]
						g.Expect(aep.Spec.URL).To(Equal("tcp://1.tcp.ngrok.io:12345"))
					})
				})

				It("should not create a cloud endpoint in default mapping", func() {
					kginkgo.ConsistentlyWithCloudEndpoints(ctx, namespace, func(g Gomega, cleps []ngrokv1alpha1.CloudEndpoint) {
						By("checking no cloud endpoints exist")
						g.Expect(cleps).To(BeEmpty())
					})
				})

				It("should have owner reference pointing to the service", func() {
					kginkgo.EventuallyWithAgentEndpoints(ctx, namespace, func(g Gomega, aeps []ngrokv1alpha1.AgentEndpoint) {
						By("checking an agent endpoint exists")
						g.Expect(aeps).To(HaveLen(1))

						By("checking the agent endpoint has correct owner reference")
						aep := aeps[0]
						g.Expect(aep.OwnerReferences).To(HaveLen(1))
						g.Expect(aep.OwnerReferences[0].Kind).To(Equal("Service"))
						g.Expect(aep.OwnerReferences[0].Name).To(Equal(svc.Name))
						g.Expect(aep.OwnerReferences[0].UID).To(Equal(svc.UID))
						g.Expect(*aep.OwnerReferences[0].Controller).To(BeTrue())
					})
				})
			})

			When("service has tls:// URL annotation", func() {
				BeforeEach(func() {
					modifiers.Add(AddAnnotation(Annotation_URL, "tls://example.ngrok.app"))
				})

				It("should create an agent endpoint with the TLS URL", func() {
					kginkgo.EventuallyWithAgentEndpoints(ctx, namespace, func(g Gomega, aeps []ngrokv1alpha1.AgentEndpoint) {
						By("checking an agent endpoint exists")
						g.Expect(aeps).To(HaveLen(1))

						By("checking the agent endpoint has the TLS URL")
						aep := aeps[0]
						g.Expect(aep.Spec.URL).To(Equal("tls://example.ngrok.app"))
					})
				})

				It("should NOT update service status (known divergence from spec)", func() {
					// This test documents a known divergence between the spec and implementation.
					// The spec says tls:// URLs should update status, but the implementation only
					// uses computed-url or domain annotations for status updates.
					// The controller does NOT currently parse k8s.ngrok.com/url for status.
					Consistently(func(g Gomega) {
						fetched := &corev1.Service{}
						err := k8sClient.Get(ctx, client.ObjectKeyFromObject(svc), fetched)
						g.Expect(err).NotTo(HaveOccurred())

						By("checking the service status is empty (not populated from url annotation)")
						g.Expect(fetched.Status.LoadBalancer.Ingress).To(BeEmpty())
					}, duration, interval).Should(Succeed())
				})

				When("with endpoints-verbose mapping", func() {
					BeforeEach(func() {
						modifiers.Add(SetMappingStrategy(annotations.MappingStrategy_EndpointsVerbose))
					})

					It("should create both cloud and agent endpoints", func() {
						kginkgo.EventuallyWithCloudAndAgentEndpoints(ctx, namespace, func(g Gomega, cleps []ngrokv1alpha1.CloudEndpoint, aeps []ngrokv1alpha1.AgentEndpoint) {
							By("checking a cloud endpoint exists")
							g.Expect(cleps).To(HaveLen(1))

							By("checking an agent endpoint exists")
							g.Expect(aeps).To(HaveLen(1))

							By("checking the cloud endpoint has the TLS URL")
							g.Expect(cleps[0].Spec.URL).To(Equal("tls://example.ngrok.app"))

							By("checking the agent endpoint URL has .internal suffix")
							g.Expect(aeps[0].Spec.URL).To(ContainSubstring(".internal"))
						})
					})
				})
			})

			When("service has URL annotation with TLS domain", func() {
				BeforeEach(func() {
					modifiers.Add(AddAnnotation(annotations.MappingStrategyAnnotation, string(annotations.MappingStrategy_EndpointsVerbose)))
					modifiers.Add(AddAnnotation(annotations.URLAnnotation, "tls://test.ngrok.app:443"))
				})

				It("should create a cloud endpoint with the specified domain", func() {
					kginkgo.EventuallyWithCloudEndpoints(ctx, namespace, func(g Gomega, cleps []ngrokv1alpha1.CloudEndpoint) {
						By("checking a cloud endpoint exists")
						g.Expect(cleps).To(HaveLen(1))

						By("checking the cloud endpoint has the correct URL with domain")
						clep := cleps[0]
						g.Expect(clep.Spec.URL).To(ContainSubstring("test.ngrok.app"))
					})
				})

				It("should update service status with the domain", func() {
					Eventually(func(g Gomega) {
						fetched := &corev1.Service{}
						err := k8sClient.Get(ctx, client.ObjectKeyFromObject(svc), fetched)
						g.Expect(err).NotTo(HaveOccurred())

						By("checking the service status uses the domain")
						g.Expect(fetched.Status.LoadBalancer.Ingress).NotTo(BeEmpty())
						g.Expect(fetched.Status.LoadBalancer.Ingress[0].Hostname).To(ContainSubstring("test.ngrok.app"))
					}, timeout, interval).Should(Succeed())
				})
			})

			When("service has a traffic policy annotation", func() {
				var (
					policy     *ngrokv1alpha1.NgrokTrafficPolicy
					policyName string
				)

				BeforeEach(func() {
					policyName = fmt.Sprintf("test-policy-%d", rand.IntN(100000))
					policy = &ngrokv1alpha1.NgrokTrafficPolicy{
						ObjectMeta: metav1.ObjectMeta{
							Name:      policyName,
							Namespace: namespace,
						},
						Spec: ngrokv1alpha1.NgrokTrafficPolicySpec{
							Policy: json.RawMessage(`{"on_tcp_connect": [{"actions": [{"type": "restrict-ips", "config": {"deny": ["1.2.3.4/32"]}}]}]}`),
						},
					}
					Expect(k8sClient.Create(ctx, policy)).To(Succeed())

					modifiers.Add(SetMappingStrategy(annotations.MappingStrategy_EndpointsVerbose))
					modifiers.Add(AddAnnotation(Annotation_TrafficPolicy, policyName))
				})

				It("should create a cloud endpoint with the traffic policy", func() {
					kginkgo.EventuallyWithCloudEndpoints(ctx, namespace, func(g Gomega, cleps []ngrokv1alpha1.CloudEndpoint) {
						By("checking a cloud endpoint exists")
						g.Expect(cleps).To(HaveLen(1))

						By("checking the cloud endpoint has the traffic policy")
						clep := cleps[0]
						g.Expect(clep.Spec.TrafficPolicy).NotTo(BeNil())
					})
				})

				When("the traffic policy is updated", func() {
					It("should trigger service reconciliation", func() {
						// Wait for initial reconciliation
						kginkgo.EventuallyWithCloudAndAgentEndpoints(ctx, namespace, func(g Gomega, cleps []ngrokv1alpha1.CloudEndpoint, aeps []ngrokv1alpha1.AgentEndpoint) {
							By("checking a cloud endpoint exists")
							g.Expect(cleps).To(HaveLen(1))

							By("checking an agent endpoint exists")
							g.Expect(aeps).To(HaveLen(1))

							By("checking the cloud endpoint has the initial traffic policy")
							clep := cleps[0]
							g.Expect(clep.Spec.TrafficPolicy).NotTo(BeNil())
							g.Expect(string(clep.Spec.TrafficPolicy.Policy)).To(ContainSubstring("deny"))
						})

						// Update the policy
						fetched := &ngrokv1alpha1.NgrokTrafficPolicy{}
						Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(policy), fetched)).To(Succeed())
						fetched.Spec.Policy = json.RawMessage(`{"on_tcp_connect": [{"actions": [{"type": "restrict-ips", "config": {"allow": ["1.2.3.4/32"]}}]}]}`)
						Expect(k8sClient.Update(ctx, fetched)).To(Succeed())

						kginkgo.EventuallyWithCloudAndAgentEndpoints(ctx, namespace, func(g Gomega, cleps []ngrokv1alpha1.CloudEndpoint, aeps []ngrokv1alpha1.AgentEndpoint) {
							By("checking a cloud endpoint exists")
							g.Expect(cleps).To(HaveLen(1))

							By("checking an agent endpoint exists")
							g.Expect(aeps).To(HaveLen(1))

							By("checking the cloud endpoint has the updated traffic policy")
							clep := cleps[0]
							g.Expect(clep.Spec.TrafficPolicy).NotTo(BeNil())
							g.Expect(string(clep.Spec.TrafficPolicy.Policy)).To(ContainSubstring("allow"))
						})
					})
				})
			})

			When("multiple resources are owned by the service (error condition)", func() {
				BeforeEach(func() {
					modifiers.Add(SetMappingStrategy(annotations.MappingStrategy_EndpointsVerbose))
				})

				It("should delete extra resources keeping only one", func(ctx SpecContext) {
					// Wait for first resource to be created
					kginkgo.EventuallyWithCloudEndpoints(ctx, namespace, func(g Gomega, cleps []ngrokv1alpha1.CloudEndpoint) {
						By("checking a cloud endpoint exists")
						g.Expect(cleps).To(HaveLen(1))
					})

					// Manually create a second cloud endpoint owned by the service
					fetched := &corev1.Service{}
					Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(svc), fetched)).To(Succeed())

					extraClep := &ngrokv1alpha1.CloudEndpoint{
						ObjectMeta: metav1.ObjectMeta{
							Name:      fmt.Sprintf("%s-extra", svc.Name),
							Namespace: namespace,
							OwnerReferences: []metav1.OwnerReference{
								{
									APIVersion: "v1",
									Kind:       "Service",
									Name:       fetched.Name,
									UID:        fetched.UID,
									Controller: ptr.To(true),
								},
							},
						},
						Spec: ngrokv1alpha1.CloudEndpointSpec{
							URL: "tcp://1.tcp.ngrok.io:12345",
						},
					}
					Expect(k8sClient.Create(ctx, extraClep)).To(Succeed())

					// Verify only one cloud endpoint remains
					kginkgo.EventuallyWithCloudEndpoints(ctx, namespace, func(g Gomega, cleps []ngrokv1alpha1.CloudEndpoint) {
						By("checking only one cloud endpoint exists")
						g.Expect(cleps).To(HaveLen(1))
					})
				})
			})
		})
	})
})
