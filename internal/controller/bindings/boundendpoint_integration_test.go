package bindings

import (
	"context"
	"time"

	"github.com/ngrok/ngrok-api-go/v9"
	bindingsv1alpha1 "github.com/ngrok/ngrok-operator/api/bindings/v1alpha1"
	"github.com/ngrok/ngrok-operator/internal/ngrokapi"
	"github.com/ngrok/ngrok-operator/internal/testutils"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var _ = Describe("BoundEndpoint Controller", func() {
	const (
		timeout  = 30 * time.Second
		interval = 500 * time.Millisecond
	)

	var (
		testCtx context.Context
	)

	BeforeEach(func() {
		testCtx = ctx
		resetMockEndpoints()
	})

	AfterEach(func() {
		// Clean up all BoundEndpoints
		boundEndpoints := &bindingsv1alpha1.BoundEndpointList{}
		err := k8sClient.List(testCtx, boundEndpoints, &client.ListOptions{
			Namespace: pollerController.Namespace,
		})
		Expect(err).NotTo(HaveOccurred())

		for _, be := range boundEndpoints.Items {
			_ = k8sClient.Delete(testCtx, &be)
		}

		// Wait for cleanup
		Eventually(func(g Gomega) {
			list := &bindingsv1alpha1.BoundEndpointList{}
			err := k8sClient.List(testCtx, list, &client.ListOptions{
				Namespace: pollerController.Namespace,
			})
			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(list.Items).To(BeEmpty())
		}, timeout, interval).Should(Succeed())

		resetMockEndpoints()
	})

	Context("Single endpoint", func() {
		It("should create services and set conditions", func(ctx SpecContext) {
			By("Creating target namespace")
			kginkgo.ExpectCreateNamespace(ctx, "test-namespace")
			defer kginkgo.ExpectDeleteNamespace(ctx, "test-namespace")

			By("Setting up mock API with one projected endpoint")
			setMockEndpoints([]ngrok.Endpoint{
				{
					ID:       "ep_abc123",
					URI:      "https://api.ngrok.com/endpoints/ep_abc123",
					URL:      "tcp://test.internal:8080",
					Proto:    "tcp",
					Bindings: []string{"internal"},
					Kubernetes: kubernetesTargets(ngrok.EndpointKubernetesTarget{
						Service: "test-service", Namespace: "test-namespace", Port: 8080,
					}),
				},
			})

			By("Triggering poller to create BoundEndpoint")
			err := triggerPoller(testCtx)
			Expect(err).NotTo(HaveOccurred())

			By("Waiting for BoundEndpoint to be created")
			var boundEndpointName string
			Eventually(func(g Gomega) {
				list := &bindingsv1alpha1.BoundEndpointList{}
				err := k8sClient.List(testCtx, list, &client.ListOptions{
					Namespace: pollerController.Namespace,
				})
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(list.Items).To(HaveLen(1))

				be := list.Items[0]
				boundEndpointName = be.Name

				// The endpoint keeps its own url as the dial identity; the
				// projection comes from the target.
				g.Expect(be.Spec.EndpointURL).To(Equal("tcp://test.internal:8080"))
				g.Expect(be.Spec.Target.Service).To(Equal("test-service"))
				g.Expect(be.Spec.Target.Namespace).To(Equal("test-namespace"))

				// Poller should have set these fields
				g.Expect(be.Status.Endpoints).To(HaveLen(1))
				g.Expect(be.Status.Endpoints[0].ID).To(Equal("ep_abc123"))
				g.Expect(be.Status.EndpointsSummary).To(Equal("1 endpoint"))
				g.Expect(be.Status.HashedName).NotTo(BeEmpty())
			}, timeout, interval).Should(Succeed())

			By("Waiting for controller to create services and set conditions")
			Eventually(func(g Gomega) {
				be := &bindingsv1alpha1.BoundEndpoint{}
				err := k8sClient.Get(testCtx, types.NamespacedName{
					Name:      boundEndpointName,
					Namespace: pollerController.Namespace,
				}, be)
				g.Expect(err).NotTo(HaveOccurred())

				// Check ServicesCreated condition
				servicesCreatedCond := testutils.FindCondition(be.Status.Conditions, ConditionTypeServicesCreated)
				g.Expect(servicesCreatedCond).NotTo(BeNil(), "ServicesCreated condition should exist")
				g.Expect(servicesCreatedCond.Status).To(Equal(metav1.ConditionTrue), "ServicesCreated should be True")

				// Check service references are set
				g.Expect(be.Status.TargetServiceRef).NotTo(BeNil(), "TargetServiceRef should be set")
				g.Expect(be.Status.TargetServiceRef.Name).To(Equal("test-service"))
				g.Expect(be.Status.TargetServiceRef.Namespace).NotTo(BeNil())
				g.Expect(*be.Status.TargetServiceRef.Namespace).To(Equal("test-namespace"))

				g.Expect(be.Status.UpstreamServiceRef).NotTo(BeNil(), "UpstreamServiceRef should be set")
				g.Expect(be.Status.UpstreamServiceRef.Name).NotTo(BeEmpty())

				// NOTE: Ready condition will be False in test env because connectivity check fails
				// (no actual service to dial). We just verify the condition exists and services were created.
				readyCond := testutils.FindCondition(be.Status.Conditions, ConditionTypeReady)
				g.Expect(readyCond).NotTo(BeNil(), "Ready condition should exist")
			}, timeout, interval).Should(Succeed())

			By("Verifying target service was created in user namespace")
			targetSvc := &v1.Service{}
			err = k8sClient.Get(testCtx, types.NamespacedName{
				Name:      "test-service",
				Namespace: "test-namespace",
			}, targetSvc)
			Expect(err).NotTo(HaveOccurred())
			Expect(targetSvc.Spec.Type).To(Equal(v1.ServiceTypeExternalName))

			By("Verifying upstream service was created in operator namespace")
			be := &bindingsv1alpha1.BoundEndpoint{}
			err = k8sClient.Get(testCtx, types.NamespacedName{
				Name:      boundEndpointName,
				Namespace: pollerController.Namespace,
			}, be)
			Expect(err).NotTo(HaveOccurred())

			upstreamSvc := &v1.Service{}
			err = k8sClient.Get(testCtx, types.NamespacedName{
				Name:      be.Status.UpstreamServiceRef.Name,
				Namespace: pollerController.Namespace,
			}, upstreamSvc)
			Expect(err).NotTo(HaveOccurred())
			Expect(upstreamSvc.Spec.Type).To(Equal(v1.ServiceTypeClusterIP))
		})
	})

	Context("Endpoints without a projection", func() {
		It("should ignore endpoints that carry no kubernetes targets", func(_ SpecContext) {
			By("Setting up mock API with an endpoint that is not projected")
			setMockEndpoints([]ngrok.Endpoint{
				{
					ID:       "ep_notprojected",
					URI:      "https://api.ngrok.com/endpoints/ep_notprojected",
					URL:      "https://example.ngrok.app",
					Proto:    "https",
					Bindings: []string{"public"},
				},
			})

			By("Triggering poller")
			Expect(triggerPoller(testCtx)).To(Succeed())

			By("Verifying no BoundEndpoint was created")
			Consistently(func(g Gomega) {
				list := &bindingsv1alpha1.BoundEndpointList{}
				err := k8sClient.List(testCtx, list, &client.ListOptions{
					Namespace: pollerController.Namespace,
				})
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(list.Items).To(BeEmpty())
			}, 3*time.Second, interval).Should(Succeed())
		})
	})

	Context("Multiple targets", func() {
		It("should project one endpoint into every target namespace", func(ctx SpecContext) {
			By("Creating target namespaces")
			kginkgo.ExpectCreateNamespace(ctx, "team-a")
			defer kginkgo.ExpectDeleteNamespace(ctx, "team-a")
			kginkgo.ExpectCreateNamespace(ctx, "team-b")
			defer kginkgo.ExpectDeleteNamespace(ctx, "team-b")

			By("Setting up mock API with one endpoint projected into two namespaces")
			setMockEndpoints([]ngrok.Endpoint{
				{
					ID:       "ep_multi",
					URI:      "https://api.ngrok.com/endpoints/ep_multi",
					URL:      "tcp://echo.internal:80",
					Proto:    "tcp",
					Bindings: []string{"internal"},
					Kubernetes: kubernetesTargets(
						ngrok.EndpointKubernetesTarget{Service: "myservice", Namespace: "team-a", Port: 80},
						ngrok.EndpointKubernetesTarget{Service: "myservice", Namespace: "team-b", Port: 80},
					),
				},
			})

			By("Triggering poller")
			Expect(triggerPoller(testCtx)).To(Succeed())

			By("Waiting for one BoundEndpoint per target")
			Eventually(func(g Gomega) {
				list := &bindingsv1alpha1.BoundEndpointList{}
				err := k8sClient.List(testCtx, list, &client.ListOptions{
					Namespace: pollerController.Namespace,
				})
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(list.Items).To(HaveLen(2))

				byNamespace := map[string]bindingsv1alpha1.BoundEndpoint{}
				ports := map[uint16]struct{}{}
				for _, be := range list.Items {
					byNamespace[be.Spec.Target.Namespace] = be
					ports[be.Spec.Port] = struct{}{}
				}
				g.Expect(byNamespace).To(HaveKey("team-a"))
				g.Expect(byNamespace).To(HaveKey("team-b"))

				// Both projections dial the same endpoint...
				g.Expect(byNamespace["team-a"].Spec.EndpointURL).To(Equal("tcp://echo.internal:80"))
				g.Expect(byNamespace["team-b"].Spec.EndpointURL).To(Equal("tcp://echo.internal:80"))
				// ...on their own forwarder port, which is why they need
				// distinct BoundEndpoints.
				g.Expect(ports).To(HaveLen(2))

				g.Expect(byNamespace["team-a"].Name).To(Equal(ngrokapi.BoundEndpointName("ep_multi", "myservice", "team-a")))
				g.Expect(byNamespace["team-b"].Name).To(Equal(ngrokapi.BoundEndpointName("ep_multi", "myservice", "team-b")))
			}, timeout, interval).Should(Succeed())

			By("Verifying a target service was created in each namespace")
			Eventually(func(g Gomega) {
				for _, ns := range []string{"team-a", "team-b"} {
					targetSvc := &v1.Service{}
					err := k8sClient.Get(testCtx, types.NamespacedName{Name: "myservice", Namespace: ns}, targetSvc)
					g.Expect(err).NotTo(HaveOccurred(), "target service missing in %s", ns)
					g.Expect(targetSvc.Spec.Type).To(Equal(v1.ServiceTypeExternalName))
				}
			}, timeout, interval).Should(Succeed())

			By("Removing one target from the endpoint")
			setMockEndpoints([]ngrok.Endpoint{
				{
					ID:       "ep_multi",
					URI:      "https://api.ngrok.com/endpoints/ep_multi",
					URL:      "tcp://echo.internal:80",
					Proto:    "tcp",
					Bindings: []string{"internal"},
					Kubernetes: kubernetesTargets(
						ngrok.EndpointKubernetesTarget{Service: "myservice", Namespace: "team-a", Port: 80},
					),
				},
			})
			Expect(triggerPoller(testCtx)).To(Succeed())

			By("Verifying the dropped target's BoundEndpoint is removed and the kept one stays")
			Eventually(func(g Gomega) {
				list := &bindingsv1alpha1.BoundEndpointList{}
				err := k8sClient.List(testCtx, list, &client.ListOptions{
					Namespace: pollerController.Namespace,
				})
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(list.Items).To(HaveLen(1))
				g.Expect(list.Items[0].Spec.Target.Namespace).To(Equal("team-a"))
			}, timeout, interval).Should(Succeed())
		})
	})

	Context("Status updates", func() {
		It("should not get stuck in provisioning when the endpoint changes", func(ctx SpecContext) {
			By("Creating target namespace")
			kginkgo.ExpectCreateNamespace(ctx, "status-namespace")
			defer kginkgo.ExpectDeleteNamespace(ctx, "status-namespace")

			endpoint := ngrok.Endpoint{
				ID:       "ep_initial",
				URI:      "https://api.ngrok.com/endpoints/ep_initial",
				URL:      "tcp://my-app.internal:8080",
				Proto:    "tcp",
				Bindings: []string{"internal"},
				Kubernetes: kubernetesTargets(ngrok.EndpointKubernetesTarget{
					Service: "my-app", Namespace: "status-namespace", Port: 8080,
				}),
			}

			By("Setting up mock API with one endpoint initially")
			setMockEndpoints([]ngrok.Endpoint{endpoint})

			By("Triggering poller to create initial BoundEndpoint")
			Expect(triggerPoller(testCtx)).To(Succeed())

			By("Waiting for services to be created")
			var boundEndpointName string
			Eventually(func(g Gomega) {
				list := &bindingsv1alpha1.BoundEndpointList{}
				err := k8sClient.List(testCtx, list, &client.ListOptions{
					Namespace: pollerController.Namespace,
				})
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(list.Items).To(HaveLen(1))

				be := list.Items[0]
				boundEndpointName = be.Name

				servicesCreatedCond := testutils.FindCondition(be.Status.Conditions, ConditionTypeServicesCreated)
				g.Expect(servicesCreatedCond).NotTo(BeNil())
				g.Expect(servicesCreatedCond.Status).To(Equal(metav1.ConditionTrue))
			}, timeout, interval).Should(Succeed())

			By("Changing the target port on the endpoint")
			endpoint.Kubernetes = kubernetesTargets(ngrok.EndpointKubernetesTarget{
				Service: "my-app", Namespace: "status-namespace", Port: 9090,
			})
			setMockEndpoints([]ngrok.Endpoint{endpoint})

			By("Triggering poller to update BoundEndpoint")
			Expect(triggerPoller(testCtx)).To(Succeed())

			By("Verifying ServicesCreated condition stays True (not reset to provisioning)")
			Eventually(func(g Gomega) {
				be := &bindingsv1alpha1.BoundEndpoint{}
				err := k8sClient.Get(testCtx, types.NamespacedName{
					Name:      boundEndpointName,
					Namespace: pollerController.Namespace,
				}, be)
				g.Expect(err).NotTo(HaveOccurred())

				// The BoundEndpoint keeps its name, because the endpoint and
				// the projection target are unchanged.
				g.Expect(be.Spec.Target.Port).To(Equal(int32(9090)))

				// KEY TEST: ServicesCreated condition should remain True
				servicesCreatedCond := testutils.FindCondition(be.Status.Conditions, ConditionTypeServicesCreated)
				g.Expect(servicesCreatedCond).NotTo(BeNil())
				g.Expect(servicesCreatedCond.Status).To(Equal(metav1.ConditionTrue),
					"ServicesCreated should stay True after the target changes")
			}, timeout, interval).Should(Succeed())
		})
	})

	Context("Error handling", func() {
		It("should set ServicesCreated condition to False when target namespace missing", func() {
			By("NOT creating target namespace - this will cause service creation to fail")

			By("Setting up mock API with endpoint pointing to non-existent namespace")
			setMockEndpoints([]ngrok.Endpoint{
				{
					ID:       "ep_missing_ns",
					URI:      "https://api.ngrok.com/endpoints/ep_missing_ns",
					URL:      "tcp://my-service.internal:8080",
					Proto:    "tcp",
					Bindings: []string{"internal"},
					Kubernetes: kubernetesTargets(ngrok.EndpointKubernetesTarget{
						Service: "my-service", Namespace: "missing-namespace", Port: 8080,
					}),
				},
			})

			By("Triggering poller to create BoundEndpoint")
			err := triggerPoller(testCtx)
			Expect(err).NotTo(HaveOccurred())

			By("Waiting for BoundEndpoint to be created")
			var boundEndpointName string
			Eventually(func(g Gomega) {
				list := &bindingsv1alpha1.BoundEndpointList{}
				err := k8sClient.List(testCtx, list, &client.ListOptions{
					Namespace: pollerController.Namespace,
				})
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(list.Items).To(HaveLen(1))
				boundEndpointName = list.Items[0].Name
			}, timeout, interval).Should(Succeed())

			By("Verifying ServicesCreated condition is False with namespace error")
			Eventually(func(g Gomega) {
				be := &bindingsv1alpha1.BoundEndpoint{}
				err := k8sClient.Get(testCtx, types.NamespacedName{
					Name:      boundEndpointName,
					Namespace: pollerController.Namespace,
				}, be)
				g.Expect(err).NotTo(HaveOccurred())

				servicesCreatedCond := testutils.FindCondition(be.Status.Conditions, ConditionTypeServicesCreated)
				g.Expect(servicesCreatedCond).NotTo(BeNil())
				g.Expect(servicesCreatedCond.Status).To(Equal(metav1.ConditionFalse))
				g.Expect(servicesCreatedCond.Reason).To(Equal(ReasonServiceCreationFailed))
				g.Expect(servicesCreatedCond.Message).To(ContainSubstring("namespace"))

				// Ready should also be False
				readyCond := testutils.FindCondition(be.Status.Conditions, ConditionTypeReady)
				g.Expect(readyCond).NotTo(BeNil())
				g.Expect(readyCond.Status).To(Equal(metav1.ConditionFalse))
			}, timeout, interval).Should(Succeed())
		})
	})

	Context("Schema validation", func() {
		It("should reject a BoundEndpoint without endpointURL", func(ctx SpecContext) {
			be := &bindingsv1alpha1.BoundEndpoint{
				Name:      "missing-endpoint-url",
				Namespace: pollerController.Namespace,
				Spec: bindingsv1alpha1.BoundEndpointSpec{
					Scheme: "https",
					Port:   8080,
					Target: bindingsv1alpha1.EndpointTarget{
						Service:   "test-service",
						Namespace: "test-namespace",
						Protocol:  "TCP",
						Port:      8080,
					},
				},
			}

			err := k8sClient.Create(ctx, be)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("endpointURL"))
		})

		It("should accept a .internal endpointURL", func(ctx SpecContext) {
			// The dial host is now the endpoint's own hostname, so the schema
			// must take `.internal` names -- including names with more than
			// the two labels the old service.namespace pattern allowed.
			for i, url := range []string{
				"tcp://foo.internal:80",
				"tcp://foo.bar.internal:80",
				"https://foo.internal",
			} {
				be := &bindingsv1alpha1.BoundEndpoint{
					Name:      "internal-url-" + string(rune('a'+i)),
					Namespace: pollerController.Namespace,
					Spec: bindingsv1alpha1.BoundEndpointSpec{
						EndpointURL: url,
						Scheme:      "tcp",
						Port:        uint16(19000 + i),
						Target: bindingsv1alpha1.EndpointTarget{
							Service:   "test-service",
							Namespace: "test-namespace",
							Protocol:  "TCP",
							Port:      8080,
						},
					},
				}

				Expect(k8sClient.Create(ctx, be)).To(Succeed(), "should accept %s", url)
			}
		})
	})
})
